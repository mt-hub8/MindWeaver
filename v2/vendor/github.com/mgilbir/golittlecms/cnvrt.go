// Port of src/cmscnvrt.c (lcms2 2.19): rendering-intent handling, the profile
// chaining that builds a device-link pipeline from a list of profiles, black
// point compensation and the non-ICC black-preserving (K-only / K-plane)
// intents.
//
// Naming: the C names map to Go as
//
//	_cmsLinkProfiles              -> (*Context).LinkProfiles
//	_cmsDefaultICCintents        -> (*Context).DefaultICCintents
//	cmsGetSupportedIntents(THR)  -> (*Context).GetSupportedIntents / GetSupportedIntents
//	cmsPluginRenderingIntent     -> PluginRenderingIntent
//	cmsIntentFn                  -> IntentFn
//	ComputeBlackPointCompensation, ComputeConversion, AddConversion,
//	ComputeAbsoluteIntent, ... keep their reference names (unexported).
//
// PORTNOTES:
//   - Intent handlers return (*Pipeline, error) instead of the reference's
//     "cmsPipeline* or NULL". A nil pipeline with a non-nil error is the failure
//     mode; callers treat any nil result as failure.
//   - The intents plug-in registry reuses the generic pluginList in context.go
//     (ctx.intents); SearchIntent consults it (newest-first) before the built-in
//     table, matching cmsIntentsList search order.
//   - Black point detection (cmsDetectBlackPoint / cmsDetectDestinationBlackPoint)
//     lives in samp.go (extended from cmssamp.c as part of this task).
//   - DEFERRED to W12/W14: the CMYK->CMYK CLUT-sampling body of the
//     black-preserving intents needs cmsDoTransform (W12, cmsxform.c) plus
//     _cmsBuildKToneCurve / cmsDetectTAC (W14, cmsgmt.c). Those helpers do not
//     exist yet, so the CMYK sampling path returns an ErrNotSuitable error with
//     a TODO. The non-CMYK fallback (translate the intent to its ICC base and
//     defer to DefaultICCintents) — the common case — is fully ported.

package lcms2

import "math"

// Non-ICC black-preserving rendering intents, mirroring the INTENT_PRESERVE_*
// macros in include/lcms2.h.
const (
	IntentPreserveKOnlyPerceptual            uint32 = 10
	IntentPreserveKOnlyRelativeColorimetric  uint32 = 11
	IntentPreserveKOnlySaturation            uint32 = 12
	IntentPreserveKPlanePerceptual           uint32 = 13
	IntentPreserveKPlaneRelativeColorimetric uint32 = 14
	IntentPreserveKPlaneSaturation           uint32 = 15
)

// flagsNoNegatives mirrors cmsFLAGS_NONEGATIVES: clip negative numbers on the
// device side of a float transform.
const flagsNoNegatives = 0x8000

// IntentFn is the Go analogue of cmsIntentFn: an intent handler that builds the
// device-link pipeline chaining nProfiles profiles. The slices carry one entry
// per profile (intents, profiles, BPC flags, adaptation states).
type IntentFn func(ctx *Context, nProfiles uint32, intents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (*Pipeline, error)

// intentsList mirrors cmsIntentsList: one supported intent and its handler.
type intentsList struct {
	Intent      uint32
	Description string
	Link        IntentFn
}

// PluginRenderingIntent mirrors cmsPluginRenderingIntent: a plug-in that adds a
// new intent number or overrides a default routine. Embed it and set Base().Type
// to pluginRenderingIntentSig, then register it with RegisterPlugins.
type PluginRenderingIntent struct {
	PluginBase
	Intent      uint32
	Link        IntentFn
	Description string
}

// defaultIntents is the built-in intents table, mirroring DefaultIntents[]. It
// is populated in init() rather than as a var initializer: the K-preserving
// handlers transitively reference this table (via LinkProfiles -> searchIntent),
// which the Go compiler would otherwise reject as a static initialization cycle.
var defaultIntents []intentsList

func init() {
	defaultIntents = []intentsList{
		{IntentPerceptual, "Perceptual", defaultICCintentsFn},
		{IntentRelativeColorimetric, "Relative colorimetric", defaultICCintentsFn},
		{IntentSaturation, "Saturation", defaultICCintentsFn},
		{IntentAbsoluteColorimetric, "Absolute colorimetric", defaultICCintentsFn},
		{IntentPreserveKOnlyPerceptual, "Perceptual preserving black ink", blackPreservingKOnlyIntentsFn},
		{IntentPreserveKOnlyRelativeColorimetric, "Relative colorimetric preserving black ink", blackPreservingKOnlyIntentsFn},
		{IntentPreserveKOnlySaturation, "Saturation preserving black ink", blackPreservingKOnlyIntentsFn},
		{IntentPreserveKPlanePerceptual, "Perceptual preserving black plane", blackPreservingKPlaneIntentsFn},
		{IntentPreserveKPlaneRelativeColorimetric, "Relative colorimetric preserving black plane", blackPreservingKPlaneIntentsFn},
		{IntentPreserveKPlaneSaturation, "Saturation preserving black plane", blackPreservingKPlaneIntentsFn},
	}
}

// searchIntent ports SearchIntent: look for a handler for Intent, consulting the
// context plug-in list first (newest registered wins) then the built-in table.
func (ctx *Context) searchIntent(intent uint32) *intentsList {
	if ctx != nil {
		ctx.mu.Lock()
		entries := ctx.intents.entries
		ctx.mu.Unlock()
		for _, e := range entries {
			pl, ok := e.(*PluginRenderingIntent)
			if !ok {
				continue
			}
			if pl.Intent == intent {
				return &intentsList{Intent: pl.Intent, Description: pl.Description, Link: pl.Link}
			}
		}
	}
	for i := range defaultIntents {
		if defaultIntents[i].Intent == intent {
			return &defaultIntents[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Black point compensation
// ---------------------------------------------------------------------------

// computeBlackPointCompensation ports ComputeBlackPointCompensation: a linear
// XYZ scaling m*bp + off mapping BlackPointIn->BlackPointOut and D50->D50. The
// returned matrix is diagonal and the offset is a 3-vector (both in XYZ space,
// not yet encoding-adjusted).
func computeBlackPointCompensation(blackPointIn, blackPointOut CIEXYZ) (MAT3, VEC3) {
	d50 := D50XYZ()

	tx := blackPointIn.X - d50.X
	ty := blackPointIn.Y - d50.Y
	tz := blackPointIn.Z - d50.Z

	ax := (blackPointOut.X - d50.X) / tx
	ay := (blackPointOut.Y - d50.Y) / ty
	az := (blackPointOut.Z - d50.Z) / tz

	bx := -d50.X * (blackPointOut.X - blackPointIn.X) / tx
	by := -d50.Y * (blackPointOut.Y - blackPointIn.Y) / ty
	bz := -d50.Z * (blackPointOut.Z - blackPointIn.Z) / tz

	m := MAT3{
		VEC3Init(ax, 0, 0),
		VEC3Init(0, ay, 0),
		VEC3Init(0, 0, az),
	}
	off := VEC3Init(bx, by, bz)
	return m, off
}

// ---------------------------------------------------------------------------
// Absolute colorimetric white-point handling
// ---------------------------------------------------------------------------

// chad2Temp ports CHAD2Temp: approximate a blackbody temperature from a CHAD
// matrix by mapping D50 across its inverse to the absolute white point.
func chad2Temp(chad MAT3) float64 {
	m2, ok := MAT3Inverse(chad)
	if !ok {
		return 0 // C returns FALSE (0.0)
	}

	d50 := D50XYZ()
	s := VEC3Init(d50.X, d50.Y, d50.Z)
	d := MAT3Eval(m2, s)

	dest := CIEXYZ{X: d[0], Y: d[1], Z: d[2]}
	destChromaticity := XYZ2xyY(dest)

	tempK, err := TempFromWhitePoint(destChromaticity)
	if err != nil {
		return -1.0
	}
	return tempK
}

// temp2CHAD ports Temp2CHAD: compute a CHAD from a temperature.
func temp2CHAD(temp float64) MAT3 {
	chromaticityOfWhite, err := WhitePointFromTemp(temp)
	if err != nil {
		return MAT3Identity()
	}
	white := XyY2XYZ(chromaticityOfWhite)
	m, _ := adaptationMatrix(nil, white, D50XYZ())
	return m
}

// computeAbsoluteIntent ports ComputeAbsoluteIntent: join relative-input,
// absolute and relative-output scalings into a single 3x3 matrix, honoring the
// observer adaptation state. Returns the matrix and true, or the identity and
// false on failure.
func computeAbsoluteIntent(adaptationState float64,
	whitePointIn CIEXYZ, chromaticAdaptationMatrixIn MAT3,
	whitePointOut CIEXYZ, chromaticAdaptationMatrixOut MAT3) (MAT3, bool) {

	if adaptationState == 1.0 {
		// Observer is fully adapted (standard V4 behaviour).
		m := MAT3{
			VEC3Init(whitePointIn.X/whitePointOut.X, 0, 0),
			VEC3Init(0, whitePointIn.Y/whitePointOut.Y, 0),
			VEC3Init(0, 0, whitePointIn.Z/whitePointOut.Z),
		}
		return m, true
	}

	// Incomplete adaptation (advanced feature).
	scale := MAT3{
		VEC3Init(whitePointIn.X/whitePointOut.X, 0, 0),
		VEC3Init(0, whitePointIn.Y/whitePointOut.Y, 0),
		VEC3Init(0, 0, whitePointIn.Z/whitePointOut.Z),
	}

	if adaptationState == 0.0 {
		m2 := MAT3Per(chromaticAdaptationMatrixOut, scale)
		// m2 holds CHAD from output white to D50 times abs. col. scaling.

		// Observer is not adapted; undo the chromatic adaptation.
		// (The reference overwrites m twice; the second assignment wins.)
		m4, ok := MAT3Inverse(chromaticAdaptationMatrixIn)
		if !ok {
			return MAT3Identity(), false
		}
		m := MAT3Per(m2, m4)
		return m, true
	}

	// General incomplete adaptation.
	m2, ok := MAT3Inverse(chromaticAdaptationMatrixIn)
	if !ok {
		return MAT3Identity(), false
	}
	m3 := MAT3Per(m2, scale)
	// m3 holds CHAD from input white to D50 times abs. col. scaling.

	tempSrc := chad2Temp(chromaticAdaptationMatrixIn)
	tempDest := chad2Temp(chromaticAdaptationMatrixOut)

	if tempSrc < 0.0 || tempDest < 0.0 {
		return MAT3Identity(), false // Something went wrong
	}

	if MAT3IsIdentity(scale) && absFloat(tempSrc-tempDest) < 0.01 {
		return MAT3Identity(), true
	}

	temp := (1.0-adaptationState)*tempDest + adaptationState*tempSrc

	// Get a CHAD from whatever output temperature to D50; replaces output CHAD.
	mixedCHAD := temp2CHAD(temp)

	m := MAT3Per(m3, mixedCHAD)
	return m, true
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ---------------------------------------------------------------------------
// Conversion layer
// ---------------------------------------------------------------------------

// isEmptyLayer ports IsEmptyLayer: reports whether m/off is (numerically) an
// identity layer that need not be applied.
func isEmptyLayer(m MAT3, off VEC3) bool {
	ident := MAT3Identity()
	var diff float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			diff += absFloat(m[i][j] - ident[i][j])
		}
	}
	for i := 0; i < 3; i++ {
		diff += absFloat(off[i])
	}
	return diff < 0.002
}

// computeConversion ports ComputeConversion: build the conversion matrix/offset
// between profile i-1's PCS and profile i's PCS for the given intent. The offset
// is encoding-adjusted (divided by MAX_ENCODEABLE_XYZ) on return.
func computeConversion(i uint32, profiles []*Profile, intent uint32, bpc bool,
	adaptationState float64) (MAT3, VEC3, bool) {

	m := MAT3Identity()
	var off VEC3

	if intent == IntentAbsoluteColorimetric {
		whitePointIn := profiles[i-1].readMediaWhitePoint()
		chadIn, ok := profiles[i-1].readCHAD()
		if !ok {
			return m, off, false
		}
		whitePointOut := profiles[i].readMediaWhitePoint()
		chadOut, ok := profiles[i].readCHAD()
		if !ok {
			return m, off, false
		}

		am, ok := computeAbsoluteIntent(adaptationState,
			whitePointIn, chadIn, whitePointOut, chadOut)
		if !ok {
			return m, off, false
		}
		m = am
	} else if bpc {
		// Rest of intents may apply BPC.
		blackPointIn, _ := profiles[i-1].DetectBlackPoint(intent, 0)
		blackPointOut, _ := profiles[i].DetectDestinationBlackPoint(intent, 0)

		// If black points are equal, then do nothing.
		if blackPointIn.X != blackPointOut.X ||
			blackPointIn.Y != blackPointOut.Y ||
			blackPointIn.Z != blackPointOut.Z {
			m, off = computeBlackPointCompensation(blackPointIn, blackPointOut)
		}
	}

	// Adjust the offset for the XYZ 0..1.0 encoding (see the reference comment).
	for k := 0; k < 3; k++ {
		off[k] /= maxEncodeableXYZ
	}

	return m, off, true
}

// addConversion ports AddConversion: append the PCS-connection stages (and the
// optional matrix/offset layer) needed to go from inPCS to outPCS.
func addConversion(result *Pipeline, inPCS, outPCS ColorSpaceSignature, m MAT3, off VEC3) error {
	ctx := result.ContextID()
	mAsDbl := matFlat(m)
	offAsDbl := []float64{off[0], off[1], off[2]}

	switch inPCS {
	case SigXYZData: // Input profile operates in XYZ
		switch outPCS {
		case SigXYZData: // XYZ -> XYZ
			if !isEmptyLayer(m, off) {
				st, err := ctx.StageAllocMatrix(3, 3, mAsDbl, offAsDbl)
				if err != nil {
					return err
				}
				if err := result.InsertStage(AtEnd, st); err != nil {
					return err
				}
			}
		case SigLabData: // XYZ -> Lab
			if !isEmptyLayer(m, off) {
				st, err := ctx.StageAllocMatrix(3, 3, mAsDbl, offAsDbl)
				if err != nil {
					return err
				}
				if err := result.InsertStage(AtEnd, st); err != nil {
					return err
				}
			}
			if err := result.InsertStage(AtEnd, ctx.stageAllocXYZ2Lab()); err != nil {
				return err
			}
		default:
			return colorspaceMismatch(ctx)
		}

	case SigLabData: // Input profile operates in Lab
		switch outPCS {
		case SigXYZData: // Lab -> XYZ
			if err := result.InsertStage(AtEnd, ctx.stageAllocLab2XYZ()); err != nil {
				return err
			}
			if !isEmptyLayer(m, off) {
				st, err := ctx.StageAllocMatrix(3, 3, mAsDbl, offAsDbl)
				if err != nil {
					return err
				}
				if err := result.InsertStage(AtEnd, st); err != nil {
					return err
				}
			}
		case SigLabData: // Lab -> Lab
			if !isEmptyLayer(m, off) {
				if err := result.InsertStage(AtEnd, ctx.stageAllocLab2XYZ()); err != nil {
					return err
				}
				st, err := ctx.StageAllocMatrix(3, 3, mAsDbl, offAsDbl)
				if err != nil {
					return err
				}
				if err := result.InsertStage(AtEnd, st); err != nil {
					return err
				}
				if err := result.InsertStage(AtEnd, ctx.stageAllocXYZ2Lab()); err != nil {
					return err
				}
			}
		default:
			return colorspaceMismatch(ctx)
		}

	default:
		// On colorspaces other than PCS, check for same space.
		if inPCS != outPCS {
			return colorspaceMismatch(ctx)
		}
	}

	return nil
}

func colorspaceMismatch(ctx *Context) error {
	return ctx.signalError(ErrColorspaceCheck, "ColorSpace mismatch")
}

// colorSpaceIsCompatible ports ColorSpaceIsCompatible.
func colorSpaceIsCompatible(a, b ColorSpaceSignature) bool {
	if a == b {
		return true
	}
	// MCH4 <-> CMYK.
	if a == Sig4colorData && b == SigCmykData {
		return true
	}
	if a == SigCmykData && b == Sig4colorData {
		return true
	}
	// XYZ <-> Lab (interchangeable).
	if a == SigXYZData && b == SigLabData {
		return true
	}
	if a == SigLabData && b == SigXYZData {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Default ICC intents
// ---------------------------------------------------------------------------

// defaultICCintentsFn is the IntentFn wrapper around (*Context).DefaultICCintents.
func defaultICCintentsFn(ctx *Context, nProfiles uint32, intents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (*Pipeline, error) {
	return ctx.DefaultICCintents(nProfiles, intents, profiles, bpc, adaptationStates, dwFlags)
}

// DefaultICCintents ports DefaultICCintents / _cmsDefaultICCintents: build the
// device-link pipeline that chains nProfiles profiles for the given per-profile
// intents. Returns the pipeline or an error.
func (ctx *Context) DefaultICCintents(nProfiles uint32, theIntents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (*Pipeline, error) {

	// For safety.
	if nProfiles == 0 {
		return nil, ctx.signalError(ErrNull, "DefaultICCintents: no profiles")
	}
	if uint32(len(theIntents)) < nProfiles || uint32(len(profiles)) < nProfiles ||
		uint32(len(bpc)) < nProfiles || uint32(len(adaptationStates)) < nProfiles {
		return nil, ctx.signalError(ErrRange, "DefaultICCintents: argument arrays too short")
	}

	// Allocate an empty LUT for holding the result (0 channels = undefined).
	result, err := ctx.PipelineAlloc(0, 0)
	if err != nil {
		return nil, err
	}

	var colorSpaceOut ColorSpaceSignature = SigLabData
	currentColorSpace := profiles[0].GetColorSpace()

	for i := uint32(0); i < nProfiles; i++ {
		hProfile := profiles[i]
		classSig := hProfile.GetDeviceClass()
		lIsDeviceLink := classSig == SigLinkClass || classSig == SigAbstractClass

		var lIsInput bool
		if i == 0 && !lIsDeviceLink {
			// First profile is used as input unless devicelink or abstract.
			lIsInput = true
		} else {
			// Otherwise use the profile in the input direction if current space
			// is not PCS.
			lIsInput = currentColorSpace != SigXYZData && currentColorSpace != SigLabData
		}

		intent := theIntents[i]

		var colorSpaceIn ColorSpaceSignature
		if lIsInput || lIsDeviceLink {
			colorSpaceIn = hProfile.GetColorSpace()
			colorSpaceOut = hProfile.GetPCS()
		} else {
			colorSpaceIn = hProfile.GetPCS()
			colorSpaceOut = hProfile.GetColorSpace()
		}

		if !colorSpaceIsCompatible(colorSpaceIn, currentColorSpace) {
			return nil, ctx.signalError(ErrColorspaceCheck, "ColorSpace mismatch")
		}

		var lut *Pipeline

		// If devicelink is found, no custom intent is allowed and we read the
		// LUT to be applied. Settings don't apply here.
		if lIsDeviceLink || (classSig == SigNamedColorClass && nProfiles == 1) {
			lut, err = hProfile.ReadDevicelinkLUT(intent)
			if err != nil || lut == nil {
				return nil, orErr(err, ctx, "DefaultICCintents: cannot read devicelink LUT")
			}

			// Abstract profiles apply a conversion layer.
			if classSig == SigAbstractClass && i > 0 {
				m, off, ok := computeConversion(i, profiles, intent, bpc[i], adaptationStates[i])
				if !ok {
					return nil, ctx.signalError(ErrColorspaceCheck, "DefaultICCintents: conversion failed")
				}
				if err := addConversion(result, currentColorSpace, colorSpaceIn, m, off); err != nil {
					return nil, err
				}
			} else {
				if err := addConversion(result, currentColorSpace, colorSpaceIn, MAT3Identity(), VEC3{}); err != nil {
					return nil, err
				}
			}
		} else if lIsInput {
			// Input direction: non-PCS connection, proceed like devicelinks.
			lut, err = hProfile.ReadInputLUT(intent)
			if err != nil || lut == nil {
				return nil, orErr(err, ctx, "DefaultICCintents: cannot read input LUT")
			}
		} else {
			// Output direction: PCS connection. Intent may apply here.
			lut, err = hProfile.ReadOutputLUT(intent)
			if err != nil || lut == nil {
				return nil, orErr(err, ctx, "DefaultICCintents: cannot read output LUT")
			}

			m, off, ok := computeConversion(i, profiles, intent, bpc[i], adaptationStates[i])
			if !ok {
				return nil, ctx.signalError(ErrColorspaceCheck, "DefaultICCintents: conversion failed")
			}
			if err := addConversion(result, currentColorSpace, colorSpaceIn, m, off); err != nil {
				return nil, err
			}
		}

		// Concatenate to the output LUT.
		if err := result.Cat(lut); err != nil {
			return nil, err
		}

		// Update current space.
		currentColorSpace = colorSpaceOut
	}

	// Check for non-negatives clip.
	if dwFlags&flagsNoNegatives != 0 {
		if colorSpaceOut == SigGrayData || colorSpaceOut == SigRgbData || colorSpaceOut == SigCmykData {
			clip := result.ContextID().stageClipNegatives(ChannelsOf(colorSpaceOut))
			if err := result.InsertStage(AtEnd, clip); err != nil {
				return nil, err
			}
		}
	}

	if ChannelsOfColorSpace(colorSpaceOut) != int32(result.OutputChannelsCount()) {
		return nil, ctx.signalError(ErrColorspaceCheck, "DefaultICCintents: output channel mismatch")
	}

	return result, nil
}

// orErr returns err if non-nil, otherwise a fresh signalled error with msg.
func orErr(err error, ctx *Context, msg string) error {
	if err != nil {
		return err
	}
	return ctx.signalError(ErrColorspaceCheck, "%s", msg)
}

// ---------------------------------------------------------------------------
// Black-preserving intents
// ---------------------------------------------------------------------------

// translateNonICCIntents ports TranslateNonICCIntents: map a black-preserving
// intent to its underlying ICC intent.
func translateNonICCIntents(intent uint32) uint32 {
	switch intent {
	case IntentPreserveKOnlyPerceptual, IntentPreserveKPlanePerceptual:
		return IntentPerceptual
	case IntentPreserveKOnlyRelativeColorimetric, IntentPreserveKPlaneRelativeColorimetric:
		return IntentRelativeColorimetric
	case IntentPreserveKOnlySaturation, IntentPreserveKPlaneSaturation:
		return IntentSaturation
	default:
		return intent
	}
}

// isCmykDevicelink ports is_cmyk_devicelink.
func isCmykDevicelink(p *Profile) bool {
	return p.GetDeviceClass() == SigLinkClass && p.GetColorSpace() == SigCmykData
}

// blackPreservingCommon holds the shared prologue of the two K-preserving
// handlers: sanity check, intent translation, CMYK-devicelink trimming, and the
// non-CMYK fallback decision. It returns the translated ICC intents, the trimmed
// preservation-profile count, and — when the CMYK preservation path does not
// apply — a fallback pipeline to return directly.
func blackPreservingCommon(ctx *Context, nProfiles uint32, theIntents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (iccIntents []uint32, preservationCount uint32, lastProfilePos uint32, fallback *Pipeline, fallbackErr error, isFallback bool) {

	// Sanity check.
	if nProfiles < 1 || nProfiles > 255 {
		return nil, 0, 0, nil, ctx.signalError(ErrRange, "BlackPreserving: bad profile count %d", nProfiles), true
	}
	if uint32(len(theIntents)) < nProfiles || uint32(len(profiles)) < nProfiles ||
		uint32(len(bpc)) < nProfiles || uint32(len(adaptationStates)) < nProfiles {
		return nil, 0, 0, nil, ctx.signalError(ErrRange, "BlackPreserving: argument arrays too short"), true
	}

	iccIntents = make([]uint32, nProfiles)
	for i := uint32(0); i < nProfiles; i++ {
		iccIntents[i] = translateNonICCIntents(theIntents[i])
	}

	// Trim CMYK devicelinks at the end.
	lastProfilePos = nProfiles - 1
	hLastProfile := profiles[lastProfilePos]
	for isCmykDevicelink(hLastProfile) {
		if lastProfilePos < 2 {
			break
		}
		lastProfilePos--
		hLastProfile = profiles[lastProfilePos]
	}

	preservationCount = lastProfilePos + 1

	// Check for non-CMYK profiles: fall back to the plain ICC intents.
	if profiles[0].GetColorSpace() != SigCmykData ||
		!(hLastProfile.GetColorSpace() == SigCmykData || hLastProfile.GetDeviceClass() == SigOutputClass) {
		lut, err := ctx.DefaultICCintents(nProfiles, iccIntents, profiles, bpc, adaptationStates, dwFlags)
		return iccIntents, preservationCount, lastProfilePos, lut, err, true
	}

	return iccIntents, preservationCount, lastProfilePos, nil, nil, false
}

// grayOnlyParams mirrors GrayOnlyParams.
type grayOnlyParams struct {
	cmyk2cmyk *Pipeline  // The original transform
	kTone     *ToneCurve // Black-to-black tone curve
}

// blackPreservingGrayOnlySampler ports BlackPreservingGrayOnlySampler: preserve
// black only when it is the only ink used.
func blackPreservingGrayOnlySampler(in, out []uint16, cargo any) bool {
	bp, ok := cargo.(*grayOnlyParams)
	if !ok || bp == nil {
		return false
	}

	// If going across black only, keep black only.
	if in[0] == 0 && in[1] == 0 && in[2] == 0 {
		// TAC does not apply because it is black ink!
		out[0] = 0
		out[1] = 0
		out[2] = 0
		out[3] = bp.kTone.Eval16(in[3])
		return true
	}

	// Keep normal transform for other colors.
	bp.cmyk2cmyk.Eval16(in, out)
	return true
}

// blackPreservingKOnlyIntentsFn ports BlackPreservingKOnlyIntents: preserve the
// black plane on CMYK->CMYK chains when only black ink is used.
func blackPreservingKOnlyIntentsFn(ctx *Context, nProfiles uint32, theIntents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (*Pipeline, error) {

	iccIntents, preservationCount, lastProfilePos, fallback, fallbackErr, isFallback := blackPreservingCommon(
		ctx, nProfiles, theIntents, profiles, bpc, adaptationStates, dwFlags)
	if isFallback {
		return fallback, fallbackErr
	}

	result, err := ctx.PipelineAlloc(4, 4)
	if err != nil || result == nil {
		return nil, err
	}

	var bp grayOnlyParams

	// Create a LUT holding the normal ICC transform.
	bp.cmyk2cmyk, err = ctx.DefaultICCintents(preservationCount, iccIntents, profiles, bpc, adaptationStates, dwFlags)
	if err != nil || bp.cmyk2cmyk == nil {
		result.Free()
		return nil, err
	}

	// Now compute the tone curve.
	bp.kTone = ctx.BuildKToneCurve(4096, preservationCount, iccIntents, profiles, bpc, adaptationStates, dwFlags)
	if bp.kTone == nil {
		bp.cmyk2cmyk.Free()
		result.Free()
		return nil, ctx.signalError(ErrNotSuitable, "BlackPreservingKOnly: cannot build K tone curve")
	}

	// How many gridpoints?
	nGridPoints := ReasonableGridpointsByColorspace(SigCmykData, dwFlags)

	clut, err := ctx.StageAllocCLut16bit(nGridPoints, 4, 4, nil)
	if err != nil || clut == nil {
		bp.cmyk2cmyk.Free()
		bp.kTone.Free()
		result.Free()
		return nil, err
	}

	if err := result.InsertStage(AtBegin, clut); err != nil {
		bp.cmyk2cmyk.Free()
		bp.kTone.Free()
		result.Free()
		return nil, err
	}

	// Sample it. No pre/post linearization this time.
	if !clut.SampleCLut16bit(blackPreservingGrayOnlySampler, &bp, 0) {
		bp.cmyk2cmyk.Free()
		bp.kTone.Free()
		result.Free()
		return nil, ctx.signalError(ErrNotSuitable, "BlackPreservingKOnly: sampling failed")
	}

	// Insert possible devicelinks at the end.
	for i := lastProfilePos + 1; i < nProfiles; i++ {
		devlink, derr := profiles[i].ReadDevicelinkLUT(iccIntents[i])
		if derr != nil || devlink == nil {
			bp.cmyk2cmyk.Free()
			bp.kTone.Free()
			result.Free()
			if derr == nil {
				// Preserve the "nil pipeline implies a non-nil error" contract of
				// LinkProfiles even when the devicelink read returned (nil, nil).
				derr = ctx.signalError(ErrNotSuitable, "BlackPreservingKOnly: cannot read devicelink LUT")
			}
			return nil, derr
		}
		if err := result.Cat(devlink); err != nil {
			bp.cmyk2cmyk.Free()
			bp.kTone.Free()
			result.Free()
			return nil, err
		}
	}

	bp.cmyk2cmyk.Free()
	bp.kTone.Free()
	return result, nil
}

// preserveKPlaneParams mirrors PreserveKPlaneParams.
type preserveKPlaneParams struct {
	cmyk2cmyk    *Pipeline  // The original transform
	hProofOutput *Transform // Output CMYK 16-bit to Lab DBL (last profile)
	cmyk2Lab     *Transform // CMYK float to Lab float
	kTone        *ToneCurve // Black-to-black tone curve
	labK2cmyk    *Pipeline  // The output profile input LUT (reversed)
	maxError     float64
	maxTAC       float64
}

// blackPreservingSampler ports BlackPreservingSampler. The CLUT is stored at 16
// bits but calculations run at float32 precision.
func blackPreservingSampler(in, out []uint16, cargo any) bool {
	bp, ok := cargo.(*preserveKPlaneParams)
	if !ok || bp == nil {
		return false
	}

	var inf, outf [4]float32
	var labK [4]float32

	// 16 bits to floating point.
	for i := 0; i < 4; i++ {
		inf[i] = float32(float64(in[i]) / 65535.0)
	}

	// Get the K across the tone curve.
	labK[3] = bp.kTone.EvalFloat(inf[3])

	// If going across black only, keep black only.
	if in[0] == 0 && in[1] == 0 && in[2] == 0 {
		out[0] = 0
		out[1] = 0
		out[2] = 0
		out[3] = quickSaturateWord(float64(labK[3]) * 65535.0)
		return true
	}

	// Try the original transform.
	bp.cmyk2cmyk.EvalFloat(inf[:], outf[:])

	// Store a copy of the float result into 16-bit.
	for i := 0; i < 4; i++ {
		out[i] = quickSaturateWord(float64(outf[i]) * 65535.0)
	}

	// Maybe K is already ok (mostly on K=0).
	if math.Abs(float64(outf[3]-labK[3])) < (3.0 / 65535.0) {
		return true
	}

	// K differs; measure and keep Lab for further usage (relative colorimetric).
	proofIn := u16sToBytes(out[:4])
	colorimetricLabBytes := make([]byte, 24)
	bp.hProofOutput.DoTransform(proofIn, colorimetricLabBytes, 1)
	colorimetricLab := bytesToLab(colorimetricLabBytes)

	// Obtain the Lab of output CMYK. After this we have Lab + K.
	cmyk2LabOut := make([]byte, 12)
	bp.cmyk2Lab.DoTransform(f32sToBytes(outf[:]), cmyk2LabOut, 1)
	lab3 := bytesToF32s(cmyk2LabOut, 3)
	labK[0] = lab3[0]
	labK[1] = lab3[1]
	labK[2] = lab3[2]

	// Obtain the corresponding CMY using reverse interpolation (K fixed).
	if !bp.labK2cmyk.EvalReverseFloat(labK[:], outf[:], outf[:]) {
		// Cannot find a suitable value; use colorimetric xform (in out[]).
		return true
	}

	// Pass through K (now fixed).
	outf[3] = labK[3]

	// Apply TAC if needed.
	sumCMY := float64(outf[0]) + float64(outf[1]) + float64(outf[2])
	sumCMYK := sumCMY + float64(outf[3])

	var ratio float64
	if sumCMYK > bp.maxTAC {
		ratio = 1 - ((sumCMYK - bp.maxTAC) / sumCMY)
		if ratio < 0 {
			ratio = 0
		}
	} else {
		ratio = 1.0
	}

	out[0] = quickSaturateWord(float64(outf[0]) * ratio * 65535.0) // C
	out[1] = quickSaturateWord(float64(outf[1]) * ratio * 65535.0) // M
	out[2] = quickSaturateWord(float64(outf[2]) * ratio * 65535.0) // Y
	out[3] = quickSaturateWord(float64(outf[3]) * 65535.0)

	// Estimate the error (16-bit CMYK to Lab DBL).
	bpLabBytes := make([]byte, 24)
	bp.hProofOutput.DoTransform(u16sToBytes(out[:4]), bpLabBytes, 1)
	blackPreservingLab := bytesToLab(bpLabBytes)
	e := DeltaE(colorimetricLab, blackPreservingLab)
	if e > bp.maxError {
		bp.maxError = e
	}

	return true
}

// blackPreservingKPlaneIntentsFn ports BlackPreservingKPlaneIntents: preserve
// the black plane on CMYK->CMYK chains.
func blackPreservingKPlaneIntentsFn(ctx *Context, nProfiles uint32, theIntents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (*Pipeline, error) {

	iccIntents, preservationCount, lastProfilePos, fallback, fallbackErr, isFallback := blackPreservingCommon(
		ctx, nProfiles, theIntents, profiles, bpc, adaptationStates, dwFlags)
	if isFallback {
		return fallback, fallbackErr
	}

	hLastProfile := profiles[lastProfilePos]

	var bp preserveKPlaneParams
	cleanup := func() {
		if bp.cmyk2cmyk != nil {
			bp.cmyk2cmyk.Free()
		}
		if bp.cmyk2Lab != nil {
			bp.cmyk2Lab.DeleteTransform()
		}
		if bp.hProofOutput != nil {
			bp.hProofOutput.DeleteTransform()
		}
		if bp.kTone != nil {
			bp.kTone.Free()
		}
		if bp.labK2cmyk != nil {
			bp.labK2cmyk.Free()
		}
	}

	// Input LUT of the last profile, searched in inverse order.
	var err error
	bp.labK2cmyk, err = hLastProfile.ReadInputLUT(IntentRelativeColorimetric)
	if err != nil || bp.labK2cmyk == nil {
		cleanup()
		return nil, err
	}

	// Total area coverage (0..1 domain).
	bp.maxTAC = hLastProfile.DetectTAC() / 100.0
	if bp.maxTAC <= 0 {
		cleanup()
		return nil, ctx.signalError(ErrNotSuitable, "BlackPreservingKPlane: TAC detection failed")
	}

	// Normal ICC transform.
	bp.cmyk2cmyk, err = ctx.DefaultICCintents(preservationCount, iccIntents, profiles, bpc, adaptationStates, dwFlags)
	if err != nil || bp.cmyk2cmyk == nil {
		cleanup()
		return nil, err
	}

	// The tone curve.
	bp.kTone = ctx.BuildKToneCurve(4096, preservationCount, iccIntents, profiles, bpc, adaptationStates, dwFlags)
	if bp.kTone == nil {
		cleanup()
		return nil, ctx.signalError(ErrNotSuitable, "BlackPreservingKPlane: cannot build K tone curve")
	}

	// To measure the output, last profile to Lab.
	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		cleanup()
		return nil, ctx.signalError(ErrNull, "BlackPreservingKPlane: cannot create Lab profile")
	}
	bp.hProofOutput, err = ctx.CreateTransform(hLastProfile, channelsSH(4)|bytesSH(2),
		hLab, fmtLabDBL, IntentRelativeColorimetric, FlagsNoCache|FlagsNoOptimize)
	if err != nil || bp.hProofOutput == nil {
		cleanup()
		return nil, err
	}

	// Same, but Lab in the 0..1 range (raw float encoding).
	bp.cmyk2Lab, err = ctx.CreateTransform(hLastProfile, floatSH(1)|channelsSH(4)|bytesSH(4),
		hLab, floatSH(1)|channelsSH(3)|bytesSH(4), IntentRelativeColorimetric, FlagsNoCache|FlagsNoOptimize)
	if err != nil || bp.cmyk2Lab == nil {
		cleanup()
		return nil, err
	}

	bp.maxError = 0

	nGridPoints := ReasonableGridpointsByColorspace(SigCmykData, dwFlags)

	clut, err := ctx.StageAllocCLut16bit(nGridPoints, 4, 4, nil)
	if err != nil || clut == nil {
		cleanup()
		return nil, err
	}

	result, err := ctx.PipelineAlloc(4, 4)
	if err != nil || result == nil {
		cleanup()
		return nil, err
	}

	if err := result.InsertStage(AtBegin, clut); err != nil {
		result.Free()
		cleanup()
		return nil, err
	}

	clut.SampleCLut16bit(blackPreservingSampler, &bp, 0)

	// Insert possible devicelinks at the end.
	for i := lastProfilePos + 1; i < nProfiles; i++ {
		devlink, derr := profiles[i].ReadDevicelinkLUT(iccIntents[i])
		if derr != nil || devlink == nil {
			result.Free()
			cleanup()
			if derr == nil {
				derr = ctx.signalError(ErrNotSuitable, "BlackPreservingKPlane: cannot read devicelink LUT")
			}
			return nil, derr
		}
		if err := result.Cat(devlink); err != nil {
			result.Free()
			cleanup()
			return nil, err
		}
	}

	cleanup()
	return result, nil
}

// ---------------------------------------------------------------------------
// Link routines
// ---------------------------------------------------------------------------

// LinkProfiles ports _cmsLinkProfiles: validate arguments, adjust BPC per the
// Adobe rules, find the handler for the first intent, and dispatch.
func (ctx *Context) LinkProfiles(nProfiles uint32, theIntents []uint32, profiles []*Profile,
	bpc []bool, adaptationStates []float64, dwFlags uint32) (*Pipeline, error) {

	// Make sure a reasonable number of profiles is provided.
	if nProfiles == 0 || nProfiles > 255 {
		return nil, ctx.signalError(ErrRange, "Couldn't link '%d' profiles", nProfiles)
	}
	if uint32(len(theIntents)) < nProfiles || uint32(len(profiles)) < nProfiles ||
		uint32(len(bpc)) < nProfiles || uint32(len(adaptationStates)) < nProfiles {
		return nil, ctx.signalError(ErrRange, "LinkProfiles: argument arrays too short")
	}

	for i := uint32(0); i < nProfiles; i++ {
		// BPC does not apply to devicelink profiles, nor to abs colorimetric,
		// and always applies on V4 perceptual and saturation.
		if theIntents[i] == IntentAbsoluteColorimetric {
			bpc[i] = false
		}
		if theIntents[i] == IntentPerceptual || theIntents[i] == IntentSaturation {
			if profiles[i].GetEncodedICCversion() >= 0x4000000 {
				bpc[i] = true
			}
		}
	}

	// The first intent in the chain defines the handler.
	intent := ctx.searchIntent(theIntents[0])
	if intent == nil {
		return nil, ctx.signalError(ErrUnknownExtension, "Unsupported intent '%d'", theIntents[0])
	}

	return intent.Link(ctx, nProfiles, theIntents, profiles, bpc, adaptationStates, dwFlags)
}

// ---------------------------------------------------------------------------
// Supported intents query
// ---------------------------------------------------------------------------

// GetSupportedIntents ports cmsGetSupportedIntentsTHR: return up to nMax intent
// codes and descriptions, and the total number of supported intents (which may
// exceed nMax). The built-in intents come first, then any plug-in intents.
func (ctx *Context) GetSupportedIntents(nMax uint32) (total uint32, codes []uint32, descriptions []string) {
	appendIntent := func(code uint32, desc string) {
		if total < nMax {
			codes = append(codes, code)
			descriptions = append(descriptions, desc)
		}
		total++
	}

	for i := range defaultIntents {
		appendIntent(defaultIntents[i].Intent, defaultIntents[i].Description)
	}

	if ctx != nil {
		ctx.mu.Lock()
		entries := ctx.intents.entries
		ctx.mu.Unlock()
		for _, e := range entries {
			pl, ok := e.(*PluginRenderingIntent)
			if !ok {
				continue
			}
			appendIntent(pl.Intent, pl.Description)
		}
	}

	return total, codes, descriptions
}

// GetSupportedIntents queries the default context, mirroring
// cmsGetSupportedIntents.
func GetSupportedIntents(nMax uint32) (total uint32, codes []uint32, descriptions []string) {
	return defaultContext.GetSupportedIntents(nMax)
}
