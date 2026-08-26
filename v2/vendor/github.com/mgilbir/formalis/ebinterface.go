package formalis

import (
	"context"
	"strings"
)

// This file validates the Austrian ebInterface format (ebinterface.at) — the
// Austrian national e-invoice XML, across its schema versions (3.x … 6.x). Like
// the other national formats it is XSD-validated rather than rule-validated, so
// this checks the mandatory structure directly against the parsed tree. The
// element set below (invoice number and date, biller and recipient with a VAT
// identifier and an address name) is common to every ebInterface version.
//
// Rule identifiers are EB-* (this package's own). Not vendored: the ebInterface
// sample instances (phax/phive-rules) are used only as the oracle.

// IsEbInterface reports whether the XML is an ebInterface document. The root
// element is "Invoice" (as in UBL), so it is disambiguated by the ebInterface-
// specific Biller element.
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
// predicate and IsSvefaktura both. Detect applies a documented precedence — this
// one wins that pair — and returns a single answer; route with it.
func IsEbInterface(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Invoice" && d.hasBiller, nil
}

// ValidateEbInterface validates an Austrian ebInterface document against its
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
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceEbInterface),
// says what was not checked.
func ValidateEbInterface(ctx context.Context, xmlData []byte) (Report, error) {
	return ebInterfaceValidator.validate(ctx, xmlData)
}

var ebInterfaceValidator = treeValidator{
	source:   SourceEbInterface,
	rootRule: "EB-root",
	rootMsg:  "the document root shall be an ebInterface Invoice with a Biller",
	accepts:  rootNamedWith("Invoice", "Biller"),
	check:    checkEbInterface,
}

func checkEbInterface(root *ciiNode, add func(rule, msg string)) {
	// EB-number/EB-date: the invoice number and date elements are mandatory. The
	// number is checked for presence only (the ebInterface XSD, and thus the
	// official validation, accepts an empty element).
	if root.child("InvoiceNumber") == nil {
		add("EB-number", "the invoice shall contain an InvoiceNumber")
	}
	if strings.TrimSpace(root.str("InvoiceDate")) == "" {
		add("EB-date", "the invoice shall contain an InvoiceDate")
	}

	// EB-biller: the Biller has a VAT identifier and an address with a name.
	biller := root.child("Biller").orNil()
	if strings.TrimSpace(biller.str("VATIdentificationNumber")) == "" {
		add("EB-biller-vat", "the Biller shall contain a VATIdentificationNumber")
	}
	if strings.TrimSpace(biller.str("Address", "Name")) == "" {
		add("EB-biller-name", "the Biller address shall contain a Name")
	}

	// EB-recipient: the InvoiceRecipient is present with an address name.
	rec := root.child("InvoiceRecipient").orNil()
	if rec.name == "" {
		add("EB-recipient", "the invoice shall contain an InvoiceRecipient")
	} else if strings.TrimSpace(rec.str("Address", "Name")) == "" {
		add("EB-recipient-name", "the InvoiceRecipient address shall contain a Name")
	}
}
