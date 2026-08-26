// Port of src/cmsio0.c — the ICC profile container: the _cmsICCPROFILE struct,
// header and tag-directory parse/write with all of the reference's validations,
// raw and cooked tag access, the two-pass serializer, and the header accessors.
// The MD5 profile-ID computation from src/cmsmd5.c (cmsMD5computeID) lives here
// too, since it drives the serializer.
//
// Behavioural fidelity notes:
//
//   - cmsHPROFILE becomes *Profile. The C code snapshots the whole struct with
//     memmove during save/ID and restores it afterwards; the Go port separates
//     the copyable state into an embedded profileData value so the snapshot is a
//     plain struct copy that go vet accepts (no lock copying).
//   - The tag arrays are fixed [maxTableTag] arrays exactly as in C, so a
//     profileData copy duplicates them by value. TagPtrs hold parsed Go values
//     (any); tagRaw holds the byte block for tags written or saved as raw.
//   - cmsReadTag/cmsWriteTag dispatch through the tag-type registry (tagtype.go).
//     Until W8 populates the built-in tables every cooked tag read/write returns
//     an error, but the container, raw access, header parsing and the blind-copy
//     save path are fully functional without any handlers.

package lcms2

import (
	"sync"
	"time"
)

// iccHeaderSize is sizeof(cmsICCHeader): the fixed 128-byte profile header.
const iccHeaderSize = 128

// tagEntrySize is sizeof(cmsTagEntry): signature, offset and size, 12 bytes.
const tagEntrySize = 12

// profileData holds all of a profile's copyable state. It is separated from the
// Profile's mutex so the serializer can snapshot and restore it with a plain
// assignment, mirroring the C memmove of _cmsICCPROFILE without copying a lock.
type profileData struct {
	IOhandler *IOHandler
	ContextID *Context

	Created DateTimeNumber

	CMM             uint32
	Version         uint32
	DeviceClass     ProfileClassSignature
	ColorSpace      ColorSpaceSignature
	PCS             ColorSpaceSignature
	RenderingIntent uint32

	platform     PlatformSignature
	flags        uint32
	manufacturer uint32
	model        uint32
	attributes   uint64
	creator      uint32

	ProfileID profileID

	TagCount        uint32
	TagNames        [maxTableTag]TagSignature
	TagLinked       [maxTableTag]TagSignature
	TagSizes        [maxTableTag]uint32
	TagOffsets      [maxTableTag]uint32
	TagSaveAsRaw    [maxTableTag]bool
	TagPtrs         [maxTableTag]any
	tagRaw          [maxTableTag][]byte
	TagTypeHandlers [maxTableTag]*TagTypeHandler

	IsWrite bool
}

// Profile is the pure-Go replacement for cmsHPROFILE (_cmsICCPROFILE). It is not
// safe for concurrent mutation; the embedded mutex guards the tag-read cache and
// the serializer, matching the intent of the C UsrMutex.
type Profile struct {
	mu sync.Mutex
	profileData
}

// Context returns the profile's context, mirroring cmsGetProfileContextID.
func (p *Profile) Context() *Context {
	if p == nil {
		return nil
	}
	return p.ContextID
}

// IOHandler returns the profile's underlying IO handler, mirroring
// cmsGetProfileIOhandler.
func (p *Profile) IOHandler() *IOHandler {
	if p == nil {
		return nil
	}
	return p.IOhandler
}

// CreateProfilePlaceholder builds an empty profile with the reference defaults,
// mirroring cmsCreateProfilePlaceholder.
func (ctx *Context) CreateProfilePlaceholder() *Profile {
	p := &Profile{}
	p.ContextID = ctx
	p.TagCount = 0
	p.Version = 0x02100000
	p.CMM = LcmsSignature
	p.creator = LcmsSignature
	// The reference defaults to the host platform; the oracle builds on a
	// Unix-like host, so mirror its cmsSigMacintosh default.
	p.platform = SigMacintosh
	p.DeviceClass = SigDisplayClass
	now := time.Now().UTC()
	p.Created = DateTimeNumber{
		Year:    uint16(now.Year()),
		Month:   uint16(now.Month()),
		Day:     uint16(now.Day()),
		Hours:   uint16(now.Hour()),
		Minutes: uint16(now.Minute()),
		Seconds: uint16(now.Second()),
	}
	return p
}

// CreateProfilePlaceholder builds an empty profile on the default context.
func CreateProfilePlaceholder() *Profile { return defaultContext.CreateProfilePlaceholder() }

// GetTagCount returns the number of tags, mirroring cmsGetTagCount. A nil
// profile yields -1.
func (p *Profile) GetTagCount() int {
	if p == nil {
		return -1
	}
	return int(p.TagCount)
}

// GetTagSignature returns the signature of tag n, or 0 if out of range,
// mirroring cmsGetTagSignature.
func (p *Profile) GetTagSignature(n uint32) TagSignature {
	if n > p.TagCount {
		return 0
	}
	if n >= maxTableTag {
		return 0
	}
	return p.TagNames[n]
}

// GetTagOffsetAndSize returns the on-disk location of tag n, mirroring
// cmsGetTagOffsetAndSize.
func (p *Profile) GetTagOffsetAndSize(n uint32) (offset, size uint32, ok bool) {
	if n > p.TagCount || n >= maxTableTag {
		return 0, 0, false
	}
	return p.TagOffsets[n], p.TagSizes[n], true
}

// searchOneTag ports SearchOneTag: linear scan of the directory.
func (p *Profile) searchOneTag(sig TagSignature) int {
	for i := 0; i < int(p.TagCount); i++ {
		if sig == p.TagNames[i] {
			return i
		}
	}
	return -1
}

// searchTag ports _cmsSearchTag: find a tag, optionally following links.
func (p *Profile) searchTag(sig TagSignature, followLinks bool) int {
	var n int
	var linked TagSignature
	// Bound the link-following to the directory size: a user-created cycle
	// (LinkTag(A,B); LinkTag(B,A)) would otherwise spin forever while holding the
	// profile mutex. Parsed profiles cannot form a cycle (links point to earlier,
	// duplicate-free entries), so this only guards programmatic misuse.
	for hops := 0; hops <= maxTableTag; hops++ {
		n = p.searchOneTag(sig)
		if n < 0 {
			return -1
		}
		if !followLinks {
			return n
		}
		linked = p.TagLinked[n]
		if linked != 0 {
			sig = linked
		}
		if linked == 0 {
			return n
		}
	}
	return -1
}

// deleteTagByPos ports _cmsDeleteTagByPos: drop the parsed/raw value at i. Under
// the Go GC there is nothing to free; the pointers are simply cleared.
func (p *Profile) deleteTagByPos(i int) {
	if i < 0 || i >= maxTableTag {
		return
	}
	if p.TagPtrs[i] != nil {
		if p.TagSaveAsRaw[i] {
			p.TagPtrs[i] = nil
			p.tagRaw[i] = nil
			p.TagSaveAsRaw[i] = false
		} else {
			p.TagPtrs[i] = nil
		}
	}
}

// newTag ports _cmsNewTag: return the slot for sig, reusing and clearing an
// existing one or allocating a fresh one, enforcing the maxTableTag cap.
func (p *Profile) newTag(sig TagSignature) (int, error) {
	i := p.searchTag(sig, false)
	if i >= 0 {
		p.deleteTagByPos(i)
		return i, nil
	}
	if p.TagCount >= maxTableTag {
		return -1, p.ContextID.signalError(ErrRange, "Too many tags (%d)", maxTableTag)
	}
	pos := int(p.TagCount)
	p.TagCount++
	return pos, nil
}

// IsTag reports whether sig is present, mirroring cmsIsTag.
func (p *Profile) IsTag(sig TagSignature) bool {
	return p.searchTag(sig, false) >= 0
}

// validatedVersion ports _validatedVersion, operating on the big-endian version
// value read from the header (byte0 = BCD major, byte1 = two BCD digits, bytes
// 2 and 3 reserved and forced to zero).
func validatedVersion(v uint32) uint32 {
	major := byte(v >> 24)
	minor := byte(v >> 16)
	if major > 0x09 {
		major = 0x09
	}
	temp1 := minor & 0xf0
	temp2 := minor & 0x0f
	if temp1 > 0x90 {
		temp1 = 0x90
	}
	if temp2 > 0x09 {
		temp2 = 0x09
	}
	minor = temp1 | temp2
	return uint32(major)<<24 | uint32(minor)<<16
}

// validDeviceClass ports validDeviceClass.
func validDeviceClass(cl ProfileClassSignature) bool {
	if cl == 0 {
		return true // older lcms defaulted to zero
	}
	switch cl {
	case SigInputClass, SigDisplayClass, SigOutputClass, SigLinkClass,
		SigAbstractClass, SigColorSpaceClass, SigNamedColorClass,
		SigColorEncodingSpaceClass, SigMultiplexIdentificationClass,
		SigMultiplexLinkClass, SigMultiplexVisualizationClass:
		return true
	default:
		return false
	}
}

// readHeader ports _cmsReadHeader: parse and validate the 128-byte header and
// the tag directory. Returns an error (never panics) on any malformed input.
func (p *Profile) readHeader() error {
	io := p.IOhandler
	var hdr [iccHeaderSize]byte
	if io.Read(hdr[:], iccHeaderSize, 1) != 1 {
		return errorf(ErrRead, "Read error reading profile header")
	}

	be32 := func(off int) uint32 { return uint32(hdr[off])<<24 | uint32(hdr[off+1])<<16 | uint32(hdr[off+2])<<8 | uint32(hdr[off+3]) }

	// Validate magic 'acsp' at offset 36.
	if be32(36) != MagicNumber {
		return p.ContextID.signalError(ErrBadSignature, "not an ICC profile, invalid signature")
	}

	p.CMM = be32(4)
	p.DeviceClass = ProfileClassSignature(be32(12))
	p.ColorSpace = ColorSpaceSignature(be32(16))
	p.PCS = ColorSpaceSignature(be32(20))
	p.RenderingIntent = be32(64)
	p.platform = PlatformSignature(be32(40))
	p.flags = be32(44)
	p.manufacturer = be32(48)
	p.model = be32(52)
	p.attributes = uint64(be32(56))<<32 | uint64(be32(60))
	p.creator = be32(80)
	p.Version = validatedVersion(be32(8))

	if p.Version > 0x5000000 {
		return p.ContextID.signalError(ErrUnknownExtension, "Unsupported profile version '0x%x'", p.Version)
	}
	if !validDeviceClass(p.DeviceClass) {
		return p.ContextID.signalError(ErrUnknownExtension, "Unsupported device class '0x%x'", uint32(p.DeviceClass))
	}

	headerSize := be32(0)
	if headerSize >= io.ReportedSize {
		headerSize = io.ReportedSize
	}

	p.Created = readDateTime(hdr[24:36])
	copy(p.ProfileID[:], hdr[84:100])

	// Tag directory.
	tagCount, ok := readUInt32(io)
	if !ok {
		return errorf(ErrRead, "Read error reading tag count")
	}
	if tagCount > maxTableTag {
		return p.ContextID.signalError(ErrRange, "Too many tags (%d)", tagCount)
	}

	corruptedTagPos := false
	p.TagCount = 0
	for i := uint32(0); i < tagCount; i++ {
		sig, ok := readUInt32(io)
		if !ok {
			return errorf(ErrRead, "Read error reading tag entry")
		}
		offset, ok := readUInt32(io)
		if !ok {
			return errorf(ErrRead, "Read error reading tag entry")
		}
		size, ok := readUInt32(io)
		if !ok {
			return errorf(ErrRead, "Read error reading tag entry")
		}

		if size == 0 || offset == 0 {
			continue
		}
		// offset+size must fall inside the file and must not overflow.
		if offset+size > headerSize || offset+size < offset {
			corruptedTagPos = true
			continue
		}

		p.TagNames[p.TagCount] = TagSignature(sig)
		p.TagOffsets[p.TagCount] = offset
		p.TagSizes[p.TagCount] = size

		// Detect links: an earlier entry with identical offset and size whose
		// descriptor is type-compatible. Empty built-in tables mean no links
		// are detected until W8 provides descriptors (documented deviation).
		for j := uint32(0); j < p.TagCount; j++ {
			if p.TagOffsets[j] == offset && p.TagSizes[j] == size {
				if compatibleTypes(p.ContextID.getTagDescriptor(p.TagNames[j]),
					p.ContextID.getTagDescriptor(TagSignature(sig))) {
					p.TagLinked[p.TagCount] = p.TagNames[j]
				}
			}
		}
		p.TagCount++
	}

	// Tags cannot be duplicated.
	for i := uint32(0); i < p.TagCount; i++ {
		for j := uint32(0); j < p.TagCount; j++ {
			if i != j && p.TagNames[i] == p.TagNames[j] {
				return p.ContextID.signalError(ErrRange, "Duplicate tag found")
			}
		}
	}

	if corruptedTagPos {
		// A diagnostic only; the reference continues past it, and so do we.
		p.ContextID.signalError(ErrCorruptionDetected, "'size' field in header seems incorrect.")
	}
	return nil
}

// writeHeader ports _cmsWriteHeader: serialize the 128-byte header and the tag
// directory. usedSpace is written into the size field.
func (p *Profile) writeHeader(usedSpace uint32) bool {
	io := p.IOhandler
	var hdr [iccHeaderSize]byte
	put32 := func(off int, v uint32) {
		hdr[off] = byte(v >> 24)
		hdr[off+1] = byte(v >> 16)
		hdr[off+2] = byte(v >> 8)
		hdr[off+3] = byte(v)
	}

	put32(0, usedSpace)
	put32(4, p.CMM)
	put32(8, p.Version)
	put32(12, uint32(p.DeviceClass))
	put32(16, uint32(p.ColorSpace))
	put32(20, uint32(p.PCS))
	encodeDateTime(hdr[24:36], p.Created)
	put32(36, MagicNumber)
	put32(40, uint32(p.platform))
	put32(44, p.flags)
	put32(48, p.manufacturer)
	put32(52, p.model)
	put32(56, uint32(p.attributes>>32))
	put32(60, uint32(p.attributes))
	put32(64, p.RenderingIntent)
	// Illuminant is always D50.
	d50 := D50XYZ()
	put32(68, uint32(doubleTo15Fixed16(d50.X)))
	put32(72, uint32(doubleTo15Fixed16(d50.Y)))
	put32(76, uint32(doubleTo15Fixed16(d50.Z)))
	put32(80, p.creator)
	copy(hdr[84:100], p.ProfileID[:])
	// reserved[28] left zero.

	if !io.Write(iccHeaderSize, hdr[:]) {
		return false
	}

	// Tag directory: count of non-placeholder tags, then the entries.
	count := uint32(0)
	for i := uint32(0); i < p.TagCount; i++ {
		if p.TagNames[i] != 0 {
			count++
		}
	}
	if !writeUInt32(io, count) {
		return false
	}
	for i := uint32(0); i < p.TagCount; i++ {
		if p.TagNames[i] == 0 {
			continue
		}
		var e [tagEntrySize]byte
		be := func(off int, v uint32) {
			e[off] = byte(v >> 24)
			e[off+1] = byte(v >> 16)
			e[off+2] = byte(v >> 8)
			e[off+3] = byte(v)
		}
		be(0, uint32(p.TagNames[i]))
		be(4, p.TagOffsets[i])
		be(8, p.TagSizes[i])
		if !io.Write(tagEntrySize, e[:]) {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------- Open functions

// openFromIOhandler ports cmsOpenProfileFromIOhandlerTHR: attach io and parse
// the header.
func (ctx *Context) openFromIOhandler(io *IOHandler) (*Profile, error) {
	p := ctx.CreateProfilePlaceholder()
	p.IOhandler = io
	if err := p.readHeader(); err != nil {
		return nil, err
	}
	return p, nil
}

// OpenProfileFromFile opens an ICC profile from disk, mirroring
// cmsOpenProfileFromFileTHR. Only read access ("r") parses a header; write
// access ("w") returns a placeholder bound to the file.
func (ctx *Context) OpenProfileFromFile(fileName, access string) (*Profile, error) {
	io, err := ctx.OpenIOhandlerFromFile(fileName, access)
	if err != nil {
		return nil, err
	}
	p := ctx.CreateProfilePlaceholder()
	p.IOhandler = io
	if len(access) > 0 && (access[0] == 'w' || access[0] == 'W') {
		p.IsWrite = true
		return p, nil
	}
	if err := p.readHeader(); err != nil {
		io.Close()
		return nil, err
	}
	return p, nil
}

// OpenProfileFromFile opens an ICC profile from disk on the default context.
func OpenProfileFromFile(fileName, access string) (*Profile, error) {
	return defaultContext.OpenProfileFromFile(fileName, access)
}

// OpenProfileFromMem opens an ICC profile from a memory block, mirroring
// cmsOpenProfileFromMemTHR. The bytes are copied, so the caller may reuse buf.
func (ctx *Context) OpenProfileFromMem(buf []byte) (*Profile, error) {
	io, err := ctx.OpenIOhandlerFromMem(buf, false)
	if err != nil {
		return nil, err
	}
	return ctx.openFromIOhandler(io)
}

// OpenProfileFromMem opens an ICC profile from a memory block on the default
// context.
func OpenProfileFromMem(buf []byte) (*Profile, error) {
	return defaultContext.OpenProfileFromMem(buf)
}

// ------------------------------------------------------------- Save functions

// saveTags ports SaveTags: write each tag body. Untouched tags are blind-copied
// from fileOrig; raw tags are copied verbatim; cooked tags dispatch to their
// type handler (which requires W8's tables). Returns an error on failure.
func (p *Profile) saveTags(fileOrig *profileData) error {
	io := p.IOhandler
	version := p.GetProfileVersion()

	for i := uint32(0); i < p.TagCount; i++ {
		if p.TagNames[i] == 0 {
			continue
		}
		if p.TagLinked[i] != 0 {
			continue // linked tags are not written
		}

		begin := io.UsedSpace
		p.TagOffsets[i] = begin

		if p.TagPtrs[i] == nil {
			// Untouched tag copied from a disk/memory-based original.
			if fileOrig != nil && p.TagOffsets[i] != 0 && fileOrig.IOhandler != nil {
				tagSize := fileOrig.TagSizes[i]
				tagOffset := fileOrig.TagOffsets[i]
				if !fileOrig.IOhandler.Seek(tagOffset) {
					return errorf(ErrSeek, "Seek error copying tag")
				}
				mem := make([]byte, tagSize)
				if fileOrig.IOhandler.Read(mem, tagSize, 1) != 1 {
					return errorf(ErrRead, "Read error copying tag")
				}
				if !io.Write(tagSize, mem) {
					return errorf(ErrWrite, "Write error copying tag")
				}
				p.TagSizes[i] = io.UsedSpace - begin
				if !writeAlignment(io) {
					return errorf(ErrWrite, "Alignment write error")
				}
			}
			continue
		}

		if p.TagSaveAsRaw[i] {
			if !io.Write(p.TagSizes[i], p.tagRaw[i]) {
				return errorf(ErrWrite, "Write error writing raw tag")
			}
		} else {
			desc := p.ContextID.getTagDescriptor(p.TagNames[i])
			if desc == nil {
				continue // unsupported, ignore
			}
			var typ TagTypeSignature
			if desc.DecideType != nil {
				typ = desc.DecideType(version, p.TagPtrs[i])
			} else if len(desc.SupportedTypes) > 0 {
				typ = desc.SupportedTypes[0]
			}
			handler := p.ContextID.getTagTypeHandler(typ)
			if handler == nil {
				p.ContextID.signalError(ErrInternal, "(Internal) no handler for tag %x", uint32(p.TagNames[i]))
				continue
			}
			if !writeTypeBase(io, handler.Signature) {
				return errorf(ErrWrite, "Write error writing type base")
			}
			local := *handler
			local.ContextID = p.ContextID
			local.ICCVersion = p.Version
			if local.Write == nil {
				return p.ContextID.signalError(ErrWrite, "Couldn't write type '%s'", handler.Signature)
			}
			if err := local.Write(&local, io, p.TagPtrs[i], desc.ElemCount); err != nil {
				return p.ContextID.signalError(ErrWrite, "Couldn't write type '%s': %v", handler.Signature, err)
			}
		}

		p.TagSizes[i] = io.UsedSpace - begin
		if !writeAlignment(io) {
			return errorf(ErrWrite, "Alignment write error")
		}
	}
	return nil
}

// setLinks ports SetLinks: fill offset and size for every linked tag from its
// target.
func (p *Profile) setLinks() {
	for i := uint32(0); i < p.TagCount; i++ {
		lnk := p.TagLinked[i]
		if lnk != 0 {
			j := p.searchTag(lnk, false)
			if j >= 0 {
				p.TagOffsets[i] = p.TagOffsets[j]
				p.TagSizes[i] = p.TagSizes[j]
			}
		}
	}
}

// saveToIOhandler ports cmsSaveProfileToIOhandler: the two-pass serializer. Pass
// one computes offsets against a NULL handler; pass two writes to io. When io is
// nil only the size is computed. Returns the number of bytes used, or 0 on
// error.
//
// The caller must hold p.mu: this swaps p.IOhandler and rewrites the tag offset
// tables in place before restoring them, so a concurrent tag read would
// otherwise observe the transient NULL handler and read garbage. The reference
// takes Icc->UsrMutex for the whole save for the same reason.
func (p *Profile) saveToIOhandler(io *IOHandler) (uint32, error) {
	keep := p.profileData

	prevIO := p.ContextID.OpenIOhandlerFromNULL()
	p.IOhandler = prevIO

	restore := func() { p.profileData = keep }

	// Pass 1: compute offsets.
	if !p.writeHeader(0) {
		restore()
		return 0, errorf(ErrWrite, "Header write error")
	}
	if err := p.saveTags(&keep); err != nil {
		restore()
		return 0, err
	}
	usedSpace := prevIO.UsedSpace

	// Pass 2: write to the real handler.
	if io != nil {
		p.IOhandler = io
		p.setLinks()
		if !p.writeHeader(usedSpace) {
			restore()
			return 0, errorf(ErrWrite, "Header write error")
		}
		if err := p.saveTags(&keep); err != nil {
			restore()
			return 0, err
		}
	}

	restore()
	return usedSpace, nil
}

// SaveProfileToMem serializes the profile and returns the bytes, mirroring
// cmsSaveProfileToMem. It computes the exact size in a first pass, then writes.
func (p *Profile) SaveProfileToMem() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.saveToMemLocked()
}

// saveToMemLocked is SaveProfileToMem with p.mu already held (used by
// ComputeProfileID, which serializes while holding the lock).
func (p *Profile) saveToMemLocked() ([]byte, error) {
	needed, err := p.saveToIOhandler(nil)
	if err != nil {
		return nil, err
	}
	if needed == 0 {
		return nil, errorf(ErrWrite, "profile serialization produced no data")
	}
	buf := make([]byte, needed)
	io, err := p.ContextID.OpenIOhandlerFromMem(buf, true)
	if err != nil {
		return nil, err
	}
	if _, err := p.saveToIOhandler(io); err != nil {
		return nil, err
	}
	return buf, nil
}

// SaveProfileToFile serializes the profile to disk, mirroring
// cmsSaveProfileToFile.
func (p *Profile) SaveProfileToFile(fileName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.saveToFileLocked(fileName)
}

// saveToFileLocked is SaveProfileToFile with p.mu already held.
func (p *Profile) saveToFileLocked(fileName string) error {
	io, err := p.ContextID.OpenIOhandlerFromFile(fileName, "w")
	if err != nil {
		return err
	}
	n, saveErr := p.saveToIOhandler(io)
	closeErr := io.Close()
	if saveErr != nil || n == 0 || closeErr != nil {
		// Best-effort cleanup of a partial file, mirroring the C remove().
		_ = removeFile(fileName)
		if saveErr != nil {
			return saveErr
		}
		if closeErr != nil {
			return closeErr
		}
		return errorf(ErrWrite, "profile serialization produced no data")
	}
	return nil
}

// CloseProfile releases the profile, saving it first if it was opened for
// writing, mirroring cmsCloseProfile.
func (p *Profile) CloseProfile() error {
	if p == nil {
		return errorf(ErrNull, "nil profile")
	}
	var rc error
	if p.IsWrite {
		p.IsWrite = false
		if p.IOhandler == nil {
			rc = errorf(ErrWrite, "profile opened for writing has no IO handler")
		} else {
			rc = p.SaveProfileToFile(p.IOhandler.PhysicalFile)
		}
	}
	if p.IOhandler != nil {
		if err := p.IOhandler.Close(); err != nil && rc == nil {
			rc = err
		}
		// Drop the handler so a second CloseProfile is a no-op rather than
		// re-closing an already-closed file.
		p.IOhandler = nil
	}
	return rc
}

// ------------------------------------------------------------- Tag read/write

// ReadTag ports cmsReadTag: parse (and cache) a tag's cooked value, following
// links. Until W8 populates the type tables this returns an error for every tag
// that is not already cached. A missing tag returns (nil, nil), matching the C
// NULL-without-error contract for "tag not present".
func (p *Profile) ReadTag(sig TagSignature) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.readTagLocked(sig)
}

func (p *Profile) readTagLocked(sig TagSignature) (any, error) {
	avoidCheck := p.ContextID.avoidTypeCheckOnTags()

	n := p.searchTag(sig, true)
	if n < 0 {
		return nil, nil // not found
	}

	// Already in memory?
	if p.TagPtrs[n] != nil {
		if p.TagTypeHandlers[n] == nil {
			p.freeOneTag(n)
			return nil, errorf(ErrCorruptionDetected, "Corrupted tag '%s'", sig)
		}
		baseType := p.TagTypeHandlers[n].Signature
		if baseType == 0 {
			p.freeOneTag(n)
			return nil, errorf(ErrCorruptionDetected, "Corrupted tag '%s'", sig)
		}
		if !avoidCheck {
			desc := p.ContextID.getTagDescriptor(sig)
			if desc == nil || !isTypeSupported(desc, baseType) {
				p.freeOneTag(n)
				return nil, errorf(ErrCorruptionDetected, "Corrupted tag '%s'", sig)
			}
		}
		if p.TagSaveAsRaw[n] {
			p.freeOneTag(n)
			return nil, errorf(ErrNotSuitable, "cannot read raw tag '%s' as cooked", sig)
		}
		return p.TagPtrs[n], nil
	}

	offset := p.TagOffsets[n]
	tagSize := p.TagSizes[n]
	if tagSize < 8 {
		p.freeOneTag(n)
		return nil, errorf(ErrCorruptionDetected, "Corrupted tag '%s'", sig)
	}

	io := p.IOhandler
	if io == nil {
		p.freeOneTag(n)
		return nil, p.ContextID.signalError(ErrCorruptionDetected, "Corrupted built-in profile.")
	}
	if !io.Seek(offset) {
		p.freeOneTag(n)
		return nil, errorf(ErrSeek, "Seek error reading tag '%s'", sig)
	}

	var desc *TagDescriptor
	if !avoidCheck {
		desc = p.ContextID.getTagDescriptor(sig)
		if desc == nil {
			p.freeOneTag(n)
			return nil, p.ContextID.signalError(ErrUnknownExtension, "Unknown tag type '%s' found.", sig)
		}
	}

	baseType := readTypeBase(io)
	if baseType == 0 {
		p.freeOneTag(n)
		return nil, errorf(ErrRead, "Read error reading tag '%s'", sig)
	}
	if !avoidCheck {
		if !isTypeSupported(desc, baseType) {
			p.freeOneTag(n)
			return nil, errorf(ErrUnknownExtension, "Unsupported type for tag '%s'", sig)
		}
	}

	tagSize -= 8 // type base already consumed

	handler := p.ContextID.getTagTypeHandler(baseType)
	if handler == nil || handler.Read == nil {
		p.freeOneTag(n)
		return nil, p.ContextID.signalError(ErrUnknownExtension, "No handler for type '%s' of tag '%s'", baseType, sig)
	}

	local := *handler
	local.ContextID = p.ContextID
	local.ICCVersion = p.Version
	p.TagTypeHandlers[n] = handler

	value, elemCount, err := local.Read(&local, io, tagSize)
	if err != nil || value == nil {
		p.freeOneTag(n)
		if err != nil {
			return nil, err
		}
		return nil, p.ContextID.signalError(ErrCorruptionDetected, "Corrupted tag '%s'", sig)
	}
	p.TagPtrs[n] = value

	if !avoidCheck && desc != nil && elemCount < desc.ElemCount {
		p.freeOneTag(n)
		return nil, p.ContextID.signalError(ErrCorruptionDetected,
			"'%s' Inconsistent number of items: expected %d, got %d", sig, desc.ElemCount, elemCount)
	}
	return p.TagPtrs[n], nil
}

// freeOneTag ports freeOneTag: clear the cached value at i.
func (p *Profile) freeOneTag(i int) {
	if i < 0 || i >= maxTableTag {
		return
	}
	p.TagPtrs[i] = nil
	p.tagRaw[i] = nil
}

// GetTagTrueType ports _cmsGetTagTrueType: the true stored type of a tag, or 0.
func (p *Profile) GetTagTrueType(sig TagSignature) TagTypeSignature {
	n := p.searchTag(sig, true)
	if n < 0 {
		return 0
	}
	h := p.TagTypeHandlers[n]
	if h == nil {
		return 0
	}
	return h.Signature
}

// WriteTag ports cmsWriteTag: stage a cooked value for serialization, or delete
// the tag when value is nil. Cooked writes require W8's type tables.
func (p *Profile) WriteTag(sig TagSignature, value any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if value == nil {
		i := p.searchTag(sig, false)
		if i >= 0 {
			p.deleteTagByPos(i)
			p.TagNames[i] = 0
			return nil
		}
		return errorf(ErrUnknownExtension, "tag '%s' not found", sig)
	}

	i, err := p.newTag(sig)
	if err != nil {
		return err
	}
	if p.TagSaveAsRaw[i] {
		return p.ContextID.signalError(ErrAlreadyDefined, "Tag '%x' was already saved as RAW", uint32(sig))
	}
	p.TagLinked[i] = 0

	desc := p.ContextID.getTagDescriptor(sig)
	if desc == nil {
		return p.ContextID.signalError(ErrUnknownExtension, "Unsupported tag '%x'", uint32(sig))
	}

	version := p.GetProfileVersion()
	var typ TagTypeSignature
	if desc.DecideType != nil {
		typ = desc.DecideType(version, value)
	} else if len(desc.SupportedTypes) > 0 {
		typ = desc.SupportedTypes[0]
	}
	if !isTypeSupported(desc, typ) {
		return p.ContextID.signalError(ErrUnknownExtension, "Unsupported type '%s' for tag '%s'", typ, sig)
	}
	handler := p.ContextID.getTagTypeHandler(typ)
	if handler == nil {
		return p.ContextID.signalError(ErrUnknownExtension, "Unsupported type '%s' for tag '%s'", typ, sig)
	}

	p.TagTypeHandlers[i] = handler
	p.TagNames[i] = sig
	p.TagSizes[i] = 0
	p.TagOffsets[i] = 0

	local := *handler
	local.ContextID = p.ContextID
	local.ICCVersion = p.Version
	var dup any = value
	if local.Dup != nil {
		dup, err = local.Dup(&local, value, desc.ElemCount)
		if err != nil {
			return err
		}
	}
	if dup == nil {
		return p.ContextID.signalError(ErrCorruptionDetected, "Malformed struct for tag '%s'", sig)
	}
	p.TagPtrs[i] = dup
	return nil
}

// ReadRawTag ports cmsReadRawTag: copy up to len(dst) raw bytes of a tag into
// dst and return the number available. When dst is nil it returns the tag's
// on-disk size without copying.
func (p *Profile) ReadRawTag(sig TagSignature, dst []byte) (uint32, error) {
	p.mu.Lock()

	i := p.searchTag(sig, true)
	if i < 0 {
		p.mu.Unlock()
		return 0, nil
	}

	if p.TagPtrs[i] == nil {
		offset := p.TagOffsets[i]
		tagSize := p.TagSizes[i]
		if dst != nil {
			if uint32(len(dst)) < tagSize {
				tagSize = uint32(len(dst))
			}
			// A placeholder/built-in profile has no IOhandler. A directory entry
			// with a nil cached value and no handler can arise (e.g. a WriteTag
			// whose Dup failed left the name set but the pointer nil); reading it
			// back must error, not dereference a nil handler. cmsReadTag guards
			// the same state.
			if p.IOhandler == nil {
				p.mu.Unlock()
				return 0, errorf(ErrRead, "Corrupted profile: tag has no data and no IO handler")
			}
			if !p.IOhandler.Seek(offset) {
				p.mu.Unlock()
				return 0, errorf(ErrSeek, "Seek error reading raw tag")
			}
			if p.IOhandler.Read(dst, 1, tagSize) != tagSize {
				p.mu.Unlock()
				return 0, errorf(ErrRead, "Read error reading raw tag")
			}
			p.mu.Unlock()
			return tagSize, nil
		}
		p.mu.Unlock()
		return p.TagSizes[i], nil
	}

	if p.TagSaveAsRaw[i] {
		tagSize := p.TagSizes[i]
		if dst != nil {
			if uint32(len(dst)) < tagSize {
				tagSize = uint32(len(dst))
			}
			copy(dst[:tagSize], p.tagRaw[i][:tagSize])
			p.mu.Unlock()
			return tagSize, nil
		}
		p.mu.Unlock()
		return p.TagSizes[i], nil
	}

	// Already cooked: serialize it back to raw through a memory handler.
	p.mu.Unlock()
	obj, err := p.ReadTag(sig)
	if err != nil {
		return 0, err
	}
	if obj == nil {
		return 0, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	var memIO *IOHandler
	if dst == nil {
		memIO = p.ContextID.OpenIOhandlerFromNULL()
	} else {
		memIO, err = p.ContextID.OpenIOhandlerFromMem(dst, true)
		if err != nil {
			return 0, err
		}
	}

	handler := p.TagTypeHandlers[i]
	desc := p.ContextID.getTagDescriptor(sig)
	if desc == nil || handler == nil || handler.Write == nil {
		memIO.Close()
		return 0, errorf(ErrUnknownExtension, "cannot serialize tag '%s'", sig)
	}
	local := *handler
	local.ContextID = p.ContextID
	local.ICCVersion = p.Version
	if !writeTypeBase(memIO, handler.Signature) {
		memIO.Close()
		return 0, errorf(ErrWrite, "Write error serializing raw tag")
	}
	if err := local.Write(&local, memIO, obj, desc.ElemCount); err != nil {
		memIO.Close()
		return 0, err
	}
	rc := memIO.Tell()
	memIO.Close()
	return rc, nil
}

// WriteRawTag ports cmsWriteRawTag: stage raw bytes for a tag verbatim.
func (p *Profile) WriteRawTag(sig TagSignature, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	i, err := p.newTag(sig)
	if err != nil {
		return err
	}
	p.TagSaveAsRaw[i] = true
	p.TagNames[i] = sig
	p.TagLinked[i] = 0
	p.TagTypeHandlers[i] = nil

	cp := make([]byte, len(data))
	copy(cp, data)
	p.tagRaw[i] = cp
	p.TagPtrs[i] = cp // non-nil marks the slot as populated
	p.TagSizes[i] = uint32(len(data))
	return nil
}

// LinkTag ports cmsLinkTag: collapse sig onto dest so both share one block.
func (p *Profile) LinkTag(sig, dest TagSignature) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	i, err := p.newTag(sig)
	if err != nil {
		return err
	}
	p.TagSaveAsRaw[i] = false
	p.TagNames[i] = sig
	p.TagLinked[i] = dest
	p.TagPtrs[i] = nil
	p.tagRaw[i] = nil
	p.TagSizes[i] = 0
	p.TagOffsets[i] = 0
	return nil
}

// TagLinkedTo ports cmsTagLinkedTo: the tag sig is linked to, or 0.
func (p *Profile) TagLinkedTo(sig TagSignature) TagSignature {
	i := p.searchTag(sig, false)
	if i < 0 {
		return 0
	}
	return p.TagLinked[i]
}

// ------------------------------------------------------------- MD5 profile ID

// ComputeProfileID ports cmsMD5computeID: zero the flags, rendering intent and
// profile ID, serialize the profile, MD5 the bytes, and store the digest as the
// profile ID (per ICC 4.4 section 7.2.18). The header state is restored around
// the computation.
func (p *Profile) ComputeProfileID() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	keep := p.profileData

	p.flags = 0
	p.RenderingIntent = 0
	p.ProfileID = profileID{}

	buf, err := p.saveToMemLocked()
	if err != nil {
		p.profileData = keep
		return err
	}

	md5 := md5Alloc()
	md5.add(buf)

	// Restore the header, then store the freshly computed ID.
	p.profileData = keep
	p.ProfileID = md5.finish()
	return nil
}

// GetProfileID returns a copy of the 16-byte profile ID, mirroring
// cmsGetHeaderProfileID.
func (p *Profile) GetProfileID() [16]byte {
	var id [16]byte
	copy(id[:], p.ProfileID[:])
	return id
}

// SetProfileID sets the 16-byte profile ID, mirroring cmsSetHeaderProfileID.
func (p *Profile) SetProfileID(id [16]byte) {
	copy(p.ProfileID[:], id[:])
}
