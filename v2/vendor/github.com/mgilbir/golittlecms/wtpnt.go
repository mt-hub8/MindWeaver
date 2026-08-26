package lcms2

// White points and chromatic adaptation (Bradford). Port of src/cmswtpnt.c.

import "math"

// D50 tristimulus values, mirroring the cmsD50X/Y/Z macros in include/lcms2.h.
const (
	D50X = 0.9642
	D50Y = 1.0
	D50Z = 0.8249
)

// d50XYZ returns the D50 white point in XYZ. Internal accessor used by pcs.go.
func d50XYZ() CIEXYZ {
	return CIEXYZ{X: D50X, Y: D50Y, Z: D50Z}
}

// D50XYZ returns the D50 white point in XYZ (cmsD50_XYZ).
func D50XYZ() CIEXYZ {
	return d50XYZ()
}

// D50xyY returns the D50 white point in xyY (cmsD50_xyY).
func D50xyY() CIExyY {
	return XYZ2xyY(d50XYZ())
}

// WhitePointFromTemp computes a white point (in xyY) from a correlated color
// temperature in kelvin (cmsWhitePointFromTemp). It returns an ErrRange error
// for temperatures outside 4000K..25000K.
func WhitePointFromTemp(tempK float64) (CIExyY, error) {
	var x, y float64

	t := tempK
	t2 := t * t  // Square
	t3 := t2 * t // Cube

	// For correlated color temperature (T) between 4000K and 7000K:
	if t >= 4000. && t <= 7000. {
		x = fmul64(-4.6070, 1e9/t3) + fmul64(2.9678, 1e6/t2) + fmul64(0.09911, 1e3/t) + 0.244063
	} else if t > 7000.0 && t <= 25000.0 {
		// or for correlated color temperature (T) between 7000K and 25000K:
		x = fmul64(-2.0064, 1e9/t3) + fmul64(1.9018, 1e6/t2) + fmul64(0.24748, 1e3/t) + 0.237040
	} else {
		return CIExyY{}, defaultContext.signalError(ErrRange, "cmsWhitePointFromTemp: invalid temp")
	}

	// Obtain y(x)
	y = fmul64(-3.000, x*x) + fmul64(2.870, x) - 0.275

	return CIExyY{X: x, Y: y, YY: 1.0}, nil
}

// isotemperature is one row of Robertson's isotemperature-line table.
type isotemperature struct {
	mirek float64 // temp (in microreciprocal kelvin)
	ut    float64 // u coord of intersection w/ blackbody locus
	vt    float64 // v coord of intersection w/ blackbody locus
	tt    float64 // slope of isotemperature line
}

var isotempdata = [...]isotemperature{
	//  {Mirek, Ut,       Vt,      Tt      }
	{0, 0.18006, 0.26352, -0.24341},
	{10, 0.18066, 0.26589, -0.25479},
	{20, 0.18133, 0.26846, -0.26876},
	{30, 0.18208, 0.27119, -0.28539},
	{40, 0.18293, 0.27407, -0.30470},
	{50, 0.18388, 0.27709, -0.32675},
	{60, 0.18494, 0.28021, -0.35156},
	{70, 0.18611, 0.28342, -0.37915},
	{80, 0.18740, 0.28668, -0.40955},
	{90, 0.18880, 0.28997, -0.44278},
	{100, 0.19032, 0.29326, -0.47888},
	{125, 0.19462, 0.30141, -0.58204},
	{150, 0.19962, 0.30921, -0.70471},
	{175, 0.20525, 0.31647, -0.84901},
	{200, 0.21142, 0.32312, -1.0182},
	{225, 0.21807, 0.32909, -1.2168},
	{250, 0.22511, 0.33439, -1.4512},
	{275, 0.23247, 0.33904, -1.7298},
	{300, 0.24010, 0.34308, -2.0637},
	{325, 0.24702, 0.34655, -2.4681},
	{350, 0.25591, 0.34951, -2.9641},
	{375, 0.26400, 0.35200, -3.5814},
	{400, 0.27218, 0.35407, -4.3633},
	{425, 0.28039, 0.35577, -5.3762},
	{450, 0.28863, 0.35714, -6.7262},
	{475, 0.29685, 0.35823, -8.5955},
	{500, 0.30505, 0.35907, -11.324},
	{525, 0.31320, 0.35968, -15.628},
	{550, 0.32129, 0.36011, -23.325},
	{575, 0.32931, 0.36038, -40.770},
	{600, 0.33724, 0.36051, -116.45},
}

// TempFromWhitePoint computes the correlated color temperature (kelvin) for a
// white point using Robertson's method (cmsTempFromWhitePoint). It returns an
// ErrRange error when no isotemperature line brackets the input; the C
// reference returns FALSE without signalling in that case.
func TempFromWhitePoint(whitePoint CIExyY) (float64, error) {
	var di, mi float64
	di, mi = 0, 0

	xs := whitePoint.X
	ys := whitePoint.Y

	// convert (x,y) to CIE 1960 (u,v)
	us := (2 * xs) / (-xs + 6*ys + 1.5)
	vs := (3 * ys) / (-xs + 6*ys + 1.5)

	for j := 0; j < len(isotempdata); j++ {
		uj := isotempdata[j].ut
		vj := isotempdata[j].vt
		tj := isotempdata[j].tt
		mj := isotempdata[j].mirek

		dj := ((vs - vj) - tj*(us-uj)) / math.Sqrt(1.0+tj*tj)

		if (j != 0) && (di/dj < 0.0) {
			// Found a match
			tempK := 1000000.0 / (mi + (di/(di-dj))*(mj-mi))
			return tempK, nil
		}

		di = dj
		mi = mj
	}

	// Not found
	return 0, errorf(ErrRange, "cmsTempFromWhitePoint: temperature not found")
}

// computeChromaticAdaptation builds a chromatic adaptation matrix using chad as
// the cone response matrix (ComputeChromaticAdaptation). ok=false indicates a
// singular chad or a zero cone response.
func computeChromaticAdaptation(sourceWhitePoint, destWhitePoint CIEXYZ, chad MAT3) (conversion MAT3, ok bool) {
	chadInv, ok := MAT3Inverse(chad)
	if !ok {
		return MAT3{}, false
	}

	coneSourceXYZ := VEC3Init(sourceWhitePoint.X, sourceWhitePoint.Y, sourceWhitePoint.Z)
	coneDestXYZ := VEC3Init(destWhitePoint.X, destWhitePoint.Y, destWhitePoint.Z)

	coneSourceRGB := MAT3Eval(chad, coneSourceXYZ)
	coneDestRGB := MAT3Eval(chad, coneDestXYZ)

	if (math.Abs(coneSourceRGB[0]) < matrixDetTolerance) ||
		(math.Abs(coneSourceRGB[1]) < matrixDetTolerance) ||
		(math.Abs(coneSourceRGB[2]) < matrixDetTolerance) {
		return MAT3{}, false
	}

	// Build matrix
	var cone MAT3
	cone[0] = VEC3Init(coneDestRGB[0]/coneSourceRGB[0], 0.0, 0.0)
	cone[1] = VEC3Init(0.0, coneDestRGB[1]/coneSourceRGB[1], 0.0)
	cone[2] = VEC3Init(0.0, 0.0, coneDestRGB[2]/coneSourceRGB[2])

	// Normalize
	tmp := MAT3Per(cone, chad)
	conversion = MAT3Per(chadInv, tmp)

	return conversion, true
}

// bradfordMatrix is the Bradford cone response matrix (LamRigg in cmswtpnt.c).
var bradfordMatrix = MAT3{
	{0.8951, 0.2664, -0.1614},
	{-0.7502, 1.7135, 0.0367},
	{0.0389, -0.0685, 1.0296},
}

// adaptationMatrix returns the chromatic adaptation matrix from illuminant
// fromIll to illuminant toIll. If coneMatrix is nil, the Bradford matrix is
// used (_cmsAdaptationMatrix).
func adaptationMatrix(coneMatrix *MAT3, fromIll, toIll CIEXYZ) (MAT3, bool) {
	cone := bradfordMatrix
	if coneMatrix != nil {
		cone = *coneMatrix
	}
	return computeChromaticAdaptation(fromIll, toIll, cone)
}

// adaptMatrixToD50 adapts matrix r to a D50 destination, given the source
// white point in xyY (_cmsAdaptMatrixToD50).
func adaptMatrixToD50(r MAT3, sourceWhitePt CIExyY) (MAT3, bool) {
	dn := XyY2XYZ(sourceWhitePt)

	bradford, ok := adaptationMatrix(nil, dn, d50XYZ())
	if !ok {
		return MAT3{}, false
	}

	return MAT3Per(bradford, r), true
}

// buildRGB2XYZTransferMatrix builds a white point / primary chroma transfer
// matrix from RGB to CIE XYZ (_cmsBuildRGB2XYZtransferMatrix). This is an
// approximation that assumes gamma correction has the transitive property in
// the transformation chain. ok=false indicates a singular primaries matrix.
func buildRGB2XYZTransferMatrix(whitePt CIExyY, primrs CIExyYTRIPLE) (MAT3, bool) {
	xn := whitePt.X
	yn := whitePt.Y
	xr := primrs.Red.X
	yr := primrs.Red.Y
	xg := primrs.Green.X
	yg := primrs.Green.Y
	xb := primrs.Blue.X
	yb := primrs.Blue.Y

	// Build Primaries matrix
	var primaries MAT3
	primaries[0] = VEC3Init(xr, xg, xb)
	primaries[1] = VEC3Init(yr, yg, yb)
	primaries[2] = VEC3Init((1 - xr - yr), (1 - xg - yg), (1 - xb - yb))

	// Result = Primaries ^ (-1) inverse matrix
	result, ok := MAT3Inverse(primaries)
	if !ok {
		return MAT3{}, false
	}

	whitePoint := VEC3Init(xn/yn, 1.0, (1.0-xn-yn)/yn)

	// Across inverse primaries ...
	coef := MAT3Eval(result, whitePoint)

	// Give us the Coefs, then build the transformation matrix
	var r MAT3
	r[0] = VEC3Init(coef[VX]*xr, coef[VY]*xg, coef[VZ]*xb)
	r[1] = VEC3Init(coef[VX]*yr, coef[VY]*yg, coef[VZ]*yb)
	r[2] = VEC3Init(coef[VX]*(1.0-xr-yr), coef[VY]*(1.0-xg-yg), coef[VZ]*(1.0-xb-yb))

	return adaptMatrixToD50(r, whitePt)
}

// AdaptToIlluminant adapts a color to a given illuminant. The original color is
// expected to have the sourceWhitePt white point (cmsAdaptToIlluminant).
// ok=false indicates the adaptation matrix could not be built.
func AdaptToIlluminant(sourceWhitePt, illuminant, value CIEXYZ) (CIEXYZ, bool) {
	bradford, ok := adaptationMatrix(nil, sourceWhitePt, illuminant)
	if !ok {
		return CIEXYZ{}, false
	}

	in := VEC3Init(value.X, value.Y, value.Z)
	out := MAT3Eval(bradford, in)

	return CIEXYZ{X: out[0], Y: out[1], Z: out[2]}, true
}
