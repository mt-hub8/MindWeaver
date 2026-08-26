package lcms2

// This file ports src/cmsxform.c: transform creation, the per-pixel transform
// engine, the 1-pixel cache, null transforms and the format-switching helpers.
//
// PORTNOTES (intentional, faithful deviations from the C reference):
//
//   - The C _cmsTRANSFORM struct becomes *Transform. Buffers are []byte; the
//     working channel arrays that C keeps on the stack (cmsUInt16Number
//     wIn[cmsMAXCHANNELS] etc.) come from a sync.Pool of xformScratch so that
//     DoTransform allocates nothing in steady state (verified with
//     testing.AllocsPerRun) while remaining safe for concurrent calls: each
//     call owns its own scratch and its own local copy of the 1-pixel cache,
//     exactly as C gives each call its own stack copies.
//
//   - The 1-pixel cache. C copies p->Cache into a stack _cmsCACHE at the top of
//     the cached workers and only mutates that local copy; the transform's seed
//     Cache is written once at creation and read-only thereafter. We do the
//     same: transformCache is a value (two fixed arrays), `cache := p.cache`
//     copies it, and comparison is a plain array equality (the analogue of the
//     memcmp over sizeof(CacheIn)). No shared mutable state during a transform,
//     so -race stays clean with concurrent DoTransform on one *Transform.
//
//   - The optimization hook. C's AllocEmptyTransform calls _cmsOptimizePipeline
//     to (possibly) replace the pipeline with an optimized one. That is
//     implemented in opt.go: (*Context).optimizePipeline installs a specialised
//     evaluator (matrix-shaper, prelinearized/resampled CLUT, joined curves).
//     Passing cmsFLAGS_NOOPTIMIZE keeps the fully-correct unoptimized linked
//     pipeline (evaluated stage by stage) after the trivial-no-op pre-pass.
//
//   - Transform plug-ins (the _cmsTransformCollection factory walk) and the
//     parallelization hook (ParalellizeIfSuitable) are not wired: no such
//     plug-ins exist in the core and parallel.go is W18. The seams
//     (dwOriginalFlags, UserData/FreeUserData, the xform function field) are
//     present so those workers can attach later. See TODO(W18).
//
//   - The linear-RGB "disable optimization" probe in cmsCreateExtendedTransform
//     calls cmsDetectRGBProfileGamma (cmsgmt.c, W14). It only ever sets
//     cmsFLAGS_NOOPTIMIZE; the probe itself is deferred to W14, so linear-RGB
//     transforms are still optimized for now. See TODO(W14).
//
//   - The gamut-check pipeline for proofing/gamut-check transforms is built by
//     _cmsCreateGamutCheckPipeline (cmsgmt.c, W14). The GamutCheck field and the
//     four gamut-check-aware workers are fully ported; only the construction of
//     the marker pipeline is deferred. See TODO(W14).

import "sync"

// ---------------------------------------------------------------------------
// Transform flags (include/lcms2.h + lcms2_internal.h). FlagsHighResPrecalc /
// FlagsLowResPrecalc already live in sigs.go.
// ---------------------------------------------------------------------------

const (
	FlagsNoWhiteOnWhiteFixup    uint32 = 0x0004     // cmsFLAGS_NOWHITEONWHITEFIXUP
	Flags8BitsDeviceLink        uint32 = 0x0008     // cmsFLAGS_8BITS_DEVICELINK
	FlagsGuessDeviceClass       uint32 = 0x0020     // cmsFLAGS_GUESSDEVICECLASS
	FlagsNoCache                uint32 = 0x0040     // cmsFLAGS_NOCACHE
	FlagsKeepSequence           uint32 = 0x0080     // cmsFLAGS_KEEP_SEQUENCE
	FlagsNoOptimize             uint32 = 0x0100     // cmsFLAGS_NOOPTIMIZE
	FlagsNullTransform          uint32 = 0x0200     // cmsFLAGS_NULLTRANSFORM
	FlagsGamutCheck             uint32 = 0x1000     // cmsFLAGS_GAMUTCHECK
	FlagsBlackPointCompensation uint32 = 0x2000     // cmsFLAGS_BLACKPOINTCOMPENSATION
	FlagsSoftProofing           uint32 = 0x4000     // cmsFLAGS_SOFTPROOFING
	FlagsNoNegatives            uint32 = 0x8000     // cmsFLAGS_NONEGATIVES
	FlagsForceCLUT              uint32 = 0x0002     // cmsFLAGS_FORCE_CLUT
	FlagsCLUTPostLinearization  uint32 = 0x0001     // cmsFLAGS_CLUT_POST_LINEARIZATION
	FlagsCLUTPreLinearization   uint32 = 0x0010     // cmsFLAGS_CLUT_PRE_LINEARIZATION
	FlagsNoDefaultResourceDef   uint32 = 0x01000000 // cmsFLAGS_NODEFAULTRESOURCEDEF
	FlagsCanChangeFormatter     uint32 = 0x02000000 // cmsFLAGS_CAN_CHANGE_FORMATTER
	FlagsCopyAlpha              uint32 = 0x04000000 // cmsFLAGS_COPY_ALPHA
)

// FlagsGridPoints ports cmsFLAGS_GRIDPOINTS(n).
func FlagsGridPoints(n uint32) uint32 { return (n & 0xFF) << 16 }

// ---------------------------------------------------------------------------
// Stride, worker function type and 1-pixel cache.
// ---------------------------------------------------------------------------

// Stride mirrors cmsStride: the per-line and per-plane byte strides handed to a
// transform worker.
type Stride struct {
	BytesPerLineIn   uint32
	BytesPerLineOut  uint32
	BytesPerPlaneIn  uint32
	BytesPerPlaneOut uint32
}

// transformFn mirrors _cmsTransform2Fn: the per-scanline transform worker.
type transformFn func(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride)

// transformCache mirrors _cmsCACHE: the read-only 1-pixel cache seed. Each
// cached worker copies it to a local before the loop, so the seed is immutable
// during transforms and the copy makes concurrent DoTransform safe.
type transformCache struct {
	CacheIn  [maxChannels]uint16
	CacheOut [maxChannels]uint16
}

// xformScratch holds the per-call working channel arrays (C's stack wIn/wOut/
// fIn/fOut). Pooled so DoTransform allocates nothing in steady state.
type xformScratch struct {
	wIn  [maxChannels]uint16
	wOut [maxChannels]uint16
	fIn  [maxChannels]float32
	fOut [maxChannels]float32
}

var scratchPool = sync.Pool{New: func() any { return new(xformScratch) }}

// ---------------------------------------------------------------------------
// Transform: the pure-Go _cmsTRANSFORM.
// ---------------------------------------------------------------------------

// Transform is the pure-Go replacement for cmsHTRANSFORM (_cmsTRANSFORM). It is
// safe for concurrent DoTransform calls: no field is mutated during a
// transform. It must not be mutated (ChangeBuffersFormat, DeleteTransform)
// concurrently with a DoTransform.
type Transform struct {
	InputFormat  uint32
	OutputFormat uint32

	// xform is the selected worker (16-bit cached / precalc / float / null).
	xform transformFn

	// Formatters (kept out of the LUT because of the cache).
	fromInput      Formatter16
	toOutput       Formatter16
	fromInputFloat FormatterFloat
	toOutputFloat  FormatterFloat

	// finfo is the (immutable during transforms) FormatterInfo the pixel
	// formatters consult. Rebuilt by ChangeBuffersFormat.
	finfo FormatterInfo

	// cache is the 1-pixel cache seed for zero input (16-bit only, read-only).
	cache transformCache

	// Lut is the full (optimized) transform pipeline.
	Lut *Pipeline

	// GamutCheck goes from the input space to a bilevel out-of-gamut marker.
	GamutCheck *Pipeline

	// Colorant tables (informational).
	InputColorant  *NamedColorList
	OutputColorant *NamedColorList

	// Informational only.
	EntryColorSpace ColorSpaceSignature
	ExitColorSpace  ColorSpaceSignature
	EntryWhitePoint CIEXYZ
	ExitWhitePoint  CIEXYZ

	// Profile sequence (kept when cmsFLAGS_KEEP_SEQUENCE is set).
	Sequence *ProfileSequence

	dwOriginalFlags uint32
	adaptationState float64
	RenderingIntent uint32

	ContextID *Context

	// Plug-in private data seam (W18).
	userData     any
	freeUserData func(ctx *Context, data any)
}

// ---------------------------------------------------------------------------
// Small buffer helpers (never panic on out-of-range offsets).
// ---------------------------------------------------------------------------

// sliceFrom returns b[off:] clamped so that an over-large offset yields an empty
// tail rather than panicking. The bounds-checked formatter accessors then read/
// write nothing, matching how a correctly-sized buffer behaves.
func sliceFrom(b []byte, off int) []byte {
	if off < 0 {
		off = 0
	}
	if off >= len(b) {
		return b[len(b):]
	}
	return b[off:]
}

// pixelSizeFmt ports PixelSize(Format): T_BYTES, or 8 for double (T_BYTES == 0).
// pack.go already provides pixelSize with identical semantics; reuse it.
func pixelSizeFmt(format uint32) uint32 { return uint32(pixelSize(format)) }

// ---------------------------------------------------------------------------
// The transform entry points (cmsDoTransform family).
// ---------------------------------------------------------------------------

// DoTransform ports cmsDoTransform: transform size pixels from in to out.
func (p *Transform) DoTransform(in, out []byte, size uint32) {
	if p == nil || p.xform == nil {
		return
	}
	stride := Stride{
		BytesPerLineIn:   0,
		BytesPerLineOut:  0,
		BytesPerPlaneIn:  size * pixelSizeFmt(p.InputFormat),
		BytesPerPlaneOut: size * pixelSizeFmt(p.OutputFormat),
	}
	p.xform(p, in, out, size, 1, stride)
}

// DoTransformStride ports cmsDoTransformStride: the legacy planar stride entry.
func (p *Transform) DoTransformStride(in, out []byte, size, strideBytes uint32) {
	if p == nil || p.xform == nil {
		return
	}
	stride := Stride{
		BytesPerLineIn:   0,
		BytesPerLineOut:  0,
		BytesPerPlaneIn:  strideBytes,
		BytesPerPlaneOut: strideBytes,
	}
	p.xform(p, in, out, size, 1, stride)
}

// DoTransformLineStride ports cmsDoTransformLineStride: the full stride API.
func (p *Transform) DoTransformLineStride(in, out []byte,
	pixelsPerLine, lineCount,
	bytesPerLineIn, bytesPerLineOut,
	bytesPerPlaneIn, bytesPerPlaneOut uint32) {
	if p == nil || p.xform == nil {
		return
	}
	stride := Stride{
		BytesPerLineIn:   bytesPerLineIn,
		BytesPerLineOut:  bytesPerLineOut,
		BytesPerPlaneIn:  bytesPerPlaneIn,
		BytesPerPlaneOut: bytesPerPlaneOut,
	}
	p.xform(p, in, out, pixelsPerLine, lineCount, stride)
}

// ---------------------------------------------------------------------------
// Transform workers (the _cmsTransform2Fn implementations).
// ---------------------------------------------------------------------------

// floatXFORM ports FloatXFORM. One routine covers float with/without gamut
// check; no cache.
func floatXFORM(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.fIn = [maxChannels]float32{}
	s.fOut = [maxChannels]float32{}

	var alarm [maxChannels]uint16
	if p.GamutCheck != nil {
		alarm = p.ContextID.GetAlarmCodes()
	}

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInputFloat(&p.finfo, s.fIn[:], sliceFrom(in, accum), sbppIn)

			if p.GamutCheck != nil {
				var oog [1]float32
				p.GamutCheck.EvalFloat(s.fIn[:], oog[:])
				if oog[0] > 0.0 {
					for c := 0; c < maxChannels; c++ {
						s.fOut[c] = float32(alarm[c]) / 65535.0
					}
				} else {
					p.Lut.EvalFloat(s.fIn[:], s.fOut[:])
				}
			} else {
				p.Lut.EvalFloat(s.fIn[:], s.fOut[:])
			}

			output += p.toOutputFloat(&p.finfo, s.fOut[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// nullFloatXFORM ports NullFloatXFORM: apply only the formatters (float).
func nullFloatXFORM(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.fIn = [maxChannels]float32{}

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInputFloat(&p.finfo, s.fIn[:], sliceFrom(in, accum), sbppIn)
			output += p.toOutputFloat(&p.finfo, s.fIn[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// nullXFORM ports NullXFORM: apply only the formatters (16-bit). No cache.
func nullXFORM(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.wIn = [maxChannels]uint16{}

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInput(&p.finfo, s.wIn[:], sliceFrom(in, accum), sbppIn)
			output += p.toOutput(&p.finfo, s.wIn[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// precalculatedXFORM ports PrecalculatedXFORM: 16-bit, no gamut check, no cache.
func precalculatedXFORM(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.wIn = [maxChannels]uint16{}
	s.wOut = [maxChannels]uint16{}

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInput(&p.finfo, s.wIn[:], sliceFrom(in, accum), sbppIn)
			p.Lut.Eval16(s.wIn[:], s.wOut[:])
			output += p.toOutput(&p.finfo, s.wOut[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// transformOnePixelWithGamutCheck ports TransformOnePixelWithGamutCheck.
func transformOnePixelWithGamutCheck(p *Transform, wIn, wOut []uint16, alarm *[maxChannels]uint16) {
	var oog [1]uint16
	p.GamutCheck.Eval16(wIn, oog[:])
	if oog[0] >= 1 {
		n := p.Lut.OutputChannels
		for i := uint32(0); i < n && int(i) < len(wOut); i++ {
			wOut[i] = alarm[i]
		}
	} else {
		p.Lut.Eval16(wIn, wOut)
	}
}

// precalculatedXFORMGamutCheck ports PrecalculatedXFORMGamutCheck.
func precalculatedXFORMGamutCheck(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.wIn = [maxChannels]uint16{}
	s.wOut = [maxChannels]uint16{}
	// Intentional deviation: the alarm codes are read once per DoTransform call
	// rather than per out-of-gamut pixel as the reference does, so a concurrent
	// SetAlarmCodes takes effect on the next call instead of mid-buffer — the
	// saner behaviour for the concurrency-safe transform contract.
	alarm := p.ContextID.GetAlarmCodes()

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInput(&p.finfo, s.wIn[:], sliceFrom(in, accum), sbppIn)
			transformOnePixelWithGamutCheck(p, s.wIn[:], s.wOut[:], &alarm)
			output += p.toOutput(&p.finfo, s.wOut[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// cachedXFORM ports CachedXFORM: 16-bit, 1-pixel cache, no gamut check.
func cachedXFORM(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.wIn = [maxChannels]uint16{}
	s.wOut = [maxChannels]uint16{}

	// Local copy of the read-only cache seed (per-call, race-free).
	cache := p.cache

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInput(&p.finfo, s.wIn[:], sliceFrom(in, accum), sbppIn)

			if s.wIn == cache.CacheIn {
				s.wOut = cache.CacheOut
			} else {
				p.Lut.Eval16(s.wIn[:], s.wOut[:])
				cache.CacheIn = s.wIn
				cache.CacheOut = s.wOut
			}

			output += p.toOutput(&p.finfo, s.wOut[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// cachedXFORMGamutCheck ports CachedXFORMGamutCheck: cache + gamut check.
func cachedXFORMGamutCheck(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	handleExtraChannels(p, in, out, pixelsPerLine, lineCount, stride)

	s := scratchPool.Get().(*xformScratch)
	defer scratchPool.Put(s)
	s.wIn = [maxChannels]uint16{}
	s.wOut = [maxChannels]uint16{}
	alarm := p.ContextID.GetAlarmCodes()

	cache := p.cache

	strideIn := 0
	strideOut := 0
	sbppIn := int(stride.BytesPerPlaneIn)
	sbppOut := int(stride.BytesPerPlaneOut)

	for i := uint32(0); i < lineCount; i++ {
		accum := strideIn
		output := strideOut
		for j := uint32(0); j < pixelsPerLine; j++ {
			accum += p.fromInput(&p.finfo, s.wIn[:], sliceFrom(in, accum), sbppIn)

			if s.wIn == cache.CacheIn {
				s.wOut = cache.CacheOut
			} else {
				transformOnePixelWithGamutCheck(p, s.wIn[:], s.wOut[:], &alarm)
				cache.CacheIn = s.wIn
				cache.CacheOut = s.wOut
			}

			output += p.toOutput(&p.finfo, s.wOut[:], sliceFrom(out, output), sbppOut)
		}
		strideIn += int(stride.BytesPerLineIn)
		strideOut += int(stride.BytesPerLineOut)
	}
}

// ---------------------------------------------------------------------------
// The empty formatters (UnrollNothing / PackNothing).
// ---------------------------------------------------------------------------

func unrollNothing(info *FormatterInfo, values []uint16, buf []byte, stride int) int { return 0 }
func packNothing(info *FormatterInfo, values []uint16, buf []byte, stride int) int   { return 0 }

// ---------------------------------------------------------------------------
// The optimization hook seam (W13).
// ---------------------------------------------------------------------------

// optimizePipeline (the seam for _cmsOptimizePipeline) is implemented in opt.go
// (cmsopt.c, W13). It may replace *lut with an optimized pipeline (installing a
// specialised evaluator) and adjust the formats/flags.

// ---------------------------------------------------------------------------
// allocEmptyTransform (AllocEmptyTransform).
// ---------------------------------------------------------------------------

// allocEmptyTransform ports AllocEmptyTransform: allocate the transform, run the
// optimization hook, choose the formatters and the worker function. It returns
// (nil, error) on an unsupported raster format or mismatched alpha channels.
func allocEmptyTransform(ctx *Context, lut *Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) (*Transform, error) {

	p := &Transform{Lut: lut}

	if p.Lut != nil {
		// Transform plug-in factory walk is not wired (see PORTNOTES). Consult
		// the pipeline optimization hook (W13).
		ctx.optimizePipeline(&p.Lut, intent, inputFormat, outputFormat, dwFlags)
	}

	// True floating point transform?
	if FormatterIsFloat(*inputFormat) || FormatterIsFloat(*outputFormat) {
		p.fromInputFloat = ctx.GetFormatter(*inputFormat, FormatterInput, PackFlagsFloat).FmtFloat
		p.toOutputFloat = ctx.GetFormatter(*outputFormat, FormatterOutput, PackFlagsFloat).FmtFloat
		*dwFlags |= FlagsCanChangeFormatter

		if p.fromInputFloat == nil || p.toOutputFloat == nil {
			return nil, ctx.signalError(ErrUnknownExtension, "Unsupported raster format")
		}

		if *dwFlags&FlagsNullTransform != 0 {
			p.xform = nullFloatXFORM
		} else {
			p.xform = floatXFORM
		}
	} else {
		if *inputFormat == 0 && *outputFormat == 0 {
			p.fromInput = unrollNothing
			p.toOutput = packNothing
			*dwFlags |= FlagsCanChangeFormatter
		} else {
			p.fromInput = ctx.GetFormatter(*inputFormat, FormatterInput, PackFlags16Bits).Fmt16
			p.toOutput = ctx.GetFormatter(*outputFormat, FormatterOutput, PackFlags16Bits).Fmt16

			if p.fromInput == nil || p.toOutput == nil {
				return nil, ctx.signalError(ErrUnknownExtension, "Unsupported raster format")
			}

			bytesPerPixelInput := tBytes(*inputFormat)
			if bytesPerPixelInput == 0 || bytesPerPixelInput >= 2 {
				*dwFlags |= FlagsCanChangeFormatter
			}
		}

		if *dwFlags&FlagsNullTransform != 0 {
			p.xform = nullXFORM
		} else {
			if *dwFlags&FlagsNoCache != 0 {
				if *dwFlags&FlagsGamutCheck != 0 {
					p.xform = precalculatedXFORMGamutCheck
				} else {
					p.xform = precalculatedXFORM
				}
			} else {
				if *dwFlags&FlagsGamutCheck != 0 {
					p.xform = cachedXFORMGamutCheck
				} else {
					p.xform = cachedXFORM
				}
			}
		}
	}

	// Consistency check for alpha channel copy.
	if *dwFlags&FlagsCopyAlpha != 0 {
		if tExtra(*inputFormat) != tExtra(*outputFormat) {
			return nil, ctx.signalError(ErrNotSuitable, "Mismatched alpha channels")
		}
	}

	p.InputFormat = *inputFormat
	p.OutputFormat = *outputFormat
	p.finfo = FormatterInfo{InputFormat: *inputFormat, OutputFormat: *outputFormat}
	p.dwOriginalFlags = *dwFlags
	p.ContextID = ctx
	return p, nil
}

// ---------------------------------------------------------------------------
// Colorspace probing (GetXFormColorSpaces / IsProperColorSpace).
// ---------------------------------------------------------------------------

// getXFormColorSpaces ports GetXFormColorSpaces: the entry/exit color spaces of
// the whole profile chain.
func getXFormColorSpaces(nProfiles uint32, profiles []*Profile) (input, output ColorSpaceSignature, ok bool) {
	if nProfiles == 0 || profiles[0] == nil {
		return 0, 0, false
	}

	postColorSpace := profiles[0].GetColorSpace()
	input = postColorSpace

	for i := uint32(0); i < nProfiles; i++ {
		hProfile := profiles[i]
		if hProfile == nil {
			return 0, 0, false
		}

		lIsInput := postColorSpace != SigXYZData && postColorSpace != SigLabData
		cls := hProfile.GetDeviceClass()

		var colorSpaceIn, colorSpaceOut ColorSpaceSignature
		switch {
		case cls == SigNamedColorClass:
			colorSpaceIn = Sig1colorData
			if nProfiles > 1 {
				colorSpaceOut = hProfile.GetPCS()
			} else {
				colorSpaceOut = hProfile.GetColorSpace()
			}
		case lIsInput || cls == SigLinkClass:
			colorSpaceIn = hProfile.GetColorSpace()
			colorSpaceOut = hProfile.GetPCS()
		default:
			colorSpaceIn = hProfile.GetPCS()
			colorSpaceOut = hProfile.GetColorSpace()
		}

		if i == 0 {
			input = colorSpaceIn
		}
		postColorSpace = colorSpaceOut
	}

	return input, postColorSpace, true
}

// isProperColorSpace ports IsProperColorSpace.
func isProperColorSpace(check ColorSpaceSignature, dwFormat uint32) bool {
	space1 := int(tColorspace(dwFormat))
	space2 := LCMScolorSpace(check)

	if dwFormat == 0 {
		return true // Bypass used by linkicc
	}

	if space1 == PTANY {
		return tChannels(dwFormat) == ChannelsOf(check)
	}
	if space1 == space2 {
		return true
	}
	if space1 == PTLabV2 && space2 == PTLab {
		return true
	}
	if space1 == PTLab && space2 == PTLabV2 {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// White point sanity (NormalizeXYZ / SetWhitePoint).
// ---------------------------------------------------------------------------

// normalizeXYZ ports NormalizeXYZ: some profiles carry media white x 100.
func normalizeXYZ(dest *CIEXYZ) {
	for dest.X > 2.0 && dest.Y > 2.0 && dest.Z > 2.0 {
		dest.X /= 10.0
		dest.Y /= 10.0
		dest.Z /= 10.0
	}
}

// setWhitePoint ports SetWhitePoint.
func setWhitePoint(wtPt *CIEXYZ, src *CIEXYZ) {
	if src == nil {
		wtPt.X = D50X
		wtPt.Y = D50Y
		wtPt.Z = D50Z
	} else {
		*wtPt = *src
		normalizeXYZ(wtPt)
	}
}

// ---------------------------------------------------------------------------
// cmsCreateExtendedTransform.
// ---------------------------------------------------------------------------

// CreateExtendedTransform ports cmsCreateExtendedTransform: the fully-parameterised
// transform builder that every other constructor funnels into.
func (ctx *Context) CreateExtendedTransform(
	nProfiles uint32, profiles []*Profile,
	bpc []bool, intents []uint32, adaptationStates []float64,
	gamutProfile *Profile, nGamutPCSposition uint32,
	inputFormat, outputFormat, dwFlags uint32) (*Transform, error) {

	if nProfiles == 0 || nProfiles > 255 {
		return nil, ctx.signalError(ErrRange, "Wrong number of profiles. 1..255 expected, %d found.", nProfiles)
	}
	if uint32(len(profiles)) < nProfiles || uint32(len(intents)) < nProfiles ||
		uint32(len(bpc)) < nProfiles || uint32(len(adaptationStates)) < nProfiles {
		return nil, ctx.signalError(ErrRange, "CreateExtendedTransform: argument arrays too short")
	}

	lastIntent := intents[nProfiles-1]

	// A fake transform.
	if dwFlags&FlagsNullTransform != 0 {
		return allocEmptyTransform(ctx, nil, IntentPerceptual, &inputFormat, &outputFormat, &dwFlags)
	}

	// Gamut check needs a gamut profile.
	if dwFlags&FlagsGamutCheck != 0 && gamutProfile == nil {
		dwFlags &^= FlagsGamutCheck
	}

	if dwFlags&FlagsGamutCheck != 0 && (nGamutPCSposition == 0 || nGamutPCSposition > nProfiles-1) {
		return nil, ctx.signalError(ErrRange, "Wrong gamut PCS position '%d'", nGamutPCSposition)
	}

	// On floating point transforms, inhibit cache.
	if FormatterIsFloat(inputFormat) || FormatterIsFloat(outputFormat) {
		dwFlags |= FlagsNoCache
	}

	entryColorSpace, exitColorSpace, ok := getXFormColorSpaces(nProfiles, profiles)
	if !ok {
		return nil, ctx.signalError(ErrNull, "NULL input profiles on transform")
	}

	if !isProperColorSpace(entryColorSpace, inputFormat) {
		return nil, ctx.signalError(ErrColorspaceCheck, "Wrong input color space on transform")
	}
	if !isProperColorSpace(exitColorSpace, outputFormat) {
		return nil, ctx.signalError(ErrColorspaceCheck, "Wrong output color space on transform")
	}

	// For a near-linear 16-bit RGB input profile, disable optimization so the
	// full-precision pipeline is kept: the resampling optimizer would otherwise
	// collapse it into a grid CLUT and band the near-linear ramp (cmsxform.c).
	if entryColorSpace == SigRgbData && tBytes(inputFormat) == 2 && dwFlags&FlagsNoOptimize == 0 {
		if gamma := profiles[0].DetectRGBProfileGamma(0.1); gamma > 0 && gamma < 1.6 {
			dwFlags |= FlagsNoOptimize
		}
	}

	// Create a pipeline with all transformations.
	lut, err := ctx.LinkProfiles(nProfiles, intents, profiles, bpc, adaptationStates, dwFlags)
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNotSuitable, "Couldn't link the profiles")
	}

	// Check channel count.
	if uint32(ChannelsOfColorSpace(entryColorSpace)) != lut.InputChannelsCount() ||
		uint32(ChannelsOfColorSpace(exitColorSpace)) != lut.OutputChannelsCount() {
		lut.Free()
		return nil, ctx.signalError(ErrNotSuitable, "Channel count doesn't match. Profile is corrupted")
	}

	xform, err := allocEmptyTransform(ctx, lut, lastIntent, &inputFormat, &outputFormat, &dwFlags)
	if err != nil {
		return nil, err
	}

	xform.EntryColorSpace = entryColorSpace
	xform.ExitColorSpace = exitColorSpace
	xform.RenderingIntent = intents[nProfiles-1]

	// White points.
	wpIn, _ := profiles[0].readTagPtr(SigMediaWhitePointTag).(*CIEXYZ)
	wpOut, _ := profiles[nProfiles-1].readTagPtr(SigMediaWhitePointTag).(*CIEXYZ)
	setWhitePoint(&xform.EntryWhitePoint, wpIn)
	setWhitePoint(&xform.ExitWhitePoint, wpOut)

	// Gamut check LUT (cmsgmt.c). The marker pipeline flags out-of-gamut
	// pixels; the four gamut-check-aware workers dereference xform.GamutCheck.
	if gamutProfile != nil && dwFlags&FlagsGamutCheck != 0 {
		xform.GamutCheck = ctx.createGamutCheckPipeline(profiles[:nProfiles], bpc, intents,
			adaptationStates, nGamutPCSposition, gamutProfile)
		// The gamut-check-aware workers were already selected from the flag and
		// dereference xform.GamutCheck per pixel. createGamutCheckPipeline can
		// fail (e.g. the gamut profile's reverse transform cannot be built);
		// rather than hand back a transform that panics on the first pixel, fail
		// creation here.
		if xform.GamutCheck == nil {
			return nil, ctx.signalError(ErrColorspaceCheck,
				"cannot build a gamut-check pipeline from the supplied gamut profile")
		}
	}

	// Input / output colorant tables.
	if profiles[0].IsTag(SigColorantTableTag) {
		if nc, ok := profiles[0].readTagPtr(SigColorantTableTag).(*NamedColorList); ok && nc != nil {
			xform.InputColorant = nc.Dup()
		}
	}

	last := profiles[nProfiles-1]
	if last.GetDeviceClass() == SigLinkClass {
		if last.IsTag(SigColorantTableOutTag) {
			if nc, ok := last.readTagPtr(SigColorantTableOutTag).(*NamedColorList); ok && nc != nil {
				xform.OutputColorant = nc.Dup()
			}
		}
	} else {
		if last.IsTag(SigColorantTableTag) {
			if nc, ok := last.readTagPtr(SigColorantTableTag).(*NamedColorList); ok && nc != nil {
				xform.OutputColorant = nc.Dup()
			}
		}
	}

	// Profile sequence.
	if dwFlags&FlagsKeepSequence != 0 {
		xform.Sequence = ctx.CompileProfileSequence(profiles[:nProfiles])
	}

	// Seed the 1-pixel cache (16-bit) for zero input.
	if dwFlags&FlagsNoCache == 0 {
		xform.cache.CacheIn = [maxChannels]uint16{}
		if xform.GamutCheck != nil {
			alarm := ctx.GetAlarmCodes()
			transformOnePixelWithGamutCheck(xform, xform.cache.CacheIn[:], xform.cache.CacheOut[:], &alarm)
		} else {
			xform.Lut.Eval16(xform.cache.CacheIn[:], xform.cache.CacheOut[:])
		}
	}

	return xform, nil
}

// ---------------------------------------------------------------------------
// Multiprofile / simple / proofing constructors.
// ---------------------------------------------------------------------------

// CreateMultiprofileTransform ports cmsCreateMultiprofileTransformTHR.
func (ctx *Context) CreateMultiprofileTransform(profiles []*Profile, nProfiles,
	inputFormat, outputFormat, intent, dwFlags uint32) (*Transform, error) {

	if nProfiles == 0 || nProfiles > 255 {
		return nil, ctx.signalError(ErrRange, "Wrong number of profiles. 1..255 expected, %d found.", nProfiles)
	}
	if uint32(len(profiles)) < nProfiles {
		return nil, ctx.signalError(ErrRange, "CreateMultiprofileTransform: too few profiles")
	}

	bpc := make([]bool, nProfiles)
	intents := make([]uint32, nProfiles)
	adaptationStates := make([]float64, nProfiles)

	for i := uint32(0); i < nProfiles; i++ {
		bpc[i] = dwFlags&FlagsBlackPointCompensation != 0
		intents[i] = intent
		adaptationStates[i] = ctx.SetAdaptationState(-1)
	}

	return ctx.CreateExtendedTransform(nProfiles, profiles, bpc, intents, adaptationStates,
		nil, 0, inputFormat, outputFormat, dwFlags)
}

// CreateMultiprofileTransform ports cmsCreateMultiprofileTransform (default context).
func CreateMultiprofileTransform(profiles []*Profile, nProfiles,
	inputFormat, outputFormat, intent, dwFlags uint32) (*Transform, error) {

	if nProfiles == 0 || nProfiles > 255 {
		return nil, defaultContext.signalError(ErrRange,
			"Wrong number of profiles. 1..255 expected, %d found.", nProfiles)
	}
	var ctx *Context
	if len(profiles) > 0 {
		ctx = profileContextID(profiles[0])
	} else {
		ctx = defaultContext
	}
	return ctx.CreateMultiprofileTransform(profiles, nProfiles, inputFormat, outputFormat, intent, dwFlags)
}

// CreateTransform ports cmsCreateTransformTHR.
func (ctx *Context) CreateTransform(input *Profile, inputFormat uint32,
	output *Profile, outputFormat, intent, dwFlags uint32) (*Transform, error) {

	arr := []*Profile{input, output}
	n := uint32(2)
	if output == nil {
		n = 1
	}
	return ctx.CreateMultiprofileTransform(arr, n, inputFormat, outputFormat, intent, dwFlags)
}

// CreateTransform ports cmsCreateTransform (default/profile context).
func CreateTransform(input *Profile, inputFormat uint32,
	output *Profile, outputFormat, intent, dwFlags uint32) (*Transform, error) {

	return profileContextID(input).CreateTransform(input, inputFormat, output, outputFormat, intent, dwFlags)
}

// CreateProofingTransform ports cmsCreateProofingTransformTHR.
func (ctx *Context) CreateProofingTransform(
	input *Profile, inputFormat uint32,
	output *Profile, outputFormat uint32,
	proofing *Profile, nIntent, proofingIntent, dwFlags uint32) (*Transform, error) {

	arr := []*Profile{input, proofing, proofing, output}
	intents := []uint32{nIntent, nIntent, IntentRelativeColorimetric, proofingIntent}
	doBPC := dwFlags&FlagsBlackPointCompensation != 0
	bpc := []bool{doBPC, doBPC, false, false}
	adaptation := ctx.SetAdaptationState(-1)
	adaptationStates := []float64{adaptation, adaptation, adaptation, adaptation}

	if dwFlags&(FlagsSoftProofing|FlagsGamutCheck) == 0 {
		return ctx.CreateTransform(input, inputFormat, output, outputFormat, nIntent, dwFlags)
	}

	return ctx.CreateExtendedTransform(4, arr, bpc, intents, adaptationStates,
		proofing, 1, inputFormat, outputFormat, dwFlags)
}

// CreateProofingTransform ports cmsCreateProofingTransform (profile context).
func CreateProofingTransform(
	input *Profile, inputFormat uint32,
	output *Profile, outputFormat uint32,
	proofing *Profile, nIntent, proofingIntent, dwFlags uint32) (*Transform, error) {

	return profileContextID(input).CreateProofingTransform(input, inputFormat,
		output, outputFormat, proofing, nIntent, proofingIntent, dwFlags)
}

// profileContextID returns the profile's context, or the default context when
// the profile (or its context) is nil. Mirrors cmsGetProfileContextID + the
// NULL-context fallback the *THR wrappers rely on.
func profileContextID(p *Profile) *Context {
	if p == nil || p.ContextID == nil {
		return defaultContext
	}
	return p.ContextID
}

// ---------------------------------------------------------------------------
// Accessors / mutators (the rest of cmsxform.c).
// ---------------------------------------------------------------------------

// DeleteTransform ports cmsDeleteTransform. Under the Go GC there is nothing to
// free; the user-data free hook is still honoured for plug-in parity.
func (p *Transform) DeleteTransform() {
	if p == nil {
		return
	}
	if p.freeUserData != nil && p.userData != nil {
		p.freeUserData(p.ContextID, p.userData)
		p.userData = nil
	}
	p.GamutCheck = nil
	p.Lut = nil
	p.InputColorant = nil
	p.OutputColorant = nil
	p.Sequence = nil
	// Clearing the worker makes a subsequent DoTransform a no-op rather than a
	// nil-pointer dereference through the now-nil Lut.
	p.xform = nil
}

// GetTransformContextID ports cmsGetTransformContextID.
func (p *Transform) GetTransformContextID() *Context {
	if p == nil {
		return nil
	}
	return p.ContextID
}

// GetTransformInputFormat ports cmsGetTransformInputFormat.
func (p *Transform) GetTransformInputFormat() uint32 {
	if p == nil {
		return 0
	}
	return p.InputFormat
}

// GetTransformOutputFormat ports cmsGetTransformOutputFormat.
func (p *Transform) GetTransformOutputFormat() uint32 {
	if p == nil {
		return 0
	}
	return p.OutputFormat
}

// GetTransformPipeline ports cmsGetTransformPipeline (read-only; do not free).
func (p *Transform) GetTransformPipeline() *Pipeline {
	if p == nil {
		return nil
	}
	return p.Lut
}

// GetTransformGamutCheckPipeline ports cmsGetTransformGamutCheckPipeline.
func (p *Transform) GetTransformGamutCheckPipeline() *Pipeline {
	if p == nil {
		return nil
	}
	return p.GamutCheck
}

// GetTransformInputColorants ports cmsGetTransformInputColorants.
func (p *Transform) GetTransformInputColorants() *NamedColorList {
	if p == nil {
		return nil
	}
	return p.InputColorant
}

// GetTransformOutputColorants ports cmsGetTransformOutputColorants.
func (p *Transform) GetTransformOutputColorants() *NamedColorList {
	if p == nil {
		return nil
	}
	return p.OutputColorant
}

// GetNamedColorList ports cmsGetNamedColorList: return the named-color list a
// named-color transform carries. The reference reads it straight off the first
// pipeline stage when that stage is a named-color element, and returns NULL for
// any other transform. Mirroring that keeps the accessor free of extra state on
// the Transform.
func (p *Transform) GetNamedColorList() *NamedColorList {
	if p == nil || p.Lut == nil {
		return nil
	}
	mpe := p.Lut.GetPtrToFirstStage()
	if mpe == nil || mpe.StageType() != SigNamedColorElemType {
		return nil
	}
	list, _ := mpe.Data().(*NamedColorList)
	return list
}

// ChangeBuffersFormat ports cmsChangeBuffersFormat: swap the input/output pixel
// formats of an existing (>= 16-bit) transform, rebuilding the formatters.
func (p *Transform) ChangeBuffersFormat(inputFormat, outputFormat uint32) error {
	if p == nil {
		return errorf(ErrNull, "nil transform")
	}

	if p.dwOriginalFlags&FlagsCanChangeFormatter == 0 {
		return p.ContextID.signalError(ErrNotSuitable,
			"cmsChangeBuffersFormat works only on transforms created originally with at least 16 bits of precision")
	}

	fromInput := p.ContextID.GetFormatter(inputFormat, FormatterInput, PackFlags16Bits).Fmt16
	toOutput := p.ContextID.GetFormatter(outputFormat, FormatterOutput, PackFlags16Bits).Fmt16

	if fromInput == nil || toOutput == nil {
		return p.ContextID.signalError(ErrUnknownExtension, "Unsupported raster format")
	}

	p.InputFormat = inputFormat
	p.OutputFormat = outputFormat
	p.finfo = FormatterInfo{InputFormat: inputFormat, OutputFormat: outputFormat}
	p.fromInput = fromInput
	p.toOutput = toOutput
	return nil
}

// SetTransformUserData ports _cmsSetTransformUserData.
func (p *Transform) SetTransformUserData(ptr any, freeFn func(ctx *Context, data any)) {
	if p == nil {
		return
	}
	p.userData = ptr
	p.freeUserData = freeFn
}

// GetTransformUserData ports _cmsGetTransformUserData.
func (p *Transform) GetTransformUserData() any {
	if p == nil {
		return nil
	}
	return p.userData
}

// GetTransformFormatters16 ports _cmsGetTransformFormatters16.
func (p *Transform) GetTransformFormatters16() (fromInput, toOutput Formatter16) {
	if p == nil {
		return nil, nil
	}
	return p.fromInput, p.toOutput
}

// GetTransformFormattersFloat ports _cmsGetTransformFormattersFloat.
func (p *Transform) GetTransformFormattersFloat() (fromInput, toOutput FormatterFloat) {
	if p == nil {
		return nil, nil
	}
	return p.fromInputFloat, p.toOutputFloat
}

// GetTransformFlags ports _cmsGetTransformFlags.
func (p *Transform) GetTransformFlags() uint32 {
	if p == nil {
		return 0
	}
	return p.dwOriginalFlags
}
