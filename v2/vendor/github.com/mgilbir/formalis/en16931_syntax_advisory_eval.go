package formalis

import (
	"fmt"
	"strings"
)

// The compiler and evaluator for the generated advisory tables. Three things
// happen here, and they are separate on purpose:
//
//   - compileAdvisoryPattern turns a generated table into a form that can be run:
//     each assertion's XPath is parsed once at load, and each is checked to be in
//     a position the evaluator can honour. Anything it cannot is a panic at load
//     rather than a rule that quietly never fires.
//   - gatherAdvisoryNodes walks the document once, assigns every element to the
//     first rule whose context matches it — ISO Schematron's semantics, and the
//     reason CII-DT-010/011/012 are unreportable — and indexes the handful of
//     names the document-wide `//` paths need.
//   - runAdvisoryPattern evaluates each node's assertions.
//
// # One walk, not 1,168
//
// The naive shape of this — evaluate 1,168 XPaths against the tree — would walk
// the document a thousand times per invoice, and this package validates 1,690
// documents on every test run. Two things keep it to one walk and a bounded
// amount of work per node.
//
// The walk is single: it collects the node population of every context and, in
// the same pass, the document-wide index that the `//` paths read. Nothing below
// re-traverses.
//
// And an assertion is only evaluated when it could possibly fail. 1,141 of the
// 1,168 have the shape not(path) with path relative to the context node, so if
// the context node has no child named by the path's first step the assertion is
// vacuously satisfied. compileAdvisoryPattern works out which assertions have
// that property — by evaluating each against an empty context node, not by
// pattern-matching its source — and gives each one a bit; the evaluator sets the
// bits for the child names a node actually has and tests them with a shift and a
// mask. So a UBL invoice answers its 697 assertions with one pass over the
// document element's children and 697 bit tests, and the cost of the whole family
// is a few microseconds rather than a few milliseconds.

// advisoryPattern is a generated table compiled for evaluation.
type advisoryPattern struct {
	rules []advisoryCompiledRule

	// byName maps an element local name to the indices of the rules whose match
	// could accept it, ascending, and suffixRules holds the indices of the rules
	// matched by name suffix, ascending. Together they are how an element finds
	// its rule without testing all of them: pattern order is index order, so the
	// first index in either list whose match succeeds is the rule ISO Schematron
	// would give the node to.
	byName      map[string][]int
	suffixRules []int

	// rootNames and rootAttrs are the element and attribute names the
	// document-wide `//` paths start from — the only names the index has to hold.
	// Indexing the whole document would cost a slice per distinct element name on
	// every invoice to answer two rules.
	rootNames map[string]bool
	rootAttrs map[string]bool

	// scratch is how wide the per-node child-name bitset has to be.
	scratchWords int
	// asserts is the total number of advisory assertions, which the tests ratchet.
	asserts int
}

// advisoryCompiledRule is one <rule> of the pattern, ready to run.
type advisoryCompiledRule struct {
	context string
	match   advisoryMatch
	asserts []advisoryCompiledAssert

	// nameID numbers the child names the conditional assertions of this rule
	// depend on, so the evaluator can record which of them a node has in a
	// bitset instead of a map.
	nameID map[string]int

	// needsAncestors is set when some assertion uses the ancestor axis, and is
	// what decides whether the walk copies a node's ancestor names. One rule in
	// the CII binding does (CII-DT-041/054/058, which exempt a header-level trade
	// tax), so the copy is made for that rule's nodes and for no others.
	needsAncestors bool
}

// advisoryCompiledAssert is one assertion with its XPath parsed.
type advisoryCompiledAssert struct {
	id      string
	test    string
	message string
	expr    *advisoryExpr

	// bits are the nameID bits of the child names this assertion reads. When
	// conditional is set and none of them is present on the context node, the
	// assertion is satisfied without being evaluated.
	bits        []int
	conditional bool
}

// advisoryIndex is the document-wide index the `//` paths read: the elements of
// each name a rooted path starts from, and the elements carrying each attribute
// one names.
type advisoryIndex struct {
	byName map[string][]*ciiNode
	byAttr map[string][]*ciiNode
}

// advisoryEntry is one element together with the rule that claimed it.
type advisoryEntry struct {
	rule int
	node *ciiNode
	// parent is the element's parent, or — for the document element — a synthetic
	// node standing in for the document node, so that a `../` step means what
	// XPath says it means rather than nothing. UBL-CR-412 is the one rule that
	// needs it.
	parent *ciiNode
	// ancestors are the local names of the element's ancestors, innermost last.
	// It is nil unless the claiming rule uses the ancestor axis.
	ancestors []string
}

// advisoryCtx is the evaluation context for one assertion.
type advisoryCtx struct {
	node      *ciiNode
	parent    *ciiNode
	idx       *advisoryIndex
	ancestors []string
}

// The two compiled patterns. They are built at load from the generated tables
// and only read afterwards, like the code-list maps and the compiled regexps
// elsewhere in this package, so they are safe to share across goroutines.
//
// A parse failure panics. That is the same choice regexp.MustCompile makes and
// for the same reason: the table is generated and committed, so a row this
// evaluator cannot read is a defect in the build rather than anything a caller
// did, TestAdvisorySyntaxTableParses fails on it before it can ship, and the
// alternative — dropping the row — is a rule that silently stops being checked.
var (
	advisoryUBL = mustCompileAdvisoryPattern("advisoryUBLPattern", advisoryUBLPattern)
	advisoryCII = mustCompileAdvisoryPattern("advisoryCIIPattern", advisoryCIIPattern)
)

func mustCompileAdvisoryPattern(name string, rules []advisorySyntaxRule) *advisoryPattern {
	p, err := compileAdvisoryPattern(rules)
	if err != nil {
		panic("formalis: " + name + ": " + err.Error())
	}
	return p
}

// compileAdvisoryPattern parses every assertion in a generated table and works
// out how to run it.
func compileAdvisoryPattern(rules []advisorySyntaxRule) (*advisoryPattern, error) {
	p := &advisoryPattern{
		byName:    map[string][]int{},
		rootNames: map[string]bool{},
		rootAttrs: map[string]bool{},
	}
	for i, r := range rules {
		cr := advisoryCompiledRule{context: r.context, match: r.match, nameID: map[string]int{}}
		for _, a := range r.asserts {
			expr, err := advisorySyntaxParse(a.test)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", a.id, a.test, err)
			}
			var u advisoryUsage
			if err := u.collect(expr, false); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", a.id, a.test, err)
			}
			ca := advisoryCompiledAssert{id: a.id, test: a.test, message: a.message, expr: expr}
			for n := range u.rootNames {
				p.rootNames[n] = true
			}
			for n := range u.rootAttrs {
				p.rootAttrs[n] = true
			}
			cr.needsAncestors = cr.needsAncestors || u.ancestor
			// An assertion may be skipped when none of the child names it reads is
			// present only if nothing else in it can depend on the document: a
			// rooted `//` path, a step out of the context node, the self or
			// ancestor axis. Given that, "no child of any of those names" is
			// exactly the empty context node, so the question is settled by
			// evaluating the assertion there rather than by recognising its shape.
			if len(u.firstNames) > 0 && !u.root && !u.parent && !u.self && !u.ancestor && !u.relativeAttr &&
				advisoryEvalBool(expr, &advisoryCtx{node: &ciiNode{}, parent: &ciiNode{}, idx: &advisoryIndex{}}) {
				ca.conditional = true
				for _, n := range u.firstNames {
					id, ok := cr.nameID[n]
					if !ok {
						id = len(cr.nameID)
						cr.nameID[n] = id
					}
					ca.bits = append(ca.bits, id)
				}
			}
			cr.asserts = append(cr.asserts, ca)
			p.asserts++
		}
		if w := (len(cr.nameID) + 63) / 64; w > p.scratchWords {
			p.scratchWords = w
		}
		p.rules = append(p.rules, cr)

		// The index a node consults to find its rule. A rule matched by name
		// suffix cannot be keyed by name; there are seven of those across the two
		// bindings and they are tested linearly.
		switch {
		case len(r.match.names) > 0:
			for _, n := range r.match.names {
				p.byName[n] = append(p.byName[n], i)
			}
		case len(r.match.paths) > 0:
			for _, path := range r.match.paths {
				n := path[len(path)-1]
				if l := p.byName[n]; len(l) == 0 || l[len(l)-1] != i {
					p.byName[n] = append(p.byName[n], i)
				}
			}
		case r.match.suffix != "":
			p.suffixRules = append(p.suffixRules, i)
		default:
			return nil, fmt.Errorf("the rule with context %q names no element, no path and no suffix, "+
				"so nothing can find it", r.context)
		}
	}
	return p, nil
}

// advisoryUsage is what compileAdvisoryPattern needs to know about an
// expression: which names it reads, and which constructs it uses.
type advisoryUsage struct {
	firstNames []string        // first-step names of its relative paths
	rootNames  map[string]bool // first-step element names of its `//` paths
	rootAttrs  map[string]bool // first-step attribute names of its `//` paths

	root         bool // uses a `//` path
	parent       bool // steps out of the context node with `../`
	self         bool // uses the self axis
	ancestor     bool // uses the ancestor axis
	relativeAttr bool // reads an attribute of the context node directly
}

// collect walks an expression, recording what it reads and refusing anything the
// evaluator would get wrong. inPred is set while descending into a step
// predicate, where the context node changes and the axes this evaluator carries
// no state for would silently mean something else.
func (u *advisoryUsage) collect(e *advisoryExpr, inPred bool) error {
	switch e.op {
	case advOr, advAnd, advNot:
		for _, a := range e.args {
			if err := u.collect(a, inPred); err != nil {
				return err
			}
		}
	case advEq, advNe:
		l, r := e.args[0], e.args[1]
		switch l.op {
		case advExists, advNormSpace, advCount, advSub, advNumber:
		default:
			return fmt.Errorf("a comparison's left side must be a path, normalize-space, a count or a number")
		}
		switch r.op {
		case advLiteral, advBool, advNumber, advCount, advSub:
		default:
			return fmt.Errorf("a comparison's right side must be a literal, a boolean, a number or a count")
		}
		if (l.op == advCount || l.op == advSub || l.op == advNumber) != (r.op == advCount || r.op == advSub || r.op == advNumber) {
			return fmt.Errorf("a comparison mixes a numeric side with a sequence side")
		}
		for _, a := range e.args {
			if err := u.collect(a, inPred); err != nil {
				return err
			}
		}
	case advRel:
		for _, a := range e.args {
			switch a.op {
			case advCount, advSub, advNumber:
			default:
				return fmt.Errorf("%s compares two numbers, and one side is not one", e.relop)
			}
			if err := u.collect(a, inPred); err != nil {
				return err
			}
		}
	case advSub:
		for _, a := range e.args {
			if err := u.collect(a, inPred); err != nil {
				return err
			}
		}
	case advExists, advCount, advNormSpace:
		return u.collectPath(e.path, inPred)
	case advSelf, advAncestor:
		if inPred {
			return fmt.Errorf("the %s axis inside a predicate would be read against the wrong node", map[advisoryOp]string{advSelf: "self", advAncestor: "ancestor"}[e.op])
		}
		if e.op == advSelf {
			u.self = true
		} else {
			u.ancestor = true
		}
	case advNumber, advLiteral, advBool:
	default:
		return fmt.Errorf("unhandled operator %d", e.op)
	}
	return nil
}

func (u *advisoryUsage) collectPath(path *advisoryPath, inPred bool) error {
	first := path.steps[0]
	switch {
	case path.fromRoot:
		u.root = true
		if first.attr != "" {
			if u.rootAttrs == nil {
				u.rootAttrs = map[string]bool{}
			}
			u.rootAttrs[first.attr] = true
		} else {
			if u.rootNames == nil {
				u.rootNames = map[string]bool{}
			}
			for _, n := range first.names {
				u.rootNames[n] = true
			}
		}
	case path.fromParent:
		if inPred {
			return fmt.Errorf("a `../` step inside a predicate would be read against the wrong node")
		}
		u.parent = true
	case first.attr != "":
		u.relativeAttr = true
	default:
		if !inPred {
			u.firstNames = append(u.firstNames, first.names...)
		}
	}
	for _, s := range path.steps {
		if s.pred != nil {
			if err := u.collect(s.pred, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Running a pattern
// ---------------------------------------------------------------------------

// advisorySyntaxRules evaluates the advisory half of whichever binding the
// document is in. It is a no-op for a root neither binding describes, so a
// FatturaPA document reaching this code path is never asked to answer a UBL rule.
func advisorySyntaxRules(r *run, root *ciiNode) []Violation {
	if root == nil {
		return nil
	}
	var p *advisoryPattern
	switch root.name {
	case "Invoice", "CreditNote":
		p = advisoryUBL
	case "CrossIndustryInvoice":
		p = advisoryCII
	default:
		return nil
	}
	var out []Violation
	runAdvisoryPattern(p, r, root, advisoryAdder(&out, SourceEN16931))
	return out
}

// runAdvisoryPattern walks the document once and reports every advisory
// assertion the document breaks: the claimed elements in document order, and each
// element's assertions in the order CEN wrote them.
//
// The cancellation check is per element rather than per assertion. It is a
// non-blocking channel receive either way, and an element's assertions are
// bounded by the pattern, so the granularity costs at most one element's work on
// a cancelled run and saves a check per rule on every uncancelled one.
func runAdvisoryPattern(p *advisoryPattern, r *run, root *ciiNode, add func(rule, msg string)) {
	idx, entries := gatherAdvisoryNodes(p, root)
	scratch := make([]uint64, p.scratchWords)
	// One context reused across the entries. It is read by the evaluator and never
	// retained, and the alternative allocates one per matched element on every
	// document validated.
	var c advisoryCtx
	c.idx = idx
	for _, e := range entries {
		if r.stopped() {
			return
		}
		cr := &p.rules[e.rule]
		for i := range scratch {
			scratch[i] = 0
		}
		if len(cr.nameID) > 0 {
			for _, ch := range e.node.children {
				if id, ok := cr.nameID[ch.name]; ok {
					scratch[id/64] |= 1 << uint(id%64)
				}
			}
		}
		c.node, c.parent, c.ancestors = e.node, e.parent, e.ancestors
		for i := range cr.asserts {
			a := &cr.asserts[i]
			if a.conditional && !advisoryAnyBit(scratch, a.bits) {
				continue
			}
			if !advisoryEvalBool(a.expr, &c) {
				add(a.id, a.message+" ("+a.test+")")
			}
		}
	}
}

func advisoryAnyBit(words []uint64, bits []int) bool {
	for _, b := range bits {
		if words[b/64]&(1<<uint(b%64)) != 0 {
			return true
		}
	}
	return false
}

// gatherAdvisoryNodes is the single walk: it assigns every element to the first
// rule whose context matches it and indexes the names the rooted paths read.
//
// The document element gets a synthetic parent standing in for the XPath
// document node, so a `../` step out of it selects the document element's
// siblings — which is to say the document element itself, since a well-formed
// document has exactly one.
func gatherAdvisoryNodes(p *advisoryPattern, root *ciiNode) (*advisoryIndex, []advisoryEntry) {
	idx := &advisoryIndex{}
	if len(p.rootNames) > 0 {
		idx.byName = map[string][]*ciiNode{}
	}
	if len(p.rootAttrs) > 0 {
		idx.byAttr = map[string][]*ciiNode{}
	}
	var entries []advisoryEntry
	docNode := &ciiNode{children: []*ciiNode{root}}
	stack := make([]*ciiNode, 0, 16)

	var rec func(n, parent *ciiNode)
	rec = func(n, parent *ciiNode) {
		stack = append(stack, n)
		if p.rootNames[n.name] {
			idx.byName[n.name] = append(idx.byName[n.name], n)
		}
		if len(p.rootAttrs) > 0 {
			for a := range n.attrs {
				if p.rootAttrs[a] {
					idx.byAttr[a] = append(idx.byAttr[a], n)
				}
			}
		}
		// A rule with no advisory assertion needs no entry: its whole job is to
		// claim the node away from the rules below it, and ruleFor has just done
		// that. Skipping it here is what keeps the entry list to the nodes that
		// have something to answer, rather than to every element in the document
		// whose name happens to end in "Amount".
		if i := p.ruleFor(n, stack); i >= 0 && len(p.rules[i].asserts) > 0 {
			e := advisoryEntry{rule: i, node: n, parent: parent}
			if p.rules[i].needsAncestors {
				e.ancestors = make([]string, 0, len(stack)-1)
				for _, a := range stack[:len(stack)-1] {
					e.ancestors = append(e.ancestors, a.name)
				}
			}
			entries = append(entries, e)
		}
		for _, ch := range n.children {
			rec(ch, n)
		}
		stack = stack[:len(stack)-1]
	}
	rec(root, docNode)
	return idx, entries
}

// ruleFor is ISO Schematron's first-match-wins, and returns -1 for an element no
// context matches. Pattern order is index order, so the answer is the smallest
// index whose match succeeds.
func (p *advisoryPattern) ruleFor(n *ciiNode, stack []*ciiNode) int {
	best := len(p.rules)
	for _, i := range p.byName[n.name] {
		if p.rules[i].match.matches(n, stack) {
			best = i
			break
		}
	}
	for _, i := range p.suffixRules {
		if i >= best {
			break
		}
		if p.rules[i].match.matches(n, stack) {
			best = i
			break
		}
	}
	if best == len(p.rules) {
		return -1
	}
	return best
}

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

// advisoryEvalBool is the assertion's own verdict: true means the document
// satisfies it.
func advisoryEvalBool(e *advisoryExpr, c *advisoryCtx) bool {
	switch e.op {
	case advOr:
		return advisoryEvalBool(e.args[0], c) || advisoryEvalBool(e.args[1], c)
	case advAnd:
		return advisoryEvalBool(e.args[0], c) && advisoryEvalBool(e.args[1], c)
	case advNot:
		return !advisoryEvalBool(e.args[0], c)
	case advExists:
		return advisoryPathExists(e.path, c)
	case advEq, advNe:
		return advisoryCompare(e, c)
	case advRel:
		l, r := advisoryEvalNum(e.args[0], c), advisoryEvalNum(e.args[1], c)
		switch e.relop {
		case "<=":
			return l <= r
		case ">=":
			return l >= r
		case "<":
			return l < r
		case ">":
			return l > r
		}
		return false
	case advCount:
		return advisoryPathExists(e.path, c)
	case advBool:
		return e.b
	case advSelf:
		return c.node.name == e.str
	case advAncestor:
		return containsString(c.ancestors, e.str)
	}
	return false
}

func advisoryEvalNum(e *advisoryExpr, c *advisoryCtx) float64 {
	switch e.op {
	case advNumber:
		return e.num
	case advCount:
		return float64(len(advisorySelect(e.path, c)))
	case advSub:
		return advisoryEvalNum(e.args[0], c) - advisoryEvalNum(e.args[1], c)
	}
	return 0
}

// advisoryCompare is XPath's general comparison, whose behaviour on an empty
// sequence is the one thing about this subset that has to be got right. `a = 'x'`
// and `a != 'x'` are both false when a selects nothing, because the comparison is
// existentially quantified over the left sequence — so an assertion written
// `not(a) or a = '2.1'` passes on a document with no a, and one written
// `a != 'x'` fails on it. Reading either as its logical negation would turn a
// silent rule into a false positive on every conforming invoice.
func advisoryCompare(e *advisoryExpr, c *advisoryCtx) bool {
	l, r := e.args[0], e.args[1]
	eq := e.op == advEq

	// count(x) = 1 and its like: both sides are numbers, and an empty sequence
	// counts zero rather than vanishing.
	if l.op == advCount || l.op == advSub || l.op == advNumber {
		return (advisoryEvalNum(l, c) == advisoryEvalNum(r, c)) == eq
	}

	for _, ref := range advisorySelect(l.path, c) {
		v := strings.TrimSpace(ref.value())
		if l.op == advNormSpace {
			v = normalizeSpace(v)
		}
		switch r.op {
		case advLiteral:
			if (v == r.str) == eq {
				return true
			}
		case advBool:
			// Under queryBinding="xslt2" an untyped value compared with xs:boolean
			// is cast to xs:boolean. A value that is neither of the two lexical
			// forms is a dynamic error in a reference validator; here it matches
			// neither side, which is the reading en16931_ubl_rules.go already takes
			// of the same construct in the two document-level allowance contexts.
			b, ok := advisoryCastBool(v)
			if ok && (b == r.b) == eq {
				return true
			}
		}
	}
	return false
}

func advisoryCastBool(v string) (bool, bool) {
	switch v {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

// advisorySelect evaluates a location path against the context.
func advisorySelect(path *advisoryPath, c *advisoryCtx) []advisoryRef {
	first := path.steps[0]
	var cur []advisoryRef
	switch {
	case path.fromRoot:
		// `//x` is `/descendant-or-self::node()/x`, an absolute step: it is
		// document-wide whatever the context node is.
		if first.attr != "" {
			for _, n := range c.idx.byAttr[first.attr] {
				cur = append(cur, advisoryRef{node: n, attr: first.attr})
			}
		} else {
			for _, name := range first.names {
				for _, n := range c.idx.byName[name] {
					if advisoryPredOK(first.pred, n, c) {
						cur = append(cur, advisoryRef{node: n})
					}
				}
			}
		}
	case path.fromParent:
		cur = advisoryStepFromNode(c.parent, first, c)
	default:
		// The first step of a relative path is taken from the single context node,
		// which is the overwhelmingly common case: wrapping that node in a
		// one-element slice to reach the general routine below allocates once per
		// assertion evaluated, and there are 1,168 assertions.
		cur = advisoryStepFromNode(c.node, first, c)
	}
	for _, s := range path.steps[1:] {
		if len(cur) == 0 {
			return nil
		}
		cur = advisoryStepFrom(cur, s, c)
	}
	return cur
}

// advisoryStepFromNode takes one step from one element.
func advisoryStepFromNode(n *ciiNode, s advisoryStep, c *advisoryCtx) []advisoryRef {
	if n == nil {
		return nil
	}
	if s.attr != "" {
		if n.hasAttr(s.attr) {
			return []advisoryRef{{node: n, attr: s.attr}}
		}
		return nil
	}
	var out []advisoryRef
	for _, ch := range n.children {
		if containsString(s.names, ch.name) && advisoryPredOK(s.pred, ch, c) {
			out = append(out, advisoryRef{node: ch})
		}
	}
	return out
}

func advisoryStepFrom(refs []advisoryRef, s advisoryStep, c *advisoryCtx) []advisoryRef {
	var out []advisoryRef
	for _, r := range refs {
		if r.attr != "" {
			// An attribute has no children; XPath allows the step and selects
			// nothing.
			continue
		}
		if s.attr != "" {
			if r.node.hasAttr(s.attr) {
				out = append(out, advisoryRef{node: r.node, attr: s.attr})
			}
			continue
		}
		for _, ch := range r.node.children {
			if containsString(s.names, ch.name) && advisoryPredOK(s.pred, ch, c) {
				out = append(out, advisoryRef{node: ch})
			}
		}
	}
	return out
}

// advisoryPathExists is advisorySelect reduced to the one question 1,141 of the
// 1,168 assertions ask: does this path select anything at all.
//
// It exists for allocation rather than for clarity. advisorySelect materialises
// the node set at each step, and a bare `not(cac:InvoicePeriod/cbc:StartTime)`
// pays for a slice at the first step on every invoice that has an invoicing
// period, on every one of a few hundred assertions that reach evaluation. Asking
// the question recursively instead answers it with no allocation at all and stops
// at the first hit.
func advisoryPathExists(path *advisoryPath, c *advisoryCtx) bool {
	first := path.steps[0]
	rest := path.steps[1:]
	switch {
	case path.fromRoot:
		if first.attr != "" {
			return len(rest) == 0 && len(c.idx.byAttr[first.attr]) > 0
		}
		for _, name := range first.names {
			for _, n := range c.idx.byName[name] {
				if !advisoryPredOK(first.pred, n, c) {
					continue
				}
				if len(rest) == 0 || advisoryExistsFrom(n, rest, c) {
					return true
				}
			}
		}
		return false
	case path.fromParent:
		return advisoryExistsFrom(c.parent, path.steps, c)
	default:
		return advisoryExistsFrom(c.node, path.steps, c)
	}
}

func advisoryExistsFrom(n *ciiNode, steps []advisoryStep, c *advisoryCtx) bool {
	if n == nil {
		return false
	}
	s := steps[0]
	if s.attr != "" {
		// An attribute node has no children, so a step after one selects nothing.
		return len(steps) == 1 && n.hasAttr(s.attr)
	}
	for _, ch := range n.children {
		if !containsString(s.names, ch.name) || !advisoryPredOK(s.pred, ch, c) {
			continue
		}
		if len(steps) == 1 || advisoryExistsFrom(ch, steps[1:], c) {
			return true
		}
	}
	return false
}

// advisoryPredOK evaluates a step predicate with n as the context node. The
// axes and the parent step are refused inside a predicate at compile time —
// see advisoryUsage.collect — so nothing here reads state that has not moved
// with n.
func advisoryPredOK(pred *advisoryExpr, n *ciiNode, c *advisoryCtx) bool {
	if pred == nil {
		return true
	}
	return advisoryEvalBool(pred, &advisoryCtx{node: n, idx: c.idx})
}
