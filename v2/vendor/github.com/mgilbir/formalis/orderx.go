package formalis

import (
	"context"
	"fmt"
)

// This file validates the embedded Cross Industry Order document of an Order-X
// (a.k.a. ZUGFeRD Order) — the order-document sibling of Factur-X. Order-X
// business rules differ from EN 16931 (which is invoice-specific), so this checks
// the order's structure and mandatory head terms, reusing the shared CII parser.
// The PDF container around it is validated in pdf0, the sibling module the
// package documentation introduces; nothing here depends on it.
//
// # Why the identifiers read ORDER-01 and not BR-O-01
//
// These five checks are this package's own, not quotations from a published
// Order-X rule set, so they are numbered in this package's own namespace the way
// FPA-*, FE-*, EB-* and ZA-* are. They used to be called BR-O-01…BR-O-05, which
// was a name CEN had already taken: BR-O-01 … BR-O-14 are EN 16931's rules for
// the "Not subject to VAT" (O) category, and the conformance corpus this package
// is tested against ships CEN's own unit tests for them under exactly those
// names (testdata/en16931-artefacts/test/Invoice-unit-UBL/BR-O-01.xml …).
// The rule engine emits them from validateVATCategories and
// validateVATIdentifiers, so one string meant two unrelated things depending on
// which validator produced it, and a caller aggregating a mailbox by rule
// identifier merged the two. Violation.Source now scopes every finding, but a
// name minted inside another authority's numbering is worth correcting on its
// own account: a reader who sees BR-O-03 in a log has no reason to check the
// scope before believing it is the CEN rule.
//
// If Order-X should later publish business rules under identifiers of its own,
// quote those and retire these.
//
// # Why an unreadable document is an error and the wrong root is ORDER-root
//
// This validator used to answer every unreadable input with one finding of its
// own, "order-xml: the order XML is not a well-formed Cross Industry Order".
// That was three answers collapsed into one, and one of the three was false: a
// CrossIndustryInvoice handed to this entry point is perfectly well-formed, and
// saying otherwise accuses a document the parser never complained about. The
// other two lost information rather than inventing it — the decoder's message is
// the only thing that says *where* the XML broke, and a caller filtering on the
// exported constant for a malformed file to separate "bad file" from "bad
// invoice" saw nothing from this validator at all, though limits.go states that
// every exported validator routes its parse errors through one shared path.
//
// The three cases are now three answers: a malformed or empty document is an
// error wrapping ErrMalformedXML and carrying the decoder's own text, a
// well-formed document of another root is ORDER-root under SourceOrderX (the
// convention FPA-root, FE-root,
// ZA-root … already follow), and a run that stopped early is RuleLimit and
// nothing else. Holding to that is no longer this file's job: treeValidator owns
// it for every tree-reading validator in the package.

// orderXTypeCodes is the order document type code set (UNTDID 1001): order (220),
// order change (230), order response (231).
var orderXTypeCodes = map[string]bool{"220": true, "230": true, "231": true}

// ValidateOrderXML checks an embedded Cross Industry Order's structure and
// mandatory head business terms. An Order-X is an order, not an invoice: none
// of the EN 16931 invoice rules apply to it, and the five ORDER-* checks here
// are this package's own.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean order.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. It is a
// statement about the file rather than about the document, and the Report
// returned with it is the zero Report, so a caller who ignores the error cannot
// read the value as clean. See ErrMalformedXML.
//
// This validator checks the mandatory structure and code lists rather than the
// whole schema its authority publishes, so the Report is never Conformant even
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceOrderX),
// says what was not checked.
func ValidateOrderXML(ctx context.Context, xmlData []byte) (Report, error) {
	return orderXValidator.validate(ctx, xmlData)
}

var orderXValidator = treeValidator{
	source:   SourceOrderX,
	rootRule: "ORDER-root",
	rootMsg:  "the document root shall be a Cross Industry Order (SCRDMCCBDACIOMessageStructure)",
	accepts:  rootNamed("SCRDMCCBDACIOMessageStructure"),
	check:    checkOrderX,
}

func checkOrderX(root *ciiNode, add func(rule, msg string)) {
	doc := root.child("ExchangedDocument").orNil()
	agr := root.child("SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement").orNil()

	if doc.str("ID") == "" {
		add("ORDER-01", "an Order shall have an order number (ExchangedDocument/ID)")
	}
	if doc.str("IssueDateTime", "DateTimeString") == "" {
		add("ORDER-02", "an Order shall have an issue date (ExchangedDocument/IssueDateTime)")
	}
	if tc := doc.str("TypeCode"); tc == "" {
		add("ORDER-03", "an Order shall have a document type code (ExchangedDocument/TypeCode)")
	} else if !orderXTypeCodes[tc] {
		add("ORDER-03", fmt.Sprintf("order type code %q is not a permitted UNTDID 1001 order value (220/230/231)", tc))
	}
	if agr.str("BuyerTradeParty", "Name") == "" {
		add("ORDER-04", "an Order shall contain the Buyer name")
	}
	if agr.str("SellerTradeParty", "Name") == "" {
		add("ORDER-05", "an Order shall contain the Seller name")
	}
}
