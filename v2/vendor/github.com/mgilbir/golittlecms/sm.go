package lcms2

// sm.go ports src/cmssm.c: the gamut boundary descriptor built with Jan
// Morovic's Segment maxima method (many thanks to Jan, per the C source).
//
//	r     = C*
//	alpha = Hab
//	theta = L*

import "math"

// smSectors mirrors SECTORS: number of divisions in alpha and theta.
const smSectors = 16

// cmsSpherical mirrors the reference spherical-coordinate triple.
type cmsSpherical struct {
	r     float64
	alpha float64
	theta float64
}

// gdbPointType mirrors GDBPointType.
type gdbPointType int

const (
	gpEmpty     gdbPointType = iota // GP_EMPTY
	gpSpecified                     // GP_SPECIFIED
	gpModeled                       // GP_MODELED
)

// cmsGDBPoint mirrors the reference sector cell.
type cmsGDBPoint struct {
	Type gdbPointType
	p    cmsSpherical // Keep also alpha & theta of maximum
}

// cmsGDB mirrors the gamut boundary descriptor handle.
type cmsGDB struct {
	ContextID *Context
	Gamut     [smSectors][smSectors]cmsGDBPoint
}

// smLine is a parametric line P = a + t*u (cmsLine).
type smLine struct {
	a VEC3
	u VEC3
}

// _cmsAtan2 ports the reference helper: atan2 returning positive degrees.
func smAtan2(y, x float64) float64 {
	// Deal with undefined case
	if x == 0.0 && y == 0.0 {
		return 0
	}

	a := (math.Atan2(y, x) * 180.0) / math.Pi

	for a < 0 {
		a += 360
	}
	return a
}

// toSpherical ports ToSpherical.
func toSpherical(sp *cmsSpherical, v *VEC3) {
	L := v[VX]
	a := v[VY]
	b := v[VZ]

	sp.r = math.Sqrt(L*L + a*a + b*b)

	if sp.r == 0 {
		sp.alpha = 0
		sp.theta = 0
		return
	}

	sp.alpha = smAtan2(a, b)
	sp.theta = smAtan2(math.Sqrt(a*a+b*b), L)
}

// toCartesian ports ToCartesian.
func toCartesian(v *VEC3, sp *cmsSpherical) {
	sinAlpha := math.Sin((math.Pi * sp.alpha) / 180.0)
	cosAlpha := math.Cos((math.Pi * sp.alpha) / 180.0)
	sinTheta := math.Sin((math.Pi * sp.theta) / 180.0)
	cosTheta := math.Cos((math.Pi * sp.theta) / 180.0)

	a := sp.r * sinTheta * sinAlpha
	b := sp.r * sinTheta * cosAlpha
	L := sp.r * cosTheta

	v[VX] = L
	v[VY] = a
	v[VZ] = b
}

// quantizeToSector ports QuantizeToSector: saturate 360, 180 to last sector.
func quantizeToSector(sp *cmsSpherical, alpha, theta *int) {
	*alpha = int(math.Floor((sp.alpha * smSectors) / 360.0))
	*theta = int(math.Floor((sp.theta * smSectors) / 180.0))

	if *alpha >= smSectors {
		*alpha = smSectors - 1
	}
	if *theta >= smSectors {
		*theta = smSectors - 1
	}
}

// lineOf2Points ports LineOf2Points.
func lineOf2Points(line *smLine, a, b *VEC3) {
	line.a = VEC3Init(a[VX], a[VY], a[VZ])
	line.u = VEC3Init(b[VX]-a[VX], b[VY]-a[VY], b[VZ]-a[VZ])
}

// getPointOfLine ports GetPointOfLine.
func getPointOfLine(p *VEC3, line *smLine, t float64) {
	p[VX] = line.a[VX] + t*line.u[VX]
	p[VY] = line.a[VY] + t*line.u[VY]
	p[VZ] = line.a[VZ] + t*line.u[VZ]
}

// closestLineToLine ports ClosestLineToLine: closest point on sector line1 to
// sector line2 (both parameterised 0 <= t <= 1). SoftSurfer algorithm 0106.
func closestLineToLine(r *VEC3, line1, line2 *smLine) bool {
	var sc, sN, sD float64
	var tN, tD float64

	w0 := VEC3Minus(line1.a, line2.a)

	a := VEC3Dot(line1.u, line1.u)
	b := VEC3Dot(line1.u, line2.u)
	c := VEC3Dot(line2.u, line2.u)
	d := VEC3Dot(line1.u, w0)
	e := VEC3Dot(line2.u, w0)

	D := a*c - b*b // Denominator
	sD = D         // default sD = D >= 0
	tD = D

	if D < matrixDetTolerance { // the lines are almost parallel
		sN = 0.0 // force using point P0 on segment S1
		sD = 1.0 // to prevent possible division by 0.0 later
		tN = e
		tD = c
	} else { // get the closest points on the infinite lines
		sN = (b*e - c*d)
		tN = (a*e - b*d)

		if sN < 0.0 { // sc < 0 => the s=0 edge is visible
			sN = 0.0
			tN = e
			tD = c
		} else if sN > sD { // sc > 1 => the s=1 edge is visible
			sN = sD
			tN = e + b
			tD = c
		}
	}

	if tN < 0.0 { // tc < 0 => the t=0 edge is visible
		tN = 0.0
		// recompute sc for this edge
		if -d < 0.0 {
			sN = 0.0
		} else if -d > a {
			sN = sD
		} else {
			sN = -d
			sD = a
		}
	} else if tN > tD { // tc > 1 => the t=1 edge is visible
		tN = tD
		// recompute sc for this edge
		if (-d + b) < 0.0 {
			sN = 0
		} else if (-d + b) > a {
			sN = sD
		} else {
			sN = (-d + b)
			sD = a
		}
	}

	// finally do the division to get sc and tc
	if math.Abs(sN) < matrixDetTolerance {
		sc = 0.0
	} else {
		sc = sN / sD
	}

	getPointOfLine(r, line1, sc)
	return true
}

// ---------------------------------------------------------------- Wrapper

// GBDAlloc ports cmsGBDAlloc: allocate a gamut boundary descriptor.
func (ctx *Context) GBDAlloc() *cmsGDB {
	gbd := &cmsGDB{}
	gbd.ContextID = ctx
	return gbd
}

// GBDAlloc ports cmsGBDAlloc on the default context.
func GBDAlloc() *cmsGDB { return defaultContext.GBDAlloc() }

// GBDFree ports cmsGBDFree. Under the Go GC there is nothing to free.
func (gbd *cmsGDB) GBDFree() {}

// getPoint ports GetPoint: retrieve a pointer to the sector containing the Lab
// value, together with its spherical coordinates. Returns nil (and signals an
// error) on out-of-range input, mirroring the reference.
func (gbd *cmsGDB) getPoint(Lab *CIELab, sp *cmsSpherical) *cmsGDBPoint {
	if gbd == nil || Lab == nil || sp == nil {
		return nil
	}

	// Center L* by subtracting half of its domain, that's 50
	v := VEC3Init(Lab.L-50.0, Lab.A, Lab.B)

	// Convert to spherical coordinates
	toSpherical(sp, &v)

	if sp.r < 0 || sp.alpha < 0 || sp.theta < 0 {
		gbd.ContextID.signalError(ErrRange, "spherical value out of range")
		return nil
	}

	// On which sector it falls?
	var alpha, theta int
	quantizeToSector(sp, &alpha, &theta)

	if alpha < 0 || theta < 0 || alpha >= smSectors || theta >= smSectors {
		gbd.ContextID.signalError(ErrRange, " quadrant out of range")
		return nil
	}

	return &gbd.Gamut[theta][alpha]
}

// GDBAddPoint ports cmsGDBAddPoint: add a Lab point to the descriptor. The GBD
// is centered on a=b=0 and L*=50.
func (gbd *cmsGDB) GDBAddPoint(Lab *CIELab) bool {
	var sp cmsSpherical

	ptr := gbd.getPoint(Lab, &sp)
	if ptr == nil {
		return false
	}

	// If no samples at this sector, add it
	if ptr.Type == gpEmpty {
		ptr.Type = gpSpecified
		ptr.p = sp
	} else {
		// Substitute only if radius is greater
		if sp.r > ptr.p.r {
			ptr.Type = gpSpecified
			ptr.p = sp
		}
	}

	return true
}

// GDBCheckPoint ports cmsGDBCheckPoint: is a given Lab point inside the gamut?
func (gbd *cmsGDB) GDBCheckPoint(Lab *CIELab) bool {
	var sp cmsSpherical

	ptr := gbd.getPoint(Lab, &sp)
	if ptr == nil {
		return false
	}

	// If no samples at this sector, return no data
	if ptr.Type == gpEmpty {
		return false
	}

	// In gamut only if radius is greater
	return sp.r <= ptr.p.r
}

// ---------------------------------------------------------------------------

// smSpiral mirrors the Spiral[] relative-movement table for FindNearSectors.
var smSpiral = [...]struct{ AdvX, AdvY int }{
	{0, -1}, {+1, -1}, {+1, 0}, {+1, +1}, {0, +1}, {-1, +1},
	{-1, 0}, {-1, -1}, {-1, -2}, {0, -2}, {+1, -2}, {+2, -2},
	{+2, -1}, {+2, 0}, {+2, +1}, {+2, +2}, {+1, +2}, {0, +2},
	{-1, +2}, {-2, +2}, {-2, +1}, {-2, 0}, {-2, -1}, {-2, -2},
}

const smNSteps = len(smSpiral)

// findNearSectors ports FindNearSectors. The list of near sectors is returned
// via Close[]; the count is returned.
func (gbd *cmsGDB) findNearSectors(alpha, theta int, Close []*cmsGDBPoint) int {
	nSectors := 0

	for i := 0; i < smNSteps; i++ {
		a := alpha + smSpiral[i].AdvX
		t := theta + smSpiral[i].AdvY

		// Cycle at the end
		a %= smSectors
		t %= smSectors

		// Cycle at the begin
		if a < 0 {
			a = smSectors + a
		}
		if t < 0 {
			t = smSectors + t
		}

		pt := &gbd.Gamut[t][a]

		if pt.Type != gpEmpty {
			Close[nSectors] = pt
			nSectors++
		}
	}

	return nSectors
}

// interpolateMissingSector ports InterpolateMissingSector. Identifies whether
// this is a top, bottom, or mid sector and interpolates it.
func (gbd *cmsGDB) interpolateMissingSector(alpha, theta int) bool {
	var sp cmsSpherical
	var Lab VEC3
	var Centre VEC3
	var ray smLine
	var closel, templ cmsSpherical
	var edge smLine

	// Is that point already specified?
	if gbd.Gamut[theta][alpha].Type != gpEmpty {
		return true
	}

	// Fill close points
	var Close [smNSteps + 1]*cmsGDBPoint
	nCloseSectors := gbd.findNearSectors(alpha, theta, Close[:])

	// Find a central point on the sector
	sp.alpha = ((float64(alpha) + 0.5) * 360.0) / smSectors
	sp.theta = ((float64(theta) + 0.5) * 180.0) / smSectors
	sp.r = 50.0

	// Convert to Cartesian
	toCartesian(&Lab, &sp)

	// Create a ray line from centre to this point
	Centre = VEC3Init(50.0, 0, 0)
	lineOf2Points(&ray, &Lab, &Centre)

	// For all close sectors
	closel.r = 0.0
	closel.alpha = 0
	closel.theta = 0

	for k := 0; k < nCloseSectors; k++ {
		for m := k + 1; m < nCloseSectors; m++ {
			var temp, a1, a2 VEC3

			// A line from sector to sector
			toCartesian(&a1, &Close[k].p)
			toCartesian(&a2, &Close[m].p)

			lineOf2Points(&edge, &a1, &a2)

			// Find a line
			closestLineToLine(&temp, &ray, &edge)

			// Convert to spherical
			toSpherical(&templ, &temp)

			if templ.r > closel.r &&
				templ.theta >= (float64(theta)*180.0/smSectors) &&
				templ.theta <= (float64(theta+1)*180.0/smSectors) &&
				templ.alpha >= (float64(alpha)*360.0/smSectors) &&
				templ.alpha <= (float64(alpha+1)*360.0/smSectors) {

				closel = templ
			}
		}
	}

	gbd.Gamut[theta][alpha].p = closel
	gbd.Gamut[theta][alpha].Type = gpModeled

	return true
}

// GDBCompute ports cmsGDBCompute: interpolate missing parts. First computes
// slices at theta=0 and theta=Max, then the middle.
func (gbd *cmsGDB) GDBCompute(dwFlags uint32) bool {
	if gbd == nil {
		return false
	}

	// Interpolate black
	for alpha := 0; alpha < smSectors; alpha++ {
		if !gbd.interpolateMissingSector(alpha, 0) {
			return false
		}
	}

	// Interpolate white
	for alpha := 0; alpha < smSectors; alpha++ {
		if !gbd.interpolateMissingSector(alpha, smSectors-1) {
			return false
		}
	}

	// Interpolate Mid
	for theta := 1; theta < smSectors; theta++ {
		for alpha := 0; alpha < smSectors; alpha++ {
			if !gbd.interpolateMissingSector(alpha, theta) {
				return false
			}
		}
	}

	return true
}
