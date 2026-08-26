// Port of the fixed-point and small numeric helpers from src/lcms2_internal.h.
//
// These are hot-path primitives used throughout the pipeline. They are
// implemented to be allocation-free and to reproduce the exact integer/rounding
// semantics of the C macros and inline functions, including overflow/truncation
// behavior.
//
// Not ported here (they live in cmsplugin.c, owned by another worker): the
// double<->fixed converters _cmsDoubleTo15Fixed16, _cms15Fixed16toDouble,
// _cmsDoubleTo8Fixed8, _cms8Fixed8toDouble. This file deliberately does not
// define them.

package lcms2

import "math"

// fmul32 / fmul64 round a floating-point product before it is used as an operand
// of a following add or subtract. The explicit conversion is a rounding barrier
// that stops the compiler from contracting `a*b + c` (or `c - a*b`) into a single
// fused multiply-add on targets that support it (arm64, ppc64, riscv64, s390x).
// The C reference is compiled without FMA, so an un-barriered product would round
// once instead of twice and diverge in the last ULP — which, once quantized into
// a 16-bit table, flips table entries and breaks integer-path parity. A plain
// local temporary does NOT suffice (the compiler still fuses across it); only the
// conversion does, and it survives inlining. On amd64, where gc does not emit FMA
// for these patterns, both helpers are exact no-ops.
func fmul32(a, b float32) float32 { return float32(a * b) }
func fmul64(a, b float64) float64 { return float64(a * b) }

// s15Fixed16 mirrors cmsS15Fixed16Number: a signed 15.16 fixed-point number.
type s15Fixed16 = int32

// toFixedDomain ports _cmsToFixedDomain:
//
//	return a + ((a + 0x7fff) / 0xffff);
//
// C integer division truncates toward zero; Go's / on int does the same, so the
// behavior matches for both signs.
func toFixedDomain(a int) s15Fixed16 {
	return s15Fixed16(a + (a+0x7fff)/0xffff)
}

// fromFixedDomain ports _cmsFromFixedDomain:
//
//	return a - ((a + 0x7fff) >> 16);
//
// The shift is an arithmetic (sign-propagating) shift on the signed value, which
// Go's >> performs on a signed int32.
func fromFixedDomain(a s15Fixed16) int {
	return int(a) - int((a+0x7fff)>>16)
}

// double2FixMagic is _lcms_double2fixmagic: 2^36 * 1.5 == 2^36 + 2^35. Adding a
// value in the valid domain places its 15.16 fixed-point representation in the
// low 32 bits of the resulting IEEE-754 double.
const double2FixMagic = 68719476736.0 * 1.5

// quickFloor ports _cmsQuickFloor, the fast floor of Sree Kotay / Stuart Nixon.
//
// This is the default (non-CMS_DONT_USE_FAST_FLOOR) build of the reference,
// which is what the oracle uses, so this reproduces the oracle bit-for-bit. The
// trick is only valid for val in the range roughly -32768 .. +32767: the double
// magic constant forces the 15.16 fixed-point form of val into the low mantissa
// bits, and the low 32 bits interpreted as a signed int32, shifted right by 16,
// yield floor(val). Inside that domain the result equals math.Floor(val). Outside
// it the result wraps exactly as the C reference does (it is NOT plain floor) —
// all lcms2 callers stay within the valid domain.
func quickFloor(val float64) int {
	bits := math.Float64bits(val + double2FixMagic)
	// halves[0] on a little-endian target is the low 32 bits, taken as a
	// signed int32 and arithmetically shifted right by 16.
	return int(int32(uint32(bits)) >> 16)
}

// quickFloorWord ports _cmsQuickFloorWord:
//
//	return (cmsUInt16Number) _cmsQuickFloor(d - 32767.0) + 32767U;
//
// The cast to 16 bits binds to the quickFloor result before adding 32767; the
// whole expression is then truncated to 16 bits on return. uint16 arithmetic in
// Go wraps identically.
func quickFloorWord(d float64) uint16 {
	return uint16(quickFloor(d-32767.0)) + 32767
}

// quickSaturateWord ports _cmsQuickSaturateWord: floor to a word with saturation.
func quickSaturateWord(d float64) uint16 {
	d += 0.5
	if d <= 0 {
		return 0
	}
	if d >= 65535.0 {
		return 0xffff
	}
	return quickFloorWord(d)
}

// fixedToInt ports FIXED_TO_INT(x): ((x)>>16) — arithmetic shift of a 15.16
// fixed value to its integer part (floor for the sign).
func fixedToInt(x s15Fixed16) int32 { return x >> 16 }

// fixedRestToInt ports FIXED_REST_TO_INT(x): ((x)&0xFFFFU) — the fractional part.
func fixedRestToInt(x s15Fixed16) int32 { return x & 0xFFFF }

// roundFixedToInt ports ROUND_FIXED_TO_INT(x): (((x)+0x8000)>>16) — round to
// nearest, ties toward +inf.
func roundFixedToInt(x s15Fixed16) int32 { return (x + 0x8000) >> 16 }

// from8to16 ports FROM_8_TO_16(rgb): ((cmsUInt16Number)(rgb) << 8) | (rgb).
// Replicates an 8-bit sample to 16 bits (0xAB -> 0xABAB).
func from8to16(rgb uint8) uint16 {
	return uint16(rgb)<<8 | uint16(rgb)
}

// from16to8 ports FROM_16_TO_8(rgb):
//
//	(cmsUInt8Number)((((cmsUInt32Number)(rgb) * 65281U + 8388608U) >> 24) & 0xFFU)
//
// The exact rounding used by the reference to narrow 16 bits to 8.
func from16to8(rgb uint16) uint8 {
	return uint8(((uint32(rgb)*65281 + 8388608) >> 24) & 0xFF)
}

// s15Fixed16ToDouble ports _cms15Fixed16toDouble (cmsplugin.c). Its double->
// fixed counterparts stay with the cmsplugin.c IO helpers in Phase 3.
func s15Fixed16ToDouble(fix32 int32) float64 {
	return float64(fix32) / 65536.0
}

// ptrAlignment mirrors CMS_PTR_ALIGNMENT (sizeof(void*)): 8 on 64-bit targets,
// 4 on 32-bit. Evaluated at compile time.
const ptrAlignment = 4 << (^uintptr(0) >> 63)

// alignLong ports _cmsALIGNLONG(x): round x up to the next multiple of 4
// (cmsUInt32Number size), the ICC file-format alignment.
func alignLong(x uint32) uint32 {
	return (x + 3) &^ 3
}

// alignMem ports _cmsALIGNMEM(x): round x up to the next multiple of the pointer
// alignment.
func alignMem(x uint32) uint32 {
	const a = uint32(ptrAlignment)
	return (x + (a - 1)) &^ (a - 1)
}
