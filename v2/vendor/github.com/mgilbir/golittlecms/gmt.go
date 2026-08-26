package lcms2

// gmt.go ports src/cmsgmt.c: gamut checking / soft-proofing helpers, the
// K -> L* black tone-curve builder, total-area-coverage detection, CIELab
// desaturation and RGB-profile gamma detection.

import (
	"encoding/binary"
	"math"
)

// ---------------------------------------------------------------------------
// Local format-type codes (the raw TYPE_* encodings this file needs).
// ---------------------------------------------------------------------------

const (
	fmtCMYKFlt = 1<<22 | uint32(PTCMYK)<<16 | 4<<3 | 4 // TYPE_CMYK_FLT
	fmtLab16   = uint32(PTLab)<<16 | 3<<3 | 2          // TYPE_Lab_16
	fmtRGB16   = uint32(PTRGB)<<16 | 3<<3 | 2          // TYPE_RGB_16
)

var (
	fmtLabDBL = typeLabDBL // TYPE_Lab_DBL
	fmtXYZDBL = typeXYZDBL // TYPE_XYZ_DBL
)

// ---------------------------------------------------------------------------
// Small typed-transform marshalling helpers.
//
// The transform engine works on raw byte buffers whose layout matches the
// (little-endian) pack/unpack routines. These helpers marshal the typed values
// the samplers exchange (uint16 channels, float32 channels, Lab/XYZ doubles).
// ---------------------------------------------------------------------------

func putF64(b []byte, off int, v float64) {
	binary.LittleEndian.PutUint64(b[off:], math.Float64bits(v))
}

func getF64(b []byte, off int) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(b[off:]))
}

func u16sToBytes(in []uint16) []byte {
	b := make([]byte, len(in)*2)
	for i, v := range in {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return b
}

func f32sToBytes(in []float32) []byte {
	b := make([]byte, len(in)*4)
	for i, v := range in {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func bytesToU16s(b []byte, n int) []uint16 {
	out := make([]uint16, n)
	for i := 0; i < n; i++ {
		out[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return out
}

func bytesToF32s(b []byte, n int) []float32 {
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func labToBytes(l CIELab) []byte {
	b := make([]byte, 24)
	putF64(b, 0, l.L)
	putF64(b, 8, l.A)
	putF64(b, 16, l.B)
	return b
}

func bytesToLab(b []byte) CIELab {
	return CIELab{L: getF64(b, 0), A: getF64(b, 8), B: getF64(b, 16)}
}

// ---------------------------------------------------------------------------
// K -> L* tone-curve construction.
// ---------------------------------------------------------------------------

// chain2Lab ports _cmsChain2Lab: append a Lab identity after the given profile
// chain and return the transform (CMYK-ish in -> Lab out).
func (ctx *Context) chain2Lab(nProfiles uint32, inputFormat, outputFormat uint32,
	intents []uint32, profiles []*Profile, bpc []bool, adaptationStates []float64,
	dwFlags uint32) (*Transform, error) {

	if nProfiles > 254 {
		return nil, ctx.signalError(ErrRange, "chain2Lab: too many profiles")
	}

	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		return nil, ctx.signalError(ErrNull, "chain2Lab: cannot create Lab profile")
	}

	profileList := make([]*Profile, nProfiles+1)
	bpcList := make([]bool, nProfiles+1)
	adaptationList := make([]float64, nProfiles+1)
	intentList := make([]uint32, nProfiles+1)

	for i := uint32(0); i < nProfiles; i++ {
		profileList[i] = profiles[i]
		bpcList[i] = bpc[i]
		adaptationList[i] = adaptationStates[i]
		intentList[i] = intents[i]
	}

	profileList[nProfiles] = hLab
	bpcList[nProfiles] = false
	adaptationList[nProfiles] = 1.0
	intentList[nProfiles] = IntentRelativeColorimetric

	return ctx.CreateExtendedTransform(nProfiles+1, profileList, bpcList, intentList,
		adaptationList, nil, 0, inputFormat, outputFormat, dwFlags)
}

// computeKToLstar ports ComputeKToLstar: sample the K -> L* relationship.
func (ctx *Context) computeKToLstar(nPoints, nProfiles uint32, intents []uint32,
	profiles []*Profile, bpc []bool, adaptationStates []float64, dwFlags uint32) *ToneCurve {

	if nPoints < 2 {
		return nil
	}

	xform, err := ctx.chain2Lab(nProfiles, fmtCMYKFlt, fmtLabDBL, intents, profiles, bpc, adaptationStates, dwFlags)
	if err != nil || xform == nil {
		return nil
	}
	defer xform.DeleteTransform()

	// _cmsCalloc(nPoints, sizeof(cmsFloat32Number)) in ComputeKToLstar
	// (cmsgmt.c:111), which goes to Error and yields NULL when the manager
	// refuses. Ordered after the transform build, as the reference has it.
	if !allocSizeOK(nPoints, 4) {
		_ = ctx.signalError(ErrRange, "_cmsBuildKToneCurve: %d points exceeds the %d byte allocation limit", nPoints, maxMemoryForAlloc)
		return nil
	}

	sampledPoints := make([]float32, nPoints)
	in := make([]byte, 16)
	out := make([]byte, 24)
	for i := uint32(0); i < nPoints; i++ {
		var cmyk [4]float32
		cmyk[3] = float32((float64(i) * 100.0) / float64(nPoints-1))
		copy(in, f32sToBytes(cmyk[:]))
		xform.DoTransform(in, out, 1)
		lab := bytesToLab(out)
		sampledPoints[i] = float32(1.0 - lab.L/100.0) // Negate K for easier operation
	}

	tc, err := ctx.BuildTabulatedToneCurveFloat(sampledPoints)
	if err != nil {
		return nil
	}
	return tc
}

// BuildKToneCurve ports _cmsBuildKToneCurve: compute the black tone curve on a
// CMYK -> CMYK chain by joining the K -> L* curves of the input and output
// sides. Returns nil if the chain is not CMYK -> CMYK output, or if the joined
// curve is non-monotonic.
func (ctx *Context) BuildKToneCurve(nPoints, nProfiles uint32, intents []uint32,
	profiles []*Profile, bpc []bool, adaptationStates []float64, dwFlags uint32) *ToneCurve {

	if nProfiles < 1 {
		return nil
	}

	// Make sure CMYK -> CMYK.
	if profiles[0].GetColorSpace() != SigCmykData ||
		profiles[nProfiles-1].GetColorSpace() != SigCmykData {
		return nil
	}
	// Make sure last is an output profile.
	if profiles[nProfiles-1].GetDeviceClass() != SigOutputClass {
		return nil
	}

	in := ctx.computeKToLstar(nPoints, nProfiles-1, intents, profiles, bpc, adaptationStates, dwFlags)
	if in == nil {
		return nil
	}

	out := ctx.computeKToLstar(nPoints, 1,
		intents[nProfiles-1:],
		profiles[nProfiles-1:],
		bpc[nProfiles-1:],
		adaptationStates[nProfiles-1:],
		dwFlags)
	if out == nil {
		in.Free()
		return nil
	}

	kTone, err := ctx.JoinToneCurve(in, out, nPoints)
	in.Free()
	out.Free()
	if err != nil || kTone == nil {
		return nil
	}

	if !kTone.IsMonotonic() {
		kTone.Free()
		return nil
	}
	return kTone
}

// ---------------------------------------------------------------------------
// Gamut LUT creation (used by gamut check & soft proofing).
// ---------------------------------------------------------------------------

const gamutErrThreshold = 5 // ERR_THRESHOLD

// gamutChain mirrors GAMUTCHAIN.
type gamutChain struct {
	hInput         *Transform // From input color space, 16-bit to Lab DBL
	hForward       *Transform // Lab DBL to colorant, 16-bit
	hReverse       *Transform // colorant 16-bit to Lab DBL
	threshold      float64
	nInputChannels int
	nChannels      int
}

// gamutSampler ports GamutSampler: compute the gamut boundary by transforming
// back and forth. Values whose round-trip dE exceeds the threshold are flagged.
func gamutSampler(in, out []uint16, cargo any) bool {
	t, ok := cargo.(*gamutChain)
	if !ok || t == nil {
		return false
	}

	labIn1Bytes := make([]byte, 24)
	// Convert input to Lab. The CLUT dimension (len(in)) need not equal the
	// input-space channel count; copy into a zero-padded buffer so hInput always
	// sees exactly nInputChannels values without any out-of-range slice (the
	// reference reads from an oversized cmsMAXCHANNELS In[] buffer).
	inChans := make([]uint16, t.nInputChannels)
	copy(inChans, in)
	t.hInput.DoTransform(u16sToBytes(inChans), labIn1Bytes, 1)
	labIn1 := bytesToLab(labIn1Bytes)

	// Converts from PCS to colorant (always in-gamut).
	proofBytes := make([]byte, t.nChannels*2)
	t.hForward.DoTransform(labToBytes(labIn1), proofBytes, 1)

	// Now the inverse, colorant to PCS.
	labOut1Bytes := make([]byte, 24)
	t.hReverse.DoTransform(proofBytes, labOut1Bytes, 1)
	labOut1 := bytesToLab(labOut1Bytes)

	labIn2 := labOut1

	// Try again taking the result as input.
	proof2Bytes := make([]byte, t.nChannels*2)
	t.hForward.DoTransform(labToBytes(labOut1), proof2Bytes, 1)
	labOut2Bytes := make([]byte, 24)
	t.hReverse.DoTransform(proof2Bytes, labOut2Bytes, 1)
	labOut2 := bytesToLab(labOut2Bytes)

	dE1 := DeltaE(labIn1, labOut1)
	dE2 := DeltaE(labIn2, labOut2)

	if dE1 < t.threshold && dE2 < t.threshold {
		out[0] = 0
	} else {
		if dE1 < t.threshold && dE2 > t.threshold {
			// undefined, assume in gamut
			out[0] = 0
		} else if dE1 > t.threshold && dE2 < t.threshold {
			// clearly out of gamut
			out[0] = uint16(quickFloor((dE1 - t.threshold) + .5))
		} else {
			// both big, could be perceptual mapping: take error ratio
			var errorRatio float64
			if dE2 == 0.0 {
				errorRatio = dE1
			} else {
				errorRatio = dE1 / dE2
			}
			if errorRatio > t.threshold {
				out[0] = uint16(quickFloor((errorRatio - t.threshold) + .5))
			} else {
				out[0] = 0
			}
		}
	}

	return true
}

// createGamutCheckPipeline ports _cmsCreateGamutCheckPipeline: build a pipeline
// mapping the input color space to a single-channel out-of-gamut marker.
func (ctx *Context) createGamutCheckPipeline(profiles []*Profile, bpc []bool,
	intents []uint32, adaptationStates []float64, nGamutPCSposition uint32, hGamut *Profile) *Pipeline {

	if nGamutPCSposition == 0 || nGamutPCSposition > 255 {
		ctx.signalError(ErrRange, "Wrong position of PCS. 1..255 expected, %d found.", nGamutPCSposition)
		return nil
	}

	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		return nil
	}

	var chain gamutChain

	// The figure of merit.
	if hGamut.IsMatrixShaper() {
		chain.threshold = 1.0
	} else {
		chain.threshold = gamutErrThreshold
	}

	profileList := make([]*Profile, nGamutPCSposition+1)
	bpcList := make([]bool, nGamutPCSposition+1)
	adaptationList := make([]float64, nGamutPCSposition+1)
	intentList := make([]uint32, nGamutPCSposition+1)

	for i := uint32(0); i < nGamutPCSposition; i++ {
		profileList[i] = profiles[i]
		bpcList[i] = bpc[i]
		adaptationList[i] = adaptationStates[i]
		intentList[i] = intents[i]
	}

	// Fill Lab identity.
	profileList[nGamutPCSposition] = hLab
	bpcList[nGamutPCSposition] = false
	adaptationList[nGamutPCSposition] = 1.0
	intentList[nGamutPCSposition] = IntentRelativeColorimetric

	colorSpace := hGamut.GetColorSpace()
	nChannels := ChannelsOfColorSpace(colorSpace)
	nGridpoints := ReasonableGridpointsByColorspace(colorSpace, FlagsHighResPrecalc)

	inputColorSpace := profileList[0].GetColorSpace()
	nInputChannels := ChannelsOfColorSpace(inputColorSpace)
	dwFormat := channelsSH(uint32(nInputChannels)) | bytesSH(2)

	chain.nInputChannels = int(nInputChannels)
	chain.nChannels = int(nChannels)

	// 16 bits to Lab double.
	hInput, errIn := ctx.CreateExtendedTransform(nGamutPCSposition+1, profileList, bpcList,
		intentList, adaptationList, nil, 0, dwFormat, fmtLabDBL, FlagsNoCache)

	// Forward step: Lab double to device.
	dwFormat2 := channelsSH(uint32(nChannels)) | bytesSH(2)
	hForward, errFwd := ctx.CreateTransform(hLab, fmtLabDBL, hGamut, dwFormat2,
		IntentRelativeColorimetric, FlagsNoCache)

	// Backwards step.
	hReverse, errRev := ctx.CreateTransform(hGamut, dwFormat2, hLab, fmtLabDBL,
		IntentRelativeColorimetric, FlagsNoCache)

	chain.hInput = hInput
	chain.hForward = hForward
	chain.hReverse = hReverse

	var gamut *Pipeline
	if errIn == nil && errFwd == nil && errRev == nil &&
		hInput != nil && hForward != nil && hReverse != nil {

		gamut, _ = ctx.PipelineAlloc(3, 1)
		if gamut != nil {
			clut, err := ctx.StageAllocCLut16bit(nGridpoints, uint32(nChannels), 1, nil)
			if err != nil || clut == nil {
				gamut.Free()
				gamut = nil
			} else if err := gamut.InsertStage(AtBegin, clut); err != nil {
				gamut.Free()
				gamut = nil
			} else {
				clut.SampleCLut16bit(gamutSampler, &chain, 0)
			}
		}
	}

	if hInput != nil {
		hInput.DeleteTransform()
	}
	if hForward != nil {
		hForward.DeleteTransform()
	}
	if hReverse != nil {
		hReverse.DeleteTransform()
	}

	return gamut
}

// ---------------------------------------------------------------------------
// Total Area Coverage estimation.
// ---------------------------------------------------------------------------

// tacEstimator mirrors cmsTACestimator.
type tacEstimator struct {
	nOutputChans uint32
	hRoundTrip   *Transform
	maxTAC       float32
	maxInput     [maxChannels]float32
}

// estimateTAC ports EstimateTAC: accumulate the maximum ink laid down.
func estimateTAC(in, _ []uint16, cargo any) bool {
	bp, ok := cargo.(*tacEstimator)
	if !ok || bp == nil {
		return false
	}

	// Evaluate the xform (Lab 16 -> device float).
	inBytes := u16sToBytes(in[:3])
	outBytes := make([]byte, bp.nOutputChans*4)
	bp.hRoundTrip.DoTransform(inBytes, outBytes, 1)
	roundTrip := bytesToF32s(outBytes, int(bp.nOutputChans))

	var sum float32
	for i := uint32(0); i < bp.nOutputChans; i++ {
		sum += roundTrip[i]
	}

	if sum > bp.maxTAC {
		bp.maxTAC = sum
		// maxInput is diagnostic only (never read). The reference reads In[]
		// out to nOutputChans from an oversized (cmsMAXCHANNELS) buffer; here the
		// sampler input holds only the CLUT's input channels, so bound the copy.
		for i := uint32(0); i < bp.nOutputChans && int(i) < len(in); i++ {
			bp.maxInput[i] = float32(in[i])
		}
	}
	return true
}

// DetectTAC ports cmsDetectTAC: detect the total area coverage of an output
// profile (result in %). Returns 0 for unsupported profiles.
func (p *Profile) DetectTAC() float64 {
	ctx := profileContextID(p)

	// TAC only works on output profiles.
	if p.GetDeviceClass() != SigOutputClass {
		return 0
	}

	// Fake formatter for the result (float, 4 bytes).
	dwFormatter := FormatterForColorspaceOfProfile(p, 4, true)
	if dwFormatter == 0 {
		return 0
	}

	var bp tacEstimator
	bp.nOutputChans = tChannels(dwFormatter)
	bp.maxTAC = 0

	// For safety.
	if bp.nOutputChans >= maxChannels {
		return 0
	}

	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		return 0
	}

	// Roundtrip on perceptual intent for TAC estimation.
	xform, err := ctx.CreateTransform(hLab, fmtLab16, p, dwFormatter,
		IntentPerceptual, FlagsNoOptimize|FlagsNoCache)
	if err != nil || xform == nil {
		return 0
	}
	bp.hRoundTrip = xform

	// For L* we only need black and white. For C* we need many points.
	gridPoints := []uint32{6, 74, 74}

	if !SliceSpace16(3, gridPoints, estimateTAC, &bp) {
		bp.maxTAC = 0
	}

	xform.DeleteTransform()

	return float64(bp.maxTAC)
}

// DetectTAC ports cmsDetectTAC on the default/profile context.
func DetectTAC(p *Profile) float64 { return p.DetectTAC() }

// ---------------------------------------------------------------------------
// CIELab desaturation.
// ---------------------------------------------------------------------------

// DesaturateLab ports cmsDesaturateLab: carefully clamp on CIELab space.
func DesaturateLab(lab *CIELab, amax, amin, bmax, bmin float64) bool {
	if lab == nil {
		return false
	}

	// Whole luma surface to zero.
	if lab.L < 0 {
		lab.L = 0
		lab.A = 0
		lab.B = 0
		return false
	}

	// Clamp white, discard highlights (ICC spec disallows L>100 highlights).
	if lab.L > 100 {
		lab.L = 100
	}

	// Check out gamut prism on a, b faces.
	if lab.A < amin || lab.A > amax || lab.B < bmin || lab.B > bmax {
		if lab.A == 0.0 { // Is hue exactly 90?
			// atan will not work, so clamp here.
			if lab.B < 0 {
				lab.B = bmin
			} else {
				lab.B = bmax
			}
			return true
		}

		lch := Lab2LCh(*lab)
		slope := lab.B / lab.A
		h := lch.H

		switch {
		case (h >= 0. && h < 45.) || (h >= 315 && h <= 360.):
			// clip by amax
			lab.A = amax
			lab.B = amax * slope
		case h >= 45. && h < 135.:
			// clip by bmax
			lab.B = bmax
			lab.A = bmax / slope
		case h >= 135. && h < 225.:
			// clip by amin
			lab.A = amin
			lab.B = amin * slope
		case h >= 225. && h < 315.:
			// clip by bmin
			lab.B = bmin
			lab.A = bmin / slope
		default:
			defaultContext.signalError(ErrRange, "Invalid angle")
			return false
		}
	}

	return true
}

// ---------------------------------------------------------------------------
// RGB profile gamma detection.
// ---------------------------------------------------------------------------

// DetectRGBProfileGamma ports cmsDetectRGBProfileGamma: estimate the working
// gamma of an RGB profile via a synthetic gray ramp; returns -1 on unsupported
// profiles.
func (p *Profile) DetectRGBProfileGamma(threshold float64) float64 {
	if p.GetColorSpace() != SigRgbData {
		return -1
	}

	cl := p.GetDeviceClass()
	if cl != SigInputClass && cl != SigDisplayClass &&
		cl != SigOutputClass && cl != SigColorSpaceClass {
		return -1
	}

	ctx := profileContextID(p)
	hXYZ, err := ctx.CreateXYZProfile()
	if err != nil || hXYZ == nil {
		return -1
	}

	xform, err := ctx.CreateTransform(p, fmtRGB16, hXYZ, fmtXYZDBL,
		IntentRelativeColorimetric, FlagsNoOptimize)
	if err != nil || xform == nil {
		return -1
	}

	in := make([]byte, 256*3*2)
	for i := 0; i < 256; i++ {
		v := from8to16(uint8(i))
		for c := 0; c < 3; c++ {
			binary.LittleEndian.PutUint16(in[(i*3+c)*2:], v)
		}
	}

	out := make([]byte, 256*3*8) // 256 * XYZ DBL
	xform.DoTransform(in, out, 256)
	xform.DeleteTransform()

	yNormalized := make([]float32, 256)
	for i := 0; i < 256; i++ {
		// XYZ DBL layout: X,Y,Z doubles. Y is at offset +8.
		yNormalized[i] = float32(getF64(out, i*24+8))
	}

	yCurve, err := ctx.BuildTabulatedToneCurveFloat(yNormalized)
	if err != nil || yCurve == nil {
		return -1
	}

	gamma := yCurve.EstimateGamma(threshold)
	yCurve.Free()

	return gamma
}

// DetectRGBProfileGamma ports cmsDetectRGBProfileGamma on the default context.
func DetectRGBProfileGamma(p *Profile, threshold float64) float64 {
	return p.DetectRGBProfileGamma(threshold)
}
