// Port of the base tag-type handlers from src/cmstypes.c: every ICC tag type
// except the LUT8/LUT16/LutAtoB/LutBtoA/CLUT/multi-process-element family and
// the dictionary type (those live in W8b's types_lut.go/types_mpe.go).
//
// Each handler is a *TagTypeHandler registered from init() via
// registerBuiltinType. The container (io0.go) copies the handler and stamps
// ContextID/ICCVersion before every call, so handlers read those from the self
// pointer and hold no captured state. Read returns (value, count, error);
// (nil, 0, nil) is not produced — failures return a non-nil error.
//
// The DecideType selectors (decideXYZtype, decideTextType, decideTextDescType,
// decideCurveType, decideLUTtypeA2B/B2A) live in tagdesc.go with the tag
// descriptor table that references them.

package lcms2

import "math"

// ---------------------------------------------------------------------------
// Value types carried by the handlers
// ---------------------------------------------------------------------------

// Signature is a generic four-byte ICC signature value, the payload of a
// signatureType tag (cmsSignature).
type Signature uint32

// String renders the signature as four ASCII characters.
func (s Signature) String() string { return sigToString(uint32(s)) }

// ICCData mirrors cmsICCData: the payload of a dataType tag.
type ICCData struct {
	Flag uint32 // 0 = ASCII, 1 = binary
	Data []byte
}

// ICCMeasurementConditions mirrors cmsICCMeasurementConditions.
type ICCMeasurementConditions struct {
	Observer       uint32
	Backing        CIEXYZ
	Geometry       uint32
	Flare          float64
	IlluminantType uint32
}

// ICCViewingConditions mirrors cmsICCViewingConditions.
type ICCViewingConditions struct {
	IlluminantXYZ  CIEXYZ
	SurroundXYZ    CIEXYZ
	IlluminantType uint32
}

// ScreeningChannel mirrors cmsScreeningChannel.
type ScreeningChannel struct {
	Frequency   float64
	ScreenAngle float64
	SpotShape   uint32
}

// Screening mirrors cmsScreening.
type Screening struct {
	Flag      uint32
	NChannels uint32
	Channels  [maxChannels]ScreeningChannel
}

// UcrBg mirrors cmsUcrBg: under-color-removal and black-generation curves plus
// a description.
type UcrBg struct {
	Ucr  *ToneCurve
	Bg   *ToneCurve
	Desc *MLU
}

// VideoSignalType mirrors cmsVideoSignalType (the 'cicp' tag).
type VideoSignalType struct {
	ColourPrimaries         uint8
	TransferCharacteristics uint8
	MatrixCoefficients      uint8
	VideoFullRangeFlag      uint8
}

// MHC2Type mirrors cmsMHC2Type (Microsoft's MHC2 tag).
type MHC2Type struct {
	CurveEntries  uint32
	RedCurve      []float64
	GreenCurve    []float64
	BlueCurve     []float64
	MinLuminance  float64
	PeakLuminance float64
	XYZ2XYZmatrix [3][4]float64
}

// The "broken vendor" alias type signatures the reference tolerates on read
// (cmsCorbisBrokenXYZtype 0x17A505B8, cmsMonacoBrokenCurveType 0x9478ee00) are
// used only as literals in init() below. Named constants for them live in W8b's
// tagdesc.go, so they are intentionally not declared here to avoid a duplicate
// declaration when the branches merge.

// ICC "no language / no country" and V2 Unicode language codes (lcms2.h).
var (
	noLangCode  = strTo16("\x00\x00")
	noCntryCode = strTo16("\x00\x00")
	v2UnicodeL  = strTo16("\xff\xff")
	v2UnicodeC  = strTo16("\xff\xff")
)

// vcgt storage flavors (cmstypes.c).
const (
	videoCardGammaTableType   = 0
	videoCardGammaFormulaType = 1
)

// ---------------------------------------------------------------------------
// Serialization helpers not already in io.go
// ---------------------------------------------------------------------------

// readUInt16Array ports _cmsReadUInt16Array into dst (len dst must be >= n).
func readUInt16Array(io *IOHandler, n uint32, dst []uint16) bool {
	for i := uint32(0); i < n; i++ {
		v, ok := readUInt16(io)
		if !ok {
			return false
		}
		if i < uint32(len(dst)) {
			dst[i] = v
		}
	}
	return true
}

// writeUInt16Array ports _cmsWriteUInt16Array.
func writeUInt16Array(io *IOHandler, n uint32, src []uint16) bool {
	for i := uint32(0); i < n; i++ {
		var v uint16
		if i < uint32(len(src)) {
			v = src[i]
		}
		if !writeUInt16(io, v) {
			return false
		}
	}
	return true
}

// surrogate predicates ported from cmstypes.c.
func isSurrogate(uc uint32) bool     { return uc-0xd800 < 2048 }
func isHighSurrogate(uc uint32) bool { return uc&0xfffffc00 == 0xd800 }
func isLowSurrogate(uc uint32) bool  { return uc&0xfffffc00 == 0xdc00 }
func surrogateToUTF32(high, low uint32) uint32 {
	return (high << 10) + low - 0x35fdc00
}

// readWCharArray ports _cmsReadWCharArray (the 4-byte-wchar_t path): read count
// UTF-16 code units, combining surrogate pairs into code points, into dst.
// Extra dst slots (when surrogates shrink the output) remain zero, matching the
// reference's calloc'd block.
func readWCharArray(io *IOHandler, count uint32, dst []rune) bool {
	out := 0
	n := int64(count)
	for n > 0 {
		uc, ok := readUInt16(io)
		if !ok {
			return false
		}
		n--
		if !isSurrogate(uint32(uc)) {
			if out < len(dst) {
				dst[out] = rune(uc)
				out++
			}
			continue
		}
		low, ok := readUInt16(io)
		if !ok {
			return false
		}
		n--
		if isHighSurrogate(uint32(uc)) && isLowSurrogate(uint32(low)) {
			if out < len(dst) {
				dst[out] = rune(surrogateToUTF32(uint32(uc), uint32(low)))
				out++
			}
		} else {
			return false // corrupted string
		}
	}
	return true
}

// writeWCharArray ports _cmsWriteWCharArray (the 4-byte-wchar_t path): write n
// code points as UTF-16 code units; code points outside the BMP (or surrogate
// values) become '?'.
func writeWCharArray(io *IOHandler, n uint32, src []rune) bool {
	for i := uint32(0); i < n; i++ {
		var v rune
		if i < uint32(len(src)) {
			v = src[i]
		}
		var out uint16
		if v >= 0 && v <= 0xFFFF && !(v >= 0xD800 && v <= 0xDFFF) {
			out = uint16(v)
		} else {
			out = uint16('?')
		}
		if !writeUInt16(io, out) {
			return false
		}
	}
	return true
}

// writeZeros writes n zero bytes.
func writeZeros(io *IOHandler, n uint32) bool {
	if n == 0 {
		return true
	}
	buf := make([]byte, n)
	return io.Write(n, buf)
}

// cstrlen returns the length up to the first NUL byte.
func cstrlen(b []byte) uint32 {
	for i, c := range b {
		if c == 0 {
			return uint32(i)
		}
	}
	return uint32(len(b))
}

// wErr wraps a boolean write result as an error for the handler contract.
func wErr(ok bool) error {
	if !ok {
		return errorf(ErrWrite, "write error")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Position tables (ICC spec 4.3), ported from cmstypes.c
// ---------------------------------------------------------------------------

type positionEntryFn func(self *TagTypeHandler, io *IOHandler, cargo any, n, sizeOfTag uint32) error

// readPositionTable ports ReadPositionTable.
func readPositionTable(self *TagTypeHandler, io *IOHandler, count, baseOffset uint32, cargo any, fn positionEntryFn) error {
	currentPosition := io.Tell()
	if io.ReportedSize < currentPosition {
		return errorf(ErrRead, "position table past end")
	}
	if (io.ReportedSize-currentPosition)/(2*4) < count {
		return errorf(ErrRead, "position table too large")
	}

	offsets := make([]uint32, count)
	sizes := make([]uint32, count)
	for i := uint32(0); i < count; i++ {
		off, ok := readUInt32(io)
		if !ok {
			return errorf(ErrRead, "position table read error")
		}
		sz, ok := readUInt32(io)
		if !ok {
			return errorf(ErrRead, "position table read error")
		}
		offsets[i] = off + baseOffset
		sizes[i] = sz
	}

	for i := uint32(0); i < count; i++ {
		if !io.Seek(offsets[i]) {
			return errorf(ErrSeek, "position table seek error")
		}
		if err := fn(self, io, cargo, i, sizes[i]); err != nil {
			return err
		}
	}
	return nil
}

// writePositionTable ports WritePositionTable.
func writePositionTable(self *TagTypeHandler, io *IOHandler, sizeOfTag, count, baseOffset uint32, cargo any, fn positionEntryFn) error {
	offsets := make([]uint32, count)
	sizes := make([]uint32, count)

	directoryPos := io.Tell()
	for i := uint32(0); i < count; i++ {
		if !writeUInt32(io, 0) {
			return errorf(ErrWrite, "position table write error")
		}
		if !writeUInt32(io, 0) {
			return errorf(ErrWrite, "position table write error")
		}
	}

	for i := uint32(0); i < count; i++ {
		before := io.Tell()
		offsets[i] = before - baseOffset
		if err := fn(self, io, cargo, i, sizeOfTag); err != nil {
			return err
		}
		sizes[i] = io.Tell() - before
	}

	currentPos := io.Tell()
	if !io.Seek(directoryPos) {
		return errorf(ErrSeek, "position table seek error")
	}
	for i := uint32(0); i < count; i++ {
		if !writeUInt32(io, offsets[i]) {
			return errorf(ErrWrite, "position table write error")
		}
		if !writeUInt32(io, sizes[i]) {
			return errorf(ErrWrite, "position table write error")
		}
	}
	if !io.Seek(currentPos) {
		return errorf(ErrSeek, "position table seek error")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Type XYZ
// ---------------------------------------------------------------------------

func typeXYZRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	xyz, ok := readXYZ(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "XYZ read error")
	}
	v := xyz
	return &v, 1, nil
}

func typeXYZWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	xyz, ok := value.(*CIEXYZ)
	if !ok {
		return errorf(ErrInternal, "XYZ write: wrong type")
	}
	return wErr(writeXYZ(io, *xyz))
}

func typeXYZDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	xyz, ok := value.(*CIEXYZ)
	if !ok {
		return nil, errorf(ErrInternal, "XYZ dup: wrong type")
	}
	v := *xyz
	return &v, nil
}

// decideXYZType ports DecideXYZtype: always the canonical XYZ type.
func decideXYZType(iccVersion float64, data any) TagTypeSignature { return SigXYZType }

// ---------------------------------------------------------------------------
// Type Chromaticity
// ---------------------------------------------------------------------------

func typeChromaticityRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	chrm := &CIExyYTRIPLE{}

	nChans, ok := readUInt16(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "chromaticity read error")
	}
	// Recover from an early lcms1 bug.
	if nChans == 0 && sizeOfTag == 32 {
		if _, ok := readUInt16(io); !ok {
			return nil, 0, errorf(ErrRead, "chromaticity read error")
		}
		if nChans, ok = readUInt16(io); !ok {
			return nil, 0, errorf(ErrRead, "chromaticity read error")
		}
	}
	if nChans != 3 {
		return nil, 0, errorf(ErrCorruptionDetected, "chromaticity channels != 3")
	}
	if _, ok := readUInt16(io); !ok { // Table
		return nil, 0, errorf(ErrRead, "chromaticity read error")
	}

	read := func(dst *float64) bool {
		v, ok := read15Fixed16(io)
		if ok {
			*dst = v
		}
		return ok
	}
	if !read(&chrm.Red.X) || !read(&chrm.Red.Y) {
		return nil, 0, errorf(ErrRead, "chromaticity read error")
	}
	chrm.Red.YY = 1.0
	if !read(&chrm.Green.X) || !read(&chrm.Green.Y) {
		return nil, 0, errorf(ErrRead, "chromaticity read error")
	}
	chrm.Green.YY = 1.0
	if !read(&chrm.Blue.X) || !read(&chrm.Blue.Y) {
		return nil, 0, errorf(ErrRead, "chromaticity read error")
	}
	chrm.Blue.YY = 1.0
	return chrm, 1, nil
}

func saveOneChromaticity(x, y float64, io *IOHandler) bool {
	if !writeUInt32(io, uint32(doubleTo15Fixed16(x))) {
		return false
	}
	return writeUInt32(io, uint32(doubleTo15Fixed16(y)))
}

func typeChromaticityWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	chrm, ok := value.(*CIExyYTRIPLE)
	if !ok {
		return errorf(ErrInternal, "chromaticity write: wrong type")
	}
	if !writeUInt16(io, 3) {
		return errorf(ErrWrite, "chromaticity write error")
	}
	if !writeUInt16(io, 0) {
		return errorf(ErrWrite, "chromaticity write error")
	}
	if !saveOneChromaticity(chrm.Red.X, chrm.Red.Y, io) ||
		!saveOneChromaticity(chrm.Green.X, chrm.Green.Y, io) ||
		!saveOneChromaticity(chrm.Blue.X, chrm.Blue.Y, io) {
		return errorf(ErrWrite, "chromaticity write error")
	}
	return nil
}

func typeChromaticityDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	chrm, ok := value.(*CIExyYTRIPLE)
	if !ok {
		return nil, errorf(ErrInternal, "chromaticity dup: wrong type")
	}
	v := *chrm
	return &v, nil
}

// ---------------------------------------------------------------------------
// Type ColorantOrder
// ---------------------------------------------------------------------------

func typeColorantOrderRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "colorant order read error")
	}
	if count > maxChannels {
		return nil, 0, errorf(ErrCorruptionDetected, "too many colorants")
	}
	order := make([]uint8, maxChannels)
	for i := range order {
		order[i] = 0xFF
	}
	buf := make([]byte, count)
	if io.Read(buf, 1, count) != count {
		return nil, 0, errorf(ErrRead, "colorant order read error")
	}
	copy(order, buf)
	return order, 1, nil
}

func typeColorantOrderWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	order, ok := value.([]uint8)
	if !ok || len(order) < maxChannels {
		return errorf(ErrInternal, "colorant order write: wrong type")
	}
	var count uint32
	for i := 0; i < maxChannels; i++ {
		if order[i] != 0xFF {
			count++
		}
	}
	if !writeUInt32(io, count) {
		return errorf(ErrWrite, "colorant order write error")
	}
	if count > 0 && !io.Write(count, order[:count]) {
		return errorf(ErrWrite, "colorant order write error")
	}
	return nil
}

func typeColorantOrderDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	order, ok := value.([]uint8)
	if !ok {
		return nil, errorf(ErrInternal, "colorant order dup: wrong type")
	}
	out := make([]uint8, len(order))
	copy(out, order)
	return out, nil
}

// ---------------------------------------------------------------------------
// Types UInt8 / UInt32 / UInt64 arrays
// ---------------------------------------------------------------------------

func typeUInt8Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	n := sizeOfTag
	arr := make([]uint8, n)
	for i := uint32(0); i < n; i++ {
		v, ok := readUInt8(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "uint8 array read error")
		}
		arr[i] = v
	}
	return arr, n, nil
}

func typeUInt8Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	arr, ok := value.([]uint8)
	if !ok {
		return errorf(ErrInternal, "uint8 array write: wrong type")
	}
	for i := uint32(0); i < nItems && i < uint32(len(arr)); i++ {
		if !writeUInt8(io, arr[i]) {
			return errorf(ErrWrite, "uint8 array write error")
		}
	}
	return nil
}

func typeUInt8Dup(self *TagTypeHandler, value any, n uint32) (any, error) {
	arr, _ := value.([]uint8)
	out := make([]uint8, len(arr))
	copy(out, arr)
	return out, nil
}

func typeUInt32Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	n := sizeOfTag / 4
	arr := make([]uint32, n)
	for i := uint32(0); i < n; i++ {
		v, ok := readUInt32(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "uint32 array read error")
		}
		arr[i] = v
	}
	return arr, n, nil
}

func typeUInt32Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	arr, ok := value.([]uint32)
	if !ok {
		return errorf(ErrInternal, "uint32 array write: wrong type")
	}
	for i := uint32(0); i < nItems && i < uint32(len(arr)); i++ {
		if !writeUInt32(io, arr[i]) {
			return errorf(ErrWrite, "uint32 array write error")
		}
	}
	return nil
}

func typeUInt32Dup(self *TagTypeHandler, value any, n uint32) (any, error) {
	arr, _ := value.([]uint32)
	out := make([]uint32, len(arr))
	copy(out, arr)
	return out, nil
}

func typeUInt64Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	n := sizeOfTag / 8
	arr := make([]uint64, n)
	for i := uint32(0); i < n; i++ {
		v, ok := readUInt64(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "uint64 array read error")
		}
		arr[i] = v
	}
	return arr, n, nil
}

func typeUInt64Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	arr, ok := value.([]uint64)
	if !ok {
		return errorf(ErrInternal, "uint64 array write: wrong type")
	}
	for i := uint32(0); i < nItems && i < uint32(len(arr)); i++ {
		if !writeUInt64(io, arr[i]) {
			return errorf(ErrWrite, "uint64 array write error")
		}
	}
	return nil
}

func typeUInt64Dup(self *TagTypeHandler, value any, n uint32) (any, error) {
	arr, _ := value.([]uint64)
	out := make([]uint64, len(arr))
	copy(out, arr)
	return out, nil
}

// ---------------------------------------------------------------------------
// Types S15Fixed16 / U16Fixed16 arrays (stored as []float64)
// ---------------------------------------------------------------------------

func typeS15Fixed16Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	n := sizeOfTag / 4
	arr := make([]float64, n)
	for i := uint32(0); i < n; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "s15f16 array read error")
		}
		arr[i] = v
	}
	return arr, n, nil
}

func typeS15Fixed16Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	arr, ok := value.([]float64)
	if !ok {
		return errorf(ErrInternal, "s15f16 array write: wrong type")
	}
	for i := uint32(0); i < nItems && i < uint32(len(arr)); i++ {
		if !write15Fixed16(io, arr[i]) {
			return errorf(ErrWrite, "s15f16 array write error")
		}
	}
	return nil
}

func typeFloat64ArrDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	arr, _ := value.([]float64)
	out := make([]float64, len(arr))
	copy(out, arr)
	return out, nil
}

func typeU16Fixed16Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	n := sizeOfTag / 4
	arr := make([]float64, n)
	for i := uint32(0); i < n; i++ {
		v, ok := readUInt32(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "u16f16 array read error")
		}
		arr[i] = float64(v) / 65536.0
	}
	return arr, n, nil
}

func typeU16Fixed16Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	arr, ok := value.([]float64)
	if !ok {
		return errorf(ErrInternal, "u16f16 array write: wrong type")
	}
	for i := uint32(0); i < nItems && i < uint32(len(arr)); i++ {
		v := uint32(math.Floor(arr[i]*65536.0 + 0.5))
		if !writeUInt32(io, v) {
			return errorf(ErrWrite, "u16f16 array write error")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Type Signature
// ---------------------------------------------------------------------------

func typeSignatureRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	v, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "signature read error")
	}
	return Signature(v), 1, nil
}

func typeSignatureWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	sig, ok := value.(Signature)
	if !ok {
		return errorf(ErrInternal, "signature write: wrong type")
	}
	return wErr(writeUInt32(io, uint32(sig)))
}

func typeSignatureDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	return value, nil
}

// ---------------------------------------------------------------------------
// Type Text
// ---------------------------------------------------------------------------

func typeTextRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	if sizeOfTag == math.MaxUint32 {
		return nil, 0, errorf(ErrCorruptionDetected, "text read: bad size")
	}
	mlu := self.ContextID.NewMLU(1)
	text := make([]byte, sizeOfTag+1)
	if io.Read(text, 1, sizeOfTag) != sizeOfTag {
		return nil, 0, errorf(ErrRead, "text read error")
	}
	text[sizeOfTag] = 0
	if !mlu.SetASCII("\x00\x00", "\x00\x00", string(text[:cstrlen(text)])) {
		return nil, 0, errorf(ErrCorruptionDetected, "text read: set failed")
	}
	return mlu, 1, nil
}

func typeTextWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mlu, ok := value.(*MLU)
	if !ok {
		return errorf(ErrInternal, "text write: wrong type")
	}
	size := mlu.mluGetASCII(noLangCode, noCntryCode, nil)
	if size == 0 {
		return errorf(ErrWrite, "text write: empty")
	}
	text := make([]byte, size)
	mlu.mluGetASCII(noLangCode, noCntryCode, text)
	return wErr(io.Write(size, text))
}

func typeMLUDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	mlu, ok := value.(*MLU)
	if !ok {
		return nil, errorf(ErrInternal, "mlu dup: wrong type")
	}
	return mlu.Dup(), nil
}

// decideTextType lives in tagdesc.go with the descriptor table.

// ---------------------------------------------------------------------------
// Type Data
// ---------------------------------------------------------------------------

func typeDataRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	if sizeOfTag < 4 {
		return nil, 0, errorf(ErrCorruptionDetected, "data read: short tag")
	}
	lenOfData := sizeOfTag - 4
	if lenOfData > math.MaxInt32 {
		return nil, 0, errorf(ErrCorruptionDetected, "data read: too large")
	}
	flag, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "data read error")
	}
	data := make([]byte, lenOfData)
	if io.Read(data, 1, lenOfData) != lenOfData {
		return nil, 0, errorf(ErrRead, "data read error")
	}
	return &ICCData{Flag: flag, Data: data}, 1, nil
}

func typeDataWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	bin, ok := value.(*ICCData)
	if !ok {
		return errorf(ErrInternal, "data write: wrong type")
	}
	if !writeUInt32(io, bin.Flag) {
		return errorf(ErrWrite, "data write error")
	}
	return wErr(io.Write(uint32(len(bin.Data)), bin.Data))
}

func typeDataDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	bin, ok := value.(*ICCData)
	if !ok {
		return nil, errorf(ErrInternal, "data dup: wrong type")
	}
	out := &ICCData{Flag: bin.Flag, Data: make([]byte, len(bin.Data))}
	copy(out.Data, bin.Data)
	return out, nil
}

// ---------------------------------------------------------------------------
// Type Text Description (v2 'desc')
// ---------------------------------------------------------------------------

func typeTextDescriptionRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	if sizeOfTag < 4 {
		return nil, 0, errorf(ErrCorruptionDetected, "text desc: short tag")
	}
	asciiCount, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "text desc read error")
	}
	if asciiCount > 0x7ffff {
		return nil, 0, errorf(ErrCorruptionDetected, "text desc: ascii too long")
	}
	sizeOfTag -= 4
	if sizeOfTag < asciiCount {
		return nil, 0, errorf(ErrCorruptionDetected, "text desc: bad ascii size")
	}
	mlu := self.ContextID.NewMLU(2)

	text := make([]byte, asciiCount+1)
	if io.Read(text, 1, asciiCount) != asciiCount {
		return nil, 0, errorf(ErrRead, "text desc read error")
	}
	sizeOfTag -= asciiCount
	text[asciiCount] = 0
	if !mlu.SetASCII("\x00\x00", "\x00\x00", string(text[:cstrlen(text)])) {
		return nil, 0, errorf(ErrCorruptionDetected, "text desc: set failed")
	}

	// From here we are tolerant to truncated/mis-sized tags: bail out to Done.
	done := func() (any, uint32, error) { return mlu, 1, nil }

	if sizeOfTag < 2*4 {
		return done()
	}
	if _, ok := readUInt32(io); !ok { // Unicode code
		return done()
	}
	unicodeCount, ok := readUInt32(io)
	if !ok {
		return done()
	}
	sizeOfTag -= 2 * 4
	if unicodeCount == 0 || unicodeCount > 0x7ffff || sizeOfTag < unicodeCount*2 {
		return done()
	}
	uni := make([]rune, unicodeCount+1)
	if !readWCharArray(io, unicodeCount, uni[:unicodeCount]) {
		return done()
	}
	uni[unicodeCount] = 0
	if !mlu.SetWide("\xff\xff", "\xff\xff", uni[:mywcslen(uni)]) {
		return done()
	}
	sizeOfTag -= unicodeCount * 2

	// ScriptCode block (skipped if present).
	if sizeOfTag >= 2+1+67 {
		if _, ok := readUInt16(io); !ok { // ScriptCodeCode
			return done()
		}
		if _, ok := readUInt8(io); !ok { // ScriptCodeCount
			return done()
		}
		var dummy [1]byte
		for i := 0; i < 67; i++ {
			if io.Read(dummy[:], 1, 1) != 1 {
				return nil, 0, errorf(ErrRead, "text desc read error")
			}
		}
	}
	return done()
}

func typeTextDescriptionWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mlu, ok := value.(*MLU)
	if !ok {
		return errorf(ErrInternal, "text desc write: wrong type")
	}

	length := mlu.mluGetASCII(noLangCode, noCntryCode, nil)

	var text []byte
	var wide []rune
	if length == 0 {
		text = []byte{0}
		wide = []rune{0}
	} else {
		text = make([]byte, length)
		wide = make([]rune, length)
		mlu.mluGetASCII(noLangCode, noCntryCode, text)
		mlu.mluGetWideBuf(v2UnicodeL, v2UnicodeC, wide)
	}

	lenText := cstrlen(text) + 1
	lenTagRequirement := 8 + 4 + lenText + 4 + 4 + 2*lenText + 2 + 1 + 67
	lenAligned := alignLong(lenTagRequirement)

	if !writeUInt32(io, lenText) {
		return errorf(ErrWrite, "text desc write error")
	}
	if !io.Write(lenText, text) {
		return errorf(ErrWrite, "text desc write error")
	}
	if !writeUInt32(io, 0) { // ucLanguageCode
		return errorf(ErrWrite, "text desc write error")
	}
	if !writeUInt32(io, lenText) {
		return errorf(ErrWrite, "text desc write error")
	}
	if !writeWCharArray(io, lenText, wide) {
		return errorf(ErrWrite, "text desc write error")
	}
	if !writeUInt16(io, 0) { // ScriptCode code
		return errorf(ErrWrite, "text desc write error")
	}
	if !writeUInt8(io, 0) { // ScriptCode count
		return errorf(ErrWrite, "text desc write error")
	}
	if !writeZeros(io, 67) {
		return errorf(ErrWrite, "text desc write error")
	}
	if lenAligned > lenTagRequirement {
		if !writeZeros(io, lenAligned-lenTagRequirement) {
			return errorf(ErrWrite, "text desc write error")
		}
	}
	return nil
}

// decideTextDescType lives in tagdesc.go with the descriptor table.

// ---------------------------------------------------------------------------
// Type Curve
// ---------------------------------------------------------------------------

func typeCurveRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "curve read error")
	}
	switch count {
	case 0: // Linear
		g, err := self.ContextID.BuildParametricToneCurve(1, []float64{1.0})
		if err != nil {
			return nil, 0, err
		}
		return g, 1, nil
	case 1: // Gamma exponent in 8.8 fixed
		fixed, ok := readUInt16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "curve read error")
		}
		g, err := self.ContextID.BuildParametricToneCurve(1, []float64{fixed8ToDouble(fixed)})
		if err != nil {
			return nil, 0, err
		}
		return g, 1, nil
	default:
		if count > 0x7FFF {
			return nil, 0, errorf(ErrCorruptionDetected, "curve: too many entries")
		}
		vals := make([]uint16, count)
		if !readUInt16Array(io, count, vals) {
			return nil, 0, errorf(ErrRead, "curve read error")
		}
		g, err := self.ContextID.BuildTabulatedToneCurve16(vals)
		if err != nil {
			return nil, 0, err
		}
		return g, 1, nil
	}
}

func typeCurveWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	curve, ok := value.(*ToneCurve)
	if !ok {
		return errorf(ErrInternal, "curve write: wrong type")
	}
	if curve.nSegments == 1 && curve.segments[0].Type == 1 {
		fixed := doubleTo8Fixed8(curve.segments[0].Params[0])
		if !writeUInt32(io, 1) {
			return errorf(ErrWrite, "curve write error")
		}
		return wErr(writeUInt16(io, fixed))
	}
	if !writeUInt32(io, curve.nEntries) {
		return errorf(ErrWrite, "curve write error")
	}
	return wErr(writeUInt16Array(io, curve.nEntries, curve.table16))
}

func typeCurveDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	curve, ok := value.(*ToneCurve)
	if !ok {
		return nil, errorf(ErrInternal, "curve dup: wrong type")
	}
	return curve.Dup()
}

// decideCurveType lives in tagdesc.go with the descriptor table.

// ---------------------------------------------------------------------------
// Type ParametricCurve
// ---------------------------------------------------------------------------

var paramsByTypeRead = [...]int{1, 3, 4, 5, 7}

func typeParametricCurveRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	typ, ok := readUInt16(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "parametric curve read error")
	}
	if _, ok := readUInt16(io); !ok { // Reserved
		return nil, 0, errorf(ErrRead, "parametric curve read error")
	}
	if typ > 4 {
		return nil, 0, self.ContextID.signalError(ErrUnknownExtension, "Unknown parametric curve type '%d'", typ)
	}
	var params [10]float64
	n := paramsByTypeRead[typ]
	for i := 0; i < n; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "parametric curve read error")
		}
		params[i] = v
	}
	g, err := self.ContextID.BuildParametricToneCurve(int32(typ)+1, params[:])
	if err != nil {
		return nil, 0, err
	}
	return g, 1, nil
}

var paramsByTypeWrite = [...]int{0, 1, 3, 4, 5, 7}

func typeParametricCurveWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	curve, ok := value.(*ToneCurve)
	if !ok {
		return errorf(ErrInternal, "parametric curve write: wrong type")
	}
	typen := curve.segments[0].Type
	if curve.nSegments > 1 || typen < 1 {
		return self.ContextID.signalError(ErrUnknownExtension, "Multisegment or Inverted parametric curves cannot be written")
	}
	if typen > 5 {
		return self.ContextID.signalError(ErrUnknownExtension, "Unsupported parametric curve")
	}
	nParams := paramsByTypeWrite[typen]
	if !writeUInt16(io, uint16(typen-1)) {
		return errorf(ErrWrite, "parametric curve write error")
	}
	if !writeUInt16(io, 0) {
		return errorf(ErrWrite, "parametric curve write error")
	}
	for i := 0; i < nParams; i++ {
		if !write15Fixed16(io, curve.segments[0].Params[i]) {
			return errorf(ErrWrite, "parametric curve write error")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Type DateTime
// ---------------------------------------------------------------------------

func typeDateTimeRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	var b [12]byte
	if io.Read(b[:], 12, 1) != 1 {
		return nil, 0, errorf(ErrRead, "datetime read error")
	}
	dt := readDateTime(b[:])
	return &dt, 1, nil
}

func typeDateTimeWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	dt, ok := value.(*DateTimeNumber)
	if !ok {
		return errorf(ErrInternal, "datetime write: wrong type")
	}
	var b [12]byte
	encodeDateTime(b[:], *dt)
	return wErr(io.Write(12, b[:]))
}

func typeDateTimeDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	dt, ok := value.(*DateTimeNumber)
	if !ok {
		return nil, errorf(ErrInternal, "datetime dup: wrong type")
	}
	v := *dt
	return &v, nil
}

// ---------------------------------------------------------------------------
// Type Measurement
// ---------------------------------------------------------------------------

func typeMeasurementRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	var mc ICCMeasurementConditions
	obs, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "measurement read error")
	}
	mc.Observer = obs
	if mc.Backing, ok = readXYZ(io); !ok {
		return nil, 0, errorf(ErrRead, "measurement read error")
	}
	if mc.Geometry, ok = readUInt32(io); !ok {
		return nil, 0, errorf(ErrRead, "measurement read error")
	}
	if mc.Flare, ok = read15Fixed16(io); !ok {
		return nil, 0, errorf(ErrRead, "measurement read error")
	}
	if mc.IlluminantType, ok = readUInt32(io); !ok {
		return nil, 0, errorf(ErrRead, "measurement read error")
	}
	return &mc, 1, nil
}

func typeMeasurementWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mc, ok := value.(*ICCMeasurementConditions)
	if !ok {
		return errorf(ErrInternal, "measurement write: wrong type")
	}
	if !writeUInt32(io, mc.Observer) ||
		!writeXYZ(io, mc.Backing) ||
		!writeUInt32(io, mc.Geometry) ||
		!write15Fixed16(io, mc.Flare) ||
		!writeUInt32(io, mc.IlluminantType) {
		return errorf(ErrWrite, "measurement write error")
	}
	return nil
}

func typeMeasurementDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	mc, ok := value.(*ICCMeasurementConditions)
	if !ok {
		return nil, errorf(ErrInternal, "measurement dup: wrong type")
	}
	v := *mc
	return &v, nil
}

// ---------------------------------------------------------------------------
// Type MLU (multiLocalizedUnicode)
// ---------------------------------------------------------------------------

func typeMLURead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "mlu read error")
	}
	recLen, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "mlu read error")
	}
	if recLen != 12 {
		return nil, 0, self.ContextID.signalError(ErrUnknownExtension, "multiLocalizedUnicodeType of len != 12 is not supported.")
	}
	// Cap allocation on untrusted count: the directory (12 bytes/record after the
	// 8-byte count+reclen header) must fit within the tag body.
	if count != 0 && (sizeOfTag < 8 || uint64(count) > uint64(sizeOfTag-8)/12) {
		return nil, 0, errorf(ErrCorruptionDetected, "mlu: implausible entry count")
	}

	mlu := self.ContextID.NewMLU(count)
	if mlu == nil {
		// cmsMLUalloc returned NULL; Type_MLU_Read bails out the same way
		// (cmstypes.c:1718) rather than dereferencing it.
		return nil, 0, errorf(ErrRange, "mlu: entry directory too large")
	}
	if count > mlu.allocatedEntries {
		// NewMLU(0) yields 2 slots; that is fine since usedEntries stays 0.
		mlu.entries = make([]mluEntry, count)
		mlu.allocatedEntries = count
	}
	mlu.usedEntries = count

	sizeOfHeader := 12*count + tagBaseSize
	var largestPosition uint32

	for i := uint32(0); i < count; i++ {
		lang, ok := readUInt16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "mlu read error")
		}
		cntry, ok := readUInt16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "mlu read error")
		}
		length, ok := readUInt32(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "mlu read error")
		}
		offset, ok := readUInt32(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "mlu read error")
		}
		if offset&1 != 0 {
			return nil, 0, errorf(ErrCorruptionDetected, "mlu: odd offset")
		}
		if offset < sizeOfHeader+8 {
			return nil, 0, errorf(ErrCorruptionDetected, "mlu: offset underflow")
		}
		if offset+length < length || offset+length > sizeOfTag+8 {
			return nil, 0, errorf(ErrCorruptionDetected, "mlu: offset overflow")
		}
		begin := offset - sizeOfHeader - 8
		mlu.entries[i].Language = lang
		mlu.entries[i].Country = cntry
		mlu.entries[i].Len = (length * wcharSize) / 2
		mlu.entries[i].StrW = (begin * wcharSize) / 2
		if end := begin + length; end > largestPosition {
			largestPosition = end
		}
	}

	poolBytes := (largestPosition * wcharSize) / 2
	if poolBytes != 0 {
		if poolBytes&1 != 0 {
			return nil, 0, errorf(ErrCorruptionDetected, "mlu: odd pool size")
		}
		numOfWchar := poolBytes / wcharSize
		block := make([]rune, numOfWchar)
		if !readWCharArray(io, numOfWchar, block) {
			return nil, 0, errorf(ErrRead, "mlu read error")
		}
		mlu.pool = block
	}
	mlu.poolSize = poolBytes
	mlu.poolUsed = poolBytes
	return mlu, 1, nil
}

func typeMLUWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mlu, ok := value.(*MLU)
	if !ok || mlu == nil {
		// Empty placeholder, as the reference writes for a NULL pointer.
		if !writeUInt32(io, 0) || !writeUInt32(io, 12) {
			return errorf(ErrWrite, "mlu write error")
		}
		return nil
	}
	if !writeUInt32(io, mlu.usedEntries) || !writeUInt32(io, 12) {
		return errorf(ErrWrite, "mlu write error")
	}
	headerSize := 12*mlu.usedEntries + tagBaseSize
	for i := uint32(0); i < mlu.usedEntries; i++ {
		length := (mlu.entries[i].Len * 2) / wcharSize
		offset := (mlu.entries[i].StrW*2)/wcharSize + headerSize + 8
		if !writeUInt16(io, mlu.entries[i].Language) ||
			!writeUInt16(io, mlu.entries[i].Country) ||
			!writeUInt32(io, length) ||
			!writeUInt32(io, offset) {
			return errorf(ErrWrite, "mlu write error")
		}
	}
	if !writeWCharArray(io, mlu.poolUsed/wcharSize, mlu.pool) {
		return errorf(ErrWrite, "mlu write error")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Type ColorantTable
// ---------------------------------------------------------------------------

func typeColorantTableRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "colorant table read error")
	}
	if count > maxChannels {
		return nil, 0, self.ContextID.signalError(ErrRange, "Too many colorants '%d'", count)
	}
	list := self.ContextID.AllocNamedColorList(count, 0, "", "")
	if list == nil {
		return nil, 0, errorf(ErrInternal, "colorant table: alloc failed")
	}
	for i := uint32(0); i < count; i++ {
		var name [34]byte
		if io.Read(name[:], 32, 1) != 1 {
			return nil, 0, errorf(ErrRead, "colorant table read error")
		}
		name[32] = 0
		var pcs [3]uint16
		if !readUInt16Array(io, 3, pcs[:]) {
			return nil, 0, errorf(ErrRead, "colorant table read error")
		}
		if !list.AppendNamedColor(string(name[:cstrlen(name[:])]), &pcs, nil) {
			return nil, 0, errorf(ErrInternal, "colorant table: append failed")
		}
	}
	return list, 1, nil
}

func typeColorantTableWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	list, ok := value.(*NamedColorList)
	if !ok {
		return errorf(ErrInternal, "colorant table write: wrong type")
	}
	nColors := list.Count()
	if !writeUInt32(io, nColors) {
		return errorf(ErrWrite, "colorant table write error")
	}
	for i := uint32(0); i < nColors; i++ {
		name, _, _, pcs, _, okInfo := list.Info(i)
		if !okInfo {
			return errorf(ErrWrite, "colorant table write error")
		}
		var root [32]byte
		copyRoot(root[:], name)
		if !io.Write(32, root[:]) {
			return errorf(ErrWrite, "colorant table write error")
		}
		if !writeUInt16Array(io, 3, pcs[:]) {
			return errorf(ErrWrite, "colorant table write error")
		}
	}
	return nil
}

// copyRoot copies up to len(dst) bytes of name, zero-padding the rest, matching
// the reference's memset+strcpy into a fixed field written 32 bytes wide.
func copyRoot(dst []byte, name string) {
	n := len(dst)
	if n > len(name) {
		n = len(name)
	}
	for i := 0; i < n; i++ {
		dst[i] = name[i]
	}
	for i := n; i < len(dst); i++ {
		dst[i] = 0
	}
}

func typeNamedColorListDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	list, ok := value.(*NamedColorList)
	if !ok {
		return nil, errorf(ErrInternal, "named color list dup: wrong type")
	}
	return list.Dup(), nil
}

// ---------------------------------------------------------------------------
// Type NamedColor2
// ---------------------------------------------------------------------------

func typeNamedColorRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	if _, ok := readUInt32(io); !ok { // vendorFlag
		return nil, 0, errorf(ErrRead, "named color read error")
	}
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "named color read error")
	}
	nDeviceCoords, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "named color read error")
	}
	var prefix, suffix [32]byte
	if io.Read(prefix[:], 32, 1) != 1 {
		return nil, 0, errorf(ErrRead, "named color read error")
	}
	if io.Read(suffix[:], 32, 1) != 1 {
		return nil, 0, errorf(ErrRead, "named color read error")
	}
	prefix[31] = 0
	suffix[31] = 0

	v := self.ContextID.AllocNamedColorList(count, nDeviceCoords,
		string(prefix[:cstrlen(prefix[:])]), string(suffix[:cstrlen(suffix[:])]))
	if v == nil {
		return nil, 0, self.ContextID.signalError(ErrRange, "Too many named colors '%d'", count)
	}
	if nDeviceCoords > maxChannels {
		return nil, 0, self.ContextID.signalError(ErrRange, "Too many device coordinates '%d'", nDeviceCoords)
	}
	for i := uint32(0); i < count; i++ {
		var root [33]byte
		if io.Read(root[:], 32, 1) != 1 {
			return nil, 0, errorf(ErrRead, "named color read error")
		}
		root[32] = 0
		var pcs [3]uint16
		var colorant [maxChannels]uint16
		if !readUInt16Array(io, 3, pcs[:]) {
			return nil, 0, errorf(ErrRead, "named color read error")
		}
		if !readUInt16Array(io, nDeviceCoords, colorant[:]) {
			return nil, 0, errorf(ErrRead, "named color read error")
		}
		if !v.AppendNamedColor(string(root[:cstrlen(root[:])]), &pcs, &colorant) {
			return nil, 0, errorf(ErrInternal, "named color: append failed")
		}
	}
	return v, 1, nil
}

func typeNamedColorWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	list, ok := value.(*NamedColorList)
	if !ok {
		return errorf(ErrInternal, "named color write: wrong type")
	}
	nColors := list.Count()
	if !writeUInt32(io, 0) || !writeUInt32(io, nColors) || !writeUInt32(io, list.ColorantCount()) {
		return errorf(ErrWrite, "named color write error")
	}
	var prefix, suffix [32]byte
	copyRoot(prefix[:], list.Prefix())
	copyRoot(suffix[:], list.Suffix())
	if !io.Write(32, prefix[:]) || !io.Write(32, suffix[:]) {
		return errorf(ErrWrite, "named color write error")
	}
	for i := uint32(0); i < nColors; i++ {
		name, _, _, pcs, colorant, okInfo := list.Info(i)
		if !okInfo {
			return errorf(ErrWrite, "named color write error")
		}
		var root [32]byte
		copyRoot(root[:], name)
		if !io.Write(32, root[:]) {
			return errorf(ErrWrite, "named color write error")
		}
		if !writeUInt16Array(io, 3, pcs[:]) {
			return errorf(ErrWrite, "named color write error")
		}
		if !writeUInt16Array(io, list.ColorantCount(), colorant[:]) {
			return errorf(ErrWrite, "named color write error")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Type ProfileSequenceDesc
// ---------------------------------------------------------------------------

// readEmbeddedText ports ReadEmbeddedText: read a type base then dispatch to the
// text/desc/mlu reader, returning the parsed MLU.
func readEmbeddedText(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (*MLU, error) {
	baseType := readTypeBase(io)
	var v any
	var err error
	switch baseType {
	case SigTextType:
		v, _, err = typeTextRead(self, io, sizeOfTag)
	case SigTextDescriptionType:
		v, _, err = typeTextDescriptionRead(self, io, sizeOfTag)
	case SigMultiLocalizedUnicodeType:
		v, _, err = typeMLURead(self, io, sizeOfTag)
	default:
		return nil, errorf(ErrCorruptionDetected, "embedded text: bad type")
	}
	if err != nil {
		return nil, err
	}
	mlu, _ := v.(*MLU)
	if mlu == nil {
		return nil, errorf(ErrCorruptionDetected, "embedded text: nil")
	}
	return mlu, nil
}

// saveDescription ports SaveDescription: write a text/mlu block chosen by the
// ICC version stamped on self.
func saveDescription(self *TagTypeHandler, io *IOHandler, text *MLU) error {
	if self.ICCVersion < 0x04000000 {
		if !writeTypeBase(io, SigTextDescriptionType) {
			return errorf(ErrWrite, "save description write error")
		}
		return typeTextDescriptionWrite(self, io, text, 1)
	}
	if !writeTypeBase(io, SigMultiLocalizedUnicodeType) {
		return errorf(ErrWrite, "save description write error")
	}
	return typeMLUWrite(self, io, text, 1)
}

func typeProfileSeqDescRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "pseq read error")
	}
	if sizeOfTag < 4 {
		return nil, 0, errorf(ErrCorruptionDetected, "pseq: short tag")
	}
	sizeOfTag -= 4

	out := self.ContextID.AllocProfileSequenceDescription(count)
	if out == nil {
		return nil, 0, errorf(ErrCorruptionDetected, "pseq: bad count")
	}

	for i := uint32(0); i < count; i++ {
		sec := &out.seq[i]
		var okr bool
		if sec.DeviceMfg, okr = readUInt32(io); !okr {
			return nil, 0, errorf(ErrRead, "pseq read error")
		}
		if sizeOfTag < 4 {
			return nil, 0, errorf(ErrCorruptionDetected, "pseq: short tag")
		}
		sizeOfTag -= 4
		if sec.DeviceModel, okr = readUInt32(io); !okr {
			return nil, 0, errorf(ErrRead, "pseq read error")
		}
		if sizeOfTag < 4 {
			return nil, 0, errorf(ErrCorruptionDetected, "pseq: short tag")
		}
		sizeOfTag -= 4
		if sec.Attributes, okr = readUInt64(io); !okr {
			return nil, 0, errorf(ErrRead, "pseq read error")
		}
		if sizeOfTag < 8 {
			return nil, 0, errorf(ErrCorruptionDetected, "pseq: short tag")
		}
		sizeOfTag -= 8
		if sec.Technology, okr = readUInt32(io); !okr {
			return nil, 0, errorf(ErrRead, "pseq read error")
		}
		if sizeOfTag < 4 {
			return nil, 0, errorf(ErrCorruptionDetected, "pseq: short tag")
		}
		sizeOfTag -= 4

		mfg, err := readEmbeddedText(self, io, sizeOfTag)
		if err != nil {
			return nil, 0, err
		}
		sec.Manufacturer = mfg
		model, err := readEmbeddedText(self, io, sizeOfTag)
		if err != nil {
			return nil, 0, err
		}
		sec.Model = model
	}
	return out, 1, nil
}

func typeProfileSeqDescWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	seq, ok := value.(*ProfileSequence)
	if !ok {
		return errorf(ErrInternal, "pseq write: wrong type")
	}
	if !writeUInt32(io, seq.n) {
		return errorf(ErrWrite, "pseq write error")
	}
	for i := uint32(0); i < seq.n; i++ {
		sec := &seq.seq[i]
		if !writeUInt32(io, sec.DeviceMfg) ||
			!writeUInt32(io, sec.DeviceModel) ||
			!writeUInt64(io, sec.Attributes) ||
			!writeUInt32(io, sec.Technology) {
			return errorf(ErrWrite, "pseq write error")
		}
		if err := saveDescription(self, io, sec.Manufacturer); err != nil {
			return err
		}
		if err := saveDescription(self, io, sec.Model); err != nil {
			return err
		}
	}
	return nil
}

func typeProfileSeqDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	seq, ok := value.(*ProfileSequence)
	if !ok {
		return nil, errorf(ErrInternal, "pseq dup: wrong type")
	}
	return seq.Dup(), nil
}

// ---------------------------------------------------------------------------
// Type ProfileSequenceId
// ---------------------------------------------------------------------------

func readSeqID(self *TagTypeHandler, io *IOHandler, cargo any, n, sizeOfTag uint32) error {
	seq, ok := cargo.(*ProfileSequence)
	if !ok || n >= uint32(len(seq.seq)) {
		return errorf(ErrInternal, "psid: bad cargo")
	}
	sec := &seq.seq[n]
	if io.Read(sec.ProfileID[:], 16, 1) != 1 {
		return errorf(ErrRead, "psid read error")
	}
	mlu, err := readEmbeddedText(self, io, sizeOfTag)
	if err != nil {
		return err
	}
	sec.Description = mlu
	return nil
}

func typeProfileSeqIDRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	baseOffset := io.Tell() - tagBaseSize
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "psid read error")
	}
	out := self.ContextID.AllocProfileSequenceDescription(count)
	if out == nil {
		return nil, 0, errorf(ErrCorruptionDetected, "psid: bad count")
	}
	if err := readPositionTable(self, io, count, baseOffset, out, readSeqID); err != nil {
		return nil, 0, err
	}
	return out, 1, nil
}

func writeSeqID(self *TagTypeHandler, io *IOHandler, cargo any, n, sizeOfTag uint32) error {
	seq, ok := cargo.(*ProfileSequence)
	if !ok || n >= uint32(len(seq.seq)) {
		return errorf(ErrInternal, "psid: bad cargo")
	}
	if !io.Write(16, seq.seq[n].ProfileID[:]) {
		return errorf(ErrWrite, "psid write error")
	}
	return saveDescription(self, io, seq.seq[n].Description)
}

func typeProfileSeqIDWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	seq, ok := value.(*ProfileSequence)
	if !ok {
		return errorf(ErrInternal, "psid write: wrong type")
	}
	baseOffset := io.Tell() - tagBaseSize
	if !writeUInt32(io, seq.n) {
		return errorf(ErrWrite, "psid write error")
	}
	return writePositionTable(self, io, 0, seq.n, baseOffset, seq, writeSeqID)
}

// ---------------------------------------------------------------------------
// Type UcrBg
// ---------------------------------------------------------------------------

func typeUcrBgRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	n := &UcrBg{}
	signed := int64(sizeOfTag)

	if signed < 4 {
		return nil, 0, errorf(ErrCorruptionDetected, "ucrbg: short tag")
	}
	countUcr, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "ucrbg read error")
	}
	signed -= 4
	// Validate the declared count against the remaining tag size *before*
	// allocating: a hostile ucrBg tag can claim count==0xFFFFFFFF in only four
	// bytes, and make([]uint16, count) would otherwise attempt a multi-gigabyte
	// allocation. Each entry needs two bytes on disk.
	if signed < int64(countUcr)*2 {
		return nil, 0, errorf(ErrCorruptionDetected, "ucrbg: bad ucr size")
	}
	ucrVals := make([]uint16, countUcr)
	if !readUInt16Array(io, countUcr, ucrVals) {
		return nil, 0, errorf(ErrRead, "ucrbg read error")
	}
	signed -= int64(countUcr) * 2
	ucr, err := self.ContextID.BuildTabulatedToneCurve16(ucrVals)
	if err != nil {
		return nil, 0, err
	}
	n.Ucr = ucr

	if signed < 4 {
		return nil, 0, errorf(ErrCorruptionDetected, "ucrbg: short tag")
	}
	countBg, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "ucrbg read error")
	}
	signed -= 4
	// Same as the ucr count above: validate before allocating.
	if signed < int64(countBg)*2 {
		return nil, 0, errorf(ErrCorruptionDetected, "ucrbg: bad bg size")
	}
	bgVals := make([]uint16, countBg)
	if !readUInt16Array(io, countBg, bgVals) {
		return nil, 0, errorf(ErrRead, "ucrbg read error")
	}
	signed -= int64(countBg) * 2
	bg, err := self.ContextID.BuildTabulatedToneCurve16(bgVals)
	if err != nil {
		return nil, 0, err
	}
	n.Bg = bg

	if signed < 0 || signed > 32000 {
		return nil, 0, errorf(ErrCorruptionDetected, "ucrbg: bad text size")
	}
	n.Desc = self.ContextID.NewMLU(1)
	text := make([]byte, signed+1)
	if io.Read(text, 1, uint32(signed)) != uint32(signed) {
		return nil, 0, errorf(ErrRead, "ucrbg read error")
	}
	text[signed] = 0
	n.Desc.SetASCII("\x00\x00", "\x00\x00", string(text[:cstrlen(text)]))
	return n, 1, nil
}

func typeUcrBgWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	v, ok := value.(*UcrBg)
	if !ok {
		return errorf(ErrInternal, "ucrbg write: wrong type")
	}
	if !writeUInt32(io, v.Ucr.nEntries) || !writeUInt16Array(io, v.Ucr.nEntries, v.Ucr.table16) {
		return errorf(ErrWrite, "ucrbg write error")
	}
	if !writeUInt32(io, v.Bg.nEntries) || !writeUInt16Array(io, v.Bg.nEntries, v.Bg.table16) {
		return errorf(ErrWrite, "ucrbg write error")
	}
	textSize := v.Desc.mluGetASCII(noLangCode, noCntryCode, nil)
	text := make([]byte, textSize)
	if v.Desc.mluGetASCII(noLangCode, noCntryCode, text) != textSize {
		return errorf(ErrWrite, "ucrbg write error")
	}
	return wErr(io.Write(textSize, text))
}

func typeUcrBgDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	v, ok := value.(*UcrBg)
	if !ok {
		return nil, errorf(ErrInternal, "ucrbg dup: wrong type")
	}
	out := &UcrBg{Desc: v.Desc.Dup()}
	if v.Ucr != nil {
		c, err := v.Ucr.Dup()
		if err != nil {
			return nil, err
		}
		out.Ucr = c
	}
	if v.Bg != nil {
		c, err := v.Bg.Dup()
		if err != nil {
			return nil, err
		}
		out.Bg = c
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Type CrdInfo
// ---------------------------------------------------------------------------

func readCountAndString(self *TagTypeHandler, io *IOHandler, mlu *MLU, sizeOfTag *uint32, section string) error {
	if *sizeOfTag < 4 {
		return errorf(ErrCorruptionDetected, "crdinfo: short tag")
	}
	count, ok := readUInt32(io)
	if !ok {
		return errorf(ErrRead, "crdinfo read error")
	}
	if count > math.MaxUint32-4 {
		return errorf(ErrCorruptionDetected, "crdinfo: bad count")
	}
	if *sizeOfTag < count+4 {
		return errorf(ErrCorruptionDetected, "crdinfo: bad size")
	}
	text := make([]byte, count+1)
	if io.Read(text, 1, count) != count {
		return errorf(ErrRead, "crdinfo read error")
	}
	text[count] = 0
	mlu.SetASCII("PS", section, string(text[:cstrlen(text)]))
	*sizeOfTag -= count + 4
	return nil
}

func writeCountAndString(self *TagTypeHandler, io *IOHandler, mlu *MLU, section string) error {
	textSize := mlu.mluGetASCII(strTo16("PS"), strTo16(section), nil)
	text := make([]byte, textSize)
	if !writeUInt32(io, textSize) {
		return errorf(ErrWrite, "crdinfo write error")
	}
	if mlu.mluGetASCII(strTo16("PS"), strTo16(section), text) == 0 {
		return errorf(ErrWrite, "crdinfo write error")
	}
	return wErr(io.Write(textSize, text))
}

func typeCrdInfoRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	mlu := self.ContextID.NewMLU(5)
	for _, section := range []string{"nm", "#0", "#1", "#2", "#3"} {
		if err := readCountAndString(self, io, mlu, &sizeOfTag, section); err != nil {
			return nil, 0, err
		}
	}
	return mlu, 1, nil
}

func typeCrdInfoWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mlu, ok := value.(*MLU)
	if !ok {
		return errorf(ErrInternal, "crdinfo write: wrong type")
	}
	for _, section := range []string{"nm", "#0", "#1", "#2", "#3"} {
		if err := writeCountAndString(self, io, mlu, section); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Type Screening
// ---------------------------------------------------------------------------

func typeScreeningRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	sc := &Screening{}
	var ok bool
	if sc.Flag, ok = readUInt32(io); !ok {
		return nil, 0, errorf(ErrRead, "screening read error")
	}
	if sc.NChannels, ok = readUInt32(io); !ok {
		return nil, 0, errorf(ErrRead, "screening read error")
	}
	if sc.NChannels > maxChannels-1 {
		sc.NChannels = maxChannels - 1
	}
	for i := uint32(0); i < sc.NChannels; i++ {
		if sc.Channels[i].Frequency, ok = read15Fixed16(io); !ok {
			return nil, 0, errorf(ErrRead, "screening read error")
		}
		if sc.Channels[i].ScreenAngle, ok = read15Fixed16(io); !ok {
			return nil, 0, errorf(ErrRead, "screening read error")
		}
		if sc.Channels[i].SpotShape, ok = readUInt32(io); !ok {
			return nil, 0, errorf(ErrRead, "screening read error")
		}
	}
	return sc, 1, nil
}

func typeScreeningWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	sc, ok := value.(*Screening)
	if !ok {
		return errorf(ErrInternal, "screening write: wrong type")
	}
	if !writeUInt32(io, sc.Flag) || !writeUInt32(io, sc.NChannels) {
		return errorf(ErrWrite, "screening write error")
	}
	for i := uint32(0); i < sc.NChannels && i < maxChannels; i++ {
		if !write15Fixed16(io, sc.Channels[i].Frequency) ||
			!write15Fixed16(io, sc.Channels[i].ScreenAngle) ||
			!writeUInt32(io, sc.Channels[i].SpotShape) {
			return errorf(ErrWrite, "screening write error")
		}
	}
	return nil
}

func typeScreeningDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	sc, ok := value.(*Screening)
	if !ok {
		return nil, errorf(ErrInternal, "screening dup: wrong type")
	}
	v := *sc
	return &v, nil
}

// ---------------------------------------------------------------------------
// Type ViewingConditions
// ---------------------------------------------------------------------------

func typeViewingConditionsRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	vc := &ICCViewingConditions{}
	var ok bool
	if vc.IlluminantXYZ, ok = readXYZ(io); !ok {
		return nil, 0, errorf(ErrRead, "viewing conditions read error")
	}
	if vc.SurroundXYZ, ok = readXYZ(io); !ok {
		return nil, 0, errorf(ErrRead, "viewing conditions read error")
	}
	if vc.IlluminantType, ok = readUInt32(io); !ok {
		return nil, 0, errorf(ErrRead, "viewing conditions read error")
	}
	return vc, 1, nil
}

func typeViewingConditionsWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	vc, ok := value.(*ICCViewingConditions)
	if !ok {
		return errorf(ErrInternal, "viewing conditions write: wrong type")
	}
	if !writeXYZ(io, vc.IlluminantXYZ) ||
		!writeXYZ(io, vc.SurroundXYZ) ||
		!writeUInt32(io, vc.IlluminantType) {
		return errorf(ErrWrite, "viewing conditions write error")
	}
	return nil
}

func typeViewingConditionsDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	vc, ok := value.(*ICCViewingConditions)
	if !ok {
		return nil, errorf(ErrInternal, "viewing conditions dup: wrong type")
	}
	v := *vc
	return &v, nil
}

// ---------------------------------------------------------------------------
// Type vcgt (video card gamma table)
// ---------------------------------------------------------------------------

func typeVcgtRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	tagType, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "vcgt read error")
	}
	curves := make([]*ToneCurve, 3)

	switch tagType {
	case videoCardGammaTableType:
		nChannels, ok := readUInt16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "vcgt read error")
		}
		if nChannels != 3 {
			return nil, 0, self.ContextID.signalError(ErrUnknownExtension, "Unsupported number of channels for VCGT '%d'", nChannels)
		}
		nElems, ok := readUInt16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "vcgt read error")
		}
		nBytes, ok := readUInt16(io)
		if !ok {
			return nil, 0, errorf(ErrRead, "vcgt read error")
		}
		// Adobe quirk fixup.
		if nElems == 256 && nBytes == 1 && sizeOfTag == 1576 {
			nBytes = 2
		}
		for n := 0; n < 3; n++ {
			vals := make([]uint16, nElems)
			switch nBytes {
			case 1:
				for i := uint32(0); i < uint32(nElems); i++ {
					v, ok := readUInt8(io)
					if !ok {
						return nil, 0, errorf(ErrRead, "vcgt read error")
					}
					vals[i] = from8to16(v)
				}
			case 2:
				if !readUInt16Array(io, uint32(nElems), vals) {
					return nil, 0, errorf(ErrRead, "vcgt read error")
				}
			default:
				return nil, 0, self.ContextID.signalError(ErrUnknownExtension, "Unsupported bit depth for VCGT '%d'", nBytes*8)
			}
			c, err := self.ContextID.BuildTabulatedToneCurve16(vals)
			if err != nil {
				return nil, 0, err
			}
			curves[n] = c
		}

	case videoCardGammaFormulaType:
		for n := 0; n < 3; n++ {
			gamma, ok := read15Fixed16(io)
			if !ok {
				return nil, 0, errorf(ErrRead, "vcgt read error")
			}
			min, ok := read15Fixed16(io)
			if !ok {
				return nil, 0, errorf(ErrRead, "vcgt read error")
			}
			max, ok := read15Fixed16(io)
			if !ok {
				return nil, 0, errorf(ErrRead, "vcgt read error")
			}
			params := []float64{
				gamma,
				math.Pow(max-min, 1.0/gamma),
				0, 0, 0,
				min,
				0,
			}
			c, err := self.ContextID.BuildParametricToneCurve(5, params)
			if err != nil {
				return nil, 0, err
			}
			curves[n] = c
		}

	default:
		return nil, 0, self.ContextID.signalError(ErrUnknownExtension, "Unsupported tag type for VCGT '%d'", tagType)
	}
	return curves, 1, nil
}

func typeVcgtWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	curves, ok := value.([]*ToneCurve)
	if !ok || len(curves) < 3 {
		return errorf(ErrInternal, "vcgt write: wrong type")
	}
	if curves[0].ParametricType() == 5 && curves[1].ParametricType() == 5 && curves[2].ParametricType() == 5 {
		if !writeUInt32(io, videoCardGammaFormulaType) {
			return errorf(ErrWrite, "vcgt write error")
		}
		for i := 0; i < 3; i++ {
			gamma := curves[i].segments[0].Params[0]
			min := curves[i].segments[0].Params[5]
			max := math.Pow(curves[i].segments[0].Params[1], gamma) + min
			if !write15Fixed16(io, gamma) || !write15Fixed16(io, min) || !write15Fixed16(io, max) {
				return errorf(ErrWrite, "vcgt write error")
			}
		}
		return nil
	}
	// Store as a table of 256 words.
	if !writeUInt32(io, videoCardGammaTableType) ||
		!writeUInt16(io, 3) || !writeUInt16(io, 256) || !writeUInt16(io, 2) {
		return errorf(ErrWrite, "vcgt write error")
	}
	for i := 0; i < 3; i++ {
		for j := 0; j < 256; j++ {
			v := curves[i].EvalFloat(float32(float64(j) / 255.0))
			n := quickSaturateWord(float64(v) * 65535.0)
			if !writeUInt16(io, n) {
				return errorf(ErrWrite, "vcgt write error")
			}
		}
	}
	return nil
}

func typeVcgtDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	curves, ok := value.([]*ToneCurve)
	if !ok || len(curves) < 3 {
		return nil, errorf(ErrInternal, "vcgt dup: wrong type")
	}
	out := make([]*ToneCurve, 3)
	for i := 0; i < 3; i++ {
		c, err := curves[i].Dup()
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Type VideoSignal (cicp)
// ---------------------------------------------------------------------------

func typeVideoSignalRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	if sizeOfTag != 4 {
		return nil, 0, errorf(ErrCorruptionDetected, "cicp: bad size")
	}
	cicp := &VideoSignalType{}
	var ok bool
	if cicp.ColourPrimaries, ok = readUInt8(io); !ok {
		return nil, 0, errorf(ErrRead, "cicp read error")
	}
	if cicp.TransferCharacteristics, ok = readUInt8(io); !ok {
		return nil, 0, errorf(ErrRead, "cicp read error")
	}
	if cicp.MatrixCoefficients, ok = readUInt8(io); !ok {
		return nil, 0, errorf(ErrRead, "cicp read error")
	}
	if cicp.VideoFullRangeFlag, ok = readUInt8(io); !ok {
		return nil, 0, errorf(ErrRead, "cicp read error")
	}
	return cicp, 1, nil
}

func typeVideoSignalWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	cicp, ok := value.(*VideoSignalType)
	if !ok {
		return errorf(ErrInternal, "cicp write: wrong type")
	}
	if !writeUInt8(io, cicp.ColourPrimaries) ||
		!writeUInt8(io, cicp.TransferCharacteristics) ||
		!writeUInt8(io, cicp.MatrixCoefficients) ||
		!writeUInt8(io, cicp.VideoFullRangeFlag) {
		return errorf(ErrWrite, "cicp write error")
	}
	return nil
}

func typeVideoSignalDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	cicp, ok := value.(*VideoSignalType)
	if !ok {
		return nil, errorf(ErrInternal, "cicp dup: wrong type")
	}
	v := *cicp
	return &v, nil
}

// ---------------------------------------------------------------------------
// Type MHC2
// ---------------------------------------------------------------------------

func mhc2SetIdentity(m *[3][4]float64) {
	*m = [3][4]float64{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}
}

func mhc2CloseEnough(a, b float64) bool { return math.Abs(b-a) < 1.0/65535.0 }

func mhc2IsIdentity(m [3][4]float64) bool {
	var id [3][4]float64
	mhc2SetIdentity(&id)
	for i := 0; i < 3; i++ {
		for j := 0; j < 4; j++ {
			if !mhc2CloseEnough(m[i][j], id[i][j]) {
				return false
			}
		}
	}
	return true
}

func writeDoubles(io *IOHandler, n uint32, values []float64) bool {
	for i := uint32(0); i < n; i++ {
		var v float64
		if i < uint32(len(values)) {
			v = values[i]
		}
		if !write15Fixed16(io, v) {
			return false
		}
	}
	return true
}

func readDoublesAt(io *IOHandler, at, n uint32, dst []float64) error {
	current := io.Tell()
	if !io.Seek(at) {
		return errorf(ErrSeek, "mhc2 seek error")
	}
	for i := uint32(0); i < n; i++ {
		v, ok := read15Fixed16(io)
		if !ok {
			return errorf(ErrRead, "mhc2 read error")
		}
		if i < uint32(len(dst)) {
			dst[i] = v
		}
	}
	if !io.Seek(current) {
		return errorf(ErrSeek, "mhc2 seek error")
	}
	return nil
}

func typeMHC2Read(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	baseOffset := io.Tell() - tagBaseSize
	mhc2 := &MHC2Type{}

	var ok bool
	if mhc2.CurveEntries, ok = readUInt32(io); !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}
	if mhc2.CurveEntries > 4096 {
		return nil, 0, errorf(ErrCorruptionDetected, "mhc2: too many curve entries")
	}
	mhc2.RedCurve = make([]float64, mhc2.CurveEntries)
	mhc2.GreenCurve = make([]float64, mhc2.CurveEntries)
	mhc2.BlueCurve = make([]float64, mhc2.CurveEntries)

	if mhc2.MinLuminance, ok = read15Fixed16(io); !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}
	if mhc2.PeakLuminance, ok = read15Fixed16(io); !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}
	matrixOffset, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}
	offRed, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}
	offGreen, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}
	offBlue, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "mhc2 read error")
	}

	if matrixOffset == 0 {
		mhc2SetIdentity(&mhc2.XYZ2XYZmatrix)
	} else {
		flat := make([]float64, 12)
		if err := readDoublesAt(io, baseOffset+matrixOffset, 12, flat); err != nil {
			return nil, 0, err
		}
		for i := 0; i < 3; i++ {
			for j := 0; j < 4; j++ {
				mhc2.XYZ2XYZmatrix[i][j] = flat[i*4+j]
			}
		}
	}

	if err := readDoublesAt(io, baseOffset+offRed+8, mhc2.CurveEntries, mhc2.RedCurve); err != nil {
		return nil, 0, err
	}
	if err := readDoublesAt(io, baseOffset+offGreen+8, mhc2.CurveEntries, mhc2.GreenCurve); err != nil {
		return nil, 0, err
	}
	if err := readDoublesAt(io, baseOffset+offBlue+8, mhc2.CurveEntries, mhc2.BlueCurve); err != nil {
		return nil, 0, err
	}
	return mhc2, 1, nil
}

func typeMHC2Write(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	mhc2, ok := value.(*MHC2Type)
	if !ok {
		return errorf(ErrInternal, "mhc2 write: wrong type")
	}
	baseOffset := io.Tell() - tagBaseSize

	if !writeUInt32(io, mhc2.CurveEntries) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	if !write15Fixed16(io, mhc2.MinLuminance) || !write15Fixed16(io, mhc2.PeakLuminance) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	tablesOffsetPos := io.Tell()
	for i := 0; i < 4; i++ {
		if !writeUInt32(io, 0) {
			return errorf(ErrWrite, "mhc2 write error")
		}
	}

	var matrixOffset uint32
	if mhc2IsIdentity(mhc2.XYZ2XYZmatrix) {
		matrixOffset = 0
	} else {
		matrixOffset = io.Tell() - baseOffset
		flat := make([]float64, 12)
		for i := 0; i < 3; i++ {
			for j := 0; j < 4; j++ {
				flat[i*4+j] = mhc2.XYZ2XYZmatrix[i][j]
			}
		}
		if !writeDoubles(io, 12, flat) {
			return errorf(ErrWrite, "mhc2 write error")
		}
	}

	offRed := io.Tell() - baseOffset
	if !writeUInt32(io, uint32(SigS15Fixed16ArrayType)) || !writeUInt32(io, 0) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	if !writeDoubles(io, mhc2.CurveEntries, mhc2.RedCurve) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	offGreen := io.Tell() - baseOffset
	if !writeUInt32(io, uint32(SigS15Fixed16ArrayType)) || !writeUInt32(io, 0) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	if !writeDoubles(io, mhc2.CurveEntries, mhc2.GreenCurve) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	offBlue := io.Tell() - baseOffset
	if !writeUInt32(io, uint32(SigS15Fixed16ArrayType)) || !writeUInt32(io, 0) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	if !writeDoubles(io, mhc2.CurveEntries, mhc2.BlueCurve) {
		return errorf(ErrWrite, "mhc2 write error")
	}

	if !io.Seek(tablesOffsetPos) {
		return errorf(ErrSeek, "mhc2 write error")
	}
	if !writeUInt32(io, matrixOffset) ||
		!writeUInt32(io, offRed) ||
		!writeUInt32(io, offGreen) ||
		!writeUInt32(io, offBlue) {
		return errorf(ErrWrite, "mhc2 write error")
	}
	return nil
}

func typeMHC2Dup(self *TagTypeHandler, value any, n uint32) (any, error) {
	mhc2, ok := value.(*MHC2Type)
	if !ok {
		return nil, errorf(ErrInternal, "mhc2 dup: wrong type")
	}
	out := *mhc2
	out.RedCurve = append([]float64(nil), mhc2.RedCurve...)
	out.GreenCurve = append([]float64(nil), mhc2.GreenCurve...)
	out.BlueCurve = append([]float64(nil), mhc2.BlueCurve...)
	return &out, nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func init() {
	reg := func(sig TagTypeSignature, read func(*TagTypeHandler, *IOHandler, uint32) (any, uint32, error),
		write func(*TagTypeHandler, *IOHandler, any, uint32) error,
		dup func(*TagTypeHandler, any, uint32) (any, error)) {
		registerBuiltinType(&TagTypeHandler{Signature: sig, Read: read, Write: write, Dup: dup})
	}

	reg(SigXYZType, typeXYZRead, typeXYZWrite, typeXYZDup)
	reg(0x17A505B8, typeXYZRead, typeXYZWrite, typeXYZDup) // cmsCorbisBrokenXYZtype
	reg(SigChromaticityType, typeChromaticityRead, typeChromaticityWrite, typeChromaticityDup)
	reg(SigColorantOrderType, typeColorantOrderRead, typeColorantOrderWrite, typeColorantOrderDup)
	reg(SigUInt8ArrayType, typeUInt8Read, typeUInt8Write, typeUInt8Dup)
	reg(SigUInt32ArrayType, typeUInt32Read, typeUInt32Write, typeUInt32Dup)
	reg(SigUInt64ArrayType, typeUInt64Read, typeUInt64Write, typeUInt64Dup)
	reg(SigS15Fixed16ArrayType, typeS15Fixed16Read, typeS15Fixed16Write, typeFloat64ArrDup)
	reg(SigU16Fixed16ArrayType, typeU16Fixed16Read, typeU16Fixed16Write, typeFloat64ArrDup)
	reg(SigSignatureType, typeSignatureRead, typeSignatureWrite, typeSignatureDup)
	reg(SigTextType, typeTextRead, typeTextWrite, typeMLUDup)
	reg(SigDataType, typeDataRead, typeDataWrite, typeDataDup)
	reg(SigTextDescriptionType, typeTextDescriptionRead, typeTextDescriptionWrite, typeMLUDup)
	reg(SigCurveType, typeCurveRead, typeCurveWrite, typeCurveDup)
	reg(0x9478ee00, typeCurveRead, typeCurveWrite, typeCurveDup) // cmsMonacoBrokenCurveType
	reg(SigParametricCurveType, typeParametricCurveRead, typeParametricCurveWrite, typeCurveDup)
	reg(SigDateTimeType, typeDateTimeRead, typeDateTimeWrite, typeDateTimeDup)
	reg(SigMeasurementType, typeMeasurementRead, typeMeasurementWrite, typeMeasurementDup)
	reg(SigMultiLocalizedUnicodeType, typeMLURead, typeMLUWrite, typeMLUDup)
	reg(SigColorantTableType, typeColorantTableRead, typeColorantTableWrite, typeNamedColorListDup)
	reg(SigNamedColor2Type, typeNamedColorRead, typeNamedColorWrite, typeNamedColorListDup)
	reg(SigProfileSequenceDescType, typeProfileSeqDescRead, typeProfileSeqDescWrite, typeProfileSeqDup)
	reg(SigProfileSequenceIdType, typeProfileSeqIDRead, typeProfileSeqIDWrite, typeProfileSeqDup)
	reg(SigUcrBgType, typeUcrBgRead, typeUcrBgWrite, typeUcrBgDup)
	reg(SigCrdInfoType, typeCrdInfoRead, typeCrdInfoWrite, typeMLUDup)
	reg(SigScreeningType, typeScreeningRead, typeScreeningWrite, typeScreeningDup)
	reg(SigViewingConditionsType, typeViewingConditionsRead, typeViewingConditionsWrite, typeViewingConditionsDup)
	reg(SigVcgtType, typeVcgtRead, typeVcgtWrite, typeVcgtDup)
	reg(SigcicpType, typeVideoSignalRead, typeVideoSignalWrite, typeVideoSignalDup)
	reg(SigMHC2Type, typeMHC2Read, typeMHC2Write, typeMHC2Dup)
}
