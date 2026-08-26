// Port of src/cmshalf.c — IEEE 754-2008 binary16 ("half") conversions.
//
// The reference implements the conversions as table lookups, following the
// paper "Fast Half Float Conversions" by Jeroen van der Zijp. The C source
// ships the five lookup tables as literals; here they are computed once at
// package initialization from the paper's generation formulas. The generated
// tables are byte-identical to the C literals (verified exhaustively by the
// half-float tests, which check every one of the 65536 half values against an
// independent binary16 decoder and round-trip float->half->float).

package lcms2

import "math"

// half-to-float lookup tables.
//
//	mantissaTable[2048]  cmsUInt32Number Mantissa[2048]
//	offsetTable[64]      cmsUInt16Number Offset[64]
//	exponentTable[64]    cmsUInt32Number Exponent[64]
//
// float-to-half lookup tables.
//
//	baseTable[512]       cmsUInt16Number Base[512]
//	shiftTable[512]      cmsUInt8Number  Shift[512]
var (
	mantissaTable [2048]uint32
	offsetTable   [64]uint16
	exponentTable [64]uint32
	baseTable     [512]uint16
	shiftTable    [512]uint8
)

func init() {
	buildHalfTables()
}

// convertMantissa reproduces the paper's convertmantissa(): given a 10-bit
// (or 11-bit, for the second half of the table) half mantissa index, it
// returns the float32 bit pattern for the corresponding subnormal/normal
// significand with the half exponent treated as zero.
func convertMantissa(i uint32) uint32 {
	m := i << 13 // zero-pad the mantissa bits into float32 position
	e := uint32(0)
	for m&0x00800000 == 0 { // while not normalized
		e -= 0x00800000 // decrement exponent (1<<23)
		m <<= 1         // shift mantissa
	}
	m &^= 0x00800000 // clear the leading (implicit) 1 bit
	e += 0x38800000  // adjust exponent bias ((127-14)<<23)
	return m | e
}

func buildHalfTables() {
	// --- Mantissa[2048] ---
	mantissaTable[0] = 0
	for i := uint32(1); i <= 1023; i++ {
		mantissaTable[i] = convertMantissa(i)
	}
	for i := uint32(1024); i < 2048; i++ {
		mantissaTable[i] = 0x38000000 + ((i - 1024) << 13)
	}

	// --- Exponent[64] ---
	exponentTable[0] = 0
	for i := uint32(1); i <= 30; i++ {
		exponentTable[i] = i << 23
	}
	exponentTable[31] = 0x47800000
	exponentTable[32] = 0x80000000
	for i := uint32(33); i <= 62; i++ {
		exponentTable[i] = 0x80000000 + ((i - 32) << 23)
	}
	exponentTable[63] = 0xC7800000

	// --- Offset[64] ---
	offsetTable[0] = 0
	for i := 1; i < 64; i++ {
		offsetTable[i] = 0x0400
	}
	offsetTable[32] = 0

	// --- Base[512] / Shift[512] ---
	for i := 0; i < 256; i++ {
		e := i - 127
		switch {
		case e < -24: // very small numbers map to zero
			baseTable[i|0x000] = 0x0000
			baseTable[i|0x100] = 0x8000
			shiftTable[i|0x000] = 24
			shiftTable[i|0x100] = 24
		case e < -14: // small numbers map to denorms
			baseTable[i|0x000] = 0x0400 >> uint(-e-14)
			baseTable[i|0x100] = (0x0400 >> uint(-e-14)) | 0x8000
			shiftTable[i|0x000] = uint8(-e - 1)
			shiftTable[i|0x100] = uint8(-e - 1)
		case e <= 15: // normal numbers just lose precision
			baseTable[i|0x000] = uint16((e + 15) << 10)
			baseTable[i|0x100] = uint16((e+15)<<10) | 0x8000
			shiftTable[i|0x000] = 13
			shiftTable[i|0x100] = 13
		case e < 128: // large numbers map to Infinity
			baseTable[i|0x000] = 0x7C00
			baseTable[i|0x100] = 0xFC00
			shiftTable[i|0x000] = 24
			shiftTable[i|0x100] = 24
		default: // Infinity and NaN stay Infinity and NaN
			baseTable[i|0x000] = 0x7C00
			baseTable[i|0x100] = 0xFC00
			shiftTable[i|0x000] = 13
			shiftTable[i|0x100] = 13
		}
	}
}

// half2Float converts an IEEE 754-2008 binary16 value (as raw bits) to its
// float32 equivalent. Port of _cmsHalf2Float.
//
// Index bounds are total: n is h>>10 in [0,63]; (h&0x3ff)+Offset[n] is at most
// 1023+1024 = 2047, so no lookup can go out of range.
func half2Float(h uint16) float32 {
	n := h >> 10
	bits := mantissaTable[(h&0x3ff)+offsetTable[n]] + exponentTable[n]
	return math.Float32frombits(bits)
}

// float2Half converts a float32 to its IEEE 754-2008 binary16 representation
// (as raw bits). Port of _cmsFloat2Half.
//
// j = (bits>>23)&0x1ff is in [0,511], so Base[j] and Shift[j] are always in
// range.
func float2Half(f float32) uint16 {
	n := math.Float32bits(f)
	j := (n >> 23) & 0x1ff
	return uint16(uint32(baseTable[j]) + ((n & 0x007fffff) >> shiftTable[j]))
}
