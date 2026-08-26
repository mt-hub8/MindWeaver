package formalis

import (
	"fmt"
	"strconv"
	"strings"
)

// This file evaluates the advisory half of CEN's two EN 16931 syntax bindings:
// the 676 UBL-CR-* and 21 UBL-DT-* assertions of EN16931-syntax.sch, and the 440
// CII-SR-* and 31 CII-DT-* assertions of EN16931-CII-syntax.sch, that CEN flags
// warning rather than fatal. The fatal halves are transcribed by hand in
// en16931_ubl_rules.go and en16931_cii_rules.go, where the rules that need
// judgement live; these 1,168 do not need judgement, they need transcribing
// without error, which is the opposite problem.
//
// # Why they are generated rather than written
//
// Almost every one of them is a single forbidden path — not(cbc:UUID),
// not(cac:OrderReference/cbc:IssueTime), not(//@schemeName) — and their purpose
// as a family is to hold a UBL or CII document down to the EN 16931 core subset:
// UBL 2.1 and CII D16B are far larger vocabularies than the semantic model, and
// CEN enumerates, element by element, what a conforming invoice leaves out. A
// thousand hand-written one-line rules would be a thousand chances to mistype an
// element name, and a mistyped name is invisible: the rule simply never fires,
// and a test suite that has no failing fragment for it — the CEN unit-test suite
// ships error fragments for almost no binding rule — cannot tell. So the table
// is generated from the Schematron by testdata/en16931-syntax-rules/gen.py and
// re-derived from it by en16931_syntax_advisory_test.go, which is the same
// arrangement en16931_codelists.go has with the genericode bundle, and for the
// same reason.
//
// Each assertion's test in the generated table is CEN's own XPath,
// whitespace-normalised and otherwise verbatim. That is the whole fidelity
// story: there is no translation step in which a rule could quietly change
// meaning, the table can be read against the Schematron line by line, and the
// drift test is a string comparison rather than a claim about a transformation.
// What this file adds is a parser and evaluator for the XPath subset those 1,168
// expressions use, and a refusal to parse anything outside it.
//
// # Severity
//
// Every finding here is SeverityWarning, quoted from CEN's flag. They are the
// first advisory findings this package emits, and they change what
// len(r.Violations) == 0 means: a Peppol invoice legitimately carrying
// cbc:UBLVersionID now produces a finding where before it produced none. A
// caller's release gate belongs on Report.Fatal or Report.Conformant, neither of
// which these can move.
//
// # Rule order, and the rules CEN cannot report
//
// Under ISO Schematron a node is processed by the first rule in a pattern whose
// context matches it, and every assertion in a binding lives in one pattern. The
// generated table therefore holds *every* rule of the pattern in order,
// including the rules whose assertions are all fatal and are evaluated
// elsewhere: such a rule carries no assertion here, but its context still claims
// its nodes, and a later rule matching the same node never runs. That is not an
// optimisation, it is the semantics — and it is where CII-DT-010/011/012 go. CEN
// flags those three fatal and binds them to
// /rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:TypeCode, which //ram:TypeCode
// (CII-DT-008/009) has already claimed. No conforming processor reports them, and
// with the ordering modelled here neither does this package, without anyone
// having to remember it.
//
// # Source
//
// SourceEN16931, for the reason en16931_ubl_rules.go gives: CEN publishes the
// bindings as normative parts of the same standard as the semantic model.

// advisorySyntaxRule is one <rule> of a binding's abstract syntax pattern: the
// context that selects its nodes, and the assertions CEN flags warning under it.
// The generated table holds these in pattern order.
type advisorySyntaxRule struct {
	// context is CEN's <rule context> with the binding's parameters resolved and
	// whitespace collapsed. It is carried for the fidelity test to compare and
	// for a reader to look up; match is what the evaluator uses.
	context string

	// match describes the same node population as context, in the terms this
	// package's namespace-blind element tree can test.
	match advisoryMatch

	// asserts are the advisory assertions under this rule, in document order. It
	// is empty for a rule whose assertions CEN all flags fatal; the entry is
	// still here because its context claims nodes away from the rules below it.
	asserts []advisorySyntaxAssert
}

// advisorySyntaxAssert is one <assert flag="warning">: CEN's identifier, CEN's
// XPath verbatim, and CEN's own text with the leading "[rule-id]-" stripped.
type advisorySyntaxAssert struct {
	id      string
	test    string
	message string
}

// advisoryPred is the handful of context predicates CEN's syntax patterns use.
// They are named rather than parsed because there are four of them and each
// carries a semantic subtlety worth stating once; gen.py fails on a context
// whose predicate is not one of these.
type advisoryPred int

const (
	// predNone is a context with no predicate.
	predNone advisoryPred = iota
	// predChargeIndicatorTrue and predChargeIndicatorFalse are
	// `[cbc:ChargeIndicator = true()]` and `[cbc:ChargeIndicator = false()]`,
	// which tell a document-level charge from a document-level allowance. The
	// comparison is against xs:boolean under queryBinding="xslt2", so the element's
	// value is cast: "true" and "1" are true, "false" and "0" are false, and
	// anything else matches neither context. ublChargeIndicator is that reading,
	// and it is shared with the fatal UBL-SR-30/31 rules so the two cannot drift.
	predChargeIndicatorTrue
	predChargeIndicatorFalse
	// predFormat102 is `[@format = '102']` on udt:DateTimeString: the CCTS date
	// format the CII binding constrains.
	predFormat102
)

// advisoryMatch describes a Schematron rule context as a test on one element.
//
// These are XSLT match patterns and not paths: `cac:Delivery` matches a
// cac:Delivery anywhere in the document, and only the contexts CEN writes as an
// absolute path from the document element are anchored. The fields are
// conjunctive — every one that is set must hold — and gen.py refuses a context
// it cannot express in them, so a context whose population this evaluator would
// get wrong stops the generator rather than silently mis-scoping every assertion
// under it.
//
// parseCII keys on local names and discards namespaces, so no prefix reaches
// here. That drops the `ram:*` restriction on five CII contexts; over the whole
// conformance corpus every element in a CrossIndustryInvoice whose local name
// ends in ID, Amount, Quantity, TradeTax or ReferencedDocument is in the ram
// namespace, so the restriction selects nothing the local-name test does not.
type advisoryMatch struct {
	// names is the set of element local names the context accepts. It is empty
	// for a context written as a name suffix.
	names []string
	// notNames is excluded by local name: `not(self::ram:TaxTotalAmount)`.
	notNames []string
	// suffix and notSuffix are `ends-with(name(), ...)` and its negation, which
	// is how CEN writes the datatype contexts ("every element whose name ends in
	// Amount, other than one ending in PriceAmount").
	suffix    string
	notSuffix string
	// parent is a required parent local name, for the two contexts written as a
	// two-step relative pattern (cac:AccountingSupplierParty/cac:Party).
	parent string
	// paths are absolute paths from the document element, including its own name,
	// any one of which the element's ancestor chain may equal. A context written
	// as a union of absolute paths has several.
	paths [][]string
	// documentElement restricts the match to the document element itself, which
	// is what `/ubl:Invoice | /cn:CreditNote` and `/rsm:CrossIndustryInvoice`
	// select. names carries the accepted root names.
	documentElement bool
	// pred is the context's predicate, if it has one.
	pred advisoryPred
	// notAncestorChild excludes an element that has an ancestor named [0]
	// carrying a child named [1]: `not(ancestor::cac:Price/cac:AllowanceCharge)`,
	// which keeps the amount datatype rule off an item price discount.
	notAncestorChild [2]string
}

// matches reports whether the context selects n. stack is n's ancestor chain
// from the document element down to and including n, which is what the absolute
// and ancestor-relative fields are tested against.
func (m advisoryMatch) matches(n *ciiNode, stack []*ciiNode) bool {
	if m.documentElement && len(stack) != 1 {
		return false
	}
	if len(m.names) > 0 && !containsString(m.names, n.name) {
		return false
	}
	if len(m.notNames) > 0 && containsString(m.notNames, n.name) {
		return false
	}
	if m.suffix != "" && !strings.HasSuffix(n.name, m.suffix) {
		return false
	}
	if m.notSuffix != "" && strings.HasSuffix(n.name, m.notSuffix) {
		return false
	}
	if m.parent != "" && (len(stack) < 2 || stack[len(stack)-2].name != m.parent) {
		return false
	}
	if len(m.paths) > 0 && !anyPathEquals(m.paths, stack) {
		return false
	}
	switch m.pred {
	case predChargeIndicatorTrue:
		if ublChargeIndicator(n) != ublIndicatorCharge {
			return false
		}
	case predChargeIndicatorFalse:
		if ublChargeIndicator(n) != ublIndicatorAllowance {
			return false
		}
	case predFormat102:
		if strings.TrimSpace(n.attr("format")) != "102" {
			return false
		}
	}
	if m.notAncestorChild[0] != "" {
		for _, a := range stack[:len(stack)-1] {
			if a.name == m.notAncestorChild[0] && len(a.all(m.notAncestorChild[1])) > 0 {
				return false
			}
		}
	}
	return true
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// anyPathEquals reports whether the ancestor chain is exactly one of the paths.
func anyPathEquals(paths [][]string, stack []*ciiNode) bool {
	for _, p := range paths {
		if len(p) != len(stack) {
			continue
		}
		ok := true
		for i, name := range p {
			if stack[i].name != name {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The expression language
//
// The subset of XPath the 1,168 advisory assertions use, and nothing more. It is
// deliberately small: 1,141 of them are a single not(path), and the remaining 27
// add general comparison, count(), the `-` of one count from another, self:: and
// ancestor::, a parent step, and predicates. advisorySyntaxParse refuses
// anything else, which is what keeps "the evaluator understands the whole table"
// a checked fact rather than an assumption.
// ---------------------------------------------------------------------------

type advisoryOp int

const (
	advOr advisoryOp = iota
	advAnd
	advNot
	// advExists is a location path in boolean context: true when it selects
	// anything.
	advExists
	// advEq and advNe are XPath general comparisons between a path and a value.
	// Their asymmetry on an empty left side is the point: `a = 'x'` and
	// `a != 'x'` are *both* false when a selects nothing, because a general
	// comparison quantifies over the left sequence. A rule written
	// `not(a) or a = '2.1'` therefore passes on a document with no a, and a rule
	// written `a != 'x'` fails on one.
	advEq
	advNe
	// advRel is a numeric relation, `count(...) <= 1` and its siblings.
	advRel
	// advCount is count(path); advSub is one numeric expression minus another.
	advCount
	advSub
	// advNormSpace is normalize-space(path), which only ever appears as the left
	// side of a comparison.
	advNormSpace
	// advSelf and advAncestor are `self::name` and `ancestor::name`.
	advSelf
	advAncestor
	advNumber
	advLiteral
	advBool
)

// advisoryExpr is a node of a parsed assertion.
type advisoryExpr struct {
	op    advisoryOp
	args  []*advisoryExpr
	path  *advisoryPath
	num   float64
	str   string
	b     bool
	relop string
}

// advisoryPath is a location path: where it starts, and the steps from there.
type advisoryPath struct {
	// fromRoot is a leading `//`. In XPath that is an *absolute* step —
	// `/descendant-or-self::node()/` — so such a path is document-wide whatever
	// the context node is, which is how twenty-one UBL-DT-* rules bound to the
	// document element reach every attribute in the invoice.
	fromRoot bool
	// fromParent is a leading `../`. One rule uses it: UBL-CR-412 exempts a
	// credit note with `../cn:CreditNote`, read from the document element, whose
	// parent is the document node. advisoryEntry carries a synthetic parent for
	// the document element so that step means what XPath says it means.
	fromParent bool
	steps      []advisoryStep
}

// advisoryStep is one step: an element name (or a union of alternatives), or an
// attribute.
type advisoryStep struct {
	names []string
	attr  string
	pred  *advisoryExpr
}

// advisoryRef is one item of a selected sequence: an element, or an attribute of
// one. XPath treats an attribute as a node, and several rules count or forbid
// attributes rather than elements, so the two travel together.
type advisoryRef struct {
	node *ciiNode
	attr string
}

// value is the ref's string value: the element's text, or the attribute's.
func (r advisoryRef) value() string {
	if r.attr != "" {
		return r.node.attr(r.attr)
	}
	return r.node.text
}

// --- lexer ---------------------------------------------------------------

type advisoryToken struct {
	kind advisoryTokenKind
	text string
}

type advisoryTokenKind int

const (
	tokEOF advisoryTokenKind = iota
	tokName
	tokLiteral
	tokNumber
	tokAxis
	tokPunct
)

func advisoryLex(s string) ([]advisoryToken, error) {
	var out []advisoryToken
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unterminated string literal at offset %d", i)
			}
			out = append(out, advisoryToken{tokLiteral, s[i+1 : i+1+j]})
			i += j + 2
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			out = append(out, advisoryToken{tokNumber, s[i:j]})
			i = j
		case isAdvisoryNameStart(c):
			j := i
			for j < len(s) && isAdvisoryNameChar(s[j]) {
				j++
			}
			// A QName's prefix, and the `::` of an axis, both follow a name with a
			// colon; the axis is the one where a second colon follows.
			if j+1 < len(s) && s[j] == ':' && s[j+1] == ':' {
				out = append(out, advisoryToken{tokAxis, s[i:j]})
				i = j + 2
				continue
			}
			if j < len(s) && s[j] == ':' && j+1 < len(s) && (isAdvisoryNameStart(s[j+1]) || s[j+1] == '*') {
				j++
				for j < len(s) && (isAdvisoryNameChar(s[j]) || s[j] == '*') {
					j++
				}
			}
			out = append(out, advisoryToken{tokName, s[i:j]})
			i = j
		default:
			for _, p := range []string{"<=", ">=", "!=", "//", ".."} {
				if strings.HasPrefix(s[i:], p) {
					out = append(out, advisoryToken{tokPunct, p})
					i += 2
					goto next
				}
			}
			if strings.IndexByte("=<>/()[]@|-", c) < 0 {
				return nil, fmt.Errorf("unexpected character %q at offset %d", c, i)
			}
			out = append(out, advisoryToken{tokPunct, s[i : i+1]})
			i++
		}
	next:
	}
	return append(out, advisoryToken{tokEOF, ""}), nil
}

func isAdvisoryNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func isAdvisoryNameChar(c byte) bool {
	return isAdvisoryNameStart(c) || c >= '0' && c <= '9' || c == '.' || c == '-'
}

// --- parser --------------------------------------------------------------

type advisoryParser struct {
	toks []advisoryToken
	i    int
}

// advisorySyntaxParse parses one assertion. An error means the expression is
// outside the subset this evaluator implements, which is a rule that would go
// unchecked: it is a hard failure at load, not a skip. gen.py refuses the same
// expressions before they are ever written, and
// TestAdvisorySyntaxTableParses parses every committed row, so this error is
// reachable only by editing the generated table by hand.
func advisorySyntaxParse(s string) (*advisoryExpr, error) {
	toks, err := advisoryLex(s)
	if err != nil {
		return nil, err
	}
	p := &advisoryParser{toks: toks}
	e, err := p.expr()
	if err != nil {
		return nil, err
	}
	if p.peek(0).kind != tokEOF {
		return nil, fmt.Errorf("trailing input at %q", p.peek(0).text)
	}
	return e, nil
}

func (p *advisoryParser) peek(n int) advisoryToken {
	if p.i+n >= len(p.toks) {
		return advisoryToken{tokEOF, ""}
	}
	return p.toks[p.i+n]
}

func (p *advisoryParser) at(text string) bool { return p.peek(0).text == text }

func (p *advisoryParser) take() advisoryToken {
	t := p.peek(0)
	p.i++
	return t
}

func (p *advisoryParser) eat(text string) error {
	if !p.at(text) {
		return fmt.Errorf("expected %q, found %q", text, p.peek(0).text)
	}
	p.i++
	return nil
}

func (p *advisoryParser) expr() (*advisoryExpr, error) {
	left, err := p.andExpr()
	if err != nil {
		return nil, err
	}
	for p.at("or") {
		p.i++
		right, err := p.andExpr()
		if err != nil {
			return nil, err
		}
		left = &advisoryExpr{op: advOr, args: []*advisoryExpr{left, right}}
	}
	return left, nil
}

func (p *advisoryParser) andExpr() (*advisoryExpr, error) {
	left, err := p.cmpExpr()
	if err != nil {
		return nil, err
	}
	for p.at("and") {
		p.i++
		right, err := p.cmpExpr()
		if err != nil {
			return nil, err
		}
		left = &advisoryExpr{op: advAnd, args: []*advisoryExpr{left, right}}
	}
	return left, nil
}

func (p *advisoryParser) cmpExpr() (*advisoryExpr, error) {
	left, err := p.additive()
	if err != nil {
		return nil, err
	}
	op := p.peek(0)
	if op.kind != tokPunct {
		return left, nil
	}
	var kind advisoryOp
	switch op.text {
	case "=":
		kind = advEq
	case "!=":
		kind = advNe
	case "<=", ">=", "<", ">":
		kind = advRel
	default:
		return left, nil
	}
	p.i++
	right, err := p.additive()
	if err != nil {
		return nil, err
	}
	e := &advisoryExpr{op: kind, args: []*advisoryExpr{left, right}}
	if kind == advRel {
		e.relop = op.text
	}
	return e, nil
}

func (p *advisoryParser) additive() (*advisoryExpr, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.at("-") {
		p.i++
		right, err := p.unary()
		if err != nil {
			return nil, err
		}
		left = &advisoryExpr{op: advSub, args: []*advisoryExpr{left, right}}
	}
	return left, nil
}

func (p *advisoryParser) unary() (*advisoryExpr, error) {
	t := p.peek(0)
	if t.kind == tokName && p.peek(1).text == "(" {
		switch t.text {
		case "not":
			p.i += 2
			inner, err := p.expr()
			if err != nil {
				return nil, err
			}
			if err := p.eat(")"); err != nil {
				return nil, err
			}
			return &advisoryExpr{op: advNot, args: []*advisoryExpr{inner}}, nil
		case "count", "normalize-space":
			p.i += 2
			path, err := p.path()
			if err != nil {
				return nil, err
			}
			if err := p.eat(")"); err != nil {
				return nil, err
			}
			op := advCount
			if t.text == "normalize-space" {
				op = advNormSpace
			}
			return &advisoryExpr{op: op, path: path}, nil
		case "true", "false":
			p.i += 2
			if err := p.eat(")"); err != nil {
				return nil, err
			}
			return &advisoryExpr{op: advBool, b: t.text == "true"}, nil
		}
	}
	switch t.kind {
	case tokLiteral:
		p.i++
		return &advisoryExpr{op: advLiteral, str: t.text}, nil
	case tokNumber:
		p.i++
		n, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", t.text)
		}
		return &advisoryExpr{op: advNumber, num: n}, nil
	case tokAxis:
		p.i++
		name := p.take()
		if name.kind != tokName {
			return nil, fmt.Errorf("the %s axis must be followed by a name, found %q", t.text, name.text)
		}
		switch t.text {
		case "self":
			return &advisoryExpr{op: advSelf, str: advisoryLocal(name.text)}, nil
		case "ancestor":
			return &advisoryExpr{op: advAncestor, str: advisoryLocal(name.text)}, nil
		}
		return nil, fmt.Errorf("unsupported axis %q", t.text)
	}
	if t.text == "(" && !p.unionAhead() {
		p.i++
		inner, err := p.expr()
		if err != nil {
			return nil, err
		}
		if err := p.eat(")"); err != nil {
			return nil, err
		}
		return inner, nil
	}
	path, err := p.path()
	if err != nil {
		return nil, err
	}
	return &advisoryExpr{op: advExists, path: path}, nil
}

// unionAhead distinguishes `(a|b)/c`, a union of name alternatives at the head
// of a path, from `(expr)`, a parenthesised subexpression.
func (p *advisoryParser) unionAhead() bool {
	return p.peek(1).kind == tokName && p.peek(2).text == "|"
}

func (p *advisoryParser) path() (*advisoryPath, error) {
	path := &advisoryPath{}
	if p.at("//") {
		p.i++
		path.fromRoot = true
	} else if p.at("..") {
		p.i++
		if err := p.eat("/"); err != nil {
			return nil, err
		}
		path.fromParent = true
	}
	if p.at("(") && p.unionAhead() {
		p.i++
		var names []string
		for {
			t := p.take()
			if t.kind != tokName {
				return nil, fmt.Errorf("a union alternative must be a name, found %q", t.text)
			}
			names = append(names, advisoryLocal(t.text))
			if !p.at("|") {
				break
			}
			p.i++
		}
		if err := p.eat(")"); err != nil {
			return nil, err
		}
		path.steps = append(path.steps, advisoryStep{names: names})
	} else {
		s, err := p.step()
		if err != nil {
			return nil, err
		}
		path.steps = append(path.steps, s)
	}
	for p.at("/") {
		p.i++
		s, err := p.step()
		if err != nil {
			return nil, err
		}
		path.steps = append(path.steps, s)
	}
	return path, nil
}

func (p *advisoryParser) step() (advisoryStep, error) {
	if p.at("@") {
		p.i++
		t := p.take()
		if t.kind != tokName {
			return advisoryStep{}, fmt.Errorf("@ must be followed by an attribute name, found %q", t.text)
		}
		return advisoryStep{attr: advisoryLocal(t.text)}, nil
	}
	t := p.take()
	if t.kind != tokName {
		return advisoryStep{}, fmt.Errorf("expected an element name, found %q", t.text)
	}
	s := advisoryStep{names: []string{advisoryLocal(t.text)}}
	if p.at("[") {
		p.i++
		pred, err := p.expr()
		if err != nil {
			return advisoryStep{}, err
		}
		if err := p.eat("]"); err != nil {
			return advisoryStep{}, err
		}
		s.pred = pred
	}
	return s, nil
}

// advisoryLocal drops a QName's prefix. parseCII keys on local names throughout,
// so the prefix carries no information here; see advisoryMatch for the one place
// that costs something.
func advisoryLocal(q string) string {
	if i := strings.IndexByte(q, ':'); i >= 0 {
		return q[i+1:]
	}
	return q
}
