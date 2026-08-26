package lcms2

// This file ports src/cmsopt.c: pipeline optimization. The optimizers collapse
// a general multi-stage pipeline into a specialized fast evaluator installed via
// (*Pipeline).setOptimizationParameters (the _cmsPipelineSetOptimizationParameters
// hook). (*Context).optimizePipeline is the entry point _cmsOptimizePipeline and
// is wired into the transform in xform.go.
//
// Faithful port notes:
//   - Integer paths are bit-exact with the reference (fixed-point matrix-shaper,
//     tetrahedral prelinearized CLUT). Never "improve" the numerics.
//   - Library code never panics: every parsed/derived index is bounds-checked and
//     every type assertion uses the comma-ok form.

import "math"

// -----------------------------------------------------------------------------
// Fixed-point helpers (DOUBLE_TO_1FIXED14, cmsS1Fixed14Number).
// -----------------------------------------------------------------------------

// s1Fixed14 mirrors cmsS1Fixed14Number: a signed 1.14 fixed-point number held in
// an int32 (the reference notes it may exceed 16 bits).
type s1Fixed14 = int32

// doubleTo1Fixed14 ports DOUBLE_TO_1FIXED14(x): floor(x*16384 + 0.5).
func doubleTo1Fixed14(x float64) s1Fixed14 {
	return s1Fixed14(math.Floor(x*16384.0 + 0.5))
}

// -----------------------------------------------------------------------------
// Optimizer function type and the plug-in family.
// -----------------------------------------------------------------------------

// OptimizeFn mirrors _cmsOPToptimizeFn: an optimizer receives the pipeline by
// double pointer (it may replace it) and may adjust the formats and flags. It
// returns true when it installed an optimized evaluator.
type OptimizeFn func(ctx *Context, lut **Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) bool

// PluginOptimization mirrors cmsPluginOptimization: registers a new optimizer.
// Register it through (*Context).RegisterPlugins; the newest registered
// optimizer is consulted first, before the built-in collection.
type PluginOptimization struct {
	PluginBase
	OptimizePtr OptimizeFn
}

// defaultOptimizations is the built-in list (DefaultOptimization), consulted in
// order after any registered plug-ins: joining of curves, matrix-shaper,
// linearization and resampling.
var defaultOptimizations = []OptimizeFn{
	optimizeByJoiningCurves,
	optimizeMatrixShaper,
	optimizeByComputingLinearization,
	optimizeByResampling,
}

// -----------------------------------------------------------------------------
// Optimized evaluator data structures and their evaluators.
// -----------------------------------------------------------------------------

// prelin8Data ports Prelin8Data: precomputed nodes/offsets for 8-bit input,
// tetrahedral Shaper-CLUT (3 inputs only).
type prelin8Data struct {
	ctx *Context
	p   *InterpParams // tetrahedral interpolation params (not owned)

	rx, ry, rz [256]uint16
	x0, y0, z0 [256]uint32
}

// prelin16Data ports Prelin16Data: the fused prelinearization + CLUT +
// postlinearization evaluator for any number of channels. A nil params entry
// means an identity (nop) 1D stage.
type prelin16Data struct {
	ctx      *Context
	nInputs  uint32
	nOutputs uint32

	paramsCurveIn  [maxInputDimensions]*InterpParams
	clutParams     *InterpParams
	paramsCurveOut []*InterpParams
}

// matShaper8Data ports MatShaper8Data: matrix-shaper in 8 bits. Numbers are
// operated in n.14 signed; tables are stored in 1.14 fixed.
type matShaper8Data struct {
	ctx *Context

	shaper1R [256]s1Fixed14 // 0..255 -> 1.14
	shaper1G [256]s1Fixed14
	shaper1B [256]s1Fixed14

	mat [3][3]s1Fixed14 // n.14 -> n.14 (saturated afterwards)
	off [3]s1Fixed14

	shaper2R [16385]uint16 // 1.14 -> 0..255 (or 16-bit)
	shaper2G [16385]uint16
	shaper2B [16385]uint16
}

// curves16Data ports Curves16Data: a curves-only optimization shared between 8
// and 16 bits.
type curves16Data struct {
	ctx       *Context
	nCurves   uint32
	nElements uint32
	curves    [][]uint16
}

// prelinEval16 ports PrelinEval16.
func prelinEval16(in, out []uint16, d any) {
	p16, ok := d.(*prelin16Data)
	if !ok {
		return
	}

	var stageABC [maxInputDimensions]uint16
	var stageDEF [maxChannels]uint16

	for i := uint32(0); i < p16.nInputs; i++ {
		if p := p16.paramsCurveIn[i]; p != nil && p.Interpolation.Lerp16 != nil {
			p.Interpolation.Lerp16(in[i:i+1], stageABC[i:i+1], p)
		} else {
			stageABC[i] = in[i]
		}
	}

	if p := p16.clutParams; p != nil && p.Interpolation.Lerp16 != nil {
		p.Interpolation.Lerp16(stageABC[:], stageDEF[:], p)
	}

	for i := uint32(0); i < p16.nOutputs; i++ {
		if i >= uint32(len(p16.paramsCurveOut)) {
			break
		}
		if p := p16.paramsCurveOut[i]; p != nil && p.Interpolation.Lerp16 != nil {
			p.Interpolation.Lerp16(stageDEF[i:i+1], out[i:i+1], p)
		} else {
			out[i] = stageDEF[i]
		}
	}
}

// dens ports the DENS(i,j,k) macro of PrelinEval8: LutTable[i+j+k+OutChan].
func dens(lut []uint16, i, j, k, oc int32) int32 {
	return int32(lut[i+j+k+oc])
}

// prelinEval8 ports PrelinEval8: an optimized tetrahedral interpolation for
// 8-bit input. Bit-exact fixed-point arithmetic.
func prelinEval8(in, out []uint16, d any) {
	p8, ok := d.(*prelin8Data)
	if !ok {
		return
	}
	p := p8.p
	totalOut := int(p.NumOutputs)
	lutTable := p.table16

	r := uint8(in[0] >> 8)
	g := uint8(in[1] >> 8)
	b := uint8(in[2] >> 8)

	x0 := int32(p8.x0[r])
	y0 := int32(p8.y0[g])
	z0 := int32(p8.z0[b])

	rx := int32(p8.rx[r])
	ry := int32(p8.ry[g])
	rz := int32(p8.rz[b])

	var x1, y1, z1 int32
	if rx == 0 {
		x1 = x0
	} else {
		x1 = x0 + int32(p.Opta[2])
	}
	if ry == 0 {
		y1 = y0
	} else {
		y1 = y0 + int32(p.Opta[1])
	}
	if rz == 0 {
		z1 = z0
	} else {
		z1 = z0 + int32(p.Opta[0])
	}

	// Single bounds check covering every DENS access (all indices are within
	// [x0+y0+z0, x1+y1+z1] plus OutChan < totalOut). Keeps the library
	// panic-free on a malformed grid without per-access checks.
	if x0 < 0 || y0 < 0 || z0 < 0 || int(x1+y1+z1)+totalOut > len(lutTable) {
		for oc := 0; oc < totalOut && oc < len(out); oc++ {
			out[oc] = 0
		}
		return
	}

	for outChan := int32(0); outChan < int32(totalOut); outChan++ {
		c0 := dens(lutTable, x0, y0, z0, outChan)
		var c1, c2, c3 int32

		switch {
		case rx >= ry && ry >= rz:
			c1 = dens(lutTable, x1, y0, z0, outChan) - c0
			c2 = dens(lutTable, x1, y1, z0, outChan) - dens(lutTable, x1, y0, z0, outChan)
			c3 = dens(lutTable, x1, y1, z1, outChan) - dens(lutTable, x1, y1, z0, outChan)
		case rx >= rz && rz >= ry:
			c1 = dens(lutTable, x1, y0, z0, outChan) - c0
			c2 = dens(lutTable, x1, y1, z1, outChan) - dens(lutTable, x1, y0, z1, outChan)
			c3 = dens(lutTable, x1, y0, z1, outChan) - dens(lutTable, x1, y0, z0, outChan)
		case rz >= rx && rx >= ry:
			c1 = dens(lutTable, x1, y0, z1, outChan) - dens(lutTable, x0, y0, z1, outChan)
			c2 = dens(lutTable, x1, y1, z1, outChan) - dens(lutTable, x1, y0, z1, outChan)
			c3 = dens(lutTable, x0, y0, z1, outChan) - c0
		case ry >= rx && rx >= rz:
			c1 = dens(lutTable, x1, y1, z0, outChan) - dens(lutTable, x0, y1, z0, outChan)
			c2 = dens(lutTable, x0, y1, z0, outChan) - c0
			c3 = dens(lutTable, x1, y1, z1, outChan) - dens(lutTable, x1, y1, z0, outChan)
		case ry >= rz && rz >= rx:
			c1 = dens(lutTable, x1, y1, z1, outChan) - dens(lutTable, x0, y1, z1, outChan)
			c2 = dens(lutTable, x0, y1, z0, outChan) - c0
			c3 = dens(lutTable, x0, y1, z1, outChan) - dens(lutTable, x0, y1, z0, outChan)
		case rz >= ry && ry >= rx:
			c1 = dens(lutTable, x1, y1, z1, outChan) - dens(lutTable, x0, y1, z1, outChan)
			c2 = dens(lutTable, x0, y1, z1, outChan) - dens(lutTable, x0, y0, z1, outChan)
			c3 = dens(lutTable, x0, y0, z1, outChan) - c0
		default:
			c1, c2, c3 = 0, 0, 0
		}

		rest := c1*rx + c2*ry + c3*rz + 0x8001
		out[outChan] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
	}
}

// matShaperEval16 ports MatShaperEval16: a fast matrix-shaper evaluator for 8
// bits using 1.14 signed fixed point. Bit-exact and allocation-free.
func matShaperEval16(in, out []uint16, d any) {
	p, ok := d.(*matShaper8Data)
	if !ok {
		return
	}

	// In[] is assured to come from an 8-bit number (a<<8 | a).
	ri := in[0] & 0xFF
	gi := in[1] & 0xFF
	bi := in[2] & 0xFF

	r := p.shaper1R[ri]
	g := p.shaper1G[gi]
	b := p.shaper1B[bi]

	l1 := (p.mat[0][0]*r + p.mat[0][1]*g + p.mat[0][2]*b + p.off[0] + 0x2000) >> 14
	l2 := (p.mat[1][0]*r + p.mat[1][1]*g + p.mat[1][2]*b + p.off[1] + 0x2000) >> 14
	l3 := (p.mat[2][0]*r + p.mat[2][1]*g + p.mat[2][2]*b + p.off[2] + 0x2000) >> 14

	clip := func(l s1Fixed14) uint32 {
		if l < 0 {
			return 0
		}
		if l > 16384 {
			return 16384
		}
		return uint32(l)
	}

	out[0] = p.shaper2R[clip(l1)]
	out[1] = p.shaper2G[clip(l2)]
	out[2] = p.shaper2B[clip(l3)]
}

// fastEvaluateCurves8 ports FastEvaluateCurves8.
func fastEvaluateCurves8(in, out []uint16, d any) {
	data, ok := d.(*curves16Data)
	if !ok {
		return
	}
	for i := uint32(0); i < data.nCurves; i++ {
		x := in[i] >> 8
		out[i] = data.curves[i][x]
	}
}

// fastEvaluateCurves16 ports FastEvaluateCurves16.
func fastEvaluateCurves16(in, out []uint16, d any) {
	data, ok := d.(*curves16Data)
	if !ok {
		return
	}
	for i := uint32(0); i < data.nCurves; i++ {
		out[i] = data.curves[i][in[i]]
	}
}

// fastIdentity16 ports FastIdentity16.
func fastIdentity16(in, out []uint16, d any) {
	lut, ok := d.(*Pipeline)
	if !ok {
		return
	}
	for i := uint32(0); i < lut.InputChannels; i++ {
		out[i] = in[i]
	}
}

// clutEval16 ports the direct-CLUT evaluator installed when a resampled
// pipeline has no pre/post curves: it simply runs the CLUT's own Lerp16.
func clutEval16(in, out []uint16, d any) {
	p, ok := d.(*InterpParams)
	if !ok || p.Interpolation.Lerp16 == nil {
		return
	}
	p.Interpolation.Lerp16(in, out, p)
}

// -----------------------------------------------------------------------------
// Dup helpers. The Go GC replaces C free/dup; because the private data is
// immutable after construction, dup shares the same pointer, which is safe.
// A dupFn is still required so (*Pipeline).Dup keeps the specialized evaluator
// attached to its private data instead of falling back to the pipeline itself.
// -----------------------------------------------------------------------------

func shareDataDup(_ *Context, data any) any { return data }

// -----------------------------------------------------------------------------
// prelin allocators.
// -----------------------------------------------------------------------------

// prelinOpt16alloc ports PrelinOpt16alloc.
func prelinOpt16alloc(ctx *Context, colorMap *InterpParams,
	nInputs uint32, in []*ToneCurve, nOutputs uint32, out []*ToneCurve) *prelin16Data {

	p16 := &prelin16Data{
		ctx:            ctx,
		nInputs:        nInputs,
		nOutputs:       nOutputs,
		clutParams:     colorMap,
		paramsCurveOut: make([]*InterpParams, nOutputs),
	}

	for i := uint32(0); i < nInputs && i < maxInputDimensions; i++ {
		if in == nil {
			p16.paramsCurveIn[i] = nil
		} else {
			p16.paramsCurveIn[i] = in[i].interp
		}
	}

	for i := uint32(0); i < nOutputs; i++ {
		if out == nil {
			p16.paramsCurveOut[i] = nil
		} else {
			p16.paramsCurveOut[i] = out[i].interp
		}
	}

	return p16
}

// prelinOpt8alloc ports PrelinOpt8alloc: precompute tables for 8-bit input.
func prelinOpt8alloc(ctx *Context, p *InterpParams, g []*ToneCurve) *prelin8Data {
	p8 := &prelin8Data{ctx: ctx, p: p}

	var input [3]uint16
	for i := 0; i < 256; i++ {
		if g != nil {
			input[0] = g[0].Eval16(from8to16(uint8(i)))
			input[1] = g[1].Eval16(from8to16(uint8(i)))
			input[2] = g[2].Eval16(from8to16(uint8(i)))
		} else {
			input[0] = from8to16(uint8(i))
			input[1] = from8to16(uint8(i))
			input[2] = from8to16(uint8(i))
		}

		v1 := toFixedDomain(int(uint32(input[0]) * p.Domain[0]))
		v2 := toFixedDomain(int(uint32(input[1]) * p.Domain[1]))
		v3 := toFixedDomain(int(uint32(input[2]) * p.Domain[2]))

		p8.x0[i] = p.Opta[2] * uint32(fixedToInt(v1))
		p8.y0[i] = p.Opta[1] * uint32(fixedToInt(v2))
		p8.z0[i] = p.Opta[0] * uint32(fixedToInt(v3))

		p8.rx[i] = uint16(fixedRestToInt(v1))
		p8.ry[i] = uint16(fixedRestToInt(v2))
		p8.rz[i] = uint16(fixedRestToInt(v3))
	}

	return p8
}

// curvesAlloc ports CurvesAlloc.
func curvesAlloc(ctx *Context, nCurves, nElements uint32, g []*ToneCurve) *curves16Data {
	c16 := &curves16Data{
		ctx:       ctx,
		nCurves:   nCurves,
		nElements: nElements,
		curves:    make([][]uint16, nCurves),
	}

	for i := uint32(0); i < nCurves; i++ {
		c16.curves[i] = make([]uint16, nElements)
		if nElements == 256 {
			for j := uint32(0); j < nElements; j++ {
				c16.curves[i][j] = g[i].Eval16(from8to16(uint8(j)))
			}
		} else {
			for j := uint32(0); j < nElements; j++ {
				c16.curves[i][j] = g[i].Eval16(uint16(j))
			}
		}
	}

	return c16
}

// -----------------------------------------------------------------------------
// Simple optimizations: identity/no-op removal and matrix merging.
// -----------------------------------------------------------------------------

// removeElement ports _RemoveElement: drop the stage at *head.
func removeElement(head **Stage) {
	mpe := *head
	*head = mpe.next
}

// remove1Op ports _Remove1Op: remove all stages implementing UnaryOp.
func remove1Op(lut *Pipeline, unaryOp StageSignature) bool {
	pt := &lut.elements
	anyOpt := false
	for *pt != nil {
		if (*pt).Implements == unaryOp {
			removeElement(pt)
			anyOpt = true
		} else {
			pt = &((*pt).next)
		}
	}
	return anyOpt
}

// remove2Op ports _Remove2Op: remove adjacent Op1,Op2 pairs.
func remove2Op(lut *Pipeline, op1, op2 StageSignature) bool {
	anyOpt := false
	pt1 := &lut.elements
	if *pt1 == nil {
		return anyOpt
	}
	for *pt1 != nil {
		pt2 := &((*pt1).next)
		if *pt2 == nil {
			return anyOpt
		}
		if (*pt1).Implements == op1 && (*pt2).Implements == op2 {
			removeElement(pt2)
			removeElement(pt1)
			anyOpt = true
		} else {
			pt1 = &((*pt1).next)
		}
	}
	return anyOpt
}

// closeEnoughFloat ports CloseEnoughFloat.
func closeEnoughFloat(a, b float64) bool {
	return math.Abs(b-a) < 0.00001
}

// isFloatMatrixIdentity ports isFloatMatrixIdentity (a looser tolerance than
// _cmsMAT3isIdentity).
func isFloatMatrixIdentity(a MAT3) bool {
	identity := MAT3Identity()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if !closeEnoughFloat(a[i][j], identity[i][j]) {
				return false
			}
		}
	}
	return true
}

// mat3FromRowMajor builds a MAT3 from a row-major 3x3 stageMatrixData.double.
func mat3FromRowMajor(d []float64) MAT3 {
	return MAT3{
		{d[0], d[1], d[2]},
		{d[3], d[4], d[5]},
		{d[6], d[7], d[8]},
	}
}

// mat3ToRowMajor flattens a MAT3 into a row-major 9-element slice.
func mat3ToRowMajor(m MAT3) []float64 {
	return []float64{
		m[0][0], m[0][1], m[0][2],
		m[1][0], m[1][1], m[1][2],
		m[2][0], m[2][1], m[2][2],
	}
}

// multiplyMatrix ports _MultiplyMatrix: fuse adjacent matrix stages.
func multiplyMatrix(lut *Pipeline) bool {
	anyOpt := false
	pt1 := &lut.elements
	if *pt1 == nil {
		return anyOpt
	}
	for *pt1 != nil {
		pt2 := &((*pt1).next)
		if *pt2 == nil {
			return anyOpt
		}
		if (*pt1).Implements == SigMatrixElemType && (*pt2).Implements == SigMatrixElemType {
			m1, ok1 := (*pt1).data.(*stageMatrixData)
			m2, ok2 := (*pt2).data.(*stageMatrixData)
			if !ok1 || !ok2 {
				return anyOpt
			}

			if m1.offset != nil || m2.offset != nil ||
				(*pt1).InputChannels != 3 || (*pt1).OutputChannels != 3 ||
				(*pt2).InputChannels != 3 || (*pt2).OutputChannels != 3 {
				return false
			}

			res := MAT3Per(mat3FromRowMajor(m2.double), mat3FromRowMajor(m1.double))

			chain := (*pt2).next
			removeElement(pt2)
			removeElement(pt1)

			if !isFloatMatrixIdentity(res) {
				multmat, err := lut.ctx.StageAllocMatrix(3, 3, mat3ToRowMajor(res), nil)
				if err != nil || multmat == nil {
					return false
				}
				multmat.next = chain
				*pt1 = multmat
			}

			anyOpt = true
		} else {
			pt1 = &((*pt1).next)
		}
	}
	return anyOpt
}

// preOptimize ports PreOptimize: remove paired no-ops and merge matrices.
func preOptimize(lut *Pipeline) bool {
	anyOpt := false
	for {
		opt := false
		opt = remove1Op(lut, SigIdentityElemType) || opt
		opt = remove2Op(lut, SigXYZ2LabElemType, SigLab2XYZElemType) || opt
		opt = remove2Op(lut, SigLab2XYZElemType, SigXYZ2LabElemType) || opt
		opt = remove2Op(lut, SigLabV4toV2, SigLabV2toV4) || opt
		opt = remove2Op(lut, SigLabV2toV4, SigLabV4toV2) || opt
		opt = remove2Op(lut, SigLab2FloatPCS, SigFloatPCS2Lab) || opt
		opt = remove2Op(lut, SigXYZ2FloatPCS, SigFloatPCS2XYZ) || opt
		opt = multiplyMatrix(lut) || opt

		if opt {
			anyOpt = true
		} else {
			break
		}
	}
	return anyOpt
}

// -----------------------------------------------------------------------------
// Resampling helpers.
// -----------------------------------------------------------------------------

const prelinearizationPoints = 4096

// xformSampler16 ports XFormSampler16: sample a pipeline at a grid knot,
// evaluated in floating point then converted to 16 bits.
func xformSampler16(in, out []uint16, cargo any) bool {
	lut, ok := cargo.(*Pipeline)
	if !ok {
		return false
	}
	if lut.InputChannels >= maxChannels || lut.OutputChannels >= maxChannels {
		return false
	}

	var inFloat, outFloat [maxChannels]float32
	for i := uint32(0); i < lut.InputChannels; i++ {
		inFloat[i] = float32(float64(in[i]) / 65535.0)
	}

	lut.EvalFloat(inFloat[:], outFloat[:])

	for i := uint32(0); i < lut.OutputChannels; i++ {
		out[i] = quickSaturateWord(float64(outFloat[i]) * 65535.0)
	}
	return true
}

// allCurvesAreLinear ports AllCurvesAreLinear.
func allCurvesAreLinear(mpe *Stage) bool {
	curves := mpe.GetToneCurves()
	if curves == nil {
		return false
	}
	n := mpe.OutputChannels
	for i := uint32(0); i < n; i++ {
		if !curves[i].IsLinear() {
			return false
		}
	}
	return true
}

// patchLUT ports PatchLUT: replace the node at At[] with Value[]. Works on 1, 3
// and 4 input channels. Returns false when At[] is not on an exact node.
func patchLUT(clut *Stage, at, value []uint16, nChannelsOut, nChannelsIn uint32) bool {
	grid, ok := clut.data.(*stageCLutData)
	if !ok {
		return false
	}
	if clut.Type != SigCLutElemType {
		return false
	}
	p16 := grid.params

	var index int

	floorExact := func(v uint16, dom uint32) (int, bool) {
		p := (float64(v) * float64(dom)) / 65535.0
		f := int(math.Floor(p))
		if p-float64(f) != 0 {
			return 0, false
		}
		return f, true
	}

	switch nChannelsIn {
	case 4:
		x0, okx := floorExact(at[0], p16.Domain[0])
		y0, oky := floorExact(at[1], p16.Domain[1])
		z0, okz := floorExact(at[2], p16.Domain[2])
		w0, okw := floorExact(at[3], p16.Domain[3])
		if !okx || !oky || !okz || !okw {
			return false
		}
		index = int(p16.Opta[3])*x0 + int(p16.Opta[2])*y0 + int(p16.Opta[1])*z0 + int(p16.Opta[0])*w0
	case 3:
		x0, okx := floorExact(at[0], p16.Domain[0])
		y0, oky := floorExact(at[1], p16.Domain[1])
		z0, okz := floorExact(at[2], p16.Domain[2])
		if !okx || !oky || !okz {
			return false
		}
		index = int(p16.Opta[2])*x0 + int(p16.Opta[1])*y0 + int(p16.Opta[0])*z0
	case 1:
		x0, okx := floorExact(at[0], p16.Domain[0])
		if !okx {
			return false
		}
		index = int(p16.Opta[0]) * x0
	default:
		return false
	}

	for i := 0; i < int(nChannelsOut); i++ {
		if index+i < 0 || index+i >= len(grid.tab16) {
			return false
		}
		grid.tab16[index+i] = value[i]
	}
	return true
}

// whitesAreEqual ports WhitesAreEqual.
func whitesAreEqual(n uint32, white1, white2 []uint16) bool {
	for i := uint32(0); i < n; i++ {
		d := int(white1[i]) - int(white2[i])
		if d < 0 {
			d = -d
		}
		if d > 0xf000 {
			return true // extremely different; fixup avoided
		}
		if white1[i] != white2[i] {
			return false
		}
	}
	return true
}

// fixWhiteMisalignment ports FixWhiteMisalignment.
func fixWhiteMisalignment(lut *Pipeline, entry, exit ColorSpaceSignature) bool {
	whitePointIn, _, nIns, ok := EndPointsBySpace(entry)
	if !ok {
		return false
	}
	whitePointOut, _, nOuts, ok := EndPointsBySpace(exit)
	if !ok {
		return false
	}

	if lut.InputChannels != nIns || lut.OutputChannels != nOuts {
		return false
	}

	var obtainedOut [maxChannels]uint16
	lut.Eval16(whitePointIn, obtainedOut[:])

	if whitesAreEqual(nOuts, whitePointOut, obtainedOut[:]) {
		return true // whites already match
	}

	var preLin, clut, postLin *Stage
	if stages, okp := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType); okp {
		preLin, clut, postLin = stages[0], stages[1], stages[2]
	} else if stages, okp := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigCLutElemType); okp {
		preLin, clut = stages[0], stages[1]
	} else if stages, okp := lut.CheckAndRetrieveStages(SigCLutElemType, SigCurveSetElemType); okp {
		clut, postLin = stages[0], stages[1]
	} else if stages, okp := lut.CheckAndRetrieveStages(SigCLutElemType); okp {
		clut = stages[0]
	} else {
		return false
	}

	var whiteIn, whiteOut [maxChannels]uint16

	if preLin != nil {
		curves := preLin.GetToneCurves()
		for i := uint32(0); i < nIns; i++ {
			whiteIn[i] = curves[i].Eval16(whitePointIn[i])
		}
	} else {
		for i := uint32(0); i < nIns; i++ {
			whiteIn[i] = whitePointIn[i]
		}
	}

	if postLin != nil {
		curves := postLin.GetToneCurves()
		for i := uint32(0); i < nOuts; i++ {
			inverse, err := curves[i].Reverse()
			if err != nil || inverse == nil {
				whiteOut[i] = whitePointOut[i]
			} else {
				whiteOut[i] = inverse.Eval16(whitePointOut[i])
			}
		}
	} else {
		for i := uint32(0); i < nOuts; i++ {
			whiteOut[i] = whitePointOut[i]
		}
	}

	patchLUT(clut, whiteIn[:], whiteOut[:], nOuts, nIns)
	return true
}

// -----------------------------------------------------------------------------
// OptimizeByResampling.
// -----------------------------------------------------------------------------

// optimizeByResampling ports OptimizeByResampling: build a simple prelin/CLUT/
// postlin LUT by sampling any pipeline into a 16-bit grid.
func optimizeByResampling(ctx *Context, lutPtr **Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) bool {

	// Lossy optimization: does not apply to floating point.
	if FormatterIsFloat(*inputFormat) || FormatterIsFloat(*outputFormat) {
		return false
	}

	colorSpace := ICCcolorSpace(int(tColorspace(*inputFormat)))
	outputColorSpace := ICCcolorSpace(int(tColorspace(*outputFormat)))
	if colorSpace == 0 || outputColorSpace == 0 {
		return false
	}

	src := *lutPtr

	var nGridPoints uint32
	if src.StageCount() == 0 {
		nGridPoints = 2
	} else {
		nGridPoints = ReasonableGridpointsByColorspace(colorSpace, *dwFlags)
		// Lab16 as input cannot be optimized by a CLUT due to centering issues.
		if *dwFlags&FlagsForceCLUT == 0 && colorSpace == SigLabData && tBytes(*inputFormat) == 2 {
			return false
		}
	}

	dest, err := ctx.PipelineAlloc(src.InputChannels, src.OutputChannels)
	if err != nil || dest == nil {
		return false
	}

	var newPreLin, newPostLin *Stage
	var keepPreLin, keepPostLin *Stage

	restore := func() {
		if keepPreLin != nil {
			_ = src.InsertStage(AtBegin, keepPreLin)
		}
		if keepPostLin != nil {
			_ = src.InsertStage(AtEnd, keepPostLin)
		}
	}

	// Prelinearization tables kept unless flags say otherwise.
	if *dwFlags&FlagsCLUTPreLinearization != 0 {
		preLin := src.GetPtrToFirstStage()
		if preLin != nil && preLin.Type == SigCurveSetElemType {
			if !allCurvesAreLinear(preLin) {
				dup, derr := preLin.stageDup()
				if derr != nil || dup == nil {
					return false
				}
				newPreLin = dup
				if ierr := dest.InsertStage(AtBegin, newPreLin); ierr != nil {
					return false
				}
				keepPreLin = src.UnlinkStage(AtBegin)
			}
		}
	}

	clut, err := ctx.StageAllocCLut16bit(nGridPoints, src.InputChannels, src.OutputChannels, nil)
	if err != nil || clut == nil {
		restore()
		return false
	}
	if ierr := dest.InsertStage(AtEnd, clut); ierr != nil {
		restore()
		return false
	}

	// Postlinearization tables kept unless flags say otherwise.
	if *dwFlags&FlagsCLUTPostLinearization != 0 {
		postLin := src.GetPtrToLastStage()
		if postLin != nil && postLin.StageType() == SigCurveSetElemType {
			if !allCurvesAreLinear(postLin) {
				dup, derr := postLin.stageDup()
				if derr != nil || dup == nil {
					restore()
					return false
				}
				newPostLin = dup
				if ierr := dest.InsertStage(AtEnd, newPostLin); ierr != nil {
					restore()
					return false
				}
				keepPostLin = src.UnlinkStage(AtEnd)
			}
		}
	}

	// Sample. Src carries no pre/post curves now.
	if !clut.SampleCLut16bit(xformSampler16, src, 0) {
		restore()
		return false
	}

	dataCLUT, ok := clut.data.(*stageCLutData)
	if !ok {
		// Every other bail-out in this function restores the source pipeline's
		// unlinked pre/post-lin stages; this one must too, or the caller keeps
		// using a source pipeline stripped of its prelinearization.
		restore()
		return false
	}

	var dataSetIn, dataSetOut []*ToneCurve
	if newPreLin != nil {
		if d, okp := newPreLin.data.(*stageToneCurvesData); okp {
			dataSetIn = d.theCurves
		}
	}
	if newPostLin != nil {
		if d, okp := newPostLin.data.(*stageToneCurvesData); okp {
			dataSetOut = d.theCurves
		}
	}

	if dataSetIn == nil && dataSetOut == nil {
		dest.setOptimizationParameters(clutEval16, dataCLUT.params, nil, shareDataDup)
	} else {
		p16 := prelinOpt16alloc(dest.ctx, dataCLUT.params,
			dest.InputChannels, dataSetIn, dest.OutputChannels, dataSetOut)
		dest.setOptimizationParameters(prelinEval16, p16, nil, shareDataDup)
	}

	// Don't fix white on absolute colorimetric.
	if intent == IntentAbsoluteColorimetric {
		*dwFlags |= FlagsNoWhiteOnWhiteFixup
	}
	if *dwFlags&FlagsNoWhiteOnWhiteFixup == 0 {
		fixWhiteMisalignment(dest, colorSpace, outputColorSpace)
	}

	*lutPtr = dest
	return true
}

// -----------------------------------------------------------------------------
// OptimizeByComputingLinearization (RGB prelinearization + CLUT).
// -----------------------------------------------------------------------------

// slopeLimiting ports SlopeLimiting: normalize endpoints by slope-limiting the
// first/last 2% of the curve in place.
func slopeLimiting(g *ToneCurve) {
	atBegin := int(math.Floor(float64(g.nEntries)*0.02 + 0.5)) // 2%
	atEnd := int(g.nEntries) - atBegin - 1                     // 98%
	if atBegin <= 0 || atEnd < 0 || atEnd >= int(g.nEntries) {
		return
	}

	var beginVal, endVal int
	if g.IsDescending() {
		beginVal, endVal = 0xffff, 0
	} else {
		beginVal, endVal = 0, 0xffff
	}

	val := float64(g.table16[atBegin])
	slope := (val - float64(beginVal)) / float64(atBegin)
	beta := val - slope*float64(atBegin)
	for i := 0; i < atBegin; i++ {
		g.table16[i] = quickSaturateWord(float64(i)*slope + beta)
	}

	val = float64(g.table16[atEnd])
	slope = (float64(endVal) - val) / float64(atBegin)
	beta = val - slope*float64(atEnd)
	for i := atEnd; i < int(g.nEntries); i++ {
		g.table16[i] = quickSaturateWord(float64(i)*slope + beta)
	}
}

// isDegenerated ports IsDegenerated: curves with wide empty areas are not
// optimizable.
func isDegenerated(g *ToneCurve) bool {
	var zeros, poles uint32
	n := g.nEntries
	for i := uint32(0); i < n; i++ {
		if g.table16[i] == 0x0000 {
			zeros++
		}
		if g.table16[i] == 0xffff {
			poles++
		}
	}
	if zeros == 1 && poles == 1 {
		return false // linear tables
	}
	if zeros > (n / 20) {
		return true
	}
	if poles > (n / 20) {
		return true
	}
	return false
}

// optimizeByComputingLinearization ports OptimizeByComputingLinearization.
func optimizeByComputingLinearization(ctx *Context, lutPtr **Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) bool {

	if FormatterIsFloat(*inputFormat) || FormatterIsFloat(*outputFormat) {
		return false
	}

	// Only on chunky RGB.
	if tColorspace(*inputFormat) != PTRGB || tPlanar(*inputFormat) != 0 {
		return false
	}
	if tColorspace(*outputFormat) != PTRGB || tPlanar(*outputFormat) != 0 {
		return false
	}

	// On 16 bits, the user has to specify the feature.
	if !FormatterIs8bit(*inputFormat) {
		if *dwFlags&FlagsCLUTPreLinearization == 0 {
			return false
		}
	}

	originalLut := *lutPtr

	colorSpace := ICCcolorSpace(int(tColorspace(*inputFormat)))
	outputColorSpace := ICCcolorSpace(int(tColorspace(*outputFormat)))
	if colorSpace == 0 || outputColorSpace == 0 {
		return false
	}

	nGridPoints := ReasonableGridpointsByColorspace(colorSpace, *dwFlags)
	nIn := originalLut.InputChannels

	// If the last stage is degenerate curves, we cannot optimize.
	last := originalLut.GetPtrToLastStage()
	if last == nil {
		return false
	}
	if last.StageType() == SigCurveSetElemType {
		if d, ok := last.data.(*stageToneCurvesData); ok {
			for i := uint32(0); i < d.nCurves; i++ {
				if isDegenerated(d.theCurves[i]) {
					return false
				}
			}
		}
	}

	trans := make([]*ToneCurve, nIn)
	for t := uint32(0); t < nIn; t++ {
		c, err := ctx.BuildTabulatedToneCurve16(make([]uint16, prelinearizationPoints))
		if err != nil || c == nil {
			return false
		}
		trans[t] = c
	}

	var in, out [maxChannels]float32
	for i := 0; i < prelinearizationPoints; i++ {
		v := float32(float64(i) / float64(prelinearizationPoints-1))
		for t := uint32(0); t < nIn; t++ {
			in[t] = v
		}
		originalLut.EvalFloat(in[:], out[:])
		for t := uint32(0); t < nIn; t++ {
			if trans[t].table16 != nil {
				trans[t].table16[i] = quickSaturateWord(float64(out[t]) * 65535.0)
			}
		}
	}

	for t := uint32(0); t < nIn; t++ {
		slopeLimiting(trans[t])
	}

	lIsSuitable := true
	for t := uint32(0); lIsSuitable && t < nIn; t++ {
		if !trans[t].IsMonotonic() {
			lIsSuitable = false
		}
		if isDegenerated(trans[t]) {
			lIsSuitable = false
		}
	}
	if !lIsSuitable {
		return false
	}

	transReverse := make([]*ToneCurve, nIn)
	for t := uint32(0); t < nIn; t++ {
		rev, err := trans[t].ReverseEx(prelinearizationPoints)
		if err != nil || rev == nil {
			return false
		}
		transReverse[t] = rev
	}

	lutPlusCurves, err := originalLut.Dup()
	if err != nil || lutPlusCurves == nil {
		return false
	}
	revStage, err := ctx.StageAllocToneCurves(nIn, transReverse)
	if err != nil || revStage == nil {
		return false
	}
	if err := lutPlusCurves.InsertStage(AtBegin, revStage); err != nil {
		return false
	}

	optimizedLUT, err := ctx.PipelineAlloc(originalLut.InputChannels, originalLut.OutputChannels)
	if err != nil || optimizedLUT == nil {
		return false
	}
	optimizedPrelinMpe, err := ctx.StageAllocToneCurves(nIn, trans)
	if err != nil || optimizedPrelinMpe == nil {
		return false
	}
	if err := optimizedLUT.InsertStage(AtBegin, optimizedPrelinMpe); err != nil {
		return false
	}
	optimizedCLUTmpe, err := ctx.StageAllocCLut16bit(nGridPoints, originalLut.InputChannels, originalLut.OutputChannels, nil)
	if err != nil || optimizedCLUTmpe == nil {
		return false
	}
	if err := optimizedLUT.InsertStage(AtEnd, optimizedCLUTmpe); err != nil {
		return false
	}
	if !optimizedCLUTmpe.SampleCLut16bit(xformSampler16, lutPlusCurves, 0) {
		return false
	}

	optimizedPrelinCurves := optimizedPrelinMpe.GetToneCurves()
	optimizedPrelinCLUT, ok := optimizedCLUTmpe.data.(*stageCLutData)
	if !ok {
		return false
	}

	if FormatterIs8bit(*inputFormat) {
		p8 := prelinOpt8alloc(optimizedLUT.ctx, optimizedPrelinCLUT.params, optimizedPrelinCurves)
		if p8 == nil {
			return false
		}
		optimizedLUT.setOptimizationParameters(prelinEval8, p8, nil, shareDataDup)
	} else {
		p16 := prelinOpt16alloc(optimizedLUT.ctx, optimizedPrelinCLUT.params, 3, optimizedPrelinCurves, 3, nil)
		if p16 == nil {
			return false
		}
		optimizedLUT.setOptimizationParameters(prelinEval16, p16, nil, shareDataDup)
	}

	if intent == IntentAbsoluteColorimetric {
		*dwFlags |= FlagsNoWhiteOnWhiteFixup
	}
	if *dwFlags&FlagsNoWhiteOnWhiteFixup == 0 {
		if !fixWhiteMisalignment(optimizedLUT, colorSpace, outputColorSpace) {
			return false
		}
	}

	*lutPtr = optimizedLUT
	return true
}

// -----------------------------------------------------------------------------
// OptimizeByJoiningCurves.
// -----------------------------------------------------------------------------

// optimizeByJoiningCurves ports OptimizeByJoiningCurves: collapse a curves-only
// pipeline into one curve set.
func optimizeByJoiningCurves(ctx *Context, lutPtr **Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) bool {

	_ = intent

	if FormatterIsFloat(*inputFormat) || FormatterIsFloat(*outputFormat) {
		return false
	}

	src := *lutPtr

	// Only curves in this LUT?
	for mpe := src.GetPtrToFirstStage(); mpe != nil; mpe = mpe.next {
		if mpe.StageType() != SigCurveSetElemType {
			return false
		}
	}

	dest, err := ctx.PipelineAlloc(src.InputChannels, src.OutputChannels)
	if err != nil || dest == nil {
		return false
	}

	nIn := src.InputChannels
	gammaTables := make([]*ToneCurve, nIn)
	for i := uint32(0); i < nIn; i++ {
		c, cerr := ctx.BuildTabulatedToneCurve16(make([]uint16, prelinearizationPoints))
		if cerr != nil || c == nil {
			return false
		}
		gammaTables[i] = c
	}

	var inFloat, outFloat [maxChannels]float32
	for i := 0; i < prelinearizationPoints; i++ {
		for j := uint32(0); j < nIn; j++ {
			inFloat[j] = float32(float64(i) / float64(prelinearizationPoints-1))
		}
		src.EvalFloat(inFloat[:], outFloat[:])
		for j := uint32(0); j < nIn; j++ {
			gammaTables[j].table16[i] = quickSaturateWord(float64(outFloat[j]) * 65535.0)
		}
	}

	obtainedCurves, err := ctx.StageAllocToneCurves(nIn, gammaTables)
	if err != nil || obtainedCurves == nil {
		return false
	}

	if !allCurvesAreLinear(obtainedCurves) {
		if ierr := dest.InsertStage(AtBegin, obtainedCurves); ierr != nil {
			return false
		}
		data, ok := obtainedCurves.data.(*stageToneCurvesData)
		if !ok {
			return false
		}

		if FormatterIs8bit(*inputFormat) {
			c16 := curvesAlloc(dest.ctx, data.nCurves, 256, data.theCurves)
			if c16 == nil {
				return false
			}
			*dwFlags |= FlagsNoCache
			dest.setOptimizationParameters(fastEvaluateCurves8, c16, nil, shareDataDup)
		} else {
			c16 := curvesAlloc(dest.ctx, data.nCurves, 65536, data.theCurves)
			if c16 == nil {
				return false
			}
			*dwFlags |= FlagsNoCache
			dest.setOptimizationParameters(fastEvaluateCurves16, c16, nil, shareDataDup)
		}
	} else {
		// LUT optimizes to nothing: install an identity.
		idStage := ctx.StageAllocIdentity(src.InputChannels)
		if idStage == nil {
			return false
		}
		if ierr := dest.InsertStage(AtBegin, idStage); ierr != nil {
			return false
		}
		*dwFlags |= FlagsNoCache
		dest.setOptimizationParameters(fastIdentity16, dest, nil, nil)
	}

	*lutPtr = dest
	return true
}

// -----------------------------------------------------------------------------
// OptimizeMatrixShaper.
// -----------------------------------------------------------------------------

// fillFirstShaper ports FillFirstShaper: 8-bit -> 1.14 after applying the curve.
func fillFirstShaper(table []s1Fixed14, curve *ToneCurve) {
	for i := 0; i < 256; i++ {
		r := float32(float64(i) / 255.0)
		y := curve.EvalFloat(r)
		if y < 131072.0 {
			table[i] = doubleTo1Fixed14(float64(y))
		} else {
			table[i] = 0x7fffffff
		}
	}
}

// fillSecondShaper ports FillSecondShaper: 1.14 -> 8/16 bits after the curve.
func fillSecondShaper(table []uint16, curve *ToneCurve, is8BitsOutput bool) {
	for i := 0; i < 16385; i++ {
		r := float32(float64(i) / 16384.0)
		val := curve.EvalFloat(r) // 0..1.0
		if val < 0 {
			val = 0
		}
		if val > 1.0 {
			val = 1.0
		}
		if is8BitsOutput {
			w := quickSaturateWord(float64(val) * 65535.0)
			b := from16to8(w)
			table[i] = from8to16(b)
		} else {
			table[i] = quickSaturateWord(float64(val) * 65535.0)
		}
	}
}

// setMatShaper ports SetMatShaper: precompute the matrix-shaper tables.
func setMatShaper(dest *Pipeline, curve1 []*ToneCurve, mat MAT3, off *VEC3,
	curve2 []*ToneCurve, outputFormat *uint32) bool {

	is8Bits := FormatterIs8bit(*outputFormat)

	p := &matShaper8Data{ctx: dest.ctx}

	fillFirstShaper(p.shaper1R[:], curve1[0])
	fillFirstShaper(p.shaper1G[:], curve1[1])
	fillFirstShaper(p.shaper1B[:], curve1[2])

	fillSecondShaper(p.shaper2R[:], curve2[0], is8Bits)
	fillSecondShaper(p.shaper2G[:], curve2[1], is8Bits)
	fillSecondShaper(p.shaper2B[:], curve2[2], is8Bits)

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			p.mat[i][j] = doubleTo1Fixed14(mat[i][j])
		}
	}

	for i := 0; i < 3; i++ {
		if off == nil {
			p.off[i] = 0
		} else {
			p.off[i] = doubleTo1Fixed14(off[i])
		}
	}

	// Mark as optimized for the faster formatter.
	if is8Bits {
		*outputFormat |= optimizedSH(1)
	}

	dest.setOptimizationParameters(matShaperEval16, p, nil, shareDataDup)
	return true
}

// optimizeMatrixShaper ports OptimizeMatrixShaper.
func optimizeMatrixShaper(ctx *Context, lutPtr **Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) bool {

	// Only RGB to RGB.
	if tChannels(*inputFormat) != 3 || tChannels(*outputFormat) != 3 {
		return false
	}
	// Only 8-bit input.
	if !FormatterIs8bit(*inputFormat) {
		return false
	}

	src := *lutPtr

	var curve1, curve2, matrix1, matrix2 *Stage
	var res MAT3
	var offset []float64
	identityMat := false

	if stages, ok := src.CheckAndRetrieveStages(
		SigCurveSetElemType, SigMatrixElemType, SigMatrixElemType, SigCurveSetElemType); ok {
		curve1, matrix1, matrix2, curve2 = stages[0], stages[1], stages[2], stages[3]

		data1, ok1 := matrix1.data.(*stageMatrixData)
		data2, ok2 := matrix2.data.(*stageMatrixData)
		if !ok1 || !ok2 {
			return false
		}

		if matrix1.InputChannels != 3 || matrix1.OutputChannels != 3 ||
			matrix2.InputChannels != 3 || matrix2.OutputChannels != 3 {
			return false
		}

		// Input offset should be zero.
		if data1.offset != nil {
			return false
		}

		res = MAT3Per(mat3FromRowMajor(data2.double), mat3FromRowMajor(data1.double))
		offset = data2.offset

		if MAT3IsIdentity(res) && offset == nil {
			identityMat = true
		}
	} else if stages, ok := src.CheckAndRetrieveStages(
		SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType); ok {
		curve1, matrix1, curve2 = stages[0], stages[1], stages[2]

		data, okd := matrix1.data.(*stageMatrixData)
		if !okd {
			return false
		}
		if matrix1.InputChannels != 3 || matrix1.OutputChannels != 3 {
			return false
		}

		res = mat3FromRowMajor(data.double)
		offset = data.offset

		if MAT3IsIdentity(res) && offset == nil {
			identityMat = true
		}
	} else {
		return false
	}

	dest, err := ctx.PipelineAlloc(src.InputChannels, src.OutputChannels)
	if err != nil || dest == nil {
		return false
	}

	dupCurve1, err := curve1.stageDup()
	if err != nil || dupCurve1 == nil {
		return false
	}
	if err := dest.InsertStage(AtBegin, dupCurve1); err != nil {
		return false
	}

	if !identityMat {
		matStage, merr := ctx.StageAllocMatrix(3, 3, mat3ToRowMajor(res), offset)
		if merr != nil || matStage == nil {
			return false
		}
		if err := dest.InsertStage(AtEnd, matStage); err != nil {
			return false
		}
	}

	dupCurve2, err := curve2.stageDup()
	if err != nil || dupCurve2 == nil {
		return false
	}
	if err := dest.InsertStage(AtEnd, dupCurve2); err != nil {
		return false
	}

	if identityMat {
		optimizeByJoiningCurves(ctx, &dest, intent, inputFormat, outputFormat, dwFlags)
	} else {
		mpeC1, ok1 := curve1.data.(*stageToneCurvesData)
		mpeC2, ok2 := curve2.data.(*stageToneCurvesData)
		if !ok1 || !ok2 {
			return false
		}

		// Cache does not help; it costs more than the pixel handling.
		*dwFlags |= FlagsNoCache

		var offVec *VEC3
		if offset != nil {
			var v VEC3
			for i := 0; i < 3 && i < len(offset); i++ {
				v[i] = offset[i]
			}
			offVec = &v
		}

		setMatShaper(dest, mpeC1.theCurves, res, offVec, mpeC2.theCurves, outputFormat)
	}

	*lutPtr = dest
	return true
}

// -----------------------------------------------------------------------------
// _cmsOptimizePipeline dispatch, wired into the transform seam.
// -----------------------------------------------------------------------------

// optimizePipeline ports _cmsOptimizePipeline. It replaces *lut with an
// optimized equivalent (installing a specialized evaluator), may adjust
// *dwFlags/formats, and returns true when an optimized evaluator was installed.
func (ctx *Context) optimizePipeline(lut **Pipeline, intent uint32,
	inputFormat, outputFormat, dwFlags *uint32) bool {

	// A CLUT is being asked; force resampling.
	if *dwFlags&FlagsForceCLUT != 0 {
		preOptimize(*lut)
		(*lut).blessLUT()
		return optimizeByResampling(ctx, lut, intent, inputFormat, outputFormat, dwFlags)
	}

	// Anything to optimize?
	if (*lut).elements == nil {
		(*lut).setOptimizationParameters(fastIdentity16, *lut, nil, nil)
		return true
	}

	// Named-color pipelines cannot be optimized.
	for mpe := (*lut).GetPtrToFirstStage(); mpe != nil; mpe = mpe.next {
		if mpe.StageType() == SigNamedColorElemType {
			return false
		}
	}

	// Get rid of identities and trivial conversions.
	anySuccess := preOptimize(*lut)
	(*lut).blessLUT()

	// After removal, did we end with an identity?
	if (*lut).elements == nil {
		(*lut).setOptimizationParameters(fastIdentity16, *lut, nil, nil)
		return true
	}

	// Keep all precision?
	if *dwFlags&FlagsNoOptimize != 0 {
		return false
	}

	// Try plug-in optimizations (newest-first).
	ctx.mu.Lock()
	plugins := append([]Plugin(nil), ctx.optimization.entries...)
	ctx.mu.Unlock()
	for _, pl := range plugins {
		po, ok := pl.(*PluginOptimization)
		if !ok || po.OptimizePtr == nil {
			continue
		}
		if po.OptimizePtr(ctx, lut, intent, inputFormat, outputFormat, dwFlags) {
			return true
		}
	}

	// Try built-in optimizations.
	for _, opt := range defaultOptimizations {
		if opt(ctx, lut, intent, inputFormat, outputFormat, dwFlags) {
			return true
		}
	}

	// Only simple optimizations succeeded.
	return anySuccess
}
