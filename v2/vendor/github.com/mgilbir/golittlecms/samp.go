package lcms2

import "math"

// This file ports the space-slicing helpers from src/cmssamp.c (lcms2 2.19):
// cmsSliceSpace16 and cmsSliceSpaceFloat, which sweep an N-dimensional grid and
// call a sampler at every knot (used to build CLUTs by sampling a function),
// plus the black-point detection routines cmsDetectBlackPoint and
// cmsDetectDestinationBlackPoint and their helpers.
//
// PORTNOTES (black-point detection — W11):
//
//   The reference black-point detectors evaluate the profile through a full
//   cmsHTRANSFORM built with cmsCreateTransformTHR / cmsCreateExtendedTransform
//   and run with cmsFLAGS_NOOPTIMIZE|cmsFLAGS_NOCACHE (so the transform is a
//   plain floating-point walk of the concatenated pipeline). The transform
//   engine (cmsxform.c) is W12 and is not available yet, and the Lab virtual
//   profiles it connects to (cmsCreateLab2/4Profile) come from cmsvirt.c (W15).
//
//   Rather than stub the whole path, the two transforms these routines need are
//   reconstructed directly from pipelines, reproducing the NOOPTIMIZE float
//   behaviour numerically:
//
//     * device -> Lab (BlackPointAsDarkerColorant): ReadInputLUT(intent) gives
//       device -> encoded-PCS; addConversion glues encoded-PCS -> encoded-Lab
//       (inserting XYZ2Lab when the PCS is XYZ); a normalize-to-Lab matrix
//       decodes encoded V4 Lab to real CIELab. This is what the hInput->Lab
//       transform computes for a 16-bit device input, sans the int quantization
//       that NOOPTIMIZE already bypasses.
//
//     * Lab -> device -> Lab round trip (CreateRoundtripXForm): a
//       normalize-from-Lab matrix encodes real Lab to V4; addConversion glues
//       to the profile PCS; ReadOutputLUT(nIntent) then ReadInputLUT(relcol)
//       walk PCS -> device -> PCS; addConversion + normalize-to-Lab decode back
//       to real Lab. This is the [Lab4, P, P, Lab4] chain the reference builds,
//       with the Lab4 endpoints collapsed into the encode/decode matrices.
//
//   The #ifdef CMS_USE_PROFILE_BLACK_POINT_TAG branch of cmsDetectBlackPoint is
//   off by default in the reference build and is not ported (documented here).
//   Differential coverage is provided by the oracle `blackpoint` command.

// SliceSpace16 ports cmsSliceSpace16: sweep the whole input space defined by
// clutPoints (one node count per input) and call sampler at each knot with the
// quantized 16-bit coordinates. The sampler receives a nil output slice (this
// sweep only produces inputs). Returns false when the sweep is aborted by the
// sampler or the geometry is invalid.
func SliceSpace16(nInputs uint32, clutPoints []uint32, sampler Sampler16, cargo any) bool {
	if nInputs >= maxChannels {
		return false
	}
	if uint32(len(clutPoints)) < nInputs {
		return false
	}

	nTotalPoints := cubeSize(clutPoints, nInputs)
	if nTotalPoints == 0 {
		return false
	}

	var in [maxChannels]uint16

	for i := uint32(0); i < nTotalPoints; i++ {
		rest := i
		for t := int(nInputs) - 1; t >= 0; t-- {
			colorant := rest % clutPoints[t]
			rest /= clutPoints[t]
			in[t] = quantizeVal(float64(colorant), clutPoints[t])
		}

		if !sampler(in[:nInputs], nil, cargo) {
			return false
		}
	}

	return true
}

// SliceSpaceFloat ports cmsSliceSpaceFloat: the floating-point counterpart of
// SliceSpace16. Coordinates are quantized to 16 bits then scaled to 0..1.
func SliceSpaceFloat(nInputs uint32, clutPoints []uint32, sampler SamplerFloat, cargo any) bool {
	if nInputs >= maxChannels {
		return false
	}
	if uint32(len(clutPoints)) < nInputs {
		return false
	}

	nTotalPoints := cubeSize(clutPoints, nInputs)
	if nTotalPoints == 0 {
		return false
	}

	var in [maxChannels]float32

	for i := uint32(0); i < nTotalPoints; i++ {
		rest := i
		for t := int(nInputs) - 1; t >= 0; t-- {
			colorant := rest % clutPoints[t]
			rest /= clutPoints[t]
			in[t] = float32(quantizeVal(float64(colorant), clutPoints[t])) / 65535.0
		}

		if !sampler(in[:nInputs], nil, cargo) {
			return false
		}
	}

	return true
}

// ---------------------------------------------------------------------------
// Black point detection (cmssamp.c)
// ---------------------------------------------------------------------------

// Perceptual black, mirroring cmsPERCEPTUAL_BLACK_{X,Y,Z} in include/lcms2.h.
// It is the fixed black point of V4 profiles for perceptual and saturation
// intents.
const (
	perceptualBlackX = 0.00336
	perceptualBlackY = 0.0034731
	perceptualBlackZ = 0.00287
)

// isInkColorspace ports isInkColorspace: reports whether c is an ink (subtractive,
// N-colorant) color space.
func isInkColorspace(c ColorSpaceSignature) bool {
	switch c {
	case SigCmykData, SigCmyData,
		SigMCH1Data, SigMCH2Data, SigMCH3Data, SigMCH4Data, SigMCH5Data,
		SigMCH6Data, SigMCH7Data, SigMCH8Data, SigMCH9Data, SigMCHAData,
		SigMCHBData, SigMCHCData, SigMCHDData, SigMCHEData, SigMCHFData,
		Sig1colorData, Sig2colorData, Sig3colorData, Sig4colorData, Sig5colorData,
		Sig6colorData, Sig7colorData, Sig8colorData, Sig9colorData, Sig10colorData,
		Sig11colorData, Sig12colorData, Sig13colorData, Sig14colorData, Sig15colorData:
		return true
	default:
		return false
	}
}

// buildDeviceToLabPipeline reconstructs the hInput -> Lab transform of
// BlackPointAsDarkerColorant as a pure float pipeline (see the PORTNOTES at the
// head of this file). The returned pipeline maps device values (0..1 per
// channel) to real CIELab.
func (p *Profile) buildDeviceToLabPipeline(intent uint32) (*Pipeline, error) {
	ctx := p.ContextID
	lut, err := p.ReadInputLUT(intent)
	if err != nil || lut == nil {
		return nil, err
	}
	// Glue the profile PCS to encoded V4 Lab (identity conversion layer).
	if err := addConversion(lut, p.GetPCS(), SigLabData, MAT3Identity(), VEC3{}); err != nil {
		return nil, err
	}
	// Decode encoded V4 Lab into real CIELab.
	dec, err := ctx.stageNormalizeToLabFloat()
	if err != nil {
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, dec); err != nil {
		return nil, err
	}
	return lut, nil
}

// buildRoundtripPipeline reconstructs CreateRoundtripXForm(hProfile, nIntent) as
// a pure float pipeline: real Lab -> device -> real Lab (see the PORTNOTES).
func (p *Profile) buildRoundtripPipeline(nIntent uint32) (*Pipeline, error) {
	ctx := p.ContextID

	result, err := ctx.PipelineAlloc(3, 3)
	if err != nil {
		return nil, err
	}

	// real Lab -> encoded V4 Lab.
	enc, err := ctx.stageNormalizeFromLabFloat()
	if err != nil {
		return nil, err
	}
	if err := result.InsertStage(AtEnd, enc); err != nil {
		return nil, err
	}

	// encoded Lab -> profile PCS.
	if err := addConversion(result, SigLabData, p.GetPCS(), MAT3Identity(), VEC3{}); err != nil {
		return nil, err
	}

	// PCS -> device (output direction, nIntent).
	outLut, err := p.ReadOutputLUT(nIntent)
	if err != nil || outLut == nil {
		return nil, err
	}
	if err := result.Cat(outLut); err != nil {
		return nil, err
	}

	// device -> PCS (input direction, relative colorimetric).
	inLut, err := p.ReadInputLUT(IntentRelativeColorimetric)
	if err != nil || inLut == nil {
		return nil, err
	}
	if err := result.Cat(inLut); err != nil {
		return nil, err
	}

	// profile PCS -> encoded V4 Lab.
	if err := addConversion(result, p.GetPCS(), SigLabData, MAT3Identity(), VEC3{}); err != nil {
		return nil, err
	}

	// encoded V4 Lab -> real Lab.
	dec, err := ctx.stageNormalizeToLabFloat()
	if err != nil {
		return nil, err
	}
	if err := result.InsertStage(AtEnd, dec); err != nil {
		return nil, err
	}

	return result, nil
}

// blackPointAsDarkerColorant ports BlackPointAsDarkerColorant: obtain the black
// point by pushing the darkest colorant of the device space through the profile
// to Lab, forcing it neutral and clipping L*.
func (p *Profile) blackPointAsDarkerColorant(intent uint32) (CIEXYZ, bool) {
	// If the profile does not support input direction, assume black point 0.
	if !p.IsIntentSupported(intent, UsedAsInput) {
		return CIEXYZ{}, false
	}

	space := p.GetColorSpace()
	nFmtChannels := ChannelsOf(space)

	_, black, nChannels, ok := EndPointsBySpace(space)
	if !ok {
		return CIEXYZ{}, false
	}
	if nChannels != nFmtChannels {
		return CIEXYZ{}, false
	}
	if black == nil || uint32(len(black)) < nChannels {
		return CIEXYZ{}, false
	}

	lut, err := p.buildDeviceToLabPipeline(intent)
	if err != nil || lut == nil {
		return CIEXYZ{}, false
	}

	// Convert the 16-bit darker colorant to device floats and evaluate to Lab.
	var in, out [maxChannels]float32
	for i := uint32(0); i < nChannels; i++ {
		in[i] = float32(black[i]) / 65535.0
	}
	lut.EvalFloat(in[:lut.InputChannels], out[:])

	var lab CIELab
	lab.L = float64(out[0])
	// Force it to be neutral, check for inconsistencies.
	lab.A = 0
	lab.B = 0

	if lab.L > 95 {
		lab.L = 0 // for synthetical negative profiles
	} else if lab.L < 0 {
		lab.L = 0
	} else if lab.L > 50 {
		lab.L = 50
	}

	return Lab2XYZ(nil, lab), true
}

// blackPointUsingPerceptualBlack ports BlackPointUsingPerceptualBlack: get the
// black point of an output CMYK profile discounting ink limiting, by running
// Lab(0,0,0) through a perceptual round trip.
func (p *Profile) blackPointUsingPerceptualBlack() (CIEXYZ, bool) {
	// Is the intent supported by the profile?
	if !p.IsIntentSupported(IntentPerceptual, UsedAsInput) {
		return CIEXYZ{}, true
	}

	roundTrip, err := p.buildRoundtripPipeline(IntentPerceptual)
	if err != nil || roundTrip == nil {
		return CIEXYZ{}, false
	}

	in := [3]float32{0, 0, 0}
	var out [maxChannels]float32
	roundTrip.EvalFloat(in[:], out[:])

	var labOut CIELab
	labOut.L = float64(out[0])
	// Clip Lab to reasonable limits.
	if labOut.L > 50 {
		labOut.L = 50
	}
	labOut.A = 0
	labOut.B = 0

	return Lab2XYZ(nil, labOut), true
}

// DetectBlackPoint ports cmsDetectBlackPoint. It returns the profile's source
// black point (relative to D50) and whether the detection succeeded; on failure
// the returned XYZ is zero, mirroring the reference which zeroes the point.
func (p *Profile) DetectBlackPoint(intent, dwFlags uint32) (CIEXYZ, bool) {
	// Make sure the device class is adequate.
	devClass := p.GetDeviceClass()
	if devClass == SigLinkClass || devClass == SigAbstractClass || devClass == SigNamedColorClass {
		return CIEXYZ{}, false
	}

	// Make sure intent is adequate.
	if intent != IntentPerceptual && intent != IntentRelativeColorimetric && intent != IntentSaturation {
		return CIEXYZ{}, false
	}

	// v4 + perceptual & saturation intents have their own, well-specified black
	// point. The black point tag is deprecated in V4.
	if p.GetEncodedICCversion() >= 0x4000000 &&
		(intent == IntentPerceptual || intent == IntentSaturation) {

		// Matrix shaper shares MRC & perceptual intents.
		if p.IsMatrixShaper() {
			return p.blackPointAsDarkerColorant(IntentRelativeColorimetric)
		}
		return CIEXYZ{X: perceptualBlackX, Y: perceptualBlackY, Z: perceptualBlackZ}, true
	}

	// CMS_USE_PROFILE_BLACK_POINT_TAG is off by default in the reference build;
	// the media-black-point-tag branch is intentionally not ported (see PORTNOTES).

	// If output profile, discount ink limiting and that's all.
	if intent == IntentRelativeColorimetric &&
		p.GetDeviceClass() == SigOutputClass &&
		isInkColorspace(p.GetColorSpace()) {
		return p.blackPointUsingPerceptualBlack()
	}

	// Otherwise compute BP using the current intent.
	return p.blackPointAsDarkerColorant(intent)
}

// rootOfLeastSquaresFitQuadraticCurve ports RootOfLeastSquaresFitQuadraticCurve:
// least-squares fit of a quadratic to (x,y) and return the clamped vertex root.
func rootOfLeastSquaresFitQuadraticCurve(n int, x, y []float64) float64 {
	if n < 4 {
		return 0
	}

	var sumX, sumX2, sumX3, sumX4 float64
	var sumY, sumYX, sumYX2 float64

	for i := 0; i < n; i++ {
		xn := x[i]
		yn := y[i]

		sumX += xn
		sumX2 += xn * xn
		sumX3 += xn * xn * xn
		sumX4 += xn * xn * xn * xn

		sumY += yn
		sumYX += yn * xn
		sumYX2 += yn * xn * xn
	}

	m := MAT3{
		VEC3Init(float64(n), sumX, sumX2),
		VEC3Init(sumX, sumX2, sumX3),
		VEC3Init(sumX2, sumX3, sumX4),
	}
	v := VEC3Init(sumY, sumYX, sumYX2)

	res, ok := MAT3Solve(m, v)
	if !ok {
		return 0
	}

	a := res[2]
	b := res[1]
	c := res[0]

	if math.Abs(a) < 1.0e-10 {
		if math.Abs(b) < 1.0e-10 {
			return 0
		}
		return cmsMax(0, cmsMin(50, -c/b))
	}

	d := b*b - 4.0*a*c
	if d <= 0 {
		return 0
	}
	rt := (-b + math.Sqrt(d)) / (2.0 * a)
	return cmsMax(0, cmsMin(50, rt))
}

func cmsMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func cmsMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// DetectDestinationBlackPoint ports cmsDetectDestinationBlackPoint, the Adobe
// black-point-compensation destination black algorithm. It returns the black
// point and whether detection succeeded (zero XYZ on failure).
func (p *Profile) DetectDestinationBlackPoint(intent, dwFlags uint32) (CIEXYZ, bool) {
	// Make sure the device class is adequate.
	devClass := p.GetDeviceClass()
	if devClass == SigLinkClass || devClass == SigAbstractClass || devClass == SigNamedColorClass {
		return CIEXYZ{}, false
	}

	// Make sure intent is adequate.
	if intent != IntentPerceptual && intent != IntentRelativeColorimetric && intent != IntentSaturation {
		return CIEXYZ{}, false
	}

	// v4 + perceptual & saturation intents have their own black point.
	if p.GetEncodedICCversion() >= 0x4000000 &&
		(intent == IntentPerceptual || intent == IntentSaturation) {
		if p.IsMatrixShaper() {
			return p.blackPointAsDarkerColorant(IntentRelativeColorimetric)
		}
		return CIEXYZ{X: perceptualBlackX, Y: perceptualBlackY, Z: perceptualBlackZ}, true
	}

	// Check if the profile is LUT based and gray, rgb or cmyk (7.2 in Adobe's doc).
	colorSpace := p.GetColorSpace()
	if !p.IsCLUT(intent, UsedAsOutput) ||
		(colorSpace != SigGrayData && colorSpace != SigRgbData && !isInkColorspace(colorSpace)) {
		// Handle as the input case.
		return p.DetectBlackPoint(intent, dwFlags)
	}

	// One of the valid cases: use the Adobe algorithm.
	var initialLab CIELab

	// Set a first guess that should work on good profiles.
	if intent == IntentRelativeColorimetric {
		iniXYZ, ok := p.DetectBlackPoint(intent, dwFlags)
		if !ok {
			return CIEXYZ{}, false
		}
		initialLab = XYZ2Lab(nil, iniXYZ)
	} else {
		// Perceptual and saturation: black point is Lab (0,0,0).
		initialLab = CIELab{L: 0, A: 0, B: 0}
	}

	// Step 2: create a round trip and sample the L* ramp.
	roundTrip, err := p.buildRoundtripPipeline(intent)
	if err != nil || roundTrip == nil {
		return CIEXYZ{}, false
	}

	var inRamp, outRamp [256]float64
	for l := 0; l < 256; l++ {
		var lab CIELab
		lab.L = float64(l) * 100.0 / 255.0
		lab.A = cmsMin(50, cmsMax(-50, initialLab.A))
		lab.B = cmsMin(50, cmsMax(-50, initialLab.B))

		in := [3]float32{float32(lab.L), float32(lab.A), float32(lab.B)}
		var out [maxChannels]float32
		roundTrip.EvalFloat(in[:], out[:])

		inRamp[l] = lab.L
		outRamp[l] = float64(out[0])
	}

	// Make monotonic.
	for l := 254; l > 0; l-- {
		outRamp[l] = cmsMin(outRamp[l], outRamp[l+1])
	}

	// Check.
	if !(outRamp[0] < outRamp[255]) {
		return CIEXYZ{}, false
	}

	// Test for mid range straight (only on relative colorimetric).
	minL := outRamp[0]
	maxL := outRamp[255]
	if intent == IntentRelativeColorimetric {
		nearlyStraightMidrange := true
		for l := 0; l < 256; l++ {
			if !((inRamp[l] <= minL+0.2*(maxL-minL)) ||
				(math.Abs(inRamp[l]-outRamp[l]) < 4.0)) {
				nearlyStraightMidrange = false
			}
		}
		// If the mid range is straight then the destination black point equals
		// initialLab; otherwise it is found by curve fitting.
		if nearlyStraightMidrange {
			return Lab2XYZ(nil, initialLab), true
		}
	}

	// Curve fitting: the round-trip curve normally looks like a nearly constant
	// section at the black point, a corner, and a nearly straight line to white.
	var yRamp [256]float64
	for l := 0; l < 256; l++ {
		yRamp[l] = (outRamp[l] - minL) / (maxL - minL)
	}

	var lo, hi float64
	if intent == IntentRelativeColorimetric {
		lo = 0.1
		hi = 0.5
	} else {
		lo = 0.03
		hi = 0.25
	}

	// Capture shadow points for the fitting.
	var x, y [256]float64
	n := 0
	for l := 0; l < 256; l++ {
		ff := yRamp[l]
		if ff >= lo && ff < hi {
			x[n] = inRamp[l]
			y[n] = yRamp[l]
			n++
		}
	}

	// No suitable points.
	if n < 3 {
		return CIEXYZ{}, false
	}

	// Fit and get the vertex of the quadratic curve.
	var lab CIELab
	lab.L = rootOfLeastSquaresFitQuadraticCurve(n, x[:], y[:])
	if lab.L < 0.0 {
		lab.L = 0
	}
	lab.A = initialLab.A
	lab.B = initialLab.B

	return Lab2XYZ(nil, lab), true
}
