package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Danish OIOUBL format (oioubl.info) — Denmark's UBL 2.0
// national profile (CustomizationID "OIOUBL-2.x"). OIOUBL has an extensive
// Schematron, but it is layered across many included rule sets; this validator
// checks the OIOUBL-mandatory structure directly against the parsed UBL tree —
// the profile, the core document terms, the seller and buyer electronic
// addresses (which OIOUBL requires) and the seller name.
//
// Rule identifiers are OIO-* (this package's own). Not vendored: the OIOUBL
// sample instances (phax/phive-rules) are used only as the oracle.

// IsOIOUBL reports whether the XML is an OIOUBL Invoice.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
//
// The Is* predicates are independent tests, not a partition. This one reads the
// Specification identifier of a root four national formats, seven CIUS and the
// EN 16931 UBL binding all share, so more than one can report true about the
// same document: "TR-OIOUBL-2.02" satisfies this predicate and IsTurkishInvoice
// both. Detect applies a documented precedence — this one wins that pair — and
// returns a single answer; route with it.
//
// The identifier test is the OIOUBL entry of specIDRules, the same one the
// routing reads, so the predicate and the route cannot disagree about which
// identifiers name OIOUBL.
func IsOIOUBL(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Invoice" && declaresSpecID(SourceOIOUBL, d.str("CustomizationID")), nil
}

// ValidateOIOUBL validates a Danish OIOUBL Invoice against its mandatory structure.
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
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceOIOUBL),
// says what was not checked.
func ValidateOIOUBL(ctx context.Context, xmlData []byte) (Report, error) {
	return oioublValidator.validate(ctx, xmlData)
}

var oioublValidator = treeValidator{
	source:   SourceOIOUBL,
	rootRule: "OIO-root",
	rootMsg:  "the document root shall be an Invoice",
	accepts:  rootNamed("Invoice"),
	check:    checkOIOUBL,
}

func checkOIOUBL(root *ciiNode, add func(rule, msg string)) {
	if !strings.Contains(root.str("CustomizationID"), "OIOUBL") {
		add("OIO-customization", "the CustomizationID shall declare an OIOUBL profile")
	}
	if strings.TrimSpace(root.str("ID")) == "" {
		add("OIO-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("OIO-date", "the invoice shall contain an IssueDate")
	}
	if strings.TrimSpace(root.str("InvoiceTypeCode")) == "" {
		add("OIO-type", "the invoice shall contain an InvoiceTypeCode")
	}
	if cur := strings.TrimSpace(root.str("DocumentCurrencyCode")); !en16931Currencies[cur] {
		add("OIO-currency", fmt.Sprintf("the DocumentCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// OIOUBL requires an electronic address (EndpointID) for both parties.
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()
	if strings.TrimSpace(seller.str("EndpointID")) == "" {
		add("OIO-seller-endpoint", "the Seller shall contain an electronic address (EndpointID)")
	}
	if strings.TrimSpace(buyer.str("EndpointID")) == "" {
		add("OIO-buyer-endpoint", "the Buyer shall contain an electronic address (EndpointID)")
	}
	if strings.TrimSpace(seller.str("PartyName", "Name")) == "" &&
		strings.TrimSpace(seller.str("PartyLegalEntity", "RegistrationName")) == "" {
		add("OIO-seller-name", "the Seller shall contain a party name")
	}
}
