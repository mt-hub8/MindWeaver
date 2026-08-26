package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Polish KSeF FA (Faktura ustrukturyzowana) format — the
// structured invoice exchanged through the Krajowy System e-Faktur. Like the
// other national formats it is XSD-validated rather than rule-validated, so this
// checks the mandatory structure and Polish code lists against the parsed tree.
//
// Rule identifiers are KS-* (this package's own). Not vendored: the KSeF sample
// instances (phax/phive-rules) are used only as the oracle.

// ksefRodzajFaktury is the RodzajFaktury invoice-kind code set: VAT (standard),
// KOR (correction), ZAL (advance), ROZ (settlement), UPR (simplified), and the
// correction variants KOR_ZAL / KOR_ROZ.
var ksefRodzajFaktury = map[string]bool{
	"VAT": true, "KOR": true, "ZAL": true, "ROZ": true, "UPR": true, "KOR_ZAL": true, "KOR_ROZ": true,
}

// IsKSeF reports whether the XML is a KSeF Faktura document.
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
func IsKSeF(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "Faktura" && d.hasNaglowek, nil
}

// ValidateKSeF validates a Polish KSeF FA document against its mandatory
// structure and Polish code lists.
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
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceKSeF),
// says what was not checked.
func ValidateKSeF(ctx context.Context, xmlData []byte) (Report, error) {
	return ksefValidator.validate(ctx, xmlData)
}

var ksefValidator = treeValidator{
	source:   SourceKSeF,
	rootRule: "KS-root",
	rootMsg:  "the document root shall be Faktura",
	accepts:  rootNamed("Faktura"),
	check:    checkKSeF,
}

func checkKSeF(root *ciiNode, add func(rule, msg string)) {
	// KS-header: the form code (Naglowek/KodFormularza) is mandatory.
	if strings.TrimSpace(root.str("Naglowek", "KodFormularza")) == "" {
		add("KS-header", "the header (Naglowek) shall contain a KodFormularza")
	}

	// KS-seller (Podmiot1): NIP, name and country are mandatory.
	p1 := root.child("Podmiot1").orNil()
	if strings.TrimSpace(p1.str("DaneIdentyfikacyjne", "NIP")) == "" {
		add("KS-seller-nip", "the Seller (Podmiot1) shall have a NIP")
	}
	if strings.TrimSpace(p1.str("DaneIdentyfikacyjne", "Nazwa")) == "" {
		add("KS-seller-name", "the Seller (Podmiot1) shall have a Nazwa")
	}
	if strings.TrimSpace(p1.str("Adres", "KodKraju")) == "" {
		add("KS-seller-country", "the Seller address (Podmiot1/Adres) shall have a KodKraju")
	}

	// KS-buyer (Podmiot2): present, and — where an address is given (it is
	// omitted for some public-body buyers) — the address carries a country.
	p2 := root.child("Podmiot2").orNil()
	if p2.name == "" {
		add("KS-buyer", "the invoice shall contain a Buyer (Podmiot2)")
	} else if p2.child("Adres") != nil && strings.TrimSpace(p2.str("Adres", "KodKraju")) == "" {
		add("KS-buyer-country", "a Buyer address (Podmiot2/Adres), when present, shall have a KodKraju")
	}

	// KS-invoice (Fa): currency, issue date, number and invoice kind.
	fa := root.child("Fa").orNil()
	if cur := strings.TrimSpace(fa.str("KodWaluty")); !en16931Currencies[cur] {
		add("KS-currency", fmt.Sprintf("the currency (KodWaluty=%q) shall be a valid ISO 4217 code", cur))
	}
	if strings.TrimSpace(fa.str("P_1")) == "" {
		add("KS-date", "the invoice shall contain an issue date (Fa/P_1)")
	}
	if strings.TrimSpace(fa.str("P_2")) == "" {
		add("KS-number", "the invoice shall contain a number (Fa/P_2)")
	}
	if rf := strings.TrimSpace(fa.str("RodzajFaktury")); !ksefRodzajFaktury[rf] {
		add("KS-type", fmt.Sprintf("the invoice kind (RodzajFaktury=%q) shall be a valid code (VAT, KOR, ZAL, ROZ, UPR, KOR_ZAL, KOR_ROZ)", rf))
	}
}
