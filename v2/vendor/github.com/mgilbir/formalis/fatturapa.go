package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Italian FatturaPA / FatturaElettronica format. Unlike
// the CIUS in this package, FatturaPA is not an EN 16931 profile — it is Italy's
// own national e-invoice XML (the format exchanged through the Sistema di
// Interscambio, SdI). It has no EN 16931 business-rule Schematron; the SdI
// validates it against an XSD plus a set of consistency checks. This validator
// therefore checks the mandatory structure and the Italian code lists directly
// against the parsed XML tree (parseCII is namespace-agnostic, so it reads the
// FatturaElettronica tree by local element name).
//
// Rule identifiers are FPA-* (this package's own), since FatturaPA has no public
// rule-id scheme; the messages reference the SdI terms.
//
// Not vendored: the FatturaPA sample instances (phax/phive-rules) are used only
// as the oracle.

// fpaFormats is the set of FormatoTrasmissione values (and the matching versione
// attribute): FPA12 for public administration, FPR12 for private, and the
// superseded SDI10/SDI11.
var fpaFormats = map[string]bool{"FPA12": true, "FPR12": true, "SDI11": true, "SDI10": true}

// fpaTipoDocumento is the TD** document-type code set (TD01 … TD28).
var fpaTipoDocumento = buildRange("TD", 1, 28)

// fpaRegimeFiscale is the RF** seller tax-regime code set (RF01 … RF19).
var fpaRegimeFiscale = buildRange("RF", 1, 19)

func buildRange(prefix string, lo, hi int) map[string]bool {
	m := make(map[string]bool, hi-lo+1)
	for i := lo; i <= hi; i++ {
		m[fmt.Sprintf("%s%02d", prefix, i)] = true
	}
	return m
}

// IsFatturaPA reports whether the XML is a FatturaElettronica document.
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
func IsFatturaPA(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "FatturaElettronica", nil
}

// ValidateFatturaPA validates an Italian FatturaPA / FatturaElettronica document
// against its mandatory structure and Italian code lists.
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
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceFatturaPA),
// says what was not checked.
func ValidateFatturaPA(ctx context.Context, xmlData []byte) (Report, error) {
	return fatturaPAValidator.validate(ctx, xmlData)
}

var fatturaPAValidator = treeValidator{
	source:   SourceFatturaPA,
	rootRule: "FPA-root",
	rootMsg:  "the document root shall be FatturaElettronica",
	accepts:  rootNamed("FatturaElettronica"),
	check:    checkFatturaPA,
}

func checkFatturaPA(root *ciiNode, add func(rule, msg string)) {
	// FPA-format: FormatoTrasmissione shall be a valid value and match the
	// document's versione attribute.
	hdr := root.child("FatturaElettronicaHeader").orNil()
	tx := hdr.child("DatiTrasmissione").orNil()
	format := strings.TrimSpace(tx.str("FormatoTrasmissione"))
	if !fpaFormats[format] {
		add("FPA-format", fmt.Sprintf("the FormatoTrasmissione (%q) shall be one of FPA12, FPR12", format))
	}
	if v := strings.TrimSpace(root.attr("versione")); v != "" && format != "" && v != format {
		add("FPA-format", fmt.Sprintf("the versione attribute (%q) shall match the FormatoTrasmissione (%q)", v, format))
	}

	// FPA-transmitter: the transmitter identity (IdPaese + IdCodice) is mandatory.
	if tx.str("IdTrasmittente", "IdPaese") == "" || tx.str("IdTrasmittente", "IdCodice") == "" {
		add("FPA-transmitter", "the transmitter identifier (IdTrasmittente/IdPaese and IdCodice) shall be present")
	}
	// FPA-destination: the recipient code (CodiceDestinatario) is mandatory.
	if strings.TrimSpace(tx.str("CodiceDestinatario")) == "" {
		add("FPA-destination", "the recipient code (CodiceDestinatario) shall be present")
	}

	// Seller (CedentePrestatore) and Buyer (CessionarioCommittente).
	validateFPAParty(hdr.child("CedentePrestatore").orNil(), "seller", "CedentePrestatore", true, add)
	validateFPAParty(hdr.child("CessionarioCommittente").orNil(), "buyer", "CessionarioCommittente", false, add)

	// Each FatturaElettronicaBody: general data, lines and VAT summary.
	bodies := root.all("FatturaElettronicaBody")
	if len(bodies) == 0 {
		add("FPA-body", "the invoice shall contain at least one FatturaElettronicaBody")
	}
	for _, b := range bodies {
		dg := b.child("DatiGenerali", "DatiGeneraliDocumento").orNil()
		if td := strings.TrimSpace(dg.str("TipoDocumento")); !fpaTipoDocumento[td] {
			add("FPA-doctype", fmt.Sprintf("the document type (TipoDocumento=%q) shall be a valid TD** code (TD01-TD28)", td))
		}
		if cur := strings.TrimSpace(dg.str("Divisa")); !en16931Currencies[cur] {
			add("FPA-currency", fmt.Sprintf("the currency (Divisa=%q) shall be a valid ISO 4217 code", cur))
		}
		if strings.TrimSpace(dg.str("Data")) == "" {
			add("FPA-date", "the document date (Data) shall be present")
		}
		if strings.TrimSpace(dg.str("Numero")) == "" {
			add("FPA-number", "the document number (Numero) shall be present")
		}
		bs := b.child("DatiBeniServizi").orNil()
		if len(bs.all("DettaglioLinee")) == 0 {
			add("FPA-lines", "the invoice body shall contain at least one line (DettaglioLinee)")
		}
		if len(bs.all("DatiRiepilogo")) == 0 {
			add("FPA-summary", "the invoice body shall contain at least one VAT summary (DatiRiepilogo)")
		}
	}
}

// validateFPAParty checks a FatturaPA party's tax identity, name and address.
// seller parties additionally require a tax regime (RegimeFiscale).
func validateFPAParty(p *ciiNode, who, elem string, seller bool, add func(rule, msg string)) {
	da := p.child("DatiAnagrafici").orNil()
	// A tax identity is required: an IdFiscaleIVA (VAT number) or a CodiceFiscale.
	hasVAT := da.str("IdFiscaleIVA", "IdPaese") != "" && da.str("IdFiscaleIVA", "IdCodice") != ""
	if !hasVAT && strings.TrimSpace(da.str("CodiceFiscale")) == "" {
		add("FPA-"+who+"-id", fmt.Sprintf("the %s (%s) shall have an IdFiscaleIVA or a CodiceFiscale", who, elem))
	}
	// A name: Denominazione, or Nome and Cognome.
	an := da.child("Anagrafica").orNil()
	if strings.TrimSpace(an.str("Denominazione")) == "" &&
		(strings.TrimSpace(an.str("Nome")) == "" || strings.TrimSpace(an.str("Cognome")) == "") {
		add("FPA-"+who+"-name", fmt.Sprintf("the %s (%s) shall have a Denominazione or a Nome and Cognome", who, elem))
	}
	if seller {
		if rf := strings.TrimSpace(da.str("RegimeFiscale")); !fpaRegimeFiscale[rf] {
			add("FPA-seller-regime", fmt.Sprintf("the seller tax regime (RegimeFiscale=%q) shall be a valid RF** code (RF01-RF19)", rf))
		}
	}
	// A postal address (Sede): street, post code, town and country.
	sede := p.child("Sede").orNil()
	if strings.TrimSpace(sede.str("Indirizzo")) == "" || strings.TrimSpace(sede.str("CAP")) == "" ||
		strings.TrimSpace(sede.str("Comune")) == "" || strings.TrimSpace(sede.str("Nazione")) == "" {
		add("FPA-"+who+"-address", fmt.Sprintf("the %s address (%s/Sede) shall contain Indirizzo, CAP, Comune and Nazione", who, elem))
	}
}
