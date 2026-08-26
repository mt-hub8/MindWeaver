package lcms2

// This file ports src/cmspack.c: the pixel formatters. Every TYPE_* pixel
// format has an "unpack" routine (input bytes -> internal channel array) and a
// "pack" routine (internal channels -> output bytes). The internal working
// array is []uint16 for the 16-bit engine and []float32 for the float engine,
// exactly mirroring the cmsUInt16Number wIn[] / cmsFloat32Number wIn[] of C.
//
// The dispatch tables (InputFormatters16, OutputFormatters16,
// InputFormattersFloat, OutputFormattersFloat) and the T_* bitfield macro
// helpers are ported verbatim; matching is `(dwInput & ~Mask) == Type` just as
// in _cmsGetStockInputFormatter / _cmsGetStockOutputFormatter.
//
// Unpack routines live in pack_unpack.go, pack routines in pack_pack.go.
//
// PORTNOTES (intentional faithful deviations from a "clean" implementation):
//   - The C formatters take the whole _cmsTRANSFORM and read only its
//     InputFormat / OutputFormat fields. We pass a small *FormatterInfo instead
//     (W12's Transform will supply one).
//   - A C formatter returns the advanced accumulator pointer. We return the
//     byte delta by which the buffer advanced; the per-pixel loop reslices
//     buf = buf[delta:]. For planar formatters the delta is one element (as in
//     C, which returns Init + sizeof(element)); the plane jumps use Stride.
//   - Buffer reads/writes go through bounds-checked helpers (rdU8/rdU16/... and
//     wrU8/wrU16/...) so a malformed or too-small buffer can never panic. C
//     assumes caller-sized buffers (undefined behaviour otherwise); when the
//     buffer is adequately sized (as W12 guarantees) the helpers are exact.
//   - UnrollHalfTo16 divides Stride by PixelSize(OutputFormat) — a known quirk
//     of the reference (an input formatter reading the output format). Ported
//     verbatim; harness sets both formats equal so parity holds.
//   - PackLabDoubleFrom16 (planar) writes at element index Stride (byte offset
//     Stride*8), not Stride/PixelSize like the XYZ variant. Ported verbatim.

import (
	"encoding/binary"
	"math"
)

// maxEncodeableXYZforPack mirrors MAX_ENCODEABLE_XYZ (lcms2_internal.h). pcs.go
// keeps its own unexported copy (maxEncodeableXYZ); we reference that.

// ---------------------------------------------------------------------------
// Pixel-format bitfield macros (include/lcms2.h).
// ---------------------------------------------------------------------------

// The *_SH builders shift a field into place. They are used to assemble the
// TYPE_* codes and dispatch-table entries at package-init time (Go const
// expressions cannot call functions, but package-level var initialisers can).
func premulSH(m uint32) uint32     { return m << 23 }
func floatSH(a uint32) uint32      { return a << 22 }
func optimizedSH(s uint32) uint32  { return s << 21 }
func colorspaceSH(s uint32) uint32 { return s << 16 }
func swapfirstSH(s uint32) uint32  { return s << 14 }
func flavorSH(s uint32) uint32     { return s << 13 }
func planarSH(p uint32) uint32     { return p << 12 }
func endian16SH(e uint32) uint32   { return e << 11 }
func doswapSH(e uint32) uint32     { return e << 10 }
func extraSH(e uint32) uint32      { return e << 7 }
func channelsSH(c uint32) uint32   { return c << 3 }
func bytesSH(b uint32) uint32      { return b }

// The T_* accessors extract a field. Kept unexported (mirroring the macros).
func tPremul(v uint32) uint32     { return (v >> 23) & 1 }
func tFloat(v uint32) uint32      { return (v >> 22) & 1 }
func tOptimized(v uint32) uint32  { return (v >> 21) & 1 }
func tColorspace(v uint32) uint32 { return (v >> 16) & 31 }
func tSwapFirst(v uint32) uint32  { return (v >> 14) & 1 }
func tFlavor(v uint32) uint32     { return (v >> 13) & 1 }
func tPlanar(v uint32) uint32     { return (v >> 12) & 1 }
func tEndian16(v uint32) uint32   { return (v >> 11) & 1 }
func tDoSwap(v uint32) uint32     { return (v >> 10) & 1 }
func tExtra(v uint32) uint32      { return (v >> 7) & 7 }
func tChannels(v uint32) uint32   { return (v >> 3) & 15 }
func tBytes(v uint32) uint32      { return v & 7 }

// ANY* masks: bits set to one are *not* compared during table lookup.
var (
	anySpace     = colorspaceSH(31)
	anyChannels  = channelsSH(15)
	anyExtra     = extraSH(7)
	anyPlanar    = planarSH(1)
	anyEndian    = endian16SH(1)
	anySwap      = doswapSH(1)
	anySwapFirst = swapfirstSH(1)
	anyFlavor    = flavorSH(1)
	anyPremul    = premulSH(1)
)

// ---------------------------------------------------------------------------
// CMS_PACK_FLAGS (include/lcms2_plugin.h) and formatter direction.
// ---------------------------------------------------------------------------

// Packing precision flags for the formatter lookup.
const (
	PackFlags16Bits uint32 = 0x0000 // CMS_PACK_FLAGS_16BITS
	PackFlagsFloat  uint32 = 0x0001 // CMS_PACK_FLAGS_FLOAT
)

// FormatterDirection mirrors cmsFormatterDirection.
type FormatterDirection int

// Formatter directions.
const (
	FormatterInput  FormatterDirection = 0 // cmsFormatterInput
	FormatterOutput FormatterDirection = 1 // cmsFormatterOutput
)

// ---------------------------------------------------------------------------
// Formatter function types and the union struct.
// ---------------------------------------------------------------------------

// FormatterInfo carries the two transform fields the pixel formatters consult.
// In the C reference the formatter receives the whole _cmsTRANSFORM; cmspack.c
// reads only InputFormat and OutputFormat from it. W12 (xform.go) will populate
// one from its Transform. buf is the slice starting at the current pixel; a
// formatter returns the number of bytes by which the accumulator advanced (the
// per-pixel loop then reslices buf = buf[delta:]).
type FormatterInfo struct {
	InputFormat  uint32
	OutputFormat uint32
}

// Formatter16 unpacks (input) or packs (output) one pixel using the 16-bit
// working array. It mirrors cmsFormatter16.
//
// The values slice is the per-pixel working buffer and must have at least
// [MaxChannels] elements: some formatters write more slots than the format has
// channels (for example the grayscale unpacker replicates its single sample
// across three), so sizing values by channel count is not sufficient and would
// index out of range. The transform engine always supplies a full-width buffer;
// callers using GetFormatter directly are responsible for the same.
type Formatter16 func(info *FormatterInfo, values []uint16, buf []byte, stride int) int

// FormatterFloat is the float32 analogue, mirroring cmsFormatterFloat. The same
// working-buffer contract as [Formatter16] applies: values must have at least
// [MaxChannels] elements.
type FormatterFloat func(info *FormatterInfo, values []float32, buf []byte, stride int) int

// Formatter is the tagged analogue of the cmsFormatter union: exactly one of
// Fmt16 / FmtFloat is non-nil (or both nil when no formatter matched).
type Formatter struct {
	Fmt16    Formatter16
	FmtFloat FormatterFloat
}

// FormatterFactory mirrors cmsFormatterFactory: a plug-in supplied function
// that returns a Formatter for a given format, direction and flags, or a zero
// Formatter to decline (so the next factory / the stock table is consulted).
type FormatterFactory func(typ uint32, dir FormatterDirection, flags uint32) Formatter

// PluginFormatters mirrors cmsPluginFormatters: registers one formatter
// factory. A pointer to it satisfies Plugin.
type PluginFormatters struct {
	PluginBase
	FormattersFactory FormatterFactory
}

// NewPluginFormatters builds a formatters plug-in with the standard header.
func NewPluginFormatters(factory FormatterFactory) *PluginFormatters {
	return &PluginFormatters{
		PluginBase: PluginBase{
			Magic:           pluginMagicNumber,
			ExpectedVersion: Version,
			Type:            pluginFormattersSig,
		},
		FormattersFactory: factory,
	}
}

// ---------------------------------------------------------------------------
// Bounds-checked buffer accessors (little-endian, matching the oracle host).
// ---------------------------------------------------------------------------

func rdU8(b []byte, i int) uint8 {
	if i < 0 || i >= len(b) {
		return 0
	}
	return b[i]
}

func wrU8(b []byte, i int, v uint8) {
	if i >= 0 && i < len(b) {
		b[i] = v
	}
}

// The guards below are written as len(b)-i < n rather than i+n > len(b): with a
// hostile stride an accumulated offset can approach math.MaxInt, and i+n would
// wrap negative and slip past the bound. Because i is already known non-negative
// and len(b) is non-negative, len(b)-i never overflows.
func rdU16(b []byte, i int) uint16 {
	if i < 0 || len(b)-i < 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(b[i:])
}

func wrU16(b []byte, i int, v uint16) {
	if i >= 0 && len(b)-i >= 2 {
		binary.LittleEndian.PutUint16(b[i:], v)
	}
}

func rdF32(b []byte, i int) float32 {
	if i < 0 || len(b)-i < 4 {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
}

func wrF32(b []byte, i int, v float32) {
	if i >= 0 && len(b)-i >= 4 {
		binary.LittleEndian.PutUint32(b[i:], math.Float32bits(v))
	}
}

func rdF64(b []byte, i int) float64 {
	if i < 0 || len(b)-i < 8 {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(b[i:]))
}

func wrF64(b []byte, i int, v float64) {
	if i >= 0 && len(b)-i >= 8 {
		binary.LittleEndian.PutUint64(b[i:], math.Float64bits(v))
	}
}

// changeEndian ports CHANGE_ENDIAN(w): byte-swaps a 16-bit word.
func changeEndian(w uint16) uint16 { return w<<8 | w>>8 }

// reverseFlavor8 ports REVERSE_FLAVOR_8(x).
func reverseFlavor8(x uint8) uint8 { return 0xff - x }

// reverseFlavor16 ports REVERSE_FLAVOR_16(x).
func reverseFlavor16(x uint16) uint16 { return 0xffff - x }

// fomLabV2ToLabV4 ports FomLabV2ToLabV4: * 257 / 256, clamped to 0xffff.
func fomLabV2ToLabV4(x uint16) uint16 {
	a := (int(x)<<8 | int(x)) >> 8
	if a > 0xffff {
		return 0xffff
	}
	return uint16(a)
}

// fomLabV4ToLabV2 ports FomLabV4ToLabV2: * 256 / 257.
func fomLabV4ToLabV2(x uint16) uint16 {
	return uint16(((int(x) << 8) + 0x80) / 257)
}

// isInkSpace ports IsInkSpace: whether the colorspace samples are inks
// (percentage-scaled) rather than 0..1.
func isInkSpace(typ uint32) bool {
	switch tColorspace(typ) {
	case PTCMY, PTCMYK,
		PTMCH5, PTMCH6, PTMCH7, PTMCH8, PTMCH9,
		PTMCH10, PTMCH11, PTMCH12, PTMCH13, PTMCH14, PTMCH15:
		return true
	default:
		return false
	}
}

// pixelSize ports PixelSize: the size in bytes of one component of the format.
// For double (T_BYTES == 0) it is 8.
func pixelSize(format uint32) int {
	fmtBytes := tBytes(format)
	if fmtBytes == 0 {
		return 8
	}
	return int(fmtBytes)
}

// ---------------------------------------------------------------------------
// TYPE_* codes referenced by the dispatch tables.
// ---------------------------------------------------------------------------

var (
	typeLabDBL  = floatSH(1) | colorspaceSH(PTLab) | channelsSH(3) | bytesSH(0)
	typeXYZDBL  = floatSH(1) | colorspaceSH(PTXYZ) | channelsSH(3) | bytesSH(0)
	typeGrayDBL = floatSH(1) | colorspaceSH(PTGray) | channelsSH(1) | bytesSH(0)
	typeLabFLT  = floatSH(1) | colorspaceSH(PTLab) | channelsSH(3) | bytesSH(4)
	typeXYZFLT  = floatSH(1) | colorspaceSH(PTXYZ) | channelsSH(3) | bytesSH(4)
	typeLabV2_8 = colorspaceSH(PTLabV2) | channelsSH(3) | bytesSH(1)
	typeALabV2_8 = colorspaceSH(PTLabV2) | channelsSH(3) | bytesSH(1) |
		extraSH(1) | swapfirstSH(1)
	typeLabV2_16 = colorspaceSH(PTLabV2) | channelsSH(3) | bytesSH(2)
)

// ---------------------------------------------------------------------------
// Dispatch tables.
// ---------------------------------------------------------------------------

type formatters16Entry struct {
	Type uint32
	Mask uint32
	Frm  Formatter16
}

type formattersFloatEntry struct {
	Type uint32
	Mask uint32
	Frm  FormatterFloat
}

var inputFormatters16 []formatters16Entry
var inputFormattersFloat []formattersFloatEntry
var outputFormatters16 []formatters16Entry
var outputFormattersFloat []formattersFloatEntry

func init() {
	inputFormatters16 = []formatters16Entry{
		{typeLabDBL, anyPlanar | anyExtra, unrollLabDoubleTo16},
		{typeXYZDBL, anyPlanar | anyExtra, unrollXYZDoubleTo16},
		{typeLabFLT, anyPlanar | anyExtra, unrollLabFloatTo16},
		{typeXYZFLT, anyPlanar | anyExtra, unrollXYZFloatTo16},
		{typeGrayDBL, 0, unrollDouble1Chan},
		{floatSH(1) | bytesSH(0), anyChannels | anyPlanar | anySwapFirst | anyFlavor |
			anySwap | anyExtra | anySpace, unrollDoubleTo16},
		{floatSH(1) | bytesSH(4), anyChannels | anyPlanar | anySwapFirst | anyFlavor |
			anySwap | anyExtra | anySpace, unrollFloatTo16},
		{floatSH(1) | bytesSH(2), anyChannels | anyPlanar | anySwapFirst | anyFlavor |
			anyExtra | anySwap | anySpace, unrollHalfTo16},

		{channelsSH(1) | bytesSH(1), anySpace, unroll1Byte},
		{channelsSH(1) | bytesSH(1) | extraSH(1), anySpace, unroll1ByteSkip1},
		{channelsSH(1) | bytesSH(1) | extraSH(2), anySpace, unroll1ByteSkip2},
		{channelsSH(1) | bytesSH(1) | flavorSH(1), anySpace, unroll1ByteReversed},
		{colorspaceSH(PTMCH2) | channelsSH(2) | bytesSH(1), 0, unroll2Bytes},

		{typeLabV2_8, 0, unrollLabV2_8},
		{typeALabV2_8, 0, unrollALabV2_8},
		{typeLabV2_16, 0, unrollLabV2_16},

		{channelsSH(3) | bytesSH(1), anySpace, unroll3Bytes},
		{channelsSH(3) | bytesSH(1) | doswapSH(1), anySpace, unroll3BytesSwap},
		{channelsSH(3) | extraSH(1) | bytesSH(1) | doswapSH(1), anySpace, unroll3BytesSkip1Swap},
		{channelsSH(3) | extraSH(1) | bytesSH(1) | swapfirstSH(1), anySpace, unroll3BytesSkip1SwapFirst},
		{channelsSH(3) | extraSH(1) | bytesSH(1) | doswapSH(1) | swapfirstSH(1), anySpace, unroll3BytesSkip1SwapSwapFirst},

		{channelsSH(4) | bytesSH(1), anySpace, unroll4Bytes},
		{channelsSH(4) | bytesSH(1) | flavorSH(1), anySpace, unroll4BytesReverse},
		{channelsSH(4) | bytesSH(1) | swapfirstSH(1), anySpace, unroll4BytesSwapFirst},
		{channelsSH(4) | bytesSH(1) | doswapSH(1), anySpace, unroll4BytesSwap},
		{channelsSH(4) | bytesSH(1) | doswapSH(1) | swapfirstSH(1), anySpace, unroll4BytesSwapSwapFirst},

		{bytesSH(1) | planarSH(1), anyFlavor | anySwapFirst | anyPremul |
			anySwap | anyExtra | anyChannels | anySpace, unrollPlanarBytes},
		{bytesSH(1), anyFlavor | anySwapFirst | anySwap | anyPremul |
			anyExtra | anyChannels | anySpace, unrollChunkyBytes},

		{channelsSH(1) | bytesSH(2), anySpace, unroll1Word},
		{channelsSH(1) | bytesSH(2) | flavorSH(1), anySpace, unroll1WordReversed},
		{channelsSH(1) | bytesSH(2) | extraSH(3), anySpace, unroll1WordSkip3},

		{channelsSH(2) | bytesSH(2), anySpace, unroll2Words},
		{channelsSH(3) | bytesSH(2), anySpace, unroll3Words},
		{channelsSH(4) | bytesSH(2), anySpace, unroll4Words},

		{channelsSH(3) | bytesSH(2) | doswapSH(1), anySpace, unroll3WordsSwap},
		{channelsSH(3) | bytesSH(2) | extraSH(1) | swapfirstSH(1), anySpace, unroll3WordsSkip1SwapFirst},
		{channelsSH(3) | bytesSH(2) | extraSH(1) | doswapSH(1), anySpace, unroll3WordsSkip1Swap},
		{channelsSH(4) | bytesSH(2) | flavorSH(1), anySpace, unroll4WordsReverse},
		{channelsSH(4) | bytesSH(2) | swapfirstSH(1), anySpace, unroll4WordsSwapFirst},
		{channelsSH(4) | bytesSH(2) | doswapSH(1), anySpace, unroll4WordsSwap},
		{channelsSH(4) | bytesSH(2) | doswapSH(1) | swapfirstSH(1), anySpace, unroll4WordsSwapSwapFirst},

		{bytesSH(2) | planarSH(1), anyFlavor | anySwap | anyEndian | anyExtra | anyChannels | anySpace, unrollPlanarWords},
		{bytesSH(2), anyFlavor | anySwapFirst | anySwap | anyEndian | anyExtra | anyChannels | anySpace, unrollAnyWords},

		{bytesSH(2) | planarSH(1), anyFlavor | anySwap | anyEndian | anyExtra | anyChannels | anySpace | premulSH(1), unrollPlanarWordsPremul},
		{bytesSH(2), anyFlavor | anySwapFirst | anySwap | anyEndian | anyExtra | anyChannels | anySpace | premulSH(1), unrollAnyWordsPremul},
	}

	inputFormattersFloat = []formattersFloatEntry{
		{typeLabDBL, anyPlanar | anyExtra, unrollLabDoubleToFloat},
		{typeLabFLT, anyPlanar | anyExtra, unrollLabFloatToFloat},
		{typeXYZDBL, anyPlanar | anyExtra, unrollXYZDoubleToFloat},
		{typeXYZFLT, anyPlanar | anyExtra, unrollXYZFloatToFloat},

		{floatSH(1) | bytesSH(4), anyPlanar | anySwapFirst | anySwap | anyExtra |
			anyPremul | anyChannels | anySpace, unrollFloatsToFloat},
		{floatSH(1) | bytesSH(0), anyPlanar | anySwapFirst | anySwap | anyExtra |
			anyChannels | anySpace | anyPremul, unrollDoublesToFloat},

		{typeLabV2_8, 0, unrollLabV2_8ToFloat},
		{typeALabV2_8, 0, unrollALabV2_8ToFloat},
		{typeLabV2_16, 0, unrollLabV2_16ToFloat},

		{bytesSH(1), anyPlanar | anySwapFirst | anySwap | anyExtra | anyChannels | anySpace, unroll8ToFloat},
		{bytesSH(2), anyPlanar | anySwapFirst | anySwap | anyExtra | anyChannels | anySpace, unroll16ToFloat},

		{floatSH(1) | bytesSH(2), anyPlanar | anySwapFirst | anySwap | anyExtra | anyChannels | anySpace, unrollHalfToFloat},
	}

	outputFormatters16 = []formatters16Entry{
		{typeLabDBL, anyPlanar | anyExtra, packLabDoubleFrom16},
		{typeXYZDBL, anyPlanar | anyExtra, packXYZDoubleFrom16},
		{typeLabFLT, anyPlanar | anyExtra, packLabFloatFrom16},
		{typeXYZFLT, anyPlanar | anyExtra, packXYZFloatFrom16},

		{floatSH(1) | bytesSH(0), anyFlavor | anySwapFirst | anySwap |
			anyChannels | anyPlanar | anyExtra | anySpace, packDoubleFrom16},
		{floatSH(1) | bytesSH(4), anyFlavor | anySwapFirst | anySwap |
			anyChannels | anyPlanar | anyExtra | anySpace, packFloatFrom16},
		{floatSH(1) | bytesSH(2), anyFlavor | anySwapFirst | anySwap |
			anyChannels | anyPlanar | anyExtra | anySpace, packHalfFrom16},

		{channelsSH(1) | bytesSH(1), anySpace, pack1Byte},
		{channelsSH(1) | bytesSH(1) | extraSH(1), anySpace, pack1ByteSkip1},
		{channelsSH(1) | bytesSH(1) | extraSH(1) | swapfirstSH(1), anySpace, pack1ByteSkip1SwapFirst},
		{channelsSH(1) | bytesSH(1) | flavorSH(1), anySpace, pack1ByteReversed},

		{typeLabV2_8, 0, packLabV2_8},
		{typeALabV2_8, 0, packALabV2_8},
		{typeLabV2_16, 0, packLabV2_16},

		{channelsSH(3) | bytesSH(1) | optimizedSH(1), anySpace, pack3BytesOptimized},
		{channelsSH(3) | bytesSH(1) | extraSH(1) | optimizedSH(1), anySpace, pack3BytesAndSkip1Optimized},
		{channelsSH(3) | bytesSH(1) | extraSH(1) | swapfirstSH(1) | optimizedSH(1), anySpace, pack3BytesAndSkip1SwapFirstOptimized},
		{channelsSH(3) | bytesSH(1) | extraSH(1) | doswapSH(1) | swapfirstSH(1) | optimizedSH(1), anySpace, pack3BytesAndSkip1SwapSwapFirstOptimized},
		{channelsSH(3) | bytesSH(1) | doswapSH(1) | extraSH(1) | optimizedSH(1), anySpace, pack3BytesAndSkip1SwapOptimized},
		{channelsSH(3) | bytesSH(1) | doswapSH(1) | optimizedSH(1), anySpace, pack3BytesSwapOptimized},

		{channelsSH(3) | bytesSH(1), anySpace, pack3Bytes},
		{channelsSH(3) | bytesSH(1) | extraSH(1), anySpace, pack3BytesAndSkip1},
		{channelsSH(3) | bytesSH(1) | extraSH(1) | swapfirstSH(1), anySpace, pack3BytesAndSkip1SwapFirst},
		{channelsSH(3) | bytesSH(1) | extraSH(1) | doswapSH(1) | swapfirstSH(1), anySpace, pack3BytesAndSkip1SwapSwapFirst},
		{channelsSH(3) | bytesSH(1) | doswapSH(1) | extraSH(1), anySpace, pack3BytesAndSkip1Swap},
		{channelsSH(3) | bytesSH(1) | doswapSH(1), anySpace, pack3BytesSwap},
		{channelsSH(4) | bytesSH(1), anySpace, pack4Bytes},
		{channelsSH(4) | bytesSH(1) | flavorSH(1), anySpace, pack4BytesReverse},
		{channelsSH(4) | bytesSH(1) | swapfirstSH(1), anySpace, pack4BytesSwapFirst},
		{channelsSH(4) | bytesSH(1) | doswapSH(1), anySpace, pack4BytesSwap},
		{channelsSH(4) | bytesSH(1) | doswapSH(1) | swapfirstSH(1), anySpace, pack4BytesSwapSwapFirst},
		{channelsSH(6) | bytesSH(1), anySpace, pack6Bytes},
		{channelsSH(6) | bytesSH(1) | doswapSH(1), anySpace, pack6BytesSwap},

		{bytesSH(1), anyFlavor | anySwapFirst | anySwap | anyExtra | anyChannels |
			anySpace | anyPremul, packChunkyBytes},
		{bytesSH(1) | planarSH(1), anyFlavor | anySwapFirst | anySwap | anyExtra |
			anyChannels | anySpace | anyPremul, packPlanarBytes},

		{channelsSH(1) | bytesSH(2), anySpace, pack1Word},
		{channelsSH(1) | bytesSH(2) | extraSH(1), anySpace, pack1WordSkip1},
		{channelsSH(1) | bytesSH(2) | extraSH(1) | swapfirstSH(1), anySpace, pack1WordSkip1SwapFirst},
		{channelsSH(1) | bytesSH(2) | flavorSH(1), anySpace, pack1WordReversed},
		{channelsSH(1) | bytesSH(2) | endian16SH(1), anySpace, pack1WordBigEndian},
		{channelsSH(3) | bytesSH(2), anySpace, pack3Words},
		{channelsSH(3) | bytesSH(2) | doswapSH(1), anySpace, pack3WordsSwap},
		{channelsSH(3) | bytesSH(2) | endian16SH(1), anySpace, pack3WordsBigEndian},
		{channelsSH(3) | bytesSH(2) | extraSH(1), anySpace, pack3WordsAndSkip1},
		{channelsSH(3) | bytesSH(2) | extraSH(1) | doswapSH(1), anySpace, pack3WordsAndSkip1Swap},
		{channelsSH(3) | bytesSH(2) | extraSH(1) | swapfirstSH(1), anySpace, pack3WordsAndSkip1SwapFirst},
		{channelsSH(3) | bytesSH(2) | extraSH(1) | doswapSH(1) | swapfirstSH(1), anySpace, pack3WordsAndSkip1SwapSwapFirst},

		{channelsSH(4) | bytesSH(2), anySpace, pack4Words},
		{channelsSH(4) | bytesSH(2) | flavorSH(1), anySpace, pack4WordsReverse},
		{channelsSH(4) | bytesSH(2) | doswapSH(1), anySpace, pack4WordsSwap},
		{channelsSH(4) | bytesSH(2) | endian16SH(1), anySpace, pack4WordsBigEndian},

		{channelsSH(6) | bytesSH(2), anySpace, pack6Words},
		{channelsSH(6) | bytesSH(2) | doswapSH(1), anySpace, pack6WordsSwap},

		{bytesSH(2), anyFlavor | anySwapFirst | anySwap | anyEndian |
			anyExtra | anyChannels | anySpace | anyPremul, packChunkyWords},
		{bytesSH(2) | planarSH(1), anyFlavor | anyEndian | anySwap | anyExtra |
			anyChannels | anySpace | anyPremul, packPlanarWords},
	}

	outputFormattersFloat = []formattersFloatEntry{
		{typeLabFLT, anyPlanar | anyExtra, packLabFloatFromFloat},
		{typeXYZFLT, anyPlanar | anyExtra, packXYZFloatFromFloat},
		{typeLabDBL, anyPlanar | anyExtra, packLabDoubleFromFloat},
		{typeXYZDBL, anyPlanar | anyExtra, packXYZDoubleFromFloat},
		{typeLabV2_8, anyPlanar | anyExtra, packEncodedBytesLabV2FromFloat},
		{typeLabV2_16, anyPlanar | anyExtra, packEncodedWordsLabV2FromFloat},

		{floatSH(1) | bytesSH(4), anyPlanar | anyFlavor | anySwapFirst | anySwap |
			anyExtra | anyChannels | anySpace, packFloatsFromFloat},
		{floatSH(1) | bytesSH(0), anyPlanar | anyFlavor | anySwapFirst | anySwap |
			anyExtra | anyChannels | anySpace, packDoublesFromFloat},

		{bytesSH(2), anyPlanar | anyFlavor | anySwapFirst | anySwap |
			anyExtra | anyChannels | anySpace, packWordsFromFloat},
		{bytesSH(1), anyPlanar | anyFlavor | anySwapFirst | anySwap |
			anyExtra | anyChannels | anySpace, packBytesFromFloat},

		{floatSH(1) | bytesSH(2), anyFlavor | anySwapFirst | anySwap |
			anyExtra | anyChannels | anySpace, packHalfFromFloat},
	}
}

// ---------------------------------------------------------------------------
// Stock lookup (_cmsGetStockInputFormatter / _cmsGetStockOutputFormatter).
// ---------------------------------------------------------------------------

func getStockInputFormatter(dwInput, dwFlags uint32) Formatter {
	switch dwFlags {
	case PackFlags16Bits:
		for i := range inputFormatters16 {
			f := &inputFormatters16[i]
			if (dwInput & ^f.Mask) == f.Type {
				return Formatter{Fmt16: f.Frm}
			}
		}
	case PackFlagsFloat:
		for i := range inputFormattersFloat {
			f := &inputFormattersFloat[i]
			if (dwInput & ^f.Mask) == f.Type {
				return Formatter{FmtFloat: f.Frm}
			}
		}
	}
	return Formatter{}
}

func getStockOutputFormatter(dwInput, dwFlags uint32) Formatter {
	// Optimization is only a hint.
	dwInput &= ^optimizedSH(1)

	switch dwFlags {
	case PackFlags16Bits:
		for i := range outputFormatters16 {
			f := &outputFormatters16[i]
			if (dwInput & ^f.Mask) == f.Type {
				return Formatter{Fmt16: f.Frm}
			}
		}
	case PackFlagsFloat:
		for i := range outputFormattersFloat {
			f := &outputFormattersFloat[i]
			if (dwInput & ^f.Mask) == f.Type {
				return Formatter{FmtFloat: f.Frm}
			}
		}
	}
	return Formatter{}
}

// ---------------------------------------------------------------------------
// Plug-in registry integration and the public lookup entry point.
// ---------------------------------------------------------------------------

// formattersFactories returns the registered formatter factories, newest-first,
// mirroring the cmsFormattersFactoryList walk in _cmsGetFormatter.
func (ctx *Context) formattersFactories() []FormatterFactory {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if len(ctx.formatters.entries) == 0 {
		return nil
	}
	out := make([]FormatterFactory, 0, len(ctx.formatters.entries))
	for _, pl := range ctx.formatters.entries {
		if pf, ok := pl.(*PluginFormatters); ok && pf.FormattersFactory != nil {
			out = append(out, pf.FormattersFactory)
		}
	}
	return out
}

// GetFormatter ports _cmsGetFormatter. It consults the registered formatter
// factories (newest-first) and falls back to the stock tables. A format with
// zero channels yields the zero Formatter, exactly as in C.
//
// The returned Fmt16/FmtFloat must be called with a working-values slice of at
// least [MaxChannels] elements; see [Formatter16]. A shorter slice can be
// indexed out of range by formatters that expand fewer channels into more slots.
func (ctx *Context) GetFormatter(typ uint32, dir FormatterDirection, dwFlags uint32) Formatter {
	if tChannels(typ) == 0 {
		return Formatter{}
	}

	for _, factory := range ctx.formattersFactories() {
		fn := factory(typ, dir, dwFlags)
		if fn.Fmt16 != nil || fn.FmtFloat != nil {
			return fn
		}
	}

	if dir == FormatterInput {
		return getStockInputFormatter(typ, dwFlags)
	}
	return getStockOutputFormatter(typ, dwFlags)
}

// GetFormatter looks up a formatter in the default context (package-level
// convenience mirroring _cmsGetFormatter(NULL, ...)).
func GetFormatter(typ uint32, dir FormatterDirection, dwFlags uint32) Formatter {
	return defaultContext.GetFormatter(typ, dir, dwFlags)
}

// FormatterIsFloat ports _cmsFormatterIsFloat.
func FormatterIsFloat(typ uint32) bool { return tFloat(typ) != 0 }

// FormatterIs8bit ports _cmsFormatterIs8bit.
func FormatterIs8bit(typ uint32) bool { return tBytes(typ) == 1 }

// ---------------------------------------------------------------------------
// cmsFormatterForColorspaceOfProfile / cmsFormatterForPCSOfProfile.
// ---------------------------------------------------------------------------

// FormatterForColorspaceOfProfile ports cmsFormatterForColorspaceOfProfile:
// build a formatter code for a profile's device color space. Returns 0 for an
// unsupported color space.
func FormatterForColorspaceOfProfile(profile *Profile, nBytes uint32, isFloat bool) uint32 {
	colorSpace := profile.GetColorSpace()
	colorSpaceBits := uint32(LCMScolorSpace(colorSpace))
	nOutputChans := ChannelsOfColorSpace(colorSpace)
	var float uint32
	if isFloat {
		float = 1
	}

	if nOutputChans < 0 {
		return 0
	}

	nBytes &= 7
	return floatSH(float) | colorspaceSH(colorSpaceBits) | bytesSH(nBytes) | channelsSH(uint32(nOutputChans))
}

// FormatterForPCSOfProfile ports cmsFormatterForPCSOfProfile: build a formatter
// code for a profile's PCS. Returns 0 for an unsupported color space.
func FormatterForPCSOfProfile(profile *Profile, nBytes uint32, isFloat bool) uint32 {
	colorSpace := profile.GetPCS()
	colorSpaceBits := uint32(LCMScolorSpace(colorSpace))
	nOutputChans := ChannelsOfColorSpace(colorSpace)
	var float uint32
	if isFloat {
		float = 1
	}

	if nOutputChans < 0 {
		return 0
	}

	nBytes &= 7
	return floatSH(float) | colorspaceSH(colorSpaceBits) | bytesSH(nBytes) | channelsSH(uint32(nOutputChans))
}
