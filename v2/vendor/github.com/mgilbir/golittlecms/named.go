// Port of src/cmsnamed.c: the multi-localized unicode (MLU) machinery, named
// color lists, profile sequence descriptions, and dictionaries.
//
// These are pure data containers; the tag-type handlers that serialize them to
// disk live in types_base.go (MLU, NamedColor2, ColorantTable, ProfileSequence*)
// and, for the dictionary type, in W8b's types_mpe.go. W8b's handlers and the
// higher phases consume the types defined here, so the field set is chosen to
// mirror the reference structs closely.
//
// Wide-string representation
// --------------------------
// The reference stores strings in a memory pool as wchar_t, whose width is
// platform-dependent (2 bytes on Windows, 4 on Linux). The oracle used for
// differential testing is built on Linux, where wchar_t is 4 bytes (UTF-32), so
// this port emulates a 4-byte wchar_t: the pool holds Go runes (code points) and
// all offset/length arithmetic is done in "wchar bytes" (4 per rune), exactly
// as cmsnamed.c/cmstypes.c compute it. This makes the 'mluc' serialization
// byte-for-byte identical to the reference on that platform.

package lcms2

import "strings"

// wcharSize emulates sizeof(wchar_t) on the reference's Linux build (UTF-32).
// All MLU pool offsets and lengths are measured in these units.
const wcharSize = 4

// ---------------------------------------------------------------------------
// Multi-localized unicode (MLU)
// ---------------------------------------------------------------------------

// mluEntry mirrors _cmsMLUentry: one localized string's directory record. Len
// and StrW are measured in wchar bytes (4 per code point), matching the
// reference on a 4-byte-wchar_t platform.
type mluEntry struct {
	Language uint16
	Country  uint16
	StrW     uint32 // offset into the pool, in wchar bytes
	Len      uint32 // length, in wchar bytes
}

// MLU mirrors cmsMLU (struct _cms_MLU_struct): a set of localized strings keyed
// by language/country, stored in a shared pool of code points.
type MLU struct {
	ctx *Context

	allocatedEntries uint32
	usedEntries      uint32
	entries          []mluEntry

	poolSize uint32 // capacity of the pool, in wchar bytes
	poolUsed uint32 // used size of the pool, in wchar bytes
	pool     []rune // code points; len(pool) == poolUsed/wcharSize
}

// mluEntrySize is sizeof(_cmsMLUentry) in the reference: two cmsUInt16Number
// followed by two cmsUInt32Number, which needs no tail padding at 4-byte
// alignment. mluEntry above has the same layout, so charging the allocation
// limit in these units refuses at exactly the nItems the reference refuses at.
const mluEntrySize = 12

// NewMLU ports cmsMLUalloc on a context: an empty MLU pre-sized for nItems
// entries (a non-positive request becomes 2, matching the reference). It
// returns nil where cmsMLUalloc would return NULL, i.e. when the entry
// directory would exceed the memory manager's allocation limit; callers passing
// a computed nItems must check for it.
func (ctx *Context) NewMLU(nItems uint32) *MLU {
	if nItems == 0 {
		nItems = 2
	}
	// cmsMLUalloc's _cmsCalloc(nItems, sizeof(_cmsMLUentry)) (cmsnamed.c:47)
	// hands back NULL past 512 MB and the whole allocation then fails. The
	// zero-total arm of allocSizeOK is unreachable here: C's "nItems <= 0 means
	// 2" floor above has already run, exactly as in the reference.
	if !allocSizeOK(nItems, mluEntrySize) {
		_ = ctx.signalError(ErrRange, "cmsMLUalloc: %d entries exceeds the %d byte allocation limit", nItems, maxMemoryForAlloc)
		return nil
	}
	return &MLU{
		ctx:              ctx,
		allocatedEntries: nItems,
		entries:          make([]mluEntry, nItems),
	}
}

// NewMLU allocates an MLU on the default context.
func NewMLU(nItems uint32) *MLU { return defaultContext.NewMLU(nItems) }

// growMLUtable mirrors GrowMLUtable: double the entry directory. Go slices make
// the overflow bookkeeping unnecessary, but the doubling behaviour is kept.
func (mlu *MLU) growMLUtable() bool {
	n := mlu.allocatedEntries * 2
	if mlu.allocatedEntries == 0 {
		// Dup of an empty MLU leaves a zero-capacity table; doubling zero never
		// grows, so the next Set would index entries[0] out of range. Start at
		// two, matching cmsMLUalloc's floor.
		n = 2
	}
	if n < mlu.allocatedEntries {
		return false
	}
	// GrowMLUtable's _cmsRealloc is capped at 512 MB too (cmserr.c:176), so a
	// table already close to the ceiling fails to double rather than growing
	// past it. Without this the NewMLU guard could be walked around by Setting
	// entries until the doubling crossed the limit.
	if !allocSizeOK(n, mluEntrySize) {
		return false
	}
	grown := make([]mluEntry, n)
	copy(grown, mlu.entries)
	mlu.entries = grown
	mlu.allocatedEntries = n
	return true
}

// searchMLUEntry ports SearchMLUEntry: index of the (language,country) pair, or
// -1. Country is compared first, exactly as the reference.
func (mlu *MLU) searchMLUEntry(lang, cntry uint16) int {
	for i := uint32(0); i < mlu.usedEntries; i++ {
		if mlu.entries[i].Country == cntry && mlu.entries[i].Language == lang {
			return int(i)
		}
	}
	return -1
}

// addMLUBlock ports AddMLUBlock: append a code-point block for one
// language/country. Only one entry per pair is allowed. block is stored
// verbatim; size must equal len(block)*wcharSize.
func (mlu *MLU) addMLUBlock(size uint32, block []rune, lang, cntry uint16) bool {
	if mlu.usedEntries >= mlu.allocatedEntries {
		if !mlu.growMLUtable() {
			return false
		}
	}
	if mlu.searchMLUEntry(lang, cntry) >= 0 {
		return false // only one is allowed
	}

	offset := mlu.poolUsed
	mlu.pool = append(mlu.pool, block...)
	mlu.poolUsed += size
	if mlu.poolUsed > mlu.poolSize {
		mlu.poolSize = mlu.poolUsed
	}

	mlu.entries[mlu.usedEntries] = mluEntry{
		Language: lang,
		Country:  cntry,
		StrW:     offset,
		Len:      size,
	}
	mlu.usedEntries++
	return true
}

// strTo16 ports strTo16: pack the first two bytes of a 2-3 char code into a
// big-endian uint16. Short strings are zero-padded.
func strTo16(s string) uint16 {
	var b0, b1 byte
	if len(s) > 0 {
		b0 = s[0]
	}
	if len(s) > 1 {
		b1 = s[1]
	}
	return uint16(b0)<<8 | uint16(b1)
}

// strFrom16 ports strFrom16: unpack a uint16 into a two-byte code string.
func strFrom16(n uint16) [3]byte {
	return [3]byte{byte(n >> 8), byte(n), 0}
}

// SetASCII ports cmsMLUsetASCII: add an ASCII string for a language/country.
// An empty string is stored as a single NUL code point (no terminator is added
// for non-empty strings, per ICC1v43 clause 4.1).
func (mlu *MLU) SetASCII(lang, cntry, s string) bool {
	l, c := strTo16(lang), strTo16(cntry)
	// The reference measures the string with strlen, so it stops at the first
	// embedded NUL; match that rather than storing code points past it.
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	if len(s) == 0 {
		return mlu.addMLUBlock(wcharSize, []rune{0}, l, c)
	}
	block := make([]rune, len(s))
	for i := 0; i < len(s); i++ {
		// C assigns each byte through a signed char (Linux wchar_t), so bytes
		// >= 0x80 sign-extend to a negative code point and serialize to 0xFFxx.
		block[i] = rune(int8(s[i]))
	}
	return mlu.addMLUBlock(uint32(len(block))*wcharSize, block, l, c)
}

// SetWide ports cmsMLUsetWide: add a wide (code-point) string. An empty string
// is stored as a single NUL code point.
func (mlu *MLU) SetWide(lang, cntry string, wide []rune) bool {
	if wide == nil {
		return false
	}
	l, c := strTo16(lang), strTo16(cntry)
	n := mywcslen(wide)
	if n == 0 {
		return mlu.addMLUBlock(wcharSize, []rune{0}, l, c)
	}
	return mlu.addMLUBlock(n*wcharSize, wide[:n], l, c)
}

// SetWideString is a convenience wrapper of SetWide taking a Go string.
func (mlu *MLU) SetWideString(lang, cntry, s string) bool {
	return mlu.SetWide(lang, cntry, []rune(s))
}

// SetUTF8 ports cmsMLUsetUTF8: add a UTF-8 string.
func (mlu *MLU) SetUTF8(lang, cntry, s string) bool {
	l, c := strTo16(lang), strTo16(cntry)
	// As in SetASCII, the reference's strlen-based length stops at the first NUL.
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	if len(s) == 0 {
		return mlu.addMLUBlock(wcharSize, []rune{0}, l, c)
	}
	block := decodeUTF8([]byte(s))
	return mlu.addMLUBlock(uint32(len(block))*wcharSize, block, l, c)
}

// mywcslen ports mywcslen: length up to (not including) a NUL code point.
func mywcslen(s []rune) uint32 {
	for i, r := range s {
		if r == 0 {
			return uint32(i)
		}
	}
	return uint32(len(s))
}

// Dup ports cmsMLUdup: an independent copy. A nil receiver duplicates to nil.
func (mlu *MLU) Dup() *MLU {
	if mlu == nil {
		return nil
	}
	n := &MLU{
		ctx:              mlu.ctx,
		allocatedEntries: mlu.usedEntries,
		usedEntries:      mlu.usedEntries,
		entries:          make([]mluEntry, mlu.usedEntries),
	}
	if n.allocatedEntries == 0 {
		n.entries = nil
	}
	copy(n.entries, mlu.entries[:mlu.usedEntries])
	if mlu.poolUsed != 0 {
		n.pool = make([]rune, mlu.poolUsed/wcharSize)
		copy(n.pool, mlu.pool[:mlu.poolUsed/wcharSize])
	}
	n.poolSize = mlu.poolUsed
	n.poolUsed = mlu.poolUsed
	return n
}

// mluGetWide ports _cmsMLUgetWide: find the best-matching entry. An exact
// language+country match wins; otherwise the first entry with the requested
// language; otherwise the first entry. Returns the code points, their length in
// wchar bytes, the actually-used language/country, and ok.
func (mlu *MLU) mluGetWide(lang, cntry uint16) (runes []rune, lenBytes uint32, usedLang, usedCntry uint16, ok bool) {
	if mlu == nil || mlu.allocatedEntries == 0 {
		return nil, 0, 0, 0, false
	}

	best := -1
	for i := uint32(0); i < mlu.usedEntries; i++ {
		v := &mlu.entries[i]
		if v.Language == lang {
			if best == -1 {
				best = int(i)
			}
			if v.Country == cntry {
				return mlu.poolSlice(v), v.Len, v.Language, v.Country, true
			}
		}
	}

	if best == -1 {
		best = 0
	}
	if best >= int(mlu.usedEntries) {
		// No entries at all. The reference indexes the first (never allocated)
		// slot, whose pool pointer is NULL, and its callers treat that as
		// failure — so report failure rather than a zero-length success.
		return nil, 0, 0, 0, false
	}
	v := &mlu.entries[best]
	if v.StrW+v.Len > mlu.poolSize {
		return nil, 0, 0, 0, false
	}
	return mlu.poolSlice(v), v.Len, v.Language, v.Country, true
}

// poolSlice returns the code points referenced by entry e, clamped to the pool
// so malformed offsets can never index out of range.
func (mlu *MLU) poolSlice(e *mluEntry) []rune {
	start := e.StrW / wcharSize
	n := e.Len / wcharSize
	if start > uint32(len(mlu.pool)) {
		return nil
	}
	end := start + n
	if end > uint32(len(mlu.pool)) {
		end = uint32(len(mlu.pool))
	}
	return mlu.pool[start:end]
}

// mluGetASCII ports cmsMLUgetASCII: fill buffer with an ASCII rendering (code
// points >= 0xff become '?'), NUL-terminated. A nil buffer returns the required
// size (characters + terminator). len(buffer) is the BufferSize.
func (mlu *MLU) mluGetASCII(lang, cntry uint16, buffer []byte) uint32 {
	if mlu == nil {
		return 0
	}
	wide, strLen, _, _, ok := mlu.mluGetWide(lang, cntry)
	if !ok {
		return 0
	}
	asciiLen := strLen / wcharSize
	if buffer == nil {
		return asciiLen + 1
	}
	bufSize := uint32(len(buffer))
	if bufSize == 0 {
		return 0
	}
	if bufSize < asciiLen+1 {
		asciiLen = bufSize - 1
	}
	for i := uint32(0); i < asciiLen; i++ {
		var wc rune
		if i < uint32(len(wide)) {
			wc = wide[i]
		}
		if wc < 0xff {
			// Matches C's `if (wc < 0xff) *ascii = (char)wc`: a negative
			// (sign-extended) code point is < 0xff and its low byte is emitted,
			// so a byte round-trips through SetASCII/GetASCII.
			buffer[i] = byte(wc)
		} else {
			buffer[i] = '?'
		}
	}
	buffer[asciiLen] = 0
	return asciiLen + 1
}

// mluGetWideBuf ports cmsMLUgetWide (the public buffer variant): copy the code
// points into buffer (capacity len(buffer) runes), NUL-terminated. Returns the
// number of wchar bytes written including the terminator, or, for a nil buffer,
// the required size in wchar bytes.
func (mlu *MLU) mluGetWideBuf(lang, cntry uint16, buffer []rune) uint32 {
	if mlu == nil {
		return 0
	}
	wide, strLen, _, _, ok := mlu.mluGetWide(lang, cntry)
	if !ok {
		return 0
	}
	if buffer == nil {
		return strLen + wcharSize
	}
	bufSize := uint32(len(buffer)) * wcharSize
	if bufSize < wcharSize {
		return 0
	}
	if bufSize < strLen+wcharSize {
		strLen = bufSize - wcharSize
	}
	nRunes := strLen / wcharSize
	for i := uint32(0); i < nRunes; i++ {
		if i < uint32(len(wide)) {
			buffer[i] = wide[i]
		} else {
			buffer[i] = 0
		}
	}
	buffer[nRunes] = 0
	return strLen + wcharSize
}

// mluGetUTF8 ports cmsMLUgetUTF8.
func (mlu *MLU) mluGetUTF8(lang, cntry uint16, buffer []byte) uint32 {
	if mlu == nil {
		return 0
	}
	wide, strLen, _, _, ok := mlu.mluGetWide(lang, cntry)
	if !ok {
		return 0
	}
	var bufSize uint32
	if buffer != nil {
		bufSize = uint32(len(buffer))
	}
	utf8len := encodeUTF8(nil, wide, strLen/wcharSize, bufSize)
	if buffer == nil {
		return utf8len + 1
	}
	if bufSize == 0 {
		return 0
	}
	if bufSize < utf8len+1 {
		utf8len = bufSize - 1
	}
	encodeUTF8(buffer, wide, strLen/wcharSize, bufSize)
	buffer[utf8len] = 0
	return utf8len + 1
}

// GetASCII ports cmsMLUgetASCII for callers wanting a Go string (terminator
// stripped). An absent translation yields "".
func (mlu *MLU) GetASCII(lang, cntry string) string {
	l, c := strTo16(lang), strTo16(cntry)
	size := mlu.mluGetASCII(l, c, nil)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	got := mlu.mluGetASCII(l, c, buf)
	if got == 0 {
		return ""
	}
	return string(buf[:got-1])
}

// GetWide ports cmsMLUgetWide for callers wanting the code points (terminator
// stripped).
func (mlu *MLU) GetWide(lang, cntry string) []rune {
	l, c := strTo16(lang), strTo16(cntry)
	size := mlu.mluGetWideBuf(l, c, nil)
	if size == 0 {
		return nil
	}
	buf := make([]rune, size/wcharSize)
	got := mlu.mluGetWideBuf(l, c, buf)
	if got == 0 {
		return nil
	}
	return buf[:got/wcharSize-1]
}

// GetUTF8 ports cmsMLUgetUTF8 for callers wanting a Go string.
func (mlu *MLU) GetUTF8(lang, cntry string) string {
	l, c := strTo16(lang), strTo16(cntry)
	size := mlu.mluGetUTF8(l, c, nil)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	got := mlu.mluGetUTF8(l, c, buf)
	if got == 0 {
		return ""
	}
	return string(buf[:got-1])
}

// GetTranslation ports cmsMLUgetTranslation: the language/country actually used
// for a requested pair, or ok=false when there is no match.
func (mlu *MLU) GetTranslation(lang, cntry string) (obtainedLang, obtainedCntry string, ok bool) {
	if mlu == nil {
		return "", "", false
	}
	l, c := strTo16(lang), strTo16(cntry)
	_, _, ul, uc, found := mlu.mluGetWide(l, c)
	if !found {
		return "", "", false
	}
	lb, cb := strFrom16(ul), strFrom16(uc)
	return string(lb[:2]), string(cb[:2]), true
}

// TranslationsCount ports cmsMLUtranslationsCount.
func (mlu *MLU) TranslationsCount() uint32 {
	if mlu == nil {
		return 0
	}
	return mlu.usedEntries
}

// TranslationsCodes ports cmsMLUtranslationsCodes: the language/country of the
// idx-th entry.
func (mlu *MLU) TranslationsCodes(idx uint32) (lang, cntry string, ok bool) {
	if mlu == nil || idx >= mlu.usedEntries {
		return "", "", false
	}
	e := mlu.entries[idx]
	lb, cb := strFrom16(e.Language), strFrom16(e.Country)
	return string(lb[:2]), string(cb[:2]), true
}

// ---------------------------------------------------------------------------
// UTF-8 <-> code-point conversion (ported from cmsnamed.c)
// ---------------------------------------------------------------------------

// decodeUTF8 ports decodeUTF8 (the UTF-32 output path): convert a UTF-8 byte
// string to code points. The reference's simplified state machine is preserved
// verbatim, including its handling of continuation bytes.
func decodeUTF8(in []byte) []rune {
	var out []rune
	var codepoint uint32
	i := 0
	for i < len(in) {
		ch := in[i]
		switch {
		case ch <= 0x7f:
			codepoint = uint32(ch)
		case ch <= 0xbf:
			codepoint = (codepoint << 6) | uint32(ch&0x3f)
		case ch <= 0xdf:
			codepoint = uint32(ch & 0x1f)
		case ch <= 0xef:
			codepoint = uint32(ch & 0x0f)
		default:
			codepoint = uint32(ch & 0x07)
		}
		i++
		var next byte
		if i < len(in) {
			next = in[i]
		}
		if (next&0xc0) != 0x80 && codepoint <= 0x10ffff {
			// UTF-32 wchar_t path: one code point per output element.
			out = append(out, rune(codepoint))
		}
	}
	return out
}

// encodeUTF8 ports encodeUTF8 (the UTF-32 input path): convert code points to
// UTF-8. When out is nil the byte count is returned without writing. maxWchars
// caps the input; maxChars is the destination capacity (writes stop early to
// leave room, exactly as the reference gates each store).
func encodeUTF8(out []byte, in []rune, maxWchars, maxChars uint32) uint32 {
	var size uint32
	var lenW uint32
	var codepoint uint32
	pos := 0

	write := func(b byte) {
		if pos < len(out) {
			out[pos] = b
			pos++
		}
	}

	idx := 0
	for lenW < maxWchars && idx < len(in) && in[idx] != 0 {
		v := uint32(in[idx])
		if v >= 0xd800 && v <= 0xdbff {
			codepoint = ((v - 0xd800) << 10) + 0x10000
		} else {
			if v >= 0xdc00 && v <= 0xdfff {
				codepoint |= v - 0xdc00
			} else {
				codepoint = v
			}

			switch {
			case codepoint <= 0x7f:
				if out != nil && size+1 < maxChars {
					write(byte(codepoint))
				}
				size++
			case codepoint <= 0x7ff:
				if out != nil && maxChars > 0 && size+2 < maxChars {
					write(byte(0xc0 | ((codepoint >> 6) & 0x1f)))
					write(byte(0x80 | (codepoint & 0x3f)))
				}
				size += 2
			case codepoint <= 0xffff:
				if out != nil && maxChars > 0 && size+3 < maxChars {
					write(byte(0xe0 | ((codepoint >> 12) & 0x0f)))
					write(byte(0x80 | ((codepoint >> 6) & 0x3f)))
					write(byte(0x80 | (codepoint & 0x3f)))
				}
				size += 3
			default:
				if out != nil && maxChars > 0 && size+4 < maxChars {
					write(byte(0xf0 | ((codepoint >> 18) & 0x07)))
					write(byte(0x80 | ((codepoint >> 12) & 0x3f)))
					write(byte(0x80 | ((codepoint >> 6) & 0x3f)))
					write(byte(0x80 | (codepoint & 0x3f)))
				}
				size += 4
			}
			codepoint = 0
		}
		idx++
		lenW++
	}
	return size
}

// ---------------------------------------------------------------------------
// Named color lists
// ---------------------------------------------------------------------------

// maxNamedColorNameLen is cmsMAX_PATH: the storage width for a color name.
const maxNamedColorNameLen = maxPath

// namedColor mirrors _cmsNAMEDCOLOR: a color name plus its PCS and device
// colorant coordinates.
type namedColor struct {
	Name           string
	PCS            [3]uint16
	DeviceColorant [maxChannels]uint16
}

// NamedColorList mirrors cmsNAMEDCOLORLIST: an ordered list of named colors with
// a shared prefix/suffix and a fixed colorant count.
type NamedColorList struct {
	ctx *Context

	nColors       uint32
	allocated     uint32
	colorantCount uint32

	prefix [33]byte
	suffix [33]byte

	list []namedColor
}

// growNamedColorList ports GrowNamedColorList: expand the backing array,
// enforcing the 100K-entry cap.
func (v *NamedColorList) growNamedColorList() bool {
	var size uint32
	if v.allocated == 0 {
		size = 64
	} else {
		size = v.allocated * 2
	}
	if size > 1024*100 {
		// Cap reached. Leave the existing backing array and nColors intact so the
		// list stays usable; nil-ing v.list here (as the reference frees it)
		// would strand nColors valid entries pointing at a nil slice, panicking
		// every later Info/Dup/eval on the list.
		return false
	}
	grown := make([]namedColor, size)
	copy(grown, v.list)
	v.list = grown
	v.allocated = size
	return true
}

// AllocNamedColorList ports cmsAllocNamedColorList on a context.
func (ctx *Context) AllocNamedColorList(n, colorantCount uint32, prefix, suffix string) *NamedColorList {
	if colorantCount > maxChannels {
		return nil
	}
	v := &NamedColorList{ctx: ctx}
	for v.allocated < n {
		if !v.growNamedColorList() {
			return nil
		}
	}
	copyFixedStr(v.prefix[:], prefix)
	copyFixedStr(v.suffix[:], suffix)
	v.prefix[32] = 0
	v.suffix[32] = 0
	v.colorantCount = colorantCount
	return v
}

// AllocNamedColorList allocates a named color list on the default context.
func AllocNamedColorList(n, colorantCount uint32, prefix, suffix string) *NamedColorList {
	return defaultContext.AllocNamedColorList(n, colorantCount, prefix, suffix)
}

// copyFixedStr mirrors strncpy(dst, src, len(dst)-1): copy up to len(dst)-1
// bytes, leaving the remainder (including at least a trailing byte) zero.
func copyFixedStr(dst []byte, src string) {
	n := len(dst) - 1
	if n > len(src) {
		n = len(src)
	}
	for i := 0; i < n; i++ {
		dst[i] = src[i]
	}
	for i := n; i < len(dst); i++ {
		dst[i] = 0
	}
}

// Dup ports cmsDupNamedColorList.
func (v *NamedColorList) Dup() *NamedColorList {
	if v == nil {
		return nil
	}
	n := v.ctx.AllocNamedColorList(v.nColors, v.colorantCount, cStr(v.prefix[:]), cStr(v.suffix[:]))
	if n == nil {
		return nil
	}
	for n.allocated < v.allocated {
		if !n.growNamedColorList() {
			return nil
		}
	}
	n.prefix = v.prefix
	n.suffix = v.suffix
	n.colorantCount = v.colorantCount
	if v.nColors > 0 {
		copy(n.list, v.list[:v.nColors])
	}
	n.nColors = v.nColors
	return n
}

// cStr returns the NUL-terminated prefix of a fixed byte buffer as a Go string.
func cStr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// AppendNamedColor ports cmsAppendNamedColor. A nil colorant/PCS stores zeros.
func (v *NamedColorList) AppendNamedColor(name string, pcs *[3]uint16, colorant *[maxChannels]uint16) bool {
	if v == nil {
		return false
	}
	if v.nColors+1 > v.allocated {
		if !v.growNamedColorList() {
			return false
		}
	}
	var nc namedColor
	for i := uint32(0); i < v.colorantCount; i++ {
		if colorant != nil {
			nc.DeviceColorant[i] = colorant[i]
		}
	}
	for i := 0; i < 3; i++ {
		if pcs != nil {
			nc.PCS[i] = pcs[i]
		}
	}
	// strncpy(Name, ..., cmsMAX_PATH-1) then Name[cmsMAX_PATH-1]=0.
	if len(name) > maxNamedColorNameLen-1 {
		name = name[:maxNamedColorNameLen-1]
	}
	nc.Name = name
	v.list[v.nColors] = nc
	v.nColors++
	return true
}

// Count ports cmsNamedColorCount.
func (v *NamedColorList) Count() uint32 {
	if v == nil {
		return 0
	}
	return v.nColors
}

// Info ports cmsNamedColorInfo: the name/prefix/suffix/PCS/colorant of a color.
func (v *NamedColorList) Info(nColor uint32) (name, prefix, suffix string, pcs [3]uint16, colorant [maxChannels]uint16, ok bool) {
	if v == nil || nColor >= v.Count() {
		return "", "", "", pcs, colorant, false
	}
	c := &v.list[nColor]
	return c.Name, cStr(v.prefix[:]), cStr(v.suffix[:]), c.PCS, c.DeviceColorant, true
}

// Index ports cmsNamedColorIndex: the position of a color by name (ASCII
// case-insensitive), or -1.
func (v *NamedColorList) Index(name string) int32 {
	if v == nil {
		return -1
	}
	for i := uint32(0); i < v.nColors; i++ {
		if asciiEqualFold(name, v.list[i].Name) {
			return int32(i)
		}
	}
	return -1
}

// asciiEqualFold compares two strings case-insensitively over ASCII only,
// matching cmsstrcasecmp.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if asciiLower(a[i]) != asciiLower(b[i]) {
			return false
		}
	}
	return true
}

func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// ColorantCount reports the number of device coordinates per color.
func (v *NamedColorList) ColorantCount() uint32 { return v.colorantCount }

// Prefix/Suffix expose the shared name affixes.
func (v *NamedColorList) Prefix() string { return cStr(v.prefix[:]) }
func (v *NamedColorList) Suffix() string { return cStr(v.suffix[:]) }

// ---------------------------------------------------------------------------
// Named-color MPE stage (cmsSigNamedColorElemType), ported from cmsnamed.c
// ---------------------------------------------------------------------------

// evalNamedColorPCS ports EvalNamedColorPCS: map an index (In[0]) to the color's
// PCS coordinates. Named color always uses Lab. Out-of-range indices yield 0.
func evalNamedColorPCS(in, out []float32, mpe *Stage) {
	list, ok := mpe.data.(*NamedColorList)
	if !ok || list == nil {
		return
	}
	index := quickSaturateWord(float64(in[0]) * 65535.0)
	if uint32(index) >= list.nColors {
		list.ctx.signalError(ErrRange, "Color %d out of range", index)
		out[0], out[1], out[2] = 0, 0, 0
		return
	}
	out[0] = float32(list.list[index].PCS[0]) / 65535.0
	out[1] = float32(list.list[index].PCS[1]) / 65535.0
	out[2] = float32(list.list[index].PCS[2]) / 65535.0
}

// evalNamedColor ports EvalNamedColor: map an index (In[0]) to the color's
// device colorant coordinates. Out-of-range indices yield 0.
func evalNamedColor(in, out []float32, mpe *Stage) {
	list, ok := mpe.data.(*NamedColorList)
	if !ok || list == nil {
		return
	}
	index := quickSaturateWord(float64(in[0]) * 65535.0)
	if uint32(index) >= list.nColors {
		list.ctx.signalError(ErrRange, "Color %d out of range", index)
		for j := uint32(0); j < list.colorantCount; j++ {
			out[j] = 0
		}
		return
	}
	for j := uint32(0); j < list.colorantCount; j++ {
		out[j] = float32(list.list[index].DeviceColorant[j]) / 65535.0
	}
}

// stageAllocNamedColor ports _cmsStageAllocNamedColor: a named-color lookup MPE.
// When usePCS is true the stage outputs 3 PCS channels, otherwise ColorantCount
// device channels. The stage owns a duplicate of the color list.
func stageAllocNamedColor(list *NamedColorList, usePCS bool) *Stage {
	if list == nil {
		return nil
	}
	out := list.colorantCount
	eval := evalNamedColor
	if usePCS {
		out = 3
		eval = evalNamedColorPCS
	}
	return list.ctx.stageAllocPlaceholder(SigNamedColorElemType, 1, out, eval, list.Dup())
}

// ---------------------------------------------------------------------------
// Profile sequence description
// ---------------------------------------------------------------------------

// PSeqDesc mirrors cmsPSEQDESC: one profile's identification within a sequence.
type PSeqDesc struct {
	DeviceMfg    uint32
	DeviceModel  uint32
	Attributes   uint64
	Technology   uint32
	ProfileID    [16]byte
	Manufacturer *MLU
	Model        *MLU
	Description  *MLU
}

// ProfileSequence mirrors cmsSEQ: an ordered set of profile descriptors.
type ProfileSequence struct {
	ctx *Context
	n   uint32
	seq []PSeqDesc
}

// N reports the number of entries.
func (s *ProfileSequence) N() uint32 { return s.n }

// Seq exposes the descriptor slice for handlers in the same package.
func (s *ProfileSequence) Seq() []PSeqDesc { return s.seq }

// AllocProfileSequenceDescription ports cmsAllocProfileSequenceDescription: n
// must be in 1..255.
func (ctx *Context) AllocProfileSequenceDescription(n uint32) *ProfileSequence {
	if n == 0 || n > 255 {
		return nil
	}
	return &ProfileSequence{ctx: ctx, n: n, seq: make([]PSeqDesc, n)}
}

// AllocProfileSequenceDescription on the default context.
func AllocProfileSequenceDescription(n uint32) *ProfileSequence {
	return defaultContext.AllocProfileSequenceDescription(n)
}

// Dup ports cmsDupProfileSequenceDescription.
func (s *ProfileSequence) Dup() *ProfileSequence {
	if s == nil {
		return nil
	}
	n := &ProfileSequence{ctx: s.ctx, n: s.n, seq: make([]PSeqDesc, s.n)}
	for i := uint32(0); i < s.n; i++ {
		src := &s.seq[i]
		dst := &n.seq[i]
		dst.Attributes = src.Attributes
		dst.DeviceMfg = src.DeviceMfg
		dst.DeviceModel = src.DeviceModel
		dst.ProfileID = src.ProfileID
		dst.Technology = src.Technology
		dst.Manufacturer = src.Manufacturer.Dup()
		dst.Model = src.Model.Dup()
		dst.Description = src.Description.Dup()
	}
	return n
}

// ---------------------------------------------------------------------------
// Dictionaries (linked list of entries)
// ---------------------------------------------------------------------------

// DictEntry mirrors cmsDICTentry: a name/value pair with optional localized
// display strings. Name and Value hold code points (Value may be nil).
type DictEntry struct {
	Next         *DictEntry
	DisplayName  *MLU
	DisplayValue *MLU
	Name         []rune
	Value        []rune
}

// Dict mirrors _cmsDICT: the linked-list head plus its context.
type Dict struct {
	ctx  *Context
	head *DictEntry
}

// DictAlloc ports cmsDictAlloc on a context.
func (ctx *Context) DictAlloc() *Dict { return &Dict{ctx: ctx} }

// DictAlloc on the default context.
func DictAlloc() *Dict { return defaultContext.DictAlloc() }

// dupWcs ports DupWcs: an independent copy of a code-point string, or nil.
func dupWcs(s []rune) []rune {
	if s == nil {
		return nil
	}
	n := mywcslen(s)
	out := make([]rune, n)
	copy(out, s[:n])
	return out
}

// AddEntry ports cmsDictAddEntry: prepend a new entry. Name must be non-nil.
func (d *Dict) AddEntry(name, value []rune, displayName, displayValue *MLU) bool {
	if d == nil || name == nil {
		return false
	}
	e := &DictEntry{
		DisplayName:  displayName.Dup(),
		DisplayValue: displayValue.Dup(),
		Name:         dupWcs(name),
		Value:        dupWcs(value),
		Next:         d.head,
	}
	d.head = e
	return true
}

// Dup ports cmsDictDup: a copy whose iteration order is the reverse of the
// original's, exactly as the reference walk-and-prepend produces.
func (d *Dict) Dup() *Dict {
	if d == nil {
		return nil
	}
	n := d.ctx.DictAlloc()
	for e := d.head; e != nil; e = e.Next {
		if !n.AddEntry(e.Name, e.Value, e.DisplayName, e.DisplayValue) {
			return nil
		}
	}
	return n
}

// GetEntryList ports cmsDictGetEntryList: the head of the linked list.
func (d *Dict) GetEntryList() *DictEntry {
	if d == nil {
		return nil
	}
	return d.head
}

// NextEntry ports cmsDictNextEntry.
func (e *DictEntry) NextEntry() *DictEntry {
	if e == nil {
		return nil
	}
	return e.Next
}
