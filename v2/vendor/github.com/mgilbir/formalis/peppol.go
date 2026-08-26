package formalis

import (
	"context"
)

// This file validates the OpenPEPPOL BIS Billing 3.0 CIUS on top of the EN 16931
// core. Peppol is the pan-European exchange profile; its rules (PEPPOL-EN16931-*
// and PEPPOL-COMMON-*) mandate the electronic addresses and business process,
// restrict identifiers and code lists, check the format of a participant
// identifier against the scheme it is declared under, and tie VAT exemption reason
// codes to their categories.
//
// peppol_rules.go holds the rule bodies and argues why most of them read the
// document tree rather than the syntax-neutral model, and why the two bindings are
// evaluated separately. peppol_codelists.go holds the code lists Peppol restricts
// EN 16931's to.
//
// Not vendored: the OpenPEPPOL Schematron, its per-rule unit tests and its example
// invoices are cloned by `make cius-oracles`. They are the oracle in both
// directions — the 9 example invoices for false positives, and the 102 per-rule
// test sets under rules/unit-UBL-PEPPOL and rules/unit-CII-PEPPOL, each of which
// declares which documents OpenPEPPOL considers valid and invalid against one
// named rule.

// peppolSpecID is the Specification identifier a Peppol BIS 3 invoice must carry.
const peppolSpecID = "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0"

// peppolVATEX maps a CEF VAT exemption reason code to the rule id and the VAT
// category it forces (PEPPOL-EN16931-P0104..P0111, published in the UBL binding
// only).
var peppolVATEX = map[string]struct {
	rule, category string
}{
	"VATEX-EU-G":  {"PEPPOL-EN16931-P0104", "G"},
	"VATEX-EU-O":  {"PEPPOL-EN16931-P0105", "O"},
	"VATEX-EU-IC": {"PEPPOL-EN16931-P0106", "K"},
	"VATEX-EU-AE": {"PEPPOL-EN16931-P0107", "AE"},
	"VATEX-EU-D":  {"PEPPOL-EN16931-P0108", "E"},
	"VATEX-EU-F":  {"PEPPOL-EN16931-P0109", "E"},
	"VATEX-EU-I":  {"PEPPOL-EN16931-P0110", "E"},
	"VATEX-EU-J":  {"PEPPOL-EN16931-P0111", "E"},
}

// ValidatePeppol validates an invoice XML against the OpenPEPPOL BIS Billing 3.0
// CIUS: the EN 16931 core plus the Peppol-specific rules. It accepts either syntax.
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
// The Report names the rule families neither rule set evaluates, which is now
// Coverage(SourceEN16931) alone: Coverage(SourcePeppol) is empty. Both rule sets in
// the two vendored OpenPEPPOL binding files are evaluated, each identifier in the
// binding that publishes it —
//
//   - the 59 PEPPOL-COMMON-* and PEPPOL-EN16931-* rules, and
//   - the 101 country-specific rules the same files publish under a comment
//     reading "National rules": DE-R-*, DK-R-*, GR-R-*/GR-S-*, IS-R-*, IT-R-*,
//     NL-R-*, NO-R-* and SE-R-*.
//
// 244 (identifier, binding) pairs in total. So a clean Peppol invoice reports
// Conformant() == true.
//
// The country rules are not an opt-in national profile: neither binding file
// declares a Schematron <phase>, buildconfig.xml's base configuration is the whole
// file, and each rule is gated inside itself on the supplier's country — and, for
// the domestic ones, the customer's. A French invoice matches none of their
// contexts. peppol_country_rules.go sets out the five different spellings of that
// gate, which are not interchangeable.
//
// The severity of each finding is OpenPEPPOL's published flag, so a document can
// fail an advisory rule and still be Conformant: the identifier-format warnings of
// PEPPOL-COMMON-R044..R047 and R052/R053, and eighteen of the country rules,
// including every SE-R-007..012 and the Greek GR-S-008-1 and GR-S-011.
func ValidatePeppol(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourcePeppol}, validatePeppol)
}

func validatePeppol(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	return append(out, validatePeppolRuleSet(r, p, false)...)
}
