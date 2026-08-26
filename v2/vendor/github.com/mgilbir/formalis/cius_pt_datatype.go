package formalis

import (
	"fmt"
	"strconv"
	"strings"
)

// The lexer and parser for the XPath subset AT/eSPap's CIUS-PT datatype and
// condition patterns are written in.
//
// # What this is for
//
// urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-datatype.sch and the condition pair beside it
// carry 291 fatal assertions under the DT-CIUS-PT-* prefix — four fifths of the
// Portuguese rule set by count, and until this file the whole of it was named in
// Coverage(SourceCIUSPT) as a gap. They are the attribute-level constraints AT
// places on every typed element: a length limit on every free-text field, a date
// format on every date, a code list on every code, a currency attribute that has to
// equal BT-5, a mime code on an attachment, and one arithmetic tier at the end.
//
// They are generated from the Schematron by testdata/cius-pt-rules/gen.py into
// cius_pt_datatype_table.go, for the reason en16931_syntax_advisory.go gives for
// CEN's 1,168 binding rules: a rule set of this size and this shape does not need
// judgement, it needs transcribing without error, and a mistyped element name in a
// hand-written transcription is invisible — the rule simply never fires. Each
// assertion's test in the generated table is **AT's own XPath**, whitespace-
// normalised and otherwise verbatim, so there is no translation step in which a
// rule could change meaning, the table reads against the Schematron line by line,
// and the drift test is a string comparison rather than an argument about a
// transformation.
//
// # Why this is a second evaluator rather than an extension of the advisory one
//
// PR 17's evaluator (en16931_syntax_advisory.go) implements the subset CEN's
// binding rules use, and 1,141 of those 1,168 are a single `not(path)`. Its
// expression values are booleans, numbers and node sets, and that is all they ever
// need to be.
//
// AT's datatype tier is string-processing XPath 2.0. Its 291 assertions call
// matches() with a regular expression 192 times, normalize-space() 321 times,
// starts-with() 85, contains() 78, concat() 63, substring-before()/-after() 77 and
// xs:decimal() 303; they use `every $x in E satisfies E`, index-of(),
// distinct-values(), sum(), round() and `div`. None of those has a representation
// in the advisory evaluator's value model — there is no string value in it at all —
// so reusing it would mean replacing that model with a typed XPath sequence, under
// the 1,168 rules that depend on its exact behaviour (including the general-
// comparison asymmetry advisoryCompare documents at length). The overlap between
// the two subsets is `and`, `or`, `not` and a location step; the difference is
// everything the Portuguese rules are actually made of. So this is a separate
// front end and a separate evaluator over the same parsed tree, and the advisory
// tables are not touched.
//
// # The value model
//
// XPath 2.0's, reduced to what these expressions use: every expression evaluates to
// a sequence of items, an item is a node, an attribute, a string, a number or a
// boolean, and the effective boolean value is XPath's. That last one is not a
// detail. DT-CIUS-PT-166's first branch is
//
//	(xs:decimal(cbc:PrepaidAmount) and not(xs:decimal(cbc:PayableRoundingAmount)) and …)
//
// in which a numeric value stands in boolean position, so a prepaid amount of 0.00
// takes the *other* branch — the one that compares the payable amount against the
// tax-inclusive amount without subtracting anything. Reading that `and` as "the
// element exists" would make AT's rule report every invoice that writes a zero
// prepaid amount.
//
// # Dynamic errors
//
// XPath 2.0 raises a dynamic error for a cast that cannot be performed
// (xs:decimal('abc')) and for a cast or arithmetic applied to a sequence of more
// than one item. A Schematron processor aborts on one; this evaluator returns it,
// and ptDTRun declines to report the assertion it came from. That is the safe
// direction — a finding this package cannot justify is not emitted — and it is
// measured rather than assumed: TestCIUSPTDatatypeCorpusIsClean counts the errors
// raised over the whole corpus and over AT's own twenty instances, and the
// Portuguese instances raise none.

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

type ptDTTokenKind int

const (
	ptTokEOF ptDTTokenKind = iota
	ptTokName
	ptTokLiteral
	ptTokNumber
	ptTokVar
	ptTokAxis
	ptTokPunct
)

type ptDTToken struct {
	kind ptDTTokenKind
	text string
}

// ptDTLex tokenises one expression.
//
// Two rules are worth stating because XPath's own lexer is ambiguous without them.
// A '-' is part of a name when it sits between two name characters and the one
// after it is a letter — that is what makes `substring-after`, `starts-with`,
// `distinct-values` and `index-of` single names rather than subtractions — and a
// '.' is the context item unless a digit follows it, in which case it opens a
// number.
func ptDTLex(s string) ([]ptDTToken, error) {
	var out []ptDTToken
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unterminated string literal at offset %d", i)
			}
			out = append(out, ptDTToken{ptTokLiteral, s[i+1 : i+1+j]})
			i += j + 2
		case c == '"':
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				return nil, fmt.Errorf("unterminated string literal at offset %d", i)
			}
			out = append(out, ptDTToken{ptTokLiteral, s[i+1 : i+1+j]})
			i += j + 2
		case ptDTIsDigit(c) || (c == '.' && i+1 < len(s) && ptDTIsDigit(s[i+1])):
			j := i
			for j < len(s) && (ptDTIsDigit(s[j]) || s[j] == '.') {
				j++
			}
			out = append(out, ptDTToken{ptTokNumber, s[i:j]})
			i = j
		case c == '$':
			j := i + 1
			for j < len(s) && ptDTIsNameChar(s[j]) {
				j++
			}
			if j == i+1 {
				return nil, fmt.Errorf("'$' must be followed by a variable name at offset %d", i)
			}
			out = append(out, ptDTToken{ptTokVar, s[i+1 : j]})
			i = j
		case ptDTIsNameStart(c):
			j := ptDTScanName(s, i)
			// `ancestor::` and `child::` are axes; `xs:decimal` and `cbc:ID` are
			// QNames. Both follow a name with a colon, and the axis is the one
			// where a second colon follows.
			if j+1 < len(s) && s[j] == ':' && s[j+1] == ':' {
				out = append(out, ptDTToken{ptTokAxis, s[i:j]})
				i = j + 2
				continue
			}
			if j < len(s) && s[j] == ':' && j+1 < len(s) && ptDTIsNameStart(s[j+1]) {
				j = ptDTScanName(s, j+1)
			}
			out = append(out, ptDTToken{ptTokName, s[i:j]})
			i = j
		default:
			matched := false
			for _, p := range []string{"<=", ">=", "!=", "//", ".."} {
				if strings.HasPrefix(s[i:], p) {
					out = append(out, ptDTToken{ptTokPunct, p})
					i += 2
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			if strings.IndexByte("=<>/()[]@|,.+-*", c) < 0 {
				return nil, fmt.Errorf("unexpected character %q at offset %d", c, i)
			}
			out = append(out, ptDTToken{ptTokPunct, s[i : i+1]})
			i++
		}
	}
	return append(out, ptDTToken{ptTokEOF, ""}), nil
}

// ptDTScanName consumes one NCName starting at i, taking an internal hyphen with
// it when a letter follows.
func ptDTScanName(s string, i int) int {
	j := i
	for j < len(s) {
		if ptDTIsNameChar(s[j]) {
			j++
			continue
		}
		if s[j] == '-' && j+1 < len(s) && ptDTIsNameStart(s[j+1]) {
			j += 2
			continue
		}
		break
	}
	return j
}

func ptDTIsDigit(c byte) bool { return c >= '0' && c <= '9' }

func ptDTIsNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func ptDTIsNameChar(c byte) bool {
	return ptDTIsNameStart(c) || ptDTIsDigit(c) || c == '.'
}

// ---------------------------------------------------------------------------
// The abstract syntax
// ---------------------------------------------------------------------------

type ptDTOp int

const (
	ptOpOr ptDTOp = iota
	ptOpAnd
	// ptOpCompare is XPath's general comparison, existentially quantified over
	// both operand sequences: `a = 'x'` and `a != 'x'` are *both* false when a
	// selects nothing.
	ptOpCompare
	// ptOpArith is +, -, * and div. An empty operand makes the whole expression
	// empty, which is why `xs:decimal(cbc:PrepaidAmount) - 1.00` on an invoice
	// with no prepaid amount is not zero minus one.
	ptOpArith
	ptOpNeg
	ptOpUnion
	ptOpPath
	ptOpFunc
	ptOpVar
	ptOpLiteral
	ptOpNumber
	// ptOpEvery is `every $v in E satisfies E`.
	ptOpEvery
	// ptOpCastable is `E castable as xs:date`, XPath 2.0's type test. CIUS-RO's
	// seven BR-RO-DT* rules are the only assertions in this package that use it,
	// and they use it with no other type.
	ptOpCastable
)

// ptDTExpr is one node of a parsed assertion.
type ptDTExpr struct {
	op   ptDTOp
	args []*ptDTExpr

	relop string     // ptOpCompare, ptOpArith: the operator's spelling
	fn    string     // ptOpFunc: the function's name
	name  string     // ptOpVar, ptOpEvery: the variable's name; ptOpCastable: the type
	str   string     // ptOpLiteral
	num   float64    // ptOpNumber
	path  *ptDTXPath // ptOpPath

	// matcher is the compiled regular expression of a matches() call, attached at
	// load by ptDTCheck. Looking it up per evaluation cost a string concatenation
	// and a map probe on every one of the 192 matches() calls in this rule set,
	// which is once per typed element in a UBL invoice.
	matcher ptDTMatcher
}

// ptDTXPath is a location path: where it starts, and the steps from there.
type ptDTXPath struct {
	// fromRoot is a leading `//`, which in XPath is an absolute step
	// (/descendant-or-self::node()/) and therefore document-wide whatever the
	// context node is.
	fromRoot bool
	// up counts the leading `../` steps. DT-CIUS-PT-171 and -173 use three of
	// them to climb from a VAT breakdown category to the document element.
	up    int
	steps []ptDTStep
}

// ptDTStep is one step: an element name, an attribute, the context item, an
// ancestor-axis name test, or a function call used as a step.
//
// The last is not exotic here — it is how AT writes every summation:
// `sum(../cac:AllowanceCharge[cbc:ChargeIndicator='true']/xs:decimal(cbc:Amount))`
// applies the cast to each allowance in turn, which is XPath 2.0's E1/E2 with E2
// evaluated once per item of E1.
type ptDTStep struct {
	name     string // element local name, "" for the other kinds
	attr     string // attribute local name
	self     bool   // `.`
	ancestor bool   // `ancestor::name`
	call     *ptDTExpr
	preds    []*ptDTExpr
}

// ---------------------------------------------------------------------------
// The parser
// ---------------------------------------------------------------------------

type ptDTParser struct {
	toks []ptDTToken
	i    int
}

// ptDTParse parses one expression. An error means the expression is outside the
// subset this evaluator implements, which is a rule that would go unchecked: it is
// a hard failure at load and never a skip. gen.py refuses the same expressions
// before they are written, and TestCIUSPTDatatypeTableParses parses every committed
// row, so the gate is closed from both ends.
func ptDTParse(s string) (*ptDTExpr, error) {
	toks, err := ptDTLex(s)
	if err != nil {
		return nil, err
	}
	p := &ptDTParser{toks: toks}
	e, err := p.expr()
	if err != nil {
		return nil, err
	}
	if p.peek(0).kind != ptTokEOF {
		return nil, fmt.Errorf("trailing input at %q", p.peek(0).text)
	}
	return e, nil
}

func (p *ptDTParser) peek(n int) ptDTToken {
	if p.i+n >= len(p.toks) {
		return ptDTToken{ptTokEOF, ""}
	}
	return p.toks[p.i+n]
}

// at reports whether the next token is the given *punctuation*.
//
// The kind check is load-bearing rather than defensive. Every caller passes an
// operator or a bracket, and a string literal carries its own text: without it,
// `substring-after(cbc:Amount,'.')` parses its second argument as the context item
// `.` rather than as the string ".", so the function is asked to find the empty
// string and returns its whole first argument. That is a decimal-place rule that
// passes every document, and it is what CIUS-RO's twenty-one BR-DEC-RO-* rules are
// made of. CIUS-PT's 291 assertions carry five literals of this shape — all `','`,
// all in argument position where the ambiguity does not arise — so the correction
// leaves them evaluating identically, which cius_pt_datatype_test.go pins.
func (p *ptDTParser) at(text string) bool {
	t := p.peek(0)
	return t.kind == ptTokPunct && t.text == text
}

func (p *ptDTParser) atName(text string) bool {
	t := p.peek(0)
	return t.kind == ptTokName && t.text == text
}

func (p *ptDTParser) eat(text string) error {
	if !p.at(text) {
		return fmt.Errorf("expected %q, found %q", text, p.peek(0).text)
	}
	p.i++
	return nil
}

// expr is XPath's ExprSingle reduced to the two forms these rules use: a
// quantified expression, or an OrExpr.
func (p *ptDTParser) expr() (*ptDTExpr, error) {
	if p.atName("every") && p.peek(1).kind == ptTokVar {
		p.i++
		name := p.peek(0).text
		p.i++
		if !p.atName("in") {
			return nil, fmt.Errorf("expected 'in' after `every $%s`, found %q", name, p.peek(0).text)
		}
		p.i++
		seq, err := p.orExpr()
		if err != nil {
			return nil, err
		}
		if !p.atName("satisfies") {
			return nil, fmt.Errorf("expected 'satisfies', found %q", p.peek(0).text)
		}
		p.i++
		body, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &ptDTExpr{op: ptOpEvery, name: name, args: []*ptDTExpr{seq, body}}, nil
	}
	return p.orExpr()
}

func (p *ptDTParser) orExpr() (*ptDTExpr, error) {
	left, err := p.andExpr()
	if err != nil {
		return nil, err
	}
	for p.atName("or") {
		p.i++
		right, err := p.andExpr()
		if err != nil {
			return nil, err
		}
		left = &ptDTExpr{op: ptOpOr, args: []*ptDTExpr{left, right}}
	}
	return left, nil
}

func (p *ptDTParser) andExpr() (*ptDTExpr, error) {
	left, err := p.cmpExpr()
	if err != nil {
		return nil, err
	}
	for p.atName("and") {
		p.i++
		right, err := p.cmpExpr()
		if err != nil {
			return nil, err
		}
		left = &ptDTExpr{op: ptOpAnd, args: []*ptDTExpr{left, right}}
	}
	return left, nil
}

func (p *ptDTParser) cmpExpr() (*ptDTExpr, error) {
	left, err := p.additive()
	if err != nil {
		return nil, err
	}
	op := p.peek(0)
	if op.kind != ptTokPunct {
		return left, nil
	}
	switch op.text {
	case "=", "!=", "<", "<=", ">", ">=":
	default:
		return left, nil
	}
	p.i++
	right, err := p.additive()
	if err != nil {
		return nil, err
	}
	return &ptDTExpr{op: ptOpCompare, relop: op.text, args: []*ptDTExpr{left, right}}, nil
}

func (p *ptDTParser) additive() (*ptDTExpr, error) {
	left, err := p.multiplicative()
	if err != nil {
		return nil, err
	}
	for p.at("+") || p.at("-") {
		op := p.peek(0).text
		p.i++
		right, err := p.multiplicative()
		if err != nil {
			return nil, err
		}
		left = &ptDTExpr{op: ptOpArith, relop: op, args: []*ptDTExpr{left, right}}
	}
	return left, nil
}

func (p *ptDTParser) multiplicative() (*ptDTExpr, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.at("*") || p.atName("div") {
		op := p.peek(0).text
		p.i++
		right, err := p.unary()
		if err != nil {
			return nil, err
		}
		left = &ptDTExpr{op: ptOpArith, relop: op, args: []*ptDTExpr{left, right}}
	}
	return left, nil
}

func (p *ptDTParser) unary() (*ptDTExpr, error) {
	if p.at("-") {
		p.i++
		inner, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &ptDTExpr{op: ptOpNeg, args: []*ptDTExpr{inner}}, nil
	}
	e, err := p.unionExpr()
	if err != nil {
		return nil, err
	}
	// XPath 2.0 puts `castable as` between the unary operators and the
	// multiplicative ones, so it binds tighter than every comparison. ANAF writes
	// it parenthesised — `(string(.) castable as xs:date)` — so the precedence is
	// not load-bearing in this rule set, but getting it wrong silently would be.
	for p.atName("castable") {
		p.i++
		if !p.atName("as") {
			return nil, fmt.Errorf("expected 'as' after 'castable', found %q", p.peek(0).text)
		}
		p.i++
		t := p.peek(0)
		if t.kind != ptTokName {
			return nil, fmt.Errorf("'castable as' must name a type, found %q", t.text)
		}
		p.i++
		e = &ptDTExpr{op: ptOpCastable, name: t.text, args: []*ptDTExpr{e}}
	}
	return e, nil
}

func (p *ptDTParser) unionExpr() (*ptDTExpr, error) {
	left, err := p.valueExpr()
	if err != nil {
		return nil, err
	}
	for p.at("|") {
		p.i++
		right, err := p.valueExpr()
		if err != nil {
			return nil, err
		}
		left = &ptDTExpr{op: ptOpUnion, args: []*ptDTExpr{left, right}}
	}
	return left, nil
}

// valueExpr is a path expression, which subsumes the primary expressions: a
// literal, a number, a variable, a parenthesised expression and a function call
// can all head a path, and several of them do.
func (p *ptDTParser) valueExpr() (*ptDTExpr, error) {
	path := &ptDTXPath{}
	switch {
	case p.at("//"):
		p.i++
		path.fromRoot = true
	case p.at("/"):
		p.i++
		path.fromRoot = true
		path.up = -1 // absolute from the document element rather than descendant-or-self
	}
	for p.at("..") {
		p.i++
		path.up++
		if !p.at("/") {
			if path.up > 0 && len(path.steps) == 0 {
				return &ptDTExpr{op: ptOpPath, path: path}, nil
			}
			return nil, fmt.Errorf("expected '/' after '..'")
		}
		p.i++
	}
	first := true
	for {
		s, primary, err := p.step(first && !path.fromRoot && path.up == 0)
		if err != nil {
			return nil, err
		}
		if primary != nil {
			// A parenthesised expression, a literal, a number or a variable at the
			// head of the path. It is only a path if a '/' follows; otherwise it is
			// the whole expression.
			if !p.at("/") && len(path.steps) == 0 && !path.fromRoot && path.up == 0 {
				return primary, nil
			}
			path.steps = append(path.steps, ptDTStep{call: primary})
		} else {
			path.steps = append(path.steps, s)
		}
		first = false
		if !p.at("/") {
			break
		}
		p.i++
	}
	return &ptDTExpr{op: ptOpPath, path: path}, nil
}

// step parses one step. It returns a non-nil second result when the step is a
// primary expression that may or may not head a path; head says whether this is
// the first step of a relative path, which is the only position such an expression
// may appear in.
func (p *ptDTParser) step(head bool) (ptDTStep, *ptDTExpr, error) {
	t := p.peek(0)
	punct := t.kind == ptTokPunct
	switch {
	case punct && t.text == "@":
		p.i++
		n := p.peek(0)
		if n.kind != ptTokName {
			return ptDTStep{}, nil, fmt.Errorf("'@' must be followed by an attribute name, found %q", n.text)
		}
		p.i++
		s := ptDTStep{attr: ptDTLocal(n.text)}
		preds, err := p.predicates()
		s.preds = preds
		return s, nil, err
	case punct && t.text == ".":
		p.i++
		s := ptDTStep{self: true}
		preds, err := p.predicates()
		s.preds = preds
		return s, nil, err
	case t.kind == ptTokAxis:
		p.i++
		n := p.peek(0)
		if n.kind != ptTokName {
			return ptDTStep{}, nil, fmt.Errorf("the %s axis must be followed by a name, found %q", t.text, n.text)
		}
		p.i++
		var s ptDTStep
		switch t.text {
		case "ancestor":
			s = ptDTStep{name: ptDTLocal(n.text), ancestor: true}
		case "child":
			s = ptDTStep{name: ptDTLocal(n.text)}
		default:
			return ptDTStep{}, nil, fmt.Errorf("unsupported axis %q", t.text)
		}
		preds, err := p.predicates()
		s.preds = preds
		return s, nil, err
	case t.kind == ptTokName && p.peek(1).kind == ptTokPunct && p.peek(1).text == "(":
		name := t.text
		p.i += 2
		var args []*ptDTExpr
		if !p.at(")") {
			for {
				a, err := p.expr()
				if err != nil {
					return ptDTStep{}, nil, err
				}
				args = append(args, a)
				if !p.at(",") {
					break
				}
				p.i++
			}
		}
		if err := p.eat(")"); err != nil {
			return ptDTStep{}, nil, err
		}
		call := &ptDTExpr{op: ptOpFunc, fn: name, args: args}
		if preds, err := p.predicates(); err != nil {
			return ptDTStep{}, nil, err
		} else if len(preds) > 0 {
			return ptDTStep{call: call, preds: preds}, nil, nil
		}
		if head {
			return ptDTStep{}, call, nil
		}
		return ptDTStep{call: call}, nil, nil
	case t.kind == ptTokName:
		p.i++
		s := ptDTStep{name: ptDTLocal(t.text)}
		preds, err := p.predicates()
		s.preds = preds
		return s, nil, err
	case t.kind == ptTokVar:
		p.i++
		e := &ptDTExpr{op: ptOpVar, name: t.text}
		if preds, err := p.predicates(); err != nil {
			return ptDTStep{}, nil, err
		} else if len(preds) > 0 {
			return ptDTStep{call: e, preds: preds}, nil, nil
		}
		if head {
			return ptDTStep{}, e, nil
		}
		return ptDTStep{call: e}, nil, nil
	case t.kind == ptTokLiteral:
		p.i++
		e := &ptDTExpr{op: ptOpLiteral, str: t.text}
		if !head {
			return ptDTStep{}, nil, fmt.Errorf("a string literal cannot be a path step")
		}
		return ptDTStep{}, e, nil
	case t.kind == ptTokNumber:
		p.i++
		n, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return ptDTStep{}, nil, fmt.Errorf("bad number %q", t.text)
		}
		e := &ptDTExpr{op: ptOpNumber, num: n}
		if !head {
			return ptDTStep{}, nil, fmt.Errorf("a number cannot be a path step")
		}
		return ptDTStep{}, e, nil
	case punct && t.text == "(":
		p.i++
		var items []*ptDTExpr
		for {
			a, err := p.expr()
			if err != nil {
				return ptDTStep{}, nil, err
			}
			items = append(items, a)
			if !p.at(",") {
				break
			}
			p.i++
		}
		if err := p.eat(")"); err != nil {
			return ptDTStep{}, nil, err
		}
		inner := items[0]
		for _, it := range items[1:] {
			inner = &ptDTExpr{op: ptOpUnion, args: []*ptDTExpr{inner, it}}
		}
		if preds, err := p.predicates(); err != nil {
			return ptDTStep{}, nil, err
		} else if len(preds) > 0 {
			return ptDTStep{call: inner, preds: preds}, nil, nil
		}
		if head {
			return ptDTStep{}, inner, nil
		}
		return ptDTStep{call: inner}, nil, nil
	}
	return ptDTStep{}, nil, fmt.Errorf("expected a step, found %q", t.text)
}

func (p *ptDTParser) predicates() ([]*ptDTExpr, error) {
	var out []*ptDTExpr
	for p.at("[") {
		p.i++
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		if err := p.eat("]"); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// ptDTLocal drops a QName's prefix. parseEN16931 keys on local names throughout, so
// the prefix carries no information here — with one consequence worth naming: a
// context written `//ubl:Invoice/...` and one written `//cn:CreditNote/...` are told
// apart by the document element's name and not by its namespace, which is what
// ptDTPath.root records.
func ptDTLocal(q string) string {
	if i := strings.IndexByte(q, ':'); i >= 0 {
		return q[i+1:]
	}
	return q
}
