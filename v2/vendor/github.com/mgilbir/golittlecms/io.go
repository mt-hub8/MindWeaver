// Port of the I/O handler layer and endian/serialization primitives from
// src/cmsio0.c (the FILENULL/FILEMEM/FILE handlers and their constructors) and
// src/cmsplugin.c (the _cmsReadUInt*/_cmsWrite* family, the type-base and
// alignment helpers, the date/time codec and the double<->fixed converters that
// fixed.go deferred to this phase).
//
// Design notes (idiomatic Go, C behavioural semantics preserved):
//
//   - cmsIOHANDLER becomes *IOHandler: a struct carrying the shared fields C
//     keeps on the handler (ContextID, UsedSpace, ReportedSize, PhysicalFile)
//     plus a private ioStream implementation providing Read/Seek/Tell/Write/
//     Close. The stream methods receive the *IOHandler so they can update
//     UsedSpace exactly where the C function pointers do.
//   - The C Read signature Read(io, buf, size, count) returns the number of
//     elements read; it is preserved. Callers pass a []byte at least size*count
//     long. Write(size, buf) writes the first size bytes of buf and returns a
//     bool, matching FileWrite/MemoryWrite/NULLWrite.
//   - The endian helpers return (ok bool) like their C bool counterparts; the
//     profile layer converts a false into a *Error. This keeps them allocation
//     free on the hot path and bit-for-bit faithful.

package lcms2

import (
	"encoding/binary"
	"math"
	"os"
)

// maxPath mirrors cmsMAX_PATH (include/lcms2.h): the capacity of the handler's
// PhysicalFile field. Go strings are not fixed-width, but the truncation to
// maxPath-1 bytes is preserved for behavioural parity with strncpy.
const maxPath = 256

// ioStream is the medium-specific backing of an IOHandler. Each method mirrors
// one C function pointer and receives the owning handler so it can read and
// update the shared UsedSpace/ReportedSize fields, exactly as the C handlers do.
type ioStream interface {
	// read fills buf with size*count bytes and returns the number of elements
	// (count) actually read, or 0 on error, mirroring the C Read pointer.
	read(io *IOHandler, buf []byte, size, count uint32) uint32
	// seek positions the stream at offset (SEEK_SET) and reports success.
	seek(io *IOHandler, offset uint32) bool
	// tell returns the current position.
	tell(io *IOHandler) uint32
	// write writes the first size bytes of buf and reports success, updating
	// io.UsedSpace as the C handler does.
	write(io *IOHandler, size uint32, buf []byte) bool
	// closeStream releases the backing medium.
	closeStream(io *IOHandler) error
}

// IOHandler is the pure-Go replacement for cmsIOHANDLER. It abstracts reading
// and writing over a memory block, an *os.File, or a byte counter (the NULL
// handler), so the profile container code is agnostic to the storage medium.
type IOHandler struct {
	ContextID    *Context
	UsedSpace    uint32
	ReportedSize uint32
	PhysicalFile string

	stream ioStream
}

// Read reads count elements of size bytes each into buf and returns the number
// of elements read (0 on error), mirroring io->Read.
func (io *IOHandler) Read(buf []byte, size, count uint32) uint32 {
	return io.stream.read(io, buf, size, count)
}

// Seek positions the handler at offset from the start and reports success,
// mirroring io->Seek (SEEK_SET semantics).
func (io *IOHandler) Seek(offset uint32) bool { return io.stream.seek(io, offset) }

// Tell returns the current position, mirroring io->Tell.
func (io *IOHandler) Tell() uint32 { return io.stream.tell(io) }

// Write writes the first size bytes of buf and reports success, mirroring
// io->Write. It updates UsedSpace like the C handlers.
func (io *IOHandler) Write(size uint32, buf []byte) bool { return io.stream.write(io, size, buf) }

// Close releases the handler, mirroring cmsCloseIOhandler.
func (io *IOHandler) Close() error { return io.stream.closeStream(io) }

// ---------------------------------------------------------------- NULL handler

// nullStream implements the NULL IOhandler: it writes nothing but tracks how
// many bytes would have been written, so the profile serializer can size the
// output in its first pass. Mirrors FILENULL.
type nullStream struct {
	pointer uint32
}

func (s *nullStream) read(io *IOHandler, buf []byte, size, count uint32) uint32 {
	s.pointer += size * count
	return count
}

func (s *nullStream) seek(io *IOHandler, offset uint32) bool {
	s.pointer = offset
	return true
}

func (s *nullStream) tell(io *IOHandler) uint32 { return s.pointer }

func (s *nullStream) write(io *IOHandler, size uint32, buf []byte) bool {
	// Guard against wraparound exactly as NULLWrite does.
	if size > 0xFFFFFFFF-s.pointer {
		return false
	}
	s.pointer += size
	if s.pointer > io.UsedSpace {
		io.UsedSpace = s.pointer
	}
	return true
}

func (s *nullStream) closeStream(io *IOHandler) error { return nil }

// OpenIOhandlerFromNULL creates a handler that counts bytes without storing
// them, mirroring cmsOpenIOhandlerFromNULL.
func (ctx *Context) OpenIOhandlerFromNULL() *IOHandler {
	return &IOHandler{ContextID: ctx, stream: &nullStream{}}
}

// OpenIOhandlerFromNULL creates a NULL handler on the default context.
func OpenIOhandlerFromNULL() *IOHandler { return defaultContext.OpenIOhandlerFromNULL() }

// -------------------------------------------------------------- Memory handler

// memStream implements the memory-block IOhandler. In read mode block holds a
// private copy of the source bytes; in write mode block is the caller's target
// buffer and writes are truncated to its capacity. Mirrors FILEMEM.
type memStream struct {
	block   []byte
	size    uint32
	pointer uint32
}

func (s *memStream) read(io *IOHandler, buf []byte, size, count uint32) uint32 {
	if size == 0 || count == 0 {
		return 0
	}
	length := size * count
	// Overflow check, exactly as MemoryRead.
	if length/count != size {
		io.ContextID.signalError(ErrRead, "Read from memory error")
		return 0
	}
	if buf == nil {
		io.ContextID.signalError(ErrRead, "Read from memory error")
		return 0
	}
	if length > s.size {
		io.ContextID.signalError(ErrRead, "Read from memory error")
		return 0
	}
	if s.pointer > s.size-length {
		io.ContextID.signalError(ErrRead, "Read from memory error")
		return 0
	}
	// Defensive: never index past the caller's buffer (all in-tree callers size
	// buf to size*count, but this keeps a mis-sized buffer from panicking).
	if uint32(len(buf)) < length {
		io.ContextID.signalError(ErrRead, "Read from memory error")
		return 0
	}
	copy(buf[:length], s.block[s.pointer:s.pointer+length])
	s.pointer += length
	return count
}

func (s *memStream) seek(io *IOHandler, offset uint32) bool {
	if offset > s.size {
		return false
	}
	s.pointer = offset
	return true
}

func (s *memStream) tell(io *IOHandler) uint32 { return s.pointer }

func (s *memStream) write(io *IOHandler, size uint32, buf []byte) bool {
	if buf == nil {
		io.ContextID.signalError(ErrWrite, "Write to memory error")
		return false
	}
	if size == 0 {
		return true
	}
	// Never slice past the caller's buffer (IOHandler.Write is exported).
	if uint32(len(buf)) < size {
		io.ContextID.signalError(ErrWrite, "Write to memory error: buffer shorter than declared size")
		return false
	}
	// Truncate instead of erroring when space runs out, matching MemoryWrite
	// (see colord issue #147).
	if size > s.size-s.pointer {
		size = s.size - s.pointer
	}
	copy(s.block[s.pointer:s.pointer+size], buf[:size])
	s.pointer += size
	if s.pointer > io.UsedSpace {
		io.UsedSpace = s.pointer
	}
	return true
}

func (s *memStream) closeStream(io *IOHandler) error { return nil }

// OpenIOhandlerFromMem creates a memory-backed handler. When write is false the
// bytes in buf are copied into a private read buffer (so the caller may free buf
// afterwards) and ReportedSize is set to len(buf); when write is true buf is the
// destination and writes are truncated to its length, mirroring the "r"/"w"
// branches of cmsOpenIOhandlerFromMem.
func (ctx *Context) OpenIOhandlerFromMem(buf []byte, write bool) (*IOHandler, error) {
	io := &IOHandler{ContextID: ctx}
	if write {
		io.stream = &memStream{block: buf, size: uint32(len(buf))}
		io.ReportedSize = 0
		return io, nil
	}
	if buf == nil {
		return nil, ctx.signalError(ErrRead, "Couldn't read profile from NULL pointer")
	}
	cp := make([]byte, len(buf))
	copy(cp, buf)
	io.stream = &memStream{block: cp, size: uint32(len(cp))}
	io.ReportedSize = uint32(len(cp))
	return io, nil
}

// ---------------------------------------------------------------- File handler

// fileStream implements the disk-file IOhandler over an *os.File. Mirrors the
// FILE-based handler in cmsio0.c.
type fileStream struct {
	f *os.File
}

func (s *fileStream) read(io *IOHandler, buf []byte, size, count uint32) uint32 {
	// A zero-length request reads nothing, matching FileRead (fread returns 0)
	// and the memory handler; without this, the checks below vacuously "succeed".
	if size == 0 || count == 0 {
		return 0
	}
	// The total is computed and compared in uint64 throughout: size*count in
	// uint32 could wrap and let a partial read report success.
	want := uint64(size) * uint64(count)
	toRead := want
	if toRead > uint64(len(buf)) {
		// A short caller buffer is surfaced as a short read below, never an
		// over-read.
		toRead = uint64(len(buf))
	}
	n, _ := readFull(s.f, buf[:toRead])
	if uint64(n) != want {
		io.ContextID.signalError(ErrFile, "Read error. Got %d bytes, block should be of %d bytes", n, want)
		return 0
	}
	return count
}

// readFull reads len(buf) bytes, tolerating short reads from the OS, mirroring
// fread's blocking behaviour.
func readFull(f *os.File, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := f.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			break
		}
	}
	return total, nil
}

func (s *fileStream) seek(io *IOHandler, offset uint32) bool {
	if _, err := s.f.Seek(int64(offset), 0); err != nil {
		io.ContextID.signalError(ErrFile, "Seek error; probably corrupted file")
		return false
	}
	return true
}

func (s *fileStream) tell(io *IOHandler) uint32 {
	pos, err := s.f.Seek(0, 1)
	if err != nil {
		io.ContextID.signalError(ErrFile, "Tell error; probably corrupted file")
		return 0
	}
	return uint32(pos)
}

func (s *fileStream) write(io *IOHandler, size uint32, buf []byte) bool {
	if size == 0 {
		return true
	}
	// Never slice past the caller's buffer. All in-tree callers pass a buffer of
	// exactly size, but IOHandler.Write is exported, so a short buf must fail
	// cleanly rather than panic.
	if uint32(len(buf)) < size {
		io.ContextID.signalError(ErrWrite, "Write error: buffer shorter than declared size")
		return false
	}
	io.UsedSpace += size
	n, err := s.f.Write(buf[:size])
	return err == nil && uint32(n) == size
}

func (s *fileStream) closeStream(io *IOHandler) error { return s.f.Close() }

// OpenIOhandlerFromFile opens FileName for reading ("r") or writing ("w") and
// returns a file-backed handler, mirroring cmsOpenIOhandlerFromFile. Only the
// 'r' and 'w' modes are meaningful in the Go port; the 'e' (close-on-exec) and
// 'b' (binary) modifiers the C code recognises are no-ops here since Go always
// opens in binary mode and manages descriptors itself.
func (ctx *Context) OpenIOhandlerFromFile(fileName, accessMode string) (*IOHandler, error) {
	mode := byte(0)
	for i := 0; i < len(accessMode); i++ {
		switch accessMode[i] {
		case 'r', 'w':
			if mode == 0 {
				mode = accessMode[i]
			} else {
				return nil, ctx.signalError(ErrFile, "Access mode already specified '%c'", accessMode[i])
			}
		case 'e':
			// close-on-exec: not modelled
		default:
			return nil, ctx.signalError(ErrFile, "Wrong access mode '%c'", accessMode[i])
		}
	}

	io := &IOHandler{ContextID: ctx}
	switch mode {
	case 'r':
		f, err := os.Open(fileName)
		if err != nil {
			return nil, ctx.signalError(ErrFile, "File '%s' not found", fileName)
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, ctx.signalError(ErrFile, "Cannot get size of file '%s'", fileName)
		}
		if fi.Size() > 0xFFFFFFFF {
			f.Close()
			return nil, ctx.signalError(ErrFile, "File '%s' is too large", fileName)
		}
		io.stream = &fileStream{f: f}
		io.ReportedSize = uint32(fi.Size())
	case 'w':
		f, err := os.Create(fileName)
		if err != nil {
			return nil, ctx.signalError(ErrFile, "Couldn't create '%s'", fileName)
		}
		io.stream = &fileStream{f: f}
		io.ReportedSize = 0
	default:
		return nil, ctx.signalError(ErrFile, "Wrong access mode")
	}

	if len(fileName) >= maxPath {
		io.PhysicalFile = fileName[:maxPath-1]
	} else {
		io.PhysicalFile = fileName
	}
	return io, nil
}

// ------------------------------------------------------ endian read primitives
//
// ICC profiles are big-endian; the C helpers read the raw little-endian-or-not
// bytes then _cmsAdjustEndianess to host order. Reading big-endian directly is
// equivalent and endian-independent, which is what these helpers do.

// readUInt8 ports _cmsReadUInt8Number.
func readUInt8(io *IOHandler) (uint8, bool) {
	var b [1]byte
	if io.Read(b[:], 1, 1) != 1 {
		return 0, false
	}
	return b[0], true
}

// readUInt16 ports _cmsReadUInt16Number.
func readUInt16(io *IOHandler) (uint16, bool) {
	var b [2]byte
	if io.Read(b[:], 2, 1) != 1 {
		return 0, false
	}
	return binary.BigEndian.Uint16(b[:]), true
}

// readUInt32 ports _cmsReadUInt32Number.
func readUInt32(io *IOHandler) (uint32, bool) {
	var b [4]byte
	if io.Read(b[:], 4, 1) != 1 {
		return 0, false
	}
	return binary.BigEndian.Uint32(b[:]), true
}

// readUInt64 ports _cmsReadUInt64Number.
func readUInt64(io *IOHandler) (uint64, bool) {
	var b [8]byte
	if io.Read(b[:], 8, 1) != 1 {
		return 0, false
	}
	return binary.BigEndian.Uint64(b[:]), true
}

// readFloat32 ports _cmsReadFloat32Number, including the C safeguards that
// reject values outside +-1e20 and any that are neither zero nor normal.
func readFloat32(io *IOHandler) (float32, bool) {
	var b [4]byte
	if io.Read(b[:], 4, 1) != 1 {
		return 0, false
	}
	n := math.Float32frombits(binary.BigEndian.Uint32(b[:]))
	if n > 1e20 || n < -1e20 {
		return 0, false
	}
	// C returns TRUE only for FP_ZERO or FP_NORMAL, rejecting NaN, Inf and
	// subnormals. A normal float32 has |x| >= 2^-126.
	a := math.Abs(float64(n))
	if n == 0 {
		return n, true
	}
	if math.IsNaN(a) || math.IsInf(a, 0) || a < 0x1p-126 {
		return 0, false
	}
	return n, true
}

// read15Fixed16 ports _cmsRead15Fixed16Number.
func read15Fixed16(io *IOHandler) (float64, bool) {
	v, ok := readUInt32(io)
	if !ok {
		return 0, false
	}
	return s15Fixed16ToDouble(int32(v)), true
}

// readXYZ ports _cmsReadXYZNumber.
func readXYZ(io *IOHandler) (CIEXYZ, bool) {
	x, ok := readUInt32(io)
	if !ok {
		return CIEXYZ{}, false
	}
	y, ok := readUInt32(io)
	if !ok {
		return CIEXYZ{}, false
	}
	z, ok := readUInt32(io)
	if !ok {
		return CIEXYZ{}, false
	}
	return CIEXYZ{
		X: s15Fixed16ToDouble(int32(x)),
		Y: s15Fixed16ToDouble(int32(y)),
		Z: s15Fixed16ToDouble(int32(z)),
	}, true
}

// ----------------------------------------------------- endian write primitives

// writeUInt8 ports _cmsWriteUInt8Number.
func writeUInt8(io *IOHandler, n uint8) bool {
	return io.Write(1, []byte{n})
}

// writeUInt16 ports _cmsWriteUInt16Number.
func writeUInt16(io *IOHandler, n uint16) bool {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], n)
	return io.Write(2, b[:])
}

// writeUInt32 ports _cmsWriteUInt32Number.
func writeUInt32(io *IOHandler, n uint32) bool {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return io.Write(4, b[:])
}

// writeUInt64 ports _cmsWriteUInt64Number.
func writeUInt64(io *IOHandler, n uint64) bool {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	return io.Write(8, b[:])
}

// writeFloat32 ports _cmsWriteFloat32Number.
func writeFloat32(io *IOHandler, n float32) bool {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], math.Float32bits(n))
	return io.Write(4, b[:])
}

// write15Fixed16 ports _cmsWrite15Fixed16Number.
func write15Fixed16(io *IOHandler, n float64) bool {
	return writeUInt32(io, uint32(doubleTo15Fixed16(n)))
}

// writeXYZ ports _cmsWriteXYZNumber.
func writeXYZ(io *IOHandler, xyz CIEXYZ) bool {
	var b [12]byte
	binary.BigEndian.PutUint32(b[0:], uint32(doubleTo15Fixed16(xyz.X)))
	binary.BigEndian.PutUint32(b[4:], uint32(doubleTo15Fixed16(xyz.Y)))
	binary.BigEndian.PutUint32(b[8:], uint32(doubleTo15Fixed16(xyz.Z)))
	return io.Write(12, b[:])
}

// ------------------------------------------------------- fixed-point converters
// (deferred from fixed.go, since they belong with the profile-IO layer)

// doubleTo15Fixed16 ports _cmsDoubleTo15Fixed16 (cmsplugin.c).
func doubleTo15Fixed16(v float64) s15Fixed16 {
	return s15Fixed16(math.Floor(v*65536.0 + 0.5))
}

// double8Fixed8 ports _cms8Fixed8toDouble.
func fixed8ToDouble(fixed8 uint16) float64 { return float64(fixed8) / 256.0 }

// doubleTo8Fixed8 ports _cmsDoubleTo8Fixed8.
func doubleTo8Fixed8(val float64) uint16 {
	gammaFixed32 := doubleTo15Fixed16(val)
	return uint16((gammaFixed32 >> 8) & 0xFFFF)
}

// ----------------------------------------------------------- date/time codec

// DateTimeNumber mirrors cmsDateTimeNumber: the six 16-bit fields of an ICC
// date/time, stored as their natural (decoded) values (Year is the full year,
// Month is 1..12).
type DateTimeNumber struct {
	Year, Month, Day, Hours, Minutes, Seconds uint16
}

// readDateTime reads and decodes a cmsDateTimeNumber, mirroring
// _cmsDecodeDateTimeNumber applied to bytes read from io.
func readDateTime(b []byte) DateTimeNumber {
	return DateTimeNumber{
		Year:    binary.BigEndian.Uint16(b[0:]),
		Month:   binary.BigEndian.Uint16(b[2:]),
		Day:     binary.BigEndian.Uint16(b[4:]),
		Hours:   binary.BigEndian.Uint16(b[6:]),
		Minutes: binary.BigEndian.Uint16(b[8:]),
		Seconds: binary.BigEndian.Uint16(b[10:]),
	}
}

// encodeDateTime serializes a DateTimeNumber into 12 big-endian bytes, mirroring
// _cmsEncodeDateTimeNumber.
func encodeDateTime(dst []byte, d DateTimeNumber) {
	binary.BigEndian.PutUint16(dst[0:], d.Year)
	binary.BigEndian.PutUint16(dst[2:], d.Month)
	binary.BigEndian.PutUint16(dst[4:], d.Day)
	binary.BigEndian.PutUint16(dst[6:], d.Hours)
	binary.BigEndian.PutUint16(dst[8:], d.Minutes)
	binary.BigEndian.PutUint16(dst[10:], d.Seconds)
}

// ------------------------------------------------------ type base & alignment

// tagBaseSize is sizeof(_cmsTagBase): a 4-byte type signature plus 4 reserved
// bytes.
const tagBaseSize = 8

// readTypeBase ports _cmsReadTypeBase: read an 8-byte tag base and return its
// type signature (0 on read error).
func readTypeBase(io *IOHandler) TagTypeSignature {
	var b [tagBaseSize]byte
	if io.Read(b[:], tagBaseSize, 1) != 1 {
		return 0
	}
	return TagTypeSignature(binary.BigEndian.Uint32(b[:4]))
}

// writeTypeBase ports _cmsWriteTypeBase: write the type signature followed by 4
// zero reserved bytes.
func writeTypeBase(io *IOHandler, sig TagTypeSignature) bool {
	var b [tagBaseSize]byte
	binary.BigEndian.PutUint32(b[:4], uint32(sig))
	return io.Write(tagBaseSize, b[:])
}

// readAlignment ports _cmsReadAlignment: consume padding bytes up to the next
// 32-bit boundary.
func readAlignment(io *IOHandler) bool {
	at := io.Tell()
	nextAligned := alignLong(at)
	bytesToNext := nextAligned - at
	if bytesToNext == 0 {
		return true
	}
	if bytesToNext > 4 {
		return false
	}
	var buf [4]byte
	return io.Read(buf[:], bytesToNext, 1) == 1
}

// writeAlignment ports _cmsWriteAlignment: emit zero padding up to the next
// 32-bit boundary.
func writeAlignment(io *IOHandler) bool {
	at := io.Tell()
	nextAligned := alignLong(at)
	bytesToNext := nextAligned - at
	if bytesToNext == 0 {
		return true
	}
	if bytesToNext > 4 {
		return false
	}
	var buf [4]byte
	return io.Write(bytesToNext, buf[:])
}
