package lcms2

// This file ports the pipeline / stage (LUT) model from src/cmslut.c
// (lcms2 2.19): the cmsStage multi-process-element model, every public and
// internal stage constructor, the stage evaluators, CLUT sampling, and the
// cmsPipeline container with 16-bit / float / reverse (Newton-Raphson)
// evaluation.
//
// Intentional deviations from the C reference (Go runtime model, not behaviour):
//
//   - Memory management (cmsStageFree, cmsPipelineFree, the per-element FreePtr
//     and the goto-Error cleanup chains) is owned by the Go garbage collector.
//     Free is a documented no-op kept only for API symmetry; DupElemPtr/FreePtr
//     function pointers are replaced by a type-driven deep copy (stageDup).
//
//   - Evaluation dispatch. C stores an EvalPtr function pointer per stage and
//     the pipeline walks it. Go escape analysis heap-allocates the per-call
//     scratch buffer whenever it is passed to a function *value* (an indirect
//     call), which would defeat the zero-allocation contract for
//     PipelineEval16 / PipelineEvalFloat. So the throughput evaluators dispatch
//     through a concrete type switch (lutEval16 / lutEvalFloat) whose branches
//     call named, non-escaping stage evaluators; the scratch stays on the
//     stack. The EvalPtr function field survives on Stage (mirroring C) and
//     drives the generic evaluators (lutEval16Generic / lutEvalFloatGeneric),
//     which the pipeline selects automatically when it holds a stage whose
//     interpolator was supplied by a plug-in (an indirect call is unavoidable
//     there). The default, plug-in-free case is fully concrete and allocation
//     free.
//
//   - The CLUT interpolator is likewise dispatched concretely (clutLerp16 /
//     clutLerpFloat switch on a kind classified once at construction) instead
//     of through cmsInterpParams.Interpolation.Lerp16, for the same reason. The
//     classification mirrors DefaultInterpolatorsFactory exactly; a plug-in
//     interpolator sets the kind to "generic" and routes the whole pipeline to
//     the func-value path.
//
//   - bool-returning C predicates (cmsPipelineInsertStage, cmsPipelineCat)
//     become error-returning per the project convention; the channel-mismatch
//     that makes BlessLUT return FALSE surfaces as an *Error.

import "math"

// maxChannels (cmsMAXCHANNELS) is declared in context.go.

// samplerInspect mirrors SAMPLER_INSPECT: sample the grid without writing back.
const samplerInspect = 0x01000000

// StageSignature mirrors cmsStageSignature: the four-byte type tag of a
// multi-process element (pipeline stage).
type StageSignature uint32

// Stage element type signatures, mirroring the cmsStageSignature enum in
// include/lcms2.h.
const (
	SigCurveSetElemType      StageSignature = 0x63767374 // 'cvst'
	SigMatrixElemType        StageSignature = 0x6D617466 // 'matf'
	SigCLutElemType          StageSignature = 0x636C7574 // 'clut'
	SigBAcsElemType          StageSignature = 0x62414353 // 'bACS'
	SigEAcsElemType          StageSignature = 0x65414353 // 'eACS'
	SigXYZ2LabElemType       StageSignature = 0x6C327820 // 'l2x '
	SigLab2XYZElemType       StageSignature = 0x78326C20 // 'x2l '
	SigNamedColorElemType    StageSignature = 0x6E636C20 // 'ncl '
	SigLabV2toV4             StageSignature = 0x32203420 // '2 4 '
	SigLabV4toV2             StageSignature = 0x34203220 // '4 2 '
	SigIdentityElemType      StageSignature = 0x69646E20 // 'idn '
	SigLab2FloatPCS          StageSignature = 0x64326C20 // 'd2l '
	SigFloatPCS2Lab          StageSignature = 0x6C326420 // 'l2d '
	SigXYZ2FloatPCS          StageSignature = 0x64327820 // 'd2x '
	SigFloatPCS2XYZ          StageSignature = 0x78326420 // 'x2d '
	SigClipNegativesElemType StageSignature = 0x636C7020 // 'clp '
)

// StageLoc mirrors cmsStageLoc: where to insert / remove a stage.
type StageLoc int

const (
	AtBegin StageLoc = iota // cmsAT_BEGIN
	AtEnd                   // cmsAT_END
)

// Sampler16 mirrors cmsSAMPLER16: called on every grid knot during
// cmsStageSampleCLut16bit / cmsSliceSpace16. Returning false aborts the sweep
// (mirroring the C convention where a FALSE return stops sampling). out is nil
// for the slice-space samplers.
type Sampler16 func(in, out []uint16, cargo any) bool

// SamplerFloat mirrors cmsSAMPLERFLOAT.
type SamplerFloat func(in, out []float32, cargo any) bool

// stageEvalFn mirrors _cmsStageEvalFn: evaluate a stage in floating point.
type stageEvalFn func(in, out []float32, mpe *Stage)

// Stage mirrors cmsStage (struct _cmsStage_struct): one multi-process element.
type Stage struct {
	ctx *Context

	Type       StageSignature
	Implements StageSignature

	InputChannels  uint32
	OutputChannels uint32

	eval stageEvalFn // mirrors EvalPtr; drives the generic (func-value) path
	data any         // typed payload (*stageToneCurvesData, *stageMatrixData, *stageCLutData) or nil

	// concrete reports whether the throughput evaluators can dispatch this
	// stage through the concrete type switch (true for every built-in stage
	// whose interpolator is not plug-in supplied). When false the owning
	// pipeline falls back to the generic func-value evaluators.
	concrete bool

	next *Stage
}

// ---- Stage accessors (cmsStageInputChannels, ... , cmsStageNext) ----

// InputChannelsCount ports cmsStageInputChannels.
func (mpe *Stage) InputChannelsCount() uint32 { return mpe.InputChannels }

// OutputChannelsCount ports cmsStageOutputChannels.
func (mpe *Stage) OutputChannelsCount() uint32 { return mpe.OutputChannels }

// StageType ports cmsStageType.
func (mpe *Stage) StageType() StageSignature { return mpe.Type }

// Data ports cmsStageData.
func (mpe *Stage) Data() any { return mpe.data }

// ContextID ports cmsGetStageContextID.
func (mpe *Stage) ContextID() *Context { return mpe.ctx }

// Next ports cmsStageNext.
func (mpe *Stage) Next() *Stage { return mpe.next }

// stageAllocPlaceholder ports _cmsStageAllocPlaceholder: allocate an empty
// stage with the given type, channel counts and evaluator.
func (ctx *Context) stageAllocPlaceholder(typ StageSignature, in, out uint32, eval stageEvalFn, data any) *Stage {
	return &Stage{
		ctx:            ctx,
		Type:           typ,
		Implements:     typ, // by default, no clue on what is implementing
		InputChannels:  in,
		OutputChannels: out,
		eval:           eval,
		data:           data,
	}
}

// *************************************************************************
// Identity
// *************************************************************************

// evaluateIdentity ports EvaluateIdentity.
func evaluateIdentity(in, out []float32, mpe *Stage) {
	copy(out[:mpe.InputChannels], in[:mpe.InputChannels])
}

// StageAllocIdentity ports cmsStageAllocIdentity.
func (ctx *Context) StageAllocIdentity(nChannels uint32) *Stage {
	mpe := ctx.stageAllocPlaceholder(SigIdentityElemType, nChannels, nChannels, evaluateIdentity, nil)
	mpe.concrete = true
	return mpe
}

// StageAllocIdentity builds an identity stage on the default context.
func StageAllocIdentity(nChannels uint32) *Stage { return defaultContext.StageAllocIdentity(nChannels) }

// fromFloatTo16 ports FromFloatTo16.
func fromFloatTo16(in []float32, out []uint16, n uint32) {
	for i := uint32(0); i < n; i++ {
		out[i] = quickSaturateWord(float64(in[i]) * 65535.0)
	}
}

// from16ToFloat ports From16ToFloat.
func from16ToFloat(in []uint16, out []float32, n uint32) {
	for i := uint32(0); i < n; i++ {
		out[i] = float32(in[i]) / 65535.0
	}
}

// *************************************************************************
// Curves (cmsSigCurveSetElemType)
// *************************************************************************

// stageToneCurvesData mirrors _cmsStageToneCurvesData.
type stageToneCurvesData struct {
	nCurves   uint32
	theCurves []*ToneCurve
}

// GetToneCurves ports _cmsStageGetPtrToCurveSet: the tone curves of a curve-set
// stage, or nil if this is not a curve-set stage.
func (mpe *Stage) GetToneCurves() []*ToneCurve {
	if d, ok := mpe.data.(*stageToneCurvesData); ok {
		return d.theCurves
	}
	return nil
}

// evaluateCurves ports EvaluateCurves. It reads and writes scalars only, so the
// caller's buffer never escapes.
func evaluateCurves(in, out []float32, mpe *Stage) {
	d, ok := mpe.data.(*stageToneCurvesData)
	if !ok || d.theCurves == nil {
		return
	}
	for i := uint32(0); i < d.nCurves; i++ {
		out[i] = d.theCurves[i].EvalFloat(in[i])
	}
}

// StageAllocToneCurves ports cmsStageAllocToneCurves. A nil curves slice forces
// identity gamma curves.
func (ctx *Context) StageAllocToneCurves(nChannels uint32, curves []*ToneCurve) (*Stage, error) {
	// The pipeline evaluators buffer channels in fixed [maxStageChannels] arrays;
	// a stage wider than that would index past them during Eval. The tag readers
	// already cap channel counts well below this, so this only guards direct API
	// misuse.
	if nChannels > maxStageChannels {
		return nil, ctx.signalError(ErrRange, "Too many channels (%d channels, max=%d)", nChannels, maxStageChannels)
	}
	if curves != nil && uint32(len(curves)) < nChannels {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocToneCurves: fewer curves (%d) than channels (%d)", len(curves), nChannels)
	}
	d := &stageToneCurvesData{
		nCurves:   nChannels,
		theCurves: make([]*ToneCurve, nChannels),
	}
	for i := uint32(0); i < nChannels; i++ {
		var c *ToneCurve
		var err error
		if curves == nil {
			c, err = ctx.BuildGamma(1.0)
		} else {
			c, err = curves[i].Dup()
		}
		if err != nil {
			return nil, err
		}
		d.theCurves[i] = c
	}
	mpe := ctx.stageAllocPlaceholder(SigCurveSetElemType, nChannels, nChannels, evaluateCurves, d)
	mpe.concrete = true
	return mpe, nil
}

// StageAllocToneCurves builds a curve-set stage on the default context.
func StageAllocToneCurves(nChannels uint32, curves []*ToneCurve) (*Stage, error) {
	return defaultContext.StageAllocToneCurves(nChannels, curves)
}

// stageAllocIdentityCurves ports _cmsStageAllocIdentityCurves.
func (ctx *Context) stageAllocIdentityCurves(nChannels uint32) (*Stage, error) {
	mpe, err := ctx.StageAllocToneCurves(nChannels, nil)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigIdentityElemType
	return mpe, nil
}

// *************************************************************************
// Matrix (cmsSigMatrixElemType)
// *************************************************************************

// stageMatrixData mirrors _cmsStageMatrixData. double is row-major with
// OutputChannels rows and InputChannels columns; offset has OutputChannels
// entries or is nil.
type stageMatrixData struct {
	double []float64
	offset []float64
}

// MatrixData returns the matrix payload (row-major coefficients and optional
// offset) of a matrix stage, or nil, nil otherwise.
func (mpe *Stage) MatrixData() (double, offset []float64) {
	if d, ok := mpe.data.(*stageMatrixData); ok {
		return d.double, d.offset
	}
	return nil, nil
}

// evaluateMatrix ports EvaluateMatrix. A cmsFloat64Number accumulator is used
// for precision, exactly as the reference does.
func evaluateMatrix(in, out []float32, mpe *Stage) {
	d, ok := mpe.data.(*stageMatrixData)
	if !ok {
		return
	}
	inCh := mpe.InputChannels
	for i := uint32(0); i < mpe.OutputChannels; i++ {
		var tmp float64
		for j := uint32(0); j < inCh; j++ {
			tmp += float64(in[j]) * d.double[i*inCh+j]
		}
		if d.offset != nil {
			tmp += d.offset[i]
		}
		out[i] = float32(tmp)
	}
}

// StageAllocMatrix ports cmsStageAllocMatrix. matrix has Rows*Cols entries
// (row-major); offset has Rows entries or is nil.
func (ctx *Context) StageAllocMatrix(rows, cols uint32, matrix, offset []float64) (*Stage, error) {
	n := rows * cols

	// Overflow guards, matching the reference.
	if n == 0 {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocMatrix: zero-sized matrix")
	}
	if cols != 0 && n/cols != rows {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocMatrix: matrix size overflow")
	}
	if uint32(len(matrix)) < n {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocMatrix: matrix slice too short")
	}
	if offset != nil && uint32(len(offset)) < rows {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocMatrix: offset slice too short")
	}

	d := &stageMatrixData{double: make([]float64, n)}
	copy(d.double, matrix[:n])
	if offset != nil {
		d.offset = make([]float64, rows)
		copy(d.offset, offset[:rows])
	}

	mpe := ctx.stageAllocPlaceholder(SigMatrixElemType, cols, rows, evaluateMatrix, d)
	mpe.concrete = true
	return mpe, nil
}

// StageAllocMatrix builds a matrix stage on the default context.
func StageAllocMatrix(rows, cols uint32, matrix, offset []float64) (*Stage, error) {
	return defaultContext.StageAllocMatrix(rows, cols, matrix, offset)
}

// *************************************************************************
// CLUT (cmsSigCLutElemType)
// *************************************************************************

// Interpolator kinds classify which concrete interpolator a CLUT stage uses, so
// evaluation dispatches without an indirect call. genericKind marks a plug-in
// supplied interpolator (dispatched through the func value).
const (
	genericKind uint8 = iota
	kindLinLerp1D
	kindEval1Input
	kindBilinear
	kindTrilinear
	kindTetra
	kindNInputs
)

// stageCLutData mirrors _cmsStageCLutData.
type stageCLutData struct {
	tab16          []uint16
	tabFloat       []float32
	nEntries       uint32
	hasFloatValues bool
	params         *InterpParams
	kind           uint8 // concrete interpolator id, or genericKind for a plug-in
}

// CLUTData returns a CLUT stage's payload, or nil.
func (mpe *Stage) CLUTData() *stageCLutData {
	if d, ok := mpe.data.(*stageCLutData); ok {
		return d
	}
	return nil
}

// classifyInterpKind mirrors DefaultInterpolatorsFactory's selection so the
// evaluator can call the chosen interpolator concretely. It returns genericKind
// when a plug-in factory is active (the plug-in may override the selection) or
// when the combination is the reference's unsupported safety case.
func classifyInterpKind(ctx *Context, nIn, nOut, dwFlags uint32) uint8 {
	if ctx.interpolatorsFactory() != nil {
		return genericKind
	}
	if nIn >= 4 && nOut >= maxStageChannels {
		return genericKind
	}
	trilinear := dwFlags&cmsLerpFlagsTrilinear != 0
	switch nIn {
	case 1:
		if nOut == 1 {
			return kindLinLerp1D
		}
		return kindEval1Input
	case 2:
		return kindBilinear
	case 3:
		if trilinear {
			return kindTrilinear
		}
		return kindTetra
	case 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15:
		return kindNInputs
	}
	return genericKind
}

// clutLerp16 dispatches the 16-bit interpolator concretely. The default branch
// is never reached for a stage flagged concrete (a genericKind stage routes the
// whole pipeline to the func-value evaluators); it is a safe identity copy.
func clutLerp16(clut *stageCLutData, in, out []uint16) {
	p := clut.params
	switch clut.kind {
	case kindLinLerp1D:
		linLerp1D(in, out, p)
	case kindEval1Input:
		eval1Input(in, out, p)
	case kindBilinear:
		bilinearInterp16(in, out, p)
	case kindTrilinear:
		trilinearInterp16(in, out, p)
	case kindTetra:
		tetrahedralInterp16(in, out, p)
	case kindNInputs:
		evalNInputs16(in, out, p)
	default:
		copy(out[:p.NumOutputs], in[:p.NumOutputs])
	}
}

// clutLerpFloat dispatches the float interpolator concretely.
func clutLerpFloat(clut *stageCLutData, in, out []float32) {
	p := clut.params
	switch clut.kind {
	case kindLinLerp1D:
		linLerp1Dfloat(in, out, p)
	case kindEval1Input:
		eval1InputFloat(in, out, p)
	case kindBilinear:
		bilinearInterpFloat(in, out, p)
	case kindTrilinear:
		trilinearInterpFloat(in, out, p)
	case kindTetra:
		tetrahedralInterpFloat(in, out, p)
	case kindNInputs:
		evalNInputsFloat(in, out, p)
	default:
		copy(out[:p.NumOutputs], in[:p.NumOutputs])
	}
}

// evaluateCLUTfloat ports EvaluateCLUTfloat (concrete path).
func evaluateCLUTfloat(in, out []float32, mpe *Stage) {
	d, ok := mpe.data.(*stageCLutData)
	if !ok {
		return
	}
	clutLerpFloat(d, in, out)
}

// evaluateCLUTfloatIn16 ports EvaluateCLUTfloatIn16 (concrete path): convert to
// 16 bits, interpolate, convert back. The 16-bit scratch is local and, since
// clutLerp16 makes no indirect call, stays on the stack.
func evaluateCLUTfloatIn16(in, out []float32, mpe *Stage) {
	d, ok := mpe.data.(*stageCLutData)
	if !ok {
		return
	}
	var in16, out16 [maxStageChannels]uint16
	fromFloatTo16(in, in16[:], mpe.InputChannels)
	clutLerp16(d, in16[:], out16[:])
	from16ToFloat(out16[:], out, mpe.OutputChannels)
}

// evaluateCLUTfloatGeneric / evaluateCLUTfloatIn16Generic dispatch the
// interpolator through the plug-in func value; used only by the generic
// pipeline evaluators.
func evaluateCLUTfloatGeneric(in, out []float32, mpe *Stage) {
	d, ok := mpe.data.(*stageCLutData)
	if !ok {
		return
	}
	d.params.Interpolation.LerpFloat(in, out, d.params)
}

func evaluateCLUTfloatIn16Generic(in, out []float32, mpe *Stage) {
	d, ok := mpe.data.(*stageCLutData)
	if !ok {
		return
	}
	var in16, out16 [maxStageChannels]uint16
	fromFloatTo16(in, in16[:], mpe.InputChannels)
	d.params.Interpolation.Lerp16(in16[:], out16[:], d.params)
	from16ToFloat(out16[:], out, mpe.OutputChannels)
}

// cubeSize ports CubeSize: total node count of a hypercube, with the exact
// overflow guards of the reference (returns 0 on overflow or a <=1 dimension).
func cubeSize(dims []uint32, b uint32) uint32 {
	const uintMax = ^uint32(0)
	var rv uint64 = 1
	for ; b > 0; b-- {
		dim := dims[b-1]
		if dim <= 1 {
			return 0
		}
		if rv > uint64(uintMax)/uint64(dim) {
			return 0
		}
		rv *= uint64(dim)
	}
	if rv > uint64(uintMax)/15 {
		return 0
	}
	return uint32(rv)
}

// clutEntries computes the flattened CLUT table length outputChan*gridEntries
// without the uint32 wraparound the reference warns about (cmslut.c: "There is a
// potential integer overflow on conputing n and nEntries"). It returns ok=false
// when either factor is zero or the product does not fit in a uint32 — in which
// case the caller rejects the stage rather than allocating a truncated table and
// later indexing it with the un-truncated output-channel count.
func clutEntries(outputChan, gridEntries uint32) (uint32, bool) {
	if outputChan == 0 || gridEntries == 0 {
		return 0, false
	}
	n := uint64(outputChan) * uint64(gridEntries)
	if n > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(n), true
}

// StageAllocCLut16bitGranular ports cmsStageAllocCLut16bitGranular.
func (ctx *Context) StageAllocCLut16bitGranular(clutPoints []uint32, inputChan, outputChan uint32, table []uint16) (*Stage, error) {
	if inputChan > maxInputDimensions {
		return nil, ctx.signalError(ErrRange, "Too many input channels (%d channels, max=%d)", inputChan, maxInputDimensions)
	}
	if uint32(len(clutPoints)) < inputChan {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocCLut16bitGranular: clutPoints too short")
	}
	if outputChan > maxStageChannels {
		return nil, ctx.signalError(ErrRange, "Too many output channels (%d channels, max=%d)", outputChan, maxStageChannels)
	}

	n, ok := clutEntries(outputChan, cubeSize(clutPoints, inputChan))
	if !ok {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocCLut16bitGranular: empty or overflowing CLUT")
	}
	// _cmsCalloc(n, sizeof(cmsUInt16Number)) returns NULL past 512 MB and the
	// reference constructor then returns NULL; a modest grid over a handful of
	// input channels reaches that in a few caller-supplied bytes.
	if !allocSizeOK(n, 2) {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocCLut16bitGranular: CLUT of %d entries exceeds the %d byte allocation limit", n, maxMemoryForAlloc)
	}

	d := &stageCLutData{nEntries: n, hasFloatValues: false, tab16: make([]uint16, n)}
	if table != nil {
		copy(d.tab16, table[:min32(n, uint32(len(table)))])
	}

	p, err := computeInterpParamsEx(ctx, clutPoints, inputChan, outputChan, d.tab16, cmsLerpFlags16Bits)
	if err != nil {
		return nil, err
	}
	d.params = p
	d.kind = classifyInterpKind(ctx, inputChan, outputChan, cmsLerpFlags16Bits)

	mpe := ctx.stageAllocPlaceholder(SigCLutElemType, inputChan, outputChan, evaluateCLUTfloatIn16, d)
	mpe.concrete = d.kind != genericKind
	return mpe, nil
}

// StageAllocCLut16bitGranular builds a granular 16-bit CLUT on the default context.
func StageAllocCLut16bitGranular(clutPoints []uint32, inputChan, outputChan uint32, table []uint16) (*Stage, error) {
	return defaultContext.StageAllocCLut16bitGranular(clutPoints, inputChan, outputChan, table)
}

// StageAllocCLut16bit ports cmsStageAllocCLut16bit: a uniform-grid 16-bit CLUT.
func (ctx *Context) StageAllocCLut16bit(nGridPoints, inputChan, outputChan uint32, table []uint16) (*Stage, error) {
	var dims [maxInputDimensions]uint32
	for i := range dims {
		dims[i] = nGridPoints
	}
	return ctx.StageAllocCLut16bitGranular(dims[:], inputChan, outputChan, table)
}

// StageAllocCLut16bit builds a uniform-grid 16-bit CLUT on the default context.
func StageAllocCLut16bit(nGridPoints, inputChan, outputChan uint32, table []uint16) (*Stage, error) {
	return defaultContext.StageAllocCLut16bit(nGridPoints, inputChan, outputChan, table)
}

// StageAllocCLutFloatGranular ports cmsStageAllocCLutFloatGranular.
func (ctx *Context) StageAllocCLutFloatGranular(clutPoints []uint32, inputChan, outputChan uint32, table []float32) (*Stage, error) {
	if inputChan > maxInputDimensions {
		return nil, ctx.signalError(ErrRange, "Too many input channels (%d channels, max=%d)", inputChan, maxInputDimensions)
	}
	if uint32(len(clutPoints)) < inputChan {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocCLutFloatGranular: clutPoints too short")
	}
	if outputChan > maxStageChannels {
		return nil, ctx.signalError(ErrRange, "Too many output channels (%d channels, max=%d)", outputChan, maxStageChannels)
	}

	n, ok := clutEntries(outputChan, cubeSize(clutPoints, inputChan))
	if !ok {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocCLutFloatGranular: empty or overflowing CLUT")
	}
	// As above, for _cmsCalloc(n, sizeof(cmsFloat32Number)).
	if !allocSizeOK(n, 4) {
		return nil, ctx.signalError(ErrRange, "cmsStageAllocCLutFloatGranular: CLUT of %d entries exceeds the %d byte allocation limit", n, maxMemoryForAlloc)
	}

	d := &stageCLutData{nEntries: n, hasFloatValues: true, tabFloat: make([]float32, n)}
	if table != nil {
		copy(d.tabFloat, table[:min32(n, uint32(len(table)))])
	}

	p, err := computeInterpParamsEx(ctx, clutPoints, inputChan, outputChan, d.tabFloat, cmsLerpFlagsFloat)
	if err != nil {
		return nil, err
	}
	d.params = p
	d.kind = classifyInterpKind(ctx, inputChan, outputChan, cmsLerpFlagsFloat)

	mpe := ctx.stageAllocPlaceholder(SigCLutElemType, inputChan, outputChan, evaluateCLUTfloat, d)
	mpe.concrete = d.kind != genericKind
	return mpe, nil
}

// StageAllocCLutFloatGranular builds a granular float CLUT on the default context.
func StageAllocCLutFloatGranular(clutPoints []uint32, inputChan, outputChan uint32, table []float32) (*Stage, error) {
	return defaultContext.StageAllocCLutFloatGranular(clutPoints, inputChan, outputChan, table)
}

// StageAllocCLutFloat ports cmsStageAllocCLutFloat: a uniform-grid float CLUT.
func (ctx *Context) StageAllocCLutFloat(nGridPoints, inputChan, outputChan uint32, table []float32) (*Stage, error) {
	var dims [maxInputDimensions]uint32
	for i := range dims {
		dims[i] = nGridPoints
	}
	return ctx.StageAllocCLutFloatGranular(dims[:], inputChan, outputChan, table)
}

// StageAllocCLutFloat builds a uniform-grid float CLUT on the default context.
func StageAllocCLutFloat(nGridPoints, inputChan, outputChan uint32, table []float32) (*Stage, error) {
	return defaultContext.StageAllocCLutFloat(nGridPoints, inputChan, outputChan, table)
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// identitySampler ports IdentitySampler.
func identitySampler(in, out []uint16, cargo any) bool {
	nChan, ok := cargo.(int)
	if !ok {
		return false
	}
	for i := 0; i < nChan; i++ {
		out[i] = in[i]
	}
	return true
}

// stageAllocIdentityCLut ports _cmsStageAllocIdentityCLut.
func (ctx *Context) stageAllocIdentityCLut(nChan uint32) (*Stage, error) {
	var dims [maxInputDimensions]uint32
	for i := range dims {
		dims[i] = 2
	}
	mpe, err := ctx.StageAllocCLut16bitGranular(dims[:], nChan, nChan, nil)
	if err != nil {
		return nil, err
	}
	if !mpe.SampleCLut16bit(identitySampler, int(nChan), 0) {
		return nil, ctx.signalError(ErrInternal, "_cmsStageAllocIdentityCLut: sampling failed")
	}
	mpe.Implements = SigIdentityElemType
	return mpe, nil
}

// quantizeVal ports _cmsQuantizeVal: quantize 0<=i<maxSamples to 0..0xffff.
func quantizeVal(i float64, maxSamples uint32) uint16 {
	x := (i * 65535.0) / float64(maxSamples-1)
	return quickSaturateWord(x)
}

// SampleCLut16bit ports cmsStageSampleCLut16bit: sweep the whole grid, calling
// Sampler on each knot. Returns false if the sweep is aborted or the stage is
// malformed.
func (mpe *Stage) SampleCLut16bit(sampler Sampler16, cargo any, dwFlags uint32) bool {
	if mpe == nil {
		return false
	}
	clut, ok := mpe.data.(*stageCLutData)
	if !ok {
		return false
	}

	p := clut.params
	nInputs := p.NumInputs
	nOutputs := p.NumOutputs

	if nInputs == 0 || nOutputs == 0 {
		return false
	}
	if nInputs > maxInputDimensions {
		return false
	}
	if nOutputs >= maxStageChannels {
		return false
	}

	var in [maxInputDimensions + 1]uint16
	var out [maxStageChannels]uint16

	nTotalPoints := cubeSize(p.NumSamples[:], nInputs)
	if nTotalPoints == 0 {
		return false
	}

	index := 0
	for i := uint32(0); i < nTotalPoints; i++ {
		rest := i
		for t := int(nInputs) - 1; t >= 0; t-- {
			colorant := rest % p.NumSamples[t]
			rest /= p.NumSamples[t]
			in[t] = quantizeVal(float64(colorant), p.NumSamples[t])
		}

		if clut.tab16 != nil {
			for t := uint32(0); t < nOutputs; t++ {
				out[t] = clut.tab16[index+int(t)]
			}
		}

		if !sampler(in[:nInputs], out[:nOutputs], cargo) {
			return false
		}

		if dwFlags&samplerInspect == 0 {
			if clut.tab16 != nil {
				for t := uint32(0); t < nOutputs; t++ {
					clut.tab16[index+int(t)] = out[t]
				}
			}
		}

		index += int(nOutputs)
	}

	return true
}

// SampleCLutFloat ports cmsStageSampleCLutFloat.
func (mpe *Stage) SampleCLutFloat(sampler SamplerFloat, cargo any, dwFlags uint32) bool {
	if mpe == nil {
		return false
	}
	clut, ok := mpe.data.(*stageCLutData)
	if !ok {
		return false
	}

	p := clut.params
	nInputs := p.NumInputs
	nOutputs := p.NumOutputs

	if nInputs == 0 || nOutputs == 0 {
		return false
	}
	if nInputs > maxInputDimensions {
		return false
	}
	if nOutputs >= maxStageChannels {
		return false
	}

	var in [maxInputDimensions + 1]float32
	var out [maxStageChannels]float32

	nTotalPoints := cubeSize(p.NumSamples[:], nInputs)
	if nTotalPoints == 0 {
		return false
	}

	index := 0
	for i := uint32(0); i < nTotalPoints; i++ {
		rest := i
		for t := int(nInputs) - 1; t >= 0; t-- {
			colorant := rest % p.NumSamples[t]
			rest /= p.NumSamples[t]
			in[t] = float32(quantizeVal(float64(colorant), p.NumSamples[t])) / 65535.0
		}

		if clut.tabFloat != nil {
			for t := uint32(0); t < nOutputs; t++ {
				out[t] = clut.tabFloat[index+int(t)]
			}
		}

		if !sampler(in[:nInputs], out[:nOutputs], cargo) {
			return false
		}

		if dwFlags&samplerInspect == 0 {
			if clut.tabFloat != nil {
				for t := uint32(0); t < nOutputs; t++ {
					clut.tabFloat[index+int(t)] = out[t]
				}
			}
		}

		index += int(nOutputs)
	}

	return true
}

// *************************************************************************
// Lab <-> XYZ converter stages
// *************************************************************************

// evaluateLab2XYZ ports EvaluateLab2XYZ (V4 rules).
func evaluateLab2XYZ(in, out []float32, mpe *Stage) {
	const xyzAdj = maxEncodeableXYZ
	var lab CIELab
	lab.L = float64(in[0]) * 100.0
	lab.A = float64(in[1])*255.0 - 128.0
	lab.B = float64(in[2])*255.0 - 128.0

	xyz := Lab2XYZ(nil, lab)

	out[0] = float32(xyz.X / xyzAdj)
	out[1] = float32(xyz.Y / xyzAdj)
	out[2] = float32(xyz.Z / xyzAdj)
}

// stageAllocLab2XYZ ports _cmsStageAllocLab2XYZ.
func (ctx *Context) stageAllocLab2XYZ() *Stage {
	mpe := ctx.stageAllocPlaceholder(SigLab2XYZElemType, 3, 3, evaluateLab2XYZ, nil)
	mpe.concrete = true
	return mpe
}

// evaluateXYZ2Lab ports EvaluateXYZ2Lab.
func evaluateXYZ2Lab(in, out []float32, mpe *Stage) {
	const xyzAdj = maxEncodeableXYZ
	var xyz CIEXYZ
	xyz.X = float64(in[0]) * xyzAdj
	xyz.Y = float64(in[1]) * xyzAdj
	xyz.Z = float64(in[2]) * xyzAdj

	lab := XYZ2Lab(nil, xyz)

	out[0] = float32(lab.L / 100.0)
	out[1] = float32((lab.A + 128.0) / 255.0)
	out[2] = float32((lab.B + 128.0) / 255.0)
}

// stageAllocXYZ2Lab ports _cmsStageAllocXYZ2Lab.
func (ctx *Context) stageAllocXYZ2Lab() *Stage {
	mpe := ctx.stageAllocPlaceholder(SigXYZ2LabElemType, 3, 3, evaluateXYZ2Lab, nil)
	mpe.concrete = true
	return mpe
}

// stageAllocLabV2ToV4curves ports _cmsStageAllocLabV2ToV4curves.
func (ctx *Context) stageAllocLabV2ToV4curves() (*Stage, error) {
	labTable := make([]*ToneCurve, 3)
	for j := 0; j < 3; j++ {
		c, err := ctx.BuildTabulatedToneCurve16(make([]uint16, 258))
		if err != nil {
			return nil, err
		}
		// Map * (0xffff / 0xff00), same as (257/256), via 258-entry table.
		for i := 0; i < 257; i++ {
			c.table16[i] = uint16((i*0xffff + 0x80) >> 8)
		}
		c.table16[257] = 0xffff
		labTable[j] = c
	}
	mpe, err := ctx.StageAllocToneCurves(3, labTable)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigLabV2toV4
	return mpe, nil
}

// stageAllocLabV2ToV4 ports _cmsStageAllocLabV2ToV4 (matrix based).
func (ctx *Context) stageAllocLabV2ToV4() (*Stage, error) {
	const k = 65535.0 / 65280.0
	v2ToV4 := []float64{k, 0, 0, 0, k, 0, 0, 0, k}
	mpe, err := ctx.StageAllocMatrix(3, 3, v2ToV4, nil)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigLabV2toV4
	return mpe, nil
}

// stageAllocLabV4ToV2 ports _cmsStageAllocLabV4ToV2.
func (ctx *Context) stageAllocLabV4ToV2() (*Stage, error) {
	const k = 65280.0 / 65535.0
	v4ToV2 := []float64{k, 0, 0, 0, k, 0, 0, 0, k}
	mpe, err := ctx.StageAllocMatrix(3, 3, v4ToV2, nil)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigLabV4toV2
	return mpe, nil
}

// stageNormalizeFromLabFloat ports _cmsStageNormalizeFromLabFloat.
func (ctx *Context) stageNormalizeFromLabFloat() (*Stage, error) {
	a1 := []float64{
		1.0 / 100.0, 0, 0,
		0, 1.0 / 255.0, 0,
		0, 0, 1.0 / 255.0,
	}
	o1 := []float64{0, 128.0 / 255.0, 128.0 / 255.0}
	mpe, err := ctx.StageAllocMatrix(3, 3, a1, o1)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigLab2FloatPCS
	return mpe, nil
}

// stageNormalizeFromXyzFloat ports _cmsStageNormalizeFromXyzFloat.
func (ctx *Context) stageNormalizeFromXyzFloat() (*Stage, error) {
	const n = 32768.0 / 65535.0
	a1 := []float64{n, 0, 0, 0, n, 0, 0, 0, n}
	mpe, err := ctx.StageAllocMatrix(3, 3, a1, nil)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigXYZ2FloatPCS
	return mpe, nil
}

// stageNormalizeToLabFloat ports _cmsStageNormalizeToLabFloat.
func (ctx *Context) stageNormalizeToLabFloat() (*Stage, error) {
	a1 := []float64{100.0, 0, 0, 0, 255.0, 0, 0, 0, 255.0}
	o1 := []float64{0, -128.0, -128.0}
	mpe, err := ctx.StageAllocMatrix(3, 3, a1, o1)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigFloatPCS2Lab
	return mpe, nil
}

// stageNormalizeToXyzFloat ports _cmsStageNormalizeToXyzFloat.
func (ctx *Context) stageNormalizeToXyzFloat() (*Stage, error) {
	const n = 65535.0 / 32768.0
	a1 := []float64{n, 0, 0, 0, n, 0, 0, 0, n}
	mpe, err := ctx.StageAllocMatrix(3, 3, a1, nil)
	if err != nil {
		return nil, err
	}
	mpe.Implements = SigFloatPCS2XYZ
	return mpe, nil
}

// clipper ports Clipper: clips values smaller than zero.
func clipper(in, out []float32, mpe *Stage) {
	for i := uint32(0); i < mpe.InputChannels; i++ {
		n := in[i]
		if n < 0 {
			out[i] = 0
		} else {
			out[i] = n
		}
	}
}

// stageClipNegatives ports _cmsStageClipNegatives.
func (ctx *Context) stageClipNegatives(nChannels uint32) *Stage {
	mpe := ctx.stageAllocPlaceholder(SigClipNegativesElemType, nChannels, nChannels, clipper, nil)
	mpe.concrete = true
	return mpe
}

// stageAllocLabPrelin ports _cmsStageAllocLabPrelin.
func (ctx *Context) stageAllocLabPrelin() (*Stage, error) {
	labTable := make([]*ToneCurve, 3)
	c0, err := ctx.BuildGamma(1.0)
	if err != nil {
		return nil, err
	}
	labTable[0] = c0
	for j := 1; j < 3; j++ {
		c, err := ctx.BuildParametricToneCurve(108, []float64{2.4})
		if err != nil {
			return nil, err
		}
		labTable[j] = c
	}
	return ctx.StageAllocToneCurves(3, labTable)
}

// Free ports cmsStageFree: a no-op under the Go garbage collector.
func (mpe *Stage) Free() {}

// stageDup ports cmsStageDup: an independent deep copy of a stage.
func (mpe *Stage) stageDup() (*Stage, error) {
	if mpe == nil {
		return nil, nil
	}
	dup := &Stage{
		ctx:            mpe.ctx,
		Type:           mpe.Type,
		Implements:     mpe.Implements,
		InputChannels:  mpe.InputChannels,
		OutputChannels: mpe.OutputChannels,
		eval:           mpe.eval,
		concrete:       mpe.concrete,
	}

	switch d := mpe.data.(type) {
	case nil:
		// Stateless stage (identity / Lab<->XYZ / clip): nothing to copy.
	case *stageToneCurvesData:
		nd := &stageToneCurvesData{nCurves: d.nCurves, theCurves: make([]*ToneCurve, d.nCurves)}
		for i := uint32(0); i < d.nCurves; i++ {
			c, err := d.theCurves[i].Dup()
			if err != nil {
				return nil, err
			}
			nd.theCurves[i] = c
		}
		dup.data = nd
	case *stageMatrixData:
		nd := &stageMatrixData{double: append([]float64(nil), d.double...)}
		if d.offset != nil {
			nd.offset = append([]float64(nil), d.offset...)
		}
		dup.data = nd
	case *stageCLutData:
		nd := &stageCLutData{
			nEntries:       d.nEntries,
			hasFloatValues: d.hasFloatValues,
			kind:           d.kind,
		}
		var table any
		if d.hasFloatValues {
			nd.tabFloat = append([]float32(nil), d.tabFloat...)
			table = nd.tabFloat
		} else {
			nd.tab16 = append([]uint16(nil), d.tab16...)
			table = nd.tab16
		}
		p, err := computeInterpParamsEx(mpe.ctx, d.params.NumSamples[:d.params.NumInputs],
			d.params.NumInputs, d.params.NumOutputs, table, d.params.Flags)
		if err != nil {
			return nil, err
		}
		nd.params = p
		dup.data = nd
	case *NamedColorList:
		// Named-color lookup stage (cmsSigNamedColorElemType): deep-copy the list,
		// mirroring DupNamedColorList in cmsnamed.c.
		dup.data = d.Dup()
	default:
		return nil, mpe.ctx.signalError(ErrInternal, "cmsStageDup: unknown stage payload %T", mpe.data)
	}

	return dup, nil
}

// ***********************************************************************
// Pipeline
// ***********************************************************************

// pipelineEval16Fn / pipelineEvalFloatFn mirror _cmsPipelineEval16Fn /
// _cmsPipelineEvalFloatFn.
type pipelineEval16Fn func(in, out []uint16, data any)
type pipelineEvalFloatFn func(in, out []float32, data any)

// Pipeline mirrors cmsPipeline (struct _cmsPipeline_struct).
type Pipeline struct {
	ctx *Context

	elements *Stage

	InputChannels  uint32
	OutputChannels uint32

	eval16Fn    pipelineEval16Fn
	evalFloatFn pipelineEvalFloatFn
	data        any // private data for the evaluator (the pipeline itself by default)

	dupDataFn  func(ctx *Context, data any) any
	freeDataFn func(ctx *Context, data any)

	saveAs8Bits bool
}

// blessLUT ports BlessLUT: derive the pipeline channel counts from its stages
// and verify chain consistency. It also selects the concrete or generic
// evaluators depending on whether every stage is concretely dispatchable.
func (lut *Pipeline) blessLUT() bool {
	allConcrete := true

	if lut.elements != nil {
		first := lut.elements
		last := lut.GetPtrToLastStage()
		if first == nil || last == nil {
			return false
		}

		lut.InputChannels = first.InputChannels
		lut.OutputChannels = last.OutputChannels

		prev := first
		next := prev.next
		for next != nil {
			if next.InputChannels != prev.OutputChannels {
				return false
			}
			next = next.next
			prev = prev.next
		}

		for mpe := lut.elements; mpe != nil; mpe = mpe.next {
			if !mpe.concrete {
				allConcrete = false
			}
		}
	}

	// Only refresh the built-in evaluators; leave any evaluator installed by
	// _cmsPipelineSetOptimizationParameters untouched (data != the pipeline).
	if _, isSelf := lut.data.(*Pipeline); isSelf || lut.data == nil {
		if allConcrete {
			lut.eval16Fn = lutEval16
			lut.evalFloatFn = lutEvalFloat
		} else {
			lut.eval16Fn = lutEval16Generic
			lut.evalFloatFn = lutEvalFloatGeneric
		}
	}

	return true
}

// evalStageFloat dispatches a stage evaluation concretely (no indirect call),
// so the caller's scratch buffer is not forced onto the heap.
func evalStageFloat(mpe *Stage, in, out []float32) {
	switch mpe.Type {
	case SigCurveSetElemType:
		evaluateCurves(in, out, mpe)
	case SigMatrixElemType:
		evaluateMatrix(in, out, mpe)
	case SigCLutElemType:
		if d, ok := mpe.data.(*stageCLutData); ok && d.hasFloatValues {
			evaluateCLUTfloat(in, out, mpe)
		} else {
			evaluateCLUTfloatIn16(in, out, mpe)
		}
	case SigLab2XYZElemType:
		evaluateLab2XYZ(in, out, mpe)
	case SigXYZ2LabElemType:
		evaluateXYZ2Lab(in, out, mpe)
	case SigClipNegativesElemType:
		clipper(in, out, mpe)
	case SigIdentityElemType:
		evaluateIdentity(in, out, mpe)
	default:
		// Unreachable for pipelines built from the constructors in this file;
		// a harmless identity copy keeps the library panic-free.
		n := mpe.OutputChannels
		if mpe.InputChannels < n {
			n = mpe.InputChannels
		}
		copy(out[:n], in[:n])
	}
}

// lutEval16 ports _LUTeval16 (concrete, allocation-free path).
func lutEval16(in, out []uint16, data any) {
	lut, ok := data.(*Pipeline)
	if !ok {
		return
	}
	var storage [2][maxStageChannels]float32
	phase := 0
	from16ToFloat(in, storage[phase][:], lut.InputChannels)
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		next := phase ^ 1
		evalStageFloat(mpe, storage[phase][:], storage[next][:])
		phase = next
	}
	fromFloatTo16(storage[phase][:], out, lut.OutputChannels)
}

// lutEvalFloat ports _LUTevalFloat (concrete, allocation-free path).
func lutEvalFloat(in, out []float32, data any) {
	lut, ok := data.(*Pipeline)
	if !ok {
		return
	}
	var storage [2][maxStageChannels]float32
	phase := 0
	copy(storage[phase][:lut.InputChannels], in[:lut.InputChannels])
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		next := phase ^ 1
		evalStageFloat(mpe, storage[phase][:], storage[next][:])
		phase = next
	}
	copy(out[:lut.OutputChannels], storage[phase][:lut.OutputChannels])
}

// evalStageFloatGeneric dispatches through the stage EvalPtr func value; used
// only when the pipeline holds a plug-in-supplied interpolator.
func evalStageFloatGeneric(mpe *Stage, in, out []float32) {
	if mpe.Type == SigCLutElemType {
		if d, ok := mpe.data.(*stageCLutData); ok && d.hasFloatValues {
			evaluateCLUTfloatGeneric(in, out, mpe)
		} else {
			evaluateCLUTfloatIn16Generic(in, out, mpe)
		}
		return
	}
	if mpe.eval != nil {
		mpe.eval(in, out, mpe)
	}
}

// lutEval16Generic is the func-value fallback of lutEval16.
func lutEval16Generic(in, out []uint16, data any) {
	lut, ok := data.(*Pipeline)
	if !ok {
		return
	}
	var storage [2][maxStageChannels]float32
	phase := 0
	from16ToFloat(in, storage[phase][:], lut.InputChannels)
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		next := phase ^ 1
		evalStageFloatGeneric(mpe, storage[phase][:], storage[next][:])
		phase = next
	}
	fromFloatTo16(storage[phase][:], out, lut.OutputChannels)
}

// lutEvalFloatGeneric is the func-value fallback of lutEvalFloat.
func lutEvalFloatGeneric(in, out []float32, data any) {
	lut, ok := data.(*Pipeline)
	if !ok {
		return
	}
	var storage [2][maxStageChannels]float32
	phase := 0
	copy(storage[phase][:lut.InputChannels], in[:lut.InputChannels])
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		next := phase ^ 1
		evalStageFloatGeneric(mpe, storage[phase][:], storage[next][:])
		phase = next
	}
	copy(out[:lut.OutputChannels], storage[phase][:lut.OutputChannels])
}

// PipelineAlloc ports cmsPipelineAlloc: a value of zero channels is allowed as
// a placeholder.
func (ctx *Context) PipelineAlloc(inputChannels, outputChannels uint32) (*Pipeline, error) {
	if inputChannels >= maxChannels || outputChannels >= maxChannels {
		return nil, ctx.signalError(ErrRange, "cmsPipelineAlloc: too many channels (%d->%d)", inputChannels, outputChannels)
	}

	lut := &Pipeline{
		ctx:            ctx,
		InputChannels:  inputChannels,
		OutputChannels: outputChannels,
		eval16Fn:       lutEval16,
		evalFloatFn:    lutEvalFloat,
	}
	lut.data = lut

	if !lut.blessLUT() {
		return nil, ctx.signalError(ErrInternal, "cmsPipelineAlloc: inconsistent pipeline")
	}
	return lut, nil
}

// PipelineAlloc builds a pipeline on the default context.
func PipelineAlloc(inputChannels, outputChannels uint32) (*Pipeline, error) {
	return defaultContext.PipelineAlloc(inputChannels, outputChannels)
}

// ContextID ports cmsGetPipelineContextID.
func (lut *Pipeline) ContextID() *Context { return lut.ctx }

// InputChannelsCount ports cmsPipelineInputChannels.
func (lut *Pipeline) InputChannelsCount() uint32 { return lut.InputChannels }

// OutputChannelsCount ports cmsPipelineOutputChannels.
func (lut *Pipeline) OutputChannelsCount() uint32 { return lut.OutputChannels }

// Free ports cmsPipelineFree: a no-op under the Go garbage collector (the
// optional FreeDataFn is still honoured for parity with optimization plug-ins).
func (lut *Pipeline) Free() {
	if lut == nil {
		return
	}
	if lut.freeDataFn != nil {
		lut.freeDataFn(lut.ctx, lut.data)
	}
}

// Eval16 ports cmsPipelineEval16.
func (lut *Pipeline) Eval16(in, out []uint16) {
	lut.eval16Fn(in, out, lut.data)
}

// EvalFloat ports cmsPipelineEvalFloat. Note the asymmetry with Eval16: the
// reference passes the pipeline itself here (only the 16-bit path receives
// lut->Data), so an optimization plug-in's private data feeds the 16-bit
// evaluator only.
func (lut *Pipeline) EvalFloat(in, out []float32) {
	lut.evalFloatFn(in, out, lut)
}

// Dup ports cmsPipelineDup: a deep, independent copy.
func (lut *Pipeline) Dup() (*Pipeline, error) {
	if lut == nil {
		return nil, nil
	}
	newLUT, err := lut.ctx.PipelineAlloc(lut.InputChannels, lut.OutputChannels)
	if err != nil {
		return nil, err
	}

	var anterior *Stage
	first := true
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		newMPE, err := mpe.stageDup()
		if err != nil {
			return nil, err
		}
		if first {
			newLUT.elements = newMPE
			first = false
		} else if anterior != nil {
			anterior.next = newMPE
		}
		anterior = newMPE
	}

	newLUT.eval16Fn = lut.eval16Fn
	newLUT.evalFloatFn = lut.evalFloatFn
	newLUT.dupDataFn = lut.dupDataFn
	newLUT.freeDataFn = lut.freeDataFn

	if lut.dupDataFn != nil {
		newLUT.data = lut.dupDataFn(lut.ctx, lut.data)
	}

	newLUT.saveAs8Bits = lut.saveAs8Bits

	if !newLUT.blessLUT() {
		return nil, lut.ctx.signalError(ErrInternal, "cmsPipelineDup: inconsistent pipeline")
	}
	return newLUT, nil
}

// InsertStage ports cmsPipelineInsertStage.
func (lut *Pipeline) InsertStage(loc StageLoc, mpe *Stage) error {
	if lut == nil || mpe == nil {
		return errorf(ErrNull, "cmsPipelineInsertStage: nil pipeline or stage")
	}

	switch loc {
	case AtBegin:
		mpe.next = lut.elements
		lut.elements = mpe
	case AtEnd:
		if lut.elements == nil {
			lut.elements = mpe
		} else {
			var anterior *Stage
			for pt := lut.elements; pt != nil; pt = pt.next {
				anterior = pt
			}
			anterior.next = mpe
			mpe.next = nil
		}
	default:
		return errorf(ErrRange, "cmsPipelineInsertStage: bad location")
	}

	if !lut.blessLUT() {
		return lut.ctx.signalError(ErrColorspaceCheck, "cmsPipelineInsertStage: channel count mismatch")
	}
	return nil
}

// UnlinkStage ports cmsPipelineUnlinkStage: remove a stage and return it.
func (lut *Pipeline) UnlinkStage(loc StageLoc) *Stage {
	if lut.elements == nil {
		return nil
	}

	var unlinked *Stage
	switch loc {
	case AtBegin:
		elem := lut.elements
		lut.elements = elem.next
		elem.next = nil
		unlinked = elem
	case AtEnd:
		var anterior, last *Stage
		for pt := lut.elements; pt != nil; pt = pt.next {
			anterior = last
			last = pt
		}
		unlinked = last
		if anterior != nil {
			anterior.next = nil
		} else {
			lut.elements = nil
		}
	default:
	}

	lut.blessLUT() // may fail, ignored (matches the reference)
	return unlinked
}

// Cat ports cmsPipelineCat: append a deep copy of l2's stages to lut.
func (lut *Pipeline) Cat(l2 *Pipeline) error {
	if lut == nil || l2 == nil {
		return errorf(ErrNull, "cmsPipelineCat: nil pipeline")
	}
	// If both are empty, inherit the channel counts.
	if lut.elements == nil && l2.elements == nil {
		lut.InputChannels = l2.InputChannels
		lut.OutputChannels = l2.OutputChannels
	}

	for mpe := l2.elements; mpe != nil; mpe = mpe.next {
		dup, err := mpe.stageDup()
		if err != nil {
			return err
		}
		if err := lut.InsertStage(AtEnd, dup); err != nil {
			return err
		}
	}

	if !lut.blessLUT() {
		return lut.ctx.signalError(ErrColorspaceCheck, "cmsPipelineCat: channel count mismatch")
	}
	return nil
}

// SetSaveAs8bitsFlag ports cmsPipelineSetSaveAs8bitsFlag: sets the flag and
// returns its previous value.
func (lut *Pipeline) SetSaveAs8bitsFlag(on bool) bool {
	prev := lut.saveAs8Bits
	lut.saveAs8Bits = on
	return prev
}

// GetPtrToFirstStage ports cmsPipelineGetPtrToFirstStage.
func (lut *Pipeline) GetPtrToFirstStage() *Stage { return lut.elements }

// GetPtrToLastStage ports cmsPipelineGetPtrToLastStage.
func (lut *Pipeline) GetPtrToLastStage() *Stage {
	var anterior *Stage
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		anterior = mpe
	}
	return anterior
}

// StageCount ports cmsPipelineStageCount.
func (lut *Pipeline) StageCount() uint32 {
	var n uint32
	for mpe := lut.elements; mpe != nil; mpe = mpe.next {
		n++
	}
	return n
}

// CheckAndRetrieveStages ports cmsPipelineCheckAndRetreiveStages: if the
// pipeline's stage types match types exactly (in order), return the stages and
// true; otherwise nil and false.
func (lut *Pipeline) CheckAndRetrieveStages(types ...StageSignature) ([]*Stage, bool) {
	if lut.StageCount() != uint32(len(types)) {
		return nil, false
	}
	stages := make([]*Stage, 0, len(types))
	mpe := lut.elements
	for _, want := range types {
		if mpe.Type != want {
			return nil, false
		}
		stages = append(stages, mpe)
		mpe = mpe.next
	}
	return stages, true
}

// setOptimizationParameters ports _cmsPipelineSetOptimizationParameters: install
// an optional evaluator and private data (used by opt.go, Phase 4).
func (lut *Pipeline) setOptimizationParameters(eval16 pipelineEval16Fn, privateData any,
	freeFn func(ctx *Context, data any), dupFn func(ctx *Context, data any) any) {
	lut.eval16Fn = eval16
	lut.dupDataFn = dupFn
	lut.freeDataFn = freeFn
	lut.data = privateData
}

// ---------------------------------------------------------- Reverse interpolation

const (
	jacobianEpsilon        = 0.001
	inversionMaxIterations = 30
)

// incDelta ports IncDelta: increment with reflexion on the boundary.
func incDelta(val float32) float32 {
	if val < (1.0 - jacobianEpsilon) {
		return val + jacobianEpsilon
	}
	return val - jacobianEpsilon
}

// euclideanDistance ports EuclideanDistance.
func euclideanDistance(a, b []float32, n int) float32 {
	var sum float32
	for i := 0; i < n; i++ {
		dif := b[i] - a[i]
		sum += dif * dif
	}
	return float32(math.Sqrt(float64(sum)))
}

// EvalReverseFloat ports cmsPipelineEvalReverseFloat: Newton-Raphson reverse
// evaluation for 3->3 and 4->3 pipelines. Returns false when the pipeline is
// not invertible in this way or the Jacobian is singular.
func (lut *Pipeline) EvalReverseFloat(target, result, hint []float32) bool {
	// Only 3->3 and 4->3 are supported.
	if lut.InputChannels != 3 && lut.InputChannels != 4 {
		return false
	}
	if lut.OutputChannels != 3 {
		return false
	}
	// target must supply InputChannels values (the 4->3 path reads target[3] as
	// the passed-through K) and result must hold them; a short slice from a
	// caller must fail rather than index out of range.
	if len(target) < int(lut.InputChannels) || len(result) < int(lut.InputChannels) {
		return false
	}
	if hint != nil && len(hint) < 3 {
		return false
	}

	var fx, x, xd, fxd [4]float32
	lastError := 1e20

	if hint == nil {
		x[0], x[1], x[2] = 0.3, 0.3, 0.3
	} else {
		for j := 0; j < 3; j++ {
			x[j] = hint[j]
		}
	}

	if lut.InputChannels == 4 {
		x[3] = target[3]
	} else {
		x[3] = 0
	}

	for i := 0; i < inversionMaxIterations; i++ {
		lut.EvalFloat(x[:lut.InputChannels], fx[:])

		errDist := float64(euclideanDistance(fx[:], target, 3))

		// If not convergent, return the last safe value.
		if errDist >= lastError {
			break
		}

		lastError = errDist
		for j := uint32(0); j < lut.InputChannels; j++ {
			result[j] = x[j]
		}

		if errDist <= 0 {
			break
		}

		// Obtain the Jacobian.
		var jacobian MAT3
		for j := 0; j < 3; j++ {
			xd[0], xd[1], xd[2], xd[3] = x[0], x[1], x[2], x[3]
			xd[j] = incDelta(xd[j])

			lut.EvalFloat(xd[:lut.InputChannels], fxd[:])

			jacobian[0][j] = float64((fxd[0] - fx[0]) / jacobianEpsilon)
			jacobian[1][j] = float64((fxd[1] - fx[1]) / jacobianEpsilon)
			jacobian[2][j] = float64((fxd[2] - fx[2]) / jacobianEpsilon)
		}

		var b VEC3
		b[0] = float64(fx[0] - target[0])
		b[1] = float64(fx[1] - target[1])
		b[2] = float64(fx[2] - target[2])

		sol, ok := MAT3Solve(jacobian, b)
		if !ok {
			return false
		}

		x[0] -= float32(sol[0])
		x[1] -= float32(sol[1])
		x[2] -= float32(sol[2])

		// Clip.
		for j := 0; j < 3; j++ {
			if x[j] < 0 {
				x[j] = 0
			} else if x[j] > 1.0 {
				x[j] = 1.0
			}
		}
	}

	return true
}
