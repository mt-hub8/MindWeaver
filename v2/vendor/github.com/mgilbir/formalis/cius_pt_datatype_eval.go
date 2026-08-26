package formalis

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The compiler and evaluator for the generated CIUS-PT datatype tables.
//
// Three things happen here and they are separate on purpose:
//
//   - ptDTCompile turns a generated table into a form that can be run. Every
//     assertion's XPath, every <let> and every context predicate is parsed once at
//     load, every regular expression is compiled once at load, and anything that
//     cannot be is a panic rather than a rule that quietly never fires.
//   - ptDTGather walks the document once and assigns each element to the first rule
//     of the pattern whose context matches it, which is ISO Schematron's semantics.
//   - ptDTRun evaluates each claimed element's assertions.
//
// # One walk, not 291 XPath evaluations
//
// The naive shape — resolve 173 context paths and then evaluate 291 expressions
// against the tree — walks the document a few hundred times per invoice, and
// corpusSweep puts 1,690 documents through ValidateCIUSPT on every test run. Two
// things keep it to one walk.
//
// Every context in both patterns is a union of paths anchored at the document
// element (gen.py refuses one that is not), so the whole set compiles to a trie
// keyed by element local name. The walk carries the set of trie nodes the current
// element's ancestor path has reached; an element whose name leaves the trie prunes
// its entire subtree, which is what most of a UBL invoice does.
//
// And an assertion is evaluated only for elements a rule actually claimed. A rule
// with no claimed element costs nothing beyond the trie edges it contributed.

// ---------------------------------------------------------------------------
// The generated table's shape
// ---------------------------------------------------------------------------

// ptDTPattern is one Schematron pattern of AT/eSPap's CIUS-PT: its rules in
// document order, and the pattern-level <let> declarations in scope for all of them.
type ptDTPattern struct {
	name  string
	lets  []ptDTLetSrc
	rules []ptDTRuleSrc
}

// ptDTLetSrc is one <let>: the variable's name and AT's XPath for its value,
// verbatim.
type ptDTLetSrc struct{ name, value string }

// ptDTRuleSrc is one <rule>. context is AT's own <rule context> with the abstract
// pattern's parameters resolved and whitespace collapsed, carried for the drift
// test to compare and for a reader to look up; paths is the same node population
// expressed as root-anchored steps, which is what the walk uses.
type ptDTRuleSrc struct {
	context string
	paths   []ptDTCtxPath
	lets    []ptDTLetSrc
	asserts []ptDTAssertSrc
}

// ptDTCtxPath is one branch of a context union.
//
// root says where the branch is anchored, and it takes four values:
//
//   - "Invoice" or "CreditNote" — the branch is anchored at that document element,
//     and applies only to a document with that root. That distinction is
//     load-bearing: DT-CIUS-PT-003 is two assertions because an invoice type code
//     and a credit-note type code are different elements, and DT-CIUS-PT-008 binds
//     the invoice's cbc:DueDate against the credit note's
//     cac:PaymentMeans/cbc:PaymentDueDate, which is not the same place at all.
//   - "" — anchored at the document element, whichever it is.
//   - ptDTFloating — an XSLT match pattern that may begin at any element, which is
//     what a Schematron <rule context> written without a leading slash actually
//     means. CIUS-PT's generator refuses such a branch and emits the narrower
//     reading; CIUS-RO's contexts are written that way throughout (cbc:IssueDate,
//     cac:TaxTotal/cac:TaxSubtotal, //cac:PaymentMeans) and cannot be read any
//     other way without changing which nodes they claim.
type ptDTCtxPath struct {
	root  string
	steps []ptDTCtxStep
}

// ptDTFloating marks a context branch that is an XSLT match pattern rather than a
// path from the document element: it matches an element whose own name and whose
// ancestors' names end with the branch's steps, at any depth.
//
// It is spelled as a value of ptDTCtxPath.root rather than as a fourth field so
// that the generated CIUS-PT table, which writes ptDTCtxPath as a positional
// literal in several thousand places, does not have to be rewritten to say
// "not floating" everywhere. A compiled ptDTTerm never carries it: a floating
// branch's nodes are found through the pattern's second trie and its term's root
// is "", because a match pattern applies to both document elements.
const ptDTFloating = "//"

// ptDTCtxStep is one step of a context path: an element local name, and the step's
// predicate as AT wrote it (empty when it has none).
type ptDTCtxStep struct {
	name string
	pred string
}

// ptDTAssertSrc is one <assert> or <report>: AT's identifier, the polarity, AT's
// XPath verbatim, and AT's own text with the leading "[rule-id]-" stripped.
//
// The polarity is the field to read first. An <assert> fires when its test is
// *false* and a <report> when it is *true*, so the same expression under the wrong
// element is the rule inverted — and 91 of these 291 are reports.
type ptDTAssertSrc struct {
	id      string
	kind    string // "assert" or "report"
	test    string
	message string
}

// ---------------------------------------------------------------------------
// Compilation
// ---------------------------------------------------------------------------

type ptDTCompiledPattern struct {
	name  string
	lets  []ptDTCompiledLet
	rules []ptDTCompiledRule
	trie  *ptDTTrieNode
	// floating is the second trie: the branches that are match patterns rather
	// than paths from the document element. It is consulted afresh at every
	// element, so a branch of n steps matches wherever the last n names on the
	// path from the root agree with it. It has no edges at all for CIUS-PT.
	floating *ptDTTrieNode
	// rootTerms are the rules whose context is the document element itself.
	rootTerms []ptDTTerm
	// asserts is the number of assertions the pattern holds, which the tests
	// ratchet.
	asserts int
	// rootNames are the element local names the `//` paths in this pattern start
	// from — the only names ptDTDoc has to index.
	rootNames map[string]bool
}

type ptDTCompiledLet struct {
	name string
	expr *ptDTExpr
}

type ptDTCompiledRule struct {
	context string
	lets    []ptDTCompiledLet
	asserts []ptDTCompiledAssert
	// needsAncestors is set when some assertion or <let> of this rule uses the
	// ancestor axis or a `../` step, and decides whether the walk copies the
	// element's ancestor chain. Six rules of the datatype pattern and all of the
	// condition pattern's need it; the rest do not pay for it.
	needsAncestors bool
}

type ptDTCompiledAssert struct {
	id      string
	report  bool // true for <report>: fires when the test is true
	test    string
	message string
	expr    *ptDTExpr
}

// ptDTTrieNode is one node of the context trie. Pattern order is rule-index order,
// so the smallest index among the terminals an element reaches is the rule ISO
// Schematron gives it.
type ptDTTrieNode struct {
	edges []ptDTTrieEdge
	terms []ptDTTerm
}

type ptDTTrieEdge struct {
	name string
	// predSrc is the step predicate as AT wrote it, and it is what decides whether
	// two contexts share an edge: cac:AllowanceCharge[cbc:ChargeIndicator='true']
	// and cac:AllowanceCharge[cbc:ChargeIndicator='false'] are two edges of the
	// same name and must stay two.
	predSrc string
	pred    *ptDTExpr
	next    *ptDTTrieNode
}

type ptDTTerm struct {
	rule int
	root string
}

// The two compiled patterns, built at load from the generated tables and only read
// afterwards, so they are safe to share across goroutines.
//
// A parse failure panics. That is the same choice regexp.MustCompile makes and for
// the same reason: the table is generated and committed, so a row this evaluator
// cannot read is a defect in the build rather than anything a caller did,
// TestCIUSPTDatatypeTableParses fails on it before it can ship, and the
// alternative — dropping the row — is a rule that silently stops being checked.
var (
	ptDatatype  = ptDTMustCompile(ptDatatypePattern)
	ptCondition = ptDTMustCompile(ptConditionPattern)
)

func ptDTMustCompile(p ptDTPattern) *ptDTCompiledPattern {
	c, err := ptDTCompile(p)
	if err != nil {
		panic("formalis: " + p.name + ": " + err.Error())
	}
	return c
}

func ptDTCompile(p ptDTPattern) (*ptDTCompiledPattern, error) {
	out := &ptDTCompiledPattern{
		name:      p.name,
		trie:      &ptDTTrieNode{},
		floating:  &ptDTTrieNode{},
		rootNames: map[string]bool{},
	}
	var err error
	if out.lets, err = ptDTCompileLets(p.lets); err != nil {
		return nil, err
	}
	for _, l := range out.lets {
		ptDTCollectRootNames(l.expr, out.rootNames)
	}
	for i, r := range p.rules {
		cr := ptDTCompiledRule{context: r.context}
		ruleLets, lerr := ptDTCompileLets(r.lets)
		if lerr != nil {
			return nil, fmt.Errorf("%s: %w", r.context, lerr)
		}
		// The pattern-level <let>s are in scope for every rule, so each rule holds
		// the merged list. Merging here rather than per matched element matters:
		// the datatype pattern declares two of them and claims a couple of hundred
		// elements per invoice.
		cr.lets = append(append([]ptDTCompiledLet{}, out.lets...), ruleLets...)
		for _, l := range cr.lets {
			cr.needsAncestors = cr.needsAncestors || ptDTUsesAncestors(l.expr)
			ptDTCollectRootNames(l.expr, out.rootNames)
		}
		for _, a := range r.asserts {
			e, perr := ptDTParse(a.test)
			if perr != nil {
				return nil, fmt.Errorf("%s: %s: %w", a.id, a.test, perr)
			}
			if perr := ptDTCheck(e); perr != nil {
				return nil, fmt.Errorf("%s: %s: %w", a.id, a.test, perr)
			}
			cr.needsAncestors = cr.needsAncestors || ptDTUsesAncestors(e)
			ptDTCollectRootNames(e, out.rootNames)
			cr.asserts = append(cr.asserts, ptDTCompiledAssert{
				id: a.id, report: a.kind == "report", test: a.test, message: a.message, expr: e,
			})
			out.asserts++
		}
		out.rules = append(out.rules, cr)

		for _, path := range r.paths {
			node := out.trie
			if path.root == ptDTFloating {
				if len(path.steps) == 0 {
					return nil, fmt.Errorf("%s: a floating context branch with no step matches every element", r.context)
				}
				node = out.floating
			}
			for _, s := range path.steps {
				var pred *ptDTExpr
				if s.pred != "" {
					if pred, err = ptDTParse(s.pred); err != nil {
						return nil, fmt.Errorf("%s: context predicate [%s]: %w", r.context, s.pred, err)
					}
					if err = ptDTCheck(pred); err != nil {
						return nil, fmt.Errorf("%s: context predicate [%s]: %w", r.context, s.pred, err)
					}
				}
				node = ptDTTrieEdgeFor(node, s.name, s.pred, pred)
			}
			// A floating branch applies to both document elements, so its term
			// carries no root requirement; the sentinel exists only in the table.
			term := ptDTTerm{rule: i, root: path.root}
			if path.root == ptDTFloating {
				term.root = ""
			}
			if len(path.steps) == 0 {
				out.rootTerms = append(out.rootTerms, term)
			} else {
				node.terms = append(node.terms, term)
			}
		}
	}
	return out, nil
}

// ptDTTrieEdgeFor returns the edge for one step, reusing an existing one with the
// same name and the same predicate source so that two contexts sharing a prefix
// share the walk.
func ptDTTrieEdgeFor(n *ptDTTrieNode, name, predSrc string, pred *ptDTExpr) *ptDTTrieNode {
	for i := range n.edges {
		if n.edges[i].name == name && n.edges[i].predSrc == predSrc {
			return n.edges[i].next
		}
	}
	next := &ptDTTrieNode{}
	n.edges = append(n.edges, ptDTTrieEdge{name: name, predSrc: predSrc, pred: pred, next: next})
	return next
}

func ptDTCompileLets(lets []ptDTLetSrc) ([]ptDTCompiledLet, error) {
	out := make([]ptDTCompiledLet, 0, len(lets))
	for _, l := range lets {
		e, err := ptDTParse(l.value)
		if err != nil {
			return nil, fmt.Errorf("let $%s: %s: %w", l.name, l.value, err)
		}
		if err := ptDTCheck(e); err != nil {
			return nil, fmt.Errorf("let $%s: %s: %w", l.name, l.value, err)
		}
		out = append(out, ptDTCompiledLet{name: l.name, expr: e})
	}
	return out, nil
}

// ptDTCheck walks a parsed expression and refuses anything the evaluator would get
// wrong, and pre-compiles every regular expression so that a pattern this package
// cannot compile is a load failure rather than a rule that stops matching at
// runtime.
func ptDTCheck(e *ptDTExpr) error {
	switch e.op {
	case ptOpFunc:
		spec, ok := ptDTFunctions[e.fn]
		if !ok {
			return fmt.Errorf("unsupported function %s()", e.fn)
		}
		if len(e.args) < spec.min || len(e.args) > spec.max {
			return fmt.Errorf("%s() takes %d..%d arguments, given %d", e.fn, spec.min, spec.max, len(e.args))
		}
		if e.fn == "matches" {
			if e.args[1].op != ptOpLiteral {
				return fmt.Errorf("matches()'s pattern must be a string literal so it can be compiled once")
			}
			flags := ""
			if len(e.args) == 3 {
				if e.args[2].op != ptOpLiteral {
					return fmt.Errorf("matches()'s flags must be a string literal")
				}
				flags = e.args[2].str
			}
			m, err := ptDTCompilePattern(e.args[1].str, flags)
			if err != nil {
				return err
			}
			e.matcher = m
		}
	case ptOpCastable:
		if e.name != "xs:date" {
			return fmt.Errorf("`castable as %s`: the only type this evaluator implements is xs:date", e.name)
		}
	case ptOpPath:
		for i, s := range e.path.steps {
			if s.ancestor && i != 0 {
				return fmt.Errorf("the ancestor axis is only read at the head of a path")
			}
			if s.call != nil {
				if err := ptDTCheck(s.call); err != nil {
					return err
				}
			}
			for _, pr := range s.preds {
				// The axes and the `../` step are read against the context node,
				// and inside a predicate the context node has moved. Refusing them
				// here is what keeps "this evaluator understands the whole table" a
				// checked fact.
				if err := ptDTCheckPredicate(pr); err != nil {
					return err
				}
			}
		}
	}
	for _, a := range e.args {
		if err := ptDTCheck(a); err != nil {
			return err
		}
	}
	return nil
}

func ptDTCheckPredicate(e *ptDTExpr) error {
	if err := ptDTCheck(e); err != nil {
		return err
	}
	if ptDTUsesAncestors(e) {
		return errors.New("a predicate may not use the ancestor axis or a `../` step, " +
			"which would be read against the wrong node")
	}
	return nil
}

// ptDTCollectRootNames records the first-step element names of every `//` path in
// an expression, which is exactly what ptDTDoc has to index.
func ptDTCollectRootNames(e *ptDTExpr, into map[string]bool) {
	if e == nil {
		return
	}
	if e.op == ptOpPath {
		if e.path.fromRoot && e.path.up >= 0 && len(e.path.steps) > 0 && e.path.steps[0].name != "" {
			into[e.path.steps[0].name] = true
		}
		for _, s := range e.path.steps {
			ptDTCollectRootNames(s.call, into)
			for _, pr := range s.preds {
				ptDTCollectRootNames(pr, into)
			}
		}
	}
	for _, a := range e.args {
		ptDTCollectRootNames(a, into)
	}
}

// ptDTUsesAncestors reports whether an expression climbs out of its context node,
// which is what decides whether the walk has to keep the element's ancestor chain.
func ptDTUsesAncestors(e *ptDTExpr) bool {
	if e == nil {
		return false
	}
	if e.op == ptOpPath {
		if e.path.up > 0 {
			return true
		}
		for _, s := range e.path.steps {
			if s.ancestor {
				return true
			}
			if ptDTUsesAncestors(s.call) {
				return true
			}
			for _, p := range s.preds {
				if ptDTUsesAncestors(p) {
					return true
				}
			}
		}
	}
	for _, a := range e.args {
		if ptDTUsesAncestors(a) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Regular expressions
// ---------------------------------------------------------------------------

// ptDTMatcher is a compiled XSD regular expression.
type ptDTMatcher interface {
	match(s string) bool
}

type ptDTRegexpMatcher struct{ re *regexp.Regexp }

func (m ptDTRegexpMatcher) match(s string) bool { return m.re.MatchString(s) }

// ptDTLengthMatcher is `^(.{1,N})$`, which is 149 of the 192 matches() calls in
// this rule set: "between one and N characters".
//
// It exists because Go's regexp refuses a repetition bound above 1,000 and
// DT-CIUS-PT-111.1 writes `^(.{1,6826666})$` — the 5 MB ceiling AT places on an
// embedded attachment, expressed as a character count. Compiling the shape rather
// than the bytes is the one place in this rule set where the evaluator does not
// hand the expression to a general engine, so it is held to the general engine
// wherever the general engine can be asked:
// TestCIUSPTDatatypeLengthMatchersAgreeWithRegexp compiles every such pattern whose
// bound Go accepts both ways and checks the two agree on a battery of inputs.
//
// dotAll is matches()'s 's' flag. Without it `.` does not match a newline — Go's
// reading, and XSD's, differ only over a carriage return, which Go's `.` matches
// and XSD's does not. That difference can only make an <assert> pass where a
// reference processor fails it, which is a false negative and never a false
// positive.
type ptDTLengthMatcher struct {
	min, max int
	dotAll   bool
}

func (m ptDTLengthMatcher) match(s string) bool {
	if !m.dotAll && strings.IndexByte(s, '\n') >= 0 {
		return false
	}
	n := utf8.RuneCountInString(s)
	return n >= m.min && n <= m.max
}

// ptDTLengthRE recognises the anchored length family.
var ptDTLengthRE = regexp.MustCompile(`^\^\(\.\{(\d+),(\d+)\}\)\$$`)

// ptDTPatternCache holds one compiled matcher per (pattern, flags) pair. The table
// is generated and fixed, so the cache is filled during ptDTCompile at init and
// only read afterwards.
var ptDTPatternCache = map[string]ptDTMatcher{}

func ptDTCompilePattern(pattern, flags string) (ptDTMatcher, error) {
	key := flags + "\x00" + pattern
	if m, ok := ptDTPatternCache[key]; ok {
		return m, nil
	}
	dotAll := strings.Contains(flags, "s")
	for _, f := range flags {
		if f != 's' {
			return nil, fmt.Errorf("unsupported matches() flag %q in %q", string(f), flags)
		}
	}
	if m := ptDTLengthRE.FindStringSubmatch(pattern); m != nil {
		lo, hi := ptDTAtoi(m[1]), ptDTAtoi(m[2])
		matcher := ptDTLengthMatcher{min: lo, max: hi, dotAll: dotAll}
		ptDTPatternCache[key] = matcher
		return matcher, nil
	}
	src := pattern
	if dotAll {
		src = "(?s)" + src
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, fmt.Errorf("cannot compile the regular expression %q: %w", pattern, err)
	}
	matcher := ptDTRegexpMatcher{re: re}
	ptDTPatternCache[key] = matcher
	return matcher, nil
}

func ptDTAtoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// ---------------------------------------------------------------------------
// Values
// ---------------------------------------------------------------------------

type ptDTKind uint8

const (
	ptKNode ptDTKind = iota
	ptKAttr
	ptKNum
	ptKStr
	ptKBool
)

// ptDTItem is one item of an XPath sequence.
type ptDTItem struct {
	kind ptDTKind
	node *ciiNode
	str  string // the attribute's local name for ptKAttr, the value for ptKStr
	num  float64
	b    bool
}

type ptDTSeq []ptDTItem

// ptDTTrue and ptDTFalse are the two boolean sequences, shared rather than
// allocated. Almost every operator in this subset returns one — `and`, `or`,
// `not`, every comparison, matches(), starts-with(), contains() — and a fresh
// one-element slice per operator was, measured, most of what this pass allocated.
// Nothing writes through a returned sequence: the only operator that combines two
// is the union, and it copies into a new slice.
var (
	ptDTTrue  = ptDTSeq{{kind: ptKBool, b: true}}
	ptDTFalse = ptDTSeq{{kind: ptKBool, b: false}}
)

func ptDTBoolSeq(b bool) ptDTSeq {
	if b {
		return ptDTTrue
	}
	return ptDTFalse
}

// ptDTErr is a dynamic error: a cast that cannot be performed, or an operation
// applied to a sequence of the wrong length. A Schematron processor aborts on one;
// this evaluator returns it and ptDTRun declines to report the assertion.
type ptDTErr struct{ msg string }

func (e *ptDTErr) Error() string { return e.msg }

func ptDTErrf(format string, a ...any) error { return &ptDTErr{msg: fmt.Sprintf(format, a...)} }

// value atomises one item to its string value.
func (it ptDTItem) value() string {
	switch it.kind {
	case ptKNode:
		return it.node.text
	case ptKAttr:
		return it.node.attr(it.str)
	case ptKStr:
		return it.str
	case ptKNum:
		return ptDTFormatNumber(it.num)
	case ptKBool:
		if it.b {
			return "true"
		}
		return "false"
	}
	return ""
}

// ptDTFormatNumber renders a number the way XPath's xs:string cast does for the
// values these rules produce: an integral value without a fractional part.
func ptDTFormatNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return fmt.Sprintf("%d", int64(f))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", f), "0"), ".")
}

// ptDTEffectiveBoolean is XPath's effective boolean value.
//
// It is the one piece of XPath 2.0 semantics these rules lean on hardest.
// DT-CIUS-PT-166 puts `xs:decimal(cbc:PrepaidAmount)` in boolean position, so an
// invoice that writes a *zero* prepaid amount takes a different branch from one
// that omits the element — and DT-CIUS-PT-163 does the same with
// `(cbc:ChargeTotalAmount)`, where the operand is a node and the answer is
// existence rather than value.
func ptDTEffectiveBoolean(s ptDTSeq) (bool, error) {
	if len(s) == 0 {
		return false, nil
	}
	switch s[0].kind {
	case ptKNode, ptKAttr:
		return true, nil
	}
	if len(s) > 1 {
		return false, ptDTErrf("the effective boolean value of a sequence of %d atomic values is a type error", len(s))
	}
	switch s[0].kind {
	case ptKBool:
		return s[0].b, nil
	case ptKStr:
		return s[0].str != "", nil
	case ptKNum:
		return s[0].num != 0 && !math.IsNaN(s[0].num), nil
	}
	return false, nil
}

// ptDTNumber casts one atomised value to a number the way a general comparison
// against a numeric operand does.
func ptDTNumber(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	return parseAmount(v)
}

// ptDTDecimal is xs:decimal's lexical space: an optional sign and digits with an
// optional fractional part, and no exponent.
func ptDTDecimal(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	body := v
	if body[0] == '+' || body[0] == '-' {
		body = body[1:]
	}
	if body == "" {
		return 0, false
	}
	digits, dot := 0, 0
	for i := 0; i < len(body); i++ {
		switch {
		case body[i] >= '0' && body[i] <= '9':
			digits++
		case body[i] == '.':
			dot++
		default:
			return 0, false
		}
	}
	if digits == 0 || dot > 1 {
		return 0, false
	}
	return parseAmount(v)
}

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

// ptDTCtx is the evaluation context for one assertion.
type ptDTCtx struct {
	item ptDTItem
	// ancestors is the element's ancestor chain from the document element down to
	// and including the element itself. It is nil for a rule that needs neither the
	// ancestor axis nor a `../` step.
	ancestors []*ciiNode
	doc       *ptDTDoc
	// vars is the `every $v in …` bindings, innermost last. It is a stack rather
	// than a map because there is never more than one binding live at a time and a
	// map allocated once per quantified expression showed up in the profile.
	vars []ptDTVar
	// lets are the <let> declarations in scope, and letValues memoises them: the
	// condition pattern's rules declare up to fifteen and several assertions read
	// the same one.
	lets      []ptDTCompiledLet
	letValues map[string]ptDTSeq
	letBusy   map[string]bool
}

// ptDTDoc is the per-document state the `//` paths read: the document element, and
// a lazily built index of the element names those paths start from.
//
// It indexes those names and no others. Indexing every element name would cost a
// slice per distinct name on every invoice validated, to answer the handful of
// rooted paths AT actually writes — the tax currency code, the invoice and
// credit-note lines, the document-level allowances and the VAT breakdowns.
type ptDTDoc struct {
	root   *ciiNode
	want   map[string]bool
	byName map[string][]*ciiNode
}

func (d *ptDTDoc) index() map[string][]*ciiNode {
	if d.byName == nil {
		d.byName = make(map[string][]*ciiNode, len(d.want))
		var rec func(n *ciiNode)
		rec = func(n *ciiNode) {
			if d.want[n.name] {
				d.byName[n.name] = append(d.byName[n.name], n)
			}
			for _, c := range n.children {
				rec(c)
			}
		}
		rec(d.root)
	}
	return d.byName
}

// ptDTVar is one `every $v in …` binding.
type ptDTVar struct {
	name string
	seq  ptDTSeq
}

func (c *ptDTCtx) lookup(name string) (ptDTSeq, error) {
	for i := len(c.vars) - 1; i >= 0; i-- {
		if c.vars[i].name == name {
			return c.vars[i].seq, nil
		}
	}
	if v, ok := c.letValues[name]; ok {
		return v, nil
	}
	for _, l := range c.lets {
		if l.name != name {
			continue
		}
		// The two maps are allocated on first use rather than per matched element:
		// most rules declare no <let> at all and most assertions read none.
		if c.letBusy == nil {
			c.letBusy = map[string]bool{}
			c.letValues = map[string]ptDTSeq{}
		}
		if c.letBusy[name] {
			return nil, ptDTErrf("the variable $%s is defined in terms of itself", name)
		}
		c.letBusy[name] = true
		v, err := ptDTEval(l.expr, c)
		delete(c.letBusy, name)
		if err != nil {
			return nil, err
		}
		c.letValues[name] = v
		return v, nil
	}
	return nil, ptDTErrf("undeclared variable $%s", name)
}

func ptDTEval(e *ptDTExpr, c *ptDTCtx) (ptDTSeq, error) {
	switch e.op {
	case ptOpOr:
		l, err := ptDTEvalBool(e.args[0], c)
		if err != nil {
			return nil, err
		}
		if l {
			return ptDTTrue, nil
		}
		r, err := ptDTEvalBool(e.args[1], c)
		if err != nil {
			return nil, err
		}
		return ptDTBoolSeq(r), nil
	case ptOpAnd:
		l, err := ptDTEvalBool(e.args[0], c)
		if err != nil {
			return nil, err
		}
		if !l {
			return ptDTFalse, nil
		}
		r, err := ptDTEvalBool(e.args[1], c)
		if err != nil {
			return nil, err
		}
		return ptDTBoolSeq(r), nil
	case ptOpCompare:
		return ptDTCompare(e, c)
	case ptOpArith:
		return ptDTArith(e, c)
	case ptOpNeg:
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		n, ok, err := ptDTSingleNumber(s)
		if err != nil || !ok {
			return nil, err
		}
		return ptDTSeq{{kind: ptKNum, num: -n}}, nil
	case ptOpUnion:
		l, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		r, err := ptDTEval(e.args[1], c)
		if err != nil {
			return nil, err
		}
		return append(append(ptDTSeq{}, l...), r...), nil
	case ptOpLiteral:
		return ptDTSeq{{kind: ptKStr, str: e.str}}, nil
	case ptOpNumber:
		return ptDTSeq{{kind: ptKNum, num: e.num}}, nil
	case ptOpVar:
		return c.lookup(e.name)
	case ptOpFunc:
		return ptDTCall(e, c)
	case ptOpEvery:
		return ptDTEvery(e, c)
	case ptOpCastable:
		return ptDTCastable(e, c)
	case ptOpPath:
		return ptDTSelect(e.path, c)
	}
	return nil, ptDTErrf("unhandled operator %d", e.op)
}

func ptDTEvalBool(e *ptDTExpr, c *ptDTCtx) (bool, error) {
	s, err := ptDTEval(e, c)
	if err != nil {
		return false, err
	}
	return ptDTEffectiveBoolean(s)
}

// ptDTEvery is `every $v in E satisfies E`, which is true for an empty sequence.
// AT uses it for the withholding-tax and ATM-payment cross-references in
// DT-CIUS-PT-020, and for the arithmetic tier's per-rate summations.
func ptDTEvery(e *ptDTExpr, c *ptDTCtx) (ptDTSeq, error) {
	seq, err := ptDTEval(e.args[0], c)
	if err != nil {
		return nil, err
	}
	depth := len(c.vars)
	c.vars = append(c.vars, ptDTVar{name: e.name})
	defer func() { c.vars = c.vars[:depth] }()
	for _, it := range seq {
		c.vars[depth].seq = ptDTSeq{it}
		ok, err := ptDTEvalBool(e.args[1], c)
		if err != nil {
			return nil, err
		}
		if !ok {
			return ptDTFalse, nil
		}
	}
	return ptDTTrue, nil
}

// ptDTCastable is `E castable as T`, restricted to xs:date — the only type any
// assertion in this package tests against. ptDTCheck refuses any other, so a rule
// that starts using one is a load failure rather than a rule that quietly answers
// the wrong question.
//
// XPath 2.0's SingleType without a `?` requires exactly one item, so an empty
// sequence is *not* castable — which is the reading that makes ANAF's
// `string(.) castable as xs:date` false for an empty element rather than an error.
// A sequence of more than one item is a type error, as everywhere else here.
func ptDTCastable(e *ptDTExpr, c *ptDTCtx) (ptDTSeq, error) {
	s, err := ptDTEval(e.args[0], c)
	if err != nil {
		return nil, err
	}
	switch len(s) {
	case 0:
		return ptDTFalse, nil
	case 1:
	default:
		return nil, ptDTErrf("`castable as %s` applied to a sequence of %d items", e.name, len(s))
	}
	return ptDTBoolSeq(ptDTIsDate(s[0].value())), nil
}

// ptDTIsDate reports whether s is in xs:date's lexical space:
// `-? YYYY(+) '-' MM '-' DD` with an optional timezone, and a day that exists in
// that month of that year.
//
// The calendar check is not pedantry. BR-RO-DT001's whole content is that a date
// element holds a date, and a validator that accepted 2024-02-31 would report
// nothing for the one value the rule is most likely to be written against.
func ptDTIsDate(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
	}
	// The timezone, if any: 'Z' or (+|-)HH:MM.
	if strings.HasSuffix(s, "Z") {
		s = s[:len(s)-1]
	} else if len(s) > 6 {
		if tz := s[len(s)-6:]; (tz[0] == '+' || tz[0] == '-') && tz[3] == ':' {
			if ptDTAllDigits(tz[1:3]) && ptDTAllDigits(tz[4:6]) {
				hh, mm := ptDTAtoi(tz[1:3]), ptDTAtoi(tz[4:6])
				if hh > 14 || mm > 59 || (hh == 14 && mm != 0) {
					return false
				}
				s = s[:len(s)-6]
			}
		}
	}
	// YYYY-MM-DD, with a year of four digits or more and no leading zero beyond
	// four characters.
	i := strings.IndexByte(s, '-')
	if i < 4 || len(s) != i+6 || s[i+3] != '-' {
		return false
	}
	year, month, day := s[:i], s[i+1:i+3], s[i+4:]
	if !ptDTAllDigits(year) || !ptDTAllDigits(month) || !ptDTAllDigits(day) {
		return false
	}
	if i > 4 && year[0] == '0' {
		return false
	}
	y, m, d := ptDTAtoi(year), ptDTAtoi(month), ptDTAtoi(day)
	if y == 0 || m < 1 || m > 12 || d < 1 {
		return false
	}
	return d <= ptDTDaysInMonth(y, m)
}

// ptDTBoolValue is xs:boolean's lexical space, which is four values and not two.
func ptDTBoolValue(s string) (bool, bool) {
	switch strings.TrimSpace(s) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

func ptDTAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !ptDTIsDigit(s[i]) {
			return false
		}
	}
	return true
}

func ptDTDaysInMonth(y, m int) int {
	switch m {
	case 4, 6, 9, 11:
		return 30
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	}
	return 31
}

// ptDTCompare is XPath's general comparison: true when *some* pair of items, one
// from each side, compares true. Both `a = 'x'` and `a != 'x'` are therefore false
// when a selects nothing, and reading either as the negation of the other would turn
// a silent rule into a false positive on every conforming invoice.
//
// A pair in which either side is a number is compared numerically, with the other
// side cast; a pair of untyped values is compared as strings. That is XPath 2.0's
// rule and it is what makes `normalize-space(@currencyID) = $bt05` a string
// comparison and `cac:Price/cbc:BaseQuantity > 0` a numeric one.
func ptDTCompare(e *ptDTExpr, c *ptDTCtx) (ptDTSeq, error) {
	if ptDTFastPaths && ptDTIsNumeric(e.args[0]) && ptDTIsNumeric(e.args[1]) {
		x, okx, err := ptDTEvalNum(e.args[0], c)
		if err != nil {
			return nil, err
		}
		y, oky, err := ptDTEvalNum(e.args[1], c)
		if err != nil {
			return nil, err
		}
		if !okx || !oky {
			// A general comparison quantifies over both operand sequences, so an
			// empty one makes it false whichever operator it is.
			return ptDTFalse, nil
		}
		ok, err := ptDTComparePair(ptDTItem{kind: ptKNum, num: x}, ptDTItem{kind: ptKNum, num: y}, e.relop)
		if err != nil {
			return nil, err
		}
		return ptDTBoolSeq(ok), nil
	}
	left, err := ptDTEval(e.args[0], c)
	if err != nil {
		return nil, err
	}
	right, err := ptDTEval(e.args[1], c)
	if err != nil {
		return nil, err
	}
	for _, a := range left {
		for _, b := range right {
			ok, err := ptDTComparePair(a, b, e.relop)
			if err != nil {
				return nil, err
			}
			if ok {
				return ptDTTrue, nil
			}
		}
	}
	return ptDTFalse, nil
}

func ptDTComparePair(a, b ptDTItem, op string) (bool, error) {
	if a.kind == ptKBool && b.kind == ptKBool {
		switch op {
		case "=":
			return a.b == b.b, nil
		case "!=":
			return a.b != b.b, nil
		}
		return false, ptDTErrf("%s does not apply to two booleans", op)
	}
	// One boolean and one untyped value: XPath 2.0 casts the untyped one to
	// xs:boolean, so `cbc:ChargeIndicator = false()` — ANAF's spelling of the
	// document-level allowance context — matches an element written `0` as well as
	// one written `false`. Comparing the two as strings would silently exclude the
	// first, and with it the six BR-DEC-RO-* assertions bound to that context.
	if (a.kind == ptKBool) != (b.kind == ptKBool) {
		untyped, boolean := a, b.b
		if b.kind != ptKBool {
			untyped, boolean = b, a.b
		}
		v, ok := ptDTBoolValue(untyped.value())
		if !ok {
			return false, ptDTErrf("the value %q cannot be cast to xs:boolean", untyped.value())
		}
		switch op {
		case "=":
			return v == boolean, nil
		case "!=":
			return v != boolean, nil
		}
		return false, ptDTErrf("%s does not apply to a boolean", op)
	}
	if a.kind == ptKNum || b.kind == ptKNum {
		x, okx := ptDTItemNumber(a)
		y, oky := ptDTItemNumber(b)
		if !okx || !oky {
			return false, ptDTErrf("a numeric comparison against the value %q, which is not a number",
				map[bool]string{true: b.value(), false: a.value()}[okx])
		}
		switch op {
		case "=":
			return x == y, nil
		case "!=":
			return x != y, nil
		case "<":
			return x < y, nil
		case "<=":
			return x <= y, nil
		case ">":
			return x > y, nil
		case ">=":
			return x >= y, nil
		}
		return false, ptDTErrf("unknown operator %q", op)
	}
	x, y := a.value(), b.value()
	switch op {
	case "=":
		return x == y, nil
	case "!=":
		return x != y, nil
	case "<":
		return x < y, nil
	case "<=":
		return x <= y, nil
	case ">":
		return x > y, nil
	case ">=":
		return x >= y, nil
	}
	return false, ptDTErrf("unknown operator %q", op)
}

func ptDTItemNumber(it ptDTItem) (float64, bool) {
	if it.kind == ptKNum {
		return it.num, true
	}
	return ptDTNumber(it.value())
}

// ptDTArith is +, -, * and div. An empty operand makes the result empty, which is
// XPath 2.0's rule and the reason the arithmetic tier's guards are written the way
// they are: `xs:decimal(cbc:BaseAmount) * (…)` on an allowance with no base amount
// is not zero, it is nothing, and every comparison against it is false.
func ptDTArith(e *ptDTExpr, c *ptDTCtx) (ptDTSeq, error) {
	v, ok, err := ptDTArithNum(e, c)
	if err != nil || !ok {
		return nil, err
	}
	return ptDTSeq{{kind: ptKNum, num: v}}, nil
}

// ptDTArithNum applies one arithmetic operator to its two operands. It asks for the
// *operands* rather than for the node it was given, which is what keeps the descent
// finite when ptDTFastPaths is off and ptDTEvalNum routes an arithmetic node back
// through ptDTEval.
func ptDTArithNum(e *ptDTExpr, c *ptDTCtx) (float64, bool, error) {
	x, ok, err := ptDTEvalNum(e.args[0], c)
	if err != nil || !ok {
		return 0, false, err
	}
	y, ok, err := ptDTEvalNum(e.args[1], c)
	if err != nil || !ok {
		return 0, false, err
	}
	switch e.relop {
	case "+":
		return x + y, true, nil
	case "-":
		return x - y, true, nil
	case "*":
		return x * y, true, nil
	case "div":
		if y == 0 {
			return 0, false, ptDTErrf("division by zero")
		}
		return x / y, true, nil
	}
	return 0, false, ptDTErrf("unknown arithmetic operator %q", e.relop)
}

// ptDTEvalNum evaluates an expression in numeric position without materialising a
// sequence for it or for its operands.
//
// It is an optimisation and not a second semantics. The second return is XPath's
// empty sequence, which is what makes `xs:decimal(cbc:PrepaidAmount) - 1.00` on an
// invoice with no prepaid amount *nothing* rather than minus one, and every
// comparison against it false. AT's arithmetic tier is built on that: eleven of its
// rules guard themselves with `not(exists(...))` on one branch and rely on the
// empty propagation on the other. TestCIUSPTDatatypeNumericFastPathAgrees runs both
// paths over the whole corpus and requires the same answer.
func ptDTEvalNum(e *ptDTExpr, c *ptDTCtx) (float64, bool, error) {
	if !ptDTFastPaths {
		// ptDTEval materialises a sequence at every level; for an arithmetic node it
		// reaches ptDTArith, which asks for its *operands* through this function
		// again, so the descent still terminates and every intermediate value goes
		// through the general machinery.
		s, err := ptDTEval(e, c)
		if err != nil {
			return 0, false, err
		}
		return ptDTSingleNumber(s)
	}
	switch e.op {
	case ptOpNumber:
		return e.num, true, nil
	case ptOpNeg:
		v, ok, err := ptDTEvalNum(e.args[0], c)
		return -v, ok, err
	case ptOpArith:
		return ptDTArithNum(e, c)
	case ptOpFunc:
		switch e.fn {
		case "round":
			v, ok, err := ptDTEvalNum(e.args[0], c)
			if err != nil || !ok {
				return 0, false, err
			}
			return math.Floor(v + 0.5), true, nil
		case "xs:decimal":
			// A single number stays one; anything else goes through the general
			// path, which is where the cast and its dynamic error live.
			if v, ok, err := ptDTEvalNum(e.args[0], c); err == nil && ok && ptDTIsNumeric(e.args[0]) {
				return v, true, nil
			}
		}
	}
	s, err := ptDTEval(e, c)
	if err != nil {
		return 0, false, err
	}
	return ptDTSingleNumber(s)
}

// ptDTFastPaths switches ptDTEvalString, ptDTEvalNum and the numeric comparison on.
//
// It exists so that the claim those three make — "an optimisation, not a second
// semantics" — is a checked fact rather than a comment.
// TestCIUSPTDatatypeFastPathsAgreeWithTheGeneralEvaluator turns it off and runs the
// whole corpus through both, requiring the same findings in the same order and the
// same number of dynamic errors. It is never false outside that test, and the
// alternative — asserting the equivalence by reading — is how an optimisation comes
// to change a rule.
var ptDTFastPaths = true

// ptDTIsNumeric reports whether an expression always evaluates to a number or the
// empty sequence, which is what lets a comparison between two of them skip the
// sequence machinery.
func ptDTIsNumeric(e *ptDTExpr) bool {
	switch e.op {
	case ptOpNumber, ptOpArith, ptOpNeg:
		return true
	case ptOpFunc:
		switch e.fn {
		case "round", "count", "sum", "string-length", "xs:decimal":
			return true
		}
	}
	return false
}

// ptDTSingleNumber atomises a sequence to one number. An empty sequence is "no
// value" rather than an error, which is what the second return says.
func ptDTSingleNumber(s ptDTSeq) (float64, bool, error) {
	if len(s) == 0 {
		return 0, false, nil
	}
	if len(s) > 1 {
		return 0, false, ptDTErrf("arithmetic on a sequence of %d items", len(s))
	}
	n, ok := ptDTItemNumber(s[0])
	if !ok {
		return 0, false, ptDTErrf("the value %q is not a number", s[0].value())
	}
	return n, true, nil
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func ptDTSelect(p *ptDTXPath, c *ptDTCtx) (ptDTSeq, error) {
	var cur ptDTSeq
	switch {
	case p.fromRoot && p.up < 0:
		// A leading single '/': the document element itself.
		cur = ptDTSeq{{kind: ptKNode, node: c.doc.root}}
	case p.fromRoot:
		// `//x` is `/descendant-or-self::node()/x`, an absolute step, so it is
		// document-wide whatever the context node is. The first step is resolved
		// through the document index and the rest walk down from there.
		if len(p.steps) == 0 {
			return nil, ptDTErrf("`//` with no step")
		}
		first := p.steps[0]
		if first.name == "" {
			return nil, ptDTErrf("`//` must be followed by an element name")
		}
		for _, n := range c.doc.index()[first.name] {
			it := ptDTItem{kind: ptKNode, node: n}
			ok, err := ptDTPredsOK(first.preds, it, c)
			if err != nil {
				return nil, err
			}
			if ok {
				cur = append(cur, it)
			}
		}
		return ptDTSteps(cur, p.steps[1:], c)
	case p.up > 0:
		if len(c.ancestors) <= p.up {
			return nil, nil
		}
		cur = ptDTSeq{{kind: ptKNode, node: c.ancestors[len(c.ancestors)-1-p.up]}}
	default:
		// The first step of a relative path is taken from the single context item,
		// which is the overwhelmingly common case: wrapping it in a one-element
		// sequence to reach the general routine below allocates once per path
		// evaluated, and this pass evaluates a few hundred per invoice.
		out, err := ptDTStepFrom(c.item, p.steps[0], c)
		if err != nil {
			return nil, err
		}
		return ptDTSteps(out, p.steps[1:], c)
	}
	return ptDTSteps(cur, p.steps, c)
}

func ptDTSteps(cur ptDTSeq, steps []ptDTStep, c *ptDTCtx) (ptDTSeq, error) {
	for _, s := range steps {
		if len(cur) == 0 {
			return nil, nil
		}
		var next ptDTSeq
		for _, it := range cur {
			out, err := ptDTStepFrom(it, s, c)
			if err != nil {
				return nil, err
			}
			next = append(next, out...)
		}
		cur = next
	}
	return cur, nil
}

func ptDTStepFrom(it ptDTItem, s ptDTStep, c *ptDTCtx) (ptDTSeq, error) {
	var out ptDTSeq
	switch {
	case s.self:
		out = ptDTSeq{it}
	case s.ancestor:
		for _, a := range c.ancestors[:max(0, len(c.ancestors)-1)] {
			if a.name == s.name {
				out = append(out, ptDTItem{kind: ptKNode, node: a})
			}
		}
	case s.attr != "":
		if it.kind == ptKNode && it.node.hasAttr(s.attr) {
			out = ptDTSeq{{kind: ptKAttr, node: it.node, str: s.attr}}
		}
	case s.call != nil:
		// A function call or a parenthesised expression as a step: XPath 2.0's
		// E1/E2, with E2 evaluated once per item of E1. This is how AT writes every
		// summation — cac:AllowanceCharge[…]/xs:decimal(cbc:Amount).
		//
		// The context item is moved and put back rather than copied into a new
		// context: nothing below retains the context, and copying it allocated once
		// per item of every path step.
		saved := c.item
		c.item = it
		v, err := ptDTEval(s.call, c)
		c.item = saved
		if err != nil {
			return nil, err
		}
		out = v
	case s.name != "":
		if it.kind != ptKNode {
			return nil, nil
		}
		for _, ch := range it.node.children {
			if ch.name == s.name {
				out = append(out, ptDTItem{kind: ptKNode, node: ch})
			}
		}
	default:
		return nil, ptDTErrf("a step with no name, axis or expression")
	}
	if len(s.preds) == 0 {
		return out, nil
	}
	var kept ptDTSeq
	for _, o := range out {
		ok, err := ptDTPredsOK(s.preds, o, c)
		if err != nil {
			return nil, err
		}
		if ok {
			kept = append(kept, o)
		}
	}
	return kept, nil
}

// ptDTPredsOK evaluates a step's predicates with the candidate as the context item.
// The axes and the `../` step are refused inside a predicate at compile time, so
// nothing here reads state that has not moved with the item.
func ptDTPredsOK(preds []*ptDTExpr, it ptDTItem, c *ptDTCtx) (bool, error) {
	saved := c.item
	defer func() { c.item = saved }()
	for _, p := range preds {
		c.item = it
		ok, err := ptDTEvalBool(p, c)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

type ptDTFuncSpec struct{ min, max int }

// ptDTFunctions is the whole function library these 291 assertions call, and
// nothing more. A function outside it stops gen.py before the rule is written and
// stops this package at load if one is written by hand.
var ptDTFunctions = map[string]ptDTFuncSpec{
	"normalize-space":  {1, 1},
	"matches":          {2, 3},
	"not":              {1, 1},
	"starts-with":      {2, 2},
	"contains":         {2, 2},
	"concat":           {2, 8},
	"substring-before": {2, 2},
	"substring-after":  {2, 2},
	"substring":        {2, 3},
	"string-length":    {1, 1},
	"exists":           {1, 1},
	"count":            {1, 1},
	"sum":              {1, 1},
	"round":            {1, 1},
	"distinct-values":  {1, 1},
	"index-of":         {2, 2},
	"xs:decimal":       {1, 1},
	// The four below are CIUS-RO's. text() is a node test rather than a function
	// in XPath, and is read here as "the character data of the context element",
	// which is the same string for every element ANAF binds it to (they all have
	// simple content) and differs only for an element with element children.
	"string": {0, 1},
	"text":   {0, 0},
	"true":   {0, 0},
	"false":  {0, 0},
}

func ptDTCall(e *ptDTExpr, c *ptDTCtx) (ptDTSeq, error) {
	str := func(i int) (string, error) { return ptDTEvalString(e.args[i], c) }
	switch e.fn {
	case "not":
		b, err := ptDTEvalBool(e.args[0], c)
		if err != nil {
			return nil, err
		}
		return ptDTBoolSeq(!b), nil
	case "exists":
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		return ptDTBoolSeq(len(s) > 0), nil
	case "count":
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		return ptDTSeq{{kind: ptKNum, num: float64(len(s))}}, nil
	case "sum":
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		total := 0.0
		for _, it := range s {
			n, ok := ptDTItemNumber(it)
			if !ok {
				return nil, ptDTErrf("sum() over the value %q, which is not a number", it.value())
			}
			total += n
		}
		return ptDTSeq{{kind: ptKNum, num: total}}, nil
	case "round":
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		n, ok, err := ptDTSingleNumber(s)
		if err != nil || !ok {
			return nil, err
		}
		return ptDTSeq{{kind: ptKNum, num: math.Floor(n + 0.5)}}, nil
	case "xs:decimal":
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		if len(s) == 0 {
			return nil, nil
		}
		if len(s) > 1 {
			return nil, ptDTErrf("xs:decimal() applied to a sequence of %d items", len(s))
		}
		if s[0].kind == ptKNum {
			return ptDTSeq{{kind: ptKNum, num: s[0].num}}, nil
		}
		v, ok := ptDTDecimal(s[0].value())
		if !ok {
			return nil, ptDTErrf("the value %q cannot be cast to xs:decimal", s[0].value())
		}
		return ptDTSeq{{kind: ptKNum, num: v}}, nil
	case "distinct-values":
		s, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var out ptDTSeq
		for _, it := range s {
			v := it.value()
			if seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, ptDTItem{kind: ptKStr, str: v})
		}
		return out, nil
	case "index-of":
		seq, err := ptDTEval(e.args[0], c)
		if err != nil {
			return nil, err
		}
		want, err := str(1)
		if err != nil {
			return nil, err
		}
		var out ptDTSeq
		for i, it := range seq {
			if it.value() == want {
				out = append(out, ptDTItem{kind: ptKNum, num: float64(i + 1)})
			}
		}
		return out, nil
	case "normalize-space":
		v, err := str(0)
		if err != nil {
			return nil, err
		}
		return ptDTSeq{{kind: ptKStr, str: normalizeSpace(v)}}, nil
	case "string-length":
		v, err := str(0)
		if err != nil {
			return nil, err
		}
		return ptDTSeq{{kind: ptKNum, num: float64(utf8.RuneCountInString(v))}}, nil
	case "concat":
		var b strings.Builder
		for i := range e.args {
			v, err := str(i)
			if err != nil {
				return nil, err
			}
			b.WriteString(v)
		}
		return ptDTSeq{{kind: ptKStr, str: b.String()}}, nil
	case "starts-with", "contains", "substring-before", "substring-after":
		a, err := str(0)
		if err != nil {
			return nil, err
		}
		b, err := str(1)
		if err != nil {
			return nil, err
		}
		switch e.fn {
		case "starts-with":
			return ptDTBoolSeq(strings.HasPrefix(a, b)), nil
		case "contains":
			return ptDTBoolSeq(strings.Contains(a, b)), nil
		case "substring-before":
			i := strings.Index(a, b)
			if b == "" || i < 0 {
				return ptDTSeq{{kind: ptKStr, str: ""}}, nil
			}
			return ptDTSeq{{kind: ptKStr, str: a[:i]}}, nil
		default:
			i := strings.Index(a, b)
			if b == "" {
				return ptDTSeq{{kind: ptKStr, str: a}}, nil
			}
			if i < 0 {
				return ptDTSeq{{kind: ptKStr, str: ""}}, nil
			}
			return ptDTSeq{{kind: ptKStr, str: a[i+len(b):]}}, nil
		}
	case "substring":
		v, err := str(0)
		if err != nil {
			return nil, err
		}
		start, err := ptDTArgNumber(e, 1, c)
		if err != nil {
			return nil, err
		}
		runes := []rune(v)
		lo := int(math.Floor(start + 0.5))
		hi := len(runes) + 1
		if len(e.args) == 3 {
			length, lerr := ptDTArgNumber(e, 2, c)
			if lerr != nil {
				return nil, lerr
			}
			hi = lo + int(math.Floor(length+0.5))
		}
		// fn:substring selects the characters at positions p with
		// round(start) <= p < round(start)+round(length), one-based. AT's
		// substring(x, 0, 3) therefore takes the first *two* characters, which is
		// what DT-CIUS-PT-029 compares against the ISO 3166 country list.
		if lo < 1 {
			lo = 1
		}
		if hi > len(runes)+1 {
			hi = len(runes) + 1
		}
		if lo >= hi {
			return ptDTSeq{{kind: ptKStr, str: ""}}, nil
		}
		return ptDTSeq{{kind: ptKStr, str: string(runes[lo-1 : hi-1])}}, nil
	case "matches":
		v, err := str(0)
		if err != nil {
			return nil, err
		}
		return ptDTBoolSeq(e.matcher.match(v)), nil
	case "true":
		return ptDTTrue, nil
	case "false":
		return ptDTFalse, nil
	case "string":
		if len(e.args) == 0 {
			return ptDTSeq{{kind: ptKStr, str: c.item.value()}}, nil
		}
		v, err := str(0)
		if err != nil {
			return nil, err
		}
		return ptDTSeq{{kind: ptKStr, str: v}}, nil
	case "text":
		if c.item.kind != ptKNode {
			return nil, ptDTErrf("text() applied to something that is not an element")
		}
		return ptDTSeq{{kind: ptKStr, str: c.item.node.text}}, nil
	}
	return nil, ptDTErrf("unsupported function %s()", e.fn)
}

func ptDTArgNumber(e *ptDTExpr, i int, c *ptDTCtx) (float64, error) {
	s, err := ptDTEval(e.args[i], c)
	if err != nil {
		return 0, err
	}
	n, ok, err := ptDTSingleNumber(s)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ptDTErrf("argument %d of %s() is empty", i+1, e.fn)
	}
	return n, nil
}

// ptDTEvalString evaluates an expression in string position without materialising a
// sequence for the shapes that make up almost all of this rule set.
//
// It is an optimisation and not a second semantics: every branch below is what
// ptDTString(ptDTEval(...)) computes, and TestCIUSPTDatatypeStringFastPathAgrees
// runs both over every assertion in the table on a corpus of documents and requires
// the same answer. The shapes are matches(., …), normalize-space(.),
// starts-with(normalize-space(.), 'lit') and contains(' list ', concat(' ',
// normalize-space(.), ' ')) — which between them are 149 of the 291 assertions, one
// per typed element in a UBL invoice.
func ptDTEvalString(e *ptDTExpr, c *ptDTCtx) (string, error) {
	if !ptDTFastPaths {
		s, err := ptDTEval(e, c)
		if err != nil {
			return "", err
		}
		return ptDTString(s)
	}
	switch e.op {
	case ptOpLiteral:
		return e.str, nil
	case ptOpPath:
		p := e.path
		if !p.fromRoot && p.up == 0 && len(p.steps) == 1 && len(p.steps[0].preds) == 0 {
			st := p.steps[0]
			switch {
			case st.self:
				return c.item.value(), nil
			case st.attr != "":
				if c.item.kind == ptKNode && c.item.node.hasAttr(st.attr) {
					return c.item.node.attr(st.attr), nil
				}
				return "", nil
			case st.name != "" && c.item.kind == ptKNode:
				var found *ciiNode
				for _, ch := range c.item.node.children {
					if ch.name != st.name {
						continue
					}
					if found != nil {
						return "", ptDTErrf("a string function applied to a sequence of 2 or more items")
					}
					found = ch
				}
				if found == nil {
					return "", nil
				}
				return found.text, nil
			}
		}
	case ptOpFunc:
		switch e.fn {
		case "normalize-space":
			v, err := ptDTEvalString(e.args[0], c)
			if err != nil {
				return "", err
			}
			return normalizeSpace(v), nil
		case "concat":
			var b strings.Builder
			for i := range e.args {
				v, err := ptDTEvalString(e.args[i], c)
				if err != nil {
					return "", err
				}
				b.WriteString(v)
			}
			return b.String(), nil
		}
	}
	s, err := ptDTEval(e, c)
	if err != nil {
		return "", err
	}
	return ptDTString(s)
}

// ptDTString atomises a sequence to one string, the way fn:string does for the
// argument of a string function: an empty sequence is the empty string, and a
// sequence of more than one item is a type error.
func ptDTString(s ptDTSeq) (string, error) {
	switch len(s) {
	case 0:
		return "", nil
	case 1:
		return s[0].value(), nil
	}
	return "", ptDTErrf("a string function applied to a sequence of %d items", len(s))
}

// ---------------------------------------------------------------------------
// Running a pattern
// ---------------------------------------------------------------------------

// ptDTEntry is one element together with the rule that claimed it.
type ptDTEntry struct {
	rule      int
	node      *ciiNode
	ancestors []*ciiNode
}

// ptDTGather walks the document once and assigns every element to the first rule of
// the pattern whose context matches it — ISO Schematron's semantics, and the reason
// rule order is modelled here rather than assumed away.
func ptDTGather(p *ptDTCompiledPattern, root *ciiNode, doc *ptDTDoc) []ptDTEntry {
	var entries []ptDTEntry
	stack := make([]*ciiNode, 0, 16)

	claim := func(terms []ptDTTerm, n *ciiNode) {
		best := -1
		for _, t := range terms {
			if t.root != "" && t.root != root.name {
				continue
			}
			if best < 0 || t.rule < best {
				best = t.rule
			}
		}
		if best < 0 || len(p.rules[best].asserts) == 0 {
			return
		}
		e := ptDTEntry{rule: best, node: n}
		if p.rules[best].needsAncestors {
			e.ancestors = append([]*ciiNode(nil), stack...)
		}
		entries = append(entries, e)
	}

	// buf is one arena for every active trie-node set the walk builds, released on
	// the way back up. Allocating a fresh slice per element cost one allocation for
	// every element of every invoice, which is most of what this walk did.
	//
	// A set handed to walk is a window into buf. When a deeper level grows buf past
	// its capacity the window keeps pointing at the old array, whose contents were
	// copied and are not written again, so the parent's set stays valid.
	buf := make([]*ptDTTrieNode, 0, 64)

	// step advances the active trie set by one element. A child whose name leaves
	// the trie prunes its whole subtree, which is what most of a UBL invoice does.
	//
	// The pattern's floating trie is offered the element too, because a match
	// pattern may begin anywhere: an element is the start of a floating branch as
	// readily as it is the continuation of one. For a pattern with no floating
	// branch — every CIUS-PT one — that trie has no edges and the second loop is
	// empty.
	advance := func(a *ptDTTrieNode, ch *ciiNode) {
		for i := range a.edges {
			ed := &a.edges[i]
			if ed.name != ch.name {
				continue
			}
			if ed.pred != nil {
				c := &ptDTCtx{item: ptDTItem{kind: ptKNode, node: ch}, doc: doc}
				// A context predicate that raises a dynamic error selects
				// nothing, which is the reading that reports no finding.
				if ok, err := ptDTEvalBool(ed.pred, c); err != nil || !ok {
					continue
				}
			}
			buf = append(buf, ed.next)
		}
	}
	step := func(active []*ptDTTrieNode, ch *ciiNode) []*ptDTTrieNode {
		start := len(buf)
		for _, a := range active {
			advance(a, ch)
		}
		advance(p.floating, ch)
		return buf[start:len(buf):len(buf)]
	}

	var walk func(n *ciiNode, active []*ptDTTrieNode)
	walk = func(n *ciiNode, active []*ptDTTrieNode) {
		stack = append(stack, n)
		defer func() { stack = stack[:len(stack)-1] }()
		if len(active) == 1 {
			if len(active[0].terms) > 0 {
				claim(active[0].terms, n)
			}
		} else {
			var terms []ptDTTerm
			for _, a := range active {
				terms = append(terms, a.terms...)
			}
			if len(terms) > 0 {
				claim(terms, n)
			}
		}
		for _, ch := range n.children {
			mark := len(buf)
			if next := step(active, ch); len(next) > 0 {
				walk(ch, next)
			}
			buf = buf[:mark]
		}
	}

	stack = append(stack, root)
	// The document element is offered to the floating trie as well as to the
	// root-anchored terms, and the two are claimed together rather than one after
	// the other: first-match-wins is over the whole pattern, so a document element
	// that a match pattern also selects must still go to the lowest-numbered rule.
	// CIUS-RO has such a case — rule 20's context is `/ubl:Invoice | cac:CreditNote`
	// and rule 10's is `/ubl:Invoice | /cn:CreditNote` — and claiming twice would
	// evaluate BR-DEC-RO-13 and BR-DEC-RO-15 against a credit note that ISO
	// Schematron gives to rule 10.
	rootFloat := step(nil, root)
	rootTerms := append([]ptDTTerm(nil), p.rootTerms...)
	for _, a := range rootFloat {
		rootTerms = append(rootTerms, a.terms...)
	}
	if len(rootTerms) > 0 {
		claim(rootTerms, root)
	}
	active := append([]*ptDTTrieNode{p.trie}, rootFloat...)
	for _, ch := range root.children {
		mark := len(buf)
		if next := step(active, ch); len(next) > 0 {
			walk(ch, next)
		}
		buf = buf[:mark]
	}
	return entries
}

// ptDTRun evaluates one compiled pattern against a document and reports every
// assertion the document breaks, in document order, each rule's assertions in the
// order AT wrote them.
//
// errs counts the assertions whose evaluation raised an XPath dynamic error. Such an
// assertion is *not* reported: a reference processor aborts on one, so this package
// has no verdict to quote, and inventing one is how a false positive is born. The
// count is returned rather than swallowed so that a test can assert it is zero on
// AT's own instances.
func ptDTRun(p *ptDTCompiledPattern, r *run, root *ciiNode, doc *ptDTDoc, add func(rule, msg string)) (errs int) {
	entries := ptDTGather(p, root, doc)
	for _, e := range entries {
		if r != nil && r.stopped() {
			return errs
		}
		cr := &p.rules[e.rule]
		c := &ptDTCtx{
			item:      ptDTItem{kind: ptKNode, node: e.node},
			ancestors: e.ancestors,
			doc:       doc,
			lets:      cr.lets,
		}
		for i := range cr.asserts {
			a := &cr.asserts[i]
			v, aerr := ptDTEvalBool(a.expr, c)
			if aerr != nil {
				errs++
				continue
			}
			if v == a.report {
				add(a.id, a.message)
			}
		}
	}
	return errs
}

// ptDTValidate is the entry point: both CIUS-PT patterns over one UBL document.
//
// The two are separate Schematron patterns, so a node claimed by a rule of one is
// still offered to the other — first-match-wins is per pattern, not per document.
// The condition pattern's BR-* half is evaluated in cius_pt_rules.go; only its
// DT-CIUS-PT-* assertions are here, and the two halves live in the same <rule>
// elements upstream, so neither shadows the other.
func ptDTValidate(r *run, root *ciiNode, add func(rule, msg string)) int {
	// The two patterns share one document index, and it holds the union of the
	// names their rooted paths start from.
	doc := &ptDTDoc{root: root, want: ptDTRootNames}
	errs := ptDTRun(ptDatatype, r, root, doc, add)
	errs += ptDTRun(ptCondition, r, root, doc, add)
	return errs
}

// ptDTRootNames is the union of both patterns' rooted-path names.
var ptDTRootNames = func() map[string]bool {
	out := map[string]bool{}
	for _, p := range []*ptDTCompiledPattern{ptDatatype, ptCondition} {
		for n := range p.rootNames {
			out[n] = true
		}
	}
	return out
}()
