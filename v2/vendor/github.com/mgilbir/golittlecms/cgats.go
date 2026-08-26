package lcms2

// This file ports src/cmscgats.c (lcms2 2.19): the IT8.7 / CGATS.17 handling —
// a lexer, parser and writer for the measurement-data sheet format exposed by
// the cmsIT8* API, plus the property / data / multi-table model.
//
// Faithful-port notes (intentional, behaviour-preserving deviations):
//
//   - The C custom sub-allocator (AllocChunk / AllocBigBlock / OWNEDMEM /
//     SUBALLOCATOR) and AllocString are replaced by ordinary Go allocations and
//     strings: Go's garbage collector owns memory. The observable behaviour is
//     identical. The *bounds* the C code enforces on counts derived from
//     untrusted input (NUMBER_OF_FIELDS / NUMBER_OF_SETS and their product) are
//     replicated exactly, because they are the security-relevant part.
//   - char* fields that C leaves NULL vs. a real string become *string (nil =
//     C NULL). This distinction is load-bearing for the writer (a nil data cell
//     is written as "", a nil value suppresses the "\t..." tail).
//   - The KEYVALUE property list keeps the C linked-list shape (Next /
//     NextSubkey) so enumeration and save order are byte-for-byte identical.
//   - The suballocator "Out of memory" error paths cannot occur under the GC,
//     so those SynError branches are omitted; every other guard is preserved.
//   - The .cube device-link reader (parseCube / CreateDeviceLinkFromCubeFile)
//     reuses this lexer with the CUBE keyword table (isCUBE selects it in
//     inSymbol), mirroring the reference's shared tokenizer. See parseCube and
//     CreateDeviceLinkFromCubeMem below.
//
// The parser is the module's untrusted-input surface: it must never panic,
// never hang and never allocate unboundedly. See FuzzIT8LoadFromMem.

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// Limits mirroring the #defines in cmscgats.c.
const (
	cgatsMaxID      = 128  // MAXID: max length of identifier
	cgatsMaxStr     = 1024 // MAXSTR: max length of string
	cgatsMaxTables  = 255  // MAXTABLES: max tables in a single stream
	cgatsMaxInclude = 20   // MAXINCLUDE: max nested includes
	cgatsMaxPath    = 256  // cmsMAX_PATH

	cgatsDefaultDblFormat = "%.10g" // DEFAULT_DBL_FORMAT
	dirChar               = '/'     // DIR_CHAR (non-Windows)
)

// symbol is the CGATS token type (the C SYMBOL enum).
type symbol int

const (
	symUndefined symbol = iota
	symInum             // integer
	symDnum             // real
	symIdent            // identifier
	symString           // string
	symComment          // comment
	symEoln             // end of line
	symEof              // end of stream
	symSynError         // syntax error found on stream

	// IT8 symbols
	symBeginData
	symBeginDataFormat
	symEndData
	symEndDataFormat
	symKeyword
	symDataFormatID
	symInclude

	// CUBE symbols (the C S* / SDOMAIN* / STITLE enumerators). Recognized only
	// when isCUBE is set, via the separate tabKeysCUBE keyword table.
	symDomainMax
	symDomainMin
	symLut1DSize
	symLut1DInputRange
	symLut3DSize
	symLut3DInputRange
	symLutInVideoRange
	symLutOutVideoRange
	symTitle
)

// writeMode mirrors the C WRITEMODE enum: how a property value is serialized.
type writeMode int

const (
	writeUncooked writeMode = iota
	writeStringify
	writeHexadecimal
	writeBinary
	writePair
)

// keyValue is a node of the property linked list (the C KEYVALUE). subkey and
// value are *string so a genuine C NULL is distinguishable from an empty
// string, which the writer depends on.
type keyValue struct {
	next       *keyValue
	keyword    string
	nextSubkey *keyValue
	subkey     *string
	value      *string
	writeAs    writeMode
}

// table is one measurement table (the C TABLE): its properties plus the
// column-labelled data grid.
type table struct {
	sheetType string // first row of the IT8 (the type)

	nSamples int // columns
	nPatches int // rows
	sampleID int // column index of the SAMPLE_ID field

	headerList *keyValue // the properties

	// dataFormat has nSamples+1 slots once allocated (nil = unallocated); each
	// entry is nil (C NULL) or a column label.
	dataFormat []*string
	// data has (nSamples+1)*(nPatches+1) slots once allocated (nil =
	// unallocated); each entry is nil (C NULL) or a cell value.
	data []*string
}

// cgatsStream is one entry of the include/file stack. For an in-memory load the
// base entry has isMem set and data holds the (NUL-truncated, NUL-terminated)
// source; for a file it holds the file bytes.
type cgatsStream struct {
	name  string
	data  []byte
	pos   int
	isMem bool
}

// IT8 is the pure-Go replacement for the C cmsIT8 handle. It holds the parsed
// (or being-built) sheet: its tables, the parser state machine, and the shared
// keyword/sample-id vocabularies.
type IT8 struct {
	ctx *Context

	tablesCount uint32
	nTable      uint32

	isCUBE bool

	tab []*table

	// Parser state machine
	sy   symbol
	ch   int
	inum int32
	dnum float64

	id  *strbuf // identifier
	str *strbuf // string

	validKeywords *keyValue
	validSampleID *keyValue

	fileStack []*cgatsStream
	includeSP int
	lineno    int

	doubleFormatter string

	parseErr error // first error raised during parsing
}

// strbuf is the tiny growable string builder replacing the C `string` type and
// its StringAlloc/StringAppend/StringClear/StringPtr helpers. Growth is handled
// by Go's append (bounded by input length), so the C out-of-memory paths do not
// arise.
type strbuf struct {
	b []byte
}

func (s *strbuf) clear()         { s.b = s.b[:0] }
func (s *strbuf) append(c byte)  { s.b = append(s.b, c) }
func (s *strbuf) cat(str string) { s.b = append(s.b, str...) }
func (s *strbuf) String() string { return string(s.b) }

// ------------------------------------------------------ keyword tables

type cgatsKeyword struct {
	id string
	sy symbol
}

// tabKeysIT8 is the sorted keyword→symbol table (C TabKeysIT8). Binary search
// requires it stay sorted (ASCII, case-insensitively already ordered here).
var tabKeysIT8 = []cgatsKeyword{
	{"$INCLUDE", symInclude},
	{".INCLUDE", symInclude},
	{"BEGIN_DATA", symBeginData},
	{"BEGIN_DATA_FORMAT", symBeginDataFormat},
	{"DATA_FORMAT_IDENTIFIER", symDataFormatID},
	{"END_DATA", symEndData},
	{"END_DATA_FORMAT", symEndDataFormat},
	{"KEYWORD", symKeyword},
}

// tabKeysCUBE mirrors the C TabKeysCUBE table used when parsing a .cube file.
// The order is copied verbatim from the reference — it is NOT strictly sorted
// (LUT_1D_SIZE precedes LUT_1D_INPUT_RANGE), so the same binary search reaches
// the same subset of keywords the C code does; the INPUT_RANGE entries are in
// fact unreachable in both implementations, a faithful reproduction of the
// reference behaviour rather than a bug to be fixed.
var tabKeysCUBE = []cgatsKeyword{
	{"DOMAIN_MAX", symDomainMax},
	{"DOMAIN_MIN", symDomainMin},
	{"LUT_1D_SIZE", symLut1DSize},
	{"LUT_1D_INPUT_RANGE", symLut1DInputRange},
	{"LUT_3D_SIZE", symLut3DSize},
	{"LUT_3D_INPUT_RANGE", symLut3DInputRange},
	{"LUT_IN_VIDEO_RANGE", symLutInVideoRange},
	{"LUT_OUT_VIDEO_RANGE", symLutOutVideoRange},
	{"TITLE", symTitle},
}

type cgatsProperty struct {
	id string
	as writeMode
}

// predefinedProperties mirrors the C PredefinedProperties table.
var predefinedProperties = []cgatsProperty{
	{"NUMBER_OF_FIELDS", writeUncooked},
	{"NUMBER_OF_SETS", writeUncooked},
	{"ORIGINATOR", writeStringify},
	{"FILE_DESCRIPTOR", writeStringify},
	{"CREATED", writeStringify},
	{"DESCRIPTOR", writeStringify},
	{"DIFFUSE_GEOMETRY", writeStringify},
	{"MANUFACTURER", writeStringify},
	{"MANUFACTURE", writeStringify},
	{"PROD_DATE", writeStringify},
	{"SERIAL", writeStringify},
	{"MATERIAL", writeStringify},
	{"INSTRUMENTATION", writeStringify},
	{"MEASUREMENT_SOURCE", writeStringify},
	{"PRINT_CONDITIONS", writeStringify},
	{"SAMPLE_BACKING", writeStringify},
	{"CHISQ_DOF", writeStringify},
	{"MEASUREMENT_GEOMETRY", writeStringify},
	{"FILTER", writeStringify},
	{"POLARIZATION", writeStringify},
	{"WEIGHTING_FUNCTION", writePair},
	{"COMPUTATIONAL_PARAMETER", writePair},
	{"TARGET_TYPE", writeStringify},
	{"COLORANT", writeStringify},
	{"TABLE_DESCRIPTOR", writeStringify},
	{"TABLE_NAME", writeStringify},
}

// predefinedSampleID mirrors the C PredefinedSampleID table.
var predefinedSampleID = []string{
	"SAMPLE_ID", "STRING",
	"CMYK_C", "CMYK_M", "CMYK_Y", "CMYK_K",
	"D_RED", "D_GREEN", "D_BLUE", "D_VIS", "D_MAJOR_FILTER",
	"RGB_R", "RGB_G", "RGB_B",
	"SPECTRAL_NM", "SPECTRAL_PCT", "SPECTRAL_DEC",
	"XYZ_X", "XYZ_Y", "XYZ_Z",
	"XYY_X", "XYY_Y", "XYY_CAPY",
	"LAB_L", "LAB_A", "LAB_B", "LAB_C", "LAB_H",
	"LAB_DE", "LAB_DE_94", "LAB_DE_CMC", "LAB_DE_2000",
	"MEAN_DE",
	"STDEV_X", "STDEV_Y", "STDEV_Z", "STDEV_L", "STDEV_A", "STDEV_B", "STDEV_DE",
	"CHI_SQD_PAR",
}

// ------------------------------------------------------ character classes

func cgatsIsSeparator(c int) bool { return c == ' ' || c == '\t' }

func cgatsIsDigit(c int) bool { return c >= '0' && c <= '9' }

func cgatsIsAlnum(c int) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func cgatsIsXDigit(c int) bool {
	return cgatsIsDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func cgatsToUpper(c int) int {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

// ismiddle: a valid identifier char that is not alphanumeric.
func cgatsIsMiddle(c int) bool {
	return !cgatsIsSeparator(c) && c != '#' && c != '"' && c != '\'' && c > 32 && c < 127
}

func cgatsIsIDChar(c int) bool { return cgatsIsAlnum(c) || cgatsIsMiddle(c) }

func cgatsIsFirstIDChar(c int) bool { return c != '-' && !cgatsIsDigit(c) && cgatsIsMiddle(c) }

// cgatsStrcasecmp mirrors cmsstrcasecmp: ASCII case-insensitive compare
// returning sign, used by the keyword binary search.
func cgatsStrcasecmp(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ca := int(asciiLower(a[i]))
		cb := int(asciiLower(b[i]))
		if ca != cb {
			return ca - cb
		}
	}
	return len(a) - len(b)
}

// ------------------------------------------------------ path helpers

func cgatsIsAbsolutePath(path string) bool {
	if path == "" {
		return false
	}
	return path[0] == dirChar
}

// cgatsBuildAbsolutePath makes a file path for an include relative to a base
// path. Mirrors BuildAbsolutePath (non-Windows). Returns ("", false) on failure.
func cgatsBuildAbsolutePath(relPath, basePath string) (string, bool) {
	if cgatsIsAbsolutePath(relPath) {
		if len(relPath) > cgatsMaxPath-1 {
			relPath = relPath[:cgatsMaxPath-1]
		}
		return relPath, true
	}

	idx := strings.LastIndexByte(basePath, dirChar)
	if idx < 0 {
		return "", false // not absolute and has no separators
	}
	joined := basePath[:idx+1] + relPath
	if len(joined) > cgatsMaxPath-1 {
		joined = joined[:cgatsMaxPath-1]
	}
	return joined, true
}

// ------------------------------------------------------ error reporting

// synError raises a corruption error, flips the state machine into the error
// state and records the first error. It always returns false so callers can
// `return it8.synError(...)`.
func (it8 *IT8) synError(format string, args ...any) bool {
	buf := fmt.Sprintf(format, args...)
	if len(buf) > 255 {
		buf = buf[:255]
	}
	name := ""
	if it8.includeSP >= 0 && it8.includeSP < len(it8.fileStack) && it8.fileStack[it8.includeSP] != nil {
		name = it8.fileStack[it8.includeSP].name
	}
	msg := fmt.Sprintf("%s: Line %d, %s", name, it8.lineno, buf)
	it8.sy = symSynError
	e := it8.ctx.signalError(ErrCorruptionDetected, "%s", msg)
	if it8.parseErr == nil {
		it8.parseErr = e
	}
	return false
}

// check verifies the current symbol, raising an error otherwise.
func (it8 *IT8) check(sy symbol, errText string) bool {
	if it8.sy != sy {
		return it8.synError("%s", errText)
	}
	return true
}

// ------------------------------------------------------ lexer

// nextCh reads the next character from the current stream (the C NextCh),
// popping include levels at EOF.
func (it8 *IT8) nextCh() {
	fs := it8.fileStack[it8.includeSP]
	if !fs.isMem {
		if fs.pos < len(fs.data) {
			it8.ch = int(fs.data[fs.pos])
			fs.pos++
		} else {
			if it8.includeSP > 0 {
				it8.includeSP--
				it8.ch = ' ' // whitespace to be ignored
			} else {
				it8.ch = 0 // EOF
			}
		}
	} else {
		// In-memory source: NUL-terminated, never advances past the NUL.
		if fs.pos < len(fs.data) {
			it8.ch = int(fs.data[fs.pos])
			if it8.ch != 0 {
				fs.pos++
			}
		} else {
			it8.ch = 0
		}
	}
}

// binSrchKeyIn mirrors BinSrchKey over an explicit keyword table.
func binSrchKeyIn(id string, keys []cgatsKeyword) symbol {
	l, r := 1, len(keys)
	for r >= l {
		x := (l + r) / 2
		res := cgatsStrcasecmp(id, keys[x-1].id)
		if res == 0 {
			return keys[x-1].sy
		}
		if res < 0 {
			r = x - 1
		} else {
			l = x + 1
		}
	}
	return symUndefined
}

// binSrchKey mirrors BinSrchKey over the (sorted) IT8 keyword table.
func binSrchKey(id string) symbol { return binSrchKeyIn(id, tabKeysIT8) }

func xpow10(n int) float64 { return math.Pow(10, float64(n)) }

// readReal reads a real number continuing from an already-consumed integer part
// (the C ReadReal). Numeric parity with C is preserved (same accumulation and
// pow-of-ten scaling).
func (it8 *IT8) readReal(inum int32) {
	it8.dnum = float64(inum)

	for cgatsIsDigit(it8.ch) {
		it8.dnum = it8.dnum*10.0 + float64(it8.ch-'0')
		it8.nextCh()
	}

	if it8.ch == '.' { // decimal point
		frac := 0.0
		prec := 0
		it8.nextCh()
		for cgatsIsDigit(it8.ch) {
			frac = frac*10.0 + float64(it8.ch-'0')
			prec++
			it8.nextCh()
		}
		it8.dnum = it8.dnum + frac/xpow10(prec)
	}

	if cgatsToUpper(it8.ch) == 'E' {
		it8.nextCh()
		sgn := 1
		if it8.ch == '-' {
			sgn = -1
			it8.nextCh()
		} else if it8.ch == '+' {
			sgn = 1
			it8.nextCh()
		}
		e := int32(0)
		for cgatsIsDigit(it8.ch) {
			digit := int32(it8.ch - '0')
			if float64(e)*10.0+float64(digit) < 2147483647.0 {
				e = e*10 + digit
			}
			it8.nextCh()
		}
		e = int32(sgn) * e
		it8.dnum = it8.dnum * xpow10(int(e))
	}
}

// parseFloatNumber mirrors ParseFloatNumber: a locale-independent atof used by
// the property/data doubles getters. '.' is always the decimal separator.
func parseFloatNumber(buffer string) float64 {
	i := 0
	n := len(buffer)
	dnum := 0.0
	sign := 1

	if i < n && (buffer[i] == '-' || buffer[i] == '+') {
		if buffer[i] == '-' {
			sign = -1
		}
		i++
	}

	for i < n && cgatsIsDigit(int(buffer[i])) {
		dnum = dnum*10.0 + float64(buffer[i]-'0')
		i++
	}

	if i < n && buffer[i] == '.' {
		frac := 0.0
		prec := 0
		i++
		for i < n && cgatsIsDigit(int(buffer[i])) {
			frac = frac*10.0 + float64(buffer[i]-'0')
			prec++
			i++
		}
		dnum = dnum + frac/xpow10(prec)
	}

	if i < n && cgatsToUpper(int(buffer[i])) == 'E' {
		i++
		sgn := 1
		if i < n && buffer[i] == '-' {
			sgn = -1
			i++
		} else if i < n && buffer[i] == '+' {
			sgn = 1
			i++
		}
		e := 0
		for i < n && cgatsIsDigit(int(buffer[i])) {
			digit := int(buffer[i] - '0')
			if float64(e)*10.0+float64(digit) < 2147483647.0 {
				e = e*10 + digit
			}
			i++
		}
		e = sgn * e
		dnum = dnum * xpow10(e)
	}

	return float64(sign) * dnum
}

// inStringSymbol reads a quoted string (the C InStringSymbol).
func (it8 *IT8) inStringSymbol() {
	for cgatsIsSeparator(it8.ch) {
		it8.nextCh()
	}

	if it8.ch == '\'' || it8.ch == '"' {
		sng := it8.ch
		it8.str.clear()
		it8.nextCh()
		for it8.ch != sng {
			if it8.ch == '\n' || it8.ch == '\r' || it8.ch == 0 {
				break
			}
			it8.str.append(byte(it8.ch))
			it8.nextCh()
		}
		it8.sy = symString
		it8.nextCh()
	} else {
		it8.synError("String expected")
	}
}

// inSymbol reads the next token (the C InSymbol), including the .INCLUDE token.
func (it8 *IT8) inSymbol() {
	for {
		for cgatsIsSeparator(it8.ch) {
			it8.nextCh()
		}

		if cgatsIsFirstIDChar(it8.ch) { // identifier
			it8.id.clear()
			for {
				it8.id.append(byte(it8.ch))
				it8.nextCh()
				if !cgatsIsIDChar(it8.ch) {
					break
				}
			}
			var key symbol
			if it8.isCUBE {
				key = binSrchKeyIn(it8.id.String(), tabKeysCUBE)
			} else {
				key = binSrchKey(it8.id.String())
			}
			if key == symUndefined {
				it8.sy = symIdent
			} else {
				it8.sy = key
			}
		} else if cgatsIsDigit(it8.ch) || it8.ch == '.' || it8.ch == '-' || it8.ch == '+' {
			sign := int32(1)
			if it8.ch == '-' {
				sign = -1
				it8.nextCh()
			} else if it8.ch == '+' {
				sign = 1
				it8.nextCh()
			}

			it8.inum = 0
			it8.sy = symInum

			if it8.ch == '0' { // 0xnnnn (hex) or 0bnnnn (binary)
				it8.nextCh()
				if cgatsToUpper(it8.ch) == 'X' {
					it8.nextCh()
					for cgatsIsXDigit(it8.ch) {
						it8.ch = cgatsToUpper(it8.ch)
						var j int32
						if it8.ch >= 'A' && it8.ch <= 'F' {
							j = int32(it8.ch - 'A' + 10)
						} else {
							j = int32(it8.ch - '0')
						}
						if float64(it8.inum)*16.0+float64(j) > 2147483647.0 {
							it8.synError("Invalid hexadecimal number")
							return
						}
						it8.inum = it8.inum*16 + j
						it8.nextCh()
					}
					return
				}
				if cgatsToUpper(it8.ch) == 'B' { // binary
					it8.nextCh()
					for it8.ch == '0' || it8.ch == '1' {
						j := int32(it8.ch - '0')
						if float64(it8.inum)*2.0+float64(j) > 2147483647.0 {
							it8.synError("Invalid binary number")
							return
						}
						it8.inum = it8.inum*2 + j
						it8.nextCh()
					}
					return
				}
			}

			for cgatsIsDigit(it8.ch) {
				digit := int32(it8.ch - '0')
				if float64(it8.inum)*10.0+float64(digit) > 2147483647.0 {
					it8.readReal(it8.inum)
					it8.sy = symDnum
					it8.dnum *= float64(sign)
					return
				}
				it8.inum = it8.inum*10 + digit
				it8.nextCh()
			}

			if it8.ch == '.' {
				it8.readReal(it8.inum)
				it8.sy = symDnum
				it8.dnum *= float64(sign)
				return
			}

			it8.inum *= sign

			// Numbers followed by letters are taken as identifiers.
			if cgatsIsIDChar(it8.ch) {
				var buffer string
				if it8.sy == symInum {
					buffer = fmt.Sprintf("%d", it8.inum)
				} else {
					buffer = it8.formatDbl(it8.dnum)
				}
				it8.id.clear()
				it8.id.cat(buffer)
				for {
					it8.id.append(byte(it8.ch))
					it8.nextCh()
					if !cgatsIsIDChar(it8.ch) {
						break
					}
				}
				it8.sy = symIdent
			}
			return
		} else {
			switch it8.ch {
			case 0x1a, 0, -1: // EOF markers
				it8.sy = symEof
			case '\r':
				it8.nextCh()
				if it8.ch == '\n' {
					it8.nextCh()
				}
				it8.sy = symEoln
				it8.lineno++
			case '\n':
				it8.nextCh()
				it8.sy = symEoln
				it8.lineno++
			case '#':
				it8.nextCh()
				for it8.ch != 0 && it8.ch != '\n' && it8.ch != '\r' {
					it8.nextCh()
				}
				it8.sy = symComment
			case '\'', '"':
				it8.inStringSymbol()
			default:
				it8.synError("Unrecognized character: 0x%x", it8.ch)
				return
			}
		}

		if it8.sy != symComment {
			break
		}
	}

	// Handle the include special token.
	if it8.sy == symInclude {
		if it8.includeSP >= cgatsMaxInclude-1 {
			it8.synError("Too many recursion levels")
			return
		}
		it8.inStringSymbol()
		if !it8.check(symString, "Filename expected") {
			return
		}
		fileName, ok := cgatsBuildAbsolutePath(it8.str.String(), it8.fileStack[it8.includeSP].name)
		if !ok {
			it8.synError("File path too long")
			return
		}
		data, err := os.ReadFile(fileName)
		if err != nil {
			it8.synError("File %s not found", fileName)
			return
		}
		it8.includeSP++
		it8.fileStack[it8.includeSP] = &cgatsStream{name: fileName, data: data}
		it8.ch = ' '
		it8.inSymbol()
	}
}

// checkEOLN consumes an end-of-line separator run (the C CheckEOLN).
func (it8 *IT8) checkEOLN() bool {
	if !it8.check(symEoln, "Expected separator") {
		return false
	}
	for it8.sy == symEoln {
		it8.inSymbol()
	}
	return true
}

func (it8 *IT8) skip(sy symbol) {
	if it8.sy == sy && it8.sy != symEof && it8.sy != symSynError {
		it8.inSymbol()
	}
}

func (it8 *IT8) skipEOLN() {
	for it8.sy == symEoln {
		it8.inSymbol()
	}
}

// getVal returns the current token as text, truncated to max bytes, mirroring
// GetVal's per-symbol formatting.
func (it8 *IT8) getVal(max int, errTitle string) (string, bool) {
	var s string
	switch it8.sy {
	case symEoln:
		s = ""
	case symIdent:
		s = it8.id.String()
	case symInum:
		s = fmt.Sprintf("%d", it8.inum)
	case symDnum:
		s = it8.formatDbl(it8.dnum)
	case symString:
		s = it8.str.String()
	default:
		return "", it8.synError("%s", errTitle)
	}
	// The reference caps content at max-1 characters (strncpy(buf, src, max) with
	// buf[max-1]=0, or snprintf(buf, max, ...)), reserving one byte for the NUL.
	if max > 0 && len(s) >= max {
		s = s[:max-1]
	}
	return s, true
}

// formatDbl renders a float64 with the current DoubleFormatter (a C printf
// format that is also a valid Go float verb for the shapes lcms uses).
func (it8 *IT8) formatDbl(v float64) string {
	return fmt.Sprintf(it8.doubleFormatter, v)
}

// ------------------------------------------------------ table access

// getTable returns the current table, raising an out-of-sequence error and
// falling back to table 0 (the C GetTable).
func (it8 *IT8) getTable() *table {
	if it8.nTable >= it8.tablesCount {
		it8.synError("Table %d out of sequence", it8.nTable)
		return it8.tab[0]
	}
	return it8.tab[it8.nTable]
}

// ------------------------------------------------------ property list

// isAvailableOnList searches a KEYVALUE list (the C IsAvailableOnList). It
// returns whether a match was found and the last node visited (the C LastPtr).
// A nil subkey means "match on keyword only".
func isAvailableOnList(head *keyValue, key string, subkey *string) (found bool, last *keyValue) {
	p := head
	last = p
	for ; p != nil; p = p.next {
		last = p
		if len(key) == 0 || key[0] != '#' { // comments are ignored
			if cgatsStrcasecmp(key, p.keyword) == 0 {
				break
			}
		}
	}
	if p == nil {
		return false, last
	}
	if subkey == nil {
		return true, last
	}
	for ; p != nil; p = p.nextSubkey {
		if p.subkey == nil {
			continue
		}
		last = p
		if cgatsStrcasecmp(*subkey, *p.subkey) == 0 {
			return true, last
		}
	}
	return false, last
}

// addToList adds/updates a property (the C AddToList). The linked-list wiring
// is preserved exactly so the writer emits properties in C order.
func (it8 *IT8) addToList(head **keyValue, key string, subkey *string, xValue *string, writeAs writeMode) *keyValue {
	found, last := isAvailableOnList(*head, key, subkey)
	var p *keyValue
	if found {
		p = last
		if cgatsStrcasecmp(key, "NUMBER_OF_FIELDS") == 0 || cgatsStrcasecmp(key, "NUMBER_OF_SETS") == 0 {
			it8.synError("duplicate key <%s>", key)
			return nil
		}
	} else {
		p = &keyValue{keyword: key}
		if subkey != nil {
			sk := *subkey
			p.subkey = &sk
		}
		if *head == nil {
			*head = p
		} else {
			if subkey != nil && last != nil {
				last.nextSubkey = p
				for last.next != nil {
					last = last.next
				}
			}
			if last != nil {
				last.next = p
			}
		}
	}

	p.writeAs = writeAs
	if xValue != nil {
		v := *xValue
		p.value = &v
	} else {
		p.value = nil
	}
	return p
}

func (it8 *IT8) addAvailableProperty(key string, as writeMode) *keyValue {
	return it8.addToList(&it8.validKeywords, key, nil, nil, as)
}

func (it8 *IT8) addAvailableSampleID(key string) *keyValue {
	return it8.addToList(&it8.validSampleID, key, nil, nil, writeUncooked)
}

// allocTable appends a fresh empty table (the C AllocTable).
func (it8 *IT8) allocTable() bool {
	if it8.tablesCount >= cgatsMaxTables-1 {
		return false
	}
	it8.tab = append(it8.tab, &table{})
	it8.tablesCount++
	return true
}

// ------------------------------------------------------ public: allocation, tables, sheet type

// IT8Alloc creates an empty IT8 container (the C cmsIT8Alloc). A nil context
// uses the default context.
func IT8Alloc(ctx *Context) *IT8 {
	if ctx == nil {
		ctx = defaultContext
	}
	it8 := &IT8{
		ctx:             ctx,
		doubleFormatter: cgatsDefaultDblFormat,
		lineno:          1,
		ch:              ' ',
		sy:              symUndefined,
	}
	it8.id = &strbuf{}
	it8.str = &strbuf{}
	it8.fileStack = make([]*cgatsStream, cgatsMaxInclude)
	it8.fileStack[0] = &cgatsStream{}

	it8.allocTable()
	it8.SetSheetType("CGATS.17")

	for _, p := range predefinedProperties {
		it8.addAvailableProperty(p.id, p.as)
	}
	for _, s := range predefinedSampleID {
		it8.addAvailableSampleID(s)
	}
	return it8
}

// Free releases the container. Under the Go GC nothing is freed; it exists for
// symmetry with cmsIT8Free and is safe to call on nil.
func (it8 *IT8) Free() {}

// SetTable selects (and, when nTable == TablesCount, appends) a table,
// returning the table index or -1 on error (the C cmsIT8SetTable).
func (it8 *IT8) SetTable(nTable uint32) int {
	if nTable >= it8.tablesCount {
		if nTable == it8.tablesCount {
			if !it8.allocTable() {
				it8.synError("Too many tables")
				return -1
			}
		} else {
			it8.synError("Table %d is out of sequence", nTable)
			return -1
		}
	}
	it8.nTable = nTable
	return int(nTable)
}

// TableCount returns the number of tables (the C cmsIT8TableCount).
func (it8 *IT8) TableCount() uint32 { return it8.tablesCount }

// GetSheetType returns the current table's sheet type (the C cmsIT8GetSheetType).
func (it8 *IT8) GetSheetType() string { return it8.getTable().sheetType }

// SetSheetType sets the current table's sheet type (the C cmsIT8SetSheetType).
func (it8 *IT8) SetSheetType(typ string) bool {
	t := it8.getTable()
	if len(typ) > cgatsMaxStr-1 {
		typ = typ[:cgatsMaxStr-1]
	}
	t.sheetType = typ
	return true
}

// SetComment adds a comment line to the current table (the C cmsIT8SetComment).
func (it8 *IT8) SetComment(val string) bool {
	if val == "" {
		return false
	}
	return it8.addToList(&it8.getTable().headerList, "# ", nil, &val, writeUncooked) != nil
}

// ------------------------------------------------------ public: properties

// SetPropertyStr sets a string property (the C cmsIT8SetPropertyStr).
func (it8 *IT8) SetPropertyStr(key, val string) bool {
	if val == "" {
		return false
	}
	return it8.addToList(&it8.getTable().headerList, key, nil, &val, writeStringify) != nil
}

// SetPropertyDbl sets a numeric property formatted with the double formatter
// (the C cmsIT8SetPropertyDbl).
func (it8 *IT8) SetPropertyDbl(prop string, val float64) bool {
	buf := it8.formatDbl(val)
	return it8.addToList(&it8.getTable().headerList, prop, nil, &buf, writeUncooked) != nil
}

// SetPropertyHex sets a hexadecimal property (the C cmsIT8SetPropertyHex). The
// value is stored decimal and re-rendered as 0xNN on save.
func (it8 *IT8) SetPropertyHex(prop string, val uint32) bool {
	buf := fmt.Sprintf("%d", val)
	return it8.addToList(&it8.getTable().headerList, prop, nil, &buf, writeHexadecimal) != nil
}

// SetPropertyUncooked sets a verbatim property (the C cmsIT8SetPropertyUncooked).
func (it8 *IT8) SetPropertyUncooked(key, buffer string) bool {
	return it8.addToList(&it8.getTable().headerList, key, nil, &buffer, writeUncooked) != nil
}

// SetPropertyMulti sets a subkey/value pair property (the C cmsIT8SetPropertyMulti).
func (it8 *IT8) SetPropertyMulti(key, subKey, buffer string) bool {
	return it8.addToList(&it8.getTable().headerList, key, &subKey, &buffer, writePair) != nil
}

// GetProperty returns a property value and whether it exists (the C
// cmsIT8GetProperty, which returns NULL when absent).
func (it8 *IT8) GetProperty(key string) (string, bool) {
	found, p := isAvailableOnList(it8.getTable().headerList, key, nil)
	if found && p != nil && p.value != nil {
		return *p.value, true
	}
	return "", false
}

// GetPropertyDbl returns a property parsed as a double, or 0 when absent (the C
// cmsIT8GetPropertyDbl).
func (it8 *IT8) GetPropertyDbl(prop string) float64 {
	v, ok := it8.GetProperty(prop)
	if !ok {
		return 0.0
	}
	return parseFloatNumber(v)
}

// GetPropertyMulti returns a subkey value and whether it exists (the C
// cmsIT8GetPropertyMulti).
func (it8 *IT8) GetPropertyMulti(key, subKey string) (string, bool) {
	found, p := isAvailableOnList(it8.getTable().headerList, key, &subKey)
	if found && p != nil && p.value != nil {
		return *p.value, true
	}
	return "", false
}

// EnumProperties returns the property names of the current table in order (the
// C cmsIT8EnumProperties).
func (it8 *IT8) EnumProperties() []string {
	t := it8.getTable()
	var out []string
	for p := t.headerList; p != nil; p = p.next {
		out = append(out, p.keyword)
	}
	return out
}

// EnumPropertyMulti returns the subkey names of a multi property (the C
// cmsIT8EnumPropertyMulti). It mirrors the C quirk of reporting the head node's
// subkey for each subkey link.
func (it8 *IT8) EnumPropertyMulti(prop string) []string {
	t := it8.getTable()
	found, p := isAvailableOnList(t.headerList, prop, nil)
	if !found {
		return nil
	}
	var out []string
	for tmp := p; tmp != nil; tmp = tmp.nextSubkey {
		if tmp.subkey != nil {
			if p.subkey != nil {
				out = append(out, *p.subkey)
			} else {
				out = append(out, "")
			}
		}
	}
	return out
}

// ------------------------------------------------------ datasets

// satoi mirrors the C satoi: atoi with saturation to ±0x7ffffff0.
func satoi(b *string) int32 {
	if b == nil {
		return 0
	}
	n := cAtoi(*b)
	if n > 0x7ffffff0 {
		return 0x7ffffff0
	}
	if n < -0x7ffffff0 {
		return -0x7ffffff0
	}
	return int32(n)
}

// cAtoi mirrors C atoi: optional leading whitespace, optional sign, decimal
// digits; stops at the first non-digit; saturates at int32 range on overflow.
func cAtoi(s string) int64 {
	i := 0
	n := len(s)
	for i < n && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\v' || s[i] == '\f') {
		i++
	}
	sign := int64(1)
	if i < n && (s[i] == '+' || s[i] == '-') {
		if s[i] == '-' {
			sign = -1
		}
		i++
	}
	var v int64
	for i < n && s[i] >= '0' && s[i] <= '9' {
		v = v*10 + int64(s[i]-'0')
		if v > 0x7fffffff {
			v = 0x7fffffff // saturate (glibc atoi overflow is UB; keep it bounded)
		}
		i++
	}
	return sign * v
}

// getPropertyPtr is the internal *string form used where NULL vs "" matters.
func (it8 *IT8) getPropertyPtr(key string) *string {
	found, p := isAvailableOnList(it8.getTable().headerList, key, nil)
	if found && p != nil {
		return p.value
	}
	return nil
}

// allocateDataFormat allocates the DataFormat column array, enforcing the
// NUMBER_OF_FIELDS bounds (the C AllocateDataFormat).
func (it8 *IT8) allocateDataFormat() bool {
	t := it8.getTable()
	if t.dataFormat != nil {
		return true // already allocated
	}
	t.nSamples = int(satoi(it8.getPropertyPtr("NUMBER_OF_FIELDS")))
	if t.nSamples <= 0 || t.nSamples > 0x7ffe {
		it8.synError("Wrong NUMBER_OF_FIELDS")
		return false
	}
	t.dataFormat = make([]*string, t.nSamples+1)
	return true
}

func (it8 *IT8) getDataFormat(n int) *string {
	t := it8.getTable()
	if t.dataFormat != nil && n >= 0 && n < len(t.dataFormat) {
		return t.dataFormat[n]
	}
	return nil
}

// setDataFormat sets a column label (the C SetDataFormat).
func (it8 *IT8) setDataFormat(n int, label string) bool {
	t := it8.getTable()
	if t.dataFormat == nil {
		if !it8.allocateDataFormat() {
			return false
		}
	}
	if n < 0 || n >= t.nSamples {
		it8.synError("Invalid or more than NUMBER_OF_FIELDS fields.")
		return false
	}
	if t.dataFormat != nil && n < len(t.dataFormat) {
		l := label
		t.dataFormat[n] = &l
	}
	return true
}

// SetDataFormat sets the label of column n (the C cmsIT8SetDataFormat).
func (it8 *IT8) SetDataFormat(n int, sample string) bool {
	if n < 0 {
		return false
	}
	return it8.setDataFormat(n, sample)
}

// satob mirrors the C satob: render a value's magnitude in binary.
func satob(v *string) string {
	if v == nil {
		return "0"
	}
	x := uint32(cAtoi(*v))
	if x == 0 {
		return "0"
	}
	var buf [33]byte
	i := len(buf)
	for x != 0 {
		i--
		buf[i] = byte('0' + x%2)
		x /= 2
	}
	return string(buf[i:])
}

// allocateDataSet allocates the data grid, enforcing the count/product bounds
// that guard against oversized allocations (the C AllocateDataSet).
func (it8 *IT8) allocateDataSet() bool {
	t := it8.getTable()
	if t.data != nil {
		return true
	}
	t.nSamples = int(satoi(it8.getPropertyPtr("NUMBER_OF_FIELDS")))
	t.nPatches = int(satoi(it8.getPropertyPtr("NUMBER_OF_SETS")))

	if t.nSamples < 0 || t.nSamples > 0x7ffe ||
		t.nPatches < 0 || t.nPatches > 0x7ffe ||
		(t.nPatches*t.nSamples) > 200000 {
		it8.synError("AllocateDataSet: too much data")
		return false
	}
	t.data = make([]*string, (t.nSamples+1)*(t.nPatches+1))
	return true
}

// getData reads a cell (the C GetData); nil when out of range or unset.
func (it8 *IT8) getData(nSet, nField int) *string {
	t := it8.getTable()
	nSamples := t.nSamples
	nPatches := t.nPatches
	if nSet < 0 || nSet >= nPatches || nField < 0 || nField >= nSamples {
		return nil
	}
	if t.data == nil {
		return nil
	}
	idx := nSet*nSamples + nField
	if idx < 0 || idx >= len(t.data) {
		return nil
	}
	return t.data[idx]
}

// setData writes a cell (the C SetData), replicating its off-by-one range
// checks and the +1 slack sizing that keeps the index in bounds.
func (it8 *IT8) setData(nSet, nField int, val string) bool {
	t := it8.getTable()
	if t.data == nil {
		if !it8.allocateDataSet() {
			return false
		}
	}
	if t.data == nil {
		return false
	}
	if nSet > t.nPatches || nSet < 0 {
		return it8.synError("Patch %d out of range, there are %d patches", nSet, t.nPatches)
	}
	if nField > t.nSamples || nField < 0 {
		return it8.synError("Sample %d out of range, there are %d samples", nField, t.nSamples)
	}
	idx := nSet*t.nSamples + nField
	if idx < 0 || idx >= len(t.data) {
		return it8.synError("Sample %d out of range, there are %d samples", nField, t.nSamples)
	}
	v := val
	t.data[idx] = &v
	return true
}

// ------------------------------------------------------ writer

// saveStream accumulates the serialized output.
type saveStream struct {
	buf []byte
}

func (s *saveStream) writeStr(str string) { s.buf = append(s.buf, str...) }

// writeStrOrSpace writes a *string, substituting a single space for nil (the C
// WriteStr NULL -> " " behaviour used by WriteDataFormat).
func (s *saveStream) writeStrOrSpace(str *string) {
	if str == nil {
		s.buf = append(s.buf, ' ')
		return
	}
	s.buf = append(s.buf, *str...)
}

func (s *saveStream) writef(format string, args ...any) {
	s.buf = append(s.buf, fmt.Sprintf(format, args...)...)
}

// writeHeader serializes the property list (the C WriteHeader).
func (it8 *IT8) writeHeader(fp *saveStream) {
	t := it8.getTable()

	fp.writeStr(t.sheetType)
	fp.writeStr("\n")

	for p := t.headerList; p != nil; p = p.next {
		if len(p.keyword) > 0 && p.keyword[0] == '#' {
			fp.writeStr("#\n# ")
			if p.value != nil {
				for i := 0; i < len(*p.value); i++ {
					c := (*p.value)[i]
					fp.writef("%c", c)
					if c == '\n' {
						fp.writeStr("# ")
					}
				}
			}
			fp.writeStr("\n#\n")
			continue
		}

		if found, _ := isAvailableOnList(it8.validKeywords, p.keyword, nil); !found {
			it8.addAvailableProperty(p.keyword, writeUncooked)
		}

		fp.writeStr(p.keyword)
		if p.value != nil {
			switch p.writeAs {
			case writeUncooked:
				fp.writef("\t%s", *p.value)
			case writeStringify:
				fp.writef("\t\"%s\"", *p.value)
			case writeHexadecimal:
				fp.writef("\t0x%X", uint32(satoi(p.value)))
			case writeBinary:
				fp.writef("\t0b%s", satob(p.value))
			case writePair:
				sub := ""
				if p.subkey != nil {
					sub = *p.subkey
				}
				fp.writef("\t\"%s,%s\"", sub, *p.value)
			default:
				it8.synError("Unknown write mode %d", p.writeAs)
				return
			}
		}
		fp.writeStr("\n")
	}
}

// writeDataFormat serializes BEGIN_DATA_FORMAT..END_DATA_FORMAT (the C
// WriteDataFormat).
func (it8 *IT8) writeDataFormat(fp *saveStream) {
	t := it8.getTable()
	if t.dataFormat == nil {
		return
	}
	fp.writeStr("BEGIN_DATA_FORMAT\n")
	fp.writeStr(" ")
	nSamples := int(satoi(it8.getPropertyPtr("NUMBER_OF_FIELDS")))
	if nSamples <= t.nSamples {
		for i := 0; i < nSamples; i++ {
			fp.writeStrOrSpace(t.dataFormat[i])
			if i == nSamples-1 {
				fp.writeStr("\n")
			} else {
				fp.writeStr("\t")
			}
		}
	}
	fp.writeStr("END_DATA_FORMAT\n")
}

// writeData serializes BEGIN_DATA..END_DATA (the C WriteData).
func (it8 *IT8) writeData(fp *saveStream) {
	t := it8.getTable()
	if t.data == nil {
		return
	}
	fp.writeStr("BEGIN_DATA\n")
	nPatches := int(satoi(it8.getPropertyPtr("NUMBER_OF_SETS")))
	if nPatches <= t.nPatches {
		for i := 0; i < nPatches; i++ {
			fp.writeStr(" ")
			for j := 0; j < t.nSamples; j++ {
				idx := i*t.nSamples + j
				var ptr *string
				if idx >= 0 && idx < len(t.data) {
					ptr = t.data[idx]
				}
				if ptr == nil {
					fp.writeStr("\"\"")
				} else {
					if strings.IndexByte(*ptr, ' ') >= 0 {
						fp.writeStr("\"")
						fp.writeStr(*ptr)
						fp.writeStr("\"")
					} else {
						fp.writeStr(*ptr)
					}
				}
				if j == t.nSamples-1 {
					fp.writeStr("\n")
				} else {
					fp.writeStr("\t")
				}
			}
		}
	}
	fp.writeStr("END_DATA\n")
}

// SaveToMem serializes the whole sheet to a byte slice, byte-for-byte identical
// to the C cmsIT8SaveToMem text excluding the trailing NUL C appends (which
// equals the C cmsIT8SaveToFile output).
func (it8 *IT8) SaveToMem() ([]byte, error) {
	sd := &saveStream{}
	saved := it8.nTable
	for i := uint32(0); i < it8.tablesCount; i++ {
		it8.SetTable(i)
		it8.writeHeader(sd)
		it8.writeDataFormat(sd)
		it8.writeData(sd)
	}
	it8.nTable = saved
	if it8.parseErr != nil {
		return nil, it8.parseErr
	}
	return sd.buf, nil
}

// SaveToFile writes the sheet to a file (the C cmsIT8SaveToFile), erroring when
// any table lacks its data or data-format section.
func (it8 *IT8) SaveToFile(fileName string) error {
	sd := &saveStream{}
	saved := it8.nTable
	for i := uint32(0); i < it8.tablesCount; i++ {
		if it8.SetTable(i) < 0 {
			it8.nTable = saved
			return it8.errOr("cmsIT8SaveToFile: bad table")
		}
		t := it8.getTable()
		if t.data == nil || t.dataFormat == nil {
			it8.nTable = saved
			return it8.errOr("cmsIT8SaveToFile: incomplete table")
		}
		it8.writeHeader(sd)
		it8.writeDataFormat(sd)
		it8.writeData(sd)
	}
	it8.nTable = saved
	if err := os.WriteFile(fileName, sd.buf, 0o644); err != nil {
		return err
	}
	return nil
}

func (it8 *IT8) errOr(msg string) error {
	if it8.parseErr != nil {
		return it8.parseErr
	}
	return it8.ctx.signalError(ErrCorruptionDetected, "%s", msg)
}

// ------------------------------------------------------ higher-level parsing

// dataFormatSection parses BEGIN_DATA_FORMAT..END_DATA_FORMAT (the C
// DataFormatSection).
func (it8 *IT8) dataFormatSection() bool {
	iField := 0
	t := it8.getTable()

	it8.inSymbol() // eats "BEGIN_DATA_FORMAT"
	it8.checkEOLN()

	for it8.sy != symEndDataFormat && it8.sy != symEoln && it8.sy != symEof && it8.sy != symSynError {
		if it8.sy != symIdent {
			return it8.synError("Sample type expected")
		}
		if !it8.setDataFormat(iField, it8.id.String()) {
			return false
		}
		iField++
		it8.inSymbol()
		it8.skipEOLN()
	}

	it8.skipEOLN()
	it8.skip(symEndDataFormat)
	it8.skipEOLN()

	if iField != t.nSamples {
		it8.synError("Count mismatch. NUMBER_OF_FIELDS was %d, found %d\n", t.nSamples, iField)
	}
	return true
}

// dataSection parses BEGIN_DATA..END_DATA (the C DataSection).
func (it8 *IT8) dataSection() bool {
	iField := 0
	iSet := 0
	t := it8.getTable()

	it8.inSymbol() // eats "BEGIN_DATA"
	it8.checkEOLN()

	if t.data == nil {
		if !it8.allocateDataSet() {
			return false
		}
	}

	for it8.sy != symEndData && it8.sy != symEof && it8.sy != symSynError {
		if iField >= t.nSamples {
			iField = 0
			iSet++
		}

		if it8.sy != symEndData && it8.sy != symEof && it8.sy != symSynError {
			switch it8.sy {
			case symIdent:
				if !it8.setData(iSet, iField, it8.id.String()) {
					return false
				}
			case symString:
				if !it8.setData(iSet, iField, it8.str.String()) {
					return false
				}
			default:
				buf, ok := it8.getVal(255, "Sample data expected")
				if !ok {
					return false
				}
				if !it8.setData(iSet, iField, buf) {
					return false
				}
			}
			iField++
			it8.inSymbol()
			it8.skipEOLN()
		}
	}

	it8.skipEOLN()
	it8.skip(symEndData)
	it8.skipEOLN()

	if (iSet + 1) != t.nPatches {
		return it8.synError("Count mismatch. NUMBER_OF_SETS was %d, found %d\n", t.nPatches, iSet+1)
	}
	return true
}

// headerSection parses the header properties (the C HeaderSection).
func (it8 *IT8) headerSection() bool {
	for it8.sy != symEof && it8.sy != symSynError &&
		it8.sy != symBeginDataFormat && it8.sy != symBeginData {

		switch it8.sy {
		case symKeyword:
			it8.inSymbol()
			buf, ok := it8.getVal(cgatsMaxStr-1, "Keyword expected")
			if !ok {
				return false
			}
			if it8.addAvailableProperty(buf, writeUncooked) == nil {
				return false
			}
			it8.inSymbol()

		case symDataFormatID:
			it8.inSymbol()
			buf, ok := it8.getVal(cgatsMaxStr-1, "Keyword expected")
			if !ok {
				return false
			}
			if it8.addAvailableSampleID(buf) == nil {
				return false
			}
			it8.inSymbol()

		case symIdent:
			varName := it8.id.String()
			if len(varName) > cgatsMaxID-1 {
				varName = varName[:cgatsMaxID-1]
			}
			found, key := isAvailableOnList(it8.validKeywords, varName, nil)
			if !found {
				key = it8.addAvailableProperty(varName, writeUncooked)
				if key == nil {
					return false
				}
			}

			it8.inSymbol()
			buf, ok := it8.getVal(cgatsMaxStr-1, "Property data expected")
			if !ok {
				return false
			}

			if key.writeAs != writePair {
				wm := writeUncooked
				if it8.sy == symString {
					wm = writeStringify
				}
				if it8.addToList(&it8.getTable().headerList, varName, nil, &buf, wm) == nil {
					return false
				}
			} else {
				if it8.sy != symString {
					return it8.synError("Invalid value '%s' for property '%s'.", buf, varName)
				}
				if !it8.parseHeaderPairs(varName, buf) {
					return false
				}
			}
			it8.inSymbol()

		case symEoln:
			// nothing

		default:
			return it8.synError("expected keyword or identifier")
		}

		it8.skipEOLN()
	}
	return true
}

// parseHeaderPairs splits a WRITE_PAIR value ("subkey,value;subkey,value")
// mirroring the in-place chopping walk in HeaderSection's SPAIR branch: one
// segment per ';', including the final ';'-less segment; a trailing ';' yields
// an empty final segment and thus an error, exactly as the C loop does.
func (it8 *IT8) parseHeaderPairs(varName, buffer string) bool {
	s := buffer
	active := true // C: Subkey != NULL
	for active {
		var segment string
		if idx := strings.IndexByte(s, ';'); idx >= 0 {
			segment = s[:idx]
			s = s[idx+1:]
		} else {
			segment = s
			active = false
		}

		// Split subkey and value at the LAST comma (C uses strrchr).
		commaIdx := strings.LastIndexByte(segment, ',')
		if commaIdx < 0 {
			return it8.synError("Invalid value for property '%s'.", varName)
		}
		subkey := strings.TrimRight(segment[:commaIdx], " ")
		value := strings.TrimRight(segment[commaIdx+1:], " ")
		subkey = strings.TrimLeft(subkey, " ")
		value = strings.TrimLeft(value, " ")

		if subkey == "" || value == "" {
			return it8.synError("Invalid value for property '%s'.", varName)
		}
		sk := subkey
		it8.addToList(&it8.getTable().headerList, varName, &sk, &value, writePair)
	}
	return true
}

// readType reads the first line as the sheet type (the C ReadType).
func (it8 *IT8) readType() string {
	var b []byte
	cnt := 0
	for cgatsIsSeparator(it8.ch) {
		it8.nextCh()
	}
	for it8.ch != '\r' && it8.ch != '\n' && it8.ch != '\t' && it8.ch != 0 {
		if cnt < cgatsMaxStr {
			b = append(b, byte(it8.ch))
			cnt++
		}
		it8.nextCh()
	}
	return string(b)
}

// parseIT8 is the top-level grammar driver (the C ParseIT8).
func (it8 *IT8) parseIT8(nosheet bool) bool {
	if !nosheet {
		it8.tab[0].sheetType = it8.readType()
	}

	it8.inSymbol()
	it8.skipEOLN()

	for it8.sy != symEof && it8.sy != symSynError {
		switch it8.sy {
		case symBeginDataFormat:
			if !it8.dataFormatSection() {
				return false
			}
		case symBeginData:
			if !it8.dataSection() {
				return false
			}
			if it8.sy != symEof && it8.sy != symSynError {
				if !it8.allocTable() {
					return false
				}
				it8.nTable = it8.tablesCount - 1

				if !nosheet {
					if it8.sy == symIdent {
						for cgatsIsSeparator(it8.ch) {
							it8.nextCh()
						}
						if it8.ch == '\n' || it8.ch == '\r' {
							it8.SetSheetType(it8.id.String())
							it8.inSymbol()
						} else {
							it8.SetSheetType("")
						}
					} else if it8.sy == symString {
						it8.SetSheetType(it8.str.String())
						it8.inSymbol()
					}
				}
			}
		case symEoln:
			it8.skipEOLN()
		default:
			if !it8.headerSection() {
				return false
			}
		}
	}

	return it8.sy != symSynError
}

// cookPointers resolves the LABEL table-reference extension (the C CookPointers).
func (it8 *IT8) cookPointers() {
	nOldTable := it8.nTable

	for j := uint32(0); j < it8.tablesCount; j++ {
		t := it8.tab[j]
		t.sampleID = 0
		it8.nTable = j

		for idField := 0; idField < t.nSamples; idField++ {
			if t.dataFormat == nil {
				it8.synError("Undefined DATA_FORMAT")
				return
			}
			fldPtr := it8.getDataFormat(idField)
			if fldPtr == nil {
				continue
			}
			fld := *fldPtr

			if cgatsStrcasecmp(fld, "SAMPLE_ID") == 0 {
				t.sampleID = idField
			}

			if cgatsStrcasecmp(fld, "LABEL") == 0 || (len(fld) > 0 && fld[0] == '$') {
				for i := 0; i < t.nPatches; i++ {
					labelPtr := it8.getData(i, idField)
					if labelPtr == nil {
						continue
					}
					label := *labelPtr
					for k := uint32(0); k < it8.tablesCount; k++ {
						tbl := it8.tab[k]
						if found, p := isAvailableOnList(tbl.headerList, label, nil); found && p != nil {
							typ := ""
							if p.value != nil {
								typ = *p.value
							}
							buf := fmt.Sprintf("%s %d %s", label, int(k), typ)
							if len(buf) > 255 {
								buf = buf[:255]
							}
							it8.setData(i, idField, buf)
						}
					}
				}
			}
		}
	}
	it8.nTable = nOldTable
}

// isMyBlock guesses whether a buffer looks like a CGATS/IT8 stream, returning
// the word count of the first line (1 or 2) or 0 if not recognized (the C
// IsMyBlock).
func isMyBlock(buffer []byte) int {
	words := 1
	space := 0
	quot := 0
	n := len(buffer)

	if n < 10 {
		return 0
	}
	if n > 132 {
		n = 132
	}

	for i := 1; i < n; i++ {
		switch buffer[i] {
		case '\n', '\r':
			if quot == 1 || words > 2 {
				return 0
			}
			return words
		case '\t', ' ':
			if quot == 0 && space == 0 {
				space = 1
			}
		case '"':
			if quot == 0 {
				quot = 1
			} else {
				quot = 0
			}
		default:
			if buffer[i] < 32 {
				return 0
			}
			if buffer[i] > 127 {
				return 0
			}
			words += space
			space = 0
		}
	}
	return 0
}

// ------------------------------------------------------ public: load / find / data access

// IT8LoadFromMem parses an IT8/CGATS sheet from memory (the C
// cmsIT8LoadFromMem). It returns an error when the input is not recognized or a
// syntax error is found.
func IT8LoadFromMem(ctx *Context, data []byte) (*IT8, error) {
	if ctx == nil {
		ctx = defaultContext
	}
	if len(data) == 0 {
		return nil, ctx.signalError(ErrCorruptionDetected, "cmsIT8LoadFromMem: empty input")
	}

	typ := isMyBlock(data)
	if typ == 0 {
		return nil, ctx.signalError(ErrCorruptionDetected, "cmsIT8LoadFromMem: not a CGATS/IT8 stream")
	}

	it8 := IT8Alloc(ctx)

	// Mirror C strncpy semantics: the source is truncated at the first NUL.
	src := data
	if i := indexByteInt(data, 0); i >= 0 {
		src = data[:i]
	}
	buf := make([]byte, len(src)+1)
	copy(buf, src)
	// buf[len(src)] stays 0 (the terminator)

	it8.fileStack[0] = &cgatsStream{name: "", data: buf, isMem: true}

	if !it8.parseIT8(typ-1 != 0) {
		if it8.parseErr != nil {
			return nil, it8.parseErr
		}
		return nil, ctx.signalError(ErrCorruptionDetected, "cmsIT8LoadFromMem: parse failed")
	}

	// cmsIT8LoadFromMem signals errors raised while cooking pointers (e.g.
	// "Undefined DATA_FORMAT") but still returns a usable handle; only
	// parse-phase failures (caught above) are fatal. Preserve that: keep the
	// pre-cook error state (nil on a successful parse) across cookPointers, which
	// already logs through signalError.
	preErr := it8.parseErr
	it8.cookPointers()
	it8.nTable = 0
	it8.parseErr = preErr
	if it8.parseErr != nil {
		return nil, it8.parseErr
	}
	return it8, nil
}

func indexByteInt(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// IT8LoadFromFile parses an IT8/CGATS sheet from a file (the C
// cmsIT8LoadFromFile).
func IT8LoadFromFile(ctx *Context, fileName string) (*IT8, error) {
	if ctx == nil {
		ctx = defaultContext
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, ctx.signalError(ErrFile, "File '%s' not found", fileName)
	}

	head := data
	if len(head) > 132 {
		head = head[:132]
	}
	typ := isMyBlock(head)
	if typ == 0 {
		return nil, ctx.signalError(ErrCorruptionDetected, "cmsIT8LoadFromFile: not a CGATS/IT8 stream")
	}

	it8 := IT8Alloc(ctx)
	base := fileName
	if len(base) > cgatsMaxPath-1 {
		base = base[:cgatsMaxPath-1]
	}
	it8.fileStack[0] = &cgatsStream{name: base, data: data, isMem: false}

	if !it8.parseIT8(typ-1 != 0) {
		if it8.parseErr != nil {
			return nil, it8.parseErr
		}
		return nil, ctx.signalError(ErrCorruptionDetected, "cmsIT8LoadFromFile: parse failed")
	}

	// As in IT8LoadFromMem, errors raised during cookPointers are signalled but
	// not fatal; only parse-phase failures fail the load.
	preErr := it8.parseErr
	it8.cookPointers()
	it8.nTable = 0
	it8.parseErr = preErr
	if it8.parseErr != nil {
		return nil, it8.parseErr
	}
	return it8, nil
}

// EnumDataFormat returns the current table's column labels and count (the C
// cmsIT8EnumDataFormat).
func (it8 *IT8) EnumDataFormat() ([]string, int) {
	t := it8.getTable()
	if t.dataFormat == nil {
		return nil, t.nSamples
	}
	out := make([]string, 0, t.nSamples)
	for i := 0; i < t.nSamples && i < len(t.dataFormat); i++ {
		if t.dataFormat[i] != nil {
			out = append(out, *t.dataFormat[i])
		} else {
			out = append(out, "")
		}
	}
	return out, t.nSamples
}

func (it8 *IT8) locatePatch(cPatch string) int {
	t := it8.getTable()
	for i := 0; i < t.nPatches; i++ {
		data := it8.getData(i, t.sampleID)
		if data != nil && cgatsStrcasecmp(*data, cPatch) == 0 {
			return i
		}
	}
	return -1
}

func (it8 *IT8) locateEmptyPatch() int {
	t := it8.getTable()
	for i := 0; i < t.nPatches; i++ {
		if it8.getData(i, t.sampleID) == nil {
			return i
		}
	}
	return -1
}

func (it8 *IT8) locateSample(cSample string) int {
	t := it8.getTable()
	for i := 0; i < t.nSamples; i++ {
		fld := it8.getDataFormat(i)
		if fld != nil && cgatsStrcasecmp(*fld, cSample) == 0 {
			return i
		}
	}
	return -1
}

// FindDataFormat returns the column index of a sample name (the C
// cmsIT8FindDataFormat).
func (it8 *IT8) FindDataFormat(cSample string) int { return it8.locateSample(cSample) }

// GetDataRowCol returns a cell by row/col, with a present flag (the C
// cmsIT8GetDataRowCol, which returns NULL when absent).
func (it8 *IT8) GetDataRowCol(row, col int) (string, bool) {
	p := it8.getData(row, col)
	if p == nil {
		return "", false
	}
	return *p, true
}

// GetDataRowColDbl returns a cell parsed as a double, or 0 (the C
// cmsIT8GetDataRowColDbl).
func (it8 *IT8) GetDataRowColDbl(row, col int) float64 {
	s, ok := it8.GetDataRowCol(row, col)
	if !ok {
		return 0.0
	}
	return parseFloatNumber(s)
}

// SetDataRowCol writes a cell by row/col (the C cmsIT8SetDataRowCol).
func (it8 *IT8) SetDataRowCol(row, col int, val string) bool {
	return it8.setData(row, col, val)
}

// SetDataRowColDbl writes a cell formatted with the double formatter (the C
// cmsIT8SetDataRowColDbl).
func (it8 *IT8) SetDataRowColDbl(row, col int, val float64) bool {
	return it8.setData(row, col, it8.formatDbl(val))
}

// GetData returns a cell by patch+sample name, with a present flag (the C
// cmsIT8GetData).
func (it8 *IT8) GetData(cPatch, cSample string) (string, bool) {
	iField := it8.locateSample(cSample)
	if iField < 0 {
		return "", false
	}
	iSet := it8.locatePatch(cPatch)
	if iSet < 0 {
		return "", false
	}
	p := it8.getData(iSet, iField)
	if p == nil {
		return "", false
	}
	return *p, true
}

// GetDataDbl returns a cell by patch+sample parsed as a double (the C
// cmsIT8GetDataDbl).
func (it8 *IT8) GetDataDbl(cPatch, cSample string) float64 {
	s, _ := it8.GetData(cPatch, cSample)
	return parseFloatNumber(s)
}

// SetData writes a cell by patch+sample name (the C cmsIT8SetData). When the
// table is empty it allocates format+data and, for SAMPLE_ID, appends a patch.
func (it8 *IT8) SetData(cPatch, cSample, val string) bool {
	t := it8.getTable()
	iField := it8.locateSample(cSample)
	if iField < 0 {
		return false
	}

	if t.nPatches == 0 {
		if !it8.allocateDataFormat() {
			return false
		}
		if !it8.allocateDataSet() {
			return false
		}
		it8.cookPointers()
	}

	var iSet int
	if cgatsStrcasecmp(cSample, "SAMPLE_ID") == 0 {
		iSet = it8.locateEmptyPatch()
		if iSet < 0 {
			return it8.synError("Couldn't add more patches '%s'\n", cPatch)
		}
		iField = t.sampleID
	} else {
		iSet = it8.locatePatch(cPatch)
		if iSet < 0 {
			return false
		}
	}
	return it8.setData(iSet, iField, val)
}

// SetDataDbl writes a cell by patch+sample formatted with the double formatter
// (the C cmsIT8SetDataDbl).
func (it8 *IT8) SetDataDbl(cPatch, cSample string, val float64) bool {
	return it8.SetData(cPatch, cSample, it8.formatDbl(val))
}

// GetPatchName returns the SAMPLE_ID of patch nPatch, with a present flag (the
// C cmsIT8GetPatchName).
func (it8 *IT8) GetPatchName(nPatch int) (string, bool) {
	t := it8.getTable()
	data := it8.getData(nPatch, t.sampleID)
	if data == nil {
		return "", false
	}
	s := *data
	if len(s) > cgatsMaxStr-1 {
		s = s[:cgatsMaxStr-1]
	}
	return s, true
}

// GetPatchByName returns the patch index for a SAMPLE_ID (the C
// cmsIT8GetPatchByName).
func (it8 *IT8) GetPatchByName(cPatch string) int { return it8.locatePatch(cPatch) }

// SetTableByLabel resolves the LABEL extension, selecting the referenced table
// (the C cmsIT8SetTableByLabel). Returns the selected table index or -1.
func (it8 *IT8) SetTableByLabel(cSet, cField, expectedType string) int {
	if cField == "" {
		cField = "LABEL"
	}
	cLabelFld, ok := it8.GetData(cSet, cField)
	if !ok {
		return -1
	}
	// C uses sscanf("%255s %u %255s", ...): it binds the first three
	// whitespace-delimited tokens and ignores any trailing ones, so a cooked
	// LABEL cell with more than three tokens (e.g. from a multi-word property
	// value) still resolves rather than being rejected.
	fields := strings.Fields(cLabelFld)
	if len(fields) < 3 {
		return -1
	}
	if !allDigits(fields[1]) {
		return -1
	}
	nTable := cAtoi(fields[1])
	typ := fields[2]
	if expectedType != "" {
		if cgatsStrcasecmp(typ, expectedType) != 0 {
			return -1
		}
	}
	if nTable < 0 {
		return -1
	}
	return it8.SetTable(uint32(nTable))
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// SetIndexColumn selects the column used as the patch index (the C
// cmsIT8SetIndexColumn).
func (it8 *IT8) SetIndexColumn(cSample string) bool {
	pos := it8.locateSample(cSample)
	if pos == -1 {
		return false
	}
	it8.tab[it8.nTable].sampleID = pos
	return true
}

// DefineDblFormat sets the printf-style formatter for doubles (the C
// cmsIT8DefineDblFormat). An empty formatter restores the default.
func (it8 *IT8) DefineDblFormat(formatter string) {
	if formatter == "" {
		it8.doubleFormatter = cgatsDefaultDblFormat
		return
	}
	if len(formatter) > cgatsMaxID-1 {
		formatter = formatter[:cgatsMaxID-1]
	}
	it8.doubleFormatter = formatter
}

// ------------------------------------------------------ .cube device links
//
// Ports the .cube (Adobe/IRIDAS LUT) reader from cmscgats.c: ParseCube plus
// cmsCreateDeviceLinkFromCubeFileTHR / cmsCreateDeviceLinkFromCubeFile. The
// tokenizer above is reused with the CUBE keyword table (isCUBE == true). The
// result is an RGB->RGB device-link Profile whose AToB0 tag holds a pipeline of
// an optional 1-D shaper (tone curves) followed by an optional 3-D CLUT.

// readNumbers mirrors ReadNumbers: read exactly n numeric tokens into arr, then
// consume the end-of-line. Returns false (recording a syntax error) on the
// first non-numeric token.
func (it8 *IT8) readNumbers(n int, arr []float64) bool {
	for i := 0; i < n; i++ {
		switch it8.sy {
		case symInum:
			arr[i] = float64(it8.inum)
		case symDnum:
			arr[i] = it8.dnum
		default:
			return it8.synError("Number expected")
		}
		it8.inSymbol()
	}
	return it8.checkEOLN()
}

// parseCube mirrors ParseCube. On success it returns the (optional) shaper and
// CLUT stages plus the profile title; a nil stage means that section was absent.
func (it8 *IT8) parseCube() (shaper, clut *Stage, title string, ok bool) {
	domainMin := [3]float64{0, 0, 0}
	domainMax := [3]float64{1.0, 1.0, 1.0}
	check01 := [2]float64{0, 1.0}
	shaperSize := 0
	lutSize := 0
	ctx := it8.ctx

	it8.inSymbol()

	for it8.sy != symEof && it8.sy != symSynError {
		switch it8.sy {

		case symTitle:
			it8.inSymbol()
			if !it8.check(symString, "Title string expected") {
				return nil, nil, "", false
			}
			title = it8.str.String()
			if len(title) > cgatsMaxStr-1 {
				title = title[:cgatsMaxStr-1]
			}
			it8.inSymbol()

		case symDomainMin:
			it8.inSymbol()
			if !it8.readNumbers(3, domainMin[:]) {
				return nil, nil, "", false
			}

		case symDomainMax:
			it8.inSymbol()
			if !it8.readNumbers(3, domainMax[:]) {
				return nil, nil, "", false
			}

		case symLut1DSize:
			it8.inSymbol()
			if !it8.check(symInum, "Shaper size expected") {
				return nil, nil, "", false
			}
			shaperSize = int(it8.inum)
			if shaperSize < 2 || shaperSize > 65536 {
				it8.synError("LUT_1D_SIZE '%d' is out of bounds", shaperSize)
				return nil, nil, "", false
			}
			it8.inSymbol()

		case symLut3DSize:
			it8.inSymbol()
			if !it8.check(symInum, "LUT size expected") {
				return nil, nil, "", false
			}
			lutSize = int(it8.inum)
			it8.inSymbol()

		case symLut1DInputRange, symLut3DInputRange:
			it8.inSymbol()
			if !it8.readNumbers(2, check01[:]) {
				return nil, nil, "", false
			}
			if check01[0] != 0 || check01[1] != 1.0 {
				it8.synError("Unsupported format")
				return nil, nil, "", false
			}

		case symEoln:
			it8.inSymbol()

		case symInum, symDnum:
			// Data rows: shaper block (if any) then CLUT block (if any).
			if shaperSize > 0 {
				shapers := make([]float32, 3*shaperSize)
				for i := 0; i < shaperSize; i++ {
					var nums [3]float64
					if !it8.readNumbers(3, nums[:]) {
						return nil, nil, "", false
					}
					shapers[i+0*shaperSize] = float32((nums[0] - domainMin[0]) / (domainMax[0] - domainMin[0]))
					shapers[i+1*shaperSize] = float32((nums[1] - domainMin[1]) / (domainMax[1] - domainMin[1]))
					shapers[i+2*shaperSize] = float32((nums[2] - domainMin[2]) / (domainMax[2] - domainMin[2]))
				}

				var curves [3]*ToneCurve
				for i := 0; i < 3; i++ {
					c, err := ctx.BuildTabulatedToneCurveFloat(shapers[i*shaperSize : i*shaperSize+shaperSize])
					if err != nil || c == nil {
						return nil, nil, "", false
					}
					curves[i] = c
				}

				st, err := ctx.StageAllocToneCurves(3, curves[:])
				if err != nil || st == nil {
					return nil, nil, "", false
				}
				shaper = st
			}

			if lutSize > 0 {
				// Professional LUT generation tools list 65x65x65 as their
				// highest supported size (see the reference comment).
				if lutSize < 2 || lutSize > 65 {
					it8.synError("LUT size '%d' is not allowed", lutSize)
					return nil, nil, "", false
				}

				nodes := lutSize * lutSize * lutSize
				lutTable := make([]float32, nodes*3)
				for i := 0; i < nodes; i++ {
					var nums [3]float64
					if !it8.readNumbers(3, nums[:]) {
						return nil, nil, "", false
					}
					lutTable[i*3+2] = float32((nums[0] - domainMin[0]) / (domainMax[0] - domainMin[0]))
					lutTable[i*3+1] = float32((nums[1] - domainMin[1]) / (domainMax[1] - domainMin[1]))
					lutTable[i*3+0] = float32((nums[2] - domainMin[2]) / (domainMax[2] - domainMin[2]))
				}

				st, err := ctx.StageAllocCLutFloat(uint32(lutSize), 3, 3, lutTable)
				if err != nil || st == nil {
					return nil, nil, "", false
				}
				clut = st
			}

			if !it8.check(symEof, "Extra symbols found in file") {
				return nil, nil, "", false
			}

		default:
			// symLutInVideoRange / symLutOutVideoRange and any stray token.
			it8.synError("Unsupported format")
			return nil, nil, "", false
		}
	}

	return shaper, clut, title, true
}

// createDeviceLinkFromCube is the shared builder behind the file/mem entry
// points. It parses cube (already seeded with the .cube bytes) and returns the
// RGB->RGB device-link profile.
func (ctx *Context) createDeviceLinkFromCube(cube *IT8) (*Profile, error) {
	cube.isCUBE = true

	shaper, clut, title, ok := cube.parseCube()
	if !ok {
		if cube.parseErr != nil {
			return nil, cube.parseErr
		}
		return nil, ctx.signalError(ErrCorruptionDetected, "CreateDeviceLinkFromCube: parse failed")
	}

	hProfile := ctx.CreateProfilePlaceholder()
	if hProfile == nil {
		return nil, ctx.signalError(ErrNull, "CreateDeviceLinkFromCube: cannot allocate profile")
	}

	hProfile.SetProfileVersion(4.4)
	hProfile.SetDeviceClass(SigLinkClass)
	hProfile.SetColorSpace(SigRgbData)
	hProfile.SetPCS(SigRgbData)
	hProfile.SetHeaderRenderingIntent(IntentPerceptual)

	pipeline, err := ctx.PipelineAlloc(3, 3)
	if err != nil || pipeline == nil {
		return nil, ctx.signalError(ErrNull, "CreateDeviceLinkFromCube: cannot allocate pipeline")
	}

	if shaper != nil {
		if err := pipeline.InsertStage(AtBegin, shaper); err != nil {
			return nil, err
		}
	}
	if clut != nil {
		if err := pipeline.InsertStage(AtEnd, clut); err != nil {
			return nil, err
		}
	}

	// Propagate the description. No copyright is written because the
	// copyrighted state of the .cube is unknown (matches the reference).
	descMLU := ctx.NewMLU(1)
	if descMLU == nil {
		return nil, ctx.signalError(ErrNull, "CreateDeviceLinkFromCube: cannot allocate MLU")
	}
	if !descMLU.SetUTF8("", "", title) {
		return nil, ctx.signalError(ErrInternal, "CreateDeviceLinkFromCube: cannot set description")
	}

	if err := hProfile.WriteTag(SigProfileDescriptionTag, descMLU); err != nil {
		return nil, err
	}
	if err := hProfile.WriteTag(SigAToB0Tag, pipeline); err != nil {
		return nil, err
	}

	return hProfile, nil
}

// CreateDeviceLinkFromCubeMem builds an RGB->RGB device-link profile from the
// bytes of a .cube (Adobe/IRIDAS) LUT held in memory. It is the idiomatic
// counterpart of the reference's cmsCreateDeviceLinkFromCubeFileTHR (our IT8
// core loads from memory).
func (ctx *Context) CreateDeviceLinkFromCubeMem(data []byte) (*Profile, error) {
	if ctx == nil {
		ctx = defaultContext
	}

	cube := IT8Alloc(ctx)

	// Mirror the C fopen/fgetc source: NUL-truncate then NUL-terminate.
	src := data
	if i := indexByteInt(data, 0); i >= 0 {
		src = data[:i]
	}
	buf := make([]byte, len(src)+1)
	copy(buf, src)
	cube.fileStack[0] = &cgatsStream{name: "", data: buf, isMem: true}

	return ctx.createDeviceLinkFromCube(cube)
}

// CreateDeviceLinkFromCubeMem builds a .cube device link on the default context.
func CreateDeviceLinkFromCubeMem(data []byte) (*Profile, error) {
	return defaultContext.CreateDeviceLinkFromCubeMem(data)
}

// CreateDeviceLinkFromCubeFile builds an RGB->RGB device-link profile from a
// .cube file on disk (the C cmsCreateDeviceLinkFromCubeFileTHR).
func (ctx *Context) CreateDeviceLinkFromCubeFile(fileName string) (*Profile, error) {
	if ctx == nil {
		ctx = defaultContext
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, ctx.signalError(ErrFile, "File '%s' not found", fileName)
	}

	cube := IT8Alloc(ctx)
	base := fileName
	if len(base) > cgatsMaxPath-1 {
		base = base[:cgatsMaxPath-1]
	}
	cube.fileStack[0] = &cgatsStream{name: base, data: data, isMem: false}

	return ctx.createDeviceLinkFromCube(cube)
}

// CreateDeviceLinkFromCubeFile builds a .cube device link on the default context
// (the C cmsCreateDeviceLinkFromCubeFile).
func CreateDeviceLinkFromCubeFile(fileName string) (*Profile, error) {
	return defaultContext.CreateDeviceLinkFromCubeFile(fileName)
}
