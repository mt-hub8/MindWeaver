// Port of the LUT tag-type handlers from src/cmstypes.c (lcms2 2.19):
// Type_LUT8 ('mft1'), Type_LUT16 ('mft2'), Type_LUTA2B ('mAB ') and
// Type_LUTB2A ('mBA '), together with their shared sub-readers/writers
// (Read8bitTables/Write8bitTables, Read16bitTables/Write16bitTables, ReadMatrix/
// WriteMatrix, ReadCLUT/WriteCLUT, ReadEmbeddedCurve, ReadSetOfCurves/
// WriteSetOfCurves) and the uipow overflow-checked power helper.
//
// The handlers build the same cmsPipeline stage sequence the reference builds,
// so a profile round-trips byte-for-byte where the reference write is an
// identity of the read. Lab/PCS fixups are deliberately NOT applied at the type
// level, exactly as in C: these handlers operate at the byte level only.
//
// Cross-worker contract (parallel worker W8a owns the base curve types):
//   - ReadEmbeddedCurve / WriteSetOfCurves dispatch the embedded 'curv'/'para'
//     curve serialization through the tag-type registry (getTagTypeHandler),
//     which returns W8a's Type_Curve / Type_ParametricCurve handlers. The
//     handlers must Read into / Write from a *ToneCurve value. When those
//     handlers are not yet registered (W8a not merged) the embedded-curve path
//     returns an error rather than panicking, and A2B/B2A curve round-trip
//     tests skip. This is a deliberate deviation from the C code, which calls
//     the static Type_Curve_Read/Type_ParametricCurve_Read directly (bypassing
//     plug-in overrides); using the registry additionally honours a plug-in
//     curve type, which is harmless for the built-in profiles.

package lcms2

// subHandler returns a per-call copy of the registered handler for sig, stamped
// with this handler's ContextID and ICCVersion, mirroring the LocalTypeHandler
// stamping the container does. Returns nil when no handler is registered.
func (self *TagTypeHandler) subHandler(sig TagTypeSignature) *TagTypeHandler {
	if self.ContextID == nil {
		return nil
	}
	h := self.ContextID.getTagTypeHandler(sig)
	if h == nil {
		return nil
	}
	local := *h
	local.ContextID = self.ContextID
	local.ICCVersion = self.ICCVersion
	return &local
}

// readUInt16Array/writeUInt16Array live in types_base.go (shared with the
// base type handlers).

// uipowSentinel is the (cmsUInt32Number)-1 overflow marker uipow returns.
const uipowSentinel = ^uint32(0)

// uipow ports uipow: compute n * a^b with the reference's overflow checks. It
// returns 0 when a==0 or n==0 and uipowSentinel on overflow.
func uipow(n, a, b uint32) uint32 {
	if a == 0 {
		return 0
	}
	if n == 0 {
		return 0
	}
	var rv uint32 = 1
	for ; b > 0; b-- {
		rv *= a
		if rv > uipowSentinel/a {
			return uipowSentinel
		}
	}
	rc := rv * n
	if rv != rc/n {
		return uipowSentinel
	}
	return rc
}

// mat3FromArray builds a MAT3 from 9 row-major coefficients, mirroring the C
// cast (cmsMAT3*)Matrix.
func mat3FromArray(m []float64) MAT3 {
	return MAT3{
		{m[0], m[1], m[2]},
		{m[3], m[4], m[5]},
		{m[6], m[7], m[8]},
	}
}

// toneCurvesData returns the tone-curve payload of a curve-set stage, or nil.
func toneCurvesData(mpe *Stage) *stageToneCurvesData {
	if mpe == nil {
		return nil
	}
	if d, ok := mpe.data.(*stageToneCurvesData); ok {
		return d
	}
	return nil
}

// ********************************************************************************
// Type cmsSigLut8Type
// ********************************************************************************

// read8bitTables ports Read8bitTables: read nChannels 256-entry 8-bit tables as
// tabulated 16-bit tone curves and append them as a curve-set stage.
func read8bitTables(ctx *Context, io *IOHandler, lut *Pipeline, nChannels uint32) bool {
	if nChannels > maxChannels {
		return false
	}
	if nChannels == 0 {
		return false
	}

	tables := make([]*ToneCurve, nChannels)
	for i := uint32(0); i < nChannels; i++ {
		t, err := ctx.BuildTabulatedToneCurve16(make([]uint16, 256))
		if err != nil {
			return false
		}
		tables[i] = t
	}

	var temp [256]byte
	for i := uint32(0); i < nChannels; i++ {
		if io.Read(temp[:], 256, 1) != 1 {
			return false
		}
		for j := 0; j < 256; j++ {
			tables[i].table16[j] = from8to16(temp[j])
		}
	}

	stage, err := ctx.StageAllocToneCurves(nChannels, tables)
	if err != nil || stage == nil {
		return false
	}
	if err := lut.InsertStage(AtEnd, stage); err != nil {
		return false
	}
	return true
}

// write8bitTables ports Write8bitTables. tables may be nil, in which case (as in
// the reference) nothing is written.
func write8bitTables(ctx *Context, io *IOHandler, n uint32, tables *stageToneCurvesData) bool {
	for i := uint32(0); i < n; i++ {
		if tables == nil {
			continue
		}
		curve := tables.theCurves[i]
		// Usual case of identity curves.
		if curve.nEntries == 2 && curve.table16[0] == 0 && curve.table16[1] == 65535 {
			for j := 0; j < 256; j++ {
				if !writeUInt8(io, uint8(j)) {
					return false
				}
			}
		} else if curve.nEntries != 256 {
			ctx.signalError(ErrRange, "LUT8 needs 256 entries on prelinearization")
			return false
		} else {
			for j := 0; j < 256; j++ {
				val := from16to8(curve.table16[j])
				if !writeUInt8(io, val) {
					return false
				}
			}
		}
	}
	return true
}

// typeLUT8Read ports Type_LUT8_Read.
func typeLUT8Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID

	inputChannels, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChannels, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	clutPoints, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if clutPoints == 1 { // Impossible value, 0 for no CLUT and then 2 at least
		return nil, 0, errCorrupt()
	}
	// Padding
	if _, ok := readUInt8(io); !ok {
		return nil, 0, errCorrupt()
	}

	if inputChannels == 0 || uint32(inputChannels) > maxChannels {
		return nil, 0, errCorrupt()
	}
	if outputChannels == 0 || uint32(outputChannels) > maxChannels {
		return nil, 0, errCorrupt()
	}

	newLUT, err := ctx.PipelineAlloc(uint32(inputChannels), uint32(outputChannels))
	if err != nil {
		return nil, 0, err
	}

	var matrix [9]float64
	for i := 0; i < 9; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return nil, 0, errCorrupt()
		}
		matrix[i] = v
	}

	// Only operates if not identity...
	if inputChannels == 3 && !MAT3IsIdentity(mat3FromArray(matrix[:])) {
		mStage, err := ctx.StageAllocMatrix(3, 3, matrix[:], nil)
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtBegin, mStage); err != nil {
			return nil, 0, err
		}
	}

	// Input tables.
	if !read8bitTables(ctx, io, newLUT, uint32(inputChannels)) {
		return nil, 0, errCorrupt()
	}

	// 3D CLUT.
	nTabSize := uipow(uint32(outputChannels), uint32(clutPoints), uint32(inputChannels))
	if nTabSize == uipowSentinel {
		return nil, 0, errCorrupt()
	}
	if nTabSize > 0 {
		// Sanity cap: the CLUT needs nTabSize bytes on disk, so it cannot exceed
		// the profile size. This bounds allocation from an untrusted length
		// without rejecting any input the reference would accept (a shorter
		// stream fails the read below regardless).
		if nTabSize > io.ReportedSize {
			return nil, 0, errCorrupt()
		}
		t := make([]uint16, nTabSize)
		temp := make([]byte, nTabSize)
		if io.Read(temp, nTabSize, 1) != 1 {
			return nil, 0, errCorrupt()
		}
		for i := uint32(0); i < nTabSize; i++ {
			t[i] = from8to16(temp[i])
		}
		clut, err := ctx.StageAllocCLut16bit(uint32(clutPoints), uint32(inputChannels), uint32(outputChannels), t)
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, clut); err != nil {
			return nil, 0, err
		}
	}

	// Output tables.
	if !read8bitTables(ctx, io, newLUT, uint32(outputChannels)) {
		return nil, 0, errCorrupt()
	}

	return newLUT, 1, nil
}

// disassembleLUT walks a pipeline and classifies it into the optional
// matrix/pre-curves/clut/post-curves components a LUT8/LUT16 permits, matching
// the reference disassembly. It returns ok=false when the pipeline holds a
// stage that does not fit that shape.
func disassembleLUT(lut *Pipeline) (mat *Stage, pre *stageToneCurvesData, clut *Stage, post *stageToneCurvesData, ok bool) {
	mpe := lut.GetPtrToFirstStage()

	if mpe != nil && mpe.Type == SigMatrixElemType {
		if mpe.InputChannels != 3 || mpe.OutputChannels != 3 {
			return nil, nil, nil, nil, false
		}
		mat = mpe
		mpe = mpe.Next()
	}
	if mpe != nil && mpe.Type == SigCurveSetElemType {
		pre = toneCurvesData(mpe)
		mpe = mpe.Next()
	}
	if mpe != nil && mpe.Type == SigCLutElemType {
		clut = mpe
		mpe = mpe.Next()
	}
	if mpe != nil && mpe.Type == SigCurveSetElemType {
		post = toneCurvesData(mpe)
		mpe = mpe.Next()
	}
	if mpe != nil {
		return nil, nil, nil, nil, false
	}
	return mat, pre, clut, post, true
}

// clutSamePointsPerDim returns the common grid-point count of clut, requiring
// every input dimension to share it (a LUT8/LUT16 constraint). ok is false when
// the dimensions differ.
func clutSamePointsPerDim(clut *Stage, inputChannels uint32) (points uint32, ok bool) {
	if clut == nil {
		return 0, true
	}
	d := clut.CLUTData()
	if d == nil {
		return 0, false
	}
	points = d.params.NumSamples[0]
	for i := uint32(1); i < inputChannels; i++ {
		if d.params.NumSamples[i] != points {
			return 0, false
		}
	}
	return points, true
}

// typeLUT8Write ports Type_LUT8_Write.
func typeLUT8Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	ctx := self.ContextID
	newLUT, ok := value.(*Pipeline)
	if !ok || newLUT == nil {
		return errorf(ErrNull, "Type_LUT8_Write: nil pipeline")
	}

	if newLUT.GetPtrToFirstStage() == nil {
		return ctx.signalError(ErrUnknownExtension, "empty LUT8 is not supported")
	}

	matMPE, preMPE, clut, postMPE, okShape := disassembleLUT(newLUT)
	if !okShape {
		return ctx.signalError(ErrUnknownExtension, "LUT is not suitable to be saved as LUT8")
	}

	inputChannels := newLUT.InputChannelsCount()
	outputChannels := newLUT.OutputChannelsCount()

	clutPoints, okp := clutSamePointsPerDim(clut, inputChannels)
	if !okp {
		return ctx.signalError(ErrUnknownExtension, "LUT with different samples per dimension not suitable to be saved as LUT16")
	}

	if !writeUInt8(io, uint8(inputChannels)) {
		return errWrite()
	}
	if !writeUInt8(io, uint8(outputChannels)) {
		return errWrite()
	}
	if !writeUInt8(io, uint8(clutPoints)) {
		return errWrite()
	}
	if !writeUInt8(io, 0) { // Padding
		return errWrite()
	}

	if matMPE != nil {
		md, _ := matMPE.MatrixData()
		for i := 0; i < 9; i++ {
			if !write15Fixed16(io, md[i]) {
				return errWrite()
			}
		}
	} else {
		ident := []float64{1, 0, 0, 0, 1, 0, 0, 0, 1}
		for i := 0; i < 9; i++ {
			if !write15Fixed16(io, ident[i]) {
				return errWrite()
			}
		}
	}

	// Prelinearization table.
	if !write8bitTables(ctx, io, inputChannels, preMPE) {
		return errWrite()
	}

	nTabSize := uipow(outputChannels, clutPoints, inputChannels)
	if nTabSize == uipowSentinel {
		return errWrite()
	}
	if nTabSize > 0 && clut != nil {
		d := clut.CLUTData()
		if d == nil {
			return errWrite()
		}
		for j := uint32(0); j < nTabSize; j++ {
			if !writeUInt8(io, from16to8(d.tab16[j])) {
				return errWrite()
			}
		}
	}

	// Postlinearization table.
	if !write8bitTables(ctx, io, outputChannels, postMPE) {
		return errWrite()
	}
	return nil
}

// ********************************************************************************
// Type cmsSigLut16Type
// ********************************************************************************

// read16bitTables ports Read16bitTables.
func read16bitTables(ctx *Context, io *IOHandler, lut *Pipeline, nChannels, nEntries uint32) bool {
	// Maybe an empty table? (this is a lcms extension)
	if nEntries == 0 {
		return true
	}
	// Check for malicious profiles.
	if nEntries < 2 {
		return false
	}
	if nChannels > maxChannels {
		return false
	}

	tables := make([]*ToneCurve, nChannels)
	for i := uint32(0); i < nChannels; i++ {
		t, err := ctx.BuildTabulatedToneCurve16(make([]uint16, nEntries))
		if err != nil {
			return false
		}
		tables[i] = t
		if !readUInt16Array(io, nEntries, tables[i].table16) {
			return false
		}
	}

	stage, err := ctx.StageAllocToneCurves(nChannels, tables)
	if err != nil || stage == nil {
		return false
	}
	if err := lut.InsertStage(AtEnd, stage); err != nil {
		return false
	}
	return true
}

// write16bitTables ports Write16bitTables. tables must be non-nil.
func write16bitTables(io *IOHandler, tables *stageToneCurvesData) bool {
	for i := uint32(0); i < tables.nCurves; i++ {
		nEntries := tables.theCurves[i].nEntries
		for j := uint32(0); j < nEntries; j++ {
			if !writeUInt16(io, tables.theCurves[i].table16[j]) {
				return false
			}
		}
	}
	return true
}

// typeLUT16Read ports Type_LUT16_Read.
func typeLUT16Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID

	inputChannels, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChannels, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	clutPoints, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if _, ok := readUInt8(io); !ok { // Padding
		return nil, 0, errCorrupt()
	}

	if inputChannels == 0 || uint32(inputChannels) > maxChannels {
		return nil, 0, errCorrupt()
	}
	if outputChannels == 0 || uint32(outputChannels) > maxChannels {
		return nil, 0, errCorrupt()
	}

	newLUT, err := ctx.PipelineAlloc(uint32(inputChannels), uint32(outputChannels))
	if err != nil {
		return nil, 0, err
	}

	var matrix [9]float64
	for i := 0; i < 9; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return nil, 0, errCorrupt()
		}
		matrix[i] = v
	}

	if inputChannels == 3 && !MAT3IsIdentity(mat3FromArray(matrix[:])) {
		mStage, err := ctx.StageAllocMatrix(3, 3, matrix[:], nil)
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, mStage); err != nil {
			return nil, 0, err
		}
	}

	inputEntries, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputEntries, ok := readUInt16(io)
	if !ok {
		return nil, 0, errCorrupt()
	}

	if inputEntries > 0x7FFF || outputEntries > 0x7FFF {
		return nil, 0, errCorrupt()
	}
	if clutPoints == 1 {
		return nil, 0, errCorrupt()
	}

	if !read16bitTables(ctx, io, newLUT, uint32(inputChannels), uint32(inputEntries)) {
		return nil, 0, errCorrupt()
	}

	nTabSize := uipow(uint32(outputChannels), uint32(clutPoints), uint32(inputChannels))
	if nTabSize == uipowSentinel {
		return nil, 0, errCorrupt()
	}
	if nTabSize > 0 {
		// Sanity cap: the CLUT needs nTabSize*2 bytes on disk (2-byte entries).
		if nTabSize > io.ReportedSize/2 {
			return nil, 0, errCorrupt()
		}
		t := make([]uint16, nTabSize)
		if !readUInt16Array(io, nTabSize, t) {
			return nil, 0, errCorrupt()
		}
		clut, err := ctx.StageAllocCLut16bit(uint32(clutPoints), uint32(inputChannels), uint32(outputChannels), t)
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, clut); err != nil {
			return nil, 0, err
		}
	}

	if !read16bitTables(ctx, io, newLUT, uint32(outputChannels), uint32(outputEntries)) {
		return nil, 0, errCorrupt()
	}

	return newLUT, 1, nil
}

// typeLUT16Write ports Type_LUT16_Write.
func typeLUT16Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	ctx := self.ContextID
	newLUT, ok := value.(*Pipeline)
	if !ok || newLUT == nil {
		return errorf(ErrNull, "Type_LUT16_Write: nil pipeline")
	}

	matMPE, preMPE, clut, postMPE, okShape := disassembleLUT(newLUT)
	if !okShape {
		return ctx.signalError(ErrUnknownExtension, "LUT is not suitable to be saved as LUT16")
	}

	inputChannels := newLUT.InputChannelsCount()
	outputChannels := newLUT.OutputChannelsCount()

	clutPoints, okp := clutSamePointsPerDim(clut, inputChannels)
	if !okp {
		return ctx.signalError(ErrUnknownExtension, "LUT with different samples per dimension not suitable to be saved as LUT16")
	}

	if !writeUInt8(io, uint8(inputChannels)) {
		return errWrite()
	}
	if !writeUInt8(io, uint8(outputChannels)) {
		return errWrite()
	}
	if !writeUInt8(io, uint8(clutPoints)) {
		return errWrite()
	}
	if !writeUInt8(io, 0) { // Padding
		return errWrite()
	}

	if matMPE != nil {
		md, _ := matMPE.MatrixData()
		for i := 0; i < 9; i++ {
			if !write15Fixed16(io, md[i]) {
				return errWrite()
			}
		}
	} else {
		ident := []float64{1, 0, 0, 0, 1, 0, 0, 0, 1}
		for i := 0; i < 9; i++ {
			if !write15Fixed16(io, ident[i]) {
				return errWrite()
			}
		}
	}

	if preMPE != nil {
		if !writeUInt16(io, uint16(preMPE.theCurves[0].nEntries)) {
			return errWrite()
		}
	} else {
		if !writeUInt16(io, 2) {
			return errWrite()
		}
	}
	if postMPE != nil {
		if !writeUInt16(io, uint16(postMPE.theCurves[0].nEntries)) {
			return errWrite()
		}
	} else {
		if !writeUInt16(io, 2) {
			return errWrite()
		}
	}

	// Prelinearization table.
	if preMPE != nil {
		if !write16bitTables(io, preMPE) {
			return errWrite()
		}
	} else {
		for i := uint32(0); i < inputChannels; i++ {
			if !writeUInt16(io, 0) {
				return errWrite()
			}
			if !writeUInt16(io, 0xffff) {
				return errWrite()
			}
		}
	}

	nTabSize := uipow(outputChannels, clutPoints, inputChannels)
	if nTabSize == uipowSentinel {
		return errWrite()
	}
	if nTabSize > 0 && clut != nil {
		d := clut.CLUTData()
		if d == nil {
			return errWrite()
		}
		if !writeUInt16Array(io, nTabSize, d.tab16) {
			return errWrite()
		}
	}

	// Postlinearization table.
	if postMPE != nil {
		if !write16bitTables(io, postMPE) {
			return errWrite()
		}
	} else {
		for i := uint32(0); i < outputChannels; i++ {
			if !writeUInt16(io, 0) {
				return errWrite()
			}
			if !writeUInt16(io, 0xffff) {
				return errWrite()
			}
		}
	}

	return nil
}

// ********************************************************************************
// Type cmsSigLutAToBType / cmsSigLutBToAType (V4)
// ********************************************************************************

// readMatrix ports ReadMatrix: seek to Offset and read a 3x3 matrix plus a
// 3-element offset into a matrix stage.
func readMatrix(self *TagTypeHandler, io *IOHandler, offset uint32) (*Stage, error) {
	ctx := self.ContextID
	if !io.Seek(offset) {
		return nil, errCorrupt()
	}
	var dMat [9]float64
	for i := 0; i < 9; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return nil, errCorrupt()
		}
		dMat[i] = v
	}
	var dOff [3]float64
	for i := 0; i < 3; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return nil, errCorrupt()
		}
		dOff[i] = v
	}
	return ctx.StageAllocMatrix(3, 3, dMat[:], dOff[:])
}

// readCLUT ports ReadCLUT: seek to Offset and read a granular 16-bit CLUT (1 or
// 2 byte precision) with inputChannels/outputChannels.
func readCLUT(self *TagTypeHandler, io *IOHandler, offset, inputChannels, outputChannels uint32) (*Stage, error) {
	ctx := self.ContextID
	if !io.Seek(offset) {
		return nil, errCorrupt()
	}

	var gridPoints8 [maxChannels]byte
	if io.Read(gridPoints8[:], maxChannels, 1) != 1 {
		return nil, errCorrupt()
	}
	var gridPoints [maxChannels]uint32
	for i := 0; i < maxChannels; i++ {
		if gridPoints8[i] == 1 { // Impossible value
			return nil, errCorrupt()
		}
		gridPoints[i] = uint32(gridPoints8[i])
	}

	precision, ok := readUInt8(io)
	if !ok {
		return nil, errCorrupt()
	}
	for i := 0; i < 3; i++ { // 3 reserved bytes
		if _, ok := readUInt8(io); !ok {
			return nil, errCorrupt()
		}
	}

	// Sanity cap before allocation: the CLUT needs at least nEntries bytes on
	// disk (1-byte precision), so it cannot exceed the profile size. cubeSize
	// returns 0 on overflow / degenerate dimensions, which the allocator below
	// rejects.
	if cube := cubeSize(gridPoints[:], inputChannels); cube == 0 ||
		outputChannels > uipowSentinel/cube ||
		outputChannels*cube > io.ReportedSize {
		return nil, errCorrupt()
	}

	clut, err := ctx.StageAllocCLut16bitGranular(gridPoints[:], inputChannels, outputChannels, nil)
	if err != nil {
		return nil, err
	}
	data := clut.CLUTData()
	if data == nil {
		return nil, errCorrupt()
	}

	switch precision {
	case 1:
		for i := uint32(0); i < data.nEntries; i++ {
			v, ok := readUInt8(io)
			if !ok {
				return nil, errCorrupt()
			}
			data.tab16[i] = from8to16(v)
		}
	case 2:
		if !readUInt16Array(io, data.nEntries, data.tab16) {
			return nil, errCorrupt()
		}
	default:
		return nil, ctx.signalError(ErrUnknownExtension, "Unknown precision of '%d'", precision)
	}

	return clut, nil
}

// readEmbeddedCurve ports ReadEmbeddedCurve: read a type base then dispatch the
// embedded 'curv'/'para' curve through the registered handler.
func readEmbeddedCurve(self *TagTypeHandler, io *IOHandler) (*ToneCurve, error) {
	baseType := readTypeBase(io)
	switch baseType {
	case SigCurveType, SigParametricCurveType:
		h := self.subHandler(baseType)
		if h == nil || h.Read == nil {
			return nil, errorf(ErrUnknownExtension, "no handler for embedded curve type '%s' (base curve types are owned by W8a)", baseType)
		}
		v, _, err := h.Read(h, io, 0)
		if err != nil {
			return nil, err
		}
		c, ok := v.(*ToneCurve)
		if !ok || c == nil {
			return nil, errCorrupt()
		}
		return c, nil
	default:
		return nil, self.ContextID.signalError(ErrUnknownExtension, "Unknown curve type '%s'", baseType)
	}
}

// readSetOfCurves ports ReadSetOfCurves: read nCurves embedded curves from
// Offset and build a curve-set stage.
func readSetOfCurves(self *TagTypeHandler, io *IOHandler, offset, nCurves uint32) (*Stage, error) {
	ctx := self.ContextID
	if nCurves > maxChannels {
		return nil, errCorrupt()
	}
	if !io.Seek(offset) {
		return nil, errCorrupt()
	}

	curves := make([]*ToneCurve, nCurves)
	for i := uint32(0); i < nCurves; i++ {
		c, err := readEmbeddedCurve(self, io)
		if err != nil {
			return nil, err
		}
		curves[i] = c
		if !readAlignment(io) {
			return nil, errCorrupt()
		}
	}
	return ctx.StageAllocToneCurves(nCurves, curves)
}

// writeMatrix ports WriteMatrix.
func writeMatrix(io *IOHandler, mpe *Stage) bool {
	double, offset := mpe.MatrixData()
	n := mpe.InputChannels * mpe.OutputChannels
	for i := uint32(0); i < n; i++ {
		if !write15Fixed16(io, double[i]) {
			return false
		}
	}
	if offset != nil {
		for i := uint32(0); i < mpe.OutputChannels; i++ {
			if !write15Fixed16(io, offset[i]) {
				return false
			}
		}
	} else {
		for i := uint32(0); i < mpe.OutputChannels; i++ {
			if !write15Fixed16(io, 0) {
				return false
			}
		}
	}
	return true
}

// writeSetOfCurves ports WriteSetOfCurves: write each curve in mpe using typ,
// downgrading to 'curv' when the curve is tabulated or inverted (matching the
// reference selection).
func writeSetOfCurves(self *TagTypeHandler, io *IOHandler, typ TagTypeSignature, mpe *Stage) error {
	ctx := self.ContextID
	n := mpe.OutputChannelsCount()
	curves := mpe.GetToneCurves()

	for i := uint32(0); i < n; i++ {
		currentType := typ
		c := curves[i]

		if c.nSegments == 0 || // 16-bit tabulated
			(c.nSegments == 3 && len(c.segments) >= 2 && c.segments[1].Type == 0) { // floating-point tabulated
			currentType = SigCurveType
		} else if len(c.segments) >= 1 && c.segments[0].Type < 0 {
			currentType = SigCurveType
		}

		if !writeTypeBase(io, currentType) {
			return errWrite()
		}

		switch currentType {
		case SigCurveType, SigParametricCurveType:
			h := self.subHandler(currentType)
			if h == nil || h.Write == nil {
				return errorf(ErrUnknownExtension, "no handler for embedded curve type '%s' (base curve types are owned by W8a)", currentType)
			}
			if err := h.Write(h, io, c, 1); err != nil {
				return err
			}
		default:
			return ctx.signalError(ErrUnknownExtension, "Unknown curve type '%s'", typ)
		}

		if !writeAlignment(io) {
			return errWrite()
		}
	}
	return nil
}

// writeCLUT ports WriteCLUT.
func writeCLUT(self *TagTypeHandler, io *IOHandler, precision uint8, mpe *Stage) error {
	ctx := self.ContextID
	clut := mpe.CLUTData()
	if clut == nil {
		return errCorrupt()
	}
	if clut.hasFloatValues {
		return ctx.signalError(ErrNotSuitable, "Cannot save floating point data, CLUT are 8 or 16 bit only")
	}

	var gridPoints [maxChannels]byte
	for i := uint32(0); i < clut.params.NumInputs; i++ {
		gridPoints[i] = uint8(clut.params.NumSamples[i])
	}
	if !io.Write(maxChannels, gridPoints[:]) {
		return errWrite()
	}

	if !writeUInt8(io, precision) {
		return errWrite()
	}
	if !writeUInt8(io, 0) {
		return errWrite()
	}
	if !writeUInt8(io, 0) {
		return errWrite()
	}
	if !writeUInt8(io, 0) {
		return errWrite()
	}

	switch precision {
	case 1:
		for i := uint32(0); i < clut.nEntries; i++ {
			if !writeUInt8(io, from16to8(clut.tab16[i])) {
				return errWrite()
			}
		}
	case 2:
		if !writeUInt16Array(io, clut.nEntries, clut.tab16) {
			return errWrite()
		}
	default:
		return ctx.signalError(ErrUnknownExtension, "Unknown precision of '%d'", precision)
	}

	if !writeAlignment(io) {
		return errWrite()
	}
	return nil
}

// typeLUTA2BRead ports Type_LUTA2B_Read.
func typeLUTA2BRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID
	baseOffset := io.Tell() - tagBaseSize

	inputChan, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChan, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	if _, ok := readUInt16(io); !ok {
		return nil, 0, errCorrupt()
	}

	offsetB, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetMat, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetM, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetC, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetA, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}

	if inputChan == 0 || uint32(inputChan) >= maxChannels {
		return nil, 0, errCorrupt()
	}
	if outputChan == 0 || uint32(outputChan) >= maxChannels {
		return nil, 0, errCorrupt()
	}

	newLUT, err := ctx.PipelineAlloc(uint32(inputChan), uint32(outputChan))
	if err != nil {
		return nil, 0, err
	}

	if offsetA != 0 {
		s, err := readSetOfCurves(self, io, baseOffset+offsetA, uint32(inputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetC != 0 {
		s, err := readCLUT(self, io, baseOffset+offsetC, uint32(inputChan), uint32(outputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetM != 0 {
		s, err := readSetOfCurves(self, io, baseOffset+offsetM, uint32(outputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetMat != 0 {
		s, err := readMatrix(self, io, baseOffset+offsetMat)
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetB != 0 {
		s, err := readSetOfCurves(self, io, baseOffset+offsetB, uint32(outputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}

	return newLUT, 1, nil
}

// typeLUTA2BWrite ports Type_LUTA2B_Write.
func typeLUTA2BWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	ctx := self.ContextID
	lut, ok := value.(*Pipeline)
	if !ok || lut == nil {
		return errorf(ErrNull, "Type_LUTA2B_Write: nil pipeline")
	}

	baseOffset := io.Tell() - tagBaseSize

	var a, b, m, matrix, clut *Stage
	if lut.GetPtrToFirstStage() != nil {
		if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType); okc {
			b = st[0]
		} else if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType); okc {
			m, matrix, b = st[0], st[1], st[2]
		} else if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType); okc {
			a, clut, b = st[0], st[1], st[2]
		} else if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType); okc {
			a, clut, m, matrix, b = st[0], st[1], st[2], st[3], st[4]
		} else {
			return ctx.signalError(ErrNotSuitable, "LUT is not suitable to be saved as LutAToB")
		}
	}

	return writeAToBDirectory(self, io, lut, baseOffset, a, clut, m, matrix, b)
}

// typeLUTB2ARead ports Type_LUTB2A_Read.
func typeLUTB2ARead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	ctx := self.ContextID
	baseOffset := io.Tell() - tagBaseSize

	inputChan, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	outputChan, ok := readUInt8(io)
	if !ok {
		return nil, 0, errCorrupt()
	}

	if inputChan == 0 || uint32(inputChan) >= maxChannels {
		return nil, 0, errCorrupt()
	}
	if outputChan == 0 || uint32(outputChan) >= maxChannels {
		return nil, 0, errCorrupt()
	}

	if _, ok := readUInt16(io); !ok { // Padding
		return nil, 0, errCorrupt()
	}

	offsetB, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetMat, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetM, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetC, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}
	offsetA, ok := readUInt32(io)
	if !ok {
		return nil, 0, errCorrupt()
	}

	newLUT, err := ctx.PipelineAlloc(uint32(inputChan), uint32(outputChan))
	if err != nil {
		return nil, 0, err
	}

	if offsetB != 0 {
		s, err := readSetOfCurves(self, io, baseOffset+offsetB, uint32(inputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetMat != 0 {
		s, err := readMatrix(self, io, baseOffset+offsetMat)
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetM != 0 {
		s, err := readSetOfCurves(self, io, baseOffset+offsetM, uint32(inputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetC != 0 {
		s, err := readCLUT(self, io, baseOffset+offsetC, uint32(inputChan), uint32(outputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}
	if offsetA != 0 {
		s, err := readSetOfCurves(self, io, baseOffset+offsetA, uint32(outputChan))
		if err != nil {
			return nil, 0, err
		}
		if err := newLUT.InsertStage(AtEnd, s); err != nil {
			return nil, 0, err
		}
	}

	return newLUT, 1, nil
}

// typeLUTB2AWrite ports Type_LUTB2A_Write.
func typeLUTB2AWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	ctx := self.ContextID
	lut, ok := value.(*Pipeline)
	if !ok || lut == nil {
		return errorf(ErrNull, "Type_LUTB2A_Write: nil pipeline")
	}

	baseOffset := io.Tell() - tagBaseSize

	var a, b, m, matrix, clut *Stage
	if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType); okc {
		b = st[0]
	} else if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType); okc {
		b, matrix, m = st[0], st[1], st[2]
	} else if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType); okc {
		b, clut, a = st[0], st[1], st[2]
	} else if st, okc := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType); okc {
		b, matrix, m, clut, a = st[0], st[1], st[2], st[3], st[4]
	} else {
		return ctx.signalError(ErrNotSuitable, "LUT is not suitable to be saved as LutBToA")
	}

	return writeAToBDirectory(self, io, lut, baseOffset, a, clut, m, matrix, b)
}

// writeAToBDirectory writes the shared header + directory + element blocks that
// both Type_LUTA2B_Write and Type_LUTB2A_Write emit (they are byte-identical
// apart from the stage classification order).
func writeAToBDirectory(self *TagTypeHandler, io *IOHandler, lut *Pipeline, baseOffset uint32, a, clut, m, matrix, b *Stage) error {
	inputChan := lut.InputChannelsCount()
	outputChan := lut.OutputChannelsCount()

	if !writeUInt8(io, uint8(inputChan)) {
		return errWrite()
	}
	if !writeUInt8(io, uint8(outputChan)) {
		return errWrite()
	}
	if !writeUInt16(io, 0) {
		return errWrite()
	}

	directoryPos := io.Tell()

	for i := 0; i < 5; i++ {
		if !writeUInt32(io, 0) {
			return errWrite()
		}
	}

	var offsetB, offsetMat, offsetM, offsetC, offsetA uint32

	if a != nil {
		offsetA = io.Tell() - baseOffset
		if err := writeSetOfCurves(self, io, SigParametricCurveType, a); err != nil {
			return err
		}
	}
	if clut != nil {
		offsetC = io.Tell() - baseOffset
		precision := uint8(2)
		if lut.saveAs8Bits {
			precision = 1
		}
		if err := writeCLUT(self, io, precision, clut); err != nil {
			return err
		}
	}
	if m != nil {
		offsetM = io.Tell() - baseOffset
		if err := writeSetOfCurves(self, io, SigParametricCurveType, m); err != nil {
			return err
		}
	}
	if matrix != nil {
		offsetMat = io.Tell() - baseOffset
		if !writeMatrix(io, matrix) {
			return errWrite()
		}
	}
	if b != nil {
		offsetB = io.Tell() - baseOffset
		if err := writeSetOfCurves(self, io, SigParametricCurveType, b); err != nil {
			return err
		}
	}

	currentPos := io.Tell()

	if !io.Seek(directoryPos) {
		return errWrite()
	}
	if !writeUInt32(io, offsetB) {
		return errWrite()
	}
	if !writeUInt32(io, offsetMat) {
		return errWrite()
	}
	if !writeUInt32(io, offsetM) {
		return errWrite()
	}
	if !writeUInt32(io, offsetC) {
		return errWrite()
	}
	if !writeUInt32(io, offsetA) {
		return errWrite()
	}
	if !io.Seek(currentPos) {
		return errWrite()
	}
	return nil
}

// pipelineDupValue is the shared Dup for every LUT tag type: an independent deep
// copy of the pipeline (mirrors cmsPipelineDup).
func pipelineDupValue(self *TagTypeHandler, value any, n uint32) (any, error) {
	lut, ok := value.(*Pipeline)
	if !ok || lut == nil {
		return nil, errorf(ErrNull, "LUT Dup: nil pipeline")
	}
	return lut.Dup()
}

// errCorrupt / errWrite return the generic errors the LUT handlers surface for
// short reads and failed writes (the reference merely returns NULL/FALSE).
func errCorrupt() error { return errorf(ErrCorruptionDetected, "corrupted tag data") }
func errWrite() error   { return errorf(ErrWrite, "write error") }

func init() {
	registerBuiltinType(&TagTypeHandler{
		Signature: SigLut8Type,
		Read:      typeLUT8Read,
		Write:     typeLUT8Write,
		Dup:       pipelineDupValue,
	})
	registerBuiltinType(&TagTypeHandler{
		Signature: SigLut16Type,
		Read:      typeLUT16Read,
		Write:     typeLUT16Write,
		Dup:       pipelineDupValue,
	})
	registerBuiltinType(&TagTypeHandler{
		Signature: SigLutAtoBType,
		Read:      typeLUTA2BRead,
		Write:     typeLUTA2BWrite,
		Dup:       pipelineDupValue,
	})
	registerBuiltinType(&TagTypeHandler{
		Signature: SigLutBtoAType,
		Read:      typeLUTB2ARead,
		Write:     typeLUTB2AWrite,
		Dup:       pipelineDupValue,
	})
}
