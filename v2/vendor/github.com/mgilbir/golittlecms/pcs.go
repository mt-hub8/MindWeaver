package lcms2

// PCS encodings and colorimetric conversions. Port of src/cmspcs.c:
// XYZ<->Lab, Lab<->LCh, xyY<->XYZ, the V2/V4 Lab encodings, the XYZ 1.15
// fixed-point encoding, and the deltaE family. Constants and operation order
// follow the reference digit-for-digit for numeric parity.

import "math"

// ---------------------------------------------------------------------------
// Colorimetric value types (from include/lcms2.h). Field layout matches C.
// ---------------------------------------------------------------------------

// CIEXYZ is a color in CIE XYZ tristimulus space (cmsCIEXYZ).
type CIEXYZ struct {
	X, Y, Z float64
}

// CIExyY is a color in CIE xyY space (cmsCIExyY).
type CIExyY struct {
	X, Y, YY float64 // C fields: x, y, Y
}

// CIELab is a color in CIE L*a*b* space (cmsCIELab).
type CIELab struct {
	L, A, B float64
}

// CIELCh is a color in CIE LCh cylindrical space (cmsCIELCh).
type CIELCh struct {
	L, C, H float64
}

// CIEXYZTRIPLE is a triple of XYZ colors, e.g. RGB primaries (cmsCIEXYZTRIPLE).
type CIEXYZTRIPLE struct {
	Red, Green, Blue CIEXYZ
}

// CIExyYTRIPLE is a triple of xyY colors, e.g. RGB primaries (cmsCIExyYTRIPLE).
type CIExyYTRIPLE struct {
	Red, Green, Blue CIExyY
}

// ---------------------------------------------------------------------------
// Encodeable-range constants (lcms2_internal.h).
// ---------------------------------------------------------------------------

const (
	maxEncodeableXYZ = 1.0 + 32767.0/32768.0     // MAX_ENCODEABLE_XYZ
	minEncodeableAb2 = -128.0                    // MIN_ENCODEABLE_ab2
	maxEncodeableAb2 = (65535.0 / 256.0) - 128.0 // MAX_ENCODEABLE_ab2
	minEncodeableAb4 = -128.0                    // MIN_ENCODEABLE_ab4
	maxEncodeableAb4 = 127.0                     // MAX_ENCODEABLE_ab4
)

// ---------------------------------------------------------------------------
// xyY <-> XYZ (cmsXYZ2xyY / cmsxyY2XYZ).
// ---------------------------------------------------------------------------

// XYZ2xyY converts XYZ to xyY (cmsXYZ2xyY).
func XYZ2xyY(src CIEXYZ) CIExyY {
	iSum := 1. / (src.X + src.Y + src.Z)
	return CIExyY{
		X:  src.X * iSum,
		Y:  src.Y * iSum,
		YY: src.Y,
	}
}

// XyY2XYZ converts xyY to XYZ (cmsxyY2XYZ).
func XyY2XYZ(src CIExyY) CIEXYZ {
	return CIEXYZ{
		X: (src.X / src.Y) * src.YY,
		Y: src.YY,
		Z: ((1 - src.X - src.Y) / src.Y) * src.YY,
	}
}

// ---------------------------------------------------------------------------
// XYZ <-> Lab (cmsXYZ2Lab / cmsLab2XYZ).
// ---------------------------------------------------------------------------

// f is the CIELab forward nonlinearity (static f in cmspcs.c).
func f(t float64) float64 {
	const limit = (24.0 / 116.0) * (24.0 / 116.0) * (24.0 / 116.0)
	if t <= limit {
		return fmul64(841.0/108.0, t) + (16.0 / 116.0)
	}
	return math.Pow(t, 1.0/3.0)
}

// f1 is the CIELab inverse nonlinearity (static f_1 in cmspcs.c).
func f1(t float64) float64 {
	const limit = 24.0 / 116.0
	if t <= limit {
		return (108.0 / 841.0) * (t - (16.0 / 116.0))
	}
	return t * t * t
}

// XYZ2Lab converts XYZ to Lab relative to whitePoint. A nil whitePoint means
// D50 (cmsXYZ2Lab). It can handle some negative XYZ inputs.
func XYZ2Lab(whitePoint *CIEXYZ, xyz CIEXYZ) CIELab {
	wp := d50XYZ()
	if whitePoint != nil {
		wp = *whitePoint
	}

	fx := f(xyz.X / wp.X)
	fy := f(xyz.Y / wp.Y)
	fz := f(xyz.Z / wp.Z)

	return CIELab{
		L: fmul64(116.0, fy) - 16.0,
		A: 500.0 * (fx - fy),
		B: 200.0 * (fy - fz),
	}
}

// Lab2XYZ converts Lab to XYZ relative to whitePoint. A nil whitePoint means
// D50 (cmsLab2XYZ). It can return some negative XYZ values.
func Lab2XYZ(whitePoint *CIEXYZ, lab CIELab) CIEXYZ {
	wp := d50XYZ()
	if whitePoint != nil {
		wp = *whitePoint
	}

	y := (lab.L + 16.0) / 116.0
	x := y + fmul64(0.002, lab.A)
	z := y - fmul64(0.005, lab.B)

	return CIEXYZ{
		X: f1(x) * wp.X,
		Y: f1(y) * wp.Y,
		Z: f1(z) * wp.Z,
	}
}

// ---------------------------------------------------------------------------
// Lab V2 encoding (cmsLabEncoded2FloatV2 / cmsFloat2LabEncodedV2).
// ---------------------------------------------------------------------------

func l2float2(v uint16) float64  { return float64(v) / 652.800 }
func ab2float2(v uint16) float64 { return (float64(v) / 256.0) - 128.0 }

func l2Fix2(l float64) uint16   { return quickSaturateWord(l * 652.8) }
func ab2Fix2(ab float64) uint16 { return quickSaturateWord((ab + 128.0) * 256.0) }

func l2float4(v uint16) float64  { return float64(v) / 655.35 }
func ab2float4(v uint16) float64 { return (float64(v) / 257.0) - 128.0 }

// LabEncoded2FloatV2 decodes an ICC V2 16-bit Lab encoding (cmsLabEncoded2FloatV2).
func LabEncoded2FloatV2(wLab [3]uint16) CIELab {
	return CIELab{
		L: l2float2(wLab[0]),
		A: ab2float2(wLab[1]),
		B: ab2float2(wLab[2]),
	}
}

// LabEncoded2Float decodes an ICC V4 16-bit Lab encoding (cmsLabEncoded2Float).
func LabEncoded2Float(wLab [3]uint16) CIELab {
	return CIELab{
		L: l2float4(wLab[0]),
		A: ab2float4(wLab[1]),
		B: ab2float4(wLab[2]),
	}
}

func clampLDoubleV2(l float64) float64 {
	const lMax = (0xFFFF * 100.0) / 0xFF00
	if l < 0 {
		l = 0
	}
	if l > lMax {
		l = lMax
	}
	return l
}

func clampAbDoubleV2(ab float64) float64 {
	if ab < minEncodeableAb2 {
		ab = minEncodeableAb2
	}
	if ab > maxEncodeableAb2 {
		ab = maxEncodeableAb2
	}
	return ab
}

// Float2LabEncodedV2 encodes Lab into an ICC V2 16-bit encoding
// (cmsFloat2LabEncodedV2).
func Float2LabEncodedV2(fLab CIELab) [3]uint16 {
	var lab CIELab
	lab.L = clampLDoubleV2(fLab.L)
	lab.A = clampAbDoubleV2(fLab.A)
	lab.B = clampAbDoubleV2(fLab.B)
	return [3]uint16{l2Fix2(lab.L), ab2Fix2(lab.A), ab2Fix2(lab.B)}
}

// ---------------------------------------------------------------------------
// Lab V4 encoding (cmsLabEncoded2Float / cmsFloat2LabEncoded).
// ---------------------------------------------------------------------------

func clampLDoubleV4(l float64) float64 {
	if l < 0 {
		l = 0
	}
	if l > 100.0 {
		l = 100.0
	}
	return l
}

func clampAbDoubleV4(ab float64) float64 {
	if ab < minEncodeableAb4 {
		ab = minEncodeableAb4
	}
	if ab > maxEncodeableAb4 {
		ab = maxEncodeableAb4
	}
	return ab
}

func l2Fix4(l float64) uint16   { return quickSaturateWord(l * 655.35) }
func ab2Fix4(ab float64) uint16 { return quickSaturateWord((ab + 128.0) * 257.0) }

// Float2LabEncoded encodes Lab into an ICC V4 16-bit encoding
// (cmsFloat2LabEncoded).
func Float2LabEncoded(fLab CIELab) [3]uint16 {
	var lab CIELab
	lab.L = clampLDoubleV4(fLab.L)
	lab.A = clampAbDoubleV4(fLab.A)
	lab.B = clampAbDoubleV4(fLab.B)
	return [3]uint16{l2Fix4(lab.L), ab2Fix4(lab.A), ab2Fix4(lab.B)}
}

// ---------------------------------------------------------------------------
// Lab <-> LCh (cmsLab2LCh / cmsLCh2Lab).
// ---------------------------------------------------------------------------

// radians converts degrees to radians (RADIANS in cmspcs.c).
func radians(deg float64) float64 {
	return (deg * math.Pi) / 180.
}

// atan2deg is atan2 in degrees, returning 0 when a==b==0 (atan2deg in cmspcs.c).
func atan2deg(a, b float64) float64 {
	var h float64
	if a == 0 && b == 0 {
		h = 0
	} else {
		h = math.Atan2(a, b)
	}

	h *= (180. / math.Pi)

	for h > 360. {
		h -= 360.
	}
	for h < 0 {
		h += 360.
	}
	return h
}

func sqr(v float64) float64 { return v * v }

// Lab2LCh converts Lab to LCh. No range check is performed, so negative values
// are allowed (cmsLab2LCh).
func Lab2LCh(lab CIELab) CIELCh {
	return CIELCh{
		L: lab.L,
		C: math.Pow(sqr(lab.A)+sqr(lab.B), 0.5),
		H: atan2deg(lab.B, lab.A),
	}
}

// LCh2Lab converts LCh to Lab. No range check is performed (cmsLCh2Lab).
func LCh2Lab(lch CIELCh) CIELab {
	h := (lch.H * math.Pi) / 180.0
	return CIELab{
		L: lch.L,
		A: lch.C * math.Cos(h),
		B: lch.C * math.Sin(h),
	}
}

// ---------------------------------------------------------------------------
// XYZ 1.15 fixed-point encoding (cmsFloat2XYZEncoded / cmsXYZEncoded2Float).
// ---------------------------------------------------------------------------

// xyz2Fix encodes one XYZ component using 1.15 fixed point (XYZ2Fix).
func xyz2Fix(d float64) uint16 {
	return quickSaturateWord(d * 32768.0)
}

// Float2XYZEncoded encodes XYZ into three 1.15 fixed-point words, clamping to
// the encodeable range (cmsFloat2XYZEncoded).
func Float2XYZEncoded(fXYZ CIEXYZ) [3]uint16 {
	xyz := fXYZ

	// Clamp to encodeable values.
	if xyz.Y <= 0 {
		xyz.X = 0
		xyz.Y = 0
		xyz.Z = 0
	}

	if xyz.X > maxEncodeableXYZ {
		xyz.X = maxEncodeableXYZ
	}
	if xyz.X < 0 {
		xyz.X = 0
	}

	if xyz.Y > maxEncodeableXYZ {
		xyz.Y = maxEncodeableXYZ
	}
	if xyz.Y < 0 {
		xyz.Y = 0
	}

	if xyz.Z > maxEncodeableXYZ {
		xyz.Z = maxEncodeableXYZ
	}
	if xyz.Z < 0 {
		xyz.Z = 0
	}

	return [3]uint16{xyz2Fix(xyz.X), xyz2Fix(xyz.Y), xyz2Fix(xyz.Z)}
}

// xyz2float converts a 1.15 fixed-point word to float64 (XYZ2float).
func xyz2float(v uint16) float64 {
	// From 1.15 to 15.16.
	fix32 := int32(v) << 1
	// From fixed 15.16 to float64.
	return s15Fixed16ToDouble(fix32)
}

// XYZEncoded2Float decodes three 1.15 fixed-point words into XYZ
// (cmsXYZEncoded2Float).
func XYZEncoded2Float(xyz [3]uint16) CIEXYZ {
	return CIEXYZ{
		X: xyz2float(xyz[0]),
		Y: xyz2float(xyz[1]),
		Z: xyz2float(xyz[2]),
	}
}

// ---------------------------------------------------------------------------
// deltaE family (cmsDeltaE, cmsCIE94DeltaE, cmsBFDdeltaE, cmsCMCdeltaE,
// cmsCIE2000DeltaE).
// ---------------------------------------------------------------------------

// DeltaE returns the CIE76 deltaE between two Lab values (cmsDeltaE).
func DeltaE(lab1, lab2 CIELab) float64 {
	dL := math.Abs(lab1.L - lab2.L)
	da := math.Abs(lab1.A - lab2.A)
	db := math.Abs(lab1.B - lab2.B)
	return math.Pow(sqr(dL)+sqr(da)+sqr(db), 0.5)
}

// CIE94DeltaE returns the CIE94 deltaE (cmsCIE94DeltaE).
func CIE94DeltaE(lab1, lab2 CIELab) float64 {
	dL := math.Abs(lab1.L - lab2.L)

	lch1 := Lab2LCh(lab1)
	lch2 := Lab2LCh(lab2)

	dC := math.Abs(lch1.C - lch2.C)
	dE := DeltaE(lab1, lab2)

	dhsq := sqr(dE) - sqr(dL) - sqr(dC)
	var dh float64
	if dhsq < 0 {
		dh = 0
	} else {
		dh = math.Pow(dhsq, 0.5)
	}

	c12 := math.Sqrt(lch1.C * lch2.C)

	sc := 1.0 + (0.048 * c12)
	sh := 1.0 + (0.014 * c12)

	return math.Sqrt(sqr(dL) + sqr(dC)/sqr(sc) + sqr(dh)/sqr(sh))
}

// computeLBFD is the auxiliary lightness term for the BFD metric (ComputeLBFD).
func computeLBFD(lab CIELab) float64 {
	var yt float64
	if lab.L > 7.996969 {
		yt = (sqr((lab.L+16)/116) * ((lab.L + 16) / 116)) * 100
	} else {
		yt = 100 * (lab.L / 903.3)
	}
	return 54.6*(math.Log10E*(math.Log(yt+1.5))) - 9.6
}

// BFDdeltaE returns the BFD(1:1) difference between two Lab values
// (cmsBFDdeltaE).
func BFDdeltaE(lab1, lab2 CIELab) float64 {
	lbfd1 := computeLBFD(lab1)
	lbfd2 := computeLBFD(lab2)
	deltaL := lbfd2 - lbfd1

	lch1 := Lab2LCh(lab1)
	lch2 := Lab2LCh(lab2)

	deltaC := lch2.C - lch1.C
	aveC := (lch1.C + lch2.C) / 2
	aveh := (lch1.H + lch2.H) / 2

	dE := DeltaE(lab1, lab2)

	var deltah float64
	if sqr(dE) > (sqr(lab2.L-lab1.L) + sqr(deltaC)) {
		deltah = math.Sqrt(sqr(dE) - sqr(lab2.L-lab1.L) - sqr(deltaC))
	} else {
		deltah = 0
	}

	dc := 0.035*aveC/(1+0.00365*aveC) + 0.521
	g := math.Sqrt(sqr(sqr(aveC)) / (sqr(sqr(aveC)) + 14000))
	t := 0.627 + (0.055*math.Cos((aveh-254)/(180/math.Pi)) -
		0.040*math.Cos((2*aveh-136)/(180/math.Pi)) +
		0.070*math.Cos((3*aveh-31)/(180/math.Pi)) +
		0.049*math.Cos((4*aveh+114)/(180/math.Pi)) -
		0.015*math.Cos((5*aveh-103)/(180/math.Pi)))

	dh := dc * (g*t + 1 - g)
	rh := -0.260*math.Cos((aveh-308)/(180/math.Pi)) -
		0.379*math.Cos((2*aveh-160)/(180/math.Pi)) -
		0.636*math.Cos((3*aveh+254)/(180/math.Pi)) +
		0.226*math.Cos((4*aveh+140)/(180/math.Pi)) -
		0.194*math.Cos((5*aveh+280)/(180/math.Pi))

	rc := math.Sqrt((aveC * aveC * aveC * aveC * aveC * aveC) / ((aveC * aveC * aveC * aveC * aveC * aveC) + 70000000))
	rt := rh * rc

	bfd := math.Sqrt(sqr(deltaL) + sqr(deltaC/dc) + sqr(deltah/dh) + (rt * (deltaC / dc) * (deltah / dh)))

	return bfd
}

// CMCdeltaE returns the CMC(l:c) difference between two Lab values
// (cmsCMCdeltaE).
func CMCdeltaE(lab1, lab2 CIELab, l, c float64) float64 {
	if lab1.L == 0 && lab2.L == 0 {
		return 0
	}

	lch1 := Lab2LCh(lab1)
	lch2 := Lab2LCh(lab2)

	dL := lab2.L - lab1.L
	dC := lch2.C - lch1.C

	dE := DeltaE(lab1, lab2)

	var dh float64
	if sqr(dE) > (sqr(dL) + sqr(dC)) {
		dh = math.Sqrt(sqr(dE) - sqr(dL) - sqr(dC))
	} else {
		dh = 0
	}

	var t float64
	if (lch1.H > 164) && (lch1.H < 345) {
		t = 0.56 + math.Abs(0.2*math.Cos((lch1.H+168)/(180/math.Pi)))
	} else {
		t = 0.36 + math.Abs(0.4*math.Cos((lch1.H+35)/(180/math.Pi)))
	}

	sc := 0.0638*lch1.C/(1+0.0131*lch1.C) + 0.638
	sl := 0.040975 * lab1.L / (1 + 0.01765*lab1.L)

	if lab1.L < 16 {
		sl = 0.511
	}

	fv := math.Sqrt((lch1.C * lch1.C * lch1.C * lch1.C) / ((lch1.C * lch1.C * lch1.C * lch1.C) + 1900))
	sh := sc * (t*fv + 1 - fv)
	cmc := math.Sqrt(sqr(dL/(l*sl)) + sqr(dC/(c*sc)) + sqr(dh/sh))

	return cmc
}

// CIE2000DeltaE returns the CIEDE2000 difference between two Lab values. The
// weightings Kl, Kc and Kh tune the relative importance of lightness, chroma
// and hue (cmsCIE2000DeltaE).
func CIE2000DeltaE(lab1, lab2 CIELab, kl, kc, kh float64) float64 {
	l1 := lab1.L
	a1 := lab1.A
	b1 := lab1.B
	c := math.Sqrt(sqr(a1) + sqr(b1))

	ls := lab2.L
	as := lab2.A
	bs := lab2.B
	cs := math.Sqrt(sqr(as) + sqr(bs))

	g := 0.5 * (1 - math.Sqrt(math.Pow((c+cs)/2, 7.0)/(math.Pow((c+cs)/2, 7.0)+math.Pow(25.0, 7.0))))

	aP := (1 + g) * a1
	bP := b1
	cP := math.Sqrt(sqr(aP) + sqr(bP))
	hP := atan2deg(bP, aP)

	aPs := (1 + g) * as
	bPs := bs
	cPs := math.Sqrt(sqr(aPs) + sqr(bPs))
	hPs := atan2deg(bPs, aPs)

	meanCp := (cP + cPs) / 2

	hpsPlusHp := hPs + hP
	hpsMinusHp := hPs - hP

	var meanhP float64
	switch {
	case math.Abs(hpsMinusHp) <= 180.000001:
		meanhP = hpsPlusHp / 2
	case hpsPlusHp < 360:
		meanhP = (hpsPlusHp + 360) / 2
	default:
		meanhP = (hpsPlusHp - 360) / 2
	}

	var deltaHsmall float64
	switch {
	case hpsMinusHp <= -180.000001:
		deltaHsmall = hpsMinusHp + 360
	case hpsMinusHp > 180:
		deltaHsmall = hpsMinusHp - 360
	default:
		deltaHsmall = hpsMinusHp
	}
	deltaL := ls - l1
	deltaC := cPs - cP

	deltaH := 2 * math.Sqrt(cPs*cP) * math.Sin(radians(deltaHsmall)/2)

	t := 1 - 0.17*math.Cos(radians(meanhP-30)) +
		0.24*math.Cos(radians(2*meanhP)) +
		0.32*math.Cos(radians(3*meanhP+6)) -
		0.2*math.Cos(radians(4*meanhP-63))

	sl := 1 + (0.015*sqr((ls+l1)/2-50))/math.Sqrt(20+sqr((ls+l1)/2-50))

	sc := 1 + 0.045*(cP+cPs)/2
	sh := 1 + 0.015*((cPs+cP)/2)*t

	deltaRo := 30 * math.Exp(-sqr((meanhP-275)/25))

	rc := 2 * math.Sqrt(math.Pow(meanCp, 7.0)/(math.Pow(meanCp, 7.0)+math.Pow(25.0, 7.0)))

	rt := -math.Sin(2*radians(deltaRo)) * rc

	deltaE00 := math.Sqrt(sqr(deltaL/(sl*kl)) +
		sqr(deltaC/(sc*kc)) +
		sqr(deltaH/(sh*kh)) +
		rt*(deltaC/(sc*kc))*(deltaH/(sh*kh)))

	return deltaE00
}

// ---------------------------------------------------------------------------
// Grid-point sizing and end points (helpers used by LUT building later).
// ---------------------------------------------------------------------------

// ReasonableGridpointsByColorspace returns the number of CLUT grid points to
// use for a color space, honoring precalc flags (_cmsReasonableGridpointsByColorspace).
func ReasonableGridpointsByColorspace(colorspace ColorSpaceSignature, dwFlags uint32) uint32 {
	// Already specified?
	if dwFlags&0x00FF0000 != 0 {
		return (dwFlags >> 16) & 0xFF
	}

	nChannels := ChannelsOf(colorspace)

	// HighResPrecalc is maximum resolution.
	if dwFlags&FlagsHighResPrecalc != 0 {
		if nChannels > 4 {
			return 7 // 7 for Hifi
		}
		if nChannels == 4 { // 23 for CMYK
			return 23
		}
		return 49 // 49 for RGB and others
	}

	// LowResPrecalc is lower resolution.
	if dwFlags&FlagsLowResPrecalc != 0 {
		if nChannels > 4 {
			return 6 // 6 for more than 4 channels
		}
		if nChannels == 1 {
			return 33 // For monochrome
		}
		return 17 // 17 for remaining
	}

	// Default values.
	if nChannels > 4 {
		return 7 // 7 for Hifi
	}
	if nChannels == 4 {
		return 17 // 17 for CMYK
	}
	return 33 // 33 for RGB
}

// EndPointsBySpace returns the white and black end points (and channel count)
// for the most common color spaces, and ok=false for others
// (_cmsEndPointsBySpace). The returned slices alias package-level tables and
// must not be mutated by callers.
func EndPointsBySpace(space ColorSpaceSignature) (white, black []uint16, nOutputs uint32, ok bool) {
	switch space {
	case SigGrayData:
		return grayWhite, grayBlack, 1, true
	case SigRgbData:
		return rgbWhite, rgbBlack, 3, true
	case SigLabData:
		return labWhite, labBlack, 3, true
	case SigCmykData:
		return cmykWhite, cmykBlack, 4, true
	case SigCmyData:
		return cmyWhite, cmyBlack, 3, true
	default:
		return nil, nil, 0, false
	}
}

// End-point tables for EndPointsBySpace (static arrays in cmspcs.c).
var (
	rgbBlack  = []uint16{0, 0, 0}
	rgbWhite  = []uint16{0xffff, 0xffff, 0xffff}
	cmykBlack = []uint16{0xffff, 0xffff, 0xffff, 0xffff} // 400% of ink
	cmykWhite = []uint16{0, 0, 0, 0}
	labBlack  = []uint16{0, 0x8080, 0x8080} // V4 Lab encoding
	labWhite  = []uint16{0xFFFF, 0x8080, 0x8080}
	cmyBlack  = []uint16{0xffff, 0xffff, 0xffff}
	cmyWhite  = []uint16{0, 0, 0}
	grayBlack = []uint16{0}
	grayWhite = []uint16{0xffff}
)

// ---------------------------------------------------------------------------
// Color-space signature <-> internal notation, and channel counts.
// ---------------------------------------------------------------------------

// ICCcolorSpace translates from lcms2's internal PT_* notation to an ICC color
// space signature (_cmsICCcolorSpace). Returns 0 for unknown values.
func ICCcolorSpace(ourNotation int) ColorSpaceSignature {
	switch ourNotation {
	case 1, PTGray:
		return SigGrayData
	case 2, PTRGB:
		return SigRgbData
	case PTCMY:
		return SigCmyData
	case PTCMYK:
		return SigCmykData
	case PTYCbCr:
		return SigYCbCrData
	case PTYUV:
		return SigLuvData
	case PTXYZ:
		return SigXYZData
	case PTLabV2, PTLab:
		return SigLabData
	case PTYUVK:
		return SigLuvKData
	case PTHSV:
		return SigHsvData
	case PTHLS:
		return SigHlsData
	case PTYxy:
		return SigYxyData
	case PTMCH1:
		return SigMCH1Data
	case PTMCH2:
		return SigMCH2Data
	case PTMCH3:
		return SigMCH3Data
	case PTMCH4:
		return SigMCH4Data
	case PTMCH5:
		return SigMCH5Data
	case PTMCH6:
		return SigMCH6Data
	case PTMCH7:
		return SigMCH7Data
	case PTMCH8:
		return SigMCH8Data
	case PTMCH9:
		return SigMCH9Data
	case PTMCH10:
		return SigMCHAData
	case PTMCH11:
		return SigMCHBData
	case PTMCH12:
		return SigMCHCData
	case PTMCH13:
		return SigMCHDData
	case PTMCH14:
		return SigMCHEData
	case PTMCH15:
		return SigMCHFData
	default:
		return 0
	}
}

// LCMScolorSpace translates from an ICC color space signature to lcms2's
// internal PT_* notation (_cmsLCMScolorSpace). Returns 0 for unknown values.
func LCMScolorSpace(profileSpace ColorSpaceSignature) int {
	switch profileSpace {
	case SigGrayData:
		return PTGray
	case SigRgbData:
		return PTRGB
	case SigCmyData:
		return PTCMY
	case SigCmykData:
		return PTCMYK
	case SigYCbCrData:
		return PTYCbCr
	case SigLuvData:
		return PTYUV
	case SigXYZData:
		return PTXYZ
	case SigLabData:
		return PTLab
	case SigLuvKData:
		return PTYUVK
	case SigHsvData:
		return PTHSV
	case SigHlsData:
		return PTHLS
	case SigYxyData:
		return PTYxy
	case Sig1colorData, SigMCH1Data:
		return PTMCH1
	case Sig2colorData, SigMCH2Data:
		return PTMCH2
	case Sig3colorData, SigMCH3Data:
		return PTMCH3
	case Sig4colorData, SigMCH4Data:
		return PTMCH4
	case Sig5colorData, SigMCH5Data:
		return PTMCH5
	case Sig6colorData, SigMCH6Data:
		return PTMCH6
	case SigMCH7Data, Sig7colorData:
		return PTMCH7
	case SigMCH8Data, Sig8colorData:
		return PTMCH8
	case SigMCH9Data, Sig9colorData:
		return PTMCH9
	case SigMCHAData, Sig10colorData:
		return PTMCH10
	case SigMCHBData, Sig11colorData:
		return PTMCH11
	case SigMCHCData, Sig12colorData:
		return PTMCH12
	case SigMCHDData, Sig13colorData:
		return PTMCH13
	case SigMCHEData, Sig14colorData:
		return PTMCH14
	case SigMCHFData, Sig15colorData:
		return PTMCH15
	default:
		return 0
	}
}

// ChannelsOfColorSpace returns the number of channels for a color space, or -1
// for unknown (cmsChannelsOfColorSpace).
func ChannelsOfColorSpace(colorSpace ColorSpaceSignature) int32 {
	switch colorSpace {
	case SigMCH1Data, Sig1colorData, SigGrayData:
		return 1
	case SigMCH2Data, Sig2colorData:
		return 2
	case SigXYZData, SigLabData, SigLuvData, SigYCbCrData, SigYxyData,
		SigRgbData, SigHsvData, SigHlsData, SigCmyData, SigMCH3Data, Sig3colorData:
		return 3
	case SigLuvKData, SigCmykData, SigMCH4Data, Sig4colorData:
		return 4
	case SigMCH5Data, Sig5colorData:
		return 5
	case SigMCH6Data, Sig6colorData:
		return 6
	case SigMCH7Data, Sig7colorData:
		return 7
	case SigMCH8Data, Sig8colorData:
		return 8
	case SigMCH9Data, Sig9colorData:
		return 9
	case SigMCHAData, Sig10colorData:
		return 10
	case SigMCHBData, Sig11colorData:
		return 11
	case SigMCHCData, Sig12colorData:
		return 12
	case SigMCHDData, Sig13colorData:
		return 13
	case SigMCHEData, Sig14colorData:
		return 14
	case SigMCHFData, Sig15colorData:
		return 15
	default:
		return -1
	}
}

// ChannelsOf returns the number of channels for a color space, defaulting to 3
// for unknown spaces. DEPRECATED in the reference; provided for compatibility
// (cmsChannelsOf).
func ChannelsOf(colorSpace ColorSpaceSignature) uint32 {
	n := ChannelsOfColorSpace(colorSpace)
	if n < 0 {
		return 3
	}
	return uint32(n)
}
