package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Spanish Facturae format (facturae.gob.es). Like
// FatturaPA it is a national e-invoice XML, not an EN 16931 profile, and has no
// business-rule Schematron — it is validated against an XSD. This validator
// checks the mandatory structure and Spanish code lists against the parsed tree
// (parseCII reads the Facturae tree by local element name).
//
// Rule identifiers are FE-* (this package's own). Not vendored: the Facturae
// sample instances (phax/phive-rules) are used only as the oracle.

var (
	feModality       = map[string]bool{"I": true, "B": true}               // individual / batch
	feIssuerType     = map[string]bool{"EM": true, "RE": true, "TE": true} // emitter / receiver / third party
	fePersonType     = map[string]bool{"F": true, "J": true}               // física / jurídica
	feResidenceType  = map[string]bool{"R": true, "E": true, "U": true}    // resident / EU / non-EU
	feInvoiceDocType = map[string]bool{"FC": true, "FA": true, "AF": true} // complete / simplified / self-billed
	feInvoiceClass   = map[string]bool{"OO": true, "OR": true, "OC": true, "CO": true, "CR": true, "CC": true}
)

// IsFacturae reports whether the XML is a Facturae document.
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
func IsFacturae(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Facturae", nil
}

// ValidateFacturae validates a Spanish Facturae document against its mandatory
// structure and Spanish code lists.
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
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceFacturae),
// says what was not checked.
func ValidateFacturae(ctx context.Context, xmlData []byte) (Report, error) {
	return facturaeValidator.validate(ctx, xmlData)
}

var facturaeValidator = treeValidator{
	source:   SourceFacturae,
	rootRule: "FE-root",
	rootMsg:  "the document root shall be Facturae",
	accepts:  rootNamed("Facturae"),
	check:    checkFacturae,
}

func checkFacturae(root *ciiNode, add func(rule, msg string)) {
	// File header: schema version, modality, issuer type and batch currency.
	fh := root.child("FileHeader").orNil()
	if strings.TrimSpace(fh.str("SchemaVersion")) == "" {
		add("FE-header", "the FileHeader shall contain a SchemaVersion")
	}
	if m := strings.TrimSpace(fh.str("Modality")); !feModality[m] {
		add("FE-header", fmt.Sprintf("the Modality (%q) shall be I (individual) or B (batch)", m))
	}
	if it := strings.TrimSpace(fh.str("InvoiceIssuerType")); !feIssuerType[it] {
		add("FE-header", fmt.Sprintf("the InvoiceIssuerType (%q) shall be EM, RE or TE", it))
	}
	if cur := strings.TrimSpace(fh.str("Batch", "InvoiceCurrencyCode")); !en16931Currencies[cur] {
		add("FE-currency", fmt.Sprintf("the InvoiceCurrencyCode (%q) shall be a valid ISO 4217 code", cur))
	}

	// Parties.
	parties := root.child("Parties").orNil()
	validateFacturaeParty(parties.child("SellerParty").orNil(), "seller", "SellerParty", add)
	validateFacturaeParty(parties.child("BuyerParty").orNil(), "buyer", "BuyerParty", add)

	// Invoices: at least one, each with a header and issue date.
	invoices := root.child("Invoices").orNil().all("Invoice")
	if len(invoices) == 0 {
		add("FE-invoices", "the document shall contain at least one Invoice")
	}
	for _, inv := range invoices {
		h := inv.child("InvoiceHeader").orNil()
		if strings.TrimSpace(h.str("InvoiceNumber")) == "" {
			add("FE-invoice-number", "each Invoice shall have an InvoiceNumber")
		}
		if dt := strings.TrimSpace(h.str("InvoiceDocumentType")); !feInvoiceDocType[dt] {
			add("FE-invoice-type", fmt.Sprintf("the InvoiceDocumentType (%q) shall be FC, FA or AF", dt))
		}
		if ic := strings.TrimSpace(h.str("InvoiceClass")); !feInvoiceClass[ic] {
			add("FE-invoice-class", fmt.Sprintf("the InvoiceClass (%q) shall be a valid class code (OO, OR, OC, CO, CR, CC)", ic))
		}
		if strings.TrimSpace(inv.str("InvoiceIssueData", "IssueDate")) == "" {
			add("FE-invoice-date", "each Invoice shall have an IssueDate")
		}
	}
}

// validateFacturaeParty checks a Facturae party's tax identity, name and address.
func validateFacturaeParty(p *ciiNode, who, elem string, add func(rule, msg string)) {
	ti := p.child("TaxIdentification").orNil()
	if strings.TrimSpace(ti.str("TaxIdentificationNumber")) == "" {
		add("FE-"+who+"-id", fmt.Sprintf("the %s (%s) shall have a TaxIdentificationNumber", who, elem))
	}
	if pt := strings.TrimSpace(ti.str("PersonTypeCode")); !fePersonType[pt] {
		add("FE-"+who+"-id", fmt.Sprintf("the %s PersonTypeCode (%q) shall be F or J", who, pt))
	}
	if rt := strings.TrimSpace(ti.str("ResidenceTypeCode")); !feResidenceType[rt] {
		add("FE-"+who+"-id", fmt.Sprintf("the %s ResidenceTypeCode (%q) shall be R, E or U", who, rt))
	}
	// The name and address live under the LegalEntity or Individual sub-element.
	entity := p.child("LegalEntity").orNil()
	if entity.name == "" {
		entity = p.child("Individual").orNil()
	}
	// A name: a legal entity CorporateName, or an individual Name.
	if strings.TrimSpace(entity.str("CorporateName")) == "" && strings.TrimSpace(entity.str("Name")) == "" {
		add("FE-"+who+"-name", fmt.Sprintf("the %s (%s) shall have a CorporateName or a Name", who, elem))
	}
	// An address (in Spain or abroad) with the mandatory geographic terms.
	addr := entity.child("AddressInSpain").orNil()
	overseas := entity.child("OverseasAddress").orNil()
	if addr.name != "" {
		if strings.TrimSpace(addr.str("Address")) == "" || strings.TrimSpace(addr.str("PostCode")) == "" ||
			strings.TrimSpace(addr.str("Town")) == "" || strings.TrimSpace(addr.str("Province")) == "" ||
			strings.TrimSpace(addr.str("CountryCode")) == "" {
			add("FE-"+who+"-address", fmt.Sprintf("the %s AddressInSpain shall contain Address, PostCode, Town, Province and CountryCode", who))
		}
	} else if overseas.name != "" {
		if strings.TrimSpace(overseas.str("Address")) == "" || strings.TrimSpace(overseas.str("CountryCode")) == "" {
			add("FE-"+who+"-address", fmt.Sprintf("the %s OverseasAddress shall contain Address and CountryCode", who))
		}
	} else {
		add("FE-"+who+"-address", fmt.Sprintf("the %s (%s) shall have an AddressInSpain or OverseasAddress", who, elem))
	}
}
