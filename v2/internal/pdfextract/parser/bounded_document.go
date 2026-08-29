package parser

import (
	"bytes"
	"compress/lzw"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mgilbir/pdf0"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

const (
	// Text extraction can expand one string byte to at most one four-byte UTF-8
	// rune. Non-string tokens can also contribute one layout byte. Reserving the
	// entire token/page ceilings before dividing by UTFMax therefore bounds the
	// result before pdf0 allocates it, not only when the wire frame is written.
	maxDecodedTextContentBytes = (protocol.MaxExtractedTextBytes - maxContentTokens - protocol.MaxPages) / utf8.UTFMax
	maxDecodedParserStream     = 32 << 20
	maxDecodedCMapBytes        = 2 << 20
	maxPageTreeNodes           = protocol.MaxPages * 4
	maxPageTreeDepth           = 64
	maxFormDepth               = 32
	maxCMapEntries             = 1 << 16
	maxStreamFilters           = 8
)

func parseFileContext(ctx context.Context, path string) (result protocol.Result, resultErr error) {
	defer func() {
		if recover() != nil {
			result = protocol.Result{}
			resultErr = protocol.ErrInvalidPDF
		}
	}()
	if ctx == nil {
		return protocol.Result{}, errors.New("pdfextract: nil parser context")
	}
	linkInfo, err := os.Lstat(path)
	if err != nil || !linkInfo.Mode().IsRegular() {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	file, err := os.Open(path)
	if err != nil {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	fileOpen := true
	defer func() {
		if fileOpen {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(linkInfo, info) || info.Size() < 8 {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if info.Size() > protocol.MaxSourceBytes {
		return protocol.Result{}, protocol.ErrResourceLimit
	}
	var signature [5]byte
	if _, err := file.ReadAt(signature[:], 0); err != nil || string(signature[:]) != "%PDF-" {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}

	document, err := pdf0.ReadContext(ctx, file, info.Size(),
		pdf0.WithMaxDecodedStreamBytes(maxDecodedParserStream),
		pdf0.WithMaxDecodedContentBytes(maxDecodedTextContentBytes),
		pdf0.WithMaxObjectStreamBytes(maxDecodedParserStream),
		pdf0.WithMaxContentStreamBytes(maxDecodedTextContentBytes),
		pdf0.WithMaxICCProfileBytes(1<<20),
		pdf0.WithMaxXMPPacketBytes(1<<20),
		pdf0.WithMaxCIDRangeSpan(maxCMapEntries),
		pdf0.WithMaxRoleMapSteps(1<<16),
		pdf0.WithMaxTableGridFills(1<<20),
		pdf0.WithMaxPostScriptSteps(1<<16),
		pdf0.WithMaxCmapWork(maxCMapEntries),
	)
	closeErr := file.Close()
	fileOpen = false
	if err != nil {
		if ctx.Err() != nil {
			return protocol.Result{}, ctx.Err()
		}
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if closeErr != nil {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	if document.Encrypted || document.Locked() {
		return protocol.Result{}, protocol.ErrEncryptedPDF
	}
	pages, err := boundedPages(ctx, document)
	if err != nil {
		return protocol.Result{}, err
	}
	preflight := newTextPreflight(ctx, document)
	for _, page := range pages {
		if err := preflight.scanPage(page); err != nil {
			return protocol.Result{}, err
		}
	}
	if err := preflight.installUniGBCMaps(); err != nil {
		return protocol.Result{}, err
	}

	text, err := document.ExtractTextContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return protocol.Result{}, ctx.Err()
		}
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	text = normalizeExtractedText(text)
	if text == "" {
		return protocol.Result{}, protocol.ErrNoExtractedText
	}
	if len(text) > protocol.MaxExtractedTextBytes {
		return protocol.Result{}, protocol.ErrResourceLimit
	}
	if !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
		return protocol.Result{}, protocol.ErrInvalidPDF
	}
	return protocol.Result{Text: text, Pages: len(pages)}, nil
}

func normalizeExtractedText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\f", "\n")
	lines := strings.Split(text, "\n")
	out := lines[:0]
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if len(out) == 0 || blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func boundedPages(ctx context.Context, document *pdf0.Document) ([]*pdf0.Dictionary, error) {
	catalog := document.ResolveDict(document.Trailer.Get("Root"))
	if catalog == nil {
		return nil, protocol.ErrInvalidPDF
	}
	root := catalog.Get("Pages")
	if root == nil {
		return nil, protocol.ErrInvalidPDF
	}
	var pages []*pdf0.Dictionary
	seenRefs := make(map[int]bool)
	seenDirect := make(map[*pdf0.Dictionary]bool)
	nodes := 0
	var walk func(pdf0.Object, int) error
	walk = func(object pdf0.Object, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > maxPageTreeDepth {
			return protocol.ErrResourceLimit
		}
		if ref, ok := object.(pdf0.IndirectRef); ok {
			if seenRefs[ref.Number] {
				return nil
			}
			seenRefs[ref.Number] = true
		}
		node := document.ResolveDict(object)
		if node == nil {
			return protocol.ErrInvalidPDF
		}
		if _, direct := object.(*pdf0.Dictionary); direct {
			if seenDirect[node] {
				return nil
			}
			seenDirect[node] = true
		} else if _, direct := object.(pdf0.Dictionary); direct {
			if seenDirect[node] {
				return nil
			}
			seenDirect[node] = true
		}
		nodes++
		if nodes > maxPageTreeNodes {
			return protocol.ErrResourceLimit
		}
		typeName, _ := document.Resolve(node.Get("Type")).(pdf0.Name)
		switch typeName {
		case "Pages":
			if count, ok := document.Resolve(node.Get("Count")).(pdf0.Integer); ok {
				if count < 0 {
					return protocol.ErrInvalidPDF
				}
				if count > pdf0.Integer(protocol.MaxPages) {
					return protocol.ErrResourceLimit
				}
			}
			kids, ok := document.Resolve(node.Get("Kids")).(pdf0.Array)
			if !ok {
				return protocol.ErrInvalidPDF
			}
			for _, child := range kids {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		case "Page":
			pages = append(pages, node)
			if len(pages) > protocol.MaxPages {
				return protocol.ErrResourceLimit
			}
		default:
			return protocol.ErrInvalidPDF
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, protocol.ErrInvalidPDF
	}
	return pages, nil
}

type textPreflight struct {
	ctx            context.Context
	document       *pdf0.Document
	decodedBytes   int
	tokens         int
	uniGBFonts     map[*pdf0.Dictionary]map[uint16]struct{}
	validatedCMaps map[*pdf0.Stream]bool
	validatedFonts map[*pdf0.Dictionary]bool
	activeForms    map[*pdf0.Stream]bool
}

func newTextPreflight(ctx context.Context, document *pdf0.Document) *textPreflight {
	return &textPreflight{
		ctx:            ctx,
		document:       document,
		uniGBFonts:     make(map[*pdf0.Dictionary]map[uint16]struct{}),
		validatedCMaps: make(map[*pdf0.Stream]bool),
		validatedFonts: make(map[*pdf0.Dictionary]bool),
	}
}

func (p *textPreflight) scanPage(page *pdf0.Dictionary) error {
	resources, err := p.inheritedResources(page)
	if err != nil {
		return err
	}
	p.activeForms = make(map[*pdf0.Stream]bool)
	state := contentState{resources: resources}
	return p.scanContents(page.Get("Contents"), &state, 0)
}

func (p *textPreflight) inheritedResources(page *pdf0.Dictionary) (*pdf0.Dictionary, error) {
	seen := make(map[*pdf0.Dictionary]bool)
	node := page
	for depth := 0; depth <= maxPageTreeDepth; depth++ {
		if node == nil || seen[node] {
			return nil, protocol.ErrInvalidPDF
		}
		seen[node] = true
		if value := node.Get("Resources"); value != nil {
			resources := p.document.ResolveDict(value)
			if resources == nil {
				return nil, protocol.ErrInvalidPDF
			}
			return resources, nil
		}
		parent := node.Get("Parent")
		if parent == nil {
			return &pdf0.Dictionary{}, nil
		}
		node = p.document.ResolveDict(parent)
	}
	return nil, protocol.ErrResourceLimit
}

func (p *textPreflight) scanContents(object pdf0.Object, state *contentState, depth int) error {
	if object == nil {
		return nil
	}
	if depth > maxFormDepth {
		return protocol.ErrResourceLimit
	}
	resolved := p.document.Resolve(object)
	switch value := resolved.(type) {
	case *pdf0.Stream:
		data, err := p.decodeStream(value)
		if err != nil {
			return err
		}
		return p.scanContent(data, state, depth)
	case pdf0.Array:
		for _, entry := range value {
			if err := p.scanContents(entry, state, depth); err != nil {
				return err
			}
		}
		return nil
	default:
		return protocol.ErrInvalidPDF
	}
}

func (p *textPreflight) decodeStream(stream *pdf0.Stream) ([]byte, error) {
	if stream == nil {
		return nil, protocol.ErrInvalidPDF
	}
	data := append([]byte(nil), stream.Data...)
	filters, err := p.streamFilters(&stream.Dict)
	if err != nil {
		return nil, err
	}
	if stream.Dict.Get("DecodeParms") != nil {
		return nil, protocol.ErrInvalidPDF
	}
	for _, filter := range filters {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		var reader io.ReadCloser
		switch filter {
		case "FlateDecode", "Fl":
			reader, err = zlib.NewReader(bytes.NewReader(data))
		case "LZWDecode", "LZW":
			reader = lzw.NewReader(bytes.NewReader(data), lzw.MSB, 8)
		case "ASCII85Decode", "A85":
			reader = io.NopCloser(ascii85.NewDecoder(bytes.NewReader(data)))
		case "ASCIIHexDecode", "AHx":
			data, err = decodeASCIIHex(p.ctx, data, maxDecodedTextContentBytes)
			if err != nil {
				return nil, err
			}
			continue
		default:
			return nil, protocol.ErrInvalidPDF
		}
		if err != nil {
			return nil, protocol.ErrInvalidPDF
		}
		data, err = readBounded(p.ctx, reader, maxDecodedTextContentBytes)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			if p.ctx.Err() != nil {
				return nil, p.ctx.Err()
			}
			if errors.Is(err, protocol.ErrResourceLimit) {
				return nil, err
			}
			return nil, protocol.ErrInvalidPDF
		}
	}
	if len(data) > maxDecodedTextContentBytes || p.decodedBytes > maxDecodedTextContentBytes-len(data) {
		return nil, protocol.ErrResourceLimit
	}
	p.decodedBytes += len(data)
	return data, nil
}

func (p *textPreflight) streamFilters(dictionary *pdf0.Dictionary) ([]pdf0.Name, error) {
	object := p.document.Resolve(dictionary.Get("Filter"))
	if object == nil {
		return nil, nil
	}
	switch value := object.(type) {
	case pdf0.Name:
		return []pdf0.Name{value}, nil
	case pdf0.Array:
		if len(value) > maxStreamFilters {
			return nil, protocol.ErrResourceLimit
		}
		filters := make([]pdf0.Name, 0, len(value))
		for _, entry := range value {
			name, ok := p.document.Resolve(entry).(pdf0.Name)
			if !ok {
				return nil, protocol.ErrInvalidPDF
			}
			filters = append(filters, name)
		}
		return filters, nil
	default:
		return nil, protocol.ErrInvalidPDF
	}
}

func (p *textPreflight) validateFont(font *pdf0.Dictionary) error {
	if font == nil {
		return protocol.ErrInvalidPDF
	}
	if p.validatedFonts[font] {
		return nil
	}
	p.validatedFonts[font] = true
	if cmapObject := font.Get("ToUnicode"); cmapObject != nil {
		stream, ok := p.document.Resolve(cmapObject).(*pdf0.Stream)
		if !ok {
			return protocol.ErrInvalidPDF
		}
		if !p.validatedCMaps[stream] {
			p.validatedCMaps[stream] = true
			data, err := p.decodeStream(stream)
			if err != nil {
				return err
			}
			if len(data) > maxDecodedCMapBytes || bytes.Count(data, []byte("<")) > maxCMapEntries*3 {
				return protocol.ErrResourceLimit
			}
		}
		return nil
	}
	uniGB, err := p.isUniGBFont(font)
	if err != nil {
		return err
	}
	if uniGB {
		p.uniGBFonts[font] = make(map[uint16]struct{})
	}
	return nil
}

func (p *textPreflight) isUniGBFont(font *pdf0.Dictionary) (bool, error) {
	if subtype, _ := p.document.Resolve(font.Get("Subtype")).(pdf0.Name); subtype != "Type0" {
		return false, nil
	}
	encoding, _ := p.document.Resolve(font.Get("Encoding")).(pdf0.Name)
	if encoding != "UniGB-UCS2-H" && encoding != "UniGB-UCS2-V" {
		return false, nil
	}
	descendants, ok := p.document.Resolve(font.Get("DescendantFonts")).(pdf0.Array)
	if !ok || len(descendants) != 1 {
		return false, protocol.ErrInvalidPDF
	}
	descendant := p.document.ResolveDict(descendants[0])
	if descendant == nil {
		return false, protocol.ErrInvalidPDF
	}
	subtype, _ := p.document.Resolve(descendant.Get("Subtype")).(pdf0.Name)
	if subtype != "CIDFontType0" && subtype != "CIDFontType2" {
		return false, protocol.ErrInvalidPDF
	}
	systemInfo := p.document.ResolveDict(descendant.Get("CIDSystemInfo"))
	if systemInfo == nil || !p.pdfStringEquals(systemInfo.Get("Registry"), "Adobe") || !p.pdfStringEquals(systemInfo.Get("Ordering"), "GB1") {
		return false, protocol.ErrInvalidPDF
	}
	supplement, ok := p.document.Resolve(systemInfo.Get("Supplement")).(pdf0.Integer)
	if !ok || supplement < 0 {
		return false, protocol.ErrInvalidPDF
	}
	return true, nil
}

func (p *textPreflight) pdfStringEquals(object pdf0.Object, expected string) bool {
	value, ok := p.document.Resolve(object).(pdf0.String)
	return ok && string(value.Value) == expected
}

func (p *textPreflight) noteText(font *pdf0.Dictionary, raw []byte) error {
	if font == nil {
		return nil
	}
	if err := p.validateFont(font); err != nil {
		return err
	}
	codes, ok := p.uniGBFonts[font]
	if !ok {
		return nil
	}
	if len(raw)%2 != 0 {
		return protocol.ErrInvalidPDF
	}
	for index := 0; index < len(raw); index += 2 {
		code := uint16(raw[index])<<8 | uint16(raw[index+1])
		if 0xD800 <= code && code <= 0xDFFF {
			return protocol.ErrInvalidPDF
		}
		codes[code] = struct{}{}
		if len(codes) > maxCMapEntries {
			return protocol.ErrResourceLimit
		}
	}
	return nil
}

func (p *textPreflight) installUniGBCMaps() error {
	for font, codes := range p.uniGBFonts {
		if len(codes) == 0 {
			continue
		}
		ordered := make([]int, 0, len(codes))
		for code := range codes {
			ordered = append(ordered, int(code))
		}
		sort.Ints(ordered)
		var cmap strings.Builder
		cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
		cmap.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
		cmap.WriteString("/CMapName /MindWeaver-UniGB-UCS2 def\n/CMapType 2 def\n")
		cmap.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
		for start := 0; start < len(ordered); start += 100 {
			end := start + 100
			if end > len(ordered) {
				end = len(ordered)
			}
			fmt.Fprintf(&cmap, "%d beginbfchar\n", end-start)
			for _, code := range ordered[start:end] {
				fmt.Fprintf(&cmap, "<%04X> <%04X>\n", code, code)
			}
			cmap.WriteString("endbfchar\n")
		}
		cmap.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
		data := []byte(cmap.String())
		if len(data) > maxDecodedCMapBytes {
			return protocol.ErrResourceLimit
		}
		dictionary := pdf0.Dictionary{}
		dictionary.Set("Length", pdf0.Integer(len(data)))
		font.Set("ToUnicode", &pdf0.Stream{Dict: dictionary, Data: data})
	}
	return nil
}

func readBounded(ctx context.Context, reader io.Reader, limit int) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, err := reader.Read(buffer)
		if count > 0 {
			if output.Len() > limit-count {
				return nil, protocol.ErrResourceLimit
			}
			_, _ = output.Write(buffer[:count])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return output.Bytes(), nil
			}
			return nil, err
		}
		if count == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

func decodeASCIIHex(ctx context.Context, data []byte, limit int) ([]byte, error) {
	capacity := len(data) / 2
	if capacity > limit {
		capacity = limit
	}
	result := make([]byte, 0, capacity)
	var high byte
	haveHigh := false
	for index, value := range data {
		if index%(32<<10) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if value == '>' {
			break
		}
		if value == 0 || value == '\t' || value == '\n' || value == '\f' || value == '\r' || value == ' ' {
			continue
		}
		nibble, ok := hexNibble(value)
		if !ok {
			return nil, protocol.ErrInvalidPDF
		}
		if !haveHigh {
			high = nibble
			haveHigh = true
			continue
		}
		if len(result) >= limit {
			return nil, protocol.ErrResourceLimit
		}
		result = append(result, high<<4|nibble)
		haveHigh = false
	}
	if haveHigh {
		if len(result) >= limit {
			return nil, protocol.ErrResourceLimit
		}
		result = append(result, high<<4)
	}
	return result, nil
}

func hexNibble(value byte) (byte, bool) {
	switch {
	case '0' <= value && value <= '9':
		return value - '0', true
	case 'a' <= value && value <= 'f':
		return value - 'a' + 10, true
	case 'A' <= value && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
