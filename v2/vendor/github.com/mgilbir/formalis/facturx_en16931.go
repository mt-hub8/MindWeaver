package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// The two ways this package can fail to read a document at all.
//
// Both are statements about the file rather than about the invoice, which is why
// they are errors and not findings: there is no document to judge, so any
// finding would be a claim about something nobody read. A stopped run is the
// other way round — see RuleLimit — and the two are never confused.
//
// They are sentinels so the discrimination a caller actually makes is available
// without parsing a message. "The sender's file is corrupt, ask for it again" and
// "this producer emits UTF-16, add a transcoding step" are different operational
// answers, and errors.Is is how a caller tells them apart. Nothing finer is
// offered on purpose: the decoder's own text is wrapped in and carries the line
// and column, which is the only thing that says *where* a document broke, and a
// taxonomy of XML defects on top of that would be a second vocabulary for
// something encoding/xml already describes well.
var (
	// ErrMalformedXML reports input the XML decoder rejected: not well-formed,
	// no root element at all, an entity reference this decoder refuses. The
	// wrapped error is the decoder's own, so %v on the result names the position.
	ErrMalformedXML = errors.New("the invoice XML is not well-formed")

	// ErrUnsupportedEncoding reports a document declaring a character encoding
	// this package does not implement. UTF-8, US-ASCII, ISO-8859-1/15 and
	// Windows-1252 are read; anything else is refused rather than passed through,
	// because the bytes of a UTF-16 or EBCDIC document read as UTF-8 are not the
	// document — element names come out mangled, and the rules would report
	// business-rule violations about text the sender never wrote. The wrapped
	// detail is the encoding the document declared.
	ErrUnsupportedEncoding = errors.New("the invoice XML declares a character encoding this package does not implement")
)

// One error matches neither sentinel, and deliberately: a document that nests
// deeper than maxDepth, which only the Is* predicates can return, because a
// validator answers that same document with a RuleLimit finding. It is neither
// malformed nor badly encoded — it is refused for what a tree of it would cost —
// so claiming either sentinel would send a caller to the wrong remedy. A caller
// that switches on the two sentinels needs a default arm, which it needs anyway.

// xmlCharsetReader lets the XML decoder read the non-UTF-8 encodings some
// national e-invoice formats declare (ISO-8859-1 / Windows-1252 are common in
// Austrian ebInterface). It stays dependency-free: ISO-8859-1 maps each byte to
// the same Unicode code point.
//
// An encoding it does not implement is an error rather than a passthrough. The
// bytes of, say, a UTF-16 or EBCDIC document read as UTF-8 are not the document:
// element names come out mangled, so the rules run against text the sender never
// wrote and report business-rule violations that say nothing about the invoice.
// Refusing to read it is the honest answer, and the caller gets it as an error
// rather than as a list of false accusations.
func xmlCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		// What the decoder already expects; ASCII is a subset of UTF-8.
		return input, nil
	case "iso-8859-1", "iso8859-1", "iso-8859-15", "iso8859-15", "latin1", "latin-1", "latin9", "windows-1252", "cp1252":
		b, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		sb.Grow(len(b))
		for _, c := range b {
			sb.WriteRune(rune(c))
		}
		return strings.NewReader(sb.String()), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnsupportedEncoding, charset)
}

// charsetTrap remembers an encoding refusal that the XML decoder would otherwise
// reduce to text.
//
// encoding/xml formats a CharsetReader failure with %v and not %w, so
// ErrUnsupportedEncoding does not survive the decoder and errors.Is on what
// Token returns cannot see it. Wrapping the reader keeps the original error where
// the two readers of these bytes — the tree parser and the streaming scan — can
// both reach it, which is what lets them classify one document the same way.
type charsetTrap struct{ err error }

func (t *charsetTrap) reader(charset string, input io.Reader) (io.Reader, error) {
	rd, err := xmlCharsetReader(charset, input)
	// Only the refusal is remembered. A read failure from the underlying reader
	// is not a statement about the declared encoding, and mislabelling it would
	// send a caller to add a transcoding step it does not need.
	if err != nil && errors.Is(err, ErrUnsupportedEncoding) {
		t.err = err
	}
	return rd, err
}

// classify names what defeated a read: the encoding refusal if there was one,
// otherwise the decoder's complaint under ErrMalformedXML.
func (t *charsetTrap) classify(err error) error {
	if t.err != nil {
		return t.err
	}
	return fmt.Errorf("%w: %w", ErrMalformedXML, err)
}

// This file begins the EN 16931 semantic validation of the invoice XML embedded
// in a Factur-X document: the UN/CEFACT Cross Industry Invoice (CII). It parses
// the XML and checks the foundational business rules that every profile shares —
// the mandatory document-level business terms and the invoice-total consistency.
// The rule identifiers (BR-*, BR-CO-*) and texts are those of EN 16931, as
// carried by the Factur-X Schematron; deeper rule families (VAT breakdowns, line
// items, allowances/charges, code lists, decimals) are layered on separately.
//
// The XML is walked namespace-agnostically by local element name, so it is
// resilient to the namespace-prefix variation seen across producers.

// ciiNode is a parsed CII XML element addressed by its local name.
type ciiNode struct {
	name string
	text string
	// textBuf accumulates character data while the element is open. An element's
	// text arrives as one xml.CharData token per run between markup, so a node
	// with many children — or one whose text is split by comments, CDATA
	// sections or processing instructions — receives many tokens. Appending each
	// to a []byte and materialising text once, on EndElement, keeps parsing
	// linear; `text += string(t)` reallocated and recopied the whole run per
	// token, which is quadratic and let a few megabytes of well-formed XML
	// (`x<!---->` repeated) occupy the parser for minutes.
	textBuf  []byte
	attrs    map[string]string // keyed by local attribute name
	children []*ciiNode
}

// closeNode materialises the accumulated character data and releases the
// accumulator. It is called when an element ends, and for any element left open
// at end of input.
func (n *ciiNode) closeNode() {
	if n.textBuf != nil {
		n.text = string(n.textBuf)
		n.textBuf = nil
	}
}

// attr returns the value of the named attribute (by local name), or "".
func (n *ciiNode) attr(name string) string {
	if n == nil {
		return ""
	}
	return n.attrs[name]
}

// hasAttr reports whether the named attribute is present, whatever its value.
// It is the distinction attr cannot make: several CEN binding rules are written
// as existence tests on an attribute node (`not(cbc:ID/@schemeID)`,
// `count(...@name)`), for which an attribute written empty is present.
func (n *ciiNode) hasAttr(name string) bool {
	if n == nil {
		return false
	}
	_, ok := n.attrs[name]
	return ok
}

// errStopped reports that the run ended before the document was fully parsed —
// the caller's context was cancelled, or the nesting cap tripped. It is distinct
// from a parse error because it says nothing about the document: a caller must
// report it as a RuleLimit finding, never as a defect in the file. The trip itself is
// already recorded on the run.
var errStopped = errors.New("the run stopped before the invoice was fully parsed")

// cancelParseTokens is how many XML tokens the parser covers between
// cancellation polls. The poll is a non-blocking channel receive, so it is cheap,
// but the token loop is the hot path for a large document and one poll per token
// is measurable. A thousand tokens is well under a millisecond of parsing, which
// keeps the parser's contribution to cancellation latency below the ~10 ms
// granularity pdf0 works to.
const cancelParseTokens = 1024

// parseCII parses invoice XML into a local-name element tree, or returns nil and
// an error if it could not be read. That error is always one of three things,
// and every exported validator's answer is decided by which: errStopped if r's
// context ended or the document nested deeper than maxDepth, and otherwise
// ErrUnsupportedEncoding or ErrMalformedXML, classified here so that no caller
// has to inspect a decoder message to tell them apart.
func parseCII(r *run, data []byte) (*ciiNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var trap charsetTrap
	dec.CharsetReader = trap.reader
	var stack []*ciiNode
	var root *ciiNode
	for n := 0; ; n++ {
		if n%cancelParseTokens == 0 && r.stopped() {
			return nil, errStopped
		}
		tok, err := dec.Token()
		if err != nil {
			// errors.Is, not a comparison against the error's text: scanShape reads
			// the same decoder and ends the same way, and TestScanMatchesTreeDetection
			// pins the two readers to identical acceptance decisions across the whole
			// corpus. One idiom, so a wrapped io.EOF cannot start a divergence
			// between them that reads as a malformed document on one side only.
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, trap.classify(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// Every tree walk in this package recurses once per level, and a
			// goroutine stack overflow is fatal rather than recoverable, so the
			// depth is capped here — once — instead of in each walk. The parse
			// stops rather than truncating the tree: a truncated tree would be
			// handed to the rule engine as if it were whole, which is how a guard
			// turns into a false accusation.
			if len(stack) >= maxDepth {
				r.note("xml-depth", fmt.Sprintf("the invoice XML nests deeper than %d elements", maxDepth))
				return nil, errStopped
			}
			// The tree is the largest thing this package allocates, and its size
			// is set by the element count rather than the nesting, so the two
			// guards are independent: a document of millions of shallow siblings
			// passes the depth check and still exhausts memory. Stopping here
			// rather than truncating, for the same reason as above.
			if !r.spendNode() {
				return nil, errStopped
			}
			n := &ciiNode{name: t.Name.Local}
			if len(t.Attr) > 0 {
				n.attrs = make(map[string]string, len(t.Attr))
				for _, a := range t.Attr {
					n.attrs[a.Name.Local] = a.Value
				}
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack[len(stack)-1].closeNode()
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				n.textBuf = append(n.textBuf, t...)
			}
		}
	}
	// Elements still open at end of input never saw their EndElement.
	for _, n := range stack {
		n.closeNode()
	}
	if root == nil {
		// A document with no root element is not well-formed XML: the standard
		// requires exactly one. Empty input reaches here, which is why "there was
		// nothing to read" and "what was there could not be read" are one answer.
		return nil, fmt.Errorf("%w: no root element", ErrMalformedXML)
	}
	return root, nil
}

// child returns the first descendant reached by following the given local names,
// or nil if any step is missing.
func (n *ciiNode) child(path ...string) *ciiNode {
	if n == nil {
		return nil
	}
	cur := n
	for _, name := range path {
		var next *ciiNode
		for _, c := range cur.children {
			if c.name == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// all returns every direct child with the given local name.
func (n *ciiNode) all(name string) []*ciiNode {
	if n == nil {
		return nil
	}
	var out []*ciiNode
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

// str returns the trimmed text at the given path, or "".
func (n *ciiNode) str(path ...string) string {
	if c := n.child(path...); c != nil {
		return strings.TrimSpace(c.text)
	}
	return ""
}

// Validate validates an invoice XML as Factur-X: against the EN 16931 core
// business rules, at the named Factur-X data-richness profile, and against the
// syntax binding Factur-X publishes for that profile rather than CEN's.
//
// It accepts either syntax — a UN/CEFACT Cross Industry Invoice
// (Factur-X/ZUGFeRD) or an OASIS UBL Invoice/CreditNote (Peppol BIS, XRechnung
// UBL) — detecting which from the root element and mapping it onto the shared
// semantic model before running the one rule engine. Factur-X is a CII format and
// publishes no UBL binding, so a UBL document handed to this call is judged by
// CEN's UBL binding, unscoped; there is nothing of Factur-X's for it to be judged
// by. Everything below is about the CII half.
//
// profile does two things, and the second is new. It excuses the rules a leaner
// tier is not expected to satisfy — MINIMUM and BASIC WL carry no invoice lines,
// so the line rules are not applied to them; MINIMUM also omits the buyer
// address, the VAT breakdown and the amount-due summation; EXTENDED is exempt
// from the allowance/charge total summations. And it selects the CII syntax
// binding: FNFE-MPE's for that profile, which carries four of CEN's 583
// CII-SR-*/CII-DT-* assertions and a profile data model of its own, plus, at
// EXTENDED, the BR-FXEXT-* rules Factur-X adds. Profile and SourceFacturX argue
// why; facturx.go is where it is done.
//
// A caller who wants CEN's own EN 16931 verdict — CEN's binding, no Factur-X
// rule — calls ValidateEN16931 instead. The two coincided until CEN's binding was
// measured against FNFE's published examples and found to report 76 fatal
// findings on 13 conforming EXTENDED invoices.
//
// What profile does *not* do is select a national rule set. It cannot make this
// call check XRechnung, Peppol or any other CIUS; for those use ValidateCIUS,
// which routes on the document's own BT-24, or the CIUS-specific validator.
//
// A Profile this package does not implement is refused rather than assumed:
// the call validates nothing and returns a single RuleProfile violation, so a
// typo cannot be read as a clean invoice.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice or credit note.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. That is
// a statement about the file rather than about the invoice, and it is the one
// answer no Report can carry honestly, since there is no document to judge. The
// Report returned with it is the zero Report, so a caller who ignores the error
// cannot read the value as clean either. Everything else, including a
// well-formed document that is not an invoice, is a finding: see ErrMalformedXML
// and RuleRoot.
//
// The Report names the rule families this package does not evaluate, from both
// Sources it declares — see Coverage(SourceEN16931) and Coverage(SourceFacturX),
// neither of which is empty. Neither holds a fatal evaluable gap, so
// Report.Conformant is true for a document with no fatal finding, which it was not
// while the Factur-X rule set was being landed a tier at a time.
// Report.Complete stays false: what is left under both Sources is advisory
// families and rules their own authority published and no processor can report,
// which is the distinction those two methods exist to draw.
func Validate(ctx context.Context, xmlData []byte, profile Profile) (Report, error) {
	if !knownProfile(profile) {
		// No Source: a rejected Profile chose no rule set, so there is no
		// coverage to report. The RuleProfile finding is what makes the Report
		// incomplete. It is not an error: the request was bad, the document was
		// never read, and an error would say this package could not read it.
		return newReport(unknownProfile(profile)), nil
	}
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceFacturX}, func(r *run, p *parsed) []Violation {
		return validateEN16931(r, p, profile, ciiBindingFacturX)
	})
}

// ValidateEN16931 validates an invoice XML against CEN's EN 16931 and nothing
// else: the semantic model's business rules and CEN's own syntax binding for
// whichever of the two syntaxes the document is in.
//
// It is Validate without the Factur-X reading, and it takes no Profile because
// CEN publishes none — the five data-richness tiers are FNFE-MPE's, and naming
// one is what asks for Factur-X's rule set instead of CEN's. A caller with a
// Factur-X document and its profile wants Validate; a caller asking "is this a
// conforming EN 16931 CII or UBL invoice", which is what a CEN reference
// validator answers, wants this.
//
// It is exactly what Validate with ProfileEN16931 did before Profile began
// selecting a binding, and the EN 16931 tier is where the difference is smallest:
// the two rule sets agree on the whole BR-* core there and differ only in the
// binding. They differ most at EXTENDED, whose whole purpose is to carry terms the
// EN 16931 core subset of CII does not have, and which CEN's binding therefore
// reports.
//
// Everything else — the error contract, the cancellation contract, the coverage
// the Report names — is Validate's, which documents it.
func ValidateEN16931(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931}, func(r *run, p *parsed) []Violation {
		return validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	})
}

// unknownProfile reports a Profile this package does not implement, as the one
// and only finding of a run that therefore examined nothing. See RuleProfile for
// why this is neither an error about the file, nor a finding about the document,
// nor a limit, and why it is not silence.
//
// The message names the accepted values because the failure this rejection
// exists to catch is a near miss — Profile("EN16931") for ProfileEN16931 is one
// space away, and is exactly the spelling ProfileFor takes as *input* — and it
// names the CIUS route because the other near miss is a caller reaching for a
// national rule set that Profile never offered.
func unknownProfile(p Profile) []Violation {
	names := make([]string, len(profiles))
	for i, k := range profiles {
		names[i] = strconv.Quote(string(k))
	}
	return []Violation{{
		Source:   SourceChecker,
		Rule:     RuleProfile,
		Severity: SeverityFatal,
		Message:  fmt.Sprintf("%q is not an EN 16931 conformance profile this checker implements, so no rules were run and this invoice is neither confirmed valid nor invalid; the profiles are %s, and a national CIUS is not one of them — for those use ValidateCIUS or the validator for that CIUS", string(p), strings.Join(names, ", ")),
	}}
}

// readFailure is the answer an exported validator gives when the parse produced
// no tree. There are exactly two, and which one is not a per-validator decision:
//
//   - the run was stopped — a cancelled context, a tripped budget — so nothing
//     was read and nothing can be said about the document. That is a statement
//     about the *run*, so it stays a finding: the RuleLimit trip r.finish appends
//     is the whole answer, and Report is not empty, which is the invariant a
//     caller testing len(r.Violations) == 0 depends on. pdf0 drains those trips
//     from one mixed slice by rule name, so they cannot become errors.
//   - the document could not be read at all. That is a statement about the file,
//     it is the one thing a Report has no honest way to express, and it comes
//     back as an error with the zero Report beside it — not conformant, not
//     complete, so a caller who ignores the error still cannot read the value as
//     clean.
//
// This is the one place that line is drawn, deliberately. The property finding C5
// asked for is that no validator can express a different answer, and there are
// two harnesses above this function and twenty-two validators above them.
func readFailure(r *run, err error, sources ...Source) (Report, error) {
	if errors.Is(err, errStopped) {
		return newReport(r.finish(nil), sources...), nil
	}
	return Report{}, err
}

// notAnInvoiceError reports a document that was read and is not an EN 16931
// invoice in either syntax.
//
// It is an error type only to travel out of parseEN16931 alongside the read
// failures; it never reaches a caller, because parseFailure turns it into a
// finding. Both of parseEN16931's callers go through parseFailure.
type notAnInvoiceError struct{ root string }

func (e *notAnInvoiceError) Error() string {
	return fmt.Sprintf("the invoice XML root %q is neither a CrossIndustryInvoice (CII) nor a UBL Invoice/CreditNote", e.root)
}

// parseFailure is readFailure with the third answer the semantic layer needs: a
// well-formed document that simply is not an invoice.
//
// That case is a finding and not an error, which is the boundary D8 leaves open
// and this is the argument for where it falls. "This file cannot be read" is a
// statement about the file; "this file is a Facturae, and I check EN 16931
// invoices" is a statement about the document, made after reading it
// successfully, and it is the same statement every tree-reading validator in this
// package already makes as a finding (FPA-root, ZA-root, ORDER-root, …). Answering
// it with an error here would mean the same question got two different kinds of
// answer depending on which entry point the caller reached for, which is the
// incoherence C5 was, one level up. A caller aggregating findings across formats
// keeps one drain, and RuleRoot is how it recognises this one.
func parseFailure(r *run, err error, sources ...Source) (Report, error) {
	var nai *notAnInvoiceError
	if errors.As(err, &nai) {
		v := Violation{Source: SourceChecker, Rule: RuleRoot, Severity: SeverityFatal, Message: nai.Error()}
		return newReport(r.finish([]Violation{v}), sources...), nil
	}
	return readFailure(r, err, sources...)
}

// parsed is one document, read once: the element tree and, when the document is
// an EN 16931 invoice in either syntax, the syntax-neutral model built from it.
//
// Threading this rather than the raw bytes is what makes maxNodes a property of
// the document instead of a property of how many layers the call happened to
// pass through. Every element the parser builds is drawn from one budget held by
// one run, so a second read of the same bytes would spend the budget twice and
// make the same document "too large" through a dispatching entry point and
// readable through a direct one. The exported wrappers parse; nothing below them
// does.
type parsed struct {
	root *ciiNode        // the local-name element tree
	inv  *en16931Invoice // the syntax-neutral model
}

// parseEN16931 parses the invoice XML and maps it onto the semantic model,
// dispatching on the root element to the CII or UBL mapper. It keeps the tree
// alongside the model: the UBL.BE rules are structural and want the tree, and
// re-reading the bytes to get it would spend the element budget a second time.
func parseEN16931(r *run, xmlData []byte) (*parsed, error) {
	root, err := parseCII(r, xmlData)
	if err != nil {
		// parseCII has already classified this — errStopped, ErrMalformedXML or
		// ErrUnsupportedEncoding — and re-wrapping it with a description of its own
		// would put "the invoice XML is not well-formed" in front of a cancelled
		// run and an encoding refusal alike.
		return nil, err
	}
	switch root.name {
	case "CrossIndustryInvoice":
		inv := mapCII(root)
		collectCommon(root, inv)
		return &parsed{root: root, inv: inv}, nil
	case "Invoice", "CreditNote":
		inv := mapUBL(root)
		collectCommon(root, inv)
		return &parsed{root: root, inv: inv}, nil
	}
	return nil, &notAnInvoiceError{root: root.name}
}

// modelValidate is the whole body of every exported entry point that validates
// against the syntax-neutral model: parse once, hand a parse failure to
// parseFailure, run the rule body, and report the coverage of the rule sets it
// ran.
//
// It is to the EN 16931 half what treeValidator is to the national half, and it
// exists for the same reason: nine entry points were writing out the same four
// lines, and the coverage claim is now one of them. A validator that named its
// own sources at the exit it happens to take could name a different set on the
// parse-failure path than on the success path, and Report.NotEvaluated would
// then depend on whether the document parsed. Here every path that produces a
// Report passes the same sources.
func modelValidate(ctx context.Context, xmlData []byte, sources []Source, check func(*run, *parsed) []Violation) (Report, error) {
	r := newRun(ctx)
	p, err := parseEN16931(r, xmlData)
	if err != nil {
		return parseFailure(r, err, sources...)
	}
	return newReport(r.finish(check(r, p)), sources...), nil
}

// findAll returns every descendant (self included) with the given local name.
func (n *ciiNode) findAll(name string) []*ciiNode {
	if n == nil {
		return nil
	}
	var out []*ciiNode
	var rec func(*ciiNode)
	rec = func(c *ciiNode) {
		if c.name == name {
			out = append(out, c)
		}
		for _, ch := range c.children {
			rec(ch)
		}
	}
	rec(n)
	return out
}

// matchPath returns every node in this subtree that an XSLT *relative* match
// pattern of the given local names selects — "cac:PayeeParty/cac:PartyLegalEntity/
// cbc:CompanyID" is matchPath("PayeeParty", "PartyLegalEntity", "CompanyID").
//
// It is not child() and it is not findAll(). child follows the first match at every
// step and answers one node; findAll answers every descendant with one name,
// whatever its parent. A Schematron rule context is neither: it is "every node
// whose ancestor chain ends in this sequence of names", at any depth, and a rule
// bound to cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID applies to
// each such identifier and not merely to the first.
//
// Each result is produced exactly once, because a node reachable by a path of
// length k has exactly one ancestor k steps above it.
func (n *ciiNode) matchPath(names ...string) []*ciiNode {
	if n == nil || len(names) == 0 {
		return nil
	}
	var out []*ciiNode
	var rec func(*ciiNode)
	rec = func(c *ciiNode) {
		cur := []*ciiNode{c}
		for _, nm := range names {
			var next []*ciiNode
			for _, x := range cur {
				next = append(next, x.all(nm)...)
			}
			if cur = next; len(cur) == 0 {
				break
			}
		}
		out = append(out, cur...)
		for _, ch := range c.children {
			rec(ch)
		}
	}
	rec(n)
	return out
}

// stringValue is the XPath string value of a node: the concatenation of every
// descendant text node, in document order. ciiNode.text holds only the element's
// own character data, which is the same thing for a leaf and is not for anything
// else — and one rule in this package (SRBDT's RSR-02) is bound to normalize-space()
// of a group element, where the difference is the whole rule.
func (n *ciiNode) stringValue() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var rec func(*ciiNode)
	rec = func(c *ciiNode) {
		b.WriteString(c.text)
		for _, ch := range c.children {
			rec(ch)
		}
	}
	rec(n)
	return b.String()
}

// normSpace is XPath's normalize-space: leading and trailing whitespace removed,
// internal runs collapsed to one space.
func normSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// collectAttr gathers the values of the named attribute across all descendants.
func (n *ciiNode) collectAttr(attr string) []string {
	if n == nil {
		return nil
	}
	var out []string
	var rec func(*ciiNode)
	rec = func(c *ciiNode) {
		if v := c.attr(attr); v != "" {
			out = append(out, v)
		}
		for _, ch := range c.children {
			rec(ch)
		}
	}
	rec(n)
	return out
}

// collectCommon fills the tree-wide code-list fields that are gathered the same
// way in either syntax (amount currency identifiers, party and object scheme
// identifiers — the latter using UBL element names, absent from CII so harmless).
func collectCommon(root *ciiNode, inv *en16931Invoice) {
	if root.name == "CrossIndustryInvoice" {
		inv.syntax = "CII"
	} else {
		inv.syntax = "UBL"
	}
	inv.currencyIDs = root.collectAttr("currencyID")
	for _, p := range root.findAll("PartyIdentification") {
		if s := p.child("ID").attr("schemeID"); s != "" {
			inv.partySchemes = append(inv.partySchemes, s)
		}
	}
	for _, p := range root.findAll("PartyLegalEntity") {
		if s := p.child("CompanyID").attr("schemeID"); s != "" {
			inv.legalSchemes = append(inv.legalSchemes, s)
		}
	}
	for _, d := range root.findAll("AdditionalDocumentReference") {
		if d.str("DocumentTypeCode") == "130" {
			if s := d.child("ID").attr("schemeID"); s != "" {
				inv.objectSchemes = append(inv.objectSchemes, s)
			}
		}
	}
	// A monetary amount (an element named *Amount) shall have at most two fraction
	// digits (UBL-DT-01) — except a unit price, which may carry more. Unit prices
	// are the *PriceAmount elements and anything under a price container (UBL cac:
	// Price, CII Net/GrossPriceProductTradePrice), so this holds in either syntax.
	var walk func(c *ciiNode, inPrice bool)
	walk = func(c *ciiNode, inPrice bool) {
		if !inPrice && strings.HasSuffix(c.name, "Amount") && !strings.HasSuffix(c.name, "PriceAmount") {
			if decimalCount(strings.TrimSpace(c.text)) > 2 {
				inv.amountDecimalsBad = true
			}
		}
		childInPrice := inPrice || c.name == "Price" ||
			c.name == "NetPriceProductTradePrice" || c.name == "GrossPriceProductTradePrice"
		for _, ch := range c.children {
			walk(ch, childInPrice)
		}
	}
	walk(root, false)
}

// ciiVATRegValue returns a CII party's VAT registration identifier (scheme "VA").
func ciiVATRegValue(p *ciiNode) string {
	for _, r := range p.all("SpecifiedTaxRegistration") {
		if id := r.child("ID"); id != nil && strings.TrimSpace(id.text) != "" && strings.EqualFold(id.attr("schemeID"), "VA") {
			return strings.TrimSpace(id.text)
		}
	}
	return ""
}

// ciiHasVATReg reports whether a CII trade party carries a VAT tax registration
// (SpecifiedTaxRegistration whose ID scheme is "VA").
func ciiHasVATReg(p *ciiNode) bool {
	return ciiVATRegValue(p) != ""
}

// ciiHasOtherReg reports whether a CII party carries a non-VAT tax registration.
func ciiHasOtherReg(p *ciiNode) bool {
	if p == nil {
		return false
	}
	for _, r := range p.all("SpecifiedTaxRegistration") {
		if id := r.child("ID"); id != nil && strings.TrimSpace(id.text) != "" && !strings.EqualFold(id.attr("schemeID"), "VA") {
			return true
		}
	}
	return false
}

// ublVATSchemeValue returns a UBL party's VAT PartyTaxScheme company identifier.
func ublVATSchemeValue(p *ciiNode) string {
	for _, pts := range p.all("PartyTaxScheme") {
		if id := pts.str("CompanyID"); id != "" && strings.EqualFold(pts.str("TaxScheme", "ID"), "VAT") {
			return id
		}
	}
	return ""
}

// ublHasVATScheme reports whether a UBL party carries a VAT PartyTaxScheme with a
// company identifier.
func ublHasVATScheme(p *ciiNode) bool {
	return ublVATSchemeValue(p) != ""
}

// ublHasOtherScheme reports whether a UBL party carries a non-VAT PartyTaxScheme
// company identifier.
func ublHasOtherScheme(p *ciiNode) bool {
	if p == nil {
		return false
	}
	for _, pts := range p.all("PartyTaxScheme") {
		if pts.str("CompanyID") != "" && !strings.EqualFold(pts.str("TaxScheme", "ID"), "VAT") {
			return true
		}
	}
	return false
}

// ciiPeriod extracts a CII billing period (BillingSpecifiedPeriod) from a node.
func ciiPeriod(n *ciiNode) invoicePeriod {
	p := n.child("BillingSpecifiedPeriod")
	if p == nil {
		return invoicePeriod{}
	}
	return invoicePeriod{present: true,
		start: p.str("StartDateTime", "DateTimeString"),
		end:   p.str("EndDateTime", "DateTimeString")}
}

// ublPeriod extracts a UBL invoice period (InvoicePeriod) from a node.
func ublPeriod(n *ciiNode) invoicePeriod {
	p := n.child("InvoicePeriod")
	if p == nil {
		return invoicePeriod{}
	}
	return invoicePeriod{present: true, start: p.str("StartDate"), end: p.str("EndDate"),
		desc: p.str("DescriptionCode")}
}

// mapCII extracts the EN 16931 business terms from a Cross Industry Invoice tree.
func mapCII(root *ciiNode) *en16931Invoice {
	doc := root.child("ExchangedDocument")
	tx := root.child("SupplyChainTradeTransaction")
	agr := tx.orNil().child("ApplicableHeaderTradeAgreement")
	settle := tx.orNil().child("ApplicableHeaderTradeSettlement")
	sum := settle.orNil().child("SpecifiedTradeSettlementHeaderMonetarySummation")

	inv := &en16931Invoice{
		specID:               root.str("ExchangedDocumentContext", "GuidelineSpecifiedDocumentContextParameter", "ID"),
		profileID:            root.str("ExchangedDocumentContext", "BusinessProcessSpecifiedDocumentContextParameter", "ID"),
		orderRef:             tx.orNil().str("ApplicableHeaderTradeAgreement", "BuyerOrderReferencedDocument", "IssuerAssignedID"),
		number:               doc.orNil().str("ID"),
		issueDate:            doc.orNil().str("IssueDateTime", "DateTimeString"),
		typeCode:             doc.orNil().str("TypeCode"),
		currency:             settle.orNil().str("InvoiceCurrencyCode"),
		sellerName:           agr.orNil().str("SellerTradeParty", "Name"),
		buyerName:            agr.orNil().str("BuyerTradeParty", "Name"),
		sellerCountry:        agr.orNil().str("SellerTradeParty", "PostalTradeAddress", "CountryID"),
		sellerAddressPresent: agr.orNil().child("SellerTradeParty", "PostalTradeAddress") != nil,
		buyerCountry:         agr.orNil().str("BuyerTradeParty", "PostalTradeAddress", "CountryID"),
		buyerAddressPresent:  agr.orNil().child("BuyerTradeParty", "PostalTradeAddress") != nil,
		sellerVATID:          ciiHasVATReg(agr.orNil().child("SellerTradeParty")),
		sellerTaxReg:         ciiHasOtherReg(agr.orNil().child("SellerTradeParty")),
		taxRepVATID:          ciiHasVATReg(agr.orNil().child("SellerTaxRepresentativeTradeParty")),
		buyerVATID:           ciiHasVATReg(agr.orNil().child("BuyerTradeParty")),
		buyerLegalReg:        agr.orNil().str("BuyerTradeParty", "SpecifiedLegalOrganization", "ID") != "",
		sellerEndpointScheme: agr.orNil().child("SellerTradeParty", "URIUniversalCommunication", "URIID").attr("schemeID"),
		buyerEndpointScheme:  agr.orNil().child("BuyerTradeParty", "URIUniversalCommunication", "URIID").attr("schemeID"),
		period:               ciiPeriod(settle.orNil()),
		taxRepPresent:        agr.orNil().child("SellerTaxRepresentativeTradeParty") != nil,
		taxRepName:           agr.orNil().str("SellerTaxRepresentativeTradeParty", "Name"),
		taxRepAddressPresent: agr.orNil().child("SellerTaxRepresentativeTradeParty", "PostalTradeAddress") != nil,
		taxRepCountry:        agr.orNil().str("SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "CountryID"),
		payeePresent:         settle.orNil().child("PayeeTradeParty") != nil,
		payeeName:            settle.orNil().str("PayeeTradeParty", "Name"),
		deliverToPresent:     tx.orNil().child("ApplicableHeaderTradeDelivery", "ShipToTradeParty", "PostalTradeAddress") != nil,
		deliverToCountry:     tx.orNil().str("ApplicableHeaderTradeDelivery", "ShipToTradeParty", "PostalTradeAddress", "CountryID"),
	}
	pms := settle.orNil().all("SpecifiedTradeSettlementPaymentMeans")
	inv.paymentInstrPresent = len(pms) > 0
	for _, pm := range pms {
		if tc := pm.str("TypeCode"); tc != "" {
			inv.paymentMeans = append(inv.paymentMeans, tc)
		}
		if acc := pm.child("PayeePartyCreditorFinancialAccount"); acc != nil {
			inv.creditAccountPresent = true
			if id := firstNonEmpty(acc.str("IBANID"), acc.str("ProprietaryID")); id != "" {
				inv.creditAccountID = id
			}
		}
		// BT-87, the payment card primary account number (BR-51). The CII binding
		// carries it as the card's ram:ID.
		for _, card := range pm.all("ApplicableTradeSettlementFinancialCard") {
			inv.cardPANs = append(inv.cardPANs, card.str("ID"))
		}
	}
	inv.taxCurrency = settle.orNil().str("TaxCurrencyCode")
	if inv.taxCurrency != "" {
		for _, ta := range sum.orNil().all("TaxTotalAmount") {
			if strings.EqualFold(ta.attr("currencyID"), inv.taxCurrency) {
				inv.vatInTaxCurrency = true
				inv.vatInTaxCurrencyAmt = strings.TrimSpace(ta.text)
			}
		}
	}
	if sum != nil {
		inv.hasTotals = true
		inv.totals = monetaryTotals{
			lineTotal:       sum.str("LineTotalAmount"),
			allowanceTotal:  sum.str("AllowanceTotalAmount"),
			chargeTotal:     sum.str("ChargeTotalAmount"),
			taxBasisTotal:   sum.str("TaxBasisTotalAmount"),
			taxTotal:        sum.str("TaxTotalAmount"),
			grandTotal:      sum.str("GrandTotalAmount"),
			paidAmount:      sum.str("TotalPrepaidAmount"),
			payableRounding: sum.str("RoundingAmount"),
			duePayable:      sum.str("DuePayableAmount"),
		}
	}
	for _, tt := range settle.orNil().all("ApplicableTradeTax") {
		inv.vatBreakdowns = append(inv.vatBreakdowns, vatBreakdown{
			basis:      tt.str("BasisAmount"),
			calc:       tt.str("CalculatedAmount"),
			category:   tt.str("CategoryCode"),
			rate:       tt.str("RateApplicablePercent"),
			hasReason:  tt.str("ExemptionReason") != "" || tt.str("ExemptionReasonCode") != "",
			reasonCode: tt.str("ExemptionReasonCode"),
		})
	}
	for _, ac := range settle.orNil().all("SpecifiedTradeAllowanceCharge") {
		inv.allowCharges = append(inv.allowCharges, docAllowanceCharge{
			amount:     ac.str("ActualAmount"),
			baseAmount: ac.str("BasisAmount"),
			category:   ac.str("CategoryTradeTax", "CategoryCode"),
			rate:       ac.str("CategoryTradeTax", "RateApplicablePercent"),
			hasReason:  ac.str("Reason") != "" || ac.str("ReasonCode") != "",
			reasonCode: ac.str("ReasonCode"),
			isCharge:   strings.EqualFold(ac.str("ChargeIndicator", "Indicator"), "true"),
		})
	}
	for _, li := range tx.orNil().all("IncludedSupplyChainTradeLineItem") {
		line := invoiceLine{
			lineID:        li.str("AssociatedDocumentLineDocument", "LineID"),
			parentLineID:  li.str("AssociatedDocumentLineDocument", "ParentLineID"),
			itemName:      li.str("SpecifiedTradeProduct", "Name"),
			netAmount:     li.str("SpecifiedLineTradeSettlement", "SpecifiedTradeSettlementLineMonetarySummation", "LineTotalAmount"),
			price:         li.str("SpecifiedLineTradeAgreement", "NetPriceProductTradePrice", "ChargeAmount"),
			vatCategory:   li.str("SpecifiedLineTradeSettlement", "ApplicableTradeTax", "CategoryCode"),
			vatRate:       li.str("SpecifiedLineTradeSettlement", "ApplicableTradeTax", "RateApplicablePercent"),
			originCountry: li.str("SpecifiedTradeProduct", "OriginTradeCountry", "ID"),
			period:        ciiPeriod(li.child("SpecifiedLineTradeSettlement")),
			grossPrice:    li.str("SpecifiedLineTradeAgreement", "GrossPriceProductTradePrice", "ChargeAmount"),
			baseQty:       li.str("SpecifiedLineTradeAgreement", "NetPriceProductTradePrice", "BasisQuantity"),
			baseQtyUnit:   li.child("SpecifiedLineTradeAgreement", "NetPriceProductTradePrice", "BasisQuantity").attr("unitCode"),
		}
		if qty := li.child("SpecifiedLineTradeDelivery", "BilledQuantity"); qty != nil {
			line.quantity = strings.TrimSpace(qty.text)
			line.unitCode = qty.attr("unitCode")
		}
		prod := li.child("SpecifiedTradeProduct").orNil()
		if g := prod.child("GlobalID"); g != nil {
			line.stdIDPresent, line.stdIDScheme = true, g.attr("schemeID")
		}
		if c := prod.child("DesignatedProductClassification", "ClassCode"); c != nil {
			line.classPresent, line.classListID = true, c.attr("listID")
		}
		for _, a := range prod.all("ApplicableProductCharacteristic") {
			if a.str("Description") == "" || a.str("Value") == "" {
				line.itemAttrBad = true
			}
		}
		for _, ac := range li.orNil().child("SpecifiedLineTradeSettlement").orNil().all("SpecifiedTradeAllowanceCharge") {
			line.allowCharges = append(line.allowCharges, lineAllowanceCharge{
				amount:     ac.str("ActualAmount"),
				baseAmount: ac.str("BasisAmount"),
				hasReason:  ac.str("Reason") != "" || ac.str("ReasonCode") != "",
				isCharge:   strings.EqualFold(ac.str("ChargeIndicator", "Indicator"), "true"),
			})
		}
		if li.str("SpecifiedLineTradeAgreement", "BuyerOrderReferencedDocument", "LineID") != "" {
			inv.hasOrderLineRef = true
		}
		inv.lines = append(inv.lines, line)
	}
	// Supporting documents (BG-24) and preceding invoice references (BG-3).
	for _, d := range agr.orNil().all("AdditionalReferencedDocument") {
		if d.str("TypeCode") != "916" { // 916 = supporting document
			continue
		}
		bin := d.child("AttachmentBinaryObject")
		inv.docRefs = append(inv.docRefs, docReference{
			hasID:         d.str("IssuerAssignedID") != "",
			binaryPresent: bin != nil,
			mimeCode:      bin.attr("mimeCode"),
			filename:      bin.attr("filename"),
		})
	}
	for _, r := range settle.orNil().all("InvoiceReferencedDocument") {
		inv.hasBillingRef = true
		if r.str("IssuerAssignedID") == "" {
			inv.billingRefNoID = true
		}
	}
	inv.creditorID = settle.orNil().str("CreditorReferenceID")
	for _, pm := range settle.orNil().all("SpecifiedTradeSettlementPaymentMeans") {
		if acc := pm.child("PayerPartyDebtorFinancialAccount"); acc != nil {
			inv.directDebitPresent = true
			inv.debitedAccount = firstNonEmpty(acc.str("IBANID"), acc.str("ProprietaryID"))
		}
	}
	if m := settle.orNil().str("SpecifiedTradePaymentTerms", "DirectDebitMandateID"); m != "" {
		inv.directDebitPresent = true
		inv.mandateRef = m
	}
	inv.sellerVATIDValue = ciiVATRegValue(agr.child("SellerTradeParty"))
	inv.taxRepVATIDValue = ciiVATRegValue(agr.child("SellerTaxRepresentativeTradeParty"))
	inv.buyerVATIDValue = ciiVATRegValue(agr.child("BuyerTradeParty"))
	inv.sellerID = agr.str("SellerTradeParty", "ID")
	inv.sellerLegalReg = agr.str("SellerTradeParty", "SpecifiedLegalOrganization", "ID")
	inv.sellerEndpointPresent = agr.str("SellerTradeParty", "URIUniversalCommunication", "URIID") != ""
	inv.buyerEndpointPresent = agr.str("BuyerTradeParty", "URIUniversalCommunication", "URIID") != ""
	inv.deliveryDate = tx.str("ApplicableHeaderTradeDelivery", "ActualDeliverySupplyChainEvent", "OccurrenceDateTime", "DateTimeString")
	inv.buyerReference = agr.str("BuyerReference")
	inv.sellerCity = agr.str("SellerTradeParty", "PostalTradeAddress", "CityName")
	inv.sellerStreet = agr.str("SellerTradeParty", "PostalTradeAddress", "LineOne")
	inv.sellerPostCode = agr.str("SellerTradeParty", "PostalTradeAddress", "PostcodeCode")
	inv.sellerLegalScheme = agr.child("SellerTradeParty", "SpecifiedLegalOrganization", "ID").attr("schemeID")
	inv.buyerLegalScheme = agr.child("BuyerTradeParty", "SpecifiedLegalOrganization", "ID").attr("schemeID")
	inv.buyerStreet = agr.str("BuyerTradeParty", "PostalTradeAddress", "LineOne")
	inv.taxRepStreet = agr.str("SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "LineOne")
	inv.taxRepCity = agr.str("SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "CityName")
	inv.taxRepPostCode = agr.str("SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "PostcodeCode")
	if c := agr.child("SellerTradeParty", "DefinedTradeContact"); c != nil {
		inv.sellerContactPresent = true
		inv.sellerContactName = firstNonEmpty(c.str("PersonName"), c.str("DepartmentName"))
		inv.sellerPhone = c.str("TelephoneUniversalCommunication", "CompleteNumber")
		inv.sellerEmail = c.str("EmailURIUniversalCommunication", "URIID")
	}
	inv.buyerCity = agr.str("BuyerTradeParty", "PostalTradeAddress", "CityName")
	inv.buyerPostCode = agr.str("BuyerTradeParty", "PostalTradeAddress", "PostcodeCode")
	inv.deliverToCity = tx.str("ApplicableHeaderTradeDelivery", "ShipToTradeParty", "PostalTradeAddress", "CityName")
	inv.deliverToPostCode = tx.str("ApplicableHeaderTradeDelivery", "ShipToTradeParty", "PostalTradeAddress", "PostcodeCode")
	inv.deliverToStreet = tx.str("ApplicableHeaderTradeDelivery", "ShipToTradeParty", "PostalTradeAddress", "LineOne")
	inv.sellerSubentity = agr.str("SellerTradeParty", "PostalTradeAddress", "CountrySubDivisionName")
	inv.buyerSubentity = agr.str("BuyerTradeParty", "PostalTradeAddress", "CountrySubDivisionName")
	inv.deliverToSubentity = tx.str("ApplicableHeaderTradeDelivery", "ShipToTradeParty", "PostalTradeAddress", "CountrySubDivisionName")
	inv.taxRepSubentity = agr.str("SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "CountrySubDivisionName")
	// BT-21, the Invoice note subject code (BR-CL-08). CII gives it an element of
	// its own, and the CEN rule's context is every ram:SubjectCode in the document
	// — a note on a line carries the same code list as one on the head.
	for _, sc := range root.findAll("SubjectCode") {
		if v := strings.TrimSpace(sc.text); v != "" {
			inv.noteSubjectCodes = append(inv.noteSubjectCodes, v)
		}
	}
	// BT-71, the Deliver-to location identifier (BR-CL-26). In CII it is the
	// ship-to party's ram:GlobalID; the scheme is what the rule constrains.
	for _, g := range tx.child("ApplicableHeaderTradeDelivery", "ShipToTradeParty").all("GlobalID") {
		if s := g.attr("schemeID"); s != "" {
			inv.deliverToLocSchemes = append(inv.deliverToLocSchemes, s)
		}
	}
	return inv
}
func round2(f float64) float64 { return math.Round(f*100) / 100 }

// normDate reduces a date to a fixed-width comparable calendar date (YYYYMMDD)
// and reports whether it could read one, so the CII and UBL forms compare
// lexically and an unreadable value is told apart from a readable one.
//
// Accepted, in either binding's spelling:
//
//	2013-06-01              UBL xs:date
//	20130601                CII UDT DateTimeString, format 102
//	2013-06-01Z             either, with a UTC designator
//	2013-06-01+02:00        either, with a timezone offset
//	2013-06-01T09:30:00     either, with a time of day
//
// Anything else — an empty value, a two-digit year, "1 June 2013", a nine-digit
// run — is not a date this package will order, and yields false.
//
// The width is the whole point. The previous form kept only the digits, so a
// legal xs:date carrying a timezone reduced to twelve of them: "2024-02-01+02:00"
// became "202402010200", and against the eight of "20240201" the shorter string
// is a prefix of the longer and so compares LESS. Two values naming the same
// calendar day therefore ordered as if one preceded the other, which fired BR-29,
// BR-30 and PEPPOL-EN16931-R110/R111 on the one case that must never fire.
//
// The timezone is read and discarded rather than applied. BT-73/74 and
// BT-134/135 are calendar dates in the EN 16931 semantic model — the day a
// billing period starts, not an instant — so an invoicing period that starts and
// ends on 2024-02-01 is one day long however either end is offset, and no
// ordering rule may say otherwise. (XPath's xs:date comparison, which the
// Schematron uses, does apply the offset and would order these two apart. That
// is a defensible reading of the datatype and an indefensible reading of the
// business term, so this package takes the business term's.)
//
// A caller must skip its ordering check when either side yields false, rather
// than compare a value it could not read: accusing an invoice of an out-of-order
// period on the strength of a date neither side understood is a false accusation
// dressed as a business-rule finding.
func normDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	digits := func(sub string) bool {
		for i := 0; i < len(sub); i++ {
			if sub[i] < '0' || sub[i] > '9' {
				return false
			}
		}
		return true
	}
	// Whatever follows the date part must open a timezone or a time of day; a
	// further digit means the leading run was never a date to begin with.
	tail := func(rest string) bool {
		switch {
		case rest == "":
			return true
		case rest[0] == 'T', rest[0] == 'Z', rest[0] == '+', rest[0] == '-':
			return true
		}
		return false
	}
	// Extended form, YYYY-MM-DD.
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' && digits(s[:4]) && digits(s[5:7]) && digits(s[8:10]) {
		if tail(s[10:]) {
			return s[:4] + s[5:7] + s[8:10], true
		}
		return "", false
	}
	// Basic form, YYYYMMDD.
	if len(s) >= 8 && digits(s[:8]) && tail(s[8:]) {
		return s[:8], true
	}
	return "", false
}

// decimalCount returns the number of digits after the decimal point in s.
func decimalCount(s string) int {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return len(s) - i - 1
	}
	return 0
}

// isUpperAlpha reports whether s is exactly n uppercase ASCII letters (the shape
// of ISO 4217 currency and ISO 3166-1 alpha-2 country codes).
func isUpperAlpha(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < n; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

// orNil lets a possibly-nil node be traversed without panicking. Its zero value
// is load-bearing beyond the traversal: several validators write `x.orNil()`
// and then test `x.name == ""` as the sentinel for "that element was absent"
// (facturae.go, ksef.go, ebinterface.go).
//
// The fresh node looks like an allocation per nil hit, and a package-level
// shared instance looks like the obvious fix, but the measurement says
// otherwise and the shared instance is the worse shape. orNil is small enough
// to inline, and it is inlined at all 86 of its call sites here; at every one of
// them the compiler proves the node does not escape and puts it on the stack.
// `go build -gcflags=-m` reports "&ciiNode{} does not escape" 86 times and
// "escapes to heap" once, for the out-of-line copy of this function that nothing
// in the package reaches. So there is no allocation to remove: AllocsPerRun over
// mapCII on a bare <CrossIndustryInvoice/>, the document that reaches this 41
// times in one call, counts 1 allocation, and that one is the
// en16931Invoice. Benchmarks of mapCII, mapUBL and Validate against a corpus
// invoice and against that bare root are identical, to the byte and to the
// allocation, with and without a shared node.
//
// Sharing one would also be worse than neutral. It is safe only while nothing
// ever writes through the returned pointer — true today, since parseCII is the
// only thing in this package that assigns to a ciiNode's fields and every node
// it touches is one it allocated — but that is an invariant a reader has to be
// told about, in a package whose exported functions are otherwise concurrency-
// safe without anyone having to check. The fresh node is private by
// construction, and in the one case where it *would* escape it has to be: a
// shared node aliased into a caller's data structure is the bug the private one
// cannot have.
//
// TestOrNilDoesNotAllocate pins the escape-analysis property, so a change that
// makes this function too large to inline is caught rather than discovered.
func (n *ciiNode) orNil() *ciiNode {
	if n == nil {
		return &ciiNode{}
	}
	return n
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseAmount parses a CII decimal amount.
func parseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// ublDocumentTaxTotal selects the cac:TaxTotal that carries the invoice's VAT in
// the *document* currency: the Invoice total VAT amount (BT-110) and the BG-23
// VAT breakdown groups.
//
// An invoice that declares a VAT accounting currency (BT-6 cbc:TaxCurrencyCode)
// carries two cac:TaxTotal elements — one in the document currency holding BT-110
// and the cac:TaxSubtotal groups, and one in the accounting currency holding
// BT-111 and no subtotals. Nothing constrains their order: not EN 16931, not the
// UBL binding, and not Peppol BIS (PEPPOL-EN16931-R053/R054 constrain their count
// and subtotal-presence, not their sequence). Taking whichever appears first
// therefore reads BT-110 and the entire breakdown out of the accounting-currency
// element whenever a producer emits that one first, which fabricates BR-CO-18,
// BR-CO-15 and BR-{fam}-01 findings on a conforming invoice.
//
// The currency identifier is the primary signal: a cac:TaxTotal whose
// cbc:TaxAmount/@currencyID equals the Invoice currency code (BT-5) is the
// document-currency one, and so is one carrying no @currencyID at all — many
// producers omit it, and the UBL binding's implied currency is the document's.
// Where that leaves more than one candidate, or none, the presence of
// cac:TaxSubtotal children breaks the tie, because the binding puts the breakdown
// only in the document-currency element.
//
// A document with a single cac:TaxTotal always selects that one, whatever it is
// tagged with. That is the shape of essentially every invoice, and treating it
// any other way would turn a mis-tagged @currencyID into a missing breakdown.
func ublDocumentTaxTotal(root *ciiNode, currency string) *ciiNode {
	tts := root.all("TaxTotal")
	if len(tts) < 2 {
		if len(tts) == 0 {
			return nil
		}
		return tts[0]
	}
	var candidates []*ciiNode
	for _, tt := range tts {
		if cur := tt.child("TaxAmount").attr("currencyID"); cur == "" || strings.EqualFold(cur, currency) {
			candidates = append(candidates, tt)
		}
	}
	if len(candidates) == 0 {
		// Neither element names the document currency. The breakdown is the only
		// remaining evidence; failing that, document order.
		candidates = tts
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	for _, tt := range candidates {
		if len(tt.all("TaxSubtotal")) > 0 {
			return tt
		}
	}
	return candidates[0]
}

// ublNoteSubjectCode reads the Invoice note subject code (BT-21) out of a UBL
// note, or returns "" when the note carries none.
//
// The UBL binding has no element for BT-21. It writes the code into the note
// text as a "#CODE#" prefix, and the CEN rule reproduces that convention
// exactly: the note carries a subject code only when it contains a '#' *and*
// the text between the first two '#' characters is exactly three characters
// long. Anything else — a note with no '#', a note that mentions "#" in prose,
// a prefix of some other length — is a note with no subject code, not a note
// with an invalid one. Reading it any more liberally would turn "see #1 above"
// into a code-list violation.
func ublNoteSubjectCode(note string) string {
	i := strings.IndexByte(note, '#')
	if i < 0 {
		return ""
	}
	rest := note[i+1:]
	j := strings.IndexByte(rest, '#')
	if j != 3 {
		return ""
	}
	return rest[:3]
}

// mapUBL extracts the EN 16931 business terms from an OASIS UBL Invoice or
// CreditNote. The tree is parsed namespace-agnostically (parseCII), so the cbc:/
// cac: prefixes are already stripped to local names. The document-type element
// names differ between an Invoice and a CreditNote.
func mapUBL(root *ciiNode) *en16931Invoice {
	typeCodeName, lineName, qtyName := "InvoiceTypeCode", "InvoiceLine", "InvoicedQuantity"
	if root.name == "CreditNote" {
		typeCodeName, lineName, qtyName = "CreditNoteTypeCode", "CreditNoteLine", "CreditedQuantity"
	}
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()
	total := root.child("LegalMonetaryTotal")
	currency := root.str("DocumentCurrencyCode")
	taxTotal := ublDocumentTaxTotal(root, currency).orNil()

	inv := &en16931Invoice{
		specID:    root.str("CustomizationID"),
		number:    root.str("ID"),
		issueDate: root.str("IssueDate"),
		typeCode:  root.str(typeCodeName),
		currency:  currency,
		// BT-27/BT-44 bind to the legal registration name; some producers carry
		// the name only in cac:PartyName, so fall back to it.
		sellerName:           firstNonEmpty(seller.str("PartyLegalEntity", "RegistrationName"), seller.str("PartyName", "Name")),
		buyerName:            firstNonEmpty(buyer.str("PartyLegalEntity", "RegistrationName"), buyer.str("PartyName", "Name")),
		sellerCountry:        seller.str("PostalAddress", "Country", "IdentificationCode"),
		sellerAddressPresent: seller.child("PostalAddress") != nil,
		buyerCountry:         buyer.str("PostalAddress", "Country", "IdentificationCode"),
		buyerAddressPresent:  buyer.child("PostalAddress") != nil,
		sellerVATID:          ublHasVATScheme(seller),
		sellerTaxReg:         ublHasOtherScheme(seller),
		taxRepVATID:          ublHasVATScheme(root.child("TaxRepresentativeParty").orNil()),
		buyerVATID:           ublHasVATScheme(buyer),
		buyerLegalReg:        buyer.str("PartyLegalEntity", "CompanyID") != "",
		sellerEndpointScheme: seller.child("EndpointID").attr("schemeID"),
		buyerEndpointScheme:  buyer.child("EndpointID").attr("schemeID"),
		period:               ublPeriod(root),
		taxRepPresent:        root.child("TaxRepresentativeParty") != nil,
		taxRepName:           firstNonEmpty(root.str("TaxRepresentativeParty", "PartyName", "Name"), root.str("TaxRepresentativeParty", "PartyLegalEntity", "RegistrationName")),
		taxRepAddressPresent: root.child("TaxRepresentativeParty", "PostalAddress") != nil,
		taxRepCountry:        root.str("TaxRepresentativeParty", "PostalAddress", "Country", "IdentificationCode"),
		payeePresent:         root.child("PayeeParty") != nil,
		payeeName:            root.str("PayeeParty", "PartyName", "Name"),
		deliverToPresent:     root.child("Delivery", "DeliveryLocation", "Address") != nil,
		deliverToCountry:     root.str("Delivery", "DeliveryLocation", "Address", "Country", "IdentificationCode"),
	}
	pms := root.all("PaymentMeans")
	inv.paymentInstrPresent = len(pms) > 0
	for _, pm := range pms {
		if code := pm.str("PaymentMeansCode"); code != "" {
			inv.paymentMeans = append(inv.paymentMeans, code)
		}
		if acc := pm.child("PayeeFinancialAccount"); acc != nil {
			inv.creditAccountPresent = true
			if id := acc.str("ID"); id != "" {
				inv.creditAccountID = id
			}
		}
		// BT-87, the payment card primary account number (BR-51). The UBL binding
		// carries it as cac:CardAccount/cbc:PrimaryAccountNumberID.
		for _, card := range pm.all("CardAccount") {
			inv.cardPANs = append(inv.cardPANs, card.str("PrimaryAccountNumberID"))
		}
	}
	inv.taxCurrency = root.str("TaxCurrencyCode")
	if inv.taxCurrency != "" {
		// BT-111 is the VAT total in the accounting currency: a cac:TaxTotal whose
		// cbc:TaxAmount is tagged with BT-6. This is a presence test over every
		// cac:TaxTotal, deliberately independent of which one ublDocumentTaxTotal
		// picked — the two selections answer different questions, and the one the
		// mapper chose for BT-110 is by construction not the accounting-currency
		// one unless BT-6 and BT-5 are equal, which PEPPOL-EN16931-R005 forbids.
		for _, tt := range root.all("TaxTotal") {
			if strings.EqualFold(tt.child("TaxAmount").attr("currencyID"), inv.taxCurrency) {
				inv.vatInTaxCurrency = true
				inv.vatInTaxCurrencyAmt = tt.str("TaxAmount")
				break
			}
		}
	}
	if total != nil {
		inv.hasTotals = true
		inv.totals = monetaryTotals{
			lineTotal:       total.str("LineExtensionAmount"),
			allowanceTotal:  total.str("AllowanceTotalAmount"),
			chargeTotal:     total.str("ChargeTotalAmount"),
			taxBasisTotal:   total.str("TaxExclusiveAmount"),
			grandTotal:      total.str("TaxInclusiveAmount"),
			paidAmount:      total.str("PrepaidAmount"),
			payableRounding: total.str("PayableRoundingAmount"),
			duePayable:      total.str("PayableAmount"),
		}
	}
	// The Invoice total VAT amount (BT-110) lives in TaxTotal, independent of the
	// document monetary summation, so read it even without a LegalMonetaryTotal.
	// Assigned after the block above, which replaces the whole struct.
	inv.totals.taxTotal = taxTotal.str("TaxAmount")
	// The BG-23 breakdown groups come from that same element: BT-110 and the
	// subtotals that must sum to it are one statement in one currency.
	for _, ts := range taxTotal.all("TaxSubtotal") {
		inv.vatBreakdowns = append(inv.vatBreakdowns, vatBreakdown{
			basis:      ts.str("TaxableAmount"),
			calc:       ts.str("TaxAmount"),
			category:   ts.str("TaxCategory", "ID"),
			rate:       ts.str("TaxCategory", "Percent"),
			hasReason:  ts.str("TaxCategory", "TaxExemptionReason") != "" || ts.str("TaxCategory", "TaxExemptionReasonCode") != "",
			reasonCode: ts.str("TaxCategory", "TaxExemptionReasonCode"),
		})
	}
	for _, ac := range root.all("AllowanceCharge") {
		inv.allowCharges = append(inv.allowCharges, docAllowanceCharge{
			amount:     ac.str("Amount"),
			baseAmount: ac.str("BaseAmount"),
			category:   ac.str("TaxCategory", "ID"),
			rate:       ac.str("TaxCategory", "Percent"),
			hasReason:  ac.str("AllowanceChargeReason") != "" || ac.str("AllowanceChargeReasonCode") != "",
			reasonCode: ac.str("AllowanceChargeReasonCode"),
			isCharge:   strings.EqualFold(ac.str("ChargeIndicator"), "true"),
		})
	}
	for _, li := range root.all(lineName) {
		line := invoiceLine{
			lineID:        li.str("ID"),
			itemName:      li.str("Item", "Name"),
			netAmount:     li.str("LineExtensionAmount"),
			price:         li.str("Price", "PriceAmount"),
			vatCategory:   li.str("Item", "ClassifiedTaxCategory", "ID"),
			vatRate:       li.str("Item", "ClassifiedTaxCategory", "Percent"),
			originCountry: li.str("Item", "OriginCountry", "IdentificationCode"),
			period:        ublPeriod(li),
			grossPrice:    li.child("Price", "AllowanceCharge").str("BaseAmount"),
			baseQty:       li.str("Price", "BaseQuantity"),
			baseQtyUnit:   li.child("Price", "BaseQuantity").attr("unitCode"),
		}
		if qty := li.child(qtyName); qty != nil {
			line.quantity = strings.TrimSpace(qty.text)
			line.unitCode = qty.attr("unitCode")
		}
		item := li.child("Item").orNil()
		if s := item.child("StandardItemIdentification", "ID"); s != nil {
			line.stdIDPresent, line.stdIDScheme = true, s.attr("schemeID")
		}
		if c := item.child("CommodityClassification", "ItemClassificationCode"); c != nil {
			line.classPresent, line.classListID = true, c.attr("listID")
		}
		for _, a := range item.all("AdditionalItemProperty") {
			if a.str("Name") == "" || a.str("Value") == "" {
				line.itemAttrBad = true
			}
		}
		for _, ac := range li.all("AllowanceCharge") {
			line.allowCharges = append(line.allowCharges, lineAllowanceCharge{
				amount:     ac.str("Amount"),
				baseAmount: ac.str("BaseAmount"),
				hasReason:  ac.str("AllowanceChargeReason") != "" || ac.str("AllowanceChargeReasonCode") != "",
				isCharge:   strings.EqualFold(ac.str("ChargeIndicator"), "true"),
			})
		}
		if li.str("OrderLineReference", "LineID") != "" {
			inv.hasOrderLineRef = true
		}
		inv.lines = append(inv.lines, line)
	}
	for _, d := range root.all("AdditionalDocumentReference") {
		bin := d.child("Attachment", "EmbeddedDocumentBinaryObject")
		inv.docRefs = append(inv.docRefs, docReference{
			hasID:         d.str("ID") != "",
			binaryPresent: bin != nil,
			mimeCode:      bin.attr("mimeCode"),
			filename:      bin.attr("filename"),
		})
	}
	for _, r := range root.all("BillingReference") {
		inv.hasBillingRef = true
		if r.str("InvoiceDocumentReference", "ID") == "" {
			inv.billingRefNoID = true
		}
	}
	for _, p := range append(seller.all("PartyIdentification"), root.child("PayeeParty").all("PartyIdentification")...) {
		if strings.EqualFold(p.child("ID").attr("schemeID"), "SEPA") {
			inv.creditorID = p.str("ID")
		}
	}
	for _, pm := range root.all("PaymentMeans") {
		if m := pm.child("PaymentMandate"); m != nil {
			inv.directDebitPresent = true
			inv.debitedAccount = m.str("PayerFinancialAccount", "ID")
			inv.mandateRef = m.str("ID")
		}
	}
	inv.sellerVATIDValue = ublVATSchemeValue(seller)
	inv.taxRepVATIDValue = ublVATSchemeValue(root.child("TaxRepresentativeParty"))
	inv.buyerVATIDValue = ublVATSchemeValue(buyer)
	inv.sellerID = seller.str("PartyIdentification", "ID")
	inv.sellerLegalReg = seller.str("PartyLegalEntity", "CompanyID")
	inv.sellerEndpointPresent = seller.str("EndpointID") != ""
	inv.buyerEndpointPresent = buyer.str("EndpointID") != ""
	inv.vatPointDate = root.str("TaxPointDate")
	inv.deliveryDate = root.str("Delivery", "ActualDeliveryDate")
	inv.profileID = root.str("ProfileID")
	inv.isCreditNote = root.name == "CreditNote"
	inv.orderRef = root.str("OrderReference", "ID")
	inv.buyerReference = root.str("BuyerReference")
	inv.sellerCity = seller.str("PostalAddress", "CityName")
	inv.sellerStreet = seller.str("PostalAddress", "StreetName")
	inv.sellerPostCode = seller.str("PostalAddress", "PostalZone")
	inv.sellerLegalScheme = seller.child("PartyLegalEntity", "CompanyID").attr("schemeID")
	inv.buyerLegalScheme = buyer.child("PartyLegalEntity", "CompanyID").attr("schemeID")
	inv.buyerStreet = buyer.str("PostalAddress", "StreetName")
	inv.taxRepStreet = root.str("TaxRepresentativeParty", "PostalAddress", "StreetName")
	inv.taxRepCity = root.str("TaxRepresentativeParty", "PostalAddress", "CityName")
	inv.taxRepPostCode = root.str("TaxRepresentativeParty", "PostalAddress", "PostalZone")
	if c := seller.child("Contact"); c != nil {
		inv.sellerContactPresent = true
		inv.sellerContactName = c.str("Name")
		inv.sellerPhone = c.str("Telephone")
		inv.sellerEmail = c.str("ElectronicMail")
	}
	inv.buyerCity = buyer.str("PostalAddress", "CityName")
	inv.buyerPostCode = buyer.str("PostalAddress", "PostalZone")
	inv.deliverToCity = root.str("Delivery", "DeliveryLocation", "Address", "CityName")
	inv.deliverToPostCode = root.str("Delivery", "DeliveryLocation", "Address", "PostalZone")
	inv.deliverToStreet = root.str("Delivery", "DeliveryLocation", "Address", "StreetName")
	inv.sellerSubentity = seller.str("PostalAddress", "CountrySubentity")
	inv.buyerSubentity = buyer.str("PostalAddress", "CountrySubentity")
	inv.deliverToSubentity = root.str("Delivery", "DeliveryLocation", "Address", "CountrySubentity")
	inv.taxRepSubentity = root.str("TaxRepresentativeParty", "PostalAddress", "CountrySubentity")
	// BT-21, the Invoice note subject code (BR-CL-08). UBL has no element for it:
	// the binding prefixes the note text with "#CODE#", and the CEN rule reads the
	// code back out of cbc:Note on the document element only — a note on an
	// invoice line (BT-127) carries no subject code.
	for _, n := range root.all("Note") {
		if c := ublNoteSubjectCode(n.text); c != "" {
			inv.noteSubjectCodes = append(inv.noteSubjectCodes, c)
		}
	}
	// BT-71, the Deliver-to location identifier (BR-CL-26).
	for _, loc := range root.findAll("DeliveryLocation") {
		for _, id := range loc.all("ID") {
			if s := id.attr("schemeID"); s != "" {
				inv.deliverToLocSchemes = append(inv.deliverToLocSchemes, s)
			}
		}
	}
	return inv
}
