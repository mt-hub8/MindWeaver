package formalis

import (
	"context"
	"math"
	"regexp"
	"strings"
)

// This file validates the Serbian CIUS (SRBDT, urn:mfin.gov.rs:srbdt, the Ministry
// of Finance e-invoice profile) on top of the EN 16931 core.
//
// The rules below are transcribed from EN16931-UBL-srbdt.sch and
// EN16931-UBL-srbdt-pdvcat-gen.sch rather than from the specification's prose,
// which is what they used to be. Reading the artefact changed four things when it
// was first vendored:
//
//   - The Ministry publishes SRBDT for **UBL only**: every context in the file is
//     "/ubl:Invoice | /cn:CreditNote" or a UBL path beneath it, and there is no CII
//     binding. This package evaluated all fourteen rules on the shared
//     syntax-neutral model — C32's eight-rule defect, again.
//   - RSR-03 is one identifier over two contexts with two different code sets:
//     cbc:InvoiceTypeCode must be 380, 383 or 386, and cbc:CreditNoteTypeCode must
//     be 381.
//   - RSR-11 and RSR-21 apply upper-case() before matching the PIB.
//   - RSR-16, RSR-17, RSR-20 and RSR-25 are exists() tests rather than non-empty
//     ones.
//
// # The fifteen rules no Schematron processor reaches
//
// Reading the *rule order* changed considerably more, and it is the reason eight
// identifiers this package used to report are gone from it.
//
// EN16931-UBL-srbdt.sch is a single <pattern id="UBL-srbdt"> holding thirty-five
// rules, and eleven of them are bound to the same context, "/ubl:Invoice |
// /cn:CreditNote". Under ISO Schematron a node is processed by the *first* rule in a
// pattern whose context it matches and by no other, so the document element goes to
// the rule carrying RSR-04 and RSR-05 and never reaches the ten that follow it. The
// same happens three more times: RSR-15 is bound to the seller endpoint context
// RSR-14 has already claimed, RSR-24 to the buyer endpoint context RSR-23 has
// claimed, and RSR-31 and RSR-32 to the tax-representative context RSR-29 has
// claimed.
//
// Fifteen of the Ministry's thirty-six RSR identifiers are therefore unreachable —
// RSR-08, 09, 10, 13, 15, 16, 17, 20, 22, 24, 25, 26, 31, 32 and 33 — and no
// conforming validator, the Ministry's own included, can report one of them. That is
// D10's definition of unevaluable, and it is the same shape as CEN's
// CII-DT-010/011/012 and the three CIUS-RO rules PR 25 recorded for exactly this
// reason.
//
// Eight of the fifteen were being reported here: RSR-09, RSR-10, RSR-13, RSR-16,
// RSR-17, RSR-20, RSR-22 and RSR-25. Removing them is a false-positive fix and not a
// coverage reduction, the same reading that removed ubl-BE-13 — a finding a
// reference validator cannot produce is a finding about nothing. They are in
// Coverage(SourceSRBDT) with Unevaluable set, and
// TestSRBDTUnevaluableRulesAreDerivedFromTheArtefact re-derives all fifteen from the
// file, so the day the Ministry splits its pattern they become gaps again.
//
// # The RSK family, and why its identifiers read RSK-X-*
//
// EN16931-UBL-srbdt-pdvcat-gen.sch is an abstract pattern, and the validation
// schema instantiates it four times — once per Serbian zero-rate VAT category, with
// <param name="PDVCATCODE" value="R"/> and its OE, SS and N siblings. So the seven
// assertions in it are evaluated four times over, twenty-eight times in all, and
// the rules below do the same.
//
// The identifiers stay RSK-X-01, RSK-X-05..10. The instantiating patterns declare a
// param per identifier — <param name="RSK-X-01" value="RSK-N-01"/> — but ISO
// Schematron substitutes $NAME occurrences, and the assertion's id attribute is the
// literal string "RSK-X-01" with no dollar sign. Only the message text carries
// "[$RSK-X-01]", so it is the *message* that reads [RSK-N-01] in the resolved
// schema and the id that stays RSK-X-01. This package reports the published id and
// names the category in the message, which is the same information in the same two
// places. TestSRBDTPDVCategoriesComeFromTheArtefact reads the four param sets out of
// the four instantiation files.
//
// # RSE-*, the family named in Coverage and nowhere else
//
// RSE-01, RSE-02 and RSE-03 are the srbdtext extension rules: two VAT-category-code
// checks on the tax subtotals inside sbt:InvoicedPrepaymentAmmount and
// sbt:ReducedTotals, and one arithmetic check that the extension's reduced total
// equals the document total less the prepaid amount. They are three ordinary
// assertions in the same pattern as the RSR family, bound to paths in the
// http://mfin.gov.rs/srbdt/srbdtext namespace. Nothing in the corpus exercises them:
// no vendored document carries an sbt:SrbDtExt at all.
//
// All 46 published identifiers are flagged fatal, so the plain adder is right.
// cius_artefacts_test.go checks both directions.

// srbdtCustomization is the specification identifier RSR-01 requires BT-24 to start
// with, and srbdtExtCustomization the one RSR-02 requires of an invoice using the
// extension.
const (
	srbdtCustomization    = "urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.rs:srbdt:2022"
	srbdtExtCustomization = srbdtCustomization + "#conformant#urn:mfin.gov.rs:srbdtext:2022"
)

// rsInvoiceTypeCodes and rsCreditNoteTypeCodes are RSR-03's two code sets, one per
// document element, quoted from its test.
var (
	rsInvoiceTypeCodes    = map[string]bool{"380": true, "383": true, "386": true}
	rsCreditNoteTypeCodes = map[string]bool{"381": true}
)

// rsTaxPointCodes is RSR-06's set: 35 (on delivery), 432 (on payment), 3 (on issue).
var rsTaxPointCodes = map[string]bool{"35": true, "432": true, "3": true}

// rsVATCategories is the VAT category code set RSR-34, RSR-35, RSR-36, RSE-01 and
// RSE-02 all share, quoted from the string ' S AE Z E O R OE SS N ' the four of them
// test membership of the same way.
var rsVATCategories = map[string]bool{
	"S": true, "AE": true, "Z": true, "E": true, "O": true,
	"R": true, "OE": true, "SS": true, "N": true,
}

// rsPDVCategories are the four VAT categories the abstract pdvcat pattern is
// instantiated for, in the order EN16931-UBL-srbdt-validation.sch includes them.
// They are the zero-rate Serbian categories: R (reduced), OE (other exempt), SS
// (special scheme) and N (not taxable).
var rsPDVCategories = []string{"R", "OE", "SS", "N"}

// rsPIB matches a Serbian tax identifier (PIB) as RSR-11, RSR-21 and RSR-30 test it:
// matches(normalize-space(upper-case(.)), '^RS[0-9]+$') with 9 or 13 digits after
// the two-character prefix. The caller upper-cases first, which the artefact does.
var rsPIB = regexp.MustCompile(`^RS(\d{9}|\d{13})$`)

// rsRegistrationNumber matches the Serbian company registration number (matični
// broj) as RSR-19 and RSR-28 test it: all digits, 8 or 13 of them.
var rsRegistrationNumber = regexp.MustCompile(`^(\d{8}|\d{13})$`)

// ValidateSRBDT validates an invoice XML against the Serbian CIUS (SRBDT): the
// EN 16931 core plus the SRBDT mandatory-term, format and VAT-category rules.
//
// The EN 16931 core accepts either syntax. The SRBDT rules are evaluated for a UBL
// document only, because that is the only binding the Ministry publishes: a CII
// invoice is validated against the core and reported as carrying no SRBDT finding,
// which is what a reference SRBDT validator says about it too.
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
// Coverage(SourceEN16931) and Coverage(SourceSRBDT). Every entry in the second is
// unevaluable, so a clean Serbian invoice reports Conformant.
func ValidateSRBDT(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceSRBDT}, validateSRBDT)
}

func validateSRBDT(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	return append(out, validateSRBDTRules(p.inv, p.root, nil)...)
}

// isVATScheme reports whether a party tax scheme group's cac:TaxScheme/cbc:ID is
// VAT, the way every SRBDT rule that asks writes it:
// normalize-space(upper-case(cac:TaxScheme/cbc:ID)) = 'VAT'.
func isVATScheme(n *ciiNode) bool {
	return strings.ToUpper(normSpace(n.str("TaxScheme", "ID"))) == "VAT"
}

// vatSchemeCompanyIDs returns the PartyTaxScheme/cbc:CompanyID nodes of a UBL party
// whose tax scheme is VAT (vat true) or is not (vat false), which is how RSR-11 and
// RSR-12 partition the same element.
func vatSchemeCompanyIDs(p *ciiNode, vat bool) []*ciiNode {
	var out []*ciiNode
	for _, pts := range p.all("PartyTaxScheme") {
		if isVATScheme(pts) != vat {
			continue
		}
		if id := pts.child("CompanyID"); id != nil {
			out = append(out, id)
		}
	}
	return out
}

// rsDecimal is xs:decimal() over an element's text, split into the three answers an
// XPath 2.0 processor gives: a number, "the element is not there" (the empty
// sequence, which compares unequal to everything), and "this is not a number",
// which raises a dynamic error and aborts the assertion rather than failing it.
//
// The distinction is the one D10 and PR 24's parser bug are both about. An
// assertion a processor aborts on reports nothing, so folding a dynamic error onto
// "false" would invent a finding the Ministry's validator does not produce.
func rsDecimal(n *ciiNode, path ...string) (val float64, present, numeric bool) {
	c := n.child(path...)
	if c == nil {
		return 0, false, true
	}
	v, ok := parseAmount(c.text)
	return v, true, ok
}

// rsAmountsEqual compares two decimals the way the rest of this package does, to
// half a cent. The Schematron compares xs:decimal exactly; a tolerance can only
// make these rules more permissive, which is the direction a false-positive oracle
// asks for, and it is what keeps a sum of two-decimal amounts from tripping on
// binary rounding.
func rsAmountsEqual(a, b float64) bool { return math.Abs(a-b) <= 0.005 }

// validateSRBDTRules applies the SRBDT rules to a UBL document, in the order the
// two Schematron files write them and skipping the rules ISO Schematron's
// first-match rule order makes unreachable. It reads the tree for the same reason
// validateCIUSPTRules does: half of these are exists() tests, and the syntax-neutral
// model cannot tell an absent element from an empty one.
//
// seen is nil on every production path; the reachability test passes a map. See
// ruleContexts.
func validateSRBDTRules(inv *en16931Invoice, root *ciiNode, seen ruleContexts) []Violation {
	if inv.syntax != "UBL" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceSRBDT)

	seller := root.child("AccountingSupplierParty", "Party").orNil()
	buyer := root.child("AccountingCustomerParty", "Party").orNil()

	// RSR-01, context cbc:CustomizationID: the specification identifier starts with
	// the SRBDT one. The context is a bare element name, so it is every
	// cbc:CustomizationID in the document rather than the root's.
	for _, c := range root.findAll("CustomizationID") {
		seen.reached("RSR-01")
		if !strings.HasPrefix(normSpace(c.text), srbdtCustomization) {
			add("RSR-01", "the Specification identifier (BT-24) shall start with "+srbdtCustomization)
		}
	}

	// RSR-02, context sbt:SrbDtExt: starts-with(normalize-space(.), …srbdtext:2022).
	//
	// normalize-space(.) of an element is its *string value* — every descendant text
	// node concatenated — not the value of the specification identifier the rule's
	// prose is about. So an extension group carrying any content at all fails this,
	// and one carrying none passes it. That reads like an upstream slip, and it is
	// not this package's to correct: reporting a rule differently from the way the
	// authority's own validator reports it is the defect C32 records eight times
	// over. Nothing in the corpus carries an sbt:SrbDtExt, so no document here is
	// affected either way.
	for _, ext := range root.findAll("SrbDtExt") {
		seen.reached("RSR-02")
		if !strings.HasPrefix(normSpace(ext.stringValue()), srbdtExtCustomization) {
			add("RSR-02", "an invoice using the srbdtext extension shall declare the specification identifier "+
				srbdtExtCustomization)
		}
	}

	// RSR-03, context cbc:InvoiceTypeCode | cbc:CreditNoteTypeCode, with the
	// permitted set chosen by self::. Two sets, not one union: 381 is a credit-note
	// code and nothing else, and 380/383/386 are invoice codes and nothing else.
	for _, tc := range root.findAll("InvoiceTypeCode") {
		seen.reached("RSR-03")
		if !rsInvoiceTypeCodes[normSpace(tc.text)] {
			add("RSR-03", "the Invoice type code (BT-3) shall be one of 380, 383, 386")
		}
	}
	for _, tc := range root.findAll("CreditNoteTypeCode") {
		seen.reached("RSR-03")
		if !rsCreditNoteTypeCodes[normSpace(tc.text)] {
			add("RSR-03", "the Credit note type code (BT-3) shall be 381")
		}
	}

	// RSR-04/05, context /ubl:Invoice | /cn:CreditNote. This is the rule that claims
	// the document element, and the ten later rules bound to the same context never
	// run because of it — see the file comment.
	seen.reached("RSR-04", "RSR-05")
	if root.child("TaxPointDate") != nil {
		add("RSR-04", "the invoice shall not contain a tax point date (BT-7); use the tax point date code (BT-8)")
	}
	if !anyChildWith(root, "InvoicePeriod", "DescriptionCode") {
		add("RSR-05", "the invoice shall contain the tax point date code (BT-8)")
	}

	// RSR-06, context /ubl:Invoice/cac:InvoicePeriod/cbc:DescriptionCode.
	for _, ip := range root.all("InvoicePeriod") {
		for _, dc := range ip.all("DescriptionCode") {
			seen.reached("RSR-06")
			if !rsTaxPointCodes[normSpace(dc.text)] {
				add("RSR-06", "the tax point date code (BT-8) shall be one of 35, 432, 3")
			}
		}
	}

	// RSR-07, context cac:BillingReference: exists(cac:InvoiceDocumentReference/cbc:IssueDate).
	for _, br := range root.findAll("BillingReference") {
		seen.reached("RSR-07")
		if !anyChildWith(br, "InvoiceDocumentReference", "IssueDate") {
			add("RSR-07", "a preceding invoice reference (BG-3) shall contain the preceding invoice issue date (BT-26)")
		}
	}

	// RSR-11, context
	// cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme[VAT]/cbc:CompanyID —
	// evaluated per element, and case-insensitively.
	for _, id := range vatSchemeCompanyIDs(seller, true) {
		seen.reached("RSR-11")
		if !rsPIB.MatchString(strings.ToUpper(normSpace(id.text))) {
			add("RSR-11", "the Seller PIB (BT-31) shall be 'RS' followed by 9 or 13 digits")
		}
	}

	// RSR-12, context
	// cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme[ID != 'VAT']:
	// the Seller's non-VAT tax scheme is the Serbian VAT-status one and nothing else.
	// An absent cbc:ID upper-cases to the empty string, which is not 'VAT', so such a
	// scheme is in the context and fails the test.
	for _, pts := range seller.all("PartyTaxScheme") {
		for _, ts := range pts.all("TaxScheme") {
			if strings.ToUpper(normSpace(ts.str("ID"))) == "VAT" {
				continue
			}
			seen.reached("RSR-12")
			if strings.ToUpper(normSpace(ts.str("ID"))) != "RS-VAT-STATUS" {
				add("RSR-12", "the Seller tax registration identifier (BT-32) shall be carried by a PartyTaxScheme "+
					"whose TaxScheme ID is 'RS-VAT-STATUS'")
			}
		}
	}

	// RSR-14, context cac:AccountingSupplierParty/cac:Party/cbc:EndpointID. RSR-13,
	// which required the endpoint to be there at all, is unreachable, so an invoice
	// with no seller endpoint reaches no rule here.
	for _, ep := range seller.all("EndpointID") {
		seen.reached("RSR-14")
		if normSpace(ep.attr("schemeID")) != "9948" {
			add("RSR-14", "the Seller electronic address (BT-34) shall use scheme identifier '9948'")
		}
	}

	// RSR-18/19, context
	// cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID: the
	// Buyer registration number carries no scheme identifier and is 8 or 13 digits.
	for _, ple := range buyer.all("PartyLegalEntity") {
		for _, id := range ple.all("CompanyID") {
			seen.reached("RSR-18", "RSR-19")
			if id.hasAttr("schemeID") {
				add("RSR-18", "the Buyer registration identifier (BT-47) shall not carry a scheme identifier")
			}
			if !rsRegistrationNumber.MatchString(normSpace(id.text)) {
				add("RSR-19", "the Buyer registration identifier (BT-47) shall be 8 or 13 digits")
			}
		}
	}

	// RSR-21, context
	// cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID — every
	// one of them, with no scheme condition, unlike the Seller's RSR-11.
	for _, pts := range buyer.all("PartyTaxScheme") {
		for _, id := range pts.all("CompanyID") {
			seen.reached("RSR-21")
			if !rsPIB.MatchString(strings.ToUpper(normSpace(id.text))) {
				add("RSR-21", "the Buyer PIB (BT-48) shall be 'RS' followed by 9 or 13 digits")
			}
		}
	}

	// RSR-23, context cac:AccountingCustomerParty/cac:Party/cbc:EndpointID.
	for _, ep := range buyer.all("EndpointID") {
		seen.reached("RSR-23")
		if normSpace(ep.attr("schemeID")) != "9948" {
			add("RSR-23", "the Buyer electronic address (BT-49) shall use scheme identifier '9948'")
		}
	}

	// RSR-27, context /ubl:Invoice/cac:PayeeParty: count(cac:PartyLegalEntity/cbc:CompanyID) = 1.
	for _, pp := range root.all("PayeeParty") {
		seen.reached("RSR-27")
		n := 0
		for _, ple := range pp.all("PartyLegalEntity") {
			n += len(ple.all("CompanyID"))
		}
		if n != 1 {
			add("RSR-27", "the Payee (BG-10) shall have exactly one Payee registration identifier (BT-61)")
		}
	}

	// RSR-28, context cac:PayeeParty/cac:PartyLegalEntity/cbc:CompanyID.
	for _, id := range root.matchPath("PayeeParty", "PartyLegalEntity", "CompanyID") {
		seen.reached("RSR-28")
		if !rsRegistrationNumber.MatchString(normSpace(id.text)) {
			add("RSR-28", "the Payee registration identifier (BT-61) shall be 8 or 13 digits")
		}
	}

	// RSR-29, context /ubl:Invoice/cac:TaxRepresentativeParty. RSR-31 and RSR-32,
	// bound to the same context after it, are unreachable.
	for _, trp := range root.all("TaxRepresentativeParty") {
		seen.reached("RSR-29")
		if !anyChildWith(trp, "PartyTaxScheme", "CompanyID") {
			add("RSR-29", "the Seller tax representative (BG-11) shall contain its PIB (BT-63)")
		}
	}

	// RSR-30, context cac:TaxRepresentativeParty/cac:PartyTaxScheme/cbc:CompanyID.
	for _, id := range root.matchPath("TaxRepresentativeParty", "PartyTaxScheme", "CompanyID") {
		seen.reached("RSR-30")
		if !rsPIB.MatchString(strings.ToUpper(normSpace(id.text))) {
			add("RSR-30", "the Seller tax representative PIB (BT-63) shall be 'RS' followed by 9 or 13 digits")
		}
	}

	// RSR-34/35/36, three contexts over one code set: the document-level
	// allowance/charge category (BT-95/BT-102), the VAT breakdown category (BT-118)
	// and the line category (BT-151). The first two are bound from the document
	// element and the third from anywhere.
	for _, ac := range root.all("AllowanceCharge") {
		for _, id := range ac.matchPath("TaxCategory", "ID") {
			seen.reached("RSR-34")
			if !rsVATCategories[normSpace(id.text)] {
				add("RSR-34", "the document level allowance or charge VAT category code (BT-95/BT-102) shall be "+
					"one of S, AE, Z, E, O, R, OE, SS, N")
			}
		}
	}
	for _, tt := range root.all("TaxTotal") {
		for _, id := range tt.matchPath("TaxSubtotal", "TaxCategory", "ID") {
			seen.reached("RSR-35")
			if !rsVATCategories[normSpace(id.text)] {
				add("RSR-35", "the VAT category code (BT-118) shall be one of S, AE, Z, E, O, R, OE, SS, N")
			}
		}
	}
	for _, id := range root.matchPath("ClassifiedTaxCategory", "ID") {
		seen.reached("RSR-36")
		if !rsVATCategories[normSpace(id.text)] {
			add("RSR-36", "the invoiced item VAT category code (BT-151) shall be one of S, AE, Z, E, O, R, OE, SS, N")
		}
	}

	out = append(out, validateSRBDTExtension(root, seen)...)
	for _, code := range rsPDVCategories {
		out = append(out, validateSRBDTVATCategory(root, code, seen)...)
	}
	return out
}

// validateSRBDTExtension applies RSE-01..03, the three rules bound to the srbdtext
// extension group.
func validateSRBDTExtension(root *ciiNode, seen ruleContexts) []Violation {
	var out []Violation
	add := adder(&out, SourceSRBDT)

	// RSE-01/02: the VAT category code of a tax subtotal inside the prepayment and
	// the reduced-totals groups, over the same code set as RSR-34/35/36.
	for _, id := range root.matchPath("SrbDtExt", "InvoicedPrepaymentAmmount", "TaxTotal", "TaxSubtotal", "TaxCategory", "ID") {
		seen.reached("RSE-01")
		if !rsVATCategories[normSpace(id.text)] {
			add("RSE-01", "the prepayment VAT category code (BT-E4) shall be one of S, AE, Z, E, O, R, OE, SS, N")
		}
	}
	for _, id := range root.matchPath("SrbDtExt", "ReducedTotals", "TaxTotal", "TaxSubtotal", "TaxCategory", "ID") {
		seen.reached("RSE-02")
		if !rsVATCategories[normSpace(id.text)] {
			add("RSE-02", "the reduced-amount VAT category code (BT-E8) shall be one of S, AE, Z, E, O, R, OE, SS, N")
		}
	}

	// RSE-03, context sbt:SrbDtExt/sbt:ReducedTotals: the extension's reduced total
	// equals the document total less the prepaid amount. Which two totals are
	// compared depends on the document element, and the artefact reads
	// TaxInclusiveAmount for an invoice and TaxExclusiveAmount for a credit note.
	isInvoice := root.name == "Invoice"
	term := "TaxInclusiveAmount"
	if !isInvoice {
		term = "TaxExclusiveAmount"
	}
	docTotal, docPresent, docNumeric := rsDecimal(root, "LegalMonetaryTotal", term)
	prepaid, prepaidPresent, prepaidNumeric := rsDecimal(root, "LegalMonetaryTotal", "PrepaidAmount")
	for _, rt := range root.matchPath("SrbDtExt", "ReducedTotals") {
		seen.reached("RSE-03")
		got, gotPresent, gotNumeric := rsDecimal(rt, "LegalMonetaryTotal", term)
		if !gotNumeric || !docNumeric || !prepaidNumeric {
			// A dynamic error: xs:decimal() over text that is not a number aborts the
			// assertion, and an aborted assertion reports nothing.
			continue
		}
		if gotPresent && docPresent && prepaidPresent && rsAmountsEqual(got, docTotal-prepaid) {
			continue
		}
		add("RSE-03", "the reduced total (BT-E10) shall equal the invoice total ("+term+
			") less the prepaid amount (BT-113)")
	}
	return out
}

// validateSRBDTVATCategory is one instantiation of the abstract pdvcat pattern, for
// the VAT category code the validation schema passes it as $PDVCATCODE.
//
// The identifiers are the abstract pattern's own — see the file comment on why they
// stay RSK-X-* rather than becoming RSK-R-*/RSK-N-* — so the category is in the
// message, which is where the artefact puts it too.
func validateSRBDTVATCategory(root *ciiNode, code string, seen ruleContexts) []Violation {
	var out []Violation
	add := adder(&out, SourceSRBDT)
	in := func(rule, msg string) { add(rule, "["+code+"] "+msg) }

	// A VAT category group of this code, under a VAT tax scheme. Both spellings the
	// pattern uses: cac:TaxCategory anywhere (breakdowns and allowance/charges) and
	// cac:ClassifiedTaxCategory anywhere (lines).
	usedBy := func(name string) bool {
		for _, tc := range root.findAll(name) {
			if isVATScheme(tc) && normSpace(tc.str("ID")) == code {
				return true
			}
		}
		return false
	}

	// RSK-X-01, context /ubl:Invoice | /cn:CreditNote: a document that uses this
	// category anywhere shall carry exactly one document-level VAT breakdown for it,
	// and a document that uses it nowhere shall carry none.
	seen.reached("RSK-X-01")
	breakdowns := 0
	for _, tt := range root.all("TaxTotal") {
		for _, tc := range tt.matchPath("TaxSubtotal", "TaxCategory") {
			if isVATScheme(tc) && normSpace(tc.str("ID")) == code {
				breakdowns++
			}
		}
	}
	if used := usedBy("TaxCategory") || usedBy("ClassifiedTaxCategory"); used != (breakdowns == 1) {
		in("RSK-X-01", "an invoice using VAT category code '"+code+
			"' on a line, allowance or charge shall contain exactly one VAT breakdown (BG-23) with that code")
	}

	// RSK-X-05, context cac:InvoiceLine/cac:Item/cac:ClassifiedTaxCategory[ID=$code][VAT]:
	// the line VAT rate (BT-152) is zero. An absent cbc:Percent is the empty
	// sequence, which equals nothing, so it fails.
	for _, line := range []string{"InvoiceLine", "CreditNoteLine"} {
		for _, ctc := range root.matchPath(line, "Item", "ClassifiedTaxCategory") {
			if normSpace(ctc.str("ID")) != code || !isVATScheme(ctc) {
				continue
			}
			seen.reached("RSK-X-05")
			if v, present, numeric := rsDecimal(ctc, "Percent"); numeric && !(present && v == 0) {
				in("RSK-X-05", "an invoice line with VAT category code '"+code+
					"' shall have a VAT rate (BT-152) of zero")
			}
		}
	}

	// RSK-X-06/07, contexts cac:AllowanceCharge[ChargeIndicator=false|true]/
	// cac:TaxCategory[ID=$code][VAT]: the allowance and charge VAT rates are zero.
	// The contexts are relative, so a line-level allowance or charge that carries a
	// VAT category is in them too.
	for _, ac := range root.findAll("AllowanceCharge") {
		charge := strings.EqualFold(normSpace(ac.str("ChargeIndicator")), "true")
		rule, what := "RSK-X-06", "a document level allowance"
		if charge {
			rule, what = "RSK-X-07", "a document level charge"
		}
		for _, tc := range ac.all("TaxCategory") {
			if normSpace(tc.str("ID")) != code || !isVATScheme(tc) {
				continue
			}
			seen.reached(rule)
			if v, present, numeric := rsDecimal(tc, "Percent"); numeric && !(present && v == 0) {
				in(rule, what+" with VAT category code '"+code+"' shall have a VAT rate of zero")
			}
		}
	}

	// RSK-X-08/09/10, context /*/cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[ID=$code][VAT]:
	// the breakdown's taxable amount balances, its tax amount is zero, and it states
	// an exemption reason.
	for _, tt := range root.all("TaxTotal") {
		for _, tc := range tt.matchPath("TaxSubtotal", "TaxCategory") {
			if normSpace(tc.str("ID")) != code || !isVATScheme(tc) {
				continue
			}
			seen.reached("RSK-X-08", "RSK-X-09", "RSK-X-10")
			out = append(out, srbdtBreakdownSums(root, tt, tc, code)...)
			// RSK-X-09 reads ../cbc:TaxAmount: the amount of the cac:TaxSubtotal this
			// category belongs to, not the cac:TaxTotal's.
			if v, present, numeric := subtotalDecimal(tt, tc, "TaxAmount"); numeric && !(present && v == 0) {
				in("RSK-X-09", "the VAT category tax amount (BT-117) for category '"+code+"' shall be zero")
			}
			if tc.child("TaxExemptionReason") == nil && tc.child("TaxExemptionReasonCode") == nil {
				in("RSK-X-10", "a VAT breakdown (BG-23) with category code '"+code+
					"' shall carry a VAT exemption reason code (BT-121) or text (BT-120)")
			}
		}
	}
	return out
}

// subtotalDecimal reads a decimal from the cac:TaxSubtotal that holds tc — the
// artefact's `../cbc:TaxableAmount` and `../cbc:TaxAmount`, where the context node
// is the cac:TaxCategory inside it.
func subtotalDecimal(taxTotal, tc *ciiNode, name string) (float64, bool, bool) {
	for _, ts := range taxTotal.all("TaxSubtotal") {
		for _, c := range ts.all("TaxCategory") {
			if c == tc {
				return rsDecimal(ts, name)
			}
		}
	}
	return 0, false, true
}

// srbdtBreakdownSums is RSK-X-08: the taxable amount of this category's VAT
// breakdown equals the sum of the line net amounts carrying it, plus document
// charges, less document allowances.
//
// The artefact's two arms are transcribed as written, hard-coded 'E' included. The
// invoice arm filters lines by $PDVCATCODE and the allowance/charge groups by the
// literal 'E'; the credit-note arm filters *everything* by 'E'. That is an upstream
// slip — the file was plainly written for category E and only partly parameterised
// — and correcting it here would mean reporting a rule the Ministry's own validator
// does not report, which is the defect C32 records eight times over.
func srbdtBreakdownSums(root, taxTotal, tc *ciiNode, code string) []Violation {
	var out []Violation
	add := adder(&out, SourceSRBDT)

	basis, basisPresent, basisNumeric := subtotalDecimal(taxTotal, tc, "TaxableAmount")
	if !basisNumeric {
		return nil
	}
	// sumLines totals the LineExtensionAmount of every line of the given element
	// name whose item carries category cat. A non-numeric amount is a dynamic error.
	sumLines := func(name, cat string) (float64, bool) {
		total := 0.0
		for _, ln := range root.all(name) {
			match := false
			for _, ctc := range ln.matchPath("Item", "ClassifiedTaxCategory") {
				if normSpace(ctc.str("ID")) == cat {
					match = true
				}
			}
			if !match {
				continue
			}
			v, present, numeric := rsDecimal(ln, "LineExtensionAmount")
			if !numeric {
				return 0, false
			}
			if present {
				total += v
			}
		}
		return total, true
	}
	// sumAllowanceCharges totals the Amount of every document-level allowance
	// (charge false) or charge (charge true) whose VAT category is cat.
	sumAllowanceCharges := func(charge bool, cat string) (float64, bool) {
		total := 0.0
		for _, ac := range root.all("AllowanceCharge") {
			if strings.EqualFold(normSpace(ac.str("ChargeIndicator")), "true") != charge {
				continue
			}
			match := false
			for _, c := range ac.all("TaxCategory") {
				if normSpace(c.str("ID")) == cat {
					match = true
				}
			}
			if !match {
				continue
			}
			v, present, numeric := rsDecimal(ac, "Amount")
			if !numeric {
				return 0, false
			}
			if present {
				total += v
			}
		}
		return total, true
	}

	charges, ok1 := sumAllowanceCharges(true, "E")
	allowances, ok2 := sumAllowanceCharges(false, "E")
	if !ok1 || !ok2 {
		return nil
	}
	balanced := false
	if len(root.findAll("InvoiceLine")) > 0 {
		lines, ok := sumLines("InvoiceLine", code)
		if !ok {
			return nil
		}
		balanced = basisPresent && rsAmountsEqual(basis, lines+charges-allowances)
	}
	if !balanced && len(root.findAll("CreditNoteLine")) > 0 {
		lines, ok := sumLines("CreditNoteLine", "E")
		if !ok {
			return nil
		}
		balanced = basisPresent && rsAmountsEqual(basis, lines+charges-allowances)
	}
	if !balanced {
		add("RSK-X-08", "["+code+"] the VAT breakdown taxable amount (BT-116) for category '"+code+
			"' shall equal the sum of the line net amounts carrying it, plus document charges, less document allowances")
	}
	return out
}

// anyChildWith reports whether any direct child of n named group has a descendant
// path leaf — the shape "exists(cac:X/cbc:Y)" takes when X may repeat.
//
// ciiNode.child follows the *first* match at every step, so
// child("PartyTaxScheme", "CompanyID") answers nil for a party whose first tax
// scheme carries no CompanyID and whose second does. An XPath existence test does
// not: exists(cac:PartyTaxScheme/cbc:CompanyID) is true if any of them has one.
func anyChildWith(n *ciiNode, group string, leaf ...string) bool {
	for _, g := range n.all(group) {
		if g.child(leaf...) != nil {
			return true
		}
	}
	return false
}
