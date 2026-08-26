package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Saudi ZATCA e-invoice (Fatoora) format. ZATCA invoices
// are UBL 2.1 Invoice/CreditNote documents with Kingdom-of-Saudi-Arabia specific
// requirements, exchanged through the ZATCA reporting/clearance platform. There
// is no business-rule Schematron (XSD + platform checks), so this validates the
// KSA-mandatory structure directly against the parsed UBL tree — the UUID, the
// mandatory ICV (invoice counter), PIH (previous invoice hash) and QR additional
// document references, the seller VAT registration, and the core document terms.
//
// Rule identifiers are ZA-* (this package's own). Not vendored: the ZATCA sample
// instances (phax/phive-rules) are used only as the oracle.

// zatcaDocRef reports whether the invoice carries an AdditionalDocumentReference
// with the given cbc:ID (e.g. "ICV", "PIH", "QR").
func zatcaDocRef(root *ciiNode, id string) bool {
	for _, r := range root.findAll("AdditionalDocumentReference") {
		if strings.TrimSpace(r.str("ID")) == id {
			return true
		}
	}
	return false
}

// IsZATCA reports whether the XML is a ZATCA (Fatoora) UBL invoice, identified by
// its reporting/clearance ProfileID.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
//
// The Is* predicates are independent tests, not a partition. This one reads a
// ProfileID and a document reference rather than the Specification identifier,
// on a root four national formats, seven CIUS and the EN 16931 UBL binding all
// share, so more than one can report true about the same document: an invoice
// declaring "urn:peppol:pint:x" and ProfileID "reporting:1.0" satisfies this
// predicate and IsPINT both. Detect applies a documented precedence — BT-24 is a
// claim about the rule set and a profile identifier is not, so that document is
// PINT — and returns a single answer; route with it.
func IsZATCA(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	if d.root != "Invoice" && d.root != "CreditNote" {
		return false, nil
	}
	// d.icvDocRef is the scan's equivalent of zatcaDocRef(root, "ICV"), which
	// the validator still uses against the parsed tree.
	return strings.Contains(strings.ToLower(d.str("ProfileID")), "reporting") || d.icvDocRef, nil
}

// ValidateZATCA validates a Saudi ZATCA UBL invoice against its KSA-mandatory
// structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice or credit note.
//
// The error is for input that could not be read at all — XML that is not
// well-formed, or a character encoding this package does not implement. It is a
// statement about the file rather than about the document, and the Report
// returned with it is the zero Report, so a caller who ignores the error cannot
// read the value as clean. See ErrMalformedXML.
//
// This validator checks the mandatory structure and code lists rather than the
// whole schema its authority publishes, so the Report is never Conformant even
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceZATCA),
// says what was not checked.
func ValidateZATCA(ctx context.Context, xmlData []byte) (Report, error) {
	return zatcaValidator.validate(ctx, xmlData)
}

var zatcaValidator = treeValidator{
	source:   SourceZATCA,
	rootRule: "ZA-root",
	rootMsg:  "the document root shall be a UBL Invoice or CreditNote",
	accepts:  rootNamed("Invoice", "CreditNote"),
	check:    checkZATCA,
}

func checkZATCA(root *ciiNode, add func(rule, msg string)) {
	// ZA-number/date/uuid: the document id, issue date and UUID are mandatory.
	if strings.TrimSpace(root.str("ID")) == "" {
		add("ZA-number", "the invoice shall contain an ID")
	}
	if strings.TrimSpace(root.str("IssueDate")) == "" {
		add("ZA-date", "the invoice shall contain an IssueDate")
	}
	if strings.TrimSpace(root.str("UUID")) == "" {
		add("ZA-uuid", "the invoice shall contain a UUID")
	}

	// ZA-type/currency: the type code and currency are mandatory.
	if strings.TrimSpace(root.str("InvoiceTypeCode")) == "" && strings.TrimSpace(root.str("CreditNoteTypeCode")) == "" {
		add("ZA-type", "the invoice shall contain an InvoiceTypeCode")
	}
	if cur := strings.TrimSpace(root.str("DocumentCurrencyCode")); !en16931Currencies[cur] {
		add("ZA-currency", fmt.Sprintf("the DocumentCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// ZA-icv/pih/qr: the KSA additional document references are mandatory — the
	// invoice counter value (ICV), the previous invoice hash (PIH) and the QR code.
	if !zatcaDocRef(root, "ICV") {
		add("ZA-icv", "the invoice shall contain an ICV (invoice counter value) AdditionalDocumentReference")
	}
	if !zatcaDocRef(root, "PIH") {
		add("ZA-pih", "the invoice shall contain a PIH (previous invoice hash) AdditionalDocumentReference")
	}
	if !zatcaDocRef(root, "QR") {
		add("ZA-qr", "the invoice shall contain a QR AdditionalDocumentReference")
	}

	// ZA-seller-vat: the seller VAT registration (PartyTaxScheme/CompanyID) is
	// mandatory.
	seller := root.child("AccountingSupplierParty", "Party").orNil()
	if strings.TrimSpace(seller.str("PartyTaxScheme", "CompanyID")) == "" {
		add("ZA-seller-vat", "the Seller shall contain a VAT registration (PartyTaxScheme/CompanyID)")
	}
}
