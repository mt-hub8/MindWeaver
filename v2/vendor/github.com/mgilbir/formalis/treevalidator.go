package formalis

import "context"

// The validators in this package come in two shapes. The EN 16931 half — the
// core rule engine and the CIUS layered on it — parses a document onto the
// syntax-neutral semantic model and runs one rule engine over it, so UBL and CII
// are read by the same rules. The national formats below it have no such model:
// FatturaPA, Facturae, ebInterface, KSeF, Finvoice, TEAPPS, OIOUBL, Svefaktura,
// ZATCA, NAV OSA, UBL-TR, PINT and Order-X are each their own vocabulary, and
// each is checked by walking the parsed element tree directly.
//
// This file is the harness for that second half. Thirteen files were writing out
// the same five steps — parse, route a parse failure, check the root, build the
// finding sink, wrap the result — and one of the thirteen drifted off the
// contract limits.go states as a property of the whole package. ValidateOrderXML
// answered a parse error with a finding of its own rather than through the one
// shared path, so malformed XML, empty input and a perfectly well-formed
// CrossIndustryInvoice all came back as the single claim "the order XML is not a
// well-formed Cross Industry Order": false about the third document, silent
// about *where* the first one broke, and invisible to a caller filtering on
// the exported constant for a malformed file to tell a bad file from a bad
// invoice.
//
// The point of the harness is therefore not brevity but that there is now one
// path from bytes to findings for every one of these validators, and it goes
// through readFailure. A national file can no longer express the divergence,
// because it no longer contains the code that diverged.

// treeValidator is one format's tree-reading validator: the roots it accepts,
// what it says about a document whose root it does not accept, and its rule
// body. Every field is required.
type treeValidator struct {
	// source is the authority stamped on every finding this validator emits,
	// including the wrong-root one.
	source Source

	// rootRule and rootMsg are the finding for a well-formed document that is not
	// of this format. It is deliberately a finding and not the error a document
	// this package cannot read produces: such a document may be impeccable XML and
	// a valid invoice in some other format, so refusing to have read it would be
	// an accusation the parser never made. Each format names it in its own space
	// (FPA-root, FE-root, ZA-root, ...) so that a caller can tell "I was handed the
	// wrong document" apart from "this document is broken", which is the
	// distinction C5 collapsed. The EN 16931 half says the same thing with
	// RuleRoot.
	rootRule string
	rootMsg  string

	// accepts reports whether root is a document of this format. Formats
	// genuinely differ here and so keep their own predicates: several share UBL's
	// Invoice root and are told apart by a child only they have (ebInterface's
	// Biller, Svefaktura's UBL-1.0 SellerParty), and two accept a second root
	// (ZATCA and PINT also read a CreditNote).
	accepts func(root *ciiNode) bool

	// check is the rule body — everything this format has to say about a document
	// it accepts. It is reached only with a root that accepts returned true for,
	// so it never has to restate the root check, and it emits through add so the
	// Source is fixed at the point of emission.
	check func(root *ciiNode, add func(rule, msg string))
}

// validate is the whole body of the exported entry point: parse the document
// once, hand a parse failure to readFailure, refuse a root this format does not
// describe, then run the rule body.
//
// Every exit that produces a Report is wrapped in r.finish, so the invariant that
// a stopped run never returns an empty slice holds on all of them, and the single
// parse per exported call that makes maxNodes a property of the document rather
// than of the entry point is preserved: nothing below here reads the bytes again.
//
// Every such exit is also wrapped in newReport under this format's own Source, so
// the coverage claim does not depend on which one was taken: these validators
// check the mandatory structure and code lists rather than the whole XSD their
// authority publishes, and that is true of a document they refused as much as of
// one they read to the end. The one exit that produces no Report is a document
// that could not be read, where there is no rule set to have covered anything.
func (t treeValidator) validate(ctx context.Context, xmlData []byte) (Report, error) {
	r := newRun(ctx)
	root, err := parseCII(r, xmlData)
	if err != nil {
		// readFailure answers with the run's own RuleLimit trip when the parse was
		// stopped rather than defeated, and with an error when the document could
		// not be read. Neither is this format's to decide, which is the point.
		return readFailure(r, err, t.source)
	}
	return newReport(r.finish(t.checkTree(root)), t.source), nil
}

// checkTree is the half of validate that does not read bytes: refuse a root this
// format does not describe, otherwise run the rule body.
//
// It is separate so ValidateCIUS can route a document to this format's rules
// without parsing it a second time. That dispatcher has already built a tree,
// and re-reading the bytes to build another would charge one document's element
// budget twice — the failure TestNodeBudgetIsPerDocumentNotPerEntryPoint exists
// to prevent. Both callers therefore run the same body over the same root, and a
// format cannot say one thing through its own entry point and another through
// the dispatcher.
func (t treeValidator) checkTree(root *ciiNode) []Violation {
	if !t.accepts(root) {
		return []Violation{{Source: t.source, Rule: t.rootRule, Severity: SeverityFatal, Message: t.rootMsg}}
	}
	var out []Violation
	t.check(root, adder(&out, t.source))
	return out
}

// checkParsed runs this format's rules against a document another entry point
// has already parsed onto the EN 16931 model. The tree is kept alongside that
// model precisely so this is possible.
func (t treeValidator) checkParsed(p *parsed) []Violation {
	return t.checkTree(p.root)
}

// rootNamed accepts a document whose root element carries one of the given local
// names. parseCII keys on local names throughout, so no namespace appears here.
func rootNamed(names ...string) func(*ciiNode) bool {
	return func(root *ciiNode) bool {
		for _, n := range names {
			if root.name == n {
				return true
			}
		}
		return false
	}
}

// rootNamedWith accepts a root named name that carries the direct child child.
// It is the disambiguator for the formats that share UBL's Invoice root: an
// ebInterface invoice has a Biller, a Svefaktura has the UBL 1.0 SellerParty
// that UBL 2.1 renamed AccountingSupplierParty.
func rootNamedWith(name, child string) func(*ciiNode) bool {
	return func(root *ciiNode) bool {
		return root.name == name && root.child(child) != nil
	}
}
