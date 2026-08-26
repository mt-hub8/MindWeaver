package formalis

import (
	"context"
	"regexp"
	"strings"
)

// This file validates the Romanian CIUS-RO (RO_CIUS / RO e-Factura, the ANAF
// national profile) on top of the EN 16931 core.
//
// The rules below are transcribed from cius-ro/RO16931-rules.sch rather than from
// the specification's prose, which is what they used to be. Reading the artefact
// changed five things:
//
//   - ANAF publishes CIUS-RO for **UBL only**. EN16931-CIUS_RO-UBL-validation.sch
//     binds the abstract model to UBL and there is no CII binding; every context in
//     RO16931-rules.sch is a UBL path. This package evaluated all twenty rules on
//     the shared syntax-neutral model, so every Factur-X/ZUGFeRD invoice was liable
//     to be accused of a Romanian rule that does not exist for its syntax — C32's
//     eight-rule defect, again.
//   - There is no BR-RO-020. ANAF publishes BR-RO-020_1, bound to
//     cbc:InvoiceTypeCode, and BR-RO-020_2, bound to cbc:CreditNoteTypeCode. This
//     package reported "BR-RO-020", an identifier no artefact carries — the same
//     class of defect as C34's two Peppol phantoms, in the emission site rather
//     than in the coverage table.
//   - BR-RO-110 and BR-RO-111 are not presence tests. Both are <report> elements
//     that fire when the party is Romanian and its country subdivision is *not* one
//     of the 42 ISO 3166-2:RO codes, which includes but is not limited to its being
//     absent. This package tested only for absence, so a Romanian address carrying
//     an invented subdivision passed.
//   - BR-RO-140/150/160/170 and BR-RO-180/201/202/211/212 are bound to
//     "/ubl:Invoice/… | /ubl:CreditNote/…", and in that file the prefix ubl is the
//     Invoice-2 namespace while a credit note's root is cn:CreditNote. The second
//     alternative therefore matches nothing, and no conforming validator applies
//     these nine rules to a credit note. That is upstream's slip and not this
//     package's to correct: a rule the authority cannot report is a rule this
//     package must not report either (D10's reasoning, applied one rule at a time).
//   - BR-RO-010 and BR-RO-030 fire on an absent term too, because their contexts are
//     the document element and normalize-space() of nothing is the empty string.
//
// All 121 identifiers version 1.0.9 publishes are flagged fatal, so the plain adder
// is right and the coverage table's fail-safe fatal turned out to be ANAF's own
// flag. cius_artefacts_test.go checks both directions.
//
// # Which of the four vendored releases this evaluates, and why not all of them
//
// ANAF has published four Schematron releases — 1.0.3, 1.0.4, 1.0.8 and 1.0.9 — and
// they are not the same rule set: 1.0.9 adds the seven BR-RO-DT* date rules, 1.0.8
// withdrew BR-RO-A999 and the two thirty-character limits BR-RO-L0301/L0309 and
// split BR-RO-020 into 020_1 and 020_2, and 1.0.4 added the credit-note branch to
// twenty contexts. This package evaluates **1.0.9**, the latest, and
// TestCIUSROVersionsDiffer pins what each release publishes so the divergence is a
// checked fact rather than a claim.
//
// Per-document dispatch is *possible* here and it was not for CIUS-PT — ANAF's
// sample instances declare two distinguishable BT-24 values, `...CIUS-RO:1.0.0` in
// the 1.0.3/1.0.4 sets and `...CIUS-RO:1.0.1` in the 1.0.8/1.0.9 ones, where every
// CIUS-PT instance declared the same identifier whatever directory it came from.
// It is deliberately not done, for two reasons. BT-24 names the *RO_CIUS* version
// and not the Schematron release: 1.0.8 and 1.0.9 both bind 1.0.1 and differ by
// nine assertions, so no document can select between them. And RO_CIUS 1.0.0 is
// superseded — e-Factura has required 1.0.1 since October 2022 — so dispatching on
// BT-24 could only make this package quieter about a document ANAF's own live
// system rejects. BR-RO-001 therefore requires the 1.0.1 identifier for every
// document, and the twenty-two vendored 1.0.0 samples report it. That is a true
// finding about a superseded document, not a false positive, and TestCIUSROCorpus
// derives it from each file's BT-24 rather than excusing it by name.
//
// # What is evaluated where
//
// The 25 BR-RO-NNN business rules are below, transcribed one at a time. The other
// 96 — the BR-RO-L* length limits, the BR-DEC-RO-* decimal limits, the BR-RO-DT*
// date formats and the BR-RO-A* occurrence limits — are generated from the
// Schematron into cius_ro_rules_table.go and run by the evaluator PR 24 built for
// CIUS-PT's datatype tier, which is the same shape of rule. Six of those 96 are
// published and unevaluable and carry Unevaluable in Coverage(SourceCIUSRO); the
// remaining 90 are evaluated.
//
// BR-27 is in ANAF's file too and is not evaluated here: it is a CEN identifier
// ANAF re-publishes, and this package reports CEN's identifiers under
// SourceEN16931 with CEN's own condition.
//
// ANAF ships a full copy of CEN's UBL binding beside its own rules, and copies can
// be edited — that is the audit's C40. This one is not: all 930 CEN identifiers in
// it carry a condition CEN published at some release, 904 of them CEN's current one
// and 26 an earlier one, and none that CEN never published. So CIUS-RO has no
// condition overrides, derived rather than assumed; see ciusCENCopyVerdicts and
// cius_overrides.go.

// roInvoiceTypeCodes is the invoice type code set BR-RO-020_1 permits, quoted from
// its test: contains(' 380 384 389 751 ', concat(' ', normalize-space(.), ' ')).
var roInvoiceTypeCodes = map[string]bool{"380": true, "384": true, "389": true, "751": true}

// roCountyCodes is the ISO 3166-2:RO country-subdivision code set CIUS-RO requires
// for Romanian addresses: the 41 counties (județe) plus Bucharest (RO-B). Taken
// verbatim from RO16931-rules.sch's $ISO-3166-RO-CODES.
var roCountyCodes = map[string]bool{
	"RO-AB": true, "RO-AG": true, "RO-AR": true, "RO-B": true, "RO-BC": true, "RO-BH": true,
	"RO-BN": true, "RO-BR": true, "RO-BT": true, "RO-BV": true, "RO-BZ": true, "RO-CJ": true,
	"RO-CL": true, "RO-CS": true, "RO-CT": true, "RO-CV": true, "RO-DB": true, "RO-DJ": true,
	"RO-GJ": true, "RO-GL": true, "RO-GR": true, "RO-HD": true, "RO-HR": true, "RO-IF": true,
	"RO-IL": true, "RO-IS": true, "RO-MH": true, "RO-MM": true, "RO-MS": true, "RO-NT": true,
	"RO-OT": true, "RO-PH": true, "RO-SB": true, "RO-SJ": true, "RO-SM": true, "RO-SV": true,
	"RO-TL": true, "RO-TM": true, "RO-TR": true, "RO-VL": true, "RO-VN": true, "RO-VS": true,
}

// roBucharestSectors is $SECTOR-RO-CODES: the city values a Bucharest (RO-B)
// address must use.
var roBucharestSectors = map[string]bool{
	"SECTOR1": true, "SECTOR2": true, "SECTOR3": true, "SECTOR4": true, "SECTOR5": true, "SECTOR6": true,
}

var roDigit = regexp.MustCompile(`[0-9]`)

// roCustomizationID is $RO-CIUS-ID of version 1.0.9: the concat() of ANAF's
// $RO-MAJOR-MINOR-PATCH-VERSION, which is '1.0.1' in 1.0.8 and 1.0.9 and '1.0.0' in
// 1.0.3 and 1.0.4. It is the one place a version literal appears in the whole rule
// set, which is what makes "evaluate the latest" a decision with a visible
// consequence rather than a silent one.
const roCustomizationID = "urn:cen.eu:en16931:2017#compliant#urn:efactura.mfinante.ro:CIUS-RO:1.0.1"

// roTaxPointDateCodes is the UNCL 2005 subset BR-RO-040 permits, quoted from its
// test: contains(' 3 35 432 ', concat(' ', normalize-space(.), ' ')).
var roTaxPointDateCodes = map[string]bool{"3": true, "35": true, "432": true}

// roVATCategories is the BT-95/BT-102/BT-151 category set BR-RO-065 and BR-RO-120
// both condition on, quoted from their tests: ('S','Z','E','AE','K','G','L','M').
var roVATCategories = map[string]bool{
	"S": true, "Z": true, "E": true, "AE": true, "K": true, "G": true, "L": true, "M": true,
}

// roRules is the generated mechanical half of ANAF's ROmodel pattern, compiled once
// at load. See cius_ro_rules_table.go and testdata/cius-ro-rules/gen.py.
var roRules = ptDTMustCompile(roRulesPattern)

// roValidateRules runs that pattern over one UBL document. It is CIUS-PT's
// ptDTValidate with CIUS-RO's pattern and CIUS-RO's document index: the two rule
// sets share an evaluator and share nothing else.
func roValidateRules(r *run, root *ciiNode, add func(rule, msg string)) int {
	return ptDTRun(roRules, r, root, &ptDTDoc{root: root, want: roRules.rootNames}, add)
}

// ValidateCIUSRO validates an invoice XML against the Romanian CIUS-RO: the
// EN 16931 core plus the CIUS-RO mandatory-term rules.
//
// The EN 16931 core accepts either syntax. The CIUS-RO rules are evaluated for a
// UBL document only, because that is the only binding ANAF publishes: a CII invoice
// is validated against the core and reported as carrying no CIUS-RO finding, which
// is what a reference CIUS-RO validator says about it too.
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
// The Report names the rule families neither rule set evaluates — the union of
// Coverage(SourceEN16931) and Coverage(SourceCIUSRO).
func ValidateCIUSRO(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceCIUSRO}, validateCIUSRO)
}

func validateCIUSRO(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	return append(out, validateCIUSRORules(r, p.inv, p.root)...)
}

func validateCIUSRORules(r *run, inv *en16931Invoice, root *ciiNode) []Violation {
	if inv.syntax != "UBL" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceCIUSRO)

	// BR-RO-001, context /ubl:Invoice | /cn:CreditNote:
	//   cbc:CustomizationID = $RO-CIUS-ID
	// A general comparison, so an absent or empty BT-24 makes it false and the
	// assertion fires — which is what a document that does not declare RO_CIUS at
	// all should get from a CIUS-RO validator.
	if roText(root.child("CustomizationID")) != roCustomizationID {
		add("BR-RO-001", "the Specification identifier (BT-24) shall be "+roCustomizationID)
	}

	// BR-RO-010, context /ubl:Invoice | /cn:CreditNote:
	//   matches(normalize-space(cbc:ID), '([0-9])')
	// An absent invoice number normalizes to "" and has no digit, so the rule fires
	// for it too; BR-1 in the core reports the absence separately.
	if !roDigit.MatchString(inv.number) {
		add("BR-RO-010", "the Invoice number (BT-1) shall contain at least one numeric character")
	}

	// BR-RO-020_1, context cbc:InvoiceTypeCode, and BR-RO-020_2, context
	// cbc:CreditNoteTypeCode. Two identifiers, one per document element, each
	// requiring a non-empty code from its own set: 380/384/389/751 for an invoice
	// and 381 for a credit note. The leading "." in each test is what makes an
	// empty element fail.
	if inv.isCreditNote {
		if root.child("CreditNoteTypeCode") != nil && inv.typeCode != "381" {
			add("BR-RO-020_2", "a Credit note type code (BT-3) shall be 381")
		}
	} else if root.child("InvoiceTypeCode") != nil && !roInvoiceTypeCodes[inv.typeCode] {
		add("BR-RO-020_1", "the Invoice type code (BT-3) shall be one of 380, 384, 389, 751")
	}

	// BR-RO-030, context /ubl:Invoice | /cn:CreditNote. The published test is a
	// four-way disjunction that reduces to "the VAT accounting currency (BT-6) is
	// RON, or the invoice currency (BT-5) is RON" — so an invoice with no
	// DocumentCurrencyCode at all fails it, which is why there is no non-empty
	// guard on inv.currency here.
	if inv.currency != "RON" && inv.taxCurrency != "RON" {
		add("BR-RO-030", "when the Invoice currency (BT-5) is not RON, the VAT accounting currency (BT-6) shall be RON")
	}

	// BR-RO-040, context cac:InvoicePeriod/cbc:DescriptionCode:
	//   not(contains(normalize-space(.), ' ')) and contains(' 3 35 432 ', concat(' ', normalize-space(.), ' '))
	// The context is a match pattern, so it is every VAT-date code in the document
	// and not only BT-8 at document level; an invoice line's own period carries one
	// too. An empty element normalises to "" and is not in the list, so it fires.
	for _, p := range root.findAll("InvoicePeriod") {
		for _, code := range p.all("DescriptionCode") {
			if !roTaxPointDateCodes[normalizeSpace(code.text)] {
				add("BR-RO-040", "the VAT date code (BT-8) shall be one of 3, 35, 432")
			}
		}
	}

	// BR-RO-065 and BR-RO-120, context /ubl:Invoice | /cn:CreditNote. One shared
	// condition and two different consequents:
	//
	//   not( (document-level allowance in a VAT category, with the VAT tax scheme)
	//     or (document-level charge in a VAT category)
	//     or (invoice line item in a VAT category) )
	//   or (the party identifiers)
	//
	// so each fires when the document uses one of the eight VAT categories and the
	// party in question carries no registration identifier. Two asymmetries in
	// ANAF's XPath are deliberate here rather than tidied: the allowance arm
	// requires cac:TaxScheme/cbc:ID = 'VAT' and the charge arm does not, and the
	// line arm is written cac:InvoiceLine, which a credit note does not have — so on
	// a credit note the line categories do not satisfy the condition at all.
	if roUsesVATCategory(root) {
		// (cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID,
		//  cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID[boolean(normalize-space(.))])
		// is a sequence, and its effective boolean value is "not empty". The first
		// arm has no non-empty predicate and the second has one, which is ANAF's
		// asymmetry and not a transcription slip.
		taxRep := len(roPath(root, "TaxRepresentativeParty", "PartyTaxScheme", "CompanyID")) > 0
		seller := roAnyNonEmpty(roPath(root, "AccountingSupplierParty", "Party", "PartyTaxScheme", "CompanyID"))
		if !taxRep && !seller {
			add("BR-RO-065", "the Seller VAT identifier (BT-31), tax registration identifier (BT-32) or tax "+
				"representative VAT identifier (BT-63) shall be provided when a VAT category is used")
		}
		legal := len(roPath(root, "AccountingCustomerParty", "Party", "PartyLegalEntity", "CompanyID")) > 0
		buyer := roAnyNonEmpty(roPath(root, "AccountingCustomerParty", "Party", "PartyTaxScheme", "CompanyID"))
		if !legal && !buyer {
			add("BR-RO-120", "the Buyer legal registration identifier (BT-47) or VAT identifier (BT-48) shall "+
				"be provided when a VAT category is used")
		}
	}

	// BR-RO-081/091/082/092, context /ubl:Invoice | /cn:CreditNote, each asserting
	// the address element exists *and* normalizes to something non-empty:
	//   cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:StreetName[boolean(normalize-space(.))]
	if inv.sellerStreet == "" {
		add("BR-RO-081", "the Seller address line 1 (BT-35) shall be provided")
	}
	if inv.sellerCity == "" {
		add("BR-RO-091", "the Seller city (BT-37) shall be provided")
	}
	if inv.buyerStreet == "" {
		add("BR-RO-082", "the Buyer address line 1 (BT-50) shall be provided")
	}
	if inv.buyerCity == "" {
		add("BR-RO-092", "the Buyer city (BT-52) shall be provided")
	}

	// BR-RO-110/111, context /ubl:Invoice | /cn:CreditNote, both <report>:
	//   IdentificationCode = 'RO' and not(normalize-space(CountrySubentity) = $ISO-3166-RO-CODES)
	// A validity test, not a presence test. An absent subdivision normalizes to ""
	// and is not in the list, so absence is one of the cases it covers.
	if inv.sellerCountry == "RO" && !roCountyCodes[inv.sellerSubentity] {
		add("BR-RO-110", "the Seller country subdivision (BT-39) shall be an ISO 3166-2:RO code for a Romanian Seller")
	}
	if inv.buyerCountry == "RO" && !roCountyCodes[inv.buyerSubentity] {
		add("BR-RO-111", "the Buyer country subdivision (BT-54) shall be an ISO 3166-2:RO code for a Romanian Buyer")
	}
	// BR-RO-100/101, same context, both <report>: Bucharest (RO-B) requires the city
	// to be one of $SECTOR-RO-CODES.
	if inv.sellerCountry == "RO" && inv.sellerSubentity == "RO-B" && !roBucharestSectors[inv.sellerCity] {
		add("BR-RO-100", "a Seller in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-37)")
	}
	if inv.buyerCountry == "RO" && inv.buyerSubentity == "RO-B" && !roBucharestSectors[inv.buyerCity] {
		add("BR-RO-101", "a Buyer in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-52)")
	}

	// And the 90 generated assertions of the mechanical half — the length, decimal,
	// date-format and occurrence limits — which are ANAF's own XPath run against the
	// same tree. They are assertions of the same <rule> elements as the rules above,
	// so neither shadows the other.
	roValidateRules(r, root, add)

	// The nine rules below are bound to "/ubl:Invoice/… | /ubl:CreditNote/…", and
	// ubl is the Invoice-2 namespace in RO16931-rules.sch, so the credit-note
	// alternative matches nothing and ANAF's own validator does not apply them to a
	// credit note. Reporting them there would be a finding no reference validator
	// produces.
	if inv.isCreditNote {
		return out
	}

	// BR-RO-140/150, context
	// /ubl:Invoice/cac:TaxRepresentativeParty/cac:PostalAddress:
	//   cbc:StreetName[boolean(normalize-space(.))] / cbc:CityName[…]
	if inv.taxRepAddressPresent {
		if inv.taxRepStreet == "" {
			add("BR-RO-140", "the Seller tax representative address line 1 (BT-64) shall be provided")
		}
		if inv.taxRepCity == "" {
			add("BR-RO-150", "the Seller tax representative city (BT-66) shall be provided")
		}
		// BR-RO-170/160, same context, both <report>.
		if inv.taxRepCountry == "RO" && !roCountyCodes[inv.taxRepSubentity] {
			add("BR-RO-170", "the Seller tax representative country subdivision (BT-68) shall be an ISO 3166-2:RO code")
		}
		if inv.taxRepCountry == "RO" && inv.taxRepSubentity == "RO-B" && !roBucharestSectors[inv.taxRepCity] {
			add("BR-RO-160", "a tax representative in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city")
		}
	}

	// BR-RO-180/201/211, context
	// /ubl:Invoice/cac:Delivery/cac:DeliveryLocation/cac:Address, each a non-empty
	// assertion; BR-RO-212/202 are <report> elements in the same context.
	if inv.deliverToPresent {
		if inv.deliverToStreet == "" {
			add("BR-RO-180", "the Deliver to address line 1 (BT-75) shall be provided")
		}
		if inv.deliverToCity == "" {
			add("BR-RO-201", "the Deliver to city (BT-77) shall be provided")
		}
		if inv.deliverToSubentity == "" {
			add("BR-RO-211", "the Deliver to country subdivision (BT-79) shall be provided")
		}
		if inv.deliverToCountry == "RO" && !roCountyCodes[inv.deliverToSubentity] {
			add("BR-RO-212", "the Deliver to country subdivision (BT-79) shall be an ISO 3166-2:RO code")
		}
		if inv.deliverToCountry == "RO" && inv.deliverToSubentity == "RO-B" && !roBucharestSectors[inv.deliverToCity] {
			add("BR-RO-202", "a delivery in Bucharest (RO-B) shall use a sector (SECTOR1-6) as the city (BT-77)")
		}
	}

	return out
}

// roText is a node's string value, untrimmed. BR-RO-001 and the two VAT-identifier
// rules compare a string value against a literal with no normalize-space() around
// it, so trimming here would accept a BT-24 that ANAF's own validator reports.
func roText(n *ciiNode) string {
	if n == nil {
		return ""
	}
	return n.text
}

// roPath walks a chain of element names allowing several children at each step,
// which is what a location path selects and what ciiNode.child (first match only)
// does not. BR-RO-065's seller may declare more than one cac:PartyTaxScheme, and
// the rule is satisfied by any of them.
func roPath(n *ciiNode, names ...string) []*ciiNode {
	cur := []*ciiNode{n}
	for _, name := range names {
		var next []*ciiNode
		for _, c := range cur {
			next = append(next, c.all(name)...)
		}
		cur = next
	}
	return cur
}

// roAnyNonEmpty is the [boolean(normalize-space(.))] predicate ANAF puts on the
// seller's and the buyer's tax-scheme identifier and on neither of the other two
// arms of the same sequence.
func roAnyNonEmpty(nodes []*ciiNode) bool {
	for _, n := range nodes {
		if normalizeSpace(n.text) != "" {
			return true
		}
	}
	return false
}

// roUsesVATCategory is the shared antecedent of BR-RO-065 and BR-RO-120:
//
//	(cac:AllowanceCharge/cac:TaxCategory/cbc:ID[ancestor::cac:AllowanceCharge/cbc:ChargeIndicator = 'false'
//	   and following-sibling::cac:TaxScheme/cbc:ID = 'VAT'] = ('S','Z','E','AE','K','G','L','M'))
//	or (cac:AllowanceCharge/cac:TaxCategory/cbc:ID[ancestor::cac:AllowanceCharge/cbc:ChargeIndicator = 'true'] = (…))
//	or (cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory/cbc:ID = (…))
//
// The paths are relative to the document element, so cac:AllowanceCharge is the
// document-level group and not a line's. ChargeIndicator is compared against the
// *strings* 'false' and 'true' rather than against false()/true(), so an indicator
// written "0" or "1" satisfies neither arm — ANAF's reading, kept.
func roUsesVATCategory(root *ciiNode) bool {
	for _, ac := range root.all("AllowanceCharge") {
		charge := roText(ac.child("ChargeIndicator"))
		if charge != "false" && charge != "true" {
			continue
		}
		for _, cat := range ac.all("TaxCategory") {
			for i, ch := range cat.children {
				if ch.name != "ID" || !roVATCategories[ch.text] {
					continue
				}
				if charge == "true" {
					return true
				}
				// The allowance arm, and only it, also requires the VAT tax
				// scheme, on a cac:TaxScheme that follows the cbc:ID.
				for _, sib := range cat.children[i+1:] {
					if sib.name == "TaxScheme" && roText(sib.child("ID")) == "VAT" {
						return true
					}
				}
			}
		}
	}
	for _, id := range roPath(root, "InvoiceLine", "Item", "ClassifiedTaxCategory", "ID") {
		if roVATCategories[id.text] {
			return true
		}
	}
	return false
}

// roSectorSubdivision reports whether s is a Bucharest sector written where an ISO
// 3166-2:RO subdivision belongs — "Sector 5" rather than "RO-B".
//
// It is not part of any rule. It used to be: roValidSubdivision accepted a sector
// as a valid subdivision, which made BR-RO-170 and BR-RO-212 more permissive than
// ANAF publishes them, and the comment justifying it said real Romanian invoices
// write Bucharest that way. They do — seven of ANAF's own eleven sample invoices
// write "Sector 5" in the buyer's BT-54 — but ANAF's Schematron reports BR-RO-111
// for exactly that, so accepting it made this package quieter than the authority
// rather than more accurate. The predicate survives in the test that accounts for
// those seven documents, and nowhere else.
func roSectorSubdivision(s string) bool {
	return roSector.MatchString(strings.TrimSpace(s))
}

var roSector = regexp.MustCompile(`(?i)^sector\s*[1-6]$`)
