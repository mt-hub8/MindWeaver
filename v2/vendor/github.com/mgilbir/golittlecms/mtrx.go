package lcms2

// 3x3 matrix / 3-vector math. Port of src/cmsmtrx.c.
//
// Layout mirrors the C types: cmsVEC3 is a length-3 array (accessed as v.n[VX]
// etc.), cmsMAT3 is three row vectors (a.v[i].n[j]). Here VEC3 is [3]float64
// and MAT3 is [3]VEC3, so C's a->v[i].n[j] maps directly to a[i][j]. Operation
// order is preserved exactly for floating-point parity with the reference.

import "math"

// Axis indices into a VEC3. No specific meaning; mirrors VX/VY/VZ in
// include/lcms2_plugin.h.
const (
	VX = 0
	VY = 1
	VZ = 2
)

// VEC3 is a 3-component vector of float64 (cmsVEC3).
type VEC3 [3]float64

// MAT3 is a 3x3 matrix of float64, stored as three row vectors (cmsMAT3).
type MAT3 [3]VEC3

// matrixDetTolerance: determinants smaller than this are treated as zero when
// inverting (MATRIX_DET_TOLERANCE in lcms2_internal.h).
const matrixDetTolerance = 0.0001

// VEC3Init builds a vector from its components (_cmsVEC3init).
func VEC3Init(x, y, z float64) VEC3 {
	return VEC3{x, y, z}
}

// VEC3Minus returns a - b (_cmsVEC3minus).
func VEC3Minus(a, b VEC3) VEC3 {
	return VEC3{
		a[VX] - b[VX],
		a[VY] - b[VY],
		a[VZ] - b[VZ],
	}
}

// VEC3Cross returns the cross product u x v (_cmsVEC3cross).
func VEC3Cross(u, v VEC3) VEC3 {
	return VEC3{
		u[VY]*v[VZ] - v[VY]*u[VZ],
		u[VZ]*v[VX] - v[VZ]*u[VX],
		u[VX]*v[VY] - v[VX]*u[VY],
	}
}

// VEC3Dot returns the dot product u . v (_cmsVEC3dot).
func VEC3Dot(u, v VEC3) float64 {
	return u[VX]*v[VX] + u[VY]*v[VY] + u[VZ]*v[VZ]
}

// VEC3Length returns the Euclidean length of a (_cmsVEC3length).
func VEC3Length(a VEC3) float64 {
	return math.Sqrt(a[VX]*a[VX] + a[VY]*a[VY] + a[VZ]*a[VZ])
}

// VEC3Distance returns the Euclidean distance between a and b
// (_cmsVEC3distance).
func VEC3Distance(a, b VEC3) float64 {
	d1 := a[VX] - b[VX]
	d2 := a[VY] - b[VY]
	d3 := a[VZ] - b[VZ]
	return math.Sqrt(d1*d1 + d2*d2 + d3*d3)
}

// MAT3Identity returns the 3x3 identity matrix (_cmsMAT3identity).
func MAT3Identity() MAT3 {
	return MAT3{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}
}

// closeEnough reports whether a and b differ by less than 1/65535 (CloseEnough
// in cmsmtrx.c).
func closeEnough(a, b float64) bool {
	return math.Abs(b-a) < (1.0 / 65535.0)
}

// MAT3IsIdentity reports whether a is (numerically) the identity matrix
// (_cmsMAT3isIdentity).
func MAT3IsIdentity(a MAT3) bool {
	identity := MAT3Identity()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if !closeEnough(a[i][j], identity[i][j]) {
				return false
			}
		}
	}
	return true
}

// MAT3Per returns the matrix product a*b (_cmsMAT3per). The multiply-accumulate
// order matches the C ROWCOL macro exactly.
func MAT3Per(a, b MAT3) MAT3 {
	rowcol := func(i, j int) float64 {
		return a[i][0]*b[0][j] + a[i][1]*b[1][j] + a[i][2]*b[2][j]
	}
	return MAT3{
		{rowcol(0, 0), rowcol(0, 1), rowcol(0, 2)},
		{rowcol(1, 0), rowcol(1, 1), rowcol(1, 2)},
		{rowcol(2, 0), rowcol(2, 1), rowcol(2, 2)},
	}
}

// MAT3Inverse returns b = a^(-1) and ok=true, or ok=false when a is singular
// (|det| < matrixDetTolerance). Mirrors _cmsMAT3inverse, which returns cmsBool
// with no error signalling; a bool is used here rather than an error to match
// that hot-path semantics.
func MAT3Inverse(a MAT3) (b MAT3, ok bool) {
	c0 := a[1][1]*a[2][2] - a[1][2]*a[2][1]
	c1 := -a[1][0]*a[2][2] + a[1][2]*a[2][0]
	c2 := a[1][0]*a[2][1] - a[1][1]*a[2][0]

	det := a[0][0]*c0 + a[0][1]*c1 + a[0][2]*c2

	if math.Abs(det) < matrixDetTolerance {
		return MAT3{}, false // singular matrix; can't invert
	}

	b[0][0] = c0 / det
	b[0][1] = (a[0][2]*a[2][1] - a[0][1]*a[2][2]) / det
	b[0][2] = (a[0][1]*a[1][2] - a[0][2]*a[1][1]) / det
	b[1][0] = c1 / det
	b[1][1] = (a[0][0]*a[2][2] - a[0][2]*a[2][0]) / det
	b[1][2] = (a[0][2]*a[1][0] - a[0][0]*a[1][2]) / det
	b[2][0] = c2 / det
	b[2][1] = (a[0][1]*a[2][0] - a[0][0]*a[2][1]) / det
	b[2][2] = (a[0][0]*a[1][1] - a[0][1]*a[1][0]) / det

	return b, true
}

// MAT3Solve solves the system Ax = b, returning x and ok=false when A is
// singular (_cmsMAT3solve).
func MAT3Solve(a MAT3, b VEC3) (x VEC3, ok bool) {
	aInv, ok := MAT3Inverse(a)
	if !ok {
		return VEC3{}, false // Singular matrix
	}
	return MAT3Eval(aInv, b), true
}

// MAT3Eval evaluates the vector v across the matrix a, i.e. r = a*v
// (_cmsMAT3eval).
func MAT3Eval(a MAT3, v VEC3) VEC3 {
	return VEC3{
		a[0][VX]*v[VX] + a[0][VY]*v[VY] + a[0][VZ]*v[VZ],
		a[1][VX]*v[VX] + a[1][VY]*v[VY] + a[1][VZ]*v[VZ],
		a[2][VX]*v[VX] + a[2][VY]*v[VY] + a[2][VZ]*v[VZ],
	}
}
