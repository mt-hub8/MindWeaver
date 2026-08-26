package formalis

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// This file evaluates the XRechnung (KoSIT) rules that are statements about a
// document *tree* rather than about the syntax-neutral model: the payment-means
// group rules (BR-DE-19/20/23/24/25/30/31), the settlement-discount text format
// (BR-DE-18), the two sub-profiles — EXTENSION (BR-DEX-*) and CVD
// (BR-DE-CVD-*) — and KoSIT's provisional rules (BR-DE-TMP-32, BR-TMP-2,
// BR-TMP-3, BR-TMP-CVD-01). xrechnung.go holds the entry point and the
// mandatory-term rules that the shared model already answers.
//
// Fidelity. Every rule below is transcribed from the assertion KoSIT publishes in
// testdata/xrechnung/schematron/src/validation/schematron/ubl/XRechnung-UBL-validation.sch
// and .../cii/XRechnung-CII-validation.sch, with the variables of
// .../common.sch resolved, and each cites its XPath. That is not ceremony: PR 14
// found several CEN rule titles that describe a different rule than their XPath,
// and KoSIT's own titles have the same problem. BR-DEX-09's message reads
// "BT-115 = BT-112 - BT-113 + BT-114 + Σ BT-DEX-002" while its XPath compares
// `PayableAmount - PayableRoundingAmount` with
// `TaxInclusiveAmount - PrepaidAmount + Σ PaidAmount`, i.e. the rounding amount
// moves to the other side; the XPath is the rule.
//
// Both syntaxes, separately. KoSIT publishes two Schematron files and they are
// not translations of each other. Nine identifiers exist in one binding only —
// BR-DEX-02/03 and BR-DEX-09..14 are UBL-only, BR-DEX-15 and BR-TMP-3 are
// CII-only — and where an identifier exists in both, its XPath can still test a
// different thing: BG-19 ("DIRECT DEBIT") is one cac:PaymentMandate element in
// UBL and, in CII, the *semantic* group KoSIT reconstructs from "any of BT-89,
// BT-90, BT-91 is present", which is why BR-DE-30 and BR-DE-31 have two bodies
// here rather than one over the model. Where a context names ubl:Invoice and not
// cn:CreditNote — BR-DEX-02, BR-DEX-03 and BR-DEX-10..14 do — that is honoured,
// because a rule that cannot match a credit note in KoSIT's Schematron must not
// fire on one here.
//
// Severity is quoted, never chosen: xrechnungFlags carries KoSIT's flag for every
// identifier this package evaluates, and TestXRechnungSeveritiesQuoteKoSIT reads
// the flags back out of the Schematron and fails on any divergence in either
// direction. Seven of the rules that were already implemented were being reported
// fatal against KoSIT's warning, which is how a conforming invoice could be
// refused for a rule its own authority only warns about.

// validateXRechnungTreeRules evaluates the tree-shaped half of the XRechnung
// rule set against the document as parsed, dispatching on the binding the
// invoice was expressed in.
func validateXRechnungTreeRules(r *run, p *parsed, ext, cvd bool) []Violation {
	var out []Violation
	add := xrAdder(&out)
	root := p.root
	if p.inv.syntax == "CII" {
		xrCIIRules(r, root, add)
		if ext {
			xrCIIExtensionRules(r, root, add)
		}
		if cvd {
			xrCIICVDRules(r, root, add)
		}
		return out
	}
	xrUBLRules(r, root, add)
	if ext {
		xrUBLExtensionRules(r, root, add)
	}
	if cvd {
		xrUBLCVDRules(r, root, add)
	}
	return out
}

// ---------------------------------------------------------------------------
// The UBL binding
// ---------------------------------------------------------------------------

// xrUBLRules evaluates the rules of the ubl-pattern whose context is a tree
// position rather than the whole document.
func xrUBLRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	// BR-DE-18, context /ubl:Invoice | /cn:CreditNote:
	//   every $line in cac:PaymentTerms/cbc:Note[1]/tokenize(., '(\r?\n)')
	//                  [starts-with(normalize-space(.), '#')]
	//   satisfies matches(normalize-space($line), $XR-SKONTO-REGEX) and
	//             matches(cac:PaymentTerms/cbc:Note[1]/tokenize(., '#.+#')[last()], '^\s*\n')
	xrSkontoRule(xrFirstNotes(root, "PaymentTerms", "Note"), add)

	// BR-DE-30, context /ubl:Invoice | /cn:CreditNote:
	//   not(cac:PaymentMeans/cac:PaymentMandate) or
	//   (cac:AccountingSupplierParty/cac:Party/cac:PartyIdentification/cbc:ID[@schemeID='SEPA']
	//    | cac:PayeeParty/cac:PartyIdentification/cbc:ID[@schemeID='SEPA'])
	//
	// BT-90 is a party identifier with the SEPA scheme, not the mandate's own
	// identifier: an invoice can carry a mandate reference and still be missing
	// the creditor identifier the mandate was issued under.
	mandates := nodesAt(root, "PaymentMeans", "PaymentMandate")
	if len(mandates) > 0 {
		sepa := false
		for _, id := range append(
			nodesAt(root, "AccountingSupplierParty", "Party", "PartyIdentification", "ID"),
			nodesAt(root, "PayeeParty", "PartyIdentification", "ID")...) {
			if id.attr("schemeID") == "SEPA" {
				sepa = true
			}
		}
		if !sepa {
			add("BR-DE-30", "A direct debit (BG-19) shall carry the Bank assigned creditor identifier (BT-90): "+
				"a Seller or Payee party identifier with the SEPA scheme")
		}
		// BR-DE-31: not(cac:PaymentMeans/cac:PaymentMandate) or
		//           (cac:PaymentMeans/cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID)
		if len(nodesAt(root, "PaymentMeans", "PaymentMandate", "PayerFinancialAccount", "ID")) == 0 {
			add("BR-DE-31", "A direct debit (BG-19) shall carry the Debited account identifier (BT-91)")
		}
	}

	// BR-DE-TMP-32, context /ubl:Invoice | /cn:CreditNote:
	//   cac:Delivery/cbc:ActualDeliveryDate or cac:InvoicePeriod or
	//   (every $line in (cac:InvoiceLine | cac:CreditNoteLine) satisfies $line/cac:InvoicePeriod)
	lines := append(root.all("InvoiceLine"), root.all("CreditNoteLine")...)
	everyLinePeriod := true
	for _, li := range lines {
		if li.child("InvoicePeriod") == nil {
			everyLinePeriod = false
		}
	}
	if len(nodesAt(root, "Delivery", "ActualDeliveryDate")) == 0 && root.child("InvoicePeriod") == nil && !everyLinePeriod {
		add("BR-DE-TMP-32", xrTMP32Message)
	}

	// BR-TMP-2, context /ubl:Invoice/cac:AdditionalDocumentReference/cac:Attachment/
	//   cac:ExternalReference: matches(cbc:URI, $XR-URL-REGEX).
	//
	// The URI is read untouched rather than trimmed, because the assertion is
	// matches() over the element's string value with no normalize-space around it
	// and the pattern is anchored at the start.
	for _, ref := range nodesAt(root, "AdditionalDocumentReference", "Attachment", "ExternalReference") {
		if !xrURLRE.MatchString(ref.child("URI").rawText()) {
			add("BR-TMP-2", "The External document location (BT-124) shall be an absolute URL with a valid scheme")
		}
	}

	// The three payment-means groups. Each is one Schematron rule whose context is
	// a cac:PaymentMeans selected by its BT-81, so a document with several groups
	// is checked group by group and a group of one kind may not carry another
	// kind's account details.
	for _, pm := range root.all("PaymentMeans") {
		if r.stopped() {
			return
		}
		code := normalizeSpace(pm.str("PaymentMeansCode"))
		credit := pm.child("PayeeFinancialAccount") != nil
		card := pm.child("CardAccount") != nil
		mandate := pm.child("PaymentMandate") != nil
		switch {
		// context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = ('30','58')]
		case code == "30" || code == "58":
			// BR-DE-19: not(code = '58') or the IBAN check on
			// cac:PayeeFinancialAccount/cbc:ID. KoSIT flags it warning, and its own
			// comment records that code 30 was left out of the check on purpose.
			if code == "58" && !validIBAN(pm.str("PayeeFinancialAccount", "ID")) {
				add("BR-DE-19", "The Payment account identifier (BT-84) shall be a valid IBAN for a SEPA credit transfer")
			}
			if !credit {
				add("BR-DE-23-a", "A Payment means type code (BT-81) for a credit transfer (30, 58) requires the "+
					"Credit transfer group (BG-17)")
			}
			// BR-DE-23-b: not(cac:CardAccount) and not(cac:PaymentMandate)
			if card || mandate {
				add("BR-DE-23-b", "A Payment means type code (BT-81) for a credit transfer (30, 58) shall carry "+
					"neither the Payment card group (BG-18) nor the Direct debit group (BG-19)")
			}
		// context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = ('48','54','55')]
		case code == "48" || code == "54" || code == "55":
			if !card {
				add("BR-DE-24-a", "A Payment means type code (BT-81) for a card payment (48, 54, 55) requires the "+
					"Payment card information group (BG-18)")
			}
			if credit || mandate {
				add("BR-DE-24-b", "A Payment means type code (BT-81) for a card payment (48, 54, 55) shall carry "+
					"neither the Credit transfer group (BG-17) nor the Direct debit group (BG-19)")
			}
		// context cac:PaymentMeans[normalize-space(cbc:PaymentMeansCode) = '59']
		case code == "59":
			// BR-DE-20: the IBAN check on cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID.
			if !validIBAN(pm.str("PaymentMandate", "PayerFinancialAccount", "ID")) {
				add("BR-DE-20", "The Debited account identifier (BT-91) shall be a valid IBAN for a SEPA direct debit")
			}
			if !mandate {
				add("BR-DE-25-a", "A Payment means type code (BT-81) for a direct debit (59) requires the "+
					"Direct debit group (BG-19)")
			}
			if credit || card {
				add("BR-DE-25-b", "A Payment means type code (BT-81) for a direct debit (59) shall carry neither the "+
					"Credit transfer group (BG-17) nor the Payment card group (BG-18)")
			}
		}
	}
}

// xrUBLExtensionRules evaluates the ubl-extension-pattern. Its rules replace six
// EN 16931 code-list and summation rules for a document whose BT-24 claims the
// EXTENSION sub-profile — see xrechnungSuppressedForExtension for the other half
// of that swap.
func xrUBLExtensionRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	// BR-DEX-01, context cbc:EmbeddedDocumentBinaryObject[$isExtension]: the
	// EN 16931 MIME list (BR-CL-24) plus application/xml.
	for _, b := range root.findAll("EmbeddedDocumentBinaryObject") {
		if !xrExtMIME[b.attr("mimeCode")] {
			add("BR-DEX-01", fmt.Sprintf("Attached document (BT-125) MIME code %q is not one the EXTENSION permits",
				b.attr("mimeCode")))
		}
	}
	// BR-DEX-04..08 are BR-CL-10/11/21/25/26 with XR01, XR02 and XR03 added.
	//
	// BR-DEX-04 has a second arm the other four do not:
	//
	//	... or ((not(contains(normalize-space(@schemeID), ' ')) and
	//	         contains(' SEPA ', concat(' ', normalize-space(@schemeID), ' '))) and
	//	        ((ancestor::cac:AccountingSupplierParty) or (ancestor::cac:PayeeParty)))
	//
	// so the scheme 'SEPA' is legal on a Seller or Payee party identifier and
	// nowhere else. That is BT-90, the creditor identifier BR-DE-30 requires — so
	// without this arm every EXTENSION invoice carrying a direct debit would be
	// refused for the identifier another XRechnung rule obliges it to carry, which
	// is what one of KoSIT's own conforming business cases showed.
	sepaOK := map[*ciiNode]bool{}
	for _, party := range append(root.findAll("AccountingSupplierParty"), root.findAll("PayeeParty")...) {
		for _, id := range xrFindAt(party, []string{"PartyIdentification", "ID"}) {
			sepaOK[id] = true
		}
	}
	for _, id := range xrFindAt(root, []string{"PartyIdentification", "ID"}) {
		if normalizeSpace(id.attr("schemeID")) == "SEPA" && sepaOK[id] {
			continue
		}
		xrCheckScheme(id, add, "BR-DEX-04", "party identifier (BT-29/BT-46/BT-60/BT-90)", xrISO6523Ext)
	}
	for _, id := range xrFindAt(root, []string{"PartyLegalEntity", "CompanyID"}) {
		xrCheckScheme(id, add, "BR-DEX-05", "party legal registration identifier (BT-30/BT-47/BT-61)", xrISO6523Ext)
	}
	for _, id := range xrFindAt(root, []string{"StandardItemIdentification", "ID"}) {
		xrCheckScheme(id, add, "BR-DEX-06", "item standard identifier (BT-157)", xrISO6523Ext)
	}
	for _, id := range root.findAll("EndpointID") {
		xrCheckScheme(id, add, "BR-DEX-07", "electronic address (BT-34/BT-49)", xrCEFEASExt)
	}
	for _, id := range xrFindAt(root, []string{"DeliveryLocation", "ID"}) {
		xrCheckScheme(id, add, "BR-DEX-08", "deliver-to location identifier (BT-71)", xrISO6523Ext)
	}

	// BR-DEX-09, context cac:LegalMonetaryTotal[$isExtension]:
	//   round((PayableAmount - PayableRoundingAmount) * 100) div 100 =
	//   round((TaxInclusiveAmount - PrepaidAmount + Σ ../cac:PrepaidPayment/cbc:PaidAmount) * 100) div 100
	//
	// It replaces BR-CO-16 for an EXTENSION document, adding the third-party
	// payments BG-DEX-09 introduces. Note the sign: the sum is added to the right
	// hand side, so a third-party payment *raises* the amount due. That is what
	// the XPath says and the prose says the same, twice.
	for _, tot := range root.findAll("LegalMonetaryTotal") {
		payable, okP := parseAmount(tot.str("PayableAmount"))
		inclusive, okI := parseAmount(tot.str("TaxInclusiveAmount"))
		if !okP || !okI {
			// An absent or unreadable BT-112 or BT-115 is BR-12/BR-14's finding to
			// report, and xs:decimal() of it would stop a reference validator
			// outright rather than produce a verdict on this rule.
			continue
		}
		var prepaid, rounding, thirdParty float64
		if v, ok := parseAmount(tot.str("PrepaidAmount")); ok {
			prepaid = v
		}
		if v, ok := parseAmount(tot.str("PayableRoundingAmount")); ok {
			rounding = v
		}
		for _, pp := range root.all("PrepaidPayment") {
			if v, ok := parseAmount(pp.str("PaidAmount")); ok {
				thirdParty += v
			}
		}
		if math.Abs(round2(payable-rounding)-round2(inclusive-prepaid+thirdParty)) > 0.005 {
			add("BR-DEX-09", fmt.Sprintf("Amount due for payment (BT-115=%.2f) shall equal Invoice total amount with VAT "+
				"(BT-112) - Paid amount (BT-113) + Rounding amount (BT-114) + the sum of Third party payment amounts "+
				"(BT-DEX-002), which is %.2f", payable, round2(inclusive-prepaid+thirdParty+rounding)))
		}
	}

	// BR-DEX-02, BR-DEX-03 and BR-DEX-10..14 have contexts naming /ubl:Invoice and
	// not /cn:CreditNote, so they cannot match a credit note. KoSIT wrote it that
	// way; honouring it is what keeps this an implementation of their rule rather
	// than of a rule with the same name.
	if root.name != "Invoice" {
		return
	}

	// BR-DEX-02, context /ubl:Invoice[$isExtension], two parts:
	//   every $l in cac:InvoiceLine[exists(cac:SubInvoiceLine)] satisfies
	//     $l/BT-131 = sum($l/cac:SubInvoiceLine/BT-131)
	//   and the same over every //cac:SubInvoiceLine that itself has sub-lines.
	bad := false
	var check func(parent *ciiNode)
	check = func(parent *ciiNode) {
		subs := parent.all("SubInvoiceLine")
		if len(subs) > 0 {
			own, ok := parseAmount(parent.str("LineExtensionAmount"))
			var sum float64
			for _, s := range subs {
				v, sok := parseAmount(s.str("LineExtensionAmount"))
				ok = ok && sok
				sum += v
			}
			if ok && math.Abs(round2(own)-round2(sum)) > 0.005 {
				bad = true
			}
		}
		for _, s := range subs {
			check(s)
		}
	}
	for _, li := range root.all("InvoiceLine") {
		check(li)
	}
	if bad {
		add("BR-DEX-02", "An Invoice line net amount (BT-131) should equal the sum of the Invoice line net amounts of "+
			"the Sub invoice lines (BG-DEX-01) directly below it")
	}

	// BR-DEX-03, context /ubl:Invoice[$isExtension]:
	//   not(exists(//cac:SubInvoiceLine/cac:Item[count(cac:ClassifiedTaxCategory) != 1]))
	for _, sub := range root.findAll("SubInvoiceLine") {
		for _, item := range sub.all("Item") {
			if len(item.all("ClassifiedTaxCategory")) != 1 {
				add("BR-DEX-03", "A Sub invoice line (BG-DEX-01) shall contain exactly one Sub invoice line VAT "+
					"information group (BG-DEX-06)")
			}
		}
	}

	// BR-DEX-10..14, context /ubl:Invoice/cac:PrepaidPayment[$isExtension].
	currency := normalizeSpace(root.str("DocumentCurrencyCode"))
	for _, pp := range root.all("PrepaidPayment") {
		if normalizeSpace(pp.str("ID")) == "" {
			add("BR-DEX-10", "The Third party payment type (BT-DEX-001) shall be provided when a Third party payment "+
				"group (BG-DEX-09) is present")
		}
		paid := pp.child("PaidAmount")
		if normalizeSpace(pp.str("PaidAmount")) == "" {
			add("BR-DEX-11", "The Third party payment amount (BT-DEX-002) shall be provided when a Third party payment "+
				"group (BG-DEX-09) is present")
		}
		if normalizeSpace(pp.str("InstructionID")) == "" {
			add("BR-DEX-12", "The Third party payment description (BT-DEX-003) shall be provided when a Third party "+
				"payment group (BG-DEX-09) is present")
		}
		// BR-DEX-13: string-length(substring-after(cbc:PaidAmount, '.')) <= 2, which
		// counts the characters after the first '.' and not the decimals of a number.
		if _, frac, found := strings.Cut(pp.str("PaidAmount"), "."); found && len(frac) > 2 {
			add("BR-DEX-13", "The Third party payment amount (BT-DEX-002) shall carry at most two decimals")
		}
		// BR-DEX-14: cbc:PaidAmount/@currencyID = parent::node()/cbc:DocumentCurrencyCode
		if paid.attr("currencyID") != currency {
			add("BR-DEX-14", fmt.Sprintf("The Third party payment amount (BT-DEX-002) currency (%q) shall be the "+
				"Invoice currency code (BT-5=%q)", paid.attr("currencyID"), currency))
		}
	}
}

// xrUBLCVDRules evaluates the ubl-cvd-pattern: the Clean Vehicles Directive
// sub-profile, which makes the contract and tender references mandatory and
// requires at least one line to carry a vehicle category and a clean-vehicle
// attribute.
func xrUBLCVDRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	// context (/ubl:Invoice | /cn:CreditNote)[$isCVD]
	if normalizeSpace(root.str("ContractDocumentReference", "ID")) == "" {
		add("BR-DE-CVD-01", "The Contract reference (BT-12) shall be provided in a CVD invoice")
	}
	if normalizeSpace(root.str("OriginatorDocumentReference", "ID")) == "" {
		add("BR-DE-CVD-02", "The Tender or lot reference (BT-17) shall be provided in a CVD invoice")
	}
	items := append(nodesAt(root, "InvoiceLine", "Item"), nodesAt(root, "CreditNoteLine", "Item")...)
	// BR-DE-CVD-03: at least one item carries both a 'CVD' classification scheme
	// and a 'cva' attribute name.
	found := false
	for _, item := range items {
		if xrUBLHasCVDClass(item) && xrUBLHasCVA(item) {
			found = true
		}
	}
	if !found {
		add("BR-DE-CVD-03", "A CVD invoice shall contain at least one Invoice line (BG-25) whose Item classification "+
			"identifier (BT-158) uses the scheme 'CVD' and whose Item attribute name (BT-160) is 'cva'")
	}
	for _, item := range items {
		if r.stopped() {
			return
		}
		// BR-DE-CVD-06-a / -06-b: the two halves of "'CVD' and 'cva' come in pairs,
		// exactly one of each per line".
		if xrUBLHasCVDClass(item) && xrCountCVA(item) != 1 {
			add("BR-DE-CVD-06-a", "An Invoice line whose Item classification identifier (BT-158) uses the scheme 'CVD' "+
				"shall carry exactly one Item attribute name (BT-160) with the value 'cva'")
		}
		if xrUBLHasCVA(item) && xrCountUBLCVDClass(item) != 1 {
			add("BR-DE-CVD-06-b", "An Invoice line carrying the Item attribute name (BT-160) 'cva' shall carry exactly "+
				"one Item classification identifier (BT-158) with the scheme 'CVD'")
		}
		for _, code := range nodesAt(item, "CommodityClassification", "ItemClassificationCode") {
			xrItemClassificationRules(code, add)
		}
		for _, prop := range item.all("AdditionalItemProperty") {
			if !xrHasChildValue(prop, "Name", "cva") {
				continue
			}
			if !xrCVACodes[normalizeSpace(prop.str("Value"))] {
				add("BR-DE-CVD-05", fmt.Sprintf("The Item attribute value (BT-161) for the attribute 'cva' (%q) is not "+
					"one of the permitted values", normalizeSpace(prop.str("Value"))))
			}
		}
	}
}

func xrUBLHasCVDClass(item *ciiNode) bool { return xrCountUBLCVDClass(item) > 0 }

func xrCountUBLCVDClass(item *ciiNode) int {
	n := 0
	for _, c := range nodesAt(item, "CommodityClassification", "ItemClassificationCode") {
		if normalizeSpace(c.attr("listID")) == "CVD" {
			n++
		}
	}
	return n
}

func xrUBLHasCVA(item *ciiNode) bool { return xrCountCVA(item) > 0 }

func xrCountCVA(item *ciiNode) int {
	n := 0
	for _, p := range item.all("AdditionalItemProperty") {
		if xrHasChildValue(p, "Name", "cva") {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// The CII binding
// ---------------------------------------------------------------------------

// xrCIIRules evaluates the rules of the cii-pattern whose context is a tree
// position rather than the whole document.
func xrCIIRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	tx := root.child("SupplyChainTradeTransaction").orNil()
	settle := tx.child("ApplicableHeaderTradeSettlement").orNil()
	agree := tx.child("ApplicableHeaderTradeAgreement").orNil()

	// BR-DE-18, context /rsm:CrossIndustryInvoice, over
	// ram:SpecifiedTradePaymentTerms/ram:Description[1].
	xrSkontoRule(xrFirstNotes(settle, "SpecifiedTradePaymentTerms", "Description"), add)

	// BR-DE-30 / BR-DE-31, context /rsm:CrossIndustryInvoice. CII has no element
	// for BG-19, so KoSIT reconstructs the group from its three mandatory terms:
	//   BT-89  ram:SpecifiedTradePaymentTerms/ram:DirectDebitMandateID
	//   BT-90  ram:CreditorReferenceID
	//   BT-91  ram:SpecifiedTradeSettlementPaymentMeans/ram:PayerPartyDebtorFinancialAccount/ram:IBANID
	// The group exists when any of the three is present, so BT-90 alone — a
	// creditor identifier with no mandate and no debited account — is a BG-19 that
	// is missing BT-91, which is a case the model's directDebitPresent could not
	// express because it was set from BT-89 and BT-91 only.
	bt89 := len(nodesAt(settle, "SpecifiedTradePaymentTerms", "DirectDebitMandateID")) > 0
	bt90 := len(settle.all("CreditorReferenceID")) > 0
	bt91 := len(nodesAt(settle, "SpecifiedTradeSettlementPaymentMeans", "PayerPartyDebtorFinancialAccount", "IBANID")) > 0
	if bt89 || bt90 || bt91 {
		if !((bt89 || bt91) && bt90) {
			add("BR-DE-30", "A direct debit (BG-19) shall carry the Bank assigned creditor identifier (BT-90)")
		}
		if !((bt89 || bt90) && bt91) {
			add("BR-DE-31", "A direct debit (BG-19) shall carry the Debited account identifier (BT-91)")
		}
	}

	// BR-DE-TMP-32, context /rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction,
	// so a document with no transaction group is outside the rule.
	lines := tx.all("IncludedSupplyChainTradeLineItem")
	if root.child("SupplyChainTradeTransaction") != nil {
		everyLinePeriod := true
		for _, li := range lines {
			if li.child("SpecifiedLineTradeSettlement", "BillingSpecifiedPeriod") == nil {
				everyLinePeriod = false
			}
		}
		hasDeliveryDate := len(nodesAt(tx, "ApplicableHeaderTradeDelivery", "ActualDeliverySupplyChainEvent", "OccurrenceDateTime")) > 0
		if !hasDeliveryDate && len(settle.all("BillingSpecifiedPeriod")) == 0 && !everyLinePeriod {
			add("BR-DE-TMP-32", xrTMP32Message)
		}
	}

	// BR-TMP-2, context ram:ApplicableHeaderTradeAgreement/
	//   ram:AdditionalReferencedDocument[ram:TypeCode = '916']:
	//   not(exists(ram:URIID)) or matches(ram:URIID, $XR-URL-REGEX)
	for _, d := range agree.all("AdditionalReferencedDocument") {
		if !xrHasChildValue(d, "TypeCode", "916") {
			continue
		}
		uri := d.child("URIID")
		if uri != nil && !xrURLRE.MatchString(uri.rawText()) {
			add("BR-TMP-2", "The External document location (BT-124) shall be an absolute URL with a valid scheme")
		}
	}

	// The three payment-means groups, context ram:SpecifiedTradeSettlementPaymentMeans
	// selected by BT-81. BG-18 is ram:ApplicableTradeSettlementFinancialCard and
	// BG-19 is the reconstructed group above, which is document-scoped: KoSIT's
	// BR-DE-23-b and BR-DE-24-b look at the document's BT-89/BT-90 and at this
	// group's BT-91.
	for _, pm := range settle.all("SpecifiedTradeSettlementPaymentMeans") {
		if r.stopped() {
			return
		}
		code := normalizeSpace(pm.str("TypeCode"))
		credit := pm.child("PayeePartyCreditorFinancialAccount") != nil
		card := pm.child("ApplicableTradeSettlementFinancialCard") != nil
		groupBT91 := len(nodesAt(pm, "PayerPartyDebtorFinancialAccount", "IBANID")) > 0
		directDebit := bt89 || bt90 || groupBT91
		switch {
		case code == "30" || code == "58":
			if code == "58" && !validIBAN(pm.str("PayeePartyCreditorFinancialAccount", "IBANID")) {
				add("BR-DE-19", "The Payment account identifier (BT-84) shall be a valid IBAN for a SEPA credit transfer")
			}
			if !credit {
				add("BR-DE-23-a", "A Payment means type code (BT-81) for a credit transfer (30, 58) requires the "+
					"Credit transfer group (BG-17)")
			}
			if card || directDebit {
				add("BR-DE-23-b", "A Payment means type code (BT-81) for a credit transfer (30, 58) shall carry "+
					"neither the Payment card group (BG-18) nor the Direct debit group (BG-19)")
			}
		case code == "48" || code == "54" || code == "55":
			if !card {
				add("BR-DE-24-a", "A Payment means type code (BT-81) for a card payment (48, 54, 55) requires the "+
					"Payment card information group (BG-18)")
			}
			if credit || directDebit {
				add("BR-DE-24-b", "A Payment means type code (BT-81) for a card payment (48, 54, 55) shall carry "+
					"neither the Credit transfer group (BG-17) nor the Direct debit group (BG-19)")
			}
		case code == "59":
			if !validIBAN(pm.str("PayerPartyDebtorFinancialAccount", "IBANID")) {
				add("BR-DE-20", "The Debited account identifier (BT-91) shall be a valid IBAN for a SEPA direct debit")
			}
			if !directDebit {
				add("BR-DE-25-a", "A Payment means type code (BT-81) for a direct debit (59) requires the "+
					"Direct debit group (BG-19)")
			}
			// BR-DE-25-b names four elements, two more than the UBL binding does:
			// the payee and payer financial institutions (BT-86) as well as BG-17
			// and BG-18.
			if credit || card || pm.child("PayeeSpecifiedCreditorFinancialInstitution") != nil ||
				pm.child("PayerSpecifiedDebtorFinancialInstitution") != nil {
				add("BR-DE-25-b", "A Payment means type code (BT-81) for a direct debit (59) shall carry neither the "+
					"Credit transfer group (BG-17), the Payment card group (BG-18) nor a financial institution (BT-86)")
			}
		}
	}

	// BR-TMP-3, context ram:IncludedSupplyChainTradeLineItem:
	//   not(gross/ram:BasisQuantity and net/ram:BasisQuantity) or
	//   (gross/ram:BasisQuantity = net/ram:BasisQuantity and
	//    (not(net/ram:BasisQuantity/@unitCode and gross/ram:BasisQuantity/@unitCode) or
	//     gross/ram:BasisQuantity/@unitCode = net/ram:BasisQuantity/@unitCode))
	//
	// The comparison is between untyped nodes, so it is a string comparison: "1"
	// and "1.0" are two different item price base quantities to this rule, as they
	// are to a reference validator.
	for _, li := range lines {
		if r.stopped() {
			return
		}
		gross := li.child("SpecifiedLineTradeAgreement", "GrossPriceProductTradePrice", "BasisQuantity")
		net := li.child("SpecifiedLineTradeAgreement", "NetPriceProductTradePrice", "BasisQuantity")
		if gross == nil || net == nil {
			continue
		}
		sameUnit := !(net.hasAttr("unitCode") && gross.hasAttr("unitCode")) || gross.attr("unitCode") == net.attr("unitCode")
		if strings.TrimSpace(gross.text) != strings.TrimSpace(net.text) || !sameUnit {
			add("BR-TMP-3", "An Item price base quantity (BT-149) given on both the gross and the net price shall "+
				"carry the same value and the same unit of measure (BT-150)")
		}
	}
}

// xrCIIExtensionRules evaluates the cii-extension-pattern. It is not the UBL
// pattern's twin: KoSIT publishes seven of the fifteen BR-DEX-* identifiers for
// CII, and BR-DEX-09 is not among them — an EXTENSION invoice in CII has its
// amount due checked by CEN's BR-CO-16 and nothing else, which is why
// xrechnungSuppressedForExtension suppresses BR-CO-16 for UBL alone.
func xrCIIExtensionRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	// BR-DEX-01, context ram:AttachmentBinaryObject[$isExtension].
	for _, b := range root.findAll("AttachmentBinaryObject") {
		if !xrExtMIME[b.attr("mimeCode")] {
			add("BR-DEX-01", fmt.Sprintf("Attached document (BT-125) MIME code %q is not one the EXTENSION permits",
				b.attr("mimeCode")))
		}
	}
	// BR-DEX-15, context ram:IncludedSupplyChainTradeLineItem/
	//   ram:AssociatedDocumentLineDocument[$isExtension]: not(exists(//ram:ParentLineID)).
	//
	// The test is document-wide inside a per-line context, so KoSIT reports it once
	// per line item; one finding says the same thing about the same document.
	if len(root.findAll("ParentLineID")) > 0 &&
		len(nodesAt(root, "SupplyChainTradeTransaction", "IncludedSupplyChainTradeLineItem", "AssociatedDocumentLineDocument")) > 0 {
		add("BR-DEX-15", "This CII invoice uses Sub invoice lines (ram:ParentLineID), which XRechnung does not support")
	}
	// BR-DEX-04..08. The CII contexts are wider than the UBL ones and two of them
	// are exclusions rather than paths: BR-DEX-04 is every ram:GlobalID with a
	// scheme that is neither an item's nor a ship-to party's, and BR-DEX-05 is
	// every ram:ID with a scheme that is not a tax registration's.
	xrCIISchemeRule(r, root, add, "BR-DEX-04", "party identifier (BT-29/BT-46/BT-60)", xrISO6523Ext,
		"GlobalID", []string{"SpecifiedTradeProduct", "ShipToTradeParty"})
	xrCIISchemeRule(r, root, add, "BR-DEX-05", "identifier", xrISO6523Ext,
		"ID", []string{"SpecifiedTaxRegistration"})
	for _, id := range xrFindAt(root, []string{"SpecifiedTradeProduct", "GlobalID"}) {
		xrCheckScheme(id, add, "BR-DEX-06", "item standard identifier (BT-157)", xrISO6523Ext)
	}
	for _, id := range xrFindAt(root, []string{"URIUniversalCommunication", "URIID"}) {
		xrCheckScheme(id, add, "BR-DEX-07", "electronic address (BT-34/BT-49)", xrCEFEASExt)
	}
	for _, id := range xrFindAt(root, []string{"ApplicableHeaderTradeDelivery", "ShipToTradeParty", "GlobalID"}) {
		xrCheckScheme(id, add, "BR-DEX-08", "deliver-to location identifier (BT-71)", xrISO6523Ext)
	}
}

// xrCIICVDRules evaluates the cii-cvd-pattern.
func xrCIICVDRules(r *run, root *ciiNode, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	tx := root.child("SupplyChainTradeTransaction").orNil()
	agree := tx.child("ApplicableHeaderTradeAgreement")
	// context .../ram:ApplicableHeaderTradeAgreement, so an invoice with no
	// agreement group is outside these two rules entirely.
	if agree != nil {
		if normalizeSpace(agree.str("ContractReferencedDocument", "IssuerAssignedID")) == "" {
			add("BR-DE-CVD-01", "The Contract reference (BT-12) shall be provided in a CVD invoice")
		}
		tender := false
		for _, d := range agree.all("AdditionalReferencedDocument") {
			if xrHasChildValue(d, "TypeCode", "50") && normalizeSpace(d.str("IssuerAssignedID")) != "" {
				tender = true
			}
		}
		if !tender {
			add("BR-DE-CVD-02", "The Tender or lot reference (BT-17) shall be provided in a CVD invoice")
		}
	}
	products := nodesAt(tx, "IncludedSupplyChainTradeLineItem", "SpecifiedTradeProduct")
	found := false
	for _, prod := range products {
		if xrCIIHasCVDClass(prod) && xrCIIHasCVA(prod) {
			found = true
		}
	}
	if !found {
		add("BR-DE-CVD-03", "A CVD invoice shall contain at least one Invoice line (BG-25) whose Item classification "+
			"identifier (BT-158) uses the scheme 'CVD' and whose Item attribute name (BT-160) is 'cva'")
	}
	for _, prod := range products {
		if r.stopped() {
			return
		}
		if xrCIIHasCVDClass(prod) && xrCountCIICVA(prod) != 1 {
			add("BR-DE-CVD-06-a", "An Invoice line whose Item classification identifier (BT-158) uses the scheme 'CVD' "+
				"shall carry exactly one Item attribute name (BT-160) with the value 'cva'")
		}
		if xrCIIHasCVA(prod) && xrCountCIICVDClass(prod) != 1 {
			add("BR-DE-CVD-06-b", "An Invoice line carrying the Item attribute name (BT-160) 'cva' shall carry exactly "+
				"one Item classification identifier (BT-158) with the scheme 'CVD'")
		}
		for _, code := range nodesAt(prod, "DesignatedProductClassification", "ClassCode") {
			xrItemClassificationRules(code, add)
		}
		for _, ch := range prod.all("ApplicableProductCharacteristic") {
			if !xrHasChildValue(ch, "Description", "cva") {
				continue
			}
			if !xrCVACodes[normalizeSpace(ch.str("Value"))] {
				add("BR-DE-CVD-05", fmt.Sprintf("The Item attribute value (BT-161) for the attribute 'cva' (%q) is not "+
					"one of the permitted values", normalizeSpace(ch.str("Value"))))
			}
		}
	}
}

func xrCIIHasCVDClass(prod *ciiNode) bool { return xrCountCIICVDClass(prod) > 0 }

func xrCountCIICVDClass(prod *ciiNode) int {
	n := 0
	for _, c := range nodesAt(prod, "DesignatedProductClassification", "ClassCode") {
		if normalizeSpace(c.attr("listID")) == "CVD" {
			n++
		}
	}
	return n
}

func xrCIIHasCVA(prod *ciiNode) bool { return xrCountCIICVA(prod) > 0 }

func xrCountCIICVA(prod *ciiNode) int {
	n := 0
	for _, ch := range prod.all("ApplicableProductCharacteristic") {
		if xrHasChildValue(ch, "Description", "cva") {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Shared rule bodies and code lists
// ---------------------------------------------------------------------------

// xrItemClassificationRules is BR-TMP-CVD-01 and BR-DE-CVD-04, which share a
// context — the item classification code element — in both bindings.
func xrItemClassificationRules(code *ciiNode, add func(rule, msg string)) {
	list := normalizeSpace(code.attr("listID"))
	// BR-TMP-CVD-01: the scheme is UNTDID 7143 with 'CVD' added.
	if strings.Contains(list, " ") || !(list == "CVD" || en16931ItemClassCodes[list]) {
		add("BR-TMP-CVD-01", fmt.Sprintf("Item classification identifier (BT-158) scheme %q is not in UNTDID 7143 "+
			"extended with 'CVD'", list))
	}
	// BR-DE-CVD-04: not(@listID = 'CVD') or the value is a vehicle category.
	if list == "CVD" && !xrCVDVehicleCategories[normalizeSpace(code.text)] {
		add("BR-DE-CVD-04", fmt.Sprintf("Item classification identifier (BT-158) %q with the scheme 'CVD' is not one of "+
			"the permitted vehicle categories", normalizeSpace(code.text)))
	}
}

// xrCIISchemeRule applies one CII EXTENSION scheme rule whose context is every
// element with a given name that has no ancestor among excluded.
func xrCIISchemeRule(r *run, root *ciiNode, add func(rule, msg string), rule, term string,
	codes func(string) bool, name string, excluded []string) {
	if r.stopped() {
		return
	}
	skip := map[string]bool{}
	for _, e := range excluded {
		skip[e] = true
	}
	var walk func(n *ciiNode, blocked bool)
	walk = func(n *ciiNode, blocked bool) {
		if n.name == name && !blocked {
			xrCheckScheme(n, add, rule, term, codes)
		}
		for _, c := range n.children {
			walk(c, blocked || skip[n.name])
		}
	}
	walk(root, false)
}

// xrCheckScheme is the assertion the five BR-DEX-04..08 rules share:
//
//	not(contains(normalize-space(@schemeID), ' ')) and
//	contains($LIST, concat(' ', normalize-space(@schemeID), ' '))
//
// An element with no @schemeID is outside the rule's context and is not checked.
func xrCheckScheme(node *ciiNode, add func(rule, msg string), rule, term string, codes func(string) bool) {
	if !node.hasAttr("schemeID") {
		return
	}
	s := normalizeSpace(node.attr("schemeID"))
	if strings.Contains(s, " ") || !codes(s) {
		add(rule, fmt.Sprintf("The scheme identifier %q on the %s is not in the code list the EXTENSION permits", s, term))
	}
}

// xrFindAt returns every node the given chain of local names reaches from
// anywhere in the tree — `//a/b` rather than `/root/a/b`, which is how the UBL
// EXTENSION contexts are written.
func xrFindAt(root *ciiNode, path []string) []*ciiNode {
	var out []*ciiNode
	for _, start := range root.findAll(path[0]) {
		if len(path) == 1 {
			out = append(out, start)
			continue
		}
		out = append(out, nodesAt(start, path[1:]...)...)
	}
	return out
}

// xrHasChildValue is XPath's `child = 'v'` over a node set: true when any child
// with that name has that value once normalized.
func xrHasChildValue(n *ciiNode, name, value string) bool {
	for _, c := range n.all(name) {
		if normalizeSpace(c.text) == value {
			return true
		}
	}
	return false
}

// xrFirstNotes is `<container>/<note>[1]` — the first note of *each* container,
// which is a sequence and not one value. BR-DE-18 evaluates over all of them.
func xrFirstNotes(n *ciiNode, container, note string) []string {
	var out []string
	for _, c := range n.all(container) {
		if first := c.child(note); first != nil {
			out = append(out, first.rawText())
		}
	}
	return out
}

// rawText is an element's string value with nothing done to it. Most rules want
// str(), which trims; the two that match a regular expression against an element
// — BR-DE-18 and BR-TMP-2 — must not, because their patterns are anchored and
// their subject is the value a reference validator sees.
func (n *ciiNode) rawText() string {
	if n == nil {
		return ""
	}
	return n.text
}

// The settlement-discount ("Skonto") format of BR-DE-18, from common.sch:
//
//	$XR-SKONTO-REGEX  (^|\r?\n)#(SKONTO)#TAGE=([0-9]+#PROZENT=[0-9]+\.[0-9]{2})(#BASISBETRAG=-?[0-9]+\.[0-9]{2})?#$
//
// The alternation at the front is vestigial: the assertion applies the pattern to
// normalize-space($line) of one tokenized line, which cannot contain a newline,
// so only the ^ arm can ever match.
var (
	xrSkontoRE      = regexp.MustCompile(`^#SKONTO#TAGE=[0-9]+#PROZENT=[0-9]+\.[0-9]{2}(#BASISBETRAG=-?[0-9]+\.[0-9]{2})?#$`)
	xrSkontoLineRE  = regexp.MustCompile(`\r?\n`)
	xrSkontoEntryRE = regexp.MustCompile(`#[^\n]+#`)
	xrSkontoTailRE  = regexp.MustCompile(`^[ \t\r\n]*\n`)
	// $XR-URL-REGEX: ^([a-zA-Z])([a-zA-Z0-9+.-])+:.*
	xrURLRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]+:`)
)

// xrSkontoRule is BR-DE-18 over the payment-terms notes of either binding.
//
// The assertion is one `every ... satisfies A and B`, and both halves are inside
// the quantifier, which is what makes it vacuously true for a note with no
// settlement-discount line at all — the common case, and the reason a plain
// "Zahlbar sofort ohne Abzug." payment term does not trip a format rule about a
// syntax it never uses.
//
//	A: each line starting with '#' matches $XR-SKONTO-REGEX once normalized.
//	B: tokenize(note, '#.+#')[last()] matches '^\s*\n' — i.e. what follows the
//	   last discount entry begins with a newline. '.' excludes newline in XPath,
//	   so an entry cannot span lines.
func xrSkontoRule(notes []string, add func(rule, msg string)) {
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
			if !xrSkontoRE.MatchString(line) || !tailOK {
				add("BR-DE-18", "A settlement-discount line in the Payment terms (BT-20) does not match the required "+
					"format #SKONTO#TAGE=n#PROZENT=n.nn[#BASISBETRAG=n.nn]# followed by a line break")
				return
			}
		}
	}
}

const xrTMP32Message = "An Invoice should state the delivery date with either an Actual delivery date (BT-72), an " +
	"Invoicing period (BG-14), or an Invoice line period (BG-26) on every line"

// xrExtMIME is the attachment MIME code set of the EXTENSION: the EN 16931 list
// BR-CL-24 checks, plus application/xml.
var xrExtMIME = func() map[string]bool {
	m := map[string]bool{"application/xml": true}
	for k := range en16931MIME {
		m[k] = true
	}
	return m
}()

// xrDIGACodes are the three scheme identifiers the EXTENSION adds to both of the
// code lists BR-DEX-04..08 draw on ($DIGA-CODES in common.sch). They identify
// the German digital-health-application registers.
var xrDIGACodes = map[string]bool{"XR01": true, "XR02": true, "XR03": true}

// xrISO6523Ext is $ISO-6523-ICD-EXT-CODES.
func xrISO6523Ext(s string) bool { return xrDIGACodes[s] || en16931ICD[s] }

// xrCEFEASExt is $CEF-EAS-EXT-CODES. KoSIT's copy of the CEF EAS list carries
// 0219 and 0220, which the list CEN's BR-CL-25 draws on does not, so they are
// named here rather than left to disagree silently: an EXTENSION invoice with
// either would otherwise be refused for a scheme its own authority publishes.
func xrCEFEASExt(s string) bool {
	return xrDIGACodes[s] || en16931EAS[s] || s == "0219" || s == "0220"
}

// xrCVDVehicleCategories is $CVD-VEHICLE-CATEGORY and xrCVACodes is $CVA-CODES.
var (
	xrCVDVehicleCategories = map[string]bool{"M1": true, "M2": true, "M3": true, "N1": true, "N2": true, "N3": true}
	xrCVACodes             = map[string]bool{"clean": true, "zero-emission": true, "other": true}
)
