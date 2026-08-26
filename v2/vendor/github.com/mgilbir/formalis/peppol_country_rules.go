package formalis

import (
	"math"
	"regexp"
	"strings"
)

// The country-specific half of the OpenPEPPOL rule set: the 101 identifiers
// PEPPOL-EN16931-UBL.sch and -CII.sch publish under the comment "National rules",
// alongside the 59 PEPPOL-* rules peppol_rules.go evaluates.
//
// # Why they are evaluated here at all
//
// The question is whether a plain ValidatePeppol call should run them, and the
// artefacts answer it three times over:
//
//   - Neither binding file declares a single <phase>. ISO Schematron activates
//     every pattern when no phase is selected, so there is no configuration in
//     which a reference validator runs the PEPPOL-* patterns and skips these. The
//     one <pattern> that carries an id at all — id="german-rules" — is named for a
//     reader, not selected by anything: no <phase>/<active> element refers to it.
//   - buildconfig.xml's peppolbis-en16931-base-3.0-ubl and -cii configurations are
//     `<file path="sch/PEPPOL-EN16931-UBL.sch"/>` with no phase attribute, and the
//     profile-01 configurations inherit them whole. The base configuration is the
//     file.
//   - Every rule is gated inside itself, on the supplier's country and — for the
//     domestic-only ones — the customer's too. That is what makes running them
//     unconditionally correct rather than reckless: the gate is OpenPEPPOL's, and
//     a French invoice matches no country rule's context.
//
// So these are not an opt-in national profile layered on top of Peppol BIS. They
// are part of the base rule set, and this package's not evaluating them was the
// gap Coverage(SourcePeppol) recorded.
//
// # What "gated inside itself" means, precisely
//
// It means five different things, and getting them interchangeable would be the
// same defect as evaluating a UBL rule on a CII document:
//
//	DE  $supplierCountryIsDE and $customerCountryIsDE — both postal addresses (BT-40/BT-55)
//	DK  $DKSupplierCountry / $DKCustomerCountry — both postal addresses, some rules domestic-only
//	GR  $isGreekSender / $isGreekSenderandReceiver — $supplierCountry, which prefers the VAT prefix
//	IS  $SupplierCountry / $CustomerCountry — both postal addresses
//	IT  $supplierCountry = 'IT' — the VAT prefix, then the tax representative's, then the address
//	NL  $supplierCountryIsNL / $customerCountryIsNL / $taxRepresentativeCountryIsNL — postal addresses
//	NO  $supplierCountry = 'NO' — the VAT prefix first, as IT
//	SE  no variable at all: each rule's context carries the country test inline
//
// The difference is load-bearing. A Norwegian seller whose postal address says SE
// but whose VAT identifier begins NO answers to the Norwegian rules and not the
// Swedish ones, because $supplierCountry reads the VAT prefix before the address
// and the Swedish rules read the address. peppolUBLSupplierCountry and
// peppolUBLPostalCountry are therefore two functions and not one.
//
// # Bindings
//
// The UBL file publishes all 101; the CII file publishes 41 of them — DK (13 of
// 14: not DK-R-017), IT, NL, NO and SE — with the same flags and different XPaths.
// DE, GR, IS and DK-R-017 are UBL-only, so they must not fire on a CII document:
// peppolCountryRules records the bindings and peppolEval.has is the gate, the same
// way C32's eight P0104..P0111 rules were stopped from firing on CII.
//
// KoSIT imports none of them — rule-list.xml whitelists 21 PEPPOL-EN16931-*
// identifiers and no country rule — so peppolXRImports keeps the whole family off
// the XRechnung path, and validatePeppolRuleSet does not even walk for them there.
// That matters for the German family in particular: OpenPEPPOL's DE-R-NNN is a
// re-publication of KoSIT's BR-DE-NNN, so an XRechnung validation reporting both
// would report every German defect twice under two authorities' identifiers.

// peppolCountryRules is every country-specific identifier the vendored OpenPEPPOL
// Schematron publishes, with the bindings that publish it and the flag it carries.
//
// It is peppolRules' counterpart for the other rule set in the same two files, and
// the same guards read it: TestPeppolCountryRuleTableMatchesTheSchematron compares
// it against an XML decoder's reading of both files in both directions, and
// TestEveryPublishedPeppolRuleHasBothVerdicts requires a document that trips each
// entry and one that does not.
var peppolCountryRules = map[string]peppolRule{

	// Germany — the german-rules pattern, UBL only. 24 fatal, 6 advisory.
	"DE-R-001":   {peppolUBL, SeverityFatal},
	"DE-R-002":   {peppolUBL, SeverityFatal},
	"DE-R-003":   {peppolUBL, SeverityFatal},
	"DE-R-004":   {peppolUBL, SeverityFatal},
	"DE-R-005":   {peppolUBL, SeverityFatal},
	"DE-R-006":   {peppolUBL, SeverityFatal},
	"DE-R-007":   {peppolUBL, SeverityFatal},
	"DE-R-008":   {peppolUBL, SeverityFatal},
	"DE-R-009":   {peppolUBL, SeverityFatal},
	"DE-R-010":   {peppolUBL, SeverityFatal},
	"DE-R-011":   {peppolUBL, SeverityFatal},
	"DE-R-014":   {peppolUBL, SeverityFatal},
	"DE-R-015":   {peppolUBL, SeverityFatal},
	"DE-R-016":   {peppolUBL, SeverityFatal},
	"DE-R-017":   {peppolUBL, SeverityWarning},
	"DE-R-018":   {peppolUBL, SeverityFatal},
	"DE-R-019":   {peppolUBL, SeverityWarning},
	"DE-R-020":   {peppolUBL, SeverityWarning},
	"DE-R-022":   {peppolUBL, SeverityFatal},
	"DE-R-023-1": {peppolUBL, SeverityFatal},
	"DE-R-023-2": {peppolUBL, SeverityFatal},
	"DE-R-024-1": {peppolUBL, SeverityFatal},
	"DE-R-024-2": {peppolUBL, SeverityFatal},
	"DE-R-025-1": {peppolUBL, SeverityFatal},
	"DE-R-025-2": {peppolUBL, SeverityFatal},
	"DE-R-026":   {peppolUBL, SeverityWarning},
	"DE-R-027":   {peppolUBL, SeverityWarning},
	"DE-R-028":   {peppolUBL, SeverityWarning},
	"DE-R-030":   {peppolUBL, SeverityFatal},
	"DE-R-031":   {peppolUBL, SeverityFatal},

	// Denmark — 12 fatal, 2 advisory. DK-R-017 is the one the CII file omits.
	"DK-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-003": {peppolUBL | peppolCII, SeverityWarning},
	"DK-R-004": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-005": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-006": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-007": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-008": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-009": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-010": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-011": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-013": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-014": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-016": {peppolUBL | peppolCII, SeverityFatal},
	"DK-R-017": {peppolUBL, SeverityWarning},

	// The Netherlands — 9 fatal, both bindings. Distinct from the BR-NL-* of
	// SourceNLCIUS: this is OpenPEPPOL's Dutch rule set, not NLCIUS's.
	"NL-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-004": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-005": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-006": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-007": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-008": {peppolUBL | peppolCII, SeverityFatal},
	"NL-R-009": {peppolUBL | peppolCII, SeverityFatal},

	// Greece — UBL only. 17 fatal, 2 advisory, and the only family whose
	// identifiers do not all fit XX-R-NNN: GR-R-001 and GR-R-004 are split into
	// numbered assertions, GR-R-008 into -2 and -3 with its first assertion
	// published as GR-S-008-1, and GR-S-011 has no GR-R-011 counterpart.
	"GR-R-001-1": {peppolUBL, SeverityFatal},
	"GR-R-001-2": {peppolUBL, SeverityFatal},
	"GR-R-001-3": {peppolUBL, SeverityFatal},
	"GR-R-001-4": {peppolUBL, SeverityFatal},
	"GR-R-001-5": {peppolUBL, SeverityFatal},
	"GR-R-001-6": {peppolUBL, SeverityFatal},
	"GR-R-001-7": {peppolUBL, SeverityFatal},
	"GR-R-002":   {peppolUBL, SeverityFatal},
	"GR-R-003":   {peppolUBL, SeverityFatal},
	"GR-R-004-1": {peppolUBL, SeverityFatal},
	"GR-R-004-2": {peppolUBL, SeverityFatal},
	"GR-R-005":   {peppolUBL, SeverityFatal},
	"GR-R-006":   {peppolUBL, SeverityFatal},
	"GR-R-008-2": {peppolUBL, SeverityFatal},
	"GR-R-008-3": {peppolUBL, SeverityFatal},
	"GR-R-009":   {peppolUBL, SeverityFatal},
	"GR-R-010":   {peppolUBL, SeverityFatal},
	"GR-S-008-1": {peppolUBL, SeverityWarning},
	"GR-S-011":   {peppolUBL, SeverityWarning},

	// Iceland — UBL only. 9 fatal, 1 advisory.
	"IS-R-001": {peppolUBL, SeverityWarning},
	"IS-R-002": {peppolUBL, SeverityFatal},
	"IS-R-003": {peppolUBL, SeverityFatal},
	"IS-R-004": {peppolUBL, SeverityFatal},
	"IS-R-005": {peppolUBL, SeverityFatal},
	"IS-R-006": {peppolUBL, SeverityFatal},
	"IS-R-007": {peppolUBL, SeverityFatal},
	"IS-R-008": {peppolUBL, SeverityFatal},
	"IS-R-009": {peppolUBL, SeverityFatal},
	"IS-R-010": {peppolUBL, SeverityFatal},

	// Italy — 4 fatal, both bindings.
	"IT-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"IT-R-004": {peppolUBL | peppolCII, SeverityFatal},

	// Norway — 1 fatal, 1 advisory, both bindings.
	"NO-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"NO-R-002": {peppolUBL | peppolCII, SeverityWarning},

	// Sweden — 7 fatal, 6 advisory, both bindings.
	"SE-R-001": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-002": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-003": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-004": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-005": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-006": {peppolUBL | peppolCII, SeverityFatal},
	"SE-R-007": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-008": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-009": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-010": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-011": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-012": {peppolUBL | peppolCII, SeverityWarning},
	"SE-R-013": {peppolUBL | peppolCII, SeverityFatal},
}

// ---------------------------------------------------------------------------
// The country variables both bindings declare
// ---------------------------------------------------------------------------

// peppolSubstring is XPath's fn:substring over codepoints, with the one-based
// positions and the "keep every position p where start <= p < start+length"
// semantics that make substring($v, 0, 4) the first three characters and not the
// first four. DK-R-008..011 in the CII binding are written that way.
//
// length is variadic so the two-argument form — substring($v, 3), used by NO-R-001
// and GR-R-003 — is the same function.
func peppolSubstring(s string, start int, length ...int) string {
	r := []rune(s)
	lo, hi := start, len(r)+1
	if len(length) > 0 {
		hi = start + length[0]
	}
	if lo < 1 {
		lo = 1
	}
	if hi > len(r)+1 {
		hi = len(r) + 1
	}
	if lo >= hi {
		return ""
	}
	return string(r[lo-1 : hi-1])
}

// peppolCountryOf is the `if (A) then upper-case(normalize-space(A)) else if (B)
// then ... else 'XX'` chain $supplierCountry and $customerCountry share.
//
// The condition is the raw expression's effective boolean value and the value is
// its normalized form, which are not the same test: a candidate that is one space
// selects the branch and yields "". Passing the raw strings and normalizing here
// keeps that distinction rather than collapsing it.
func peppolCountryOf(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return strings.ToUpper(normalizeSpace(c))
		}
	}
	return "XX"
}

// peppolUBLVATPrefix is `cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/substring(cbc:CompanyID, 1, 2)`
// under a UBL party.
//
// The predicate compares the element's string value with no normalization, which
// is how the artefact writes it, so a cbc:ID padded with whitespace does not
// select the group.
func peppolUBLVATPrefix(party *ciiNode) string {
	return peppolSubstring(peppolUBLVATCompanyID(party), 1, 2)
}

// peppolUBLVATCompanyID is the cbc:CompanyID of the same group, untrimmed.
func peppolUBLVATCompanyID(party *ciiNode) string {
	for _, ts := range party.orNil().all("PartyTaxScheme") {
		if ts.child("TaxScheme", "ID").rawText() == "VAT" {
			return ts.child("CompanyID").rawText()
		}
	}
	return ""
}

// peppolUBLPostalCountry is `cac:<party>/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode`
// from the document element — the term BT-40/BT-55, untrimmed.
func peppolUBLPostalCountry(root *ciiNode, party string) string {
	return root.child(party, "Party", "PostalAddress", "Country", "IdentificationCode").rawText()
}

// peppolUBLSupplierCountry is the UBL binding's $supplierCountry: the seller's VAT
// prefix, then the tax representative's, then the seller's postal country, then
// 'XX'. The Italian, Norwegian and Greek rules are gated on it.
func peppolUBLSupplierCountry(root *ciiNode) string {
	return peppolCountryOf(
		peppolUBLVATPrefix(root.child("AccountingSupplierParty", "Party")),
		peppolUBLVATPrefix(root.child("TaxRepresentativeParty")),
		peppolUBLPostalCountry(root, "AccountingSupplierParty"),
	)
}

// peppolUBLCustomerCountry is $customerCountry, which has no tax-representative
// step.
func peppolUBLCustomerCountry(root *ciiNode) string {
	return peppolCountryOf(
		peppolUBLVATPrefix(root.child("AccountingCustomerParty", "Party")),
		peppolUBLPostalCountry(root, "AccountingCustomerParty"),
	)
}

// peppolCIIVATPrefix is `ram:SpecifiedTaxRegistration[ram:ID/@schemeID = 'VAT']/substring(ram:ID, 1, 2)`
// under a CII trade party.
func peppolCIIVATPrefix(party *ciiNode) string {
	return peppolSubstring(peppolCIIVATID(party), 1, 2)
}

// peppolCIIVATID is the ram:ID of the same group, untrimmed.
func peppolCIIVATID(party *ciiNode) string {
	for _, reg := range party.orNil().all("SpecifiedTaxRegistration") {
		for _, id := range reg.all("ID") {
			if id.attr("schemeID") == "VAT" {
				return id.rawText()
			}
		}
	}
	return ""
}

// peppolCIIParty is one of the header trade parties, by element name.
func peppolCIIParty(root *ciiNode, name string) *ciiNode {
	return root.child("SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement", name)
}

// peppolCIIPostalCountry is a trade party's ram:PostalTradeAddress/ram:CountryID,
// untrimmed.
func peppolCIIPostalCountry(root *ciiNode, name string) string {
	return peppolCIIParty(root, name).child("PostalTradeAddress", "CountryID").rawText()
}

// peppolCIISupplierCountry is the CII binding's $supplierCountry.
func peppolCIISupplierCountry(root *ciiNode) string {
	return peppolCountryOf(
		peppolCIIVATPrefix(peppolCIIParty(root, "SellerTradeParty")),
		peppolCIIVATPrefix(peppolCIIParty(root, "SellerTaxRepresentativeTradeParty")),
		peppolCIIPostalCountry(root, "SellerTradeParty"),
	)
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

// peppolCountryUBLRules evaluates the country-specific rules of
// PEPPOL-EN16931-UBL.sch. It is a no-op for any other root.
func peppolCountryUBLRules(e *peppolEval, r *run, root *ciiNode) {
	if root == nil || (root.name != "Invoice" && root.name != "CreditNote") {
		return
	}
	peppolNorwegianUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolItalianUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolDutchUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolDanishUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolSwedishUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolGreekUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolIcelandicUBLRules(e, root)
	if r.stopped() {
		return
	}
	peppolGermanUBLRules(e, root)
}

// peppolCountryCIIRules evaluates the country-specific rules of
// PEPPOL-EN16931-CII.sch — the 41 identifiers that file publishes.
func peppolCountryCIIRules(e *peppolEval, r *run, root *ciiNode) {
	if root == nil || root.name != "CrossIndustryInvoice" {
		return
	}
	peppolNorwegianCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolItalianCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolDutchCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolDanishCIIRules(e, root)
	if r.stopped() {
		return
	}
	peppolSwedishCIIRules(e, root)
}

// ---------------------------------------------------------------------------
// Norway
// ---------------------------------------------------------------------------

// peppolNorwegianUBLRules is the Norwegian pattern of the UBL binding, context
// `cac:AccountingSupplierParty/cac:Party[$supplierCountry = 'NO']`.
func peppolNorwegianUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLSupplierCountry(root) != "NO" {
		return
	}
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		// NO-R-002: normalize-space(cac:PartyTaxScheme[normalize-space(cac:TaxScheme/
		// cbc:ID) = 'TAX']/cbc:CompanyID) = 'Foretaksregisteret'
		//
		// This predicate normalizes and the VAT one two lines down does not; both are
		// transcribed as written.
		found := false
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) == "TAX" &&
				normalizeSpace(ts.child("CompanyID").rawText()) == "Foretaksregisteret" {
				found = true
			}
		}
		if !found {
			e.add("NO-R-002", "For Norwegian suppliers, the word \"Foretaksregisteret\" MUST be appended to the "+
				"invoice as a TAX-scheme company identifier")
		}
		// NO-R-001: <prefix is NO> and matches(substring(id,3), '^[0-9]{9}MVA$') and
		// u:mod11(substring(id, 3, 9)) or not(<prefix is NO>)
		//
		// The `or not(...)` arm is the whole conditionality of the rule: a Norwegian
		// seller whose VAT identifier is another country's is not reported.
		id := peppolUBLVATCompanyID(party)
		if peppolNorwegianVATBad(id) {
			e.addf("NO-R-001", "The Norwegian VAT identifier (BT-31=%q) MUST be NO, nine digits passing the mod-11 "+
				"check, and the letters MVA", normalizeSpace(id))
		}
	}
}

// peppolNorwegianCIIRules is the same pattern in the CII binding, context
// `ram:SellerTradeParty[$supplierCountry = 'NO']`.
func peppolNorwegianCIIRules(e *peppolEval, root *ciiNode) {
	if peppolCIISupplierCountry(root) != "NO" {
		return
	}
	for _, party := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement", "SellerTradeParty") {
		// NO-R-002: ram:SpecifiedTaxRegistration/ram:ID[@schemeID = 'FC']
		//           [normalize-space(text()) = 'Foretaksregisteret']
		//
		// An existence test on a predicated node, not a string comparison — the two
		// bindings differ here, and the CII one is satisfied by any such element.
		found := false
		for _, reg := range party.all("SpecifiedTaxRegistration") {
			for _, id := range reg.all("ID") {
				if id.attr("schemeID") == "FC" && normalizeSpace(id.rawText()) == "Foretaksregisteret" {
					found = true
				}
			}
		}
		if !found {
			e.add("NO-R-002", "For Norwegian suppliers, the word \"Foretaksregisteret\" MUST be appended to the "+
				"invoice as an FC-scheme tax registration")
		}
		if id := peppolCIIVATID(party); peppolNorwegianVATBad(id) {
			e.addf("NO-R-001", "The Norwegian VAT identifier (BT-31=%q) MUST be NO, nine digits passing the mod-11 "+
				"check, and the letters MVA", normalizeSpace(id))
		}
	}
}

// peppolNorwegianVATBad is NO-R-001's assertion, negated: both bindings write the
// same test over a different element.
//
//	substring(id, 1, 2) = 'NO' and matches(substring(id, 3), '^[0-9]{9}MVA$')
//	  and u:mod11(substring(id, 3, 9))
//	or not(substring(id, 1, 2) = 'NO')
func peppolNorwegianVATBad(id string) bool {
	if peppolSubstring(id, 1, 2) != "NO" {
		return false
	}
	body := peppolSubstring(id, 3)
	if len(body) != 12 || !strings.HasSuffix(body, "MVA") || !peppolAllDigits(body[:9]) {
		return true
	}
	return !peppolMod11(peppolSubstring(id, 3, 9))
}

// ---------------------------------------------------------------------------
// Italy
// ---------------------------------------------------------------------------

// peppolItalianUBLRules is the Italian pattern of the UBL binding: two rules,
// both gated `cac:AccountingSupplierParty/cac:Party[$supplierCountry = 'IT']`.
func peppolItalianUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLSupplierCountry(root) != "IT" {
		return
	}
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		// IT-R-001, context .../cac:PartyTaxScheme[normalize-space(cac:TaxScheme/
		// cbc:ID) != 'VAT']: matches(normalize-space(cbc:CompanyID),'^[A-Z0-9]{11,16}$')
		//
		// A cac:PartyTaxScheme with no cac:TaxScheme at all selects, because
		// normalize-space(()) is '' and '' != 'VAT'.
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) == "VAT" {
				continue
			}
			id := normalizeSpace(ts.child("CompanyID").rawText())
			if !peppolItalianTaxRegOK(id) {
				e.addf("IT-R-001", "For Italian suppliers the Seller tax registration identifier (BT-32=%q) MUST be "+
					"11 to 16 upper-case alphanumerics", id)
			}
		}
		// IT-R-002/003/004 are existence tests on the postal address, so an element
		// written empty satisfies them — the shape C32 records for R001/R003/R010/R020.
		addr := party.child("PostalAddress")
		if addr.child("StreetName") == nil {
			e.add("IT-R-002", "Italian suppliers MUST provide the Seller address line 1 (BT-35)")
		}
		if addr.child("CityName") == nil {
			e.add("IT-R-003", "Italian suppliers MUST provide the Seller city (BT-37)")
		}
		if addr.child("PostalZone") == nil {
			e.add("IT-R-004", "Italian suppliers MUST provide the Seller post code (BT-38)")
		}
	}
}

// peppolItalianCIIRules is the Italian pattern of the CII binding. IT-R-001 is
// written differently there — the context is every ram:SpecifiedTaxRegistration and
// the test excuses any registration that is not scheme FC — while IT-R-002..004 are
// the same existence tests over the CII address element names.
func peppolItalianCIIRules(e *peppolEval, root *ciiNode) {
	if peppolCIISupplierCountry(root) != "IT" {
		return
	}
	for _, party := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement", "SellerTradeParty") {
		for _, reg := range party.all("SpecifiedTaxRegistration") {
			// IT-R-001: ram:ID[normalize-space(@schemeID) != 'FC'] or
			//           matches(normalize-space(ram:ID[normalize-space(@schemeID) = 'FC']), '^[A-Z0-9]{11,16}$')
			//
			// The first arm is an existence test, so a registration carrying any
			// non-FC identifier passes whatever its FC one looks like.
			nonFC, fc := false, ""
			for _, id := range reg.all("ID") {
				if normalizeSpace(id.attr("schemeID")) != "FC" {
					nonFC = true
				} else if fc == "" {
					fc = normalizeSpace(id.rawText())
				}
			}
			if nonFC || peppolItalianTaxRegOK(fc) {
				continue
			}
			e.addf("IT-R-001", "For Italian suppliers the Seller tax registration identifier (BT-32=%q) MUST be "+
				"11 to 16 upper-case alphanumerics", fc)
		}
		addr := party.child("PostalTradeAddress")
		if addr.child("LineOne") == nil {
			e.add("IT-R-002", "Italian suppliers MUST provide the Seller address line 1 (BT-35)")
		}
		if addr.child("CityName") == nil {
			e.add("IT-R-003", "Italian suppliers MUST provide the Seller city (BT-37)")
		}
		if addr.child("PostcodeCode") == nil {
			e.add("IT-R-004", "Italian suppliers MUST provide the Seller post code (BT-38)")
		}
	}
}

// peppolItalianTaxRegOK is IT-R-001's pattern '^[A-Z0-9]{11,16}$'.
func peppolItalianTaxRegOK(s string) bool {
	if len(s) < 11 || len(s) > 16 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// The Netherlands
// ---------------------------------------------------------------------------

// peppolDutchUBLRules is OpenPEPPOL's Dutch pattern in the UBL binding.
//
// This is not the NLCIUS rule set of nlcius.go. That one is BR-NL-* under
// SourceNLCIUS and belongs to a different specification identifier; these nine
// identifiers are OpenPEPPOL's, published in the Peppol BIS Billing files, and
// several say something NLCIUS does not — NL-R-003/005 restrict the legal
// registration scheme to KVK or OIN, and NL-R-008 restricts the payment means
// code, neither of which BR-NL-* states.
//
// The gates are the three <let> variables of the pattern, and all three read a
// postal address rather than a VAT prefix:
//
//	$supplierCountryIsNL          upper-case(normalize-space(/*/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode)) = 'NL'
//	$customerCountryIsNL          the same over cac:AccountingCustomerParty
//	$taxRepresentativeCountryIsNL the same over /*/cac:TaxRepresentativeParty/cac:PostalAddress
func peppolDutchUBLRules(e *peppolEval, root *ciiNode) {
	isNL := func(s string) bool { return strings.EqualFold(normalizeSpace(s), "NL") }
	supplier := isNL(peppolUBLPostalCountry(root, "AccountingSupplierParty"))
	if !supplier {
		return
	}
	customer := isNL(peppolUBLPostalCountry(root, "AccountingCustomerParty"))
	taxRep := isNL(root.child("TaxRepresentativeParty", "PostalAddress", "Country", "IdentificationCode").rawText())

	// NL-R-001, context cbc:CreditNoteTypeCode[$supplierCountryIsNL]:
	//   /*/cac:BillingReference/cac:InvoiceDocumentReference/cbc:ID
	//
	// The context is the credit-note type code, so an invoice is out of scope
	// without the rule needing to say so.
	for range root.all("CreditNoteTypeCode") {
		if len(nodesAt(root, "BillingReference", "InvoiceDocumentReference", "ID")) == 0 {
			e.add("NL-R-001", "For suppliers in the Netherlands, a credit note MUST contain a Preceding Invoice "+
				"reference (BT-25)")
		}
	}
	// NL-R-002 / NL-R-004 / NL-R-006: cbc:StreetName and cbc:CityName and
	// cbc:PostalZone, over the seller's, the buyer's and the tax representative's
	// postal address. Existence tests, so an empty element satisfies them.
	address := func(rule string, addr *ciiNode, who string) {
		if addr == nil {
			return
		}
		if addr.child("StreetName") == nil || addr.child("CityName") == nil || addr.child("PostalZone") == nil {
			e.addf(rule, "For suppliers in the Netherlands, the %s address MUST contain a street name, a city and a "+
				"post code", who)
		}
	}
	for _, addr := range nodesAt(root, "AccountingSupplierParty", "Party", "PostalAddress") {
		address("NL-R-002", addr, "supplier's")
	}
	// NL-R-003 / NL-R-005: the legal registration identifier's scheme (BT-30/BT-47)
	//   (contains(concat(' ', string-join(@schemeID, ' '), ' '), ' 0106 ') or
	//    contains(..., ' 0190 ')) and (normalize-space(.) != '')
	//
	// Both halves matter: the second is the one existence-test reading would miss,
	// and unlike NL-R-002 this rule does reject an empty element.
	legalID := func(rule string, party *ciiNode, who string) {
		for _, le := range party.orNil().all("PartyLegalEntity") {
			for _, id := range le.all("CompanyID") {
				if peppolDutchLegalScheme(id.attr("schemeID")) && normalizeSpace(id.rawText()) != "" {
					continue
				}
				e.addf(rule, "For suppliers in the Netherlands, the %s legal registration identifier MUST be a KVK or "+
					"OIN number (scheme 0106 or 0190) with a value", who)
			}
		}
	}
	legalID("NL-R-003", root.child("AccountingSupplierParty", "Party"), "supplier's")
	if customer {
		for _, addr := range nodesAt(root, "AccountingCustomerParty", "Party", "PostalAddress") {
			address("NL-R-004", addr, "customer's")
		}
		legalID("NL-R-005", root.child("AccountingCustomerParty", "Party"), "customer's")
	}
	if taxRep {
		for _, addr := range nodesAt(root, "TaxRepresentativeParty", "PostalAddress") {
			address("NL-R-006", addr, "fiscal representative's")
		}
	}
	// NL-R-007, context cac:LegalMonetaryTotal[$supplierCountryIsNL]:
	//   (/ubl:Invoice and xs:decimal(cbc:PayableAmount) <= 0.0) or
	//   (/cn:CreditNote and xs:decimal(cbc:PayableAmount) >= 0.0) or (//cac:PaymentMeans)
	//
	// "A means of payment is required if the payment runs from customer to
	// supplier", expressed as: either nothing is owed, or payment instructions are
	// present.
	hasPaymentMeans := len(root.findAll("PaymentMeans")) > 0
	isCredit := root.name == "CreditNote"
	for _, total := range root.all("LegalMonetaryTotal") {
		if hasPaymentMeans {
			continue
		}
		amt := total.child("PayableAmount")
		if amt == nil {
			// xs:decimal(()) is the empty sequence, and both comparisons against it are
			// false, so the assertion rests on the payment means alone.
			e.add("NL-R-007", "For suppliers in the Netherlands, Payment instructions (BG-16) MUST be provided when "+
				"the payment is from the customer to the supplier")
			continue
		}
		due, ok := parseAmount(amt.text)
		if !ok {
			// XPath raises a dynamic error rather than reporting the assertion; that is
			// peppolDecimalOr's argument for R120 and it holds here.
			continue
		}
		if (!isCredit && due <= 0) || (isCredit && due >= 0) {
			continue
		}
		e.add("NL-R-007", "For suppliers in the Netherlands, Payment instructions (BG-16) MUST be provided when the "+
			"payment is from the customer to the supplier")
	}
	// NL-R-008, context cac:PaymentMeans[$supplierCountryIsNL and $customerCountryIsNL].
	if customer {
		for _, pm := range root.findAll("PaymentMeans") {
			code := normalizeSpace(pm.child("PaymentMeansCode").rawText())
			if !peppolDutchPaymentMeans[code] {
				e.addf("NL-R-008", "For suppliers in the Netherlands invoicing a Dutch customer, the Payment means "+
					"type code (BT-81=%q) MUST be one of 30, 48, 49, 57, 58 or 59", code)
			}
		}
	}
	// NL-R-009, context cac:OrderLineReference/cbc:LineID[$supplierCountryIsNL]:
	//   exists(/*/cac:OrderReference/cbc:ID)
	if len(nodesAt(root, "OrderReference", "ID")) == 0 {
		for _, olr := range root.findAll("OrderLineReference") {
			for range olr.all("LineID") {
				e.add("NL-R-009", "For suppliers in the Netherlands, an Invoice line order line reference (BT-132) "+
					"requires a Purchase order reference (BT-13) on document level")
			}
		}
	}
}

// peppolDutchCIIRules is the same pattern in the CII binding.
//
// Two rules are written differently there and the difference is not cosmetic:
// NL-R-001's context is a document type code drawn from a five-code list rather
// than a credit-note root, because CII expresses both document kinds with one
// root; and NL-R-007 discriminates the two by BT-3 = '381' for the same reason.
func peppolDutchCIIRules(e *peppolEval, root *ciiNode) {
	isNL := func(s string) bool { return strings.EqualFold(normalizeSpace(s), "NL") }
	if !isNL(peppolCIIPostalCountry(root, "SellerTradeParty")) {
		return
	}
	customer := isNL(peppolCIIPostalCountry(root, "BuyerTradeParty"))
	taxRep := isNL(peppolCIIPostalCountry(root, "SellerTaxRepresentativeTradeParty"))
	seller := peppolCIIParty(root, "SellerTradeParty")
	buyer := peppolCIIParty(root, "BuyerTradeParty")
	settlements := nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement")

	// NL-R-001, context rsm:ExchangedDocument[some $code in tokenize('81 83 381
	// 396 532', '\s') satisfies normalize-space(ram:TypeCode) = $code]:
	//   //ram:ApplicableHeaderTradeSettlement/ram:InvoiceReferencedDocument/ram:IssuerAssignedID
	for _, doc := range root.all("ExchangedDocument") {
		credit := false
		for _, tc := range doc.all("TypeCode") {
			if peppolDutchCreditNoteTypes[normalizeSpace(tc.rawText())] {
				credit = true
			}
		}
		if !credit {
			continue
		}
		found := false
		for _, st := range root.findAll("ApplicableHeaderTradeSettlement") {
			if len(nodesAt(st, "InvoiceReferencedDocument", "IssuerAssignedID")) > 0 {
				found = true
			}
		}
		if !found {
			e.add("NL-R-001", "For suppliers in the Netherlands, a credit note MUST contain a Preceding Invoice "+
				"reference (BT-25)")
		}
	}
	address := func(rule string, party *ciiNode, who string) {
		for _, addr := range party.orNil().all("PostalTradeAddress") {
			if addr.child("LineOne") == nil || addr.child("CityName") == nil || addr.child("PostcodeCode") == nil {
				e.addf(rule, "For suppliers in the Netherlands, the %s address MUST contain a street name, a city "+
					"and a post code", who)
			}
		}
	}
	legalID := func(rule string, party *ciiNode, who string) {
		for _, org := range party.orNil().all("SpecifiedLegalOrganization") {
			for _, id := range org.all("ID") {
				if peppolDutchLegalScheme(id.attr("schemeID")) && normalizeSpace(id.rawText()) != "" {
					continue
				}
				e.addf(rule, "For suppliers in the Netherlands, the %s legal registration identifier MUST be a KVK "+
					"or OIN number (scheme 0106 or 0190) with a value", who)
			}
		}
	}
	address("NL-R-002", seller, "supplier's")
	legalID("NL-R-003", seller, "supplier's")
	if customer {
		address("NL-R-004", buyer, "customer's")
		legalID("NL-R-005", buyer, "customer's")
	}
	if taxRep {
		address("NL-R-006", peppolCIIParty(root, "SellerTaxRepresentativeTradeParty"), "fiscal representative's")
	}
	// NL-R-007, context ram:SpecifiedTradeSettlementHeaderMonetarySummation.
	isCredit := false
	for _, doc := range root.all("ExchangedDocument") {
		if normalizeSpace(doc.child("TypeCode").rawText()) == "381" {
			isCredit = true
		}
	}
	hasPaymentMeans := false
	for _, st := range settlements {
		if len(st.all("SpecifiedTradeSettlementPaymentMeans")) > 0 {
			hasPaymentMeans = true
		}
	}
	for _, st := range settlements {
		for _, sum := range st.all("SpecifiedTradeSettlementHeaderMonetarySummation") {
			if hasPaymentMeans {
				continue
			}
			amt := sum.child("DuePayableAmount")
			if amt != nil {
				due, ok := parseAmount(amt.text)
				if !ok {
					continue
				}
				if (!isCredit && due <= 0) || (isCredit && due >= 0) {
					continue
				}
			}
			e.add("NL-R-007", "For suppliers in the Netherlands, Payment instructions (BG-16) MUST be provided when "+
				"the payment is from the customer to the supplier")
		}
	}
	// NL-R-008, context ram:SpecifiedTradeSettlementPaymentMeans.
	if customer {
		for _, st := range settlements {
			for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
				code := normalizeSpace(pm.child("TypeCode").rawText())
				if !peppolDutchPaymentMeans[code] {
					e.addf("NL-R-008", "For suppliers in the Netherlands invoicing a Dutch customer, the Payment means "+
						"type code (BT-81=%q) MUST be one of 30, 48, 49, 57, 58 or 59", code)
				}
			}
		}
	}
	// NL-R-009, context ram:IncludedSupplyChainTradeLineItem/
	// ram:SpecifiedLineTradeAgreement/ram:BuyerOrderReferencedDocument/ram:LineID.
	if len(nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeAgreement",
		"BuyerOrderReferencedDocument", "IssuerAssignedID")) > 0 {
		return
	}
	for _, li := range nodesAt(root, "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem") {
		for range nodesAt(li, "SpecifiedLineTradeAgreement", "BuyerOrderReferencedDocument", "LineID") {
			e.add("NL-R-009", "For suppliers in the Netherlands, an Invoice line order line reference (BT-132) "+
				"requires a Purchase order reference (BT-13) on document level")
		}
	}
}

// peppolDutchLegalScheme is NL-R-003/005's scheme test:
//
//	contains(concat(' ', string-join(@schemeID, ' '), ' '), ' 0106 ') or
//	contains(concat(' ', string-join(@schemeID, ' '), ' '), ' 0190 ')
//
// KVK (the Dutch chamber-of-commerce register) or OIN (the government
// organisation identifier). An absent attribute joins to "" and matches neither.
func peppolDutchLegalScheme(scheme string) bool {
	padded := " " + scheme + " "
	return strings.Contains(padded, " 0106 ") || strings.Contains(padded, " 0190 ")
}

// peppolDutchPaymentMeans is NL-R-008's permitted set of BT-81 values.
var peppolDutchPaymentMeans = map[string]bool{
	"30": true, "48": true, "49": true, "57": true, "58": true, "59": true,
}

// peppolDutchCreditNoteTypes is the code list NL-R-001's CII context draws on,
// `tokenize('81 83 381 396 532', '\s')` — the UNTDID 1001 codes that make a CII
// document a credit note. The UBL binding needs no such list because it has a
// credit-note root element.
var peppolDutchCreditNoteTypes = map[string]bool{
	"81": true, "83": true, "381": true, "396": true, "532": true,
}

// ---------------------------------------------------------------------------
// Denmark
// ---------------------------------------------------------------------------

// peppolDanishPaymentMeans is DK-R-005's permitted set of BT-81 values, from
// `contains(' 1 10 31 42 48 49 50 58 59 93 97 ', concat(' ', <code>, ' '))`.
var peppolDanishPaymentMeans = map[string]bool{
	"1": true, "10": true, "31": true, "42": true, "48": true, "49": true,
	"50": true, "58": true, "59": true, "93": true, "97": true,
}

// peppolDanishUNSPSC is DK-R-003's permitted UNSPSC versions.
var peppolDanishUNSPSC = map[string]bool{
	"19.05.01": true, "19.0501": true, "26.08.01": true, "26.0801": true,
}

// peppolDanishUBLRules is the Danish pattern of the UBL binding: 14 rules across
// six contexts, gated on two pattern variables.
//
//	$DKSupplierCountry = concat(cn:CreditNote/…/cac:PostalAddress/cac:Country/cbc:IdentificationCode,
//	                            ubl:Invoice/…/cac:PostalAddress/cac:Country/cbc:IdentificationCode)
//	$DKCustomerCountry   the same over cac:AccountingCustomerParty
//
// The concat is how OpenPEPPOL writes "whichever root this document has"; only one
// arm can be non-empty. The comparison is `= 'DK'` against the untrimmed string
// value, so a padded country code does not select the family — transcribed as
// written rather than normalized, because the German and Dutch patterns *do*
// normalize and the difference is the artefact's.
//
// Nine of the fourteen are domestic-only (both parties Danish); DK-R-002, DK-R-014
// and DK-R-016 need only a Danish seller. And the payment-means rules are bound to
// `ubl:Invoice[…]/cac:PaymentMeans` with no credit-note arm, so a Danish credit
// note's payment means answers to none of DK-R-005..011. That asymmetry is
// OpenPEPPOL's; reading the family as "all Danish documents" would report seven
// rules against credit notes the authority does not check.
func peppolDanishUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLPostalCountry(root, "AccountingSupplierParty") != "DK" {
		return
	}
	domestic := peppolUBLPostalCountry(root, "AccountingCustomerParty") == "DK"
	seller := root.child("AccountingSupplierParty", "Party")
	customer := root.child("AccountingCustomerParty", "Party")

	// DK-R-002: normalize-space(…/cac:PartyLegalEntity/cbc:CompanyID/text()) != ''
	sellerLegal := seller.child("PartyLegalEntity", "CompanyID")
	if normalizeSpace(sellerLegal.rawText()) == "" {
		e.add("DK-R-002", "Danish suppliers MUST provide a legal registration identifier (BT-30, the CVR number)")
	}
	// DK-R-014: not(boolean(<id>) and normalize-space(<id>/@schemeID) != '0184')
	if sellerLegal != nil && normalizeSpace(sellerLegal.attr("schemeID")) != "0184" {
		e.add("DK-R-014", "For Danish suppliers the Seller legal registration identifier (BT-30) MUST declare "+
			"scheme 0184 (DK CVR)")
	}
	// DK-R-016: not((boolean(/cn:CreditNote) and $DKCustomerCountry = 'DK') and
	//               (number(cac:LegalMonetaryTotal/cbc:PayableAmount/text()) < 0))
	if root.name == "CreditNote" && domestic {
		if due, ok := parseAmount(root.child("LegalMonetaryTotal", "PayableAmount").rawText()); ok && due < 0 {
			e.addf("DK-R-016", "For Danish suppliers a credit note's Amount due for payment (BT-115=%s) MUST NOT be "+
				"negative", normalizeSpace(root.child("LegalMonetaryTotal", "PayableAmount").rawText()))
		}
	}
	if !domestic {
		return
	}
	// DK-R-013, context …/cac:Party/cac:PartyIdentification on either party:
	//   not(boolean(cbc:ID) and normalize-space(cbc:ID/@schemeID) = '')
	//
	// An identifier with no @schemeID at all normalizes to '' and is reported, which
	// is the case the rule exists for.
	for _, party := range []*ciiNode{seller, customer} {
		for _, pi := range party.orNil().all("PartyIdentification") {
			if id := pi.child("ID"); id != nil && normalizeSpace(id.attr("schemeID")) == "" {
				e.add("DK-R-013", "For Danish suppliers a Party identification identifier (BT-29/BT-46) MUST declare "+
					"a scheme identifier")
			}
		}
	}
	// DK-R-017, context …/cac:AccountingCustomerParty/cac:Party.
	if customerLegal := customer.child("PartyLegalEntity", "CompanyID"); customerLegal != nil &&
		normalizeSpace(customerLegal.attr("schemeID")) != "0184" {
		e.add("DK-R-017", "For Danish customers the Buyer legal registration identifier (BT-47) MUST declare "+
			"scheme 0184 (DK CVR)")
	}
	// DK-R-003, context …/cac:InvoiceLine | …/cac:CreditNoteLine.
	lineName := "InvoiceLine"
	if root.name == "CreditNote" {
		lineName = "CreditNoteLine"
	}
	for _, li := range root.all(lineName) {
		peppolDanishClassification(e, nodesAt(li, "Item", "CommodityClassification", "ItemClassificationCode"))
	}
	// DK-R-004, context cac:AllowanceCharge[$DKSupplierCountry = 'DK' and
	// $DKCustomerCountry = 'DK'] — every allowance or charge in the document, at any
	// level, unlike the CII binding where the whole rule is one document-level test.
	for _, ac := range root.findAll("AllowanceCharge") {
		if !peppolAnyChildValue(ac, "AllowanceChargeReasonCode", "ZZZ") {
			continue
		}
		if peppolDanishTaxReasonOK(ac.child("AllowanceChargeReason").rawText()) {
			continue
		}
		e.add("DK-R-004", "For Danish domestic invoices a non-VAT tax (AllowanceChargeReasonCode ZZZ) MUST state "+
			"the four-digit tax category or a value containing an internal '#' in the reason (BT-97/BT-104)")
	}
	// DK-R-005..011, context ubl:Invoice[both]/cac:PaymentMeans.
	if root.name != "Invoice" {
		return
	}
	for _, pm := range root.all("PaymentMeans") {
		code := pm.child("PaymentMeansCode").rawText()
		if !peppolDanishPaymentMeans[code] {
			e.addf("DK-R-005", "For Danish suppliers the Payment means type code (BT-81=%q) MUST be one of "+
				"1, 10, 31, 42, 48, 49, 50, 58, 59, 93 or 97", normalizeSpace(code))
		}
		account := pm.child("PayeeFinancialAccount")
		paymentID := pm.child("PaymentID").rawText()
		switch code {
		case "31", "42":
			// DK-R-006: both the account (BT-84) and the branch identifier (BT-85).
			if normalizeSpace(account.child("ID").rawText()) == "" ||
				normalizeSpace(account.child("FinancialInstitutionBranch", "ID").rawText()) == "" {
				e.add("DK-R-006", "For Danish suppliers a payment means of 31 or 42 requires both the Payment account "+
					"identifier (BT-84) and the Payment service provider identifier (BT-85)")
			}
		case "49":
			// DK-R-007: the mandate reference (BT-89) and the debited account (BT-91).
			mandate := pm.child("PaymentMandate")
			if normalizeSpace(mandate.child("ID").rawText()) == "" ||
				normalizeSpace(mandate.child("PayerFinancialAccount", "ID").rawText()) == "" {
				e.add("DK-R-007", "For Danish suppliers a payment means of 49 requires both the Mandate reference "+
					"identifier (BT-89) and the Debited account identifier (BT-91)")
			}
		case "50":
			// DK-R-008: a Giro card code prefix and a 7-or-8-digit Giro account.
			if !peppolDanishCardCode(paymentID, "01#", "04#", "15#") ||
				!peppolDanishGiroAccount(account.child("ID").rawText(), 7, 8) {
				e.add("DK-R-008", "For Danish suppliers a payment means of 50 (Giro) requires a Payment reference "+
					"(BT-83) beginning 01#, 04# or 15# and a 7 or 8 digit Giro account (BT-84)")
			}
			// DK-R-009: those two prefixes carry a 16-digit instruction identifier.
			if peppolDanishCardCode(paymentID, "04#", "15#") && len([]rune(paymentID)) != 19 {
				e.add("DK-R-009", "For Danish suppliers a Payment reference (BT-83) prefixed 04# or 15# MUST be "+
					"nineteen characters — the prefix and a sixteen digit instruction identifier")
			}
		case "93":
			// DK-R-010: an FIK card code prefix and an 8-character account.
			if !peppolDanishCardCode(paymentID, "71#", "73#", "75#") ||
				len([]rune(account.child("ID").rawText())) != 8 {
				e.add("DK-R-010", "For Danish suppliers a payment means of 93 (FIK) requires a Payment reference "+
					"(BT-83) beginning 71#, 73# or 75# and an eight character account (BT-84)")
			}
			// DK-R-011: two of those prefixes carry a 15-or-16-digit identifier.
			if peppolDanishCardCode(paymentID, "71#", "75#") {
				if n := len([]rune(paymentID)); n != 18 && n != 19 {
					e.add("DK-R-011", "For Danish suppliers a Payment reference (BT-83) prefixed 71# or 75# MUST be "+
						"eighteen or nineteen characters")
				}
			}
		}
	}
}

// peppolDanishCIIRules is the Danish pattern of the CII binding — 13 rules, all of
// DK-R-017 absent.
//
// Two differences from the UBL binding are structural rather than incidental:
// DK-R-004 and DK-R-013 are single document-level tests here, where UBL binds them
// to each allowance/charge and each party identification; and DK-R-016 tells a
// credit note from an invoice by BT-3 = '381', CII having one root for both.
//
// A third is worth naming because it looks like a transcription error and is not.
// DK-R-007's XPath reads `ram:SpecifiedTradePaymentTerms/ram:DirectDebitMandateID`
// as a *child* of the payment means, where the CII schema puts payment terms beside
// it under the settlement. OpenPEPPOL's own nine fixtures for the rule are written
// the same way — the passing one nests SpecifiedTradePaymentTerms inside
// SpecifiedTradeSettlementPaymentMeans — so this is the artefact's reading of its
// own rule, and the XPath is transcribed rather than corrected. The consequence for
// a schema-valid Danish CII invoice using payment means 49 is that DK-R-007 fires,
// and a reference Peppol validation reports it too.
func peppolDanishCIIRules(e *peppolEval, root *ciiNode) {
	if peppolCIIPostalCountry(root, "SellerTradeParty") != "DK" {
		return
	}
	domestic := peppolCIIPostalCountry(root, "BuyerTradeParty") == "DK"
	seller := peppolCIIParty(root, "SellerTradeParty")
	buyer := peppolCIIParty(root, "BuyerTradeParty")
	settlements := nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement")

	// DK-R-002: normalize-space(…/ram:SpecifiedLegalOrganization/ram:ID/text()) != ''
	sellerLegal := seller.child("SpecifiedLegalOrganization", "ID")
	if normalizeSpace(sellerLegal.rawText()) == "" {
		e.add("DK-R-002", "Danish suppliers MUST provide a legal registration identifier (BT-30)")
	}
	// DK-R-014.
	if sellerLegal != nil && normalizeSpace(sellerLegal.attr("schemeID")) != "0184" {
		e.add("DK-R-014", "For Danish suppliers the Seller legal registration identifier (BT-30) MUST declare "+
			"scheme 0184 (DK CVR)")
	}
	if domestic {
		// DK-R-013: either party's ram:GlobalID present with an empty @schemeID. It is
		// one test over two named elements here, not a per-node rule.
		for _, party := range []*ciiNode{seller, buyer} {
			if id := party.orNil().child("GlobalID"); id != nil && normalizeSpace(id.attr("schemeID")) == "" {
				e.add("DK-R-013", "For Danish suppliers a Party identification identifier (BT-29/BT-46) MUST declare "+
					"a scheme identifier")
				break
			}
		}
		// DK-R-016: BT-3 = '381' and a negative BT-115.
		credit := false
		for _, doc := range root.all("ExchangedDocument") {
			if normalizeSpace(doc.child("TypeCode").rawText()) == "381" {
				credit = true
			}
		}
		if credit {
			for _, st := range settlements {
				amt := st.child("SpecifiedTradeSettlementHeaderMonetarySummation", "DuePayableAmount")
				if due, ok := parseAmount(amt.rawText()); ok && due < 0 {
					e.addf("DK-R-016", "For Danish suppliers a credit note's Amount due for payment (BT-115=%s) MUST "+
						"NOT be negative", normalizeSpace(amt.rawText()))
				}
			}
		}
		// DK-R-004: one document-level test over the header allowances and charges.
		for _, st := range settlements {
			charges := st.all("SpecifiedTradeAllowanceCharge")
			zzz := false
			reason := ""
			for _, ac := range charges {
				if peppolAnyChildValue(ac, "ReasonCode", "ZZZ") {
					zzz = true
				}
				if reason == "" {
					reason = ac.child("Reason").rawText()
				}
			}
			if zzz && !peppolDanishTaxReasonOK(reason) {
				e.add("DK-R-004", "For Danish domestic invoices a non-VAT tax (ReasonCode ZZZ) MUST state the "+
					"four-digit tax category or a value containing an internal '#' in the reason (BT-97/BT-104)")
			}
		}
		// DK-R-003, context ram:IncludedSupplyChainTradeLineItem.
		for _, li := range nodesAt(root, "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem") {
			peppolDanishClassification(e, nodesAt(li, "SpecifiedTradeProduct", "DesignatedProductClassification", "ClassCode"))
		}
		// DK-R-005..011, context ram:SpecifiedTradeSettlementPaymentMeans.
		for _, st := range settlements {
			// `../ram:PaymentReference` and `../ram:CreditorReferenceID` are the
			// settlement's, which is the payment means' parent.
			paymentRef := st.child("PaymentReference").rawText()
			creditorRef := st.child("CreditorReferenceID").rawText()
			for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
				code := pm.child("TypeCode").rawText()
				if !peppolDanishPaymentMeans[code] {
					e.addf("DK-R-005", "For Danish suppliers the Payment means type code (BT-81=%q) MUST be one of "+
						"1, 10, 31, 42, 48, 49, 50, 58, 59, 93 or 97", normalizeSpace(code))
				}
				iban := pm.child("PayeePartyCreditorFinancialAccount", "IBANID").rawText()
				switch code {
				case "31", "42":
					if normalizeSpace(iban) == "" ||
						normalizeSpace(pm.child("PayeeSpecifiedCreditorFinancialInstitution", "BICID").rawText()) == "" {
						e.add("DK-R-006", "For Danish suppliers a payment means of 31 or 42 requires both the Payment "+
							"account identifier (BT-84) and the Payment service provider identifier (BT-86)")
					}
				case "49":
					if normalizeSpace(creditorRef) == "" ||
						normalizeSpace(pm.child("SpecifiedTradePaymentTerms", "DirectDebitMandateID").rawText()) == "" {
						e.add("DK-R-007", "For Danish suppliers a payment means of 49 requires both the Mandate "+
							"reference identifier (BT-89) and the Bank assigned creditor identifier (BT-90)")
					}
				case "50":
					// substring(../ram:PaymentReference, 0, 4) is the first three characters:
					// XPath keeps the positions p where 0 <= p < 4, and there is no position 0.
					if !peppolDanishCardCode(paymentRef, "01#", "04#", "15#") || len([]rune(iban)) != 7 {
						e.add("DK-R-008", "For Danish suppliers a payment means of 50 (Giro) requires a Payment "+
							"reference (BT-83) beginning 01#, 04# or 15# and a seven character Giro account (BT-84)")
					}
					if peppolDanishCardCode(paymentRef, "04#", "15#") && len([]rune(paymentRef)) != 19 {
						e.add("DK-R-009", "For Danish suppliers a Payment reference (BT-83) prefixed 04# or 15# MUST "+
							"be nineteen characters — the prefix and a sixteen digit instruction identifier")
					}
				case "93":
					if !peppolDanishCardCode(paymentRef, "71#", "73#", "75#") || len([]rune(iban)) != 8 {
						e.add("DK-R-010", "For Danish suppliers a payment means of 93 (FIK) requires a Payment "+
							"reference (BT-83) beginning 71#, 73# or 75# and an eight character account (BT-84)")
					}
					if peppolDanishCardCode(paymentRef, "71#", "75#") {
						if n := len([]rune(paymentRef)); n != 18 && n != 19 {
							e.add("DK-R-011", "For Danish suppliers a Payment reference (BT-83) prefixed 71# or 75# "+
								"MUST be eighteen or nineteen characters")
						}
					}
				}
			}
		}
	}
}

// peppolDanishClassification is DK-R-003 over one line's item classification codes:
//
//	not((…/@listID = 'TST') and not((…/@listVersionID = '19.05.01') or … ))
//
// Both halves are node-set comparisons, so the rule passes when *any* code carries
// a permitted version, whichever code carried the TST list identifier.
func peppolDanishClassification(e *peppolEval, codes []*ciiNode) {
	tst, version := false, false
	for _, c := range codes {
		if c.attr("listID") == "TST" {
			tst = true
		}
		if peppolDanishUNSPSC[c.attr("listVersionID")] {
			version = true
		}
	}
	if tst && !version {
		e.add("DK-R-003", "If a Danish supplier provides an Item classification identifier (BT-158) it should use "+
			"UNSPSC version 19.05.01 or 26.08.01")
	}
}

// peppolDanishTaxReasonOK is DK-R-004's inner disjunction over the
// allowance/charge reason (BT-97/BT-104):
//
//	(string-length(normalize-space(<reason>/text())) = 4 and number(<reason>) >= 0
//	   and number(<reason>) <= 9999)
//	or (<reason> and contains(<reason>, '#') and not(starts-with(<reason>, '#'))
//	   and not(ends-with(<reason>, '#')))
//
// Either a four-character Danish tax category in 0000..9999, or a value carrying a
// '#' that is neither its first nor its last character.
//
// The CII wording of the first arm is `number(<reason> <= 9999)` — a comparison
// wrapped in number(), so it yields 1 or 0 and its effective boolean value is the
// comparison's. The two bindings therefore agree despite the typo, and one function
// serves both.
func peppolDanishTaxReasonOK(reason string) bool {
	trimmed := normalizeSpace(reason)
	if len([]rune(trimmed)) == 4 {
		if v, ok := parseAmount(trimmed); ok && v >= 0 && v <= 9999 {
			return true
		}
	}
	return reason != "" && strings.Contains(reason, "#") &&
		!strings.HasPrefix(reason, "#") && !strings.HasSuffix(reason, "#")
}

// peppolDanishCardCode is `substring(<ref>, 1, 3) = '01#' or …` — the Danish
// "kortartkode" prefix test DK-R-008..011 share. The CII binding writes it
// substring(<ref>, 0, 4), which selects the same three characters.
func peppolDanishCardCode(ref string, codes ...string) bool {
	prefix := peppolSubstring(ref, 1, 3)
	for _, c := range codes {
		if prefix == c {
			return true
		}
	}
	return false
}

// peppolDanishGiroAccount is DK-R-008's `matches(<account>, '^[0-9]{7,8}$')`.
func peppolDanishGiroAccount(account string, lo, hi int) bool {
	n := len(account)
	return n >= lo && n <= hi && peppolAllDigits(account)
}

// peppolAnyChildValue is a node-set comparison against a literal:
// `<parent>/<name> = <value>`, which is true when any such child's string value
// matches. The value is untrimmed, as the artefact's comparisons are.
func peppolAnyChildValue(parent *ciiNode, name, value string) bool {
	for _, c := range parent.orNil().all(name) {
		if c.rawText() == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Sweden
// ---------------------------------------------------------------------------

// peppolSwedishVATRates is SE-R-006's permitted set: Sweden's three standard VAT
// rates.
var peppolSwedishVATRates = []float64{25, 12, 6}

// peppolSwedishUBLRules is the Swedish pattern of the UBL binding: 13 rules over
// eight contexts.
//
// Sweden is the one family with no <let> variable at all. Every rule's context
// carries its own country test inline, and they are not the same test:
//
//	SE-R-001/002/006  the seller's postal country is SE *and* its VAT identifier
//	                  begins SE
//	SE-R-003/004/013  the seller's postal country is SE and a legal registration
//	                  identifier is present — the VAT prefix is not consulted
//	SE-R-005          the seller's postal country is SE and a legal registration
//	                  identifier is present
//	SE-R-007..011     the seller's postal country is SE
//	SE-R-012          both parties' postal country is SE
//
// So a Swedish company invoicing under a foreign VAT registration answers to the
// organisation-number rules and not to the VAT-number ones. Collapsing the five
// gates into one would have widened three of them.
func peppolSwedishUBLRules(e *peppolEval, root *ciiNode) {
	sellerAddrSE, sellerVATSE := false, false
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		if peppolAnyChildValue(party.child("PostalAddress", "Country"), "IdentificationCode", "SE") {
			sellerAddrSE = true
			if peppolSubstring(peppolUBLVATCompanyID(party), 1, 2) == "SE" {
				sellerVATSE = true
			}
		}
	}
	if !sellerAddrSE {
		return
	}
	buyerAddrSE := false
	for _, party := range nodesAt(root, "AccountingCustomerParty", "Party") {
		if peppolAnyChildValue(party.child("PostalAddress", "Country"), "IdentificationCode", "SE") {
			buyerAddrSE = true
		}
	}
	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		if !peppolAnyChildValue(party.child("PostalAddress", "Country"), "IdentificationCode", "SE") {
			continue
		}
		vatID := peppolUBLVATCompanyID(party)
		if peppolSubstring(vatID, 1, 2) == "SE" {
			// SE-R-001: string-length(normalize-space(<vat id>)) = 14
			if len([]rune(normalizeSpace(vatID))) != 14 {
				e.addf("SE-R-001", "A Swedish Seller VAT identifier (BT-31=%q) MUST be fourteen characters",
					normalizeSpace(vatID))
			}
			// SE-R-002: string(number(substring(<vat id>, 3, 12))) != 'NaN'
			if !peppolIsXPathNumber(peppolSubstring(vatID, 3, 12)) {
				e.addf("SE-R-002", "A Swedish Seller VAT identifier (BT-31=%q) MUST carry twelve numeric characters "+
					"after the country prefix", normalizeSpace(vatID))
			}
		}
		// SE-R-003/004/013, context cac:PartyLegalEntity[<seller postal SE> and
		// cbc:CompanyID]. The VAT prefix is not part of this gate.
		for _, le := range party.all("PartyLegalEntity") {
			id := le.child("CompanyID")
			if id == nil {
				continue
			}
			raw, trimmed := id.rawText(), normalizeSpace(id.rawText())
			if !peppolIsXPathNumber(raw) {
				e.addf("SE-R-003", "A Swedish organisation number (BT-30=%q) MUST be numeric", trimmed)
			}
			if len([]rune(trimmed)) != 10 {
				e.addf("SE-R-004", "A Swedish organisation number (BT-30=%q) MUST be ten characters", trimmed)
			}
			if !peppolSwedishOrgNr(trimmed) {
				e.addf("SE-R-013", "The last digit of a Swedish organisation number (BT-30=%q) MUST satisfy the Luhn "+
					"check", trimmed)
			}
		}
		// SE-R-005, context cac:PartyTaxScheme[normalize-space(upper-case(
		// cac:TaxScheme/cbc:ID)) != 'VAT']/cbc:CompanyID, gated on the party having a
		// legal registration identifier. The scheme test is upper-cased here and not in
		// the Italian rule that reads the same element.
		if len(nodesAt(party, "PartyLegalEntity", "CompanyID")) == 0 {
			continue
		}
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(strings.ToUpper(ts.child("TaxScheme", "ID").rawText())) == "VAT" {
				continue
			}
			for _, id := range ts.all("CompanyID") {
				if normalizeSpace(strings.ToUpper(id.rawText())) != "GODKÄND FÖR F-SKATT" {
					e.addf("SE-R-005", "For Swedish suppliers the Seller tax registration identifier (BT-32=%q) MUST "+
						"read \"Godkänd för F-skatt\"", normalizeSpace(id.rawText()))
				}
			}
		}
	}
	// SE-R-006, context //cac:TaxCategory[<seller SE and VAT SE> and cbc:ID = 'S'] |
	// //cac:ClassifiedTaxCategory[<same>].
	//
	// The `and cbc:ID = 'S'` predicate is the UBL binding's alone — the CII wording
	// has no category-code test, so the two are not the same rule and are written
	// separately below.
	if sellerVATSE {
		for _, name := range []string{"TaxCategory", "ClassifiedTaxCategory"} {
			for _, tc := range root.findAll(name) {
				if !peppolAnyChildValue(tc, "ID", "S") {
					continue
				}
				peppolSwedishRate(e, tc.child("Percent"))
			}
		}
	}
	// SE-R-007..012, context //cac:PaymentMeans[…].
	for _, pm := range root.findAll("PaymentMeans") {
		code := pm.child("PaymentMeansCode")
		normalized := normalizeSpace(code.rawText())
		account := pm.child("PayeeFinancialAccount")
		branch := normalizeSpace(account.child("FinancialInstitutionBranch", "ID").rawText())
		if normalized == "30" {
			// The account identifier is the context of SE-R-007..010, so a payment means
			// with no cac:PayeeFinancialAccount/cbc:ID at all is out of scope rather than
			// reported.
			for _, id := range account.orNil().all("ID") {
				value := normalizeSpace(id.rawText())
				switch branch {
				case "SE:PLUSGIRO":
					if !peppolIsXPathNumber(value) {
						e.addf("SE-R-007", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be numeric", value)
					}
					if n := len([]rune(value)); n < 2 || n > 8 {
						e.addf("SE-R-010", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be two to eight "+
							"characters", value)
					}
				case "SE:BANKGIRO":
					if !peppolIsXPathNumber(value) {
						e.addf("SE-R-008", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be numeric", value)
					}
					if n := len([]rune(value)); n != 7 && n != 8 {
						e.addf("SE-R-009", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be seven or eight "+
							"characters", value)
					}
				}
			}
		}
		// SE-R-011 / SE-R-012 assert false(), so their context is the whole rule. The
		// code comparison is against the untrimmed string value here — `cbc:PaymentMeansCode
		// = normalize-space('50')` normalizes the literal, not the element — while
		// SE-R-007..010 above normalize the element. Both are transcribed as written.
		raw := code.rawText()
		if raw == "50" || raw == "56" {
			e.add("SE-R-011", "For Swedish suppliers, Bankgiro and Plusgiro are indicated with Payment means type "+
				"code 30 and a financial institution branch of SE:BANKGIRO or SE:PLUSGIRO, not with 50 or 56")
		}
		if buyerAddrSE && raw == "31" {
			e.add("SE-R-012", "For domestic Swedish transactions, a credit transfer should be indicated with Payment "+
				"means type code 30")
		}
	}
}

// peppolSwedishCIIRules is the Swedish pattern of the CII binding.
//
// SE-R-006 is the one rule whose two bindings are not the same rule. The UBL
// context is `//cac:TaxCategory[… and cbc:ID = 'S']`, restricted to the standard-
// rate category; the CII context is `//ram:ApplicableTradeTax[…] |
// //ram:CategoryTradeTax[…]` with no category predicate at all, so it applies to
// every tax group of a Swedish seller whatever its category. That is why the two
// are written out separately rather than sharing a body: a zero-rated CII line
// trips it and the UBL equivalent does not.
func peppolSwedishCIIRules(e *peppolEval, root *ciiNode) {
	seller := peppolCIIParty(root, "SellerTradeParty")
	if !peppolAnyChildValue(seller.child("PostalTradeAddress"), "CountryID", "SE") {
		return
	}
	buyerSE := peppolAnyChildValue(peppolCIIParty(root, "BuyerTradeParty").child("PostalTradeAddress"), "CountryID", "SE")
	vatID := peppolCIIVATID(seller)
	if peppolSubstring(vatID, 1, 2) == "SE" {
		if len([]rune(normalizeSpace(vatID))) != 14 {
			e.addf("SE-R-001", "A Swedish Seller VAT identifier (BT-31=%q) MUST be fourteen characters",
				normalizeSpace(vatID))
		}
		if !peppolIsXPathNumber(peppolSubstring(vatID, 3, 12)) {
			e.addf("SE-R-002", "A Swedish Seller VAT identifier (BT-31=%q) MUST carry twelve numeric characters "+
				"after the country prefix", normalizeSpace(vatID))
		}
	}
	// SE-R-003/004/013, context ram:SpecifiedLegalOrganization[… and ram:ID].
	for _, org := range seller.orNil().all("SpecifiedLegalOrganization") {
		id := org.child("ID")
		if id == nil {
			continue
		}
		raw, trimmed := id.rawText(), normalizeSpace(id.rawText())
		if !peppolIsXPathNumber(raw) {
			e.addf("SE-R-003", "A Swedish organisation number (BT-30=%q) MUST be numeric", trimmed)
		}
		if len([]rune(trimmed)) != 10 {
			e.addf("SE-R-004", "A Swedish organisation number (BT-30=%q) MUST be ten characters", trimmed)
		}
		if !peppolSwedishOrgNr(trimmed) {
			e.addf("SE-R-013", "The last digit of a Swedish organisation number (BT-30=%q) MUST satisfy the Luhn "+
				"check", trimmed)
		}
	}
	// SE-R-005, context ram:SpecifiedTaxRegistration/ram:ID[@schemeID = 'FC'], gated
	// on a legal registration identifier being present. The CII binding selects the
	// registration by scheme where UBL selects it by "not VAT".
	if len(nodesAt(seller, "SpecifiedLegalOrganization", "ID")) > 0 {
		for _, reg := range seller.orNil().all("SpecifiedTaxRegistration") {
			for _, id := range reg.all("ID") {
				if id.attr("schemeID") != "FC" {
					continue
				}
				if normalizeSpace(strings.ToUpper(id.rawText())) != "GODKÄND FÖR F-SKATT" {
					e.addf("SE-R-005", "For Swedish suppliers the Seller tax registration identifier (BT-32=%q) MUST "+
						"read \"Godkänd för F-skatt\"", normalizeSpace(id.rawText()))
				}
			}
		}
	}
	// SE-R-006 — every tax group, no category predicate.
	if peppolSubstring(vatID, 1, 2) == "SE" {
		for _, name := range []string{"ApplicableTradeTax", "CategoryTradeTax"} {
			for _, tax := range root.findAll(name) {
				peppolSwedishRate(e, tax.child("RateApplicablePercent"))
			}
		}
	}
	// SE-R-007..012, context ram:SpecifiedTradeSettlementPaymentMeans[…]. The CII
	// binding normalizes the type code in all four contexts; the UBL binding
	// normalizes it in two of them and not in the other two.
	for _, st := range nodesAt(root, "SupplyChainTradeTransaction", "ApplicableHeaderTradeSettlement") {
		for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
			code := normalizeSpace(pm.child("TypeCode").rawText())
			bic := normalizeSpace(pm.child("PayeeSpecifiedCreditorFinancialInstitution", "BICID").rawText())
			if code == "30" {
				for _, id := range pm.child("PayeePartyCreditorFinancialAccount").orNil().all("ProprietaryID") {
					value := normalizeSpace(id.rawText())
					switch bic {
					case "SE:PLUSGIRO":
						if !peppolIsXPathNumber(value) {
							e.addf("SE-R-007", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be numeric", value)
						}
						if n := len([]rune(value)); n < 2 || n > 8 {
							e.addf("SE-R-010", "A Swedish Plusgiro account identifier (BT-84=%q) MUST be two to eight "+
								"characters", value)
						}
					case "SE:BANKGIRO":
						if !peppolIsXPathNumber(value) {
							e.addf("SE-R-008", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be numeric", value)
						}
						if n := len([]rune(value)); n != 7 && n != 8 {
							e.addf("SE-R-009", "A Swedish Bankgiro account identifier (BT-84=%q) MUST be seven or "+
								"eight characters", value)
						}
					}
				}
			}
			if code == "50" || code == "56" {
				e.add("SE-R-011", "For Swedish suppliers, Bankgiro and Plusgiro are indicated with Payment means type "+
					"code 30 and a creditor financial institution of SE:BANKGIRO or SE:PLUSGIRO, not with 50 or 56")
			}
			if buyerSE && code == "31" {
				e.add("SE-R-012", "For domestic Swedish transactions, a credit transfer should be indicated with "+
					"Payment means type code 30")
			}
		}
	}
}

// peppolSwedishRate is SE-R-006's assertion:
//
//	number(<rate>) = 25 or number(<rate>) = 12 or number(<rate>) = 6
//
// An absent rate makes number() NaN, which equals nothing, so the rule reports it.
// That is the artefact's arithmetic and not a reading of intent.
func peppolSwedishRate(e *peppolEval, rate *ciiNode) {
	value, ok := parseAmount(rate.rawText())
	if ok {
		for _, allowed := range peppolSwedishVATRates {
			if value == allowed {
				return
			}
		}
	}
	e.addf("SE-R-006", "For Swedish suppliers the VAT category rate (BT-119=%q) MUST be 6, 12 or 25",
		normalizeSpace(rate.rawText()))
}

// peppolIsXPathNumber is `string(number($v)) != 'NaN'`: whether XPath's number()
// yields a number for this string. fn:number is xs:double's cast, so it takes
// surrounding whitespace, a sign and an exponent, and it maps the literal "NaN" to
// NaN — which the test then rejects.
func peppolIsXPathNumber(v string) bool {
	value, ok := parseAmount(v)
	return ok && !math.IsNaN(value)
}

// ---------------------------------------------------------------------------
// Greece
// ---------------------------------------------------------------------------

// peppolGreekDocumentTypes is $greekDocumentType, `tokenize('1.1 1.6 2.1 2.4 5.1
// 5.2 ', '\s')`. The trailing space makes the last token the empty string, which
// GR-R-001-5 cannot reach because it also requires a non-empty fourth segment.
var peppolGreekDocumentTypes = map[string]bool{
	"1.1": true, "1.6": true, "2.1": true, "2.4": true, "5.1": true, "5.2": true, "": true,
}

// peppolGreekDateRE is $dateRegExp, transcribed from the artefact's own string:
//
//	'^(0?[1-9]|[12][0-9]|3[01])[-\\/ ]?(0?[1-9]|1[0-2])[-\\/ ]?(19|20)[0-9]{2}'
//
// The separator class is four characters — hyphen, backslash, solidus, space —
// because an XPath string literal does no escape processing, so `\\` inside it is
// a regular-expression escaped backslash rather than an escaped solidus. The
// pattern has no closing anchor, and matches() is a substring search anyway.
var peppolGreekDateRE = regexp.MustCompile(`^(0?[1-9]|[12][0-9]|3[01])[-\\/ ]?(0?[1-9]|1[0-2])[-\\/ ]?(19|20)[0-9]{2}`)

// peppolGreekMARK and peppolGreekInvoiceURL are the two document descriptions the
// Greek rules key on: the MARK number the Greek tax authority assigns, and the
// invoice's published URL.
const (
	peppolGreekMARK       = "##M.AR.K##"
	peppolGreekInvoiceURL = "##INVOICE|URL##"
)

// peppolGreekUBLRules is the Greek rule set, which the CII binding does not
// publish. Nineteen identifiers across two patterns and ten contexts.
//
// The gates are $isGreekSender and $isGreekSenderandReceiver, both built on
// $supplierCountry/$customerCountry — so on the VAT prefix before the postal
// address — and both admit two codes: Greece's VAT prefix is EL and its ISO 3166
// country code is GR, and OpenPEPPOL accepts either. Two rules add a *third*
// gate on top: GR-R-004-1, GR-S-008-1, GR-R-008-2 and GR-R-004-2 require the
// seller's postal country to be literally 'GR', and GR-R-009 is gated on
// $accountingSupplierCountry, which is $supplierCountry without its tax-
// representative step. Four different country tests in one family.
func peppolGreekUBLRules(e *peppolEval, root *ciiNode) {
	greek := func(c string) bool { return c == "GR" || c == "EL" }
	if !greek(peppolUBLSupplierCountry(root)) {
		return
	}
	bothGreek := greek(peppolUBLCustomerCountry(root))
	// $accountingSupplierCountry: the seller's VAT prefix, then its postal country,
	// with no tax-representative step. GR-R-009 is the only rule gated on it.
	accountingSupplier := peppolCountryOf(
		peppolUBLVATPrefix(root.child("AccountingSupplierParty", "Party")),
		peppolUBLPostalCountry(root, "AccountingSupplierParty"),
	)
	sellerPostalGR := peppolAnyChildValue(
		root.child("AccountingSupplierParty", "Party", "PostalAddress", "Country"), "IdentificationCode", "GR")

	peppolGreekInvoiceID(e, root)

	for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
		// GR-R-002: string-length(./cac:PartyName/cbc:Name) > 0
		if len(party.child("PartyName", "Name").rawText()) == 0 {
			e.add("GR-R-002", "Greek suppliers MUST provide the Seller name as registered (BT-27)")
		}
		// GR-S-011: exactly one VAT company identifier, prefixed EL, whose remainder
		// passes the Greek TIN check.
		var vatIDs []string
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) != "VAT" {
				continue
			}
			for _, id := range ts.all("CompanyID") {
				vatIDs = append(vatIDs, id.rawText())
			}
		}
		if len(vatIDs) != 1 || peppolSubstring(vatIDs[0], 1, 2) != "EL" ||
			!peppolGreekTIN(peppolSubstring(vatIDs[0], 3)) {
			e.add("GR-S-011", "Greek suppliers should provide one Seller VAT identifier (BT-31) prefixed EL and "+
				"carrying a valid Greek TIN")
		}
		// GR-R-003, context …/cac:PartyTaxScheme[normalize-space(cac:TaxScheme/cbc:ID)
		// = 'VAT']/cbc:CompanyID: substring(., 1, 2) = 'EL' and u:TinVerification(substring(., 3))
		//
		// GR-S-011 is the same test as one warning over the party; this is one fatal
		// finding per identifier. Both are published, so both are reported.
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) != "VAT" {
				continue
			}
			for _, id := range ts.all("CompanyID") {
				if peppolSubstring(id.rawText(), 1, 2) != "EL" || !peppolGreekTIN(peppolSubstring(id.rawText(), 3)) {
					e.addf("GR-R-003", "A Greek supplier's VAT identifier (BT-31=%q) MUST begin EL and carry a valid "+
						"Greek TIN", normalizeSpace(id.rawText()))
				}
			}
		}
	}
	// GR-R-009, context cac:AccountingSupplierParty/cac:Party[$accountingSupplierCountry
	// = 'GR' or 'EL']/cbc:EndpointID: ./@schemeID = '9933' and u:TinVerification(.)
	if greek(accountingSupplier) {
		for _, party := range nodesAt(root, "AccountingSupplierParty", "Party") {
			for _, ep := range party.all("EndpointID") {
				if ep.attr("schemeID") != "9933" || !peppolGreekTIN(ep.rawText()) {
					e.addf("GR-R-009", "A Greek supplier's electronic address (BT-34=%q) MUST be a valid Greek TIN "+
						"declared under scheme 9933", normalizeSpace(ep.rawText()))
				}
			}
		}
	}
	// GR-R-005, context cac:AccountingCustomerParty[$isGreekSender]/cac:Party — gated
	// on the *sender* being Greek, not the customer.
	for _, party := range nodesAt(root, "AccountingCustomerParty", "Party") {
		if len(party.child("PartyName", "Name").rawText()) == 0 {
			e.add("GR-R-005", "Greek suppliers MUST provide the Buyer name (BT-44)")
		}
	}
	// GR-R-004-1 / GR-S-008-1 / GR-R-008-2, context the document element, gated on
	// $isGreekSender *and* the seller's postal country being literally 'GR'.
	if sellerPostalGR {
		mark := peppolGreekDocRefs(root, peppolGreekMARK)
		urls := peppolGreekDocRefs(root, peppolGreekInvoiceURL)
		if len(mark) != 1 {
			e.addf("GR-R-004-1", "A Greek supplier's invoice MUST carry exactly one MARK number as an Additional "+
				"supporting document described %q; found %d", peppolGreekMARK, len(mark))
		}
		if len(urls) != 1 {
			e.addf("GR-S-008-1", "A Greek supplier's invoice should carry exactly one invoice URL described %q; "+
				"found %d", peppolGreekInvoiceURL, len(urls))
		}
		// GR-R-008-2 is the weaker half of the same count — "no more than one" —
		// published fatal beside the advisory "exactly one".
		if len(urls) > 1 {
			e.addf("GR-R-008-2", "A Greek supplier's invoice MUST NOT carry more than one invoice URL described "+
				"%q; found %d", peppolGreekInvoiceURL, len(urls))
		}
		// GR-R-004-2, context that document reference's cbc:ID:
		//   matches(., '^[1-9]([0-9]*)')
		for _, ref := range mark {
			for _, id := range ref.all("ID") {
				if !peppolGreekPositiveInt(id.rawText()) {
					e.addf("GR-R-004-2", "A Greek MARK number (%q) MUST be a positive integer",
						normalizeSpace(id.rawText()))
				}
			}
		}
	}
	// GR-R-008-3, context cac:AdditionalDocumentReference[$isGreekSender and
	// cbc:DocumentDescription = '##INVOICE|URL##'] — this one is *not* gated on the
	// postal country, unlike the three above.
	for _, ref := range peppolGreekDocRefs(root, peppolGreekInvoiceURL) {
		if normalizeSpace(ref.child("Attachment", "ExternalReference", "URI").rawText()) == "" {
			e.add("GR-R-008-3", "A Greek supplier's invoice URL reference MUST carry the external reference URI "+
				"(BT-124)")
		}
	}
	if !bothGreek {
		return
	}
	// GR-R-006 / GR-R-010, the second Greek pattern: both parties Greek.
	for _, party := range nodesAt(root, "AccountingCustomerParty", "Party") {
		var vatIDs []string
		for _, ts := range party.all("PartyTaxScheme") {
			if normalizeSpace(ts.child("TaxScheme", "ID").rawText()) != "VAT" {
				continue
			}
			for _, id := range ts.all("CompanyID") {
				vatIDs = append(vatIDs, id.rawText())
			}
		}
		if len(vatIDs) != 1 || peppolSubstring(vatIDs[0], 1, 2) != "EL" ||
			!peppolGreekTIN(peppolSubstring(vatIDs[0], 3)) {
			e.add("GR-R-006", "For a Greek buyer, Greek suppliers MUST provide one Buyer VAT identifier (BT-48) "+
				"prefixed EL and carrying a valid Greek TIN")
		}
		for _, ep := range party.all("EndpointID") {
			if ep.attr("schemeID") != "9933" || !peppolGreekTIN(ep.rawText()) {
				e.addf("GR-R-010", "A Greek buyer's electronic address (BT-49=%q) MUST be a valid Greek TIN declared "+
					"under scheme 9933", normalizeSpace(ep.rawText()))
			}
		}
	}
}

// peppolGreekInvoiceID is GR-R-001-1..7, the seven assertions of the rule whose
// context is the document's cbc:ID.
//
// The Greek invoice number is a six-field composite separated by '|' — the
// supplier's TIN, the issue date, a serial number, a document type, and two more
// fields — and each assertion checks one field. They are seven independent
// assertions of one Schematron rule, so a malformed identifier trips several at
// once, which is what OpenPEPPOL's own fixture for a bare "06182859" declares.
func peppolGreekInvoiceID(e *peppolEval, root *ciiNode) {
	issueDate := peppolTokenize(root.child("IssueDate").rawText(), "-")
	for _, idNode := range root.all("ID") {
		segments := peppolTokenize(idNode.rawText(), "|")
		at := func(i int) string {
			if i-1 < len(segments) {
				return segments[i-1]
			}
			return ""
		}
		// GR-R-001-1: count($IdSegments) = 6
		if len(segments) != 6 {
			e.addf("GR-R-001-1", "A Greek supplier's Invoice number (BT-1=%q) MUST consist of six '|'-separated "+
				"segments; found %d", normalizeSpace(idNode.rawText()), len(segments))
		}
		// GR-R-001-2: nine characters, a valid TIN, and equal to the seller's or the
		// tax representative's VAT identifier with its EL prefix removed.
		tin := at(1)
		sellerTIN := peppolSubstring(peppolUBLVATCompanyID(root.child("AccountingSupplierParty", "Party")), 3, 9)
		repTIN := peppolSubstring(peppolUBLVATCompanyID(root.child("TaxRepresentativeParty")), 3, 9)
		if len([]rune(normalizeSpace(tin))) != 9 || !peppolGreekTIN(tin) ||
			(tin != sellerTIN && tin != repTIN) {
			e.addf("GR-R-001-2", "The first segment of a Greek Invoice number (%q) MUST be a valid TIN matching the "+
				"Seller's or the tax representative's VAT identifier", tin)
		}
		// GR-R-001-3: a date matching $dateRegExp whose three '/'-separated components
		// are the issue date's reversed.
		idDate := peppolTokenize(at(2), "/")
		dateAt := func(i int) string {
			if i-1 < len(idDate) {
				return idDate[i-1]
			}
			return ""
		}
		issueAt := func(i int) string {
			if i-1 < len(issueDate) {
				return issueDate[i-1]
			}
			return ""
		}
		if len(normalizeSpace(at(2))) == 0 || !peppolGreekDateRE.MatchString(at(2)) ||
			dateAt(1) != issueAt(3) || dateAt(2) != issueAt(2) || dateAt(3) != issueAt(1) {
			e.addf("GR-R-001-3", "The second segment of a Greek Invoice number (%q) MUST be a date equal to the "+
				"Invoice issue date (BT-2)", at(2))
		}
		// GR-R-001-4: a non-negative integer.
		//
		// The assertion's last conjunct is xs:integer($IdSegments[3]) >= 0, which raises
		// a dynamic error rather than reporting anything for a value that is numeric but
		// not an integer — so a fractional third segment is reported by nothing, here as
		// in a reference validation.
		serial := at(3)
		switch {
		case len(normalizeSpace(serial)) == 0 || !peppolIsXPathNumber(serial):
			e.addf("GR-R-001-4", "The third segment of a Greek Invoice number (%q) MUST be a positive integer", serial)
		default:
			if v, ok := parseAmount(serial); ok && v == math.Trunc(v) && v < 0 {
				e.addf("GR-R-001-4", "The third segment of a Greek Invoice number (%q) MUST be a positive integer", serial)
			}
		}
		// GR-R-001-5: one of the Greek document types.
		if len(normalizeSpace(at(4))) == 0 || !peppolGreekDocumentTypes[at(4)] {
			e.addf("GR-R-001-5", "The fourth segment of a Greek Invoice number (%q) MUST be a Greek document type "+
				"(1.1, 1.6, 2.1, 2.4, 5.1 or 5.2)", at(4))
		}
		// GR-R-001-6 / GR-R-001-7: the fifth and sixth segments are non-empty. The test
		// is string-length() with no normalization, so a segment of one space passes.
		if len(at(5)) == 0 {
			e.add("GR-R-001-6", "The fifth segment of a Greek Invoice number MUST NOT be empty")
		}
		if len(at(6)) == 0 {
			e.add("GR-R-001-7", "The sixth segment of a Greek Invoice number MUST NOT be empty")
		}
	}
}

// peppolGreekDocRefs is `cac:AdditionalDocumentReference[cbc:DocumentDescription =
// $description]` — the document references carrying one of the two Greek marker
// descriptions. The comparison is a node-set one against the untrimmed value.
func peppolGreekDocRefs(root *ciiNode, description string) []*ciiNode {
	var out []*ciiNode
	for _, ref := range root.all("AdditionalDocumentReference") {
		if peppolAnyChildValue(ref, "DocumentDescription", description) {
			out = append(out, ref)
		}
	}
	return out
}

// peppolTokenize is fn:tokenize over a single-character separator, with the one
// behaviour Go's strings.Split does not share: a zero-length input yields the empty
// sequence rather than one empty token. GR-R-001-1 counts the result, so the
// difference decides whether a document with no invoice number at all reports "found
// 0" or "found 1".
func peppolTokenize(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}

// peppolGreekPositiveInt is GR-R-004-2's `matches(., '^[1-9]([0-9]*)')` — an
// unanchored tail, so only the first character is constrained.
func peppolGreekPositiveInt(v string) bool {
	r := []rune(v)
	return len(r) > 0 && r[0] >= '1' && r[0] <= '9'
}

// peppolGreekTIN is u:TinVerification, the Greek taxpayer identification number
// check declared in PEPPOL-EN16931-UBL.sch:
//
//	$checksum = d8*2 + d7*4 + d6*8 + d5*16 + d4*32 + d3*64 + d2*128 + d1*256
//	($checksum mod 11) mod 10 = d9
//
// The function reads the first nine codepoints and ignores the rest, so a longer
// value whose first nine pass is accepted — which is the artefact's behaviour and
// the reason GR-S-011 strips the 'EL' prefix before calling it rather than after.
// A non-digit anywhere in those nine makes number() return NaN, and NaN equals
// nothing, so the check fails.
func peppolGreekTIN(v string) bool {
	r := []rune(v)
	if len(r) < 9 {
		return false
	}
	weights := [8]int{256, 128, 64, 32, 16, 8, 4, 2}
	sum := 0
	for i := 0; i < 8; i++ {
		d := int(r[i]) - '0'
		if d < 0 || d > 9 {
			return false
		}
		sum += d * weights[i]
	}
	check := int(r[8]) - '0'
	if check < 0 || check > 9 {
		return false
	}
	return (sum%11)%10 == check
}

// ---------------------------------------------------------------------------
// Iceland
// ---------------------------------------------------------------------------

// peppolIcelandicEindagi is the supporting-document description the Icelandic
// rules key on: "eindagi" is the final day for payment, carried as BT-122 on an
// Additional supporting document whose identifier is the date itself.
const peppolIcelandicEindagi = "EINDAGI"

// peppolIcelandicUBLRules is the Icelandic rule set, which the CII binding does
// not publish. Ten identifiers over two contexts, gated on the same pair of
// pattern variables the Danish family uses — a concat of the two possible roots,
// compared against 'IS' untrimmed.
//
// Iceland is the only family for which OpenPEPPOL ships no per-rule test set: there
// is no rules/unit-UBL-IS directory upstream. peppolCountryExtraCases is therefore
// where all twenty of its verdicts come from, and
// TestEveryPublishedPeppolRuleHasBothVerdicts is what says nothing else is missing.
//
// Nine of the ten are existence tests or conditionals on an element's presence, and
// the two that are neither — IS-R-006 and IS-R-007 — are both `A and B or not(C)`,
// which XPath reads as `(A and B) or not(C)`: a document with no payment means of
// that code is exempt rather than reported.
func peppolIcelandicUBLRules(e *peppolEval, root *ciiNode) {
	if peppolUBLPostalCountry(root, "AccountingSupplierParty") != "IS" {
		return
	}
	seller := root.child("AccountingSupplierParty", "Party")

	// IS-R-001: the document type code is 380 or 381, tested over whichever of the
	// two type-code elements this root carries. The `not(contains(…, ' '))` guard is
	// what stops a value like "380 381" satisfying the containment test.
	if !peppolIcelandicTypeCode(root.child("InvoiceTypeCode")) &&
		!peppolIcelandicTypeCode(root.child("CreditNoteTypeCode")) {
		e.add("IS-R-001", "If the seller is Icelandic the document type code (BT-3) should be 380 or 381")
	}
	// IS-R-002: the seller's legal registration identifier (BT-30) exists and declares
	// scheme 0196 — Iceland's kennitala.
	if !peppolIcelandicLegalID(seller) {
		e.add("IS-R-002", "If the seller is Icelandic the invoice MUST carry the Seller legal registration "+
			"identifier (BT-30) under scheme 0196")
	}
	// IS-R-003: the seller address carries a street name and a post code. Existence
	// tests, so an empty element satisfies them.
	if !peppolIcelandicAddress(seller.child("PostalAddress")) {
		e.add("IS-R-003", "If the seller is Icelandic the Seller postal address MUST carry a street name (BT-35) "+
			"and a post code (BT-38)")
	}
	// IS-R-006 / IS-R-007: a claim (payment means 9) or a transfer (42) carries a
	// twelve-character account identifier — the Icelandic bank, ledger and account
	// numbers concatenated.
	for _, tc := range []struct {
		rule, code, what string
	}{
		{"IS-R-006", "9", "a claim"},
		{"IS-R-007", "42", "a transfer"},
	} {
		var accounts []*ciiNode
		present := false
		for _, pm := range root.all("PaymentMeans") {
			if !peppolAnyChildValue(pm, "PaymentMeansCode", tc.code) {
				continue
			}
			present = true
			accounts = append(accounts, nodesAt(pm, "PayeeFinancialAccount", "ID")...)
		}
		if !present {
			continue
		}
		if len(accounts) == 0 || len([]rune(normalizeSpace(accounts[0].rawText()))) != 12 {
			e.addf(tc.rule, "If the seller is Icelandic and the Payment means type code (BT-81) is %s (%s), the "+
				"Payment account identifier (BT-84) MUST be twelve characters", tc.code, tc.what)
		}
	}
	// IS-R-008 / IS-R-009 / IS-R-010, all conditional on an EINDAGI supporting
	// document being present.
	if eindagi := peppolIcelandicEindagiRefs(root); len(eindagi) > 0 {
		// The three assertions read `<eindagi>/cbc:ID` as a node-set and take its first
		// node's string value, which is what string-length() and the comparison below
		// do.
		eindagiID := ""
		if ids := nodesAt(eindagi[0], "ID"); len(ids) > 0 {
			eindagiID = ids[0].rawText()
		}
		dueDate := root.child("DueDate")
		if len([]rune(eindagiID)) != 10 || !peppolIsCalendarDate(eindagiID) {
			e.addf("IS-R-008", "If the seller is Icelandic the eindagi (BT-122=%q) MUST be formatted YYYY-MM-DD",
				normalizeSpace(eindagiID))
		}
		if dueDate == nil {
			e.add("IS-R-009", "If the seller is Icelandic and an eindagi is present, the invoice MUST carry a Payment "+
				"due date (BT-9)")
		}
		// IS-R-010: cbc:DueDate <= <eindagi>/cbc:ID. Both operands are untyped, which
		// XPath 2.0 compares as strings — so the comparison is the lexicographic one an
		// ISO 8601 date makes correct, and an absent due date is an empty sequence,
		// which compares false and reports.
		if dueDate == nil || dueDate.rawText() > eindagiID {
			e.addf("IS-R-010", "If the seller is Icelandic the eindagi (BT-122=%q) MUST NOT precede the Payment due "+
				"date (BT-9=%q)", normalizeSpace(eindagiID), normalizeSpace(dueDate.rawText()))
		}
	}
	// IS-R-004 / IS-R-005 are a second Schematron rule, context
	// …/cac:AccountingCustomerParty, gated on both parties being Icelandic. They are
	// last because they are the only ones that need the customer's country, not
	// because anything above them is a precondition — an early return here would make
	// the three EINDAGI rules depend on the buyer being Icelandic, which they do not.
	if peppolUBLPostalCountry(root, "AccountingCustomerParty") != "IS" {
		return
	}
	for _, party := range root.all("AccountingCustomerParty") {
		if !peppolIcelandicLegalID(party.child("Party")) {
			e.add("IS-R-004", "If both parties are Icelandic the invoice MUST carry the Buyer legal registration "+
				"identifier (BT-47) under scheme 0196")
		}
		if !peppolIcelandicAddress(party.child("Party", "PostalAddress")) {
			e.add("IS-R-005", "If both parties are Icelandic the Buyer postal address MUST carry a street name "+
				"(BT-50) and a post code (BT-53)")
		}
	}
}

// peppolIcelandicEindagiRefs is `cac:AdditionalDocumentReference[cbc:DocumentDescription
// = 'EINDAGI']`.
func peppolIcelandicEindagiRefs(root *ciiNode) []*ciiNode {
	var out []*ciiNode
	for _, ref := range root.all("AdditionalDocumentReference") {
		if peppolAnyChildValue(ref, "DocumentDescription", peppolIcelandicEindagi) {
			out = append(out, ref)
		}
	}
	return out
}

// peppolIcelandicTypeCode is IS-R-001's half-assertion over one type-code element:
//
//	not(contains(normalize-space(<code>), ' ')) and
//	contains(' 380 381 ', concat(' ', normalize-space(<code>), ' '))
//
// An absent element normalizes to ”, and contains(' 380 381 ', '  ') is false, so
// a missing type code fails the half rather than passing it vacuously.
func peppolIcelandicTypeCode(code *ciiNode) bool {
	value := normalizeSpace(code.rawText())
	if strings.Contains(value, " ") {
		return false
	}
	return strings.Contains(" 380 381 ", " "+value+" ")
}

// peppolIcelandicLegalID is IS-R-002/IS-R-004's assertion: the party has a legal
// registration identifier and some one of them declares scheme 0196.
func peppolIcelandicLegalID(party *ciiNode) bool {
	ids := nodesAt(party, "PartyLegalEntity", "CompanyID")
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id.attr("schemeID") == "0196" {
			return true
		}
	}
	return false
}

// peppolIcelandicAddress is IS-R-003/IS-R-005's assertion: a street name and a post
// code are present, whatever their content.
func peppolIcelandicAddress(addr *ciiNode) bool {
	return addr.child("StreetName") != nil && addr.child("PostalZone") != nil
}

// ---------------------------------------------------------------------------
// Germany
// ---------------------------------------------------------------------------

// The German rules are the `<pattern id="german-rules">` of the UBL binding, and
// they are OpenPEPPOL's re-publication of KoSIT's XRechnung rules: DE-R-NNN carries
// the wording and, all but exactly, the test of BR-DE-NNN. xrechnung_rules.go
// evaluates the KoSIT originals on the XRechnung path, and peppolXRImports keeps
// these off it, so a German invoice validated as XRechnung is not reported twice
// under two authorities' identifiers for the same defect.
//
// Where the two artefacts have drifted, this file quotes the OpenPEPPOL copy:
//
//   - $XR-SKONTO-REGEX has no leading anchor here (KoSIT's is `(^|\r?\n)#(SKONTO)…`),
//     and matches() is a substring search, so OpenPEPPOL's DE-R-018 accepts a
//     settlement-discount line with text before the first '#'.
//   - $XR-EMAIL-REGEX is the long RFC-shaped pattern rather than KoSIT's
//     `^[^@\s]+@([^@.\s]+\.)+[^@.\s]+$`.
//   - KoSIT publishes BR-DE-23/24/25 as -a/-b and OpenPEPPOL as -1/-2.
//
// $XR-TELEPHONE-REGEX and the IBAN arithmetic of DE-R-019/020 are character for
// character KoSIT's, so validIBAN is shared rather than re-derived: one XPath, one
// implementation.

var (
	// peppolDESkontoRE is $XR-SKONTO-REGEX as OpenPEPPOL publishes it — with no
	// leading anchor, unlike KoSIT's xrSkontoRE.
	peppolDESkontoRE = regexp.MustCompile(`#(SKONTO)#TAGE=([0-9]+#PROZENT=[0-9]+\.[0-9]{2})(#BASISBETRAG=-?[0-9]+\.[0-9]{2})?#$`)
	// peppolDEEmailRE is $XR-EMAIL-REGEX. The character class holds a backtick, so
	// the pattern is assembled rather than written as one raw literal.
	peppolDEEmailRE = regexp.MustCompile(`^[a-zA-Z0-9!#\$%&"*+/=?^_` + "`" + `{|}~-]+(\.[a-zA-Z0-9!#\$%&"*+/=?^_` +
		"`" + `{|}~-]+)*@([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)
	// peppolDETelephoneRE is $XR-TELEPHONE-REGEX: at least three digits anywhere.
	peppolDETelephoneRE = regexp.MustCompile(`.*([0-9].*){3,}.*`)
)

// peppolDEVATCodes is $supportedVATCodes, the VAT category codes whose use makes
// DE-R-016's seller-tax-identifier requirement apply.
var peppolDEVATCodes = map[string]bool{
	"S": true, "Z": true, "E": true, "AE": true, "K": true, "G": true, "L": true, "M": true,
}

// peppolDETypeCodes is $supportedInvAndCNTypeCodes, DE-R-017's restricted UNTDID
// 1001 set.
var peppolDETypeCodes = map[string]bool{
	"326": true, "380": true, "384": true, "389": true, "381": true,
	"875": true, "876": true, "877": true,
}

// peppolGermanUBLRules is the german-rules pattern: 30 identifiers over ten
// contexts, every one of them gated `[$supplierCountryIsDE and $customerCountryIsDE]`.
//
//	$supplierCountryIsDE = upper-case(normalize-space(/*/cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:IdentificationCode)) = 'DE'
//	$customerCountryIsDE = the same over cac:AccountingCustomerParty
//
// Both parties, both from the postal address, both normalized and upper-cased —
// which is a third spelling of "is this document domestic", distinct from the
// Danish family's untrimmed comparison and from $supplierCountry's VAT prefix. The
// pattern's id is documentation: no <phase> selects it, so a reference validation
// runs it on every UBL document and the country test is what confines it to German
// domestic invoices.
func peppolGermanUBLRules(e *peppolEval, root *ciiNode) {
	if !peppolUBLCountryIsDE(root, "AccountingSupplierParty") ||
		!peppolUBLCountryIsDE(root, "AccountingCustomerParty") {
		return
	}
	// present is `<element>[boolean(normalize-space(.))]` — an existence test on a
	// predicated node, which is satisfied only by an element with non-blank content.
	present := func(parent *ciiNode, name string) bool {
		for _, c := range parent.orNil().all(name) {
			if normalizeSpace(c.rawText()) != "" {
				return true
			}
		}
		return false
	}

	// DE-R-001: cac:PaymentMeans — a bare existence test, so an empty group satisfies it.
	if root.child("PaymentMeans") == nil {
		e.add("DE-R-001", "A German domestic invoice shall contain Payment instructions (BG-16)")
	}
	// DE-R-015: cbc:BuyerReference[boolean(normalize-space(.))]
	if !present(root, "BuyerReference") {
		e.add("DE-R-015", "A German domestic invoice shall contain the Buyer reference (BT-10)")
	}
	peppolGermanVATIdentifier(e, root, present)
	// DE-R-017: the document type code is one of eight UNTDID 1001 values, tested over
	// whichever of the two type-code elements the root carries.
	if !peppolAnyChildIn(root, "InvoiceTypeCode", peppolDETypeCodes) &&
		!peppolAnyChildIn(root, "CreditNoteTypeCode", peppolDETypeCodes) {
		e.add("DE-R-017", "The Invoice type code (BT-3) of a German domestic invoice should be one of 326, 380, 384, "+
			"389, 381, 875, 876 or 877")
	}
	// DE-R-018: the settlement-discount ("Skonto") format in BT-20.
	peppolGermanSkonto(e, xrFirstNotes(root, "PaymentTerms", "Note"))
	// DE-R-022: attachment filenames are unique among the supporting documents.
	//
	//	count(cac:AdditionalDocumentReference) = count(cac:AdditionalDocumentReference[
	//	  not(./cac:Attachment/cbc:EmbeddedDocumentBinaryObject/@filename =
	//	      preceding-sibling::cac:AdditionalDocumentReference/…/@filename)])
	//
	// The comparison is case-sensitive even though the message says otherwise, and a
	// reference with no attachment at all compares an empty sequence with an empty
	// sequence, which is false, so it is counted rather than reported.
	var seen []string
	for _, ref := range root.all("AdditionalDocumentReference") {
		names := nodesAt(ref, "Attachment", "EmbeddedDocumentBinaryObject")
		clash := false
		for _, n := range names {
			if !n.hasAttr("filename") {
				continue
			}
			for _, previous := range seen {
				if previous == n.attr("filename") {
					clash = true
				}
			}
		}
		if clash {
			e.add("DE-R-022", "Each attached document (BT-125) of a German domestic invoice shall have a unique "+
				"file name")
		}
		for _, n := range names {
			if n.hasAttr("filename") {
				seen = append(seen, n.attr("filename"))
			}
		}
	}
	// DE-R-026: a corrected invoice (type code 384) should reference its predecessor.
	// The comparison is `= 384`, a number, so the element's value is compared
	// numerically and not as a string.
	if (peppolAnyChildNumber(root, "InvoiceTypeCode", 384) || peppolAnyChildNumber(root, "CreditNoteTypeCode", 384)) &&
		root.child("BillingReference", "InvoiceDocumentReference") == nil {
		e.add("DE-R-026", "A corrected invoice (BT-3 = 384) should carry a Preceding invoice reference (BG-3)")
	}
	// DE-R-030 / DE-R-031: a direct debit (BG-19) requires the creditor identifier
	// (BT-90) and the debited account (BT-91).
	if len(nodesAt(root, "PaymentMeans", "PaymentMandate")) > 0 {
		if !peppolGermanSEPACreditor(root) {
			e.add("DE-R-030", "A German domestic direct debit (BG-19) shall carry the Bank assigned creditor "+
				"identifier (BT-90)")
		}
		if len(nodesAt(root, "PaymentMeans", "PaymentMandate", "PayerFinancialAccount", "ID")) == 0 {
			e.add("DE-R-031", "A German domestic direct debit (BG-19) shall carry the Debited account identifier "+
				"(BT-91)")
		}
	}
	// DE-R-002, context cac:AccountingSupplierParty.
	for _, party := range root.all("AccountingSupplierParty") {
		if party.child("Party", "Contact") == nil {
			e.add("DE-R-002", "A German domestic invoice shall contain the Seller contact group (BG-6)")
		}
	}
	// DE-R-003/004, context the seller's cac:PostalAddress; DE-R-008/009, the buyer's.
	for _, tc := range []struct {
		party, city, zone, who string
	}{
		{"AccountingSupplierParty", "DE-R-003", "DE-R-004", "Seller"},
		{"AccountingCustomerParty", "DE-R-008", "DE-R-009", "Buyer"},
	} {
		for _, addr := range nodesAt(root, tc.party, "Party", "PostalAddress") {
			if !present(addr, "CityName") {
				e.addf(tc.city, "A German domestic invoice shall contain the %s city", tc.who)
			}
			if !present(addr, "PostalZone") {
				e.addf(tc.zone, "A German domestic invoice shall contain the %s post code", tc.who)
			}
		}
	}
	// DE-R-005/006/007 and DE-R-027/028, context the seller's cac:Contact.
	for _, contact := range nodesAt(root, "AccountingSupplierParty", "Party", "Contact") {
		if !present(contact, "Name") {
			e.add("DE-R-005", "A German domestic invoice shall contain the Seller contact point (BT-41)")
		}
		if !present(contact, "Telephone") {
			e.add("DE-R-006", "A German domestic invoice shall contain the Seller contact telephone number (BT-42)")
		}
		if !present(contact, "ElectronicMail") {
			e.add("DE-R-007", "A German domestic invoice shall contain the Seller contact email address (BT-43)")
		}
		if !peppolDETelephoneRE.MatchString(normalizeSpace(contact.child("Telephone").rawText())) {
			e.add("DE-R-027", "The Seller contact telephone number (BT-42) should contain at least three digits")
		}
		if !peppolDEEmailRE.MatchString(normalizeSpace(contact.child("ElectronicMail").rawText())) {
			e.add("DE-R-028", "The Seller contact email address (BT-43) should contain exactly one @ sign framed by "+
				"at least two characters on each side")
		}
	}
	// DE-R-010/011, context cac:Delivery/cac:DeliveryLocation/cac:Address.
	for _, addr := range nodesAt(root, "Delivery", "DeliveryLocation", "Address") {
		if !present(addr, "CityName") {
			e.add("DE-R-010", "A German domestic invoice shall contain the Deliver to city (BT-77) when the deliver "+
				"to address (BG-15) is present")
		}
		if !present(addr, "PostalZone") {
			e.add("DE-R-011", "A German domestic invoice shall contain the Deliver to post code (BT-78) when the "+
				"deliver to address (BG-15) is present")
		}
	}
	// DE-R-014, context cac:TaxTotal/cac:TaxSubtotal.
	for _, sub := range nodesAt(root, "TaxTotal", "TaxSubtotal") {
		if !present(sub.child("TaxCategory"), "Percent") {
			e.add("DE-R-014", "A German domestic invoice shall contain the VAT category rate (BT-119)")
		}
	}
	peppolGermanPaymentMeans(e, root)
}

// peppolGermanPaymentMeans is the three payment-means rules, whose contexts select
// on BT-81 *numerically* — `cbc:PaymentMeansCode = (30,58)` — while the two
// advisory IBAN rules inside them compare the same element with a string, `= '58'`.
// Both are transcribed as written, so a code written "058" is in the credit-transfer
// context and exempt from the IBAN check.
func peppolGermanPaymentMeans(e *peppolEval, root *ciiNode) {
	for _, pm := range root.all("PaymentMeans") {
		switch {
		case peppolAnyChildNumber(pm, "PaymentMeansCode", 30) || peppolAnyChildNumber(pm, "PaymentMeansCode", 58):
			// DE-R-019: not(cbc:PaymentMeansCode = '58') or <the IBAN check>
			if peppolAnyChildValue(pm, "PaymentMeansCode", "58") &&
				!validIBAN(pm.str("PayeeFinancialAccount", "ID")) {
				e.add("DE-R-019", "The Payment account identifier (BT-84) should be a valid IBAN when the Payment "+
					"means type code (BT-81) is 58 (SEPA credit transfer)")
			}
			if pm.child("PayeeFinancialAccount") == nil {
				e.add("DE-R-023-1", "A credit transfer (BT-81 = 30 or 58) shall carry the Credit transfer group (BG-17)")
			}
			if pm.child("CardAccount") != nil || pm.child("PaymentMandate") != nil {
				e.add("DE-R-023-2", "A credit transfer (BT-81 = 30 or 58) shall not carry the Payment card (BG-18) or "+
					"Direct debit (BG-19) group")
			}
		case peppolAnyChildNumber(pm, "PaymentMeansCode", 48) || peppolAnyChildNumber(pm, "PaymentMeansCode", 54) ||
			peppolAnyChildNumber(pm, "PaymentMeansCode", 55):
			if pm.child("CardAccount") == nil {
				e.add("DE-R-024-1", "A payment card (BT-81 = 48, 54 or 55) shall carry the Payment card group (BG-18)")
			}
			if pm.child("PayeeFinancialAccount") != nil || pm.child("PaymentMandate") != nil {
				e.add("DE-R-024-2", "A payment card (BT-81 = 48, 54 or 55) shall not carry the Credit transfer (BG-17) "+
					"or Direct debit (BG-19) group")
			}
		case peppolAnyChildNumber(pm, "PaymentMeansCode", 59):
			// DE-R-020: not(cbc:PaymentMeansCode = '59') or <the IBAN check>
			if peppolAnyChildValue(pm, "PaymentMeansCode", "59") &&
				!validIBAN(pm.str("PaymentMandate", "PayerFinancialAccount", "ID")) {
				e.add("DE-R-020", "The Debited account identifier (BT-91) should be a valid IBAN when the Payment "+
					"means type code (BT-81) is 59 (SEPA direct debit)")
			}
			if pm.child("PaymentMandate") == nil {
				e.add("DE-R-025-1", "A direct debit (BT-81 = 59) shall carry the Direct debit group (BG-19)")
			}
			if pm.child("PayeeFinancialAccount") != nil || pm.child("CardAccount") != nil {
				e.add("DE-R-025-2", "A direct debit (BT-81 = 59) shall not carry the Credit transfer (BG-17) or "+
					"Payment card (BG-18) group")
			}
		}
	}
}

// peppolGermanVATIdentifier is DE-R-016:
//
//	not( ($BT-95-UBL-Inv = $supportedVATCodes or $BT-95-UBL-CN = $supportedVATCodes)
//	     or ($BT-102 = $supportedVATCodes) or ($BT-151 = $supportedVATCodes) )
//	or (cac:TaxRepresentativeParty, $BT-31orBT-32Path)
//
// If any VAT category code in the document-level allowances and charges or on any
// line is one of the eight, the seller must be identifiable for VAT: a tax
// representative party, or a non-blank cac:PartyTaxScheme/cbc:CompanyID (BT-31 or
// BT-32). The second operand is a *sequence* of the two, so its effective boolean
// value is "either is present".
//
// $BT-95-UBL-Inv and $BT-95-UBL-CN differ only in a
// `following-sibling::cac:TaxScheme/cbc:ID = 'VAT'` predicate the second omits, and
// they are OR'd, so the first is subsumed by the second and the VAT-scheme predicate
// cannot change the outcome. Only the wider set is computed.
func peppolGermanVATIdentifier(e *peppolEval, root *ciiNode, present func(*ciiNode, string) bool) {
	used := false
	for _, ac := range root.all("AllowanceCharge") {
		indicator := ac.child("ChargeIndicator").rawText()
		if indicator != "false" && indicator != "true" {
			continue
		}
		for _, id := range nodesAt(ac, "TaxCategory", "ID") {
			if peppolDEVATCodes[id.rawText()] {
				used = true
			}
		}
	}
	for _, name := range []string{"InvoiceLine", "CreditNoteLine"} {
		for _, li := range root.all(name) {
			for _, id := range nodesAt(li, "Item", "ClassifiedTaxCategory", "ID") {
				if peppolDEVATCodes[id.rawText()] {
					used = true
				}
			}
		}
	}
	if !used {
		return
	}
	if root.child("TaxRepresentativeParty") != nil {
		return
	}
	for _, ts := range nodesAt(root, "AccountingSupplierParty", "Party", "PartyTaxScheme") {
		if present(ts, "CompanyID") {
			return
		}
	}
	e.add("DE-R-016", "A German domestic invoice using VAT category S, Z, E, AE, K, G, L or M shall carry the Seller "+
		"VAT identifier (BT-31), the Seller tax registration identifier (BT-32) or a Seller tax representative "+
		"party (BG-11)")
}

// peppolGermanSEPACreditor is DE-R-030's second operand:
//
//	cac:AccountingSupplierParty/cac:Party/cac:PartyIdentification/cbc:ID[@schemeID='SEPA']
//	| cac:PayeeParty/cac:PartyIdentification/cbc:ID[@schemeID='SEPA']
func peppolGermanSEPACreditor(root *ciiNode) bool {
	for _, path := range [][]string{
		{"AccountingSupplierParty", "Party", "PartyIdentification", "ID"},
		{"PayeeParty", "PartyIdentification", "ID"},
	} {
		for _, id := range nodesAt(root, path...) {
			if id.attr("schemeID") == "SEPA" {
				return true
			}
		}
	}
	return false
}

// peppolGermanSkonto is DE-R-018, OpenPEPPOL's copy of KoSIT's BR-DE-18.
//
// It is the same `every $line … satisfies A and B` shape xrSkontoRule implements,
// with one difference that is the whole reason it is written twice: OpenPEPPOL's
// $XR-SKONTO-REGEX has no leading anchor. matches() is a substring search, so a
// line reading "Zahlbar #SKONTO#TAGE=14#PROZENT=2.00#" satisfies OpenPEPPOL's rule
// and fails KoSIT's. Sharing one implementation would have made one of the two
// artefacts wrong.
//
// Both halves of the conjunction sit inside the quantifier, which is what makes the
// rule vacuously true for a payment term with no settlement-discount line at all.
func peppolGermanSkonto(e *peppolEval, notes []string) {
	var entryTokens []string
	for _, n := range notes {
		entryTokens = append(entryTokens, xrSkontoEntryRE.Split(n, -1)...)
	}
	tailOK := len(entryTokens) > 0 && xrSkontoTailRE.MatchString(entryTokens[len(entryTokens)-1])
	for _, n := range notes {
		for _, line := range xrSkontoLineRE.Split(n, -1) {
			line = normalizeSpace(line)
			if !strings.HasPrefix(line, "#") {
				continue
			}
			if !peppolDESkontoRE.MatchString(line) || !tailOK {
				e.add("DE-R-018", "A settlement-discount line in the Payment terms (BT-20) does not match the "+
					"required format #SKONTO#TAGE=n#PROZENT=n.nn[#BASISBETRAG=n.nn]# followed by a line break")
				return
			}
		}
	}
}

// peppolAnyChildIn is a node-set comparison against a sequence of literals:
// `<parent>/<name> = ('a', 'b', …)`, true when any such child's untrimmed string
// value is in the set.
func peppolAnyChildIn(parent *ciiNode, name string, values map[string]bool) bool {
	for _, c := range parent.orNil().all(name) {
		if values[c.rawText()] {
			return true
		}
	}
	return false
}

// peppolAnyChildNumber is a node-set comparison against a *number* literal:
// `<parent>/<name> = 30`. XPath compares numerically when one operand is a number,
// so " 30 " and "30.0" both match and "030" does too.
func peppolAnyChildNumber(parent *ciiNode, name string, want float64) bool {
	for _, c := range parent.orNil().all(name) {
		if v, ok := parseAmount(c.rawText()); ok && v == want {
			return true
		}
	}
	return false
}
