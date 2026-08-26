// Port of the cmsSigDictType ('dict') tag-type handler from src/cmstypes.c.
//
// The dictionary type stores a table of name/value pairs, each optionally
// carrying localized display strings. The on-disk layout is an offset table
// (the _cmsDICarray machinery) followed by the referenced wchar strings and
// MLUC blocks. Each record occupies 16, 24, or 32 bytes in the offset table:
//
//	16 bytes: Name(off,size) Value(off,size)
//	24 bytes: + DisplayName(off,size)
//	32 bytes: + DisplayValue(off,size)
//
// Name and Value are wchar (UTF-16) strings; DisplayName and DisplayValue are
// MLUC blocks (multiLocalizedUnicodeType). An offset of zero is a sentinel for
// an undefined string and must be preserved. The value type is *Dict (named.go).
//
// PORTNOTES:
//   - The reference allocates the column arrays (AllocArray) before validating
//     Count against the tag size; oversized counts fail later in
//     ReadOffsetArray's running size check. To avoid unbounded allocation on
//     untrusted input this port caps Count against SizeOfTag up front (a valid
//     dictionary needs at least 16 offset-table bytes per record, so the cap
//     never rejects a well-formed tag) — the accept/reject decision matches the
//     reference for both valid and oversized inputs.
//   - The reference reader rejects any record whose Name or Value wchar string
//     is a null sentinel ("Bad dictionary Name/Value"); this port replicates
//     that exactly, even though it means a dictionary written with a nil Value
//     will not read back.

package lcms2

// dicElem mirrors _cmsDICelem: the offset/size column pair for one field.
type dicElem struct {
	Offsets []uint32
	Sizes   []uint32
}

// dicArray mirrors _cmsDICarray: the four fields' column arrays. A field's
// columns are nil when the record length does not include it.
type dicArray struct {
	Name, Value, DisplayName, DisplayValue dicElem
}

// allocElem ports AllocElem: allocate the zeroed column arrays for one field.
func allocDicElem(e *dicElem, count uint32) {
	e.Offsets = make([]uint32, count)
	e.Sizes = make([]uint32, count)
}

// allocArray ports AllocArray: allocate columns per the record length.
func allocDicArray(a *dicArray, count, length uint32) {
	*a = dicArray{}
	allocDicElem(&a.Name, count)
	allocDicElem(&a.Value, count)
	if length > 16 {
		allocDicElem(&a.DisplayName, count)
	}
	if length > 24 {
		allocDicElem(&a.DisplayValue, count)
	}
}

// readOneDicElem ports ReadOneElem: read one (offset,size) pair; a non-zero
// offset is rebased onto BaseOffset (a zero offset is preserved as a sentinel).
func readOneDicElem(io *IOHandler, e *dicElem, i, baseOffset uint32) bool {
	off, ok := readUInt32(io)
	if !ok {
		return false
	}
	sz, ok := readUInt32(io)
	if !ok {
		return false
	}
	if off > 0 {
		off += baseOffset
	}
	e.Offsets[i] = off
	e.Sizes[i] = sz
	return true
}

// readOffsetArray ports ReadOffsetArray: read the full offset table, tracking a
// signed remaining-size budget so malformed lengths are rejected exactly as the
// reference does. signedSizeOfTag is updated and returned.
func readOffsetArray(io *IOHandler, a *dicArray, count, length, baseOffset uint32, signedSizeOfTag int64) (int64, bool) {
	for i := uint32(0); i < count; i++ {
		if signedSizeOfTag < 4*4 {
			return signedSizeOfTag, false
		}
		signedSizeOfTag -= 4 * 4

		if !readOneDicElem(io, &a.Name, i, baseOffset) {
			return signedSizeOfTag, false
		}
		if !readOneDicElem(io, &a.Value, i, baseOffset) {
			return signedSizeOfTag, false
		}

		if length > 16 {
			if signedSizeOfTag < 2*4 {
				return signedSizeOfTag, false
			}
			signedSizeOfTag -= 2 * 4
			if !readOneDicElem(io, &a.DisplayName, i, baseOffset) {
				return signedSizeOfTag, false
			}
		}

		if length > 24 {
			if signedSizeOfTag < 2*4 {
				return signedSizeOfTag, false
			}
			signedSizeOfTag -= 2 * 4
			if !readOneDicElem(io, &a.DisplayValue, i, baseOffset) {
				return signedSizeOfTag, false
			}
		}
	}
	return signedSizeOfTag, true
}

// writeOneDicElem ports WriteOneElem.
func writeOneDicElem(io *IOHandler, e *dicElem, i uint32) bool {
	return writeUInt32(io, e.Offsets[i]) && writeUInt32(io, e.Sizes[i])
}

// writeOffsetArray ports WriteOffsetArray.
func writeOffsetArray(io *IOHandler, a *dicArray, count, length uint32) bool {
	for i := uint32(0); i < count; i++ {
		if !writeOneDicElem(io, &a.Name, i) {
			return false
		}
		if !writeOneDicElem(io, &a.Value, i) {
			return false
		}
		if length > 16 {
			if !writeOneDicElem(io, &a.DisplayName, i) {
				return false
			}
		}
		if length > 24 {
			if !writeOneDicElem(io, &a.DisplayValue, i) {
				return false
			}
		}
	}
	return true
}

// readOneWChar ports ReadOneWChar: read a wchar string for column field i.
// Returns (runes, hasString): hasString is false for the zero-offset sentinel
// (an undefined string), in which case runes is nil.
func readOneWChar(io *IOHandler, e *dicElem, i uint32) (runes []rune, ok bool) {
	if e.Offsets[i] == 0 {
		return nil, true
	}
	if !io.Seek(e.Offsets[i]) {
		return nil, false
	}
	nChars := e.Sizes[i] / 2 // sizeof(cmsUInt16Number)
	if nChars > 0x7ffff {
		return nil, false
	}
	if nChars == 0 {
		// Non-null but empty: distinguished from the sentinel above.
		return []rune{}, true
	}
	block := make([]rune, nChars)
	if !readWCharArray(io, nChars, block) {
		return nil, false
	}
	return block, true
}

// mluOrNil converts a value returned by typeMLURead to *MLU.
func mluFromAny(v any) *MLU {
	if m, ok := v.(*MLU); ok {
		return m
	}
	return nil
}

// readOneMLUC ports ReadOneMLUC: read an MLUC block for column field i. A zero
// offset or zero size yields a nil MLU (an undefined display string).
func readOneMLUC(self *TagTypeHandler, io *IOHandler, e *dicElem, i uint32) (*MLU, bool) {
	if e.Offsets[i] == 0 || e.Sizes[i] == 0 {
		return nil, true
	}
	if !io.Seek(e.Offsets[i]) {
		return nil, false
	}
	v, _, err := typeMLURead(self, io, e.Sizes[i])
	if err != nil {
		return nil, false
	}
	mlu := mluFromAny(v)
	return mlu, mlu != nil
}

// typeDictionaryRead ports Type_Dictionary_Read.
func typeDictionaryRead(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (any, uint32, error) {
	signedSizeOfTag := int64(sizeOfTag)

	// Get actual position as a basis for element offsets.
	baseOffset := io.Tell() - tagBaseSize

	// Get name-value record count.
	signedSizeOfTag -= 4
	if signedSizeOfTag < 0 {
		return nil, 0, errorf(ErrCorruptionDetected, "dict: truncated header")
	}
	count, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "dict read error")
	}

	// Get record length.
	signedSizeOfTag -= 4
	if signedSizeOfTag < 0 {
		return nil, 0, errorf(ErrCorruptionDetected, "dict: truncated header")
	}
	length, ok := readUInt32(io)
	if !ok {
		return nil, 0, errorf(ErrRead, "dict read error")
	}

	// Check for valid lengths.
	if length != 16 && length != 24 && length != 32 {
		return nil, 0, self.ContextID.signalError(ErrUnknownExtension, "Unknown record length in dictionary '%d'", length)
	}

	// Cap Count against the tag size before allocating: each record needs at
	// least 16 bytes in the offset table. See PORTNOTES.
	if count != 0 && uint64(count) > uint64(sizeOfTag)/16 {
		return nil, 0, errorf(ErrCorruptionDetected, "dict: implausible entry count")
	}

	// Create an empty dictionary.
	hDict := self.ContextID.DictAlloc()
	if hDict == nil {
		return nil, 0, errorf(ErrInternal, "dict: alloc failed")
	}

	// On depending on record size, create column arrays.
	var a dicArray
	allocDicArray(&a, count, length)

	// Read column arrays.
	if _, ok := readOffsetArray(io, &a, count, length, baseOffset, signedSizeOfTag); !ok {
		return nil, 0, errorf(ErrCorruptionDetected, "dict: bad offset table")
	}

	// Seek to each element and read it.
	for i := uint32(0); i < count; i++ {
		nameWCS, ok := readOneWChar(io, &a.Name, i)
		if !ok {
			return nil, 0, errorf(ErrCorruptionDetected, "dict: bad name string")
		}
		valueWCS, ok := readOneWChar(io, &a.Value, i)
		if !ok {
			return nil, 0, errorf(ErrCorruptionDetected, "dict: bad value string")
		}

		var displayNameMLU, displayValueMLU *MLU
		if length > 16 {
			displayNameMLU, ok = readOneMLUC(self, io, &a.DisplayName, i)
			if !ok {
				return nil, 0, errorf(ErrCorruptionDetected, "dict: bad display name")
			}
		}
		if length > 24 {
			displayValueMLU, ok = readOneMLUC(self, io, &a.DisplayValue, i)
			if !ok {
				return nil, 0, errorf(ErrCorruptionDetected, "dict: bad display value")
			}
		}

		if nameWCS == nil || valueWCS == nil {
			return nil, 0, self.ContextID.signalError(ErrCorruptionDetected, "Bad dictionary Name/Value")
		}

		if !hDict.AddEntry(nameWCS, valueWCS, displayNameMLU, displayValueMLU) {
			return nil, 0, errorf(ErrCorruptionDetected, "dict: add entry failed")
		}
	}

	return hDict, 1, nil
}

// writeOneWChar ports WriteOneWChar: write a wchar string, filling in its
// offset/size in the column arrays. A nil string writes the zero sentinel.
func writeOneWChar(io *IOHandler, e *dicElem, i uint32, wcstr []rune, isNil bool, baseOffset uint32) bool {
	before := io.Tell()
	e.Offsets[i] = before - baseOffset

	if isNil {
		e.Sizes[i] = 0
		e.Offsets[i] = 0
		return true
	}

	n := mywcslen(wcstr)
	if !writeWCharArray(io, n, wcstr) {
		return false
	}
	e.Sizes[i] = io.Tell() - before
	return true
}

// writeOneMLUC ports WriteOneMLUC: write an MLUC block for a display string,
// filling in its offset/size. A nil MLU writes the zero sentinel.
func writeOneMLUC(self *TagTypeHandler, io *IOHandler, e *dicElem, i uint32, mlu *MLU, baseOffset uint32) bool {
	if mlu == nil {
		e.Sizes[i] = 0
		e.Offsets[i] = 0
		return true
	}
	before := io.Tell()
	if e.Offsets != nil {
		e.Offsets[i] = before - baseOffset
	}
	if err := typeMLUWrite(self, io, mlu, 1); err != nil {
		return false
	}
	if e.Sizes != nil {
		e.Sizes[i] = io.Tell() - before
	}
	return true
}

// typeDictionaryWrite ports Type_Dictionary_Write. nItems is unused, matching
// the reference's cmsUNUSED_PARAMETER(nItems).
func typeDictionaryWrite(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error {
	_ = nItems
	hDict, ok := value.(*Dict)
	if !ok || hDict == nil {
		return errorf(ErrWrite, "dict: nil dictionary")
	}

	baseOffset := io.Tell() - tagBaseSize

	// Inspect the dictionary.
	var count uint32
	anyName, anyValue := false, false
	for p := hDict.GetEntryList(); p != nil; p = p.NextEntry() {
		if p.DisplayName != nil {
			anyName = true
		}
		if p.DisplayValue != nil {
			anyValue = true
		}
		count++
	}

	length := uint32(16)
	if anyName {
		length += 8
	}
	if anyValue {
		length += 8
	}

	if !writeUInt32(io, count) || !writeUInt32(io, length) {
		return errorf(ErrWrite, "dict write error")
	}

	// Keep starting position of the offsets table.
	directoryPos := io.Tell()

	var a dicArray
	allocDicArray(&a, count, length)

	// Write a fake directory to be filled in later.
	if !writeOffsetArray(io, &a, count, length) {
		return errorf(ErrWrite, "dict write error")
	}

	// Write each element, tracking sizes.
	p := hDict.GetEntryList()
	for i := uint32(0); i < count; i++ {
		if !writeOneWChar(io, &a.Name, i, p.Name, p.Name == nil, baseOffset) {
			return errorf(ErrWrite, "dict write error")
		}
		if !writeOneWChar(io, &a.Value, i, p.Value, p.Value == nil, baseOffset) {
			return errorf(ErrWrite, "dict write error")
		}
		if p.DisplayName != nil {
			if !writeOneMLUC(self, io, &a.DisplayName, i, p.DisplayName, baseOffset) {
				return errorf(ErrWrite, "dict write error")
			}
		}
		if p.DisplayValue != nil {
			if !writeOneMLUC(self, io, &a.DisplayValue, i, p.DisplayValue, baseOffset) {
				return errorf(ErrWrite, "dict write error")
			}
		}
		p = p.NextEntry()
	}

	// Write the real directory.
	currentPos := io.Tell()
	if !io.Seek(directoryPos) {
		return errorf(ErrSeek, "dict write error")
	}
	if !writeOffsetArray(io, &a, count, length) {
		return errorf(ErrWrite, "dict write error")
	}
	if !io.Seek(currentPos) {
		return errorf(ErrSeek, "dict write error")
	}

	return nil
}

// typeDictionaryDup ports Type_Dictionary_Dup.
func typeDictionaryDup(self *TagTypeHandler, value any, n uint32) (any, error) {
	d, ok := value.(*Dict)
	if !ok {
		return nil, errorf(ErrInternal, "dict dup: wrong type")
	}
	return d.Dup(), nil
}

func init() {
	registerBuiltinType(&TagTypeHandler{
		Signature: SigDictType,
		Read:      typeDictionaryRead,
		Write:     typeDictionaryWrite,
		Dup:       typeDictionaryDup,
	})
}
