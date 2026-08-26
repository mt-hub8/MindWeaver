// Port of the multi-processing-element (MPE) machinery from src/cmstypes.c
// (lcms2 2.19): the Type_MPE container ('mpet'), its element-type registry
// (the ctx.mpeType plug-in chunk plus the built-in SupportedMPEtypes table),
// the built-in element handlers Type_MPEcurve ('cvst' segmented curves),
// Type_MPEmatrix ('matf') and Type_MPEclut ('clut'), the accepted-as-noop
// bACS/eACS placeholders, the segmented-curve codec (ReadSegmentedCurve /
// WriteSegmentedCurve) and the ICC position-table helpers (ReadPositionTable /
// WritePositionTable) shared with several tag types.
//
// tagtype.go extension flagged in the report: the MPE element registry
// (builtinMPETypes / registerBuiltinMPEType / Context.getMPETypeHandler) lives
// here because tagtype.go provides no MPE-type table. The context already
// carries the mpeType plug-in chunk (context.go), so no context change is
// needed; getMPETypeHandler mirrors getTagTypeHandler against that chunk.

package lcms2

// ---------------------------------------------------------------- MPE registry

// builtinMPETypes is the built-in MPE element-type table, the analogue of the
// static SupportedMPEtypes array in cmstypes.c.
var builtinMPETypes = map[TagTypeSignature]*TagTypeHandler{}

// registerBuiltinMPEType installs h as the built-in handler for its MPE element
// signature.
func registerBuiltinMPEType(h *TagTypeHandler) {
	builtinMPETypes[h.Signature] = h
}

// getMPETypeHandler ports GetHandler(sig, MPETypePluginChunk->TagTypes,
// SupportedMPEtypes): consult the context's registered MPE plug-ins (newest
// first) then the built-in table. Returns nil when no handler exists.
func (ctx *Context) getMPETypeHandler(sig TagTypeSignature) *TagTypeHandler {
	if ctx != nil {
		ctx.mu.Lock()
		entries := ctx.mpeType.entries
		ctx.mu.Unlock()
		for _, p := range entries {
			if pt, ok := p.(*PluginTagType); ok && pt.Handler.Signature == sig {
				return &pt.Handler
			}
		}
	}
	if h, ok := builtinMPETypes[sig]; ok {
		return h
	}
	return nil
}

// ------------------------------------------------------------- position tables

// positionTableEntryFn aliases positionEntryFn (types_base.go), which owns the
// shared ReadPositionTable/WritePositionTable port.
type positionTableEntryFn = positionEntryFn

// ------------------------------------------------------------ segmented curves

// Curve-segment signatures (include/lcms2.h cmsCurveSegSignature).
const (
	sigFormulaCurveSeg    TagTypeSignature = 0x70617266 // 'parf'
	sigSampledCurveSeg    TagTypeSignature = 0x73616D66 // 'samf'
	sigSegmentedCurve     TagTypeSignature = 0x63757266 // 'curf'
	sigBAcsElemTypeTT     TagTypeSignature = 0x62414353 // 'bACS'
	sigEAcsElemTypeTT     TagTypeSignature = 0x65414353 // 'eACS'
	sigCurveSetElemTypeTT TagTypeSignature = 0x63767374 // 'cvst'
	sigMatrixElemTypeTT   TagTypeSignature = 0x6D617466 // 'matf'
	sigCLutElemTypeTT     TagTypeSignature = 0x636C7574 // 'clut'
)

// readSegmentedCurve ports ReadSegmentedCurve. The sentinel domain bounds
// MINUS_INF / PLUS_INF are gamma.go's curveMinusInf / curvePlusInf.
func readSegmentedCurve(self *TagTypeHandler, io *IOHandler) (*ToneCurve, error) {
	ctx := self.ContextID

	elementSig, ok := readUInt32(io)
	if !ok {
		return nil, errCorrupt()
	}
	if TagTypeSignature(elementSig) != sigSegmentedCurve {
		return nil, errCorrupt()
	}
	if _, ok := readUInt32(io); !ok { // reserved
		return nil, errCorrupt()
	}
	nSegments, ok := readUInt16(io)
	if !ok {
		return nil, errCorrupt()
	}
	if _, ok := readUInt16(io); !ok { // reserved
		return nil, errCorrupt()
	}
	if nSegments < 1 {
		return nil, errCorrupt()
	}

	segments := make([]CurveSegment, nSegments)
	prevBreak := curveMinusInf

	// Read breakpoints.
	for i := 0; i < int(nSegments)-1; i++ {
		segments[i].X0 = prevBreak
		v, ok := readFloat32(io)
		if !ok {
			return nil, errCorrupt()
		}
		segments[i].X1 = v
		prevBreak = v
	}
	segments[nSegments-1].X0 = prevBreak
	segments[nSegments-1].X1 = curvePlusInf

	paramsByType := [3]uint32{4, 5, 5}

	for i := uint32(0); i < uint32(nSegments); i++ {
		segSig, ok := readUInt32(io)
		if !ok {
			return nil, errCorrupt()
		}
		if _, ok := readUInt32(io); !ok { // reserved
			return nil, errCorrupt()
		}

		switch TagTypeSignature(segSig) {
		case sigFormulaCurveSeg:
			typ, ok := readUInt16(io)
			if !ok {
				return nil, errCorrupt()
			}
			if _, ok := readUInt16(io); !ok { // reserved
				return nil, errCorrupt()
			}
			segments[i].Type = int32(typ) + 6
			if typ > 2 {
				return nil, errCorrupt()
			}
			for j := uint32(0); j < paramsByType[typ]; j++ {
				f, ok := readFloat32(io)
				if !ok {
					return nil, errCorrupt()
				}
				segments[i].Params[j] = float64(f)
			}

		case sigSampledCurveSeg:
			count, ok := readUInt32(io)
			if !ok {
				return nil, errCorrupt()
			}
			// The first point is implicit; one extra node is populated later.
			if count == uipowSentinel {
				return nil, errCorrupt()
			}
			count++
			// Bound the allocation: each remaining sample is a 4-byte float on
			// disk, so it cannot exceed the profile size.
			if count > io.ReportedSize/4+1 {
				return nil, errCorrupt()
			}
			segments[i].NGridPoints = count
			segments[i].SampledPoints = make([]float32, count)
			segments[i].SampledPoints[0] = 0
			for j := uint32(1); j < count; j++ {
				f, ok := readFloat32(io)
				if !ok {
					return nil, errCorrupt()
				}
				segments[i].SampledPoints[j] = f
			}

		default:
			return nil, ctx.signalError(ErrUnknownExtension, "Unknown curve element type '%s' found.", TagTypeSignature(segSig))
		}
	}

	curve, err := ctx.BuildSegmentedToneCurve(segments)
	if err != nil {
		return nil, err
	}

	// Explore for missing implicit points: fix the first sample of each sampled
	// segment (mirrors the ReadSegmentedCurve post-pass).
	for i := uint32(0); i < uint32(nSegments); i++ {
		if curve.segments[i].Type == 0 && len(curve.segments[i].SampledPoints) > 0 {
			curve.segments[i].SampledPoints[0] = curve.EvalFloat(curve.segments[i].X0)
		}
	}

	return curve, nil
}

// writeSegmentedCurve ports WriteSegmentedCurve. No validity check is performed,
// matching the reference.
func writeSegmentedCurve(io *IOHandler, g *ToneCurve) error {
	segments := g.segments
	nSegments := g.nSegments

	if !writeUInt32(io, uint32(sigSegmentedCurve)) {
		return errWrite()
	}
	if !writeUInt32(io, 0) {
		return errWrite()
	}
	if !writeUInt16(io, uint16(nSegments)) {
		return errWrite()
	}
	if !writeUInt16(io, 0) {
		return errWrite()
	}

	// Break-points.
	for i := uint32(0); i < nSegments-1; i++ {
		if !writeFloat32(io, segments[i].X1) {
			return errWrite()
		}
	}

	paramsByType := [3]uint32{4, 5, 5}

	for i := uint32(0); i < nSegments; i++ {
		seg := &segments[i]

		if seg.Type == 0 {
			// Sampled curve; the first point is implicit in the ICC format.
			if !writeUInt32(io, uint32(sigSampledCurveSeg)) {
				return errWrite()
			}
			if !writeUInt32(io, 0) {
				return errWrite()
			}
			if !writeUInt32(io, seg.NGridPoints-1) {
				return errWrite()
			}
			for j := uint32(1); j < seg.NGridPoints; j++ {
				if !writeFloat32(io, seg.SampledPoints[j]) {
					return errWrite()
				}
			}
		} else {
			if !writeUInt32(io, uint32(sigFormulaCurveSeg)) {
				return errWrite()
			}
			if !writeUInt32(io, 0) {
				return errWrite()
			}
			typ := seg.Type - 6
			if typ > 2 || typ < 0 {
				return errWrite()
			}
			if !writeUInt16(io, uint16(typ)) {
				return errWrite()
			}
			if !writeUInt16(io, 0) {
				return errWrite()
			}
			for j := uint32(0); j < paramsByType[typ]; j++ {
				if !writeFloat32(io, float32(seg.Params[j])) {
					return errWrite()
				}
			}
		}
	}
	return nil
}

// ----------------------------------------------------------- Type_MPEcurve

// readMPECurve ports ReadMPECurve: the position-table callback reading one
// segmented curve into the cargo []*ToneCurve.
func readMPECurve(self *TagTypeHandler, io *IOHandler, cargo any, n, sizeOfTag uint32) error {
	gammaTables, ok := cargo.([]*ToneCurve)
	if !ok || uint32(len(gammaTables)) <= n {
		return errCorrupt()
	}
	c, err := readSegmentedCurve(self, io)
	if err != nil {
		return err
	}
	gammaTables[n] = c
	return nil
}

// typeMPEcurveRead ports Type_MPEcurve_Read.
func typeMPEcurveRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID
	baseOffset := io.Tell() - tagBaseSize

	inputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if inputChans != outputChans {
		return nil, 0, errCorrupt()
	}
	if inputChans == 0 {
		return nil, 0, errCorrupt()
	}

	gammaTables := make([]*ToneCurve, inputChans)
	if err := readPositionTable(self, io, uint32(inputChans), baseOffset, gammaTables, readMPECurve); err != nil {
		return nil, 0, err
	}
	mpe, err := ctx.StageAllocToneCurves(uint32(inputChans), gammaTables)
	if err != nil {
		return nil, 0, err
	}
	return mpe, 1, nil
}

// writeMPECurve ports WriteMPECurve.
func writeMPECurve(self *TagTypeHandler, io *IOHandler, cargo any, n, sizeOfTag uint32) error {
	curves, ok := cargo.(*stageToneCurvesData)
	if !ok || uint32(len(curves.theCurves)) <= n {
		return errCorrupt()
	}
	return writeSegmentedCurve(io, curves.theCurves[n])
}

// typeMPEcurveWrite ports Type_MPEcurve_Write.
func typeMPEcurveWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mpe, ok := value.(*Stage)
	if !ok || mpe == nil {
		return errorf(ErrNull, "Type_MPEcurve_Write: nil stage")
	}
	curves := toneCurvesData(mpe)
	if curves == nil {
		return errCorrupt()
	}
	baseOffset := io.Tell() - tagBaseSize

	// Curves: input and output channels are the same.
	if !writeUInt16(io, uint16(mpe.InputChannels)) {
		return errWrite()
	}
	if !writeUInt16(io, uint16(mpe.InputChannels)) {
		return errWrite()
	}
	return writePositionTable(self, io, 0, mpe.InputChannels, baseOffset, curves, writeMPECurve)
}

// ----------------------------------------------------------- Type_MPEmatrix

// typeMPEmatrixRead ports Type_MPEmatrix_Read.
func typeMPEmatrixRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID

	inputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if uint32(inputChans) >= maxChannels {
		return nil, 0, errCorrupt()
	}
	if uint32(outputChans) >= maxChannels {
		return nil, 0, errCorrupt()
	}

	nElems := uint32(inputChans) * uint32(outputChans)
	matrix := make([]float64, nElems)
	offsets := make([]float64, outputChans)

	for i := uint32(0); i < nElems; i++ {
		v, ok := readFloat32(io)
		if !ok {
			return nil, 0, errCorrupt()
		}
		matrix[i] = float64(v)
	}
	for i := uint32(0); i < uint32(outputChans); i++ {
		v, ok := readFloat32(io)
		if !ok {
			return nil, 0, errCorrupt()
		}
		offsets[i] = float64(v)
	}

	mpe, err := ctx.StageAllocMatrix(uint32(outputChans), uint32(inputChans), matrix, offsets)
	if err != nil {
		return nil, 0, err
	}
	return mpe, 1, nil
}

// typeMPEmatrixWrite ports Type_MPEmatrix_Write.
func typeMPEmatrixWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mpe, ok := value.(*Stage)
	if !ok || mpe == nil {
		return errorf(ErrNull, "Type_MPEmatrix_Write: nil stage")
	}
	double, offset := mpe.MatrixData()
	if double == nil {
		return errCorrupt()
	}

	if !writeUInt16(io, uint16(mpe.InputChannels)) {
		return errWrite()
	}
	if !writeUInt16(io, uint16(mpe.OutputChannels)) {
		return errWrite()
	}

	nElems := mpe.InputChannels * mpe.OutputChannels
	for i := uint32(0); i < nElems; i++ {
		if !writeFloat32(io, float32(double[i])) {
			return errWrite()
		}
	}
	for i := uint32(0); i < mpe.OutputChannels; i++ {
		if offset == nil {
			if !writeFloat32(io, 0) {
				return errWrite()
			}
		} else {
			if !writeFloat32(io, float32(offset[i])) {
				return errWrite()
			}
		}
	}
	return nil
}

// ------------------------------------------------------------- Type_MPEclut

// typeMPEclutRead ports Type_MPEclut_Read.
func typeMPEclutRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID

	inputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if inputChans == 0 || uint32(inputChans) >= maxChannels {
		return nil, 0, errCorrupt()
	}
	if outputChans == 0 || uint32(outputChans) >= maxChannels {
		return nil, 0, errCorrupt()
	}

	var dimensions8 [16]byte
	if io.Read(dimensions8[:], 1, 16) != 16 {
		return nil, 0, errCorrupt()
	}

	// Copy MAX_INPUT_DIMENSIONS at most.
	nMaxGrids := uint32(inputChans)
	if nMaxGrids > maxInputDimensions {
		nMaxGrids = maxInputDimensions
	}
	var gridPoints [maxInputDimensions]uint32
	for i := uint32(0); i < nMaxGrids; i++ {
		if dimensions8[i] == 1 { // Impossible value
			return nil, 0, errCorrupt()
		}
		gridPoints[i] = uint32(dimensions8[i])
	}

	// Bound the allocation before building the CLUT (each float entry is 4
	// bytes on disk, so it cannot exceed the profile size).
	if cube := cubeSize(gridPoints[:], uint32(inputChans)); cube == 0 ||
		uint32(outputChans) > uipowSentinel/cube ||
		uint32(outputChans)*cube > io.ReportedSize/4 {
		return nil, 0, errCorrupt()
	}

	mpe, err := ctx.StageAllocCLutFloatGranular(gridPoints[:], uint32(inputChans), uint32(outputChans), nil)
	if err != nil {
		return nil, 0, err
	}
	clut := mpe.CLUTData()
	if clut == nil {
		return nil, 0, errCorrupt()
	}
	for i := uint32(0); i < clut.nEntries; i++ {
		v, ok := readFloat32(io)
		if !ok {
			return nil, 0, errCorrupt()
		}
		clut.tabFloat[i] = v
	}
	return mpe, 1, nil
}

// typeMPEclutWrite ports Type_MPEclut_Write.
func typeMPEclutWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mpe, ok := value.(*Stage)
	if !ok || mpe == nil {
		return errorf(ErrNull, "Type_MPEclut_Write: nil stage")
	}
	clut := mpe.CLUTData()
	if clut == nil {
		return errCorrupt()
	}
	if mpe.InputChannels > maxInputDimensions {
		return errCorrupt()
	}
	if !clut.hasFloatValues {
		return errCorrupt()
	}

	if !writeUInt16(io, uint16(mpe.InputChannels)) {
		return errWrite()
	}
	if !writeUInt16(io, uint16(mpe.OutputChannels)) {
		return errWrite()
	}

	var dimensions8 [16]byte
	for i := uint32(0); i < mpe.InputChannels; i++ {
		dimensions8[i] = uint8(clut.params.NumSamples[i])
	}
	if !io.Write(16, dimensions8[:]) {
		return errWrite()
	}

	for i := uint32(0); i < clut.nEntries; i++ {
		if !writeFloat32(io, clut.tabFloat[i]) {
			return errWrite()
		}
	}
	return nil
}

// ------------------------------------------------------------- Type_MPE (mpet)

// readMPEElem ports ReadMPEElem: the position-table callback reading one MPE
// element and inserting it into the pipeline (cargo).
func readMPEElem(self *TagTypeHandler, io *IOHandler, cargo any, n, sizeOfTag uint32) error {
	ctx := self.ContextID
	newLUT, ok := cargo.(*Pipeline)
	if !ok || newLUT == nil {
		return errCorrupt()
	}

	elementSig, ok := readUInt32(io)
	if !ok {
		return errCorrupt()
	}
	if _, ok := readUInt32(io); !ok { // reserved placeholder
		return errCorrupt()
	}

	handler := ctx.getMPETypeHandler(TagTypeSignature(elementSig))
	if handler == nil {
		return ctx.signalError(ErrUnknownExtension, "Unknown MPE type '%s' found.", TagTypeSignature(elementSig))
	}

	// If no read method, ignore the element (valid for bACS and eACS).
	if handler.Read == nil {
		return nil
	}

	local := *handler
	local.ContextID = ctx
	local.ICCVersion = self.ICCVersion
	value, _, err := local.Read(&local, io, sizeOfTag)
	if err != nil {
		return err
	}
	stage, ok := value.(*Stage)
	if !ok || stage == nil {
		return errCorrupt()
	}
	return newLUT.InsertStage(AtEnd, stage)
}

// typeMPERead ports Type_MPE_Read: the main MPE dispatcher.
func typeMPERead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID
	baseOffset := io.Tell() - tagBaseSize

	inputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if inputChans == 0 || uint32(inputChans) >= maxChannels {
		return nil, 0, errCorrupt()
	}
	if outputChans == 0 || uint32(outputChans) >= maxChannels {
		return nil, 0, errCorrupt()
	}

	newLUT, err := ctx.PipelineAlloc(uint32(inputChans), uint32(outputChans))
	if err != nil {
		return nil, 0, err
	}

	elementCount, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if err := readPositionTable(self, io, elementCount, baseOffset, newLUT, readMPEElem); err != nil {
		return nil, 0, err
	}

	if uint32(inputChans) != newLUT.InputChannels || uint32(outputChans) != newLUT.OutputChannels {
		return nil, 0, errCorrupt()
	}

	return newLUT, 1, nil
}

// typeMPEWrite ports Type_MPE_Write.
func typeMPEWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	ctx := self.ContextID
	lut, ok := value.(*Pipeline)
	if !ok || lut == nil {
		return errorf(ErrNull, "Type_MPE_Write: nil pipeline")
	}

	baseOffset := io.Tell() - tagBaseSize
	inputChan := lut.InputChannelsCount()
	outputChan := lut.OutputChannelsCount()
	elemCount := lut.StageCount()

	elementOffsets := make([]uint32, elemCount)
	elementSizes := make([]uint32, elemCount)

	if !writeUInt16(io, uint16(inputChan)) {
		return errWrite()
	}
	if !writeUInt16(io, uint16(outputChan)) {
		return errWrite()
	}
	if !writeUInt32(io, elemCount) {
		return errWrite()
	}

	directoryPos := io.Tell()

	for i := uint32(0); i < elemCount; i++ {
		if !writeUInt32(io, 0) { // Offset
			return errWrite()
		}
		if !writeUInt32(io, 0) { // size
			return errWrite()
		}
	}

	elem := lut.GetPtrToFirstStage()
	for i := uint32(0); i < elemCount; i++ {
		elementOffsets[i] = io.Tell() - baseOffset
		elementSig := TagTypeSignature(elem.Type)

		handler := ctx.getMPETypeHandler(elementSig)
		if handler == nil || handler.Write == nil {
			return ctx.signalError(ErrUnknownExtension, "Found unknown MPE type '%s'", elementSig)
		}

		before := io.Tell()
		if !writeUInt32(io, uint32(elementSig)) {
			return errWrite()
		}
		if !writeUInt32(io, 0) {
			return errWrite()
		}

		local := *handler
		local.ContextID = ctx
		local.ICCVersion = self.ICCVersion
		if err := local.Write(&local, io, elem, 1); err != nil {
			return err
		}
		if !writeAlignment(io) {
			return errWrite()
		}
		elementSizes[i] = io.Tell() - before
		elem = elem.Next()
	}

	currentPos := io.Tell()
	if !io.Seek(directoryPos) {
		return errWrite()
	}
	for i := uint32(0); i < elemCount; i++ {
		if !writeUInt32(io, elementOffsets[i]) {
			return errWrite()
		}
		if !writeUInt32(io, elementSizes[i]) {
			return errWrite()
		}
	}
	if !io.Seek(currentPos) {
		return errWrite()
	}
	return nil
}

func init() {
	// Built-in MPE element types (SupportedMPEtypes). bACS/eACS are accepted as
	// no-ops: a handler exists but has no Read/Write, so ReadMPEElem ignores
	// them exactly as the reference does.
	registerBuiltinMPEType(&TagTypeHandler{Signature: sigBAcsElemTypeTT})
	registerBuiltinMPEType(&TagTypeHandler{Signature: sigEAcsElemTypeTT})
	registerBuiltinMPEType(&TagTypeHandler{
		Signature: sigCurveSetElemTypeTT,
		Read:      typeMPEcurveRead,
		Write:     typeMPEcurveWrite,
	})
	registerBuiltinMPEType(&TagTypeHandler{
		Signature: sigMatrixElemTypeTT,
		Read:      typeMPEmatrixRead,
		Write:     typeMPEmatrixWrite,
	})
	registerBuiltinMPEType(&TagTypeHandler{
		Signature: sigCLutElemTypeTT,
		Read:      typeMPEclutRead,
		Write:     typeMPEclutWrite,
	})

	// The MPE container tag type ('mpet').
	registerBuiltinType(&TagTypeHandler{
		Signature: SigMultiProcessElementType,
		Read:      typeMPERead,
		Write:     typeMPEWrite,
		Dup:       pipelineDupValue,
	})
}
