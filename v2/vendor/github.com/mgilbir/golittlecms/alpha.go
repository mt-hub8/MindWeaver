package lcms2

// This file ports src/cmsalpha.c: copying the "extra" (alpha) channels straight
// from the input raster to the output raster when cmsFLAGS_COPY_ALPHA is set,
// across every bit depth (8/16/16SE/half/float/double), for both chunky and
// planar layouts.
//
// PORTNOTES (intentional, faithful deviations):
//   - The C converters take raw void* pointers (dst, src). Here each converter
//     takes (dst []byte, di int, src []byte, si int) and reads/writes through
//     the same bounds-checked helpers pack.go uses (rdU8/wrU16/...), so a
//     malformed or too-small buffer can never panic. On correctly-sized buffers
//     the result is bit-identical to C.
//   - Pointer arithmetic (SourcePtr += SourceIncrements[k]) becomes integer byte
//     offsets into the in/out slices; the starting orders and increments are the
//     same values ComputeComponentIncrements produces in C.

// alphaFn mirrors cmsFormatterAlphaFn: copy one extra-channel sample from src at
// byte offset si to dst at byte offset di, converting between depths.
type alphaFn func(dst []byte, di int, src []byte, si int)

// quickSaturateByte ports _cmsQuickSaturateByte: floor-to-byte with saturation.
func quickSaturateByte(d float64) uint8 {
	d += 0.5
	if d <= 0 {
		return 0
	}
	if d >= 255.0 {
		return 255
	}
	return uint8(quickFloorWord(d))
}

// trueBytesSize ports trueBytesSize: T_BYTES, or 8 (double) when T_BYTES == 0.
func trueBytesSize(format uint32) uint32 {
	fmtBytes := tBytes(format)
	if fmtBytes == 0 {
		return 8
	}
	return fmtBytes
}

// --- From 8 ---------------------------------------------------------------

func alphaCopy8(dst []byte, di int, src []byte, si int) { wrU8(dst, di, rdU8(src, si)) }

func alphaFrom8to16(dst []byte, di int, src []byte, si int) {
	wrU16(dst, di, from8to16(rdU8(src, si)))
}

func alphaFrom8to16SE(dst []byte, di int, src []byte, si int) {
	wrU16(dst, di, changeEndian(from8to16(rdU8(src, si))))
}

func alphaFrom8toFLT(dst []byte, di int, src []byte, si int) {
	wrF32(dst, di, float32(rdU8(src, si))/255.0)
}

func alphaFrom8toDBL(dst []byte, di int, src []byte, si int) {
	wrF64(dst, di, float64(rdU8(src, si))/255.0)
}

func alphaFrom8toHLF(dst []byte, di int, src []byte, si int) {
	n := float32(rdU8(src, si)) / 255.0
	wrU16(dst, di, float2Half(n))
}

// --- From 16 --------------------------------------------------------------

func alphaFrom16to8(dst []byte, di int, src []byte, si int) {
	wrU8(dst, di, from16to8(rdU16(src, si)))
}

func alphaFrom16SEto8(dst []byte, di int, src []byte, si int) {
	wrU8(dst, di, from16to8(changeEndian(rdU16(src, si))))
}

func alphaCopy16(dst []byte, di int, src []byte, si int) { wrU16(dst, di, rdU16(src, si)) }

func alphaFrom16to16(dst []byte, di int, src []byte, si int) {
	wrU16(dst, di, changeEndian(rdU16(src, si)))
}

func alphaFrom16toFLT(dst []byte, di int, src []byte, si int) {
	wrF32(dst, di, float32(rdU16(src, si))/65535.0)
}

func alphaFrom16SEtoFLT(dst []byte, di int, src []byte, si int) {
	wrF32(dst, di, float32(changeEndian(rdU16(src, si)))/65535.0)
}

func alphaFrom16toDBL(dst []byte, di int, src []byte, si int) {
	wrF64(dst, di, float64(rdU16(src, si))/65535.0)
}

func alphaFrom16SEtoDBL(dst []byte, di int, src []byte, si int) {
	wrF64(dst, di, float64(changeEndian(rdU16(src, si)))/65535.0)
}

func alphaFrom16toHLF(dst []byte, di int, src []byte, si int) {
	n := float32(rdU16(src, si)) / 65535.0
	wrU16(dst, di, float2Half(n))
}

func alphaFrom16SEtoHLF(dst []byte, di int, src []byte, si int) {
	n := float32(changeEndian(rdU16(src, si))) / 65535.0
	wrU16(dst, di, float2Half(n))
}

// --- From Float -----------------------------------------------------------

func alphaFromFLTto8(dst []byte, di int, src []byte, si int) {
	wrU8(dst, di, quickSaturateByte(float64(rdF32(src, si))*255.0))
}

func alphaFromFLTto16(dst []byte, di int, src []byte, si int) {
	wrU16(dst, di, quickSaturateWord(float64(rdF32(src, si))*65535.0))
}

func alphaFromFLTto16SE(dst []byte, di int, src []byte, si int) {
	i := quickSaturateWord(float64(rdF32(src, si)) * 65535.0)
	wrU16(dst, di, changeEndian(i))
}

func alphaCopy32(dst []byte, di int, src []byte, si int) { wrF32(dst, di, rdF32(src, si)) }

func alphaFromFLTtoDBL(dst []byte, di int, src []byte, si int) {
	wrF64(dst, di, float64(rdF32(src, si)))
}

func alphaFromFLTtoHLF(dst []byte, di int, src []byte, si int) {
	wrU16(dst, di, float2Half(rdF32(src, si)))
}

// --- From Half ------------------------------------------------------------

func alphaFromHLFto8(dst []byte, di int, src []byte, si int) {
	n := half2Float(rdU16(src, si))
	wrU8(dst, di, quickSaturateByte(float64(n)*255.0))
}

func alphaFromHLFto16(dst []byte, di int, src []byte, si int) {
	n := half2Float(rdU16(src, si))
	wrU16(dst, di, quickSaturateWord(float64(n)*65535.0))
}

func alphaFromHLFto16SE(dst []byte, di int, src []byte, si int) {
	n := half2Float(rdU16(src, si))
	i := quickSaturateWord(float64(n) * 65535.0)
	wrU16(dst, di, changeEndian(i))
}

func alphaFromHLFtoFLT(dst []byte, di int, src []byte, si int) {
	wrF32(dst, di, half2Float(rdU16(src, si)))
}

func alphaFromHLFtoDBL(dst []byte, di int, src []byte, si int) {
	wrF64(dst, di, float64(half2Float(rdU16(src, si))))
}

// --- From Double ----------------------------------------------------------

func alphaFromDBLto8(dst []byte, di int, src []byte, si int) {
	wrU8(dst, di, quickSaturateByte(rdF64(src, si)*255.0))
}

func alphaFromDBLto16(dst []byte, di int, src []byte, si int) {
	wrU16(dst, di, quickSaturateWord(rdF64(src, si)*65535.0))
}

func alphaFromDBLto16SE(dst []byte, di int, src []byte, si int) {
	i := quickSaturateWord(rdF64(src, si) * 65535.0)
	wrU16(dst, di, changeEndian(i))
}

func alphaFromDBLtoFLT(dst []byte, di int, src []byte, si int) {
	wrF32(dst, di, float32(rdF64(src, si)))
}

func alphaFromDBLtoHLF(dst []byte, di int, src []byte, si int) {
	n := float32(rdF64(src, si))
	wrU16(dst, di, float2Half(n))
}

func alphaCopy64(dst []byte, di int, src []byte, si int) { wrF64(dst, di, rdF64(src, si)) }

// --- Dispatch table -------------------------------------------------------

// formattersAlpha ports FormattersAlpha[6][6] (rows = from, cols = to; order
// 8, 16, 16SE, HLF, FLT, DBL).
var formattersAlpha = [6][6]alphaFn{
	/* from 8   */ {alphaCopy8, alphaFrom8to16, alphaFrom8to16SE, alphaFrom8toHLF, alphaFrom8toFLT, alphaFrom8toDBL},
	/* from 16  */ {alphaFrom16to8, alphaCopy16, alphaFrom16to16, alphaFrom16toHLF, alphaFrom16toFLT, alphaFrom16toDBL},
	/* from 16SE*/ {alphaFrom16SEto8, alphaFrom16to16, alphaCopy16, alphaFrom16SEtoHLF, alphaFrom16SEtoFLT, alphaFrom16SEtoDBL},
	/* from HLF */ {alphaFromHLFto8, alphaFromHLFto16, alphaFromHLFto16SE, alphaCopy16, alphaFromHLFtoFLT, alphaFromHLFtoDBL},
	/* from FLT */ {alphaFromFLTto8, alphaFromFLTto16, alphaFromFLTto16SE, alphaFromFLTtoHLF, alphaCopy32, alphaFromFLTtoDBL},
	/* from DBL */ {alphaFromDBLto8, alphaFromDBLto16, alphaFromDBLto16SE, alphaFromDBLtoHLF, alphaFromDBLtoFLT, alphaCopy64},
}

// formatterPos ports FormatterPos: the row/column index for a format's alpha
// width, or -1 when unrecognized.
func formatterPos(frm uint32) int {
	b := tBytes(frm)

	if b == 0 && tFloat(frm) != 0 {
		return 5 // DBL
	}
	if b == 2 && tFloat(frm) != 0 {
		return 3 // HLF
	}
	if b == 4 && tFloat(frm) != 0 {
		return 4 // FLT
	}
	if b == 2 && tFloat(frm) == 0 {
		if tEndian16(frm) != 0 {
			return 2 // 16SE
		}
		return 1 // 16
	}
	if b == 1 && tFloat(frm) == 0 {
		return 0 // 8
	}
	return -1
}

// getFormatterAlpha ports _cmsGetFormatterAlpha.
func getFormatterAlpha(ctx *Context, in, out uint32) alphaFn {
	inN := formatterPos(in)
	outN := formatterPos(out)

	if inN < 0 || outN < 0 || inN > 5 || outN > 5 {
		_ = ctx.signalError(ErrUnknownExtension, "Unrecognized alpha channel width")
		return nil
	}
	return formattersAlpha[inN][outN]
}

// --- Increment computation ------------------------------------------------

// computeIncrementsForChunky ports ComputeIncrementsForChunky.
func computeIncrementsForChunky(format uint32, componentStartingOrder, componentPointerIncrements []uint32) bool {
	var channels [maxChannels]uint32
	extra := tExtra(format)
	nchannels := tChannels(format)
	totalChans := nchannels + extra
	channelSize := trueBytesSize(format)
	pixelSize := channelSize * totalChans

	// Sanity check.
	if totalChans == 0 || totalChans >= maxChannels {
		return false
	}

	// Separation is independent of starting point and only depends on channel size.
	for i := uint32(0); i < extra; i++ {
		componentPointerIncrements[i] = pixelSize
	}

	// Handle do swap.
	for i := uint32(0); i < totalChans; i++ {
		if tDoSwap(format) != 0 {
			channels[i] = totalChans - i - 1
		} else {
			channels[i] = i
		}
	}

	// Handle swap first (ROL of positions): CMYK -> KCMY | 0123 -> 3012.
	if tSwapFirst(format) != 0 && totalChans > 1 {
		tmp := channels[0]
		for i := uint32(0); i < totalChans-1; i++ {
			channels[i] = channels[i+1]
		}
		channels[totalChans-1] = tmp
	}

	// Handle size.
	if channelSize > 1 {
		for i := uint32(0); i < totalChans; i++ {
			channels[i] *= channelSize
		}
	}

	for i := uint32(0); i < extra; i++ {
		componentStartingOrder[i] = channels[i+nchannels]
	}

	return true
}

// computeIncrementsForPlanar ports ComputeIncrementsForPlanar.
func computeIncrementsForPlanar(format, bytesPerPlane uint32, componentStartingOrder, componentPointerIncrements []uint32) bool {
	var channels [maxChannels]uint32
	extra := tExtra(format)
	nchannels := tChannels(format)
	totalChans := nchannels + extra
	channelSize := trueBytesSize(format)

	// Sanity check.
	if totalChans == 0 || totalChans >= maxChannels {
		return false
	}

	// Separation is independent of starting point and only depends on channel size.
	for i := uint32(0); i < extra; i++ {
		componentPointerIncrements[i] = channelSize
	}

	// Handle do swap.
	for i := uint32(0); i < totalChans; i++ {
		if tDoSwap(format) != 0 {
			channels[i] = totalChans - i - 1
		} else {
			channels[i] = i
		}
	}

	// Handle swap first (ROL of positions).
	if tSwapFirst(format) != 0 && totalChans > 0 {
		tmp := channels[0]
		for i := uint32(0); i < totalChans-1; i++ {
			channels[i] = channels[i+1]
		}
		channels[totalChans-1] = tmp
	}

	// Handle size.
	for i := uint32(0); i < totalChans; i++ {
		channels[i] *= bytesPerPlane
	}

	for i := uint32(0); i < extra; i++ {
		componentStartingOrder[i] = channels[i+nchannels]
	}

	return true
}

// computeComponentIncrements ports ComputeComponentIncrements: dispatcher for
// chunky vs planar.
func computeComponentIncrements(format, bytesPerPlane uint32, componentStartingOrder, componentPointerIncrements []uint32) bool {
	if tPlanar(format) != 0 {
		return computeIncrementsForPlanar(format, bytesPerPlane, componentStartingOrder, componentPointerIncrements)
	}
	return computeIncrementsForChunky(format, componentStartingOrder, componentPointerIncrements)
}

// --- The extra-channel copy loop ------------------------------------------

// handleExtraChannels ports _cmsHandleExtraChannels: copy the alpha/extra
// channels from in to out when cmsFLAGS_COPY_ALPHA is set.
func handleExtraChannels(p *Transform, in, out []byte, pixelsPerLine, lineCount uint32, stride Stride) {
	var sourceStartingOrder [maxChannels]uint32
	var sourceIncrements [maxChannels]uint32
	var destStartingOrder [maxChannels]uint32
	var destIncrements [maxChannels]uint32

	// Make sure we need some copy.
	if p.dwOriginalFlags&FlagsCopyAlpha == 0 {
		return
	}

	// Exit early for in-place color management (in == out, same format): no
	// need to copy extra channels to themselves. We compare slice identity via
	// their backing arrays (same length, same first-element address).
	if p.InputFormat == p.OutputFormat && sameSlice(in, out) {
		return
	}

	// Same number of alpha channels required (checked at transform creation).
	nExtra := tExtra(p.InputFormat)
	if nExtra != tExtra(p.OutputFormat) {
		return
	}

	// Anything to do?
	if nExtra == 0 {
		return
	}

	// Compute the increments.
	if !computeComponentIncrements(p.InputFormat, stride.BytesPerPlaneIn, sourceStartingOrder[:], sourceIncrements[:]) {
		return
	}
	if !computeComponentIncrements(p.OutputFormat, stride.BytesPerPlaneOut, destStartingOrder[:], destIncrements[:]) {
		return
	}

	// Depth conversion function.
	copyValueFn := getFormatterAlpha(p.ContextID, p.InputFormat, p.OutputFormat)
	if copyValueFn == nil {
		return
	}

	if nExtra == 1 { // Optimized routine for a single extra channel.
		sourceStrideIncrement := 0
		destStrideIncrement := 0

		for i := uint32(0); i < lineCount; i++ {
			sourcePtr := int(sourceStartingOrder[0]) + sourceStrideIncrement
			destPtr := int(destStartingOrder[0]) + destStrideIncrement

			for j := uint32(0); j < pixelsPerLine; j++ {
				copyValueFn(out, destPtr, in, sourcePtr)
				sourcePtr += int(sourceIncrements[0])
				destPtr += int(destIncrements[0])
			}

			sourceStrideIncrement += int(stride.BytesPerLineIn)
			destStrideIncrement += int(stride.BytesPerLineOut)
		}
	} else { // General case: more than one extra channel.
		var sourcePtr [maxChannels]int
		var destPtr [maxChannels]int
		var sourceStrideIncrements [maxChannels]int
		var destStrideIncrements [maxChannels]int

		for i := uint32(0); i < lineCount; i++ {
			for j := uint32(0); j < nExtra; j++ {
				sourcePtr[j] = int(sourceStartingOrder[j]) + sourceStrideIncrements[j]
				destPtr[j] = int(destStartingOrder[j]) + destStrideIncrements[j]
			}

			for j := uint32(0); j < pixelsPerLine; j++ {
				for k := uint32(0); k < nExtra; k++ {
					copyValueFn(out, destPtr[k], in, sourcePtr[k])
					sourcePtr[k] += int(sourceIncrements[k])
					destPtr[k] += int(destIncrements[k])
				}
			}

			for j := uint32(0); j < nExtra; j++ {
				sourceStrideIncrements[j] += int(stride.BytesPerLineIn)
				destStrideIncrements[j] += int(stride.BytesPerLineOut)
			}
		}
	}
}

// sameSlice reports whether two byte slices share the same backing storage from
// the same offset (the analogue of the C `in == out` pointer comparison).
func sameSlice(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}
