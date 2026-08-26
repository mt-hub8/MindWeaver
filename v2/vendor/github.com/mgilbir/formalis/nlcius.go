package formalis

import (
	"context"
	"fmt"
	"strings"
)

// This file validates the Dutch NLCIUS (SimplerInvoicing / SI-UBL) Core Invoice
// Usage Specification on top of the EN 16931 core. NLCIUS is the one CIUS in this
// package whose authority publishes **both** syntax bindings —
// si-ubl-2.0/si-ubl-2.0-nlcius.sch and nlcius-cii/NLCIUS-CII-validation.sch — and
// the two are not the same rule set.
//
// SimplerInvoicing also *includes* CEN's Schematron from a copy of its own — a
// CenPC434/ directory beside each binding, and a third copy under the G-account
// extension whose abstract syntax file is even named EN16931-syntax-modified.sch.
// Those copies were unfetched until PR 27 and so unmeasured, which left open whether
// this package was reporting CEN's reading of a rule where SimplerInvoicing reports
// its own (the audit's C40). They are fetched and read now: all three copies are
// older releases of CEN's files — SI-UBL's still publishes BR-IG-*/BR-IP-* rather
// than BR-AF-*/BR-AG-* — and every CEN *condition* in every one of them is a
// condition CEN published at some commit. None is Dutch. So NLCIUS overrides no CEN
// condition, and that is derived rather than assumed; see ciusCENCopyVerdicts.
//
// What that measurement cannot see, and what PR 28 found by reading the file rather
// than diffing conditions, is a CEN rule the copy **removes**. EN16931-syntax-modified.sch
// — the G-account extension's replacement for CEN's abstract syntax file — comments
// out UBL-CR-411, UBL-CR-453 and UBL-CR-459 and says so in its header, and a
// commented-out assertion is not an identifier a condition survey compares. So the
// "modified" in that file's name is SimplerInvoicing's edit after all; an earlier
// version of this comment said it was CEN's. See nlcius_gaccount.go.
//
// The rules below are transcribed from those two files rather than from prose,
// which is what they used to be. Reading the artefacts changed four things:
//
//   - The gate is not one condition, it is two, and the file names them $si and $s.
//     $si is "this document declares the NLCIUS customization identifier"; $s is
//     "$si *and* the supplier is in the Netherlands". This package gated every
//     BR-NL rule on the supplier being Dutch and nothing else, which is wrong in
//     both directions: BR-NL-13 is bound to $si, so it applies to a foreign
//     supplier's NLCIUS invoice and was not evaluated for one; and no rule applied
//     to a document that is not an NLCIUS invoice at all, which this package
//     checked nowhere, so a Peppol BIS invoice from a Dutch seller collected
//     findings a reference NLCIUS validator does not report. PR 21 found the same
//     shape in OpenPEPPOL's Dutch rules, where five different gates were being read
//     as one.
//   - BR-NL-10 accepts a KVK (scheme 0106) *or an OIN* (scheme 0190) for the
//     customer, exactly as BR-NL-1 does for the supplier. This package accepted only
//     the KVK, so a Dutch public body identified by its OIN — the identifier the
//     Dutch government issues for precisely this purpose — was reported as
//     non-conformant.
//   - BR-NL-11 carries no exemption for credit notes. This package excused type
//     code 381 from needing a means of payment, which the artefact does not.
//   - BR-NL-11's two bindings do not test the same thing. The UBL binding asserts
//     "xs:decimal(cbc:PayableAmount) <= 0.0 or (//cac:PaymentMeans)"; the CII one
//     asserts "xs:decimal(ram:DuePayableAmount) >= 0.0 or (…PaymentMeans/TypeCode)",
//     whose first disjunct is true for every non-negative amount. So in CII the rule
//     only fires on a *negative* amount with no payment means, and this package
//     fired it on a positive one — a finding no NLCIUS-CII validator produces.
//
// BR-NL-8 is published in the UBL binding only (it asserts that the type code
// agrees with the UBL document element, a question CII does not have), and
// BR-NL-22/23 in the CII binding only. cius_artefacts_test.go pins the difference
// between the two bindings, so a rule cannot quietly start firing in a syntax that
// does not publish it.
//
// Severity needed no correction: the twelve fatal identifiers are flagged fatal in
// both bindings and the 22 advisory ones warning, which is what
// Coverage(SourceNLCIUS) already said.
//
// # The advisory tier
//
// The "not recommended" rules — BR-NL-19 to BR-NL-35, in the numbered sub-rule form
// the artefacts actually publish — are evaluated now, as warnings. They are the
// most mechanical rules in either binding: all but four are `test="false"` in a
// context, which is the Schematron way of writing "this element should not be
// here", and the four that are not test one attribute each. NLCIUS was the last
// rule set in this package whose gap was advisory-only, so closing it is what makes
// Report.Complete() true for a Dutch invoice.
//
// # The rules neither binding's own validator can report
//
// Reading the two files as ISO Schematron rather than as a list changed four
// things, and one of them was a live false positive.
//
// A node is processed by the first rule in a pattern whose context matches it and
// by no other. Both bindings put every BR-NL rule in one pattern, and four rules
// repeat a context an earlier rule in that pattern has already claimed:
//
//   - CII: BR-NL-9's rule repeats BR-NL-7's context /*/rsm:ExchangedDocument/
//     ram:TypeCode[$si], and BR-NL-31's repeats BR-NL-12's context
//     ram:SpecifiedTradeSettlementPaymentMeans[$s]. **This package reported BR-NL-9
//     on CII documents**, which no NLCIUS-CII validator can do — the same defect as
//     C36, found the same way. BR-NL-9 is evaluated for UBL, where its rule is
//     reachable, and for CII it is in Coverage(SourceNLCIUS) as unevaluable.
//   - UBL: BR-NL-32-2 and BR-NL-32-3 are bound to cac:InvoiceLine/ and
//     cac:CreditNoteLine/cac:AllowanceCharge/cbc:AllowanceChargeReasonCode, both of
//     which BR-NL-32-1's context cac:AllowanceCharge/cbc:AllowanceChargeReasonCode
//     already matches — an XSLT match pattern is anchored at its last step, so the
//     shorter path claims the line-level nodes too. So BR-NL-32-1 is the identifier
//     that reports an allowance or charge reason code at *either* level, and its two
//     siblings report nothing.
//
// # The rest of what these two files publish
//
// The BR-NL-* family is not the whole of either binding. si-ubl-2.0-nlcius.sch ends
// with one more rule, SI-UBL-2 ("Document should not contain empty elements"), and
// NLCIUS-CII-validation.sch ends with the same rule under the identifier
// empty-element-check. Both are evaluated now — see nlciusEmptyElements, which is
// where the two decisions they need are argued. The UBL binding also carries the
// eight BR-GA-* rules of the G-account extension, in a file of their own — see
// nlcius_gaccount.go.
//
// None of the three families was visible to any guard in this repository until PR
// 28, because ciusArtefacts decided which published identifiers were NLCIUS's with
// the pattern ^BR-NL-. That is C39's defect in a second authority's artefact, and the
// fix is in cius_artefacts_test.go: every identifier a vendored file publishes is
// now enumerated and then classified, and one that no classifier accounts for fails
// the build.
//
// That also settles the BR-NL-34 curiosity PR 22 recorded. The UBL file carries a
// second trio of assertions whose message text reads "[BR-NL-34]" — the charge
// wording, against BR-NL-32's allowance wording — under the same three identifiers
// and against the same three contexts, later in the same pattern. Every one of them
// is unreachable, so BR-NL-34 is not an identifier this package fails to emit: it is
// a message the authority's own validator never prints. The CII binding says the
// same thing in one assertion and gives it the honest identifier BR-NL-32-and-34.

// nlciusPaymentMeans is the payment means code set BR-NL-12 permits: 30 credit
// transfer, 48 bank card, 49 direct debit, 57 standing agreement, 58 SEPA credit
// transfer, 59 SEPA direct debit.
var nlciusPaymentMeans = map[string]bool{"30": true, "48": true, "49": true, "57": true, "58": true, "59": true}

// nlciusTypeCodes is the invoice type code set BR-NL-7 permits.
var nlciusTypeCodes = map[string]bool{"380": true, "381": true, "384": true, "389": true}

// nlciusCustomization is the substring both bindings test the specification
// identifier for. The UBL binding writes
// contains($customizationID, '#compliant#urn:fdc:nen.nl:nlcius:v1.0') and the CII
// one compares for equality against that identifier and against its
// "#conformant#urn:fdc:nen.nl:gaccount:v1.0" extension; the containment test covers
// both, and is the one the authority itself uses in the syntax where the extension
// is expressed as a suffix.
const nlciusCustomization = "#compliant#urn:fdc:nen.nl:nlcius:v1.0"

// nlciusApplies is $si: this document declares itself an NLCIUS invoice.
//
// It is a real condition and not a formality. Both bindings put it on every rule,
// including the ones that are not otherwise about the Netherlands, so a document
// that does not declare the customization identifier collects no BR-NL finding from
// a reference validator — it collects SI-V20-INV-R000 instead, which is the
// wrapper's way of saying it was handed the wrong document. ValidateNLCIUS reports
// nothing rather than that, because a caller who asked for NLCIUS validation has
// said what they think the document is and the core rules still ran.
func nlciusApplies(inv *en16931Invoice) bool {
	return strings.Contains(inv.specID, nlciusCustomization)
}

// ValidateNLCIUS validates an invoice XML against the Dutch NLCIUS (SimplerInvoicing)
// CIUS: the EN 16931 core plus the NLCIUS-specific rules. It accepts either syntax,
// and it is the one CIUS in this package of which that is true because its
// authority publishes both bindings rather than because this package assumed so.
//
// The BR-NL rules apply to a document that declares the NLCIUS customization
// identifier, and all but BR-NL-13 additionally require the supplier to be in the
// Netherlands. Those are the artefact's own two conditions, $si and $s.
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
// The Report names the rule families neither rule set evaluates. NLCIUS is the
// one CIUS here whose fatal rules are implemented in full; its entry in the
// coverage table is the advisory half alone. The EN 16931 core it runs on is
// another matter — see Coverage(SourceEN16931).
func ValidateNLCIUS(ctx context.Context, xmlData []byte) (Report, error) {
	return modelValidate(ctx, xmlData, []Source{SourceEN16931, SourceNLCIUS}, validateNLCIUS)
}

func validateNLCIUS(r *run, p *parsed) []Violation {
	out := validateEN16931(r, p, ProfileEN16931, ciiBindingCEN)
	// A document that declares the G-account extension is judged by the extension's
	// copy of CEN's abstract syntax file, which removes three advisory rules the
	// extension's own data model requires a document to break. See
	// nlciusGAccountRemovedCEN.
	if nlciusGAccountApplies(p) {
		out = dropRemovedGAccountRules(out)
	}
	return append(out, validateNLCIUSRules(p, nil)...)
}

// validateNLCIUSRules applies the NLCIUS rules, in the binding that publishes each
// and behind the gate that publishes it.
//
// seen is nil on every production path; the reachability test passes a map. See
// ruleContexts.
func validateNLCIUSRules(p *parsed, seen ruleContexts) []Violation {
	inv := p.inv
	// The G-account extension first, and outside $si, because its pattern carries no
	// gate at all: the rules apply to whatever document the extension's Schematron is
	// pointed at. What decides that here is nlciusGAccountApplies, and one of its two
	// arms — an invoice carrying the GACCOUNT payment instruction without declaring
	// the extension — is precisely a document $si is false for. See
	// nlcius_gaccount.go.
	out := validateNLCIUSGAccount(p, seen)

	// The empty-element rule, also outside $si, and for the same kind of reason:
	// it is the one rule in either binding whose context carries no gate.
	out = append(out, nlciusEmptyElements(p, seen)...)

	// $si. Everything below is inside it.
	if !nlciusApplies(inv) {
		return out
	}
	add := adder(&out, SourceNLCIUS)

	// BR-NL-13 is gated on $si alone, in both bindings: an order line reference
	// (BT-132) requires a document-level order reference (BT-13) whatever country
	// the supplier is in. It sits before the $s guard for that reason.
	if inv.hasOrderLineRef {
		seen.reached("BR-NL-13")
	}
	if inv.hasOrderLineRef && inv.orderRef == "" {
		add("BR-NL-13", "an order line reference (BT-132) requires a document-level Purchase order reference (BT-13)")
	}

	// BR-NL-7 is gated on $si in the CII binding — context
	// /*/rsm:ExchangedDocument/ram:TypeCode[$si] — and on $s in the UBL one, where
	// the context is cbc:InvoiceTypeCode[$s]|cbc:CreditNoteTypeCode[$s]. The same
	// identifier, two gates.
	if inv.syntax == "CII" {
		seen.reached("BR-NL-7")
		if !nlciusTypeCodes[inv.typeCode] {
			add("BR-NL-7", fmt.Sprintf("the invoice type code (BT-3=%q) must be one of 380, 381, 384, 389", inv.typeCode))
		}
	}

	// $s: everything below additionally requires the supplier to be in the
	// Netherlands.
	if inv.sellerCountry != "NL" {
		return out
	}

	// BR-NL-1: the supplier must identify its legal entity with a KVK (scheme
	// 0106) or OIN (scheme 0190) number.
	seen.reached("BR-NL-1", "BR-NL-2", "BR-NL-3", "BR-NL-4", "BR-NL-5", "BR-NL-10", "BR-NL-11")
	if !((inv.sellerLegalScheme == "0106" || inv.sellerLegalScheme == "0190") && inv.sellerLegalReg != "") {
		add("BR-NL-1", "the Seller legal registration identifier (BT-30) must be a KVK (scheme 0106) or OIN (scheme 0190) number")
	}

	// BR-NL-2: the invoice must carry a buyer reference (BT-10) or a purchase
	// order reference (BT-13).
	if inv.buyerReference == "" && inv.orderRef == "" {
		add("BR-NL-2", "the invoice must contain a Buyer reference (BT-10) or a Purchase order reference (BT-13)")
	}

	// BR-NL-3: the Seller postal address must contain a street (BT-35), city
	// (BT-37) and post code (BT-38).
	if inv.sellerStreet == "" || inv.sellerCity == "" || inv.sellerPostCode == "" {
		add("BR-NL-3", "the Seller postal address must contain a street (BT-35), city (BT-37) and post code (BT-38)")
	}

	// BR-NL-4: a Dutch Buyer's postal address must contain a street, city and post code.
	if inv.buyerCountry == "NL" && (inv.buyerStreet == "" || inv.buyerCity == "" || inv.buyerPostCode == "") {
		add("BR-NL-4", "a Dutch Buyer postal address must contain a street (BT-50), city (BT-52) and post code (BT-53)")
	}

	// BR-NL-5: a Dutch tax representative's postal address must contain a street,
	// city and post code.
	if inv.taxRepCountry == "NL" && (inv.taxRepStreet == "" || inv.taxRepCity == "" || inv.taxRepPostCode == "") {
		add("BR-NL-5", "a Dutch tax representative postal address must contain a street, city and post code")
	}

	// BR-NL-7/8/9, UBL binding: one Schematron rule with three assertions, on the
	// context cbc:InvoiceTypeCode[$s]|cbc:CreditNoteTypeCode[$s]. BR-NL-7 is gated
	// on $s here and on $si in CII, above; BR-NL-8 and BR-NL-9 are UBL-only, the
	// first because CII has no second document element to disagree with and the
	// second because the CII file binds it to a context BR-NL-7's rule has already
	// claimed. See the file comment.
	if inv.syntax != "CII" {
		seen.reached("BR-NL-7", "BR-NL-8", "BR-NL-9")
		if !nlciusTypeCodes[inv.typeCode] {
			add("BR-NL-7", fmt.Sprintf("the invoice type code (BT-3=%q) must be one of 380, 381, 384, 389", inv.typeCode))
		}
		// BR-NL-8: the type code must match the UBL document element — 381 (credit
		// note) belongs to a CreditNote, every other permitted code to an Invoice.
		if inv.isCreditNote && inv.typeCode != "381" {
			add("BR-NL-8", fmt.Sprintf("a CreditNote document must use type code 381, not %q", inv.typeCode))
		} else if !inv.isCreditNote && inv.typeCode == "381" {
			add("BR-NL-8", "type code 381 (credit note) must not be used in an Invoice document")
		}
		// BR-NL-9: a corrective invoice (type 384) must reference the preceding
		// invoice it corrects (BT-25).
		if inv.typeCode == "384" && !(inv.hasBillingRef && !inv.billingRefNoID) {
			add("BR-NL-9", "a corrective invoice (type 384) must contain a Preceding Invoice reference (BT-25)")
		}
	}

	// BR-NL-10: a Dutch Buyer must identify its legal entity with a KVK (0106) or
	// an OIN (0190). The OIN half was missing, and an OIN is what every Dutch
	// public body uses.
	if inv.buyerCountry == "NL" && !((inv.buyerLegalScheme == "0106" || inv.buyerLegalScheme == "0190") && inv.buyerLegalReg) {
		add("BR-NL-10", "a Dutch Buyer legal registration identifier (BT-47) must be a KVK (scheme 0106) or OIN (scheme 0190) number")
	}

	// BR-NL-11: the invoice must state a means of payment. The two bindings differ,
	// and the difference is not a nuance:
	//
	//   UBL  xs:decimal(cbc:PayableAmount) <= 0.0 or (//cac:PaymentMeans)
	//   CII  xs:decimal(ram:DuePayableAmount) >= 0.0 or (…PaymentMeans/ram:TypeCode)
	//
	// so in UBL the rule applies to a positive amount due and in CII to a negative
	// one. That reads like an upstream slip, and it is not this package's to
	// correct: reporting a rule in a syntax where the authority's own validator
	// cannot report it is the defect C32 records eight times over. There is no
	// exemption for credit notes in either binding, and this package used to grant
	// one.
	due, dueKnown := parseAmount(inv.totals.duePayable)
	if inv.syntax == "CII" {
		if dueKnown && due < 0 && len(inv.paymentMeans) == 0 {
			add("BR-NL-11", "the invoice must provide a means of payment (BG-16)")
		}
	} else if !(dueKnown && due <= 0) && len(inv.paymentMeans) == 0 {
		add("BR-NL-11", "the invoice must provide a means of payment (BG-16)")
	}

	// BR-NL-12: each payment means code must be one NLCIUS permits.
	for range inv.paymentMeans {
		seen.reached("BR-NL-12")
	}
	for _, code := range inv.paymentMeans {
		if !nlciusPaymentMeans[code] {
			add("BR-NL-12", fmt.Sprintf("the payment means code (BT-81=%q) must be one of 30, 48, 49, 57, 58, 59", code))
			break
		}
	}

	return append(out, validateNLCIUSAdvisory(p, seen)...)
}

// nlciusDiscouraged is one "not recommended" rule of the shape both bindings write
// most of them in: a Schematron rule whose context is a location path and whose
// single assertion is test="false", i.e. "reaching this node is the finding".
//
// path is the context written as local element names, because parseCII keys on
// local names and the two bindings name the same term differently anyway
// (cbc:CountrySubentity against ram:CountrySubDivisionName).
type nlciusDiscouraged struct {
	id   string
	path []string
	what string
}

// nlciusUBLDiscouraged is si-ubl-2.0-nlcius.sch's forbidden-path tier.
//
// BR-NL-32-1 covers the line-level allowance and charge reason codes as well as the
// document-level ones, and that is not a shortcut: its context
// cac:AllowanceCharge/cbc:AllowanceChargeReasonCode is an XSLT match pattern
// anchored at its last step, so it matches a reason code under any cac:AllowanceCharge
// at any depth, and it is the first rule in the pattern that does. BR-NL-32-2 and
// BR-NL-32-3, which name the two line paths explicitly, are therefore unreachable —
// see Coverage(SourceNLCIUS).
var nlciusUBLDiscouraged = []nlciusDiscouraged{
	{"BR-NL-19", []string{"TaxCurrencyCode"}, "a VAT accounting currency code (BT-6)"},
	{"BR-NL-20", []string{"TaxPointDate"}, "a tax point date (BT-7); its value is ignored"},
	{"BR-NL-21", []string{"InvoicePeriod", "DescriptionCode"}, "a tax point date code (BT-8); its value is ignored"},
	{"BR-NL-24", []string{"BillingReference", "InvoiceDocumentReference", "IssueDate"}, "a preceding invoice issue date (BT-26)"},
	{"BR-NL-26", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "CompanyLegalForm"}, "the Seller additional legal information (BT-33), which does not apply to a Dutch supplier"},
	{"BR-NL-27-1", []string{"AccountingSupplierParty", "Party", "PostalAddress", "AddressLine", "Line"}, "a Seller address line 3 (BT-163)"},
	{"BR-NL-27-2", []string{"AccountingCustomerParty", "Party", "PostalAddress", "AddressLine", "Line"}, "a Buyer address line 3 (BT-53)"},
	{"BR-NL-27-3", []string{"TaxRepresentativeParty", "PostalAddress", "AddressLine", "Line"}, "a tax representative address line 3"},
	{"BR-NL-27-4", []string{"Delivery", "DeliveryLocation", "Address", "AddressLine", "Line"}, "a delivery address line 3 (BT-79)"},
	{"BR-NL-28-1", []string{"AccountingSupplierParty", "Party", "PostalAddress", "CountrySubentity"}, "a Seller country subdivision (BT-39)"},
	{"BR-NL-28-2", []string{"AccountingCustomerParty", "Party", "PostalAddress", "CountrySubentity"}, "a Buyer country subdivision (BT-54)"},
	{"BR-NL-28-3", []string{"TaxRepresentativeParty", "PostalAddress", "CountrySubentity"}, "a tax representative country subdivision"},
	{"BR-NL-28-4", []string{"Delivery", "DeliveryLocation", "Address", "CountrySubentity"}, "a delivery country subdivision (BT-79)"},
	{"BR-NL-30", []string{"PaymentMeans", "PayeeFinancialAccount", "Name"}, "a payment account name (BT-85)"},
	{"BR-NL-32-1", []string{"AllowanceCharge", "AllowanceChargeReasonCode"}, "an allowance or charge reason code (BT-98/BT-105/BT-140/BT-145)"},
	{"BR-NL-35", []string{"TaxTotal", "TaxSubtotal", "TaxCategory", "TaxExemptionReasonCode"}, "a VAT exemption reason code (BT-121)"},
}

// nlciusCIIDiscouraged is NLCIUS-CII-validation.sch's forbidden-path tier. It is not
// the same list: this binding publishes BR-NL-22 and BR-NL-23, which the UBL one
// does not, and it writes the allowance and charge reason codes as one rule with the
// honest identifier BR-NL-32-and-34 rather than as three under two numberings.
var nlciusCIIDiscouraged = []nlciusDiscouraged{
	{"BR-NL-19", []string{"TaxCurrencyCode"}, "a VAT accounting currency code (BT-6)"},
	{"BR-NL-20", []string{"TaxPointDate", "DateString"}, "a tax point date (BT-7); its value is ignored"},
	{"BR-NL-21", []string{"DueDateTypeCode"}, "a tax point date code (BT-8); its value is ignored"},
	{"BR-NL-22", []string{"SubjectCode"}, "an invoice note subject code (BT-21), which has to be agreed with the receiving party"},
	{"BR-NL-23", []string{"BusinessProcessSpecifiedDocumentContextParameter", "ID"}, "a business process identifier (BT-23), unless a particular network wants one"},
	{"BR-NL-24", []string{"InvoiceReferencedDocument", "FormattedIssueDateTime", "DateTimeString"}, "a preceding invoice issue date (BT-26)"},
	{"BR-NL-26", []string{"SellerTradeParty", "Description"}, "the Seller additional legal information (BT-33), which does not apply to a Dutch supplier"},
	{"BR-NL-27-1", []string{"SellerTradeParty", "PostalTradeAddress", "LineThree"}, "a Seller address line 3 (BT-163)"},
	{"BR-NL-27-2", []string{"BuyerTradeParty", "PostalTradeAddress", "LineThree"}, "a Buyer address line 3 (BT-53)"},
	{"BR-NL-27-3", []string{"SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "LineThree"}, "a tax representative address line 3"},
	{"BR-NL-27-4", []string{"ShipToTradeParty", "PostalTradeAddress", "LineThree"}, "a delivery address line 3 (BT-79)"},
	{"BR-NL-28-1", []string{"SellerTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}, "a Seller country subdivision (BT-39)"},
	{"BR-NL-28-2", []string{"BuyerTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}, "a Buyer country subdivision (BT-54)"},
	{"BR-NL-28-3", []string{"SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}, "a tax representative country subdivision"},
	{"BR-NL-28-4", []string{"ShipToTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}, "a delivery country subdivision (BT-79)"},
	{"BR-NL-29", []string{"SpecifiedTradeSettlementPaymentMeans", "Information"}, "a payment means text (BT-82)"},
	{"BR-NL-30", []string{"PayeePartyCreditorFinancialAccount", "AccountName"}, "a payment account name (BT-85)"},
	{"BR-NL-32-and-34", []string{"SpecifiedTradeAllowanceCharge", "ReasonCode"}, "an allowance or charge reason code (BT-98/BT-105/BT-140/BT-145), at document or line level"},
}

// validateNLCIUSAdvisory applies the "not recommended" tier of the binding the
// document is written in. It is reached inside $s, which is where both artefacts put
// every one of these rules.
//
// The findings are warnings, so they are absent from Report.Fatal and do not move
// Report.Conformant: an invoice whose only NLCIUS findings are these is conformant
// to NLCIUS, which is exactly what "not recommended" means. What they change is
// Report.Complete, which was false for every Dutch invoice while this tier was a
// named gap.
func validateNLCIUSAdvisory(p *parsed, seen ruleContexts) []Violation {
	var out []Violation
	warn := advisoryAdder(&out, SourceNLCIUS)

	table := nlciusUBLDiscouraged
	if p.inv.syntax == "CII" {
		table = nlciusCIIDiscouraged
	}
	for _, d := range table {
		for range p.root.matchPath(d.path...) {
			seen.reached(d.id)
			warn(d.id, "the invoice states "+d.what+", which NLCIUS does not recommend")
		}
	}

	if p.inv.syntax == "CII" {
		nlciusCIIConditional(p.root, warn, seen)
	} else {
		nlciusUBLConditional(p.root, warn, seen)
	}
	return out
}

// nlciusUBLConditional is the four UBL advisory rules that test something rather
// than merely being reached.
func nlciusUBLConditional(root *ciiNode, warn func(rule, msg string), seen ruleContexts) {
	// BR-NL-25, context cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme[$s]:
	//   not(cbc:CompanyID) or cac:TaxScheme/cbc:ID = 'VAT'
	for _, pts := range root.matchPath("AccountingSupplierParty", "Party", "PartyTaxScheme") {
		seen.reached("BR-NL-25")
		if pts.child("CompanyID") != nil && pts.str("TaxScheme", "ID") != "VAT" {
			warn("BR-NL-25", "the invoice states a Seller tax registration identifier under a tax scheme that is "+
				"not VAT, which does not apply to a Dutch supplier")
		}
	}

	// BR-NL-29, context cac:PaymentMeans/cbc:PaymentMeansCode[$s]: not(@name).
	// BR-NL-31, context cac:PaymentMeans[$s]: a SEPA payment (58 or 59) shall not
	// name a payment service provider.
	for _, pm := range root.matchPath("PaymentMeans") {
		for _, code := range pm.all("PaymentMeansCode") {
			seen.reached("BR-NL-29")
			if code.hasAttr("name") {
				warn("BR-NL-29", "the invoice states a payment means text (BT-82) on the payment means code, "+
					"which NLCIUS does not recommend")
			}
		}
		seen.reached("BR-NL-31")
		c := normSpace(pm.str("PaymentMeansCode"))
		if (c == "58" || c == "59") && pm.child("PayeeFinancialAccount", "FinancialInstitutionBranch", "ID") != nil {
			warn("BR-NL-31", "the invoice names a payment service provider (BT-86) for a SEPA payment, "+
				"which NLCIUS does not recommend")
		}
	}

	// BR-NL-33, context cac:TaxTotal/cbc:TaxAmount[$s]:
	//   @currencyID = //cbc:DocumentCurrencyCode
	// The context is every VAT total, line-level ones included, and an absent
	// @currencyID equals nothing, so it reports too.
	doc := ""
	if c := root.child("DocumentCurrencyCode"); c != nil {
		doc = normSpace(c.text)
	}
	for _, amt := range root.matchPath("TaxTotal", "TaxAmount") {
		seen.reached("BR-NL-33")
		if normSpace(amt.attr("currencyID")) != doc {
			warn("BR-NL-33", "the invoice states a VAT total in a currency other than the document currency, "+
				"which NLCIUS does not recommend")
		}
	}
}

// nlciusCIIConditional is the two CII advisory rules that test something rather than
// merely being reached. BR-NL-31 is not among them: the CII file binds it to a
// context the rule carrying BR-NL-12 has already claimed, so no processor reaches
// it. See Coverage(SourceNLCIUS).
func nlciusCIIConditional(root *ciiNode, warn func(rule, msg string), seen ruleContexts) {
	// BR-NL-25, context ram:SellerTradeParty/ram:SpecifiedTaxRegistration[$s]:
	//   not(ram:ID) or ram:ID/@schemeID = 'VA'
	for _, reg := range root.matchPath("SellerTradeParty", "SpecifiedTaxRegistration") {
		seen.reached("BR-NL-25")
		id := reg.child("ID")
		if id != nil && normSpace(id.attr("schemeID")) != "VA" {
			warn("BR-NL-25", "the invoice states a Seller tax registration identifier under a tax scheme that is "+
				"not VAT, which does not apply to a Dutch supplier")
		}
	}

	// BR-NL-33, context ram:SpecifiedTradeSettlementHeaderMonetarySummation/
	// ram:TaxTotalAmount[$s]: @currencyID = ../../ram:InvoiceCurrencyCode. The
	// currency is read from the summation's *grandparent*, so the walk is over the
	// nodes that can hold one rather than over the amounts alone.
	var rec func(*ciiNode)
	rec = func(n *ciiNode) {
		for _, sum := range n.all("SpecifiedTradeSettlementHeaderMonetarySummation") {
			for _, amt := range sum.all("TaxTotalAmount") {
				seen.reached("BR-NL-33")
				if normSpace(amt.attr("currencyID")) != normSpace(n.str("InvoiceCurrencyCode")) {
					warn("BR-NL-33", "the invoice states a VAT total in a currency other than the invoice "+
						"currency, which NLCIUS does not recommend")
				}
			}
		}
		for _, c := range n.children {
			rec(c)
		}
	}
	rec(root)
}

// ---------------------------------------------------------------------------
// The empty-element rule
// ---------------------------------------------------------------------------

// Both bindings end their pattern with the same rule under a different name:
//
//	si-ubl-2.0-nlcius.sch   <rule context="//*[not(*) and not(normalize-space())]">
//	                          <assert id="SI-UBL-2" test="false()" flag="warning">
//	NLCIUS-CII-validation.sch  … <assert id="empty-element-check" test="false()" flag="warning">
//
// with the text "Document should not contain empty elements." in both. The two
// identifiers are kept apart rather than folded into one, because each is what its
// own artefact publishes and a caller comparing this package against a reference
// validator compares identifiers.
//
// The assertion cannot pass, so the context *is* the rule: every element with no
// child element and no non-whitespace content is a finding, one per element, which
// is what a Schematron processor reports and what PEPPOL-EN16931-R008 — the same
// rule, in OpenPEPPOL's and KoSIT's files, fatal there rather than advisory —
// already does in peppol_rules.go.
//
// Two things about it needed deciding, and both are decided from the artefact.
//
// # It carries no gate, so neither does this
//
// It is the only rule in either binding whose context has no $si or $s predicate.
// Every BR-NL rule is inside "this document declares the NLCIUS specification
// identifier", and this one is not, so SimplerInvoicing's own validator reports
// empty elements in a document that is not an NLCIUS invoice at all. This package
// does the same: a caller who invoked ValidateNLCIUS asked for NLCIUS's judgement,
// and answering with less than the authority answers, under a gate of this
// package's own invention, would be the mirror image of the C36/C42 defect —
// reporting *more* than the authority can. It is the one NLCIUS rule
// ValidateNLCIUS can report for a Spanish supplier's Peppol invoice, and it is a
// warning, so it cannot move Conformant().
//
// # It is last in its pattern, so earlier rules take nodes off it
//
// ISO Schematron gives a node to the first rule in a pattern whose context matches
// it and to no other. This rule is rule 35 of 35 in the UBL binding and 34 of 34 in
// the CII one, so an empty element that an earlier rule of the same pattern already
// claims is *not* reported as an empty element: an empty cbc:TaxCurrencyCode in a
// Dutch NLCIUS invoice is BR-NL-19's node, not SI-UBL-2's, and SimplerInvoicing's
// validator says so. That is the same rule-order fact as C42, in the one position
// where it subtracts findings from a rule rather than making one unreachable, and
// nlciusUBLPatternContexts and nlciusCIIPatternContexts are the earlier contexts it
// needs. Being last also means the rule takes no node off any other, so it changes
// no other identifier's reachability.
//
// The subtraction is gated the way the earlier rules are: outside $si no earlier
// rule matches anything, so every empty element is reported; inside $si but outside
// $s only the $si contexts subtract, of which UBL has one and CII two; inside $s
// all of them do.
//
// No document in the corpus exercises the subtraction — the 739 empty elements in
// 1,690 documents are all outside every earlier context, and 735 of them are in
// documents outside $si altogether — so it is checked by fixture rather than by
// sweep. See TestNLCIUSEmptyElementYieldsTheClaimedNode.

// nlciusPatternContext is one Schematron rule context of the nlcius pattern that
// precedes the empty-element rule, in the form this package matches contexts:
// local element names, because parseCII keys on local names.
//
// absolute distinguishes the two shapes the artefacts use. A relative context
// ("cac:PaymentMeans/cbc:PaymentMeansCode") is an XSLT match pattern anchored at
// its last step and matches at any depth, which is matchPath. An absolute one
// ("/*", "/*/rsm:ExchangedDocument/ram:TypeCode") is anchored at the document
// element, which is a walk of named children from the root; an empty path with
// absolute set is the document element itself.
//
// si is the gate: false means the context carries [$s], true that it carries [$si].
// Every context in the two patterns carries one or the other.
type nlciusPatternContext struct {
	path     []string
	absolute bool
	si       bool
}

// nodes returns the elements this context claims.
func (c nlciusPatternContext) nodes(root *ciiNode) []*ciiNode {
	if !c.absolute {
		return root.matchPath(c.path...)
	}
	cur := []*ciiNode{root}
	for _, name := range c.path {
		var next []*ciiNode
		for _, n := range cur {
			next = append(next, n.all(name)...)
		}
		cur = next
	}
	return cur
}

// nlciusUBLPatternContexts is every distinct rule context of si-ubl-2.0-nlcius.sch's
// pattern that sits before the SI-UBL-2 rule — rules 1 to 34 of 35, with the three
// contexts the BR-NL-34 trio repeats folded in and cbc:InvoiceTypeCode[$s]|
// cbc:CreditNoteTypeCode[$s] split into its two alternatives.
//
// TestNLCIUSEmptyElementShadowsAreDerivedFromTheArtefacts re-derives this list from
// the file, in both directions, so a release that adds a rule to the pattern cannot
// leave it here as a stale claim about somebody else's Schematron.
var nlciusUBLPatternContexts = []nlciusPatternContext{
	{absolute: true},
	{path: []string{"AccountingSupplierParty", "Party"}},
	{path: []string{"AccountingSupplierParty", "Party", "PostalAddress"}},
	{path: []string{"AccountingCustomerParty", "Party", "PostalAddress"}},
	{path: []string{"TaxRepresentativeParty", "PostalAddress"}},
	{path: []string{"InvoiceTypeCode"}},
	{path: []string{"CreditNoteTypeCode"}},
	{path: []string{"AccountingCustomerParty", "Party", "PartyLegalEntity"}},
	{path: []string{"LegalMonetaryTotal"}},
	{path: []string{"PaymentMeans"}},
	{path: []string{"OrderLineReference", "LineID"}, si: true},
	{path: []string{"TaxCurrencyCode"}},
	{path: []string{"TaxPointDate"}},
	{path: []string{"InvoicePeriod", "DescriptionCode"}},
	{path: []string{"BillingReference", "InvoiceDocumentReference", "IssueDate"}},
	{path: []string{"AccountingSupplierParty", "Party", "PartyTaxScheme"}},
	{path: []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "CompanyLegalForm"}},
	{path: []string{"AccountingSupplierParty", "Party", "PostalAddress", "AddressLine", "Line"}},
	{path: []string{"AccountingCustomerParty", "Party", "PostalAddress", "AddressLine", "Line"}},
	{path: []string{"TaxRepresentativeParty", "PostalAddress", "AddressLine", "Line"}},
	{path: []string{"Delivery", "DeliveryLocation", "Address", "AddressLine", "Line"}},
	{path: []string{"AccountingSupplierParty", "Party", "PostalAddress", "CountrySubentity"}},
	{path: []string{"AccountingCustomerParty", "Party", "PostalAddress", "CountrySubentity"}},
	{path: []string{"TaxRepresentativeParty", "PostalAddress", "CountrySubentity"}},
	{path: []string{"Delivery", "DeliveryLocation", "Address", "CountrySubentity"}},
	{path: []string{"PaymentMeans", "PaymentMeansCode"}},
	{path: []string{"PaymentMeans", "PayeeFinancialAccount", "Name"}},
	{path: []string{"AllowanceCharge", "AllowanceChargeReasonCode"}},
	{path: []string{"InvoiceLine", "AllowanceCharge", "AllowanceChargeReasonCode"}},
	{path: []string{"CreditNoteLine", "AllowanceCharge", "AllowanceChargeReasonCode"}},
	{path: []string{"TaxTotal", "TaxAmount"}},
	{path: []string{"TaxTotal", "TaxSubtotal", "TaxCategory", "TaxExemptionReasonCode"}},
}

// nlciusCIIPatternContexts is the same for NLCIUS-CII-validation.sch: rules 1 to 33
// of 34, with the two contexts BR-NL-9 and BR-NL-31 repeat folded in — those are the
// two the file cannot reach, and a context claims nodes whether or not the rule
// holding it says anything about them.
var nlciusCIIPatternContexts = []nlciusPatternContext{
	{path: []string{"SellerTradeParty", "SpecifiedLegalOrganization"}},
	{path: []string{"ApplicableHeaderTradeAgreement"}},
	{path: []string{"SellerTradeParty", "PostalTradeAddress"}},
	{path: []string{"BuyerTradeParty", "PostalTradeAddress"}},
	{path: []string{"SellerTaxRepresentativeTradeParty", "PostalTradeAddress"}},
	{path: []string{"ExchangedDocument", "TypeCode"}, absolute: true, si: true},
	{path: []string{"BuyerTradeParty"}},
	{path: []string{"SpecifiedTradeSettlementHeaderMonetarySummation"}},
	{path: []string{"SpecifiedTradeSettlementPaymentMeans"}},
	{path: []string{"BuyerOrderReferencedDocument", "LineID"}, si: true},
	{path: []string{"TaxCurrencyCode"}},
	{path: []string{"TaxPointDate", "DateString"}},
	{path: []string{"DueDateTypeCode"}},
	{path: []string{"SubjectCode"}},
	{path: []string{"BusinessProcessSpecifiedDocumentContextParameter", "ID"}},
	{path: []string{"InvoiceReferencedDocument", "FormattedIssueDateTime", "DateTimeString"}},
	{path: []string{"SellerTradeParty", "SpecifiedTaxRegistration"}},
	{path: []string{"SellerTradeParty", "Description"}},
	{path: []string{"SellerTradeParty", "PostalTradeAddress", "LineThree"}},
	{path: []string{"BuyerTradeParty", "PostalTradeAddress", "LineThree"}},
	{path: []string{"SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "LineThree"}},
	{path: []string{"ShipToTradeParty", "PostalTradeAddress", "LineThree"}},
	{path: []string{"SellerTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}},
	{path: []string{"BuyerTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}},
	{path: []string{"SellerTaxRepresentativeTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}},
	{path: []string{"ShipToTradeParty", "PostalTradeAddress", "CountrySubDivisionName"}},
	{path: []string{"SpecifiedTradeSettlementPaymentMeans", "Information"}},
	{path: []string{"PayeePartyCreditorFinancialAccount", "AccountName"}},
	{path: []string{"SpecifiedTradeAllowanceCharge", "ReasonCode"}},
	{path: []string{"SpecifiedTradeSettlementHeaderMonetarySummation", "TaxTotalAmount"}},
	{path: []string{"ApplicableTradeTax", "ExemptionReasonCode"}},
}

// nlciusEmptyElements evaluates SI-UBL-2 (UBL) or empty-element-check (CII).
func nlciusEmptyElements(p *parsed, seen ruleContexts) []Violation {
	id, contexts := "SI-UBL-2", nlciusUBLPatternContexts
	if p.inv.syntax == "CII" {
		id, contexts = "empty-element-check", nlciusCIIPatternContexts
	}
	claimed := nlciusClaimedNodes(p, contexts)

	var out []Violation
	warn := advisoryAdder(&out, SourceNLCIUS)
	var walk func(*ciiNode)
	walk = func(n *ciiNode) {
		// not(*) and not(normalize-space()). The second half reads the element's own
		// character data rather than stringValue, which is the same thing once the
		// first half holds: an element with no element children has no descendant
		// text but its own. A comment or a processing instruction is neither an
		// element nor text in XPath, and parseCII discards both, so an element
		// containing only one of those is empty on both sides.
		if len(n.children) == 0 && strings.TrimSpace(n.text) == "" && !claimed[n] {
			seen.reached(id)
			warn(id, fmt.Sprintf("the element %q is empty; a document should not contain empty elements", n.name))
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(p.root)
	return out
}

// nlciusClaimedNodes is the set of elements an earlier rule of the same pattern
// holds, and so the set the empty-element rule never sees.
//
// It returns nil outside $si, which is not an optimisation but the artefact: every
// other context in both patterns carries [$s] or [$si], so outside $si none of them
// selects anything and every empty element falls through to the last rule.
func nlciusClaimedNodes(p *parsed, contexts []nlciusPatternContext) map[*ciiNode]bool {
	si := nlciusApplies(p.inv)
	if !si {
		return nil
	}
	s := p.inv.sellerCountry == "NL"
	claimed := map[*ciiNode]bool{}
	for _, c := range contexts {
		if !c.si && !s {
			continue
		}
		for _, n := range c.nodes(p.root) {
			claimed[n] = true
		}
	}
	return claimed
}
