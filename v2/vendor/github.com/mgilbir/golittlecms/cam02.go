package lcms2

// cam02.go ports src/cmscam02.c: the CIECAM02 color appearance model.
//
// Many thanks to Jordi Vilar for the debugging (per the C source).
//
// The port is a faithful float64 translation preserving the exact order of
// operations of the reference. Intermediate products that could otherwise be
// contracted into a fused multiply-add are kept as-is; Go does not emit an FMA
// for the plain "a*b + c*d" expressions used here (no math.FMA calls), so the
// results match the C reference bit-for-bit on tested inputs. Should a future
// toolchain diverge, wrap the offending products in float64(...) to force a
// rounding step, per the PLAN.md FMA rule.

import "math"

// Surround constants (lcms2.h): AVG/DIM/DARK/CUTSHEET.
const (
	AvgSurround      = 1 // AVG_SURROUND
	DimSurround      = 2 // DIM_SURROUND
	DarkSurround     = 3 // DARK_SURROUND
	CutsheetSurround = 4 // CUTSHEET_SURROUND

	// DCalculate signals cmsCIECAM02Init to compute D from the luminance
	// (D_CALCULATE == -1).
	DCalculate = -1.0
)

// JCh is a color in the CIECAM02 J (lightness), C (chroma), h (hue) space
// (cmsJCh).
type JCh struct {
	J, C, h float64
}

// ViewingConditions mirrors cmsViewingConditions: the observing environment fed
// to cmsCIECAM02Init.
type ViewingConditions struct {
	WhitePoint CIEXYZ  // whitePoint
	Yb         float64 // Yb
	La         float64 // La
	Surround   uint32  // surround
	DValue     float64 // D_value
}

// cam02Color mirrors CAM02COLOR: the working color plus every intermediate
// correlate. Only the fields the port actually uses are consumed, but all are
// kept to mirror the reference layout exactly.
type cam02Color struct {
	XYZ                                [3]float64
	RGB                                [3]float64
	RGBc                               [3]float64
	RGBp                               [3]float64
	RGBpa                              [3]float64
	a, b, h, e, H, A, J, Q, s, t, C, M float64
	abC                                [2]float64
	abs                                [2]float64
	abM                                [2]float64
}

// cmsCIECAM02 mirrors the reference model state.
type cmsCIECAM02 struct {
	adoptedWhite          cam02Color
	LA, Yb                float64
	F, c, Nc              float64
	surround              uint32
	n, Nbb, Ncb, z, FL, D float64

	ContextID *Context
}

func (m *cmsCIECAM02) computeN() float64 {
	return m.Yb / m.adoptedWhite.XYZ[1]
}

func (m *cmsCIECAM02) computeZ() float64 {
	return 1.48 + math.Sqrt(m.n)
}

func (m *cmsCIECAM02) computeNbb() float64 {
	return 0.725 * math.Pow(1.0/m.n, 0.2)
}

func (m *cmsCIECAM02) computeFL() float64 {
	k := 1.0 / ((5.0 * m.LA) + 1.0)
	FL := 0.2*math.Pow(k, 4.0)*(5.0*m.LA) + 0.1*
		(math.Pow(1.0-math.Pow(k, 4.0), 2.0))*
		(math.Pow(5.0*m.LA, 1.0/3.0))
	return FL
}

func (m *cmsCIECAM02) computeD() float64 {
	temp := 1.0 - ((1.0 / 3.6) * math.Exp((-m.LA-42)/92.0))
	return m.F * temp
}

func xyzToCAT02(clr cam02Color) cam02Color {
	clr.RGB[0] = (clr.XYZ[0] * 0.7328) + (clr.XYZ[1] * 0.4296) + (clr.XYZ[2] * -0.1624)
	clr.RGB[1] = (clr.XYZ[0] * -0.7036) + (clr.XYZ[1] * 1.6975) + (clr.XYZ[2] * 0.0061)
	clr.RGB[2] = (clr.XYZ[0] * 0.0030) + (clr.XYZ[1] * 0.0136) + (clr.XYZ[2] * 0.9834)
	return clr
}

func chromaticAdaptation(clr cam02Color, m *cmsCIECAM02) cam02Color {
	for i := 0; i < 3; i++ {
		clr.RGBc[i] = ((m.adoptedWhite.XYZ[1] *
			(m.D / m.adoptedWhite.RGB[i])) +
			(1.0 - m.D)) * clr.RGB[i]
	}
	return clr
}

func cat02ToHPE(clr cam02Color) cam02Color {
	var M [9]float64
	M[0] = (0.38971 * 1.096124) + (0.68898 * 0.454369) + (-0.07868 * -0.009628)
	M[1] = (0.38971 * -0.278869) + (0.68898 * 0.473533) + (-0.07868 * -0.005698)
	M[2] = (0.38971 * 0.182745) + (0.68898 * 0.072098) + (-0.07868 * 1.015326)
	M[3] = (-0.22981 * 1.096124) + (1.18340 * 0.454369) + (0.04641 * -0.009628)
	M[4] = (-0.22981 * -0.278869) + (1.18340 * 0.473533) + (0.04641 * -0.005698)
	M[5] = (-0.22981 * 0.182745) + (1.18340 * 0.072098) + (0.04641 * 1.015326)
	M[6] = -0.009628
	M[7] = -0.005698
	M[8] = 1.015326

	clr.RGBp[0] = (clr.RGBc[0] * M[0]) + (clr.RGBc[1] * M[1]) + (clr.RGBc[2] * M[2])
	clr.RGBp[1] = (clr.RGBc[0] * M[3]) + (clr.RGBc[1] * M[4]) + (clr.RGBc[2] * M[5])
	clr.RGBp[2] = (clr.RGBc[0] * M[6]) + (clr.RGBc[1] * M[7]) + (clr.RGBc[2] * M[8])
	return clr
}

func nonlinearCompression(clr cam02Color, m *cmsCIECAM02) cam02Color {
	for i := 0; i < 3; i++ {
		if clr.RGBp[i] < 0 {
			temp := math.Pow(-1.0*m.FL*clr.RGBp[i]/100.0, 0.42)
			clr.RGBpa[i] = (-1.0*400.0*temp)/(temp+27.13) + 0.1
		} else {
			temp := math.Pow(m.FL*clr.RGBp[i]/100.0, 0.42)
			clr.RGBpa[i] = (400.0*temp)/(temp+27.13) + 0.1
		}
	}

	clr.A = (((2.0 * clr.RGBpa[0]) + clr.RGBpa[1] +
		(clr.RGBpa[2] / 20.0)) - 0.305) * m.Nbb
	return clr
}

func computeCorrelates(clr cam02Color, m *cmsCIECAM02) cam02Color {
	var temp float64

	a := clr.RGBpa[0] - (12.0 * clr.RGBpa[1] / 11.0) + (clr.RGBpa[2] / 11.0)
	b := (clr.RGBpa[0] + clr.RGBpa[1] - (2.0 * clr.RGBpa[2])) / 9.0

	r2d := 180.0 / 3.141592654
	if a == 0 {
		if b == 0 {
			clr.h = 0
		} else if b > 0 {
			clr.h = 90
		} else {
			clr.h = 270
		}
	} else if a > 0 {
		temp = b / a
		if b > 0 {
			clr.h = r2d * math.Atan(temp)
		} else if b == 0 {
			clr.h = 0
		} else {
			clr.h = (r2d * math.Atan(temp)) + 360
		}
	} else {
		temp = b / a
		clr.h = (r2d * math.Atan(temp)) + 180
	}

	d2r := 3.141592654 / 180.0
	e := ((12500.0 / 13.0) * m.Nc * m.Ncb) *
		(math.Cos(clr.h*d2r+2.0) + 3.8)

	if clr.h < 20.14 {
		temp = ((clr.h + 122.47) / 1.2) + ((20.14 - clr.h) / 0.8)
		clr.H = 300 + (100*((clr.h+122.47)/1.2))/temp
	} else if clr.h < 90.0 {
		temp = ((clr.h - 20.14) / 0.8) + ((90.00 - clr.h) / 0.7)
		clr.H = (100 * ((clr.h - 20.14) / 0.8)) / temp
	} else if clr.h < 164.25 {
		temp = ((clr.h - 90.00) / 0.7) + ((164.25 - clr.h) / 1.0)
		clr.H = 100 + ((100 * ((clr.h - 90.00) / 0.7)) / temp)
	} else if clr.h < 237.53 {
		temp = ((clr.h - 164.25) / 1.0) + ((237.53 - clr.h) / 1.2)
		clr.H = 200 + ((100 * ((clr.h - 164.25) / 1.0)) / temp)
	} else {
		temp = ((clr.h - 237.53) / 1.2) + ((360 - clr.h + 20.14) / 0.8)
		clr.H = 300 + ((100 * ((clr.h - 237.53) / 1.2)) / temp)
	}

	clr.J = 100.0 * math.Pow(clr.A/m.adoptedWhite.A, m.c*m.z)

	clr.Q = (4.0 / m.c) * math.Sqrt(clr.J/100.0) *
		(m.adoptedWhite.A + 4.0) * math.Pow(m.FL, 0.25)

	t := (e * math.Pow((a*a)+(b*b), 0.5)) /
		(clr.RGBpa[0] + clr.RGBpa[1] +
			((21.0 / 20.0) * clr.RGBpa[2]))

	clr.C = math.Pow(t, 0.9) * math.Sqrt(clr.J/100.0) *
		math.Pow(1.64-math.Pow(0.29, m.n), 0.73)

	clr.M = clr.C * math.Pow(m.FL, 0.25)
	clr.s = 100.0 * math.Sqrt(clr.M/clr.Q)
	clr.e = e
	clr.a = a
	clr.b = b
	clr.t = t
	return clr
}

func inverseCorrelates(clr cam02Color, m *cmsCIECAM02) cam02Color {
	d2r := 3.141592654 / 180.0

	t := math.Pow(
		clr.C/(math.Sqrt(clr.J/100.0)*
			math.Pow(1.64-math.Pow(0.29, m.n), 0.73)),
		1.0/0.9)
	e := ((12500.0 / 13.0) * m.Nc * m.Ncb) *
		(math.Cos(clr.h*d2r+2.0) + 3.8)

	clr.A = m.adoptedWhite.A * math.Pow(
		clr.J/100.0,
		1.0/(m.c*m.z))

	p2 := (clr.A / m.Nbb) + 0.305

	if t <= 0.0 { // special case from spec notes, avoid divide by zero
		clr.a = 0.0
		clr.b = 0.0
	} else {
		hr := clr.h * d2r
		p1 := e / t
		p3 := 21.0 / 20.0

		if math.Abs(math.Sin(hr)) >= math.Abs(math.Cos(hr)) {
			p4 := p1 / math.Sin(hr)
			clr.b = (p2 * (2.0 + p3) * (460.0 / 1403.0)) /
				(p4 + (2.0+p3)*(220.0/1403.0)*
					(math.Cos(hr)/math.Sin(hr)) - (27.0 / 1403.0) +
					p3*(6300.0/1403.0))
			clr.a = clr.b * (math.Cos(hr) / math.Sin(hr))
		} else {
			p5 := p1 / math.Cos(hr)
			clr.a = (p2 * (2.0 + p3) * (460.0 / 1403.0)) /
				(p5 + (2.0+p3)*(220.0/1403.0) -
					((27.0/1403.0)-p3*(6300.0/1403.0))*
						(math.Sin(hr)/math.Cos(hr)))
			clr.b = clr.a * (math.Sin(hr) / math.Cos(hr))
		}
	}

	clr.RGBpa[0] = ((460.0 / 1403.0) * p2) +
		((451.0 / 1403.0) * clr.a) +
		((288.0 / 1403.0) * clr.b)
	clr.RGBpa[1] = ((460.0 / 1403.0) * p2) -
		((891.0 / 1403.0) * clr.a) -
		((261.0 / 1403.0) * clr.b)
	clr.RGBpa[2] = ((460.0 / 1403.0) * p2) -
		((220.0 / 1403.0) * clr.a) -
		((6300.0 / 1403.0) * clr.b)
	return clr
}

func inverseNonlinearity(clr cam02Color, m *cmsCIECAM02) cam02Color {
	for i := 0; i < 3; i++ {
		var c1 float64
		if (clr.RGBpa[i] - 0.1) < 0 {
			c1 = -1
		} else {
			c1 = 1
		}
		clr.RGBp[i] = c1 * (100.0 / m.FL) *
			math.Pow((27.13*math.Abs(clr.RGBpa[i]-0.1))/
				(400.0-math.Abs(clr.RGBpa[i]-0.1)),
				1.0/0.42)
	}
	return clr
}

func hpeToCAT02(clr cam02Color) cam02Color {
	var M [9]float64
	M[0] = (0.7328 * 1.910197) + (0.4296 * 0.370950)
	M[1] = (0.7328 * -1.112124) + (0.4296 * 0.629054)
	M[2] = (0.7328 * 0.201908) + (0.4296 * 0.000008) - 0.1624
	M[3] = (-0.7036 * 1.910197) + (1.6975 * 0.370950)
	M[4] = (-0.7036 * -1.112124) + (1.6975 * 0.629054)
	M[5] = (-0.7036 * 0.201908) + (1.6975 * 0.000008) + 0.0061
	M[6] = (0.0030 * 1.910197) + (0.0136 * 0.370950)
	M[7] = (0.0030 * -1.112124) + (0.0136 * 0.629054)
	M[8] = (0.0030 * 0.201908) + (0.0136 * 0.000008) + 0.9834

	clr.RGBc[0] = (clr.RGBp[0] * M[0]) + (clr.RGBp[1] * M[1]) + (clr.RGBp[2] * M[2])
	clr.RGBc[1] = (clr.RGBp[0] * M[3]) + (clr.RGBp[1] * M[4]) + (clr.RGBp[2] * M[5])
	clr.RGBc[2] = (clr.RGBp[0] * M[6]) + (clr.RGBp[1] * M[7]) + (clr.RGBp[2] * M[8])
	return clr
}

func inverseChromaticAdaptation(clr cam02Color, m *cmsCIECAM02) cam02Color {
	for i := 0; i < 3; i++ {
		clr.RGB[i] = clr.RGBc[i] /
			((m.adoptedWhite.XYZ[1] * m.D / m.adoptedWhite.RGB[i]) + 1.0 - m.D)
	}
	return clr
}

func cat02ToXYZ(clr cam02Color) cam02Color {
	clr.XYZ[0] = (clr.RGB[0] * 1.096124) + (clr.RGB[1] * -0.278869) + (clr.RGB[2] * 0.182745)
	clr.XYZ[1] = (clr.RGB[0] * 0.454369) + (clr.RGB[1] * 0.473533) + (clr.RGB[2] * 0.072098)
	clr.XYZ[2] = (clr.RGB[0] * -0.009628) + (clr.RGB[1] * -0.005698) + (clr.RGB[2] * 1.015326)
	return clr
}

// CIECAM02Init ports cmsCIECAM02Init: build a model handle from the viewing
// conditions. Returns nil on allocation-style failure (never in the pure-Go
// port, but the signature mirrors the reference contract).
func (ctx *Context) CIECAM02Init(pVC *ViewingConditions) *cmsCIECAM02 {
	if pVC == nil {
		return nil
	}

	m := &cmsCIECAM02{}
	m.ContextID = ctx

	m.adoptedWhite.XYZ[0] = pVC.WhitePoint.X
	m.adoptedWhite.XYZ[1] = pVC.WhitePoint.Y
	m.adoptedWhite.XYZ[2] = pVC.WhitePoint.Z

	m.LA = pVC.La
	m.Yb = pVC.Yb
	m.D = pVC.DValue
	m.surround = pVC.Surround

	switch m.surround {
	case CutsheetSurround:
		m.F = 0.8
		m.c = 0.41
		m.Nc = 0.8
	case DarkSurround:
		m.F = 0.8
		m.c = 0.525
		m.Nc = 0.8
	case DimSurround:
		m.F = 0.9
		m.c = 0.59
		m.Nc = 0.95
	default:
		// Average surround
		m.F = 1.0
		m.c = 0.69
		m.Nc = 1.0
	}

	m.n = m.computeN()
	m.z = m.computeZ()
	m.Nbb = m.computeNbb()
	m.FL = m.computeFL()

	if m.D == DCalculate {
		m.D = m.computeD()
	}

	m.Ncb = m.Nbb

	m.adoptedWhite = xyzToCAT02(m.adoptedWhite)
	m.adoptedWhite = chromaticAdaptation(m.adoptedWhite, m)
	m.adoptedWhite = cat02ToHPE(m.adoptedWhite)
	m.adoptedWhite = nonlinearCompression(m.adoptedWhite, m)

	return m
}

// CIECAM02Init ports cmsCIECAM02Init on the default context.
func CIECAM02Init(pVC *ViewingConditions) *cmsCIECAM02 {
	return defaultContext.CIECAM02Init(pVC)
}

// CIECAM02Done ports cmsCIECAM02Done. Under the Go GC there is nothing to free;
// kept for API parity.
func (m *cmsCIECAM02) CIECAM02Done() {}

// CIECAM02Forward ports cmsCIECAM02Forward: XYZ -> JCh.
func (m *cmsCIECAM02) CIECAM02Forward(pIn *CIEXYZ, pOut *JCh) {
	if m == nil || pIn == nil || pOut == nil {
		return
	}

	var clr cam02Color
	clr.XYZ[0] = pIn.X
	clr.XYZ[1] = pIn.Y
	clr.XYZ[2] = pIn.Z

	clr = xyzToCAT02(clr)
	clr = chromaticAdaptation(clr, m)
	clr = cat02ToHPE(clr)
	clr = nonlinearCompression(clr, m)
	clr = computeCorrelates(clr, m)

	pOut.J = clr.J
	pOut.C = clr.C
	pOut.h = clr.h
}

// CIECAM02Reverse ports cmsCIECAM02Reverse: JCh -> XYZ.
func (m *cmsCIECAM02) CIECAM02Reverse(pIn *JCh, pOut *CIEXYZ) {
	if m == nil || pIn == nil || pOut == nil {
		return
	}

	var clr cam02Color
	clr.J = pIn.J
	clr.C = pIn.C
	clr.h = pIn.h

	clr = inverseCorrelates(clr, m)
	clr = inverseNonlinearity(clr, m)
	clr = hpeToCAT02(clr)
	clr = inverseChromaticAdaptation(clr, m)
	clr = cat02ToXYZ(clr)

	pOut.X = clr.XYZ[0]
	pOut.Y = clr.XYZ[1]
	pOut.Z = clr.XYZ[2]
}
