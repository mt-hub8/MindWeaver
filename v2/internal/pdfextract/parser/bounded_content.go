package parser

import (
	"context"

	"github.com/mgilbir/pdf0"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

const (
	maxContentTokens = 1 << 20
	maxOperandStack  = 4096
)

type contentState struct {
	resources   *pdf0.Dictionary
	currentFont *pdf0.Dictionary
}

type contentTokenKind uint8

const (
	contentOperator contentTokenKind = iota
	contentName
	contentString
	contentOperand
)

type boundedContentToken struct {
	kind contentTokenKind
	text string
	raw  []byte
}

func (p *textPreflight) scanContent(data []byte, state *contentState, depth int) error {
	scanner := boundedContentScanner{ctx: p.ctx, data: data}
	operands := make([]boundedContentToken, 0, 16)
	for {
		token, ok, err := scanner.next()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		p.tokens++
		if p.tokens > maxContentTokens {
			return protocol.ErrResourceLimit
		}
		if token.kind != contentOperator {
			if len(operands) >= maxOperandStack {
				return protocol.ErrResourceLimit
			}
			operands = append(operands, token)
			continue
		}
		switch token.text {
		case "Tf":
			name := lastName(operands)
			font, err := p.resolveFont(state.resources, name)
			if err != nil {
				return err
			}
			state.currentFont = font
			if err := p.validateFont(font); err != nil {
				return err
			}
		case "Tj", "'", "\"":
			if raw, found := lastString(operands); found {
				if err := p.noteText(state.currentFont, raw); err != nil {
					return err
				}
			}
		case "TJ":
			for _, operand := range operands {
				if operand.kind == contentString {
					if err := p.noteText(state.currentFont, operand.raw); err != nil {
						return err
					}
				}
			}
		case "Do":
			if err := p.scanForm(state, lastName(operands), depth); err != nil {
				return err
			}
		case "BI":
			if err := scanner.skipInlineImage(); err != nil {
				return err
			}
		}
		operands = operands[:0]
	}
}

func (p *textPreflight) resolveFont(resources *pdf0.Dictionary, name string) (*pdf0.Dictionary, error) {
	if resources == nil || name == "" {
		return nil, protocol.ErrInvalidPDF
	}
	fonts := p.document.ResolveDict(resources.Get("Font"))
	if fonts == nil {
		return nil, protocol.ErrInvalidPDF
	}
	font := p.document.ResolveDict(fonts.Get(pdf0.Name(name)))
	if font == nil {
		return nil, protocol.ErrInvalidPDF
	}
	return font, nil
}

func (p *textPreflight) scanForm(state *contentState, name string, depth int) error {
	if name == "" || state.resources == nil {
		return nil
	}
	xobjects := p.document.ResolveDict(state.resources.Get("XObject"))
	if xobjects == nil {
		return nil
	}
	stream, ok := p.document.Resolve(xobjects.Get(pdf0.Name(name))).(*pdf0.Stream)
	if !ok {
		return nil
	}
	subtype, _ := p.document.Resolve(stream.Dict.Get("Subtype")).(pdf0.Name)
	if subtype != "Form" {
		return nil
	}
	if p.activeForms[stream] {
		return protocol.ErrInvalidPDF
	}
	p.activeForms[stream] = true
	defer delete(p.activeForms, stream)
	resources := p.document.ResolveDict(stream.Dict.Get("Resources"))
	if resources == nil {
		resources = state.resources
	}
	formState := contentState{resources: resources}
	return p.scanContents(stream, &formState, depth+1)
}

func lastName(tokens []boundedContentToken) string {
	for index := len(tokens) - 1; index >= 0; index-- {
		if tokens[index].kind == contentName {
			return tokens[index].text
		}
	}
	return ""
}

func lastString(tokens []boundedContentToken) ([]byte, bool) {
	for index := len(tokens) - 1; index >= 0; index-- {
		if tokens[index].kind == contentString {
			return tokens[index].raw, true
		}
	}
	return nil, false
}

type boundedContentScanner struct {
	ctx              context.Context
	data             []byte
	pos              int
	lastContextCheck int
}

func (s *boundedContentScanner) checkContext() error {
	if s.pos-s.lastContextCheck < 32<<10 {
		return nil
	}
	s.lastContextCheck = s.pos
	return s.ctx.Err()
}

func (s *boundedContentScanner) next() (boundedContentToken, bool, error) {
	if err := s.skipWhitespace(); err != nil {
		return boundedContentToken{}, false, err
	}
	if s.pos >= len(s.data) {
		return boundedContentToken{}, false, nil
	}
	start := s.pos
	value := s.data[s.pos]
	switch value {
	case '(':
		raw, err := s.literalString()
		return boundedContentToken{kind: contentString, raw: raw}, true, err
	case '<':
		if s.pos+1 < len(s.data) && s.data[s.pos+1] == '<' {
			s.pos += 2
			return boundedContentToken{kind: contentOperand}, true, nil
		}
		raw, err := s.hexString()
		return boundedContentToken{kind: contentString, raw: raw}, true, err
	case '>':
		if s.pos+1 < len(s.data) && s.data[s.pos+1] == '>' {
			s.pos += 2
			return boundedContentToken{kind: contentOperand}, true, nil
		}
		return boundedContentToken{}, false, protocol.ErrInvalidPDF
	case '/':
		name, err := s.name()
		return boundedContentToken{kind: contentName, text: name}, true, err
	case '[', ']':
		s.pos++
		return boundedContentToken{kind: contentOperand}, true, nil
	case '\'', '"':
		s.pos++
		return boundedContentToken{kind: contentOperator, text: string(value)}, true, nil
	}
	for s.pos < len(s.data) && !isContentWhitespace(s.data[s.pos]) && !isContentDelimiter(s.data[s.pos]) {
		s.pos++
		if err := s.checkContext(); err != nil {
			return boundedContentToken{}, false, err
		}
	}
	if s.pos == start {
		return boundedContentToken{}, false, protocol.ErrInvalidPDF
	}
	word := string(s.data[start:s.pos])
	if isContentNumber(word) || word == "true" || word == "false" || word == "null" {
		return boundedContentToken{kind: contentOperand}, true, nil
	}
	return boundedContentToken{kind: contentOperator, text: word}, true, nil
}

func (s *boundedContentScanner) skipWhitespace() error {
	for s.pos < len(s.data) {
		if err := s.checkContext(); err != nil {
			return err
		}
		switch {
		case isContentWhitespace(s.data[s.pos]):
			s.pos++
		case s.data[s.pos] == '%':
			for s.pos < len(s.data) && s.data[s.pos] != '\r' && s.data[s.pos] != '\n' {
				s.pos++
				if err := s.checkContext(); err != nil {
					return err
				}
			}
		default:
			return nil
		}
	}
	return nil
}

func (s *boundedContentScanner) literalString() ([]byte, error) {
	s.pos++
	depth := 1
	result := make([]byte, 0, 64)
	for s.pos < len(s.data) {
		if err := s.checkContext(); err != nil {
			return nil, err
		}
		value := s.data[s.pos]
		s.pos++
		switch value {
		case '(':
			depth++
			result = append(result, value)
		case ')':
			depth--
			if depth == 0 {
				return result, nil
			}
			result = append(result, value)
		case '\\':
			if s.pos >= len(s.data) {
				return nil, protocol.ErrInvalidPDF
			}
			escaped := s.data[s.pos]
			s.pos++
			switch escaped {
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			case 'b':
				result = append(result, '\b')
			case 'f':
				result = append(result, '\f')
			case '\r':
				if s.pos < len(s.data) && s.data[s.pos] == '\n' {
					s.pos++
				}
			case '\n':
			case '(', ')', '\\':
				result = append(result, escaped)
			default:
				if '0' <= escaped && escaped <= '7' {
					value := int(escaped - '0')
					for count := 1; count < 3 && s.pos < len(s.data) && '0' <= s.data[s.pos] && s.data[s.pos] <= '7'; count++ {
						value = value*8 + int(s.data[s.pos]-'0')
						s.pos++
					}
					result = append(result, byte(value))
				} else {
					result = append(result, escaped)
				}
			}
		default:
			result = append(result, value)
		}
		if len(result) > maxDecodedTextContentBytes {
			return nil, protocol.ErrResourceLimit
		}
	}
	return nil, protocol.ErrInvalidPDF
}

func (s *boundedContentScanner) hexString() ([]byte, error) {
	s.pos++
	result := make([]byte, 0, 64)
	var high byte
	haveHigh := false
	for s.pos < len(s.data) {
		if err := s.checkContext(); err != nil {
			return nil, err
		}
		value := s.data[s.pos]
		s.pos++
		if value == '>' {
			if haveHigh {
				result = append(result, high<<4)
			}
			return result, nil
		}
		if isContentWhitespace(value) {
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
		result = append(result, high<<4|nibble)
		haveHigh = false
		if len(result) > maxDecodedTextContentBytes {
			return nil, protocol.ErrResourceLimit
		}
	}
	return nil, protocol.ErrInvalidPDF
}

func (s *boundedContentScanner) name() (string, error) {
	s.pos++
	result := make([]byte, 0, 16)
	for s.pos < len(s.data) && !isContentWhitespace(s.data[s.pos]) && !isContentDelimiter(s.data[s.pos]) {
		if err := s.checkContext(); err != nil {
			return "", err
		}
		value := s.data[s.pos]
		s.pos++
		if value == '#' {
			if s.pos+1 >= len(s.data) {
				return "", protocol.ErrInvalidPDF
			}
			high, highOK := hexNibble(s.data[s.pos])
			low, lowOK := hexNibble(s.data[s.pos+1])
			if !highOK || !lowOK {
				return "", protocol.ErrInvalidPDF
			}
			result = append(result, high<<4|low)
			s.pos += 2
			continue
		}
		result = append(result, value)
		if len(result) > 256 {
			return "", protocol.ErrResourceLimit
		}
	}
	if len(result) == 0 {
		return "", protocol.ErrInvalidPDF
	}
	return string(result), nil
}

func (s *boundedContentScanner) skipInlineImage() error {
	id, err := s.findDelimitedWord(s.pos, "ID")
	if err != nil {
		return err
	}
	if id < 0 {
		return protocol.ErrInvalidPDF
	}
	end, err := s.findDelimitedWord(id+2, "EI")
	if err != nil {
		return err
	}
	if end < 0 {
		return protocol.ErrInvalidPDF
	}
	s.pos = end + 2
	return nil
}

func (s *boundedContentScanner) findDelimitedWord(start int, word string) (int, error) {
	for index := start; index+len(word) <= len(s.data); index++ {
		s.pos = index
		if err := s.checkContext(); err != nil {
			return -1, err
		}
		if string(s.data[index:index+len(word)]) != word {
			continue
		}
		before := index == 0 || isContentWhitespace(s.data[index-1]) || isContentDelimiter(s.data[index-1])
		afterIndex := index + len(word)
		after := afterIndex == len(s.data) || isContentWhitespace(s.data[afterIndex]) || isContentDelimiter(s.data[afterIndex])
		if before && after {
			return index, nil
		}
	}
	return -1, nil
}

func isContentWhitespace(value byte) bool {
	return value == 0 || value == '\t' || value == '\n' || value == '\f' || value == '\r' || value == ' '
}

func isContentDelimiter(value byte) bool {
	switch value {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%', '\'', '"':
		return true
	default:
		return false
	}
}

func isContentNumber(value string) bool {
	if value == "" {
		return false
	}
	digits := 0
	dots := 0
	for index, char := range []byte(value) {
		switch {
		case '0' <= char && char <= '9':
			digits++
		case char == '.' && dots == 0:
			dots++
		case (char == '+' || char == '-') && index == 0:
		default:
			return false
		}
	}
	return digits > 0
}
