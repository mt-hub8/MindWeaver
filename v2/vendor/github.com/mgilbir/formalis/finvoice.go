package formalis

import (
	"context"
	"strings"
)

// This file validates the Finnish Finvoice format (finvoice.info / Finance
// Finland) — Finland's national e-invoice XML. Like the other national formats it
// is XSD-validated rather than rule-validated, so this checks the mandatory
// structure directly against the parsed tree.
//
// Rule identifiers are FI-* (this package's own). Not vendored: the Finvoice
// sample instances (phax/phive-rules) are used only as the oracle.

// IsFinvoice reports whether the XML is a Finvoice document.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
//
// The Is* predicates are independent tests, not a partition: several of them key
// on a root element name that four national formats, seven CIUS and the EN 16931
// UBL binding all share, and more than one can report true about one document.
// This one keys on a root no other format claims, so nothing overlaps it today.
// Detect owns the precedence for the whole set and returns a single answer; route
// with it.
func IsFinvoice(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Finvoice", nil
}

// ValidateFinvoice validates a Finnish Finvoice document against its mandatory
// structure.
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
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceFinvoice),
// says what was not checked.
func ValidateFinvoice(ctx context.Context, xmlData []byte) (Report, error) {
	return finvoiceValidator.validate(ctx, xmlData)
}

var finvoiceValidator = treeValidator{
	source:   SourceFinvoice,
	rootRule: "FI-root",
	rootMsg:  "the document root shall be Finvoice",
	accepts:  rootNamed("Finvoice"),
	check:    checkFinvoice,
}

func checkFinvoice(root *ciiNode, add func(rule, msg string)) {
	// FI-seller: the seller party has an organisation name and a postal address.
	sp := root.child("SellerPartyDetails").orNil()
	if strings.TrimSpace(sp.str("SellerOrganisationName")) == "" {
		add("FI-seller-name", "the SellerPartyDetails shall contain a SellerOrganisationName")
	}
	if sp.child("SellerPostalAddressDetails") == nil {
		add("FI-seller-address", "the SellerPartyDetails shall contain a SellerPostalAddressDetails")
	}

	// FI-buyer: the buyer party has an organisation (or person) name.
	bp := root.child("BuyerPartyDetails").orNil()
	if strings.TrimSpace(bp.str("BuyerOrganisationName")) == "" &&
		strings.TrimSpace(root.str("InvoiceRecipientPartyDetails", "InvoiceRecipientOrganisationName")) == "" {
		add("FI-buyer-name", "the BuyerPartyDetails shall contain a BuyerOrganisationName")
	}

	// FI-invoice: the invoice details carry a type code, number and date.
	id := root.child("InvoiceDetails").orNil()
	if strings.TrimSpace(id.str("InvoiceTypeCode")) == "" {
		add("FI-type", "the InvoiceDetails shall contain an InvoiceTypeCode")
	}
	if strings.TrimSpace(id.str("InvoiceNumber")) == "" {
		add("FI-number", "the InvoiceDetails shall contain an InvoiceNumber")
	}
	if strings.TrimSpace(id.str("InvoiceDate")) == "" {
		add("FI-date", "the InvoiceDetails shall contain an InvoiceDate")
	}
}
