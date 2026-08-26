package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Swedish Svefaktura format (SFTI Svefaktura 1.0) — the
// legacy Swedish e-invoice, a UBL 1.0 profile. It is XSD-validated rather than
// rule-validated, so this checks the mandatory structure against the parsed tree.
// The root element is "Invoice" (as in UBL 2.1), so it is disambiguated by the
// UBL-1.0-style SellerParty child (UBL 2.1 uses AccountingSupplierParty).
//
// Rule identifiers are SV-* (this package's own). Not vendored: the Svefaktura
// sample instances (phax/phive-rules) are used only as the oracle.

// IsSvefaktura reports whether the XML is an SFTI Svefaktura invoice.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
//
// The Is* predicates are independent tests, not a partition. This one keys on a
// distinguishing child of a root four national formats, seven CIUS and the
// EN 16931 UBL binding all share — the weakest evidence of the twelve, since no
// other format forbids that child — so more than one can report true about the
// same document: <Invoice><Biller/><SellerParty/></Invoice> satisfies this
// predicate and IsEbInterface both. Detect applies a documented precedence —
// IsEbInterface wins that pair — and returns a single answer; route with it.
func IsSvefaktura(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Invoice" && d.hasSellerParty, nil
}

// ValidateSvefaktura validates a Swedish Svefaktura document against its
// mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. It is a
// statement about the file rather than about the document, and the Report
// returned with it is the zero Report, so a caller who ignores the error cannot
// read the value as clean. See ErrMalformedXML.
//
// This validator checks the mandatory structure and code lists rather than the
// whole schema its authority publishes, so the Report is never Conformant even
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceSvefaktura),
// says what was not checked.
func ValidateSvefaktura(ctx context.Context, xmlData []byte) (Report, error) {
	return svefakturaValidator.validate(ctx, xmlData)
}

var svefakturaValidator = treeValidator{
	source:   SourceSvefaktura,
	rootRule: "SV-root",
	rootMsg:  "the document root shall be a Svefaktura Invoice with a SellerParty",
	accepts:  rootNamedWith("Invoice", "SellerParty"),
	check:    checkSvefaktura,
}

func checkSvefaktura(root *ciiNode, add func(rule, msg string)) {
	if strings.TrimSpace(root.str("ID")) == "" {
		add("SV-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("SV-date", "the invoice shall contain an IssueDate")
	}
	if cur := strings.TrimSpace(root.str("InvoiceCurrencyCode")); !en16931Currencies[cur] {
		add("SV-currency", fmt.Sprintf("the InvoiceCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}
	if strings.TrimSpace(root.str("SellerParty", "Party", "PartyName", "Name")) == "" {
		add("SV-seller", "the SellerParty shall contain a party name")
	}
	if strings.TrimSpace(root.str("BuyerParty", "Party", "PartyName", "Name")) == "" {
		add("SV-buyer", "the BuyerParty shall contain a party name")
	}
}
