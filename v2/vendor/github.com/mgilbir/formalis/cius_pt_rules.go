package formalis

import "strings"

// The CIUS-PT rule bodies, transcribed from AT/eSPap's Schematron.
//
// # What the artefact is, and how it has to be read
//
// urn_feap.gov.pt_CIUS-PT_2.1.1.sch is an ISO Schematron in CEN's abstract/
// concrete shape: three abstract patterns (`model`, `syntax`, `condition`) declare
// `<rule context="$Name">` with `<assert test="$RULE-ID">`, and a UBL pattern
// instantiates each with `<param name="..." value="..."/>`. Neither half is a rule
// on its own. A reader that opens the abstract file sees `$BR-CIUS-PT-64` and
// learns nothing; a reader that opens the UBL file sees an XPath expression with
// no context and no polarity. Every condition below is the resolved pair, and
// cius_pt_artefact_test.go re-resolves them out of the vendored files on every run
// so that a transcription cannot drift from what AT publishes.
//
// Two properties of that shape decide whether a transcription is right:
//
//   - **assert and report are opposite.** `<assert>` fires when its test is
//     *false*; `<report>` fires when its test is *true*. AT uses both, and the
//     split is not cosmetic: 33 of the 73 identifiers here are `<report>`, and every
//     one of those is a "this optional group is present but incomplete" rule whose
//     test reads as the *defect*. Transcribing one as an assert inverts it, which
//     turns a conditional completeness rule into a rule that fires on every
//     conforming document that omits the optional group.
//   - **exists() is satisfied by an empty element.** Most of these are existence
//     tests. `<cbc:CityName/>` satisfies `exists(cbc:CityName)`. That is why these
//     rules read the parsed tree rather than the syntax-neutral model: the model
//     carries a trimmed string per term, in which an element that is present and
//     empty is indistinguishable from one that is absent. Four Peppol rules
//     reported invoices OpenPEPPOL's own fixtures hold up as conforming for exactly
//     this reason (C32), and seven CIUS-PT rules had the same defect before PR 22.
//
// # Why an XPath-shaped path helper
//
// ciiNode.child follows the *first* match at every step, which is not what a
// location path does. `exists(cac:PartyTaxScheme/cbc:CompanyID)` is true when *any*
// tax scheme carries a CompanyID; `child("PartyTaxScheme", "CompanyID")` answers
// nil when the first one does not. Nineteen of these rules step through a group
// that legally repeats — PartyTaxScheme, PartyIdentification, PartyName,
// AllowanceCharge, AdditionalDocumentReference, PayeeFinancialAccount,
// TaxSubtotal — so ptPath expands at every step the way XPath does, and every rule
// below is written as a path plus a polarity rather than as a chain of accessors.
//
// # The two versions
//
// `make cius-schematron` vendors 2.0.0 and 2.1.1, and both are live: phive-rules
// registers a validation set for each, and ships ten sample instances for each. The
// two publish the *same 73 identifiers*, and 59 of them with byte-identical resolved
// conditions. The fourteen that differ differ in one way only: 2.1.1 widened the VAT
// category-code sets they select on, accepting 'RED'/'INT' beside 'AA', 'NOR' beside
// 'S' and 'ISE' beside 'E'. They are BR-CIUS-PT-12, 14, 15, 16, 17, 18 and all eight
// BR-AA-*.
//
// This package evaluates 2.1.1, the current version.
// TestCIUSPTVersionsAgreeExceptOnTheCategoryAliases pins the divergence to exactly
// those fourteen identifiers, and checks that each differs only by string literals
// and not by the element steps it references, so that a third version cannot widen
// it unnoticed.
//
// Dispatching per document is not available in any case: all twenty vendored
// instances — including the ten filed under 2.1.1 — declare BT-24 as
// `urn:cen.eu:en16931:2017#compliant#urn:feap.gov.pt:CIUS-PT:1.0.0.`, so the
// specification identifier does not carry the version and there is nothing in the
// document to dispatch on. The choice is observable only on a document that uses one
// of the four alias codes, and no document in the 1,690-document corpus does: the
// only occurrences of the string "RED" in the whole corpus are two Turkish
// cbc:ResponseCode elements.

// ptPath returns every node reached from n by following the given child element
// names, expanding at every step the way an XPath location path does.
//
// It is the one traversal these rules need and it is deliberately not
// ciiNode.child: child follows the first match at each step, which answers the
// wrong question for every group UBL allows to repeat. See the file comment.
func ptPath(n *ciiNode, path ...string) []*ciiNode {
	if n == nil {
		return nil
	}
	cur := []*ciiNode{n}
	for _, name := range path {
		var next []*ciiNode
		for _, c := range cur {
			next = append(next, c.all(name)...)
		}
		if len(next) == 0 {
			return nil
		}
		cur = next
	}
	return cur
}

// ptPathFrom is ptPath over a set of context nodes, which is what a context that
// itself repeats (each invoice line, each payment instruction) needs.
func ptPathFrom(ns []*ciiNode, path ...string) []*ciiNode {
	var out []*ciiNode
	for _, n := range ns {
		out = append(out, ptPath(n, path...)...)
	}
	return out
}

// ptExists is XPath's exists(): true when the path reaches at least one element,
// whatever that element contains. An element written empty exists.
func ptExists(n *ciiNode, path ...string) bool { return len(ptPath(n, path...)) > 0 }

// ptExistsFrom is ptExists over a set of context nodes.
func ptExistsFrom(ns []*ciiNode, path ...string) bool { return len(ptPathFrom(ns, path...)) > 0 }

// ptCharge filters an AllowanceCharge set the way AT's predicate does:
// `[cbc:ChargeIndicator='true']` and `[cbc:ChargeIndicator='false']`.
//
// AT compares against the *strings* "true" and "false" where CEN's own binding
// compares against the booleans true() and false(). The difference is real —
// xs:boolean also spells them "1" and "0" — and it is transcribed rather than
// improved: a document writing <cbc:ChargeIndicator>0</cbc:ChargeIndicator> is
// outside the context of AT's rule, so reporting it would be a finding AT's own
// validator does not produce.
func ptCharge(acs []*ciiNode, want string) []*ciiNode {
	var out []*ciiNode
	for _, a := range acs {
		for _, ci := range a.all("ChargeIndicator") {
			if strings.TrimSpace(ci.text) == want {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// ptAnyText reports whether any node in ns has the given trimmed text, which is
// XPath's existential node-set-to-string comparison: `(a/b/c) = 'VAT'` is true
// when any of the c nodes holds that string.
func ptAnyText(ns []*ciiNode, want string) bool {
	for _, n := range ns {
		if strings.TrimSpace(n.text) == want {
			return true
		}
	}
	return false
}

// ptCategoryIn reports whether a cac:TaxCategory or cac:ClassifiedTaxCategory
// element carries one of the given category codes, matching AT's
// `[normalize-space(cbc:ID) = 'S' or normalize-space(cbc:ID) = 'NOR']` predicate.
func ptCategoryIn(cat *ciiNode, codes ...string) bool {
	for _, id := range cat.all("ID") {
		v := strings.TrimSpace(id.text)
		for _, c := range codes {
			if v == c {
				return true
			}
		}
	}
	return false
}

// ptCategoriesIn filters a category-element set to those carrying one of the codes.
func ptCategoriesIn(cats []*ciiNode, codes ...string) []*ciiNode {
	var out []*ciiNode
	for _, c := range cats {
		if ptCategoryIn(c, codes...) {
			out = append(out, c)
		}
	}
	return out
}

// The four VAT category-code sets AT's 2.1.1 rules select on. EN 16931's own
// BT-118 code list holds none of the four aliases and does not hold 'AA' either;
// they are AT's, and BR-CL-17 in the core reports a document that uses them. That
// is a separate finding from these rules and not a substitute for them: a
// Portuguese invoice AT accepts is one this package reports BR-CL-17 on, and
// closing that is EN 16931 code-list work rather than CIUS-PT work.
var (
	ptCatLower    = []string{"AA", "RED", "INT"} // "Lower rate"
	ptCatStandard = []string{"S", "NOR"}         // "Standard rated"
	ptCatExempt   = []string{"E", "ISE"}         // "Exempt from VAT"

	// ptCatLowerItemAttr is BR-CIUS-PT-13's, and it is 'AA' alone. AT widened the
	// "Lower rate" set in six places when 2.1.1 landed and did not widen it in
	// $VATAA_AdditionalLine, whose predicate is still
	// `cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'AA']` in both versions —
	// unlike $VATS_AdditionalLine and $VATE_AdditionalLine beside it, which did get
	// their aliases. Reusing ptCatLower here would report a 'RED'-category line
	// carrying an exemption reason, which AT's own validator does not.
	ptCatLowerItemAttr = []string{"AA"}
)

// The two magic cbc:Name values CIUS-PT uses to carry BT-160 (the VAT exemption
// reason) as an item attribute. UBL has no place for an exemption reason on a
// line, so AT encodes it as a cac:AdditionalItemProperty whose cbc:Name is one of
// these two literals and whose cbc:Value is the reason or its code.
//
// BR-CIUS-PT-13 and -15 test them with starts-with(normalize-space(.), …) and
// BR-CIUS-PT-17 with matches(., '^(#(…)#)$'). The two are not the same test and
// both are transcribed as written: -13 and -15 forbid a *prefix*, -17 requires an
// *exact* value.
const (
	ptExemptionReasonCode = "#TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY#"
	ptExemptionReason     = "#TAXEXEMPTIONREASON@CLASSIFIEDTAXCATEGORY#"
)

// ptNodes is every population the 65 rules read, gathered in one walk.
//
// The rules are bound to twenty-five distinct contexts and this package validates
// 1,690 documents on every test run; resolving each context independently would
// walk the document twenty-five times to learn what one walk knows. It follows the
// shape gatherUBLSyntaxNodes already uses for CEN's UBL binding rules, for the same
// reason.
//
// Every field names the context parameter it resolves, because that is the thing a
// reviewer has to check against the artefact.
type ptNodes struct {
	root         *ciiNode   // $Invoice          //ubl:Invoice | //cn:CreditNote
	isCreditNote bool       // the document element, read by BR-CIUS-PT-25
	seller       *ciiNode   // $Seller           cac:AccountingSupplierParty
	buyer        *ciiNode   // $Buyer            cac:AccountingCustomerParty
	sellerAddr   []*ciiNode // $Seller_postal_address
	buyerAddr    []*ciiNode // $Buyer_postal_address
	taxRepAddr   []*ciiNode // $Tax_Representative_postal_address
	payees       []*ciiNode // $Payee            cac:PayeeParty
	deliveries   []*ciiNode // $Delivery         cac:Delivery
	deliverTo    []*ciiNode // $Deliver_to_address  cac:Delivery/cac:DeliveryLocation/cac:Address
	totals       []*ciiNode // $Document_totals  cac:LegalMonetaryTotal
	docAllowance []*ciiNode // $Document_level_allowances
	docCharge    []*ciiNode // $Document_level_charges
	payMeans     []*ciiNode // $Payment_instructions  cac:PaymentMeans
	payTerms     []*ciiNode // $Payment_terms         cac:PaymentTerms
	addDocRefs   []*ciiNode // $Additional_supporting_documents
	lines        []*ciiNode // $Invoice_Line     cac:InvoiceLine | cac:CreditNoteLine
	lineItems    []*ciiNode // $Invoice_Line_Item
	linePrices   []*ciiNode // $Invoice_Line_Price
	// creditNoteItems marks the members of lineItems that hang off a
	// cac:CreditNoteLine, for the one rule whose context treats the two branches
	// differently. See ptStandardRateCodes.
	creditNoteItems map[*ciiNode]bool
	breakdowns      []*ciiNode // $VAT_breakdown    cac:TaxTotal/cac:TaxSubtotal
	// bdCategories is $VAT_breakdown/cac:TaxCategory, from which $VATAA, $VATS and
	// $VATE are the three code-filtered subsets.
	bdCategories []*ciiNode

	// The two document-wide populations the BR-AA-* rules reach for with `//`.
	// They are descendant axes and not child axes: BR-AA-01 counts every
	// cac:AllowanceCharge in the document, line-level ones included, where
	// BR-AA-06/07 are bound to the document-level ones only. Keeping both is what
	// makes that distinction visible rather than accidental.
	allClassifiedCats []*ciiNode // //cac:ClassifiedTaxCategory
	allAllowCharges   []*ciiNode // //cac:AllowanceCharge
}

// gatherPTNodes resolves every CIUS-PT context against the document element.
//
// Each assignment is a transcription of one <param name="..."> in
// urn_feap.gov.pt_CIUS-PT_2.1.1-UBL-model.sch. Note where the artefact says
// `//cac:InvoiceLine` (document-wide) rather than `cac:InvoiceLine` (a child of
// the document element): the line contexts are children, and only the *allowance*
// contexts reach with `//`. Getting that backwards would apply the line rules to
// a sub-invoice line, which UBL permits and CIUS-PT does not describe.
func gatherPTNodes(root *ciiNode) *ptNodes {
	g := &ptNodes{root: root, isCreditNote: root.name == "CreditNote"}
	g.seller = root.child("AccountingSupplierParty").orNil()
	g.buyer = root.child("AccountingCustomerParty").orNil()
	g.sellerAddr = ptPath(root, "AccountingSupplierParty", "Party", "PostalAddress")
	g.buyerAddr = ptPath(root, "AccountingCustomerParty", "Party", "PostalAddress")
	g.taxRepAddr = ptPath(root, "TaxRepresentativeParty", "PostalAddress")
	g.payees = root.all("PayeeParty")
	g.deliveries = root.all("Delivery")
	g.deliverTo = ptPathFrom(g.deliveries, "DeliveryLocation", "Address")
	g.totals = root.all("LegalMonetaryTotal")
	docAC := root.all("AllowanceCharge")
	g.docAllowance = ptCharge(docAC, "false")
	g.docCharge = ptCharge(docAC, "true")
	g.payMeans = root.all("PaymentMeans")
	g.payTerms = root.all("PaymentTerms")
	g.addDocRefs = root.all("AdditionalDocumentReference")
	invoiceLines, creditNoteLines := root.all("InvoiceLine"), root.all("CreditNoteLine")
	g.lines = append(append([]*ciiNode{}, invoiceLines...), creditNoteLines...)
	g.lineItems = ptPathFrom(g.lines, "Item")
	g.creditNoteItems = map[*ciiNode]bool{}
	for _, it := range ptPathFrom(creditNoteLines, "Item") {
		g.creditNoteItems[it] = true
	}
	g.linePrices = ptPathFrom(g.lines, "Price")
	g.breakdowns = ptPath(root, "TaxTotal", "TaxSubtotal")
	g.bdCategories = ptPathFrom(g.breakdowns, "TaxCategory")
	g.allClassifiedCats = root.findAll("ClassifiedTaxCategory")
	g.allAllowCharges = root.findAll("AllowanceCharge")
	return g
}

// validateCIUSPTRules applies the 65 BR-CIUS-PT-* rules to a UBL document.
//
// The order below is the artefact's: by context, and within a context by
// identifier, so that a reviewer can read the two side by side. Each rule carries
// its resolved context and condition in a comment, and the polarity — assert or
// report — is named, because the two are opposite and 33 of the 73 are reports.
func validateCIUSPTRules(r *run, inv *en16931Invoice, root *ciiNode) []Violation {
	if inv.syntax != "UBL" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceCIUSPT)
	g := gatherPTNodes(root)

	ptInvoiceRules(g, add)
	ptPartyRules(g, add)
	ptReferenceRules(g, add)
	ptDeliveryRules(g, add)
	ptPaymentRules(g, add)
	ptLineRules(g, add)
	ptVATRules(g, add)
	ptLowerRateRules(g, add)

	// And the 291 generated DT-CIUS-PT-* assertions, which are AT's own XPath run
	// against the same tree. They are a separate Schematron pattern pair, so they
	// see every node these rules saw: ISO Schematron's first-match-wins is per
	// pattern and not per document.
	ptDTValidate(r, root, add)
	return out
}

// ptInvoiceRules holds the rules bound to $Invoice and $Document_totals — the
// document element and its totals group.
func ptInvoiceRules(g *ptNodes, add func(rule, msg string)) {
	seller, buyer := g.seller, g.buyer

	// BR-CIUS-PT-01, assert, $Invoice:
	//   exists(cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)
	// Any tax scheme: that the scheme is VAT is BR-CIUS-PT-02's separate job.
	if !ptExists(seller, "Party", "PartyTaxScheme", "CompanyID") {
		add("BR-CIUS-PT-01", "the Invoice shall contain the Seller VAT identifier (BT-31)")
	}
	// BR-CIUS-PT-02, assert, $Invoice:
	//   (cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme/cbc:ID) = 'VAT'
	// A node-set compared with a string is existential in XPath, so one tax scheme
	// spelt VAT satisfies it however many others the party declares.
	if !ptAnyText(ptPath(seller, "Party", "PartyTaxScheme", "TaxScheme", "ID"), "VAT") {
		add("BR-CIUS-PT-02", "the Invoice shall contain the Seller VAT tax scheme (VAT)")
	}
	// BR-CIUS-PT-03, assert, $Invoice:
	//   exists(cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cbc:CompanyID)
	if !ptExists(buyer, "Party", "PartyTaxScheme", "CompanyID") {
		add("BR-CIUS-PT-03", "the Invoice shall contain the Buyer VAT identifier (BT-48)")
	}
	// BR-CIUS-PT-04, assert, $Invoice:
	//   (cac:AccountingCustomerParty/cac:Party/cac:PartyTaxScheme/cac:TaxScheme/cbc:ID) = 'VAT'
	if !ptAnyText(ptPath(buyer, "Party", "PartyTaxScheme", "TaxScheme", "ID"), "VAT") {
		add("BR-CIUS-PT-04", "the Invoice shall contain the Buyer VAT tax scheme (VAT)")
	}
	// BR-CIUS-PT-10, assert, $Invoice: exists(cac:LegalMonetaryTotal)
	if len(g.totals) == 0 {
		add("BR-CIUS-PT-10", "the Invoice shall contain the Document totals (BG-22)")
	}
	// BR-CIUS-PT-11, assert, $Invoice: exists(cac:TaxTotal/cbc:TaxAmount)
	if !ptExists(g.root, "TaxTotal", "TaxAmount") {
		add("BR-CIUS-PT-11", "the Invoice shall contain the Total VAT amount (BT-110)")
	}
	// BR-CIUS-PT-25, report, $Invoice:
	//   exists(//cn:CreditNote) and not(cac:BillingReference)
	// The first conjunct is the document element, so this is "a credit note with no
	// preceding-invoice reference".
	if g.isCreditNote && len(g.root.all("BillingReference")) == 0 {
		add("BR-CIUS-PT-25", "a Credit Note shall contain the Preceding Invoice reference (BG-3)")
	}
	// BR-CIUS-PT-65, report, $Invoice:
	//   ((cbc:InvoiceTypeCode = '383' or cbc:InvoiceTypeCode = 'ND') and not(cac:BillingReference))
	// 383 is a debit note; ND is AT's own type code for one. Neither is the credit
	// note BR-CIUS-PT-25 covers, and the two rules can both fire only on a document
	// that is a credit note carrying an invoice type code, which UBL does not allow.
	typeCodes := g.root.all("InvoiceTypeCode")
	if (ptAnyText(typeCodes, "383") || ptAnyText(typeCodes, "ND")) && len(g.root.all("BillingReference")) == 0 {
		add("BR-CIUS-PT-65", "an Invoice with type code 383 or ND shall contain the Preceding Invoice reference (BG-3)")
	}
	// BR-CIUS-PT-66, assert, $Invoice:
	//   exists(cac:Delivery/cac:DeliveryLocation/cac:Address)
	if len(g.deliverTo) == 0 {
		add("BR-CIUS-PT-66", "the Invoice shall contain at least one Deliver to address (BG-15)")
	}

	for _, t := range g.totals {
		// BR-CIUS-PT-62, report, $Document_totals:
		//   exists(/*/cac:AllowanceCharge[cbc:ChargeIndicator='false']) and not(cbc:AllowanceTotalAmount)
		if len(g.docAllowance) > 0 && !ptExists(t, "AllowanceTotalAmount") {
			add("BR-CIUS-PT-62", "an Invoice with a Document level allowance (BG-20) shall contain the "+
				"Sum of allowances on document level (BT-107)")
		}
		// BR-CIUS-PT-63, report, $Document_totals:
		//   exists(/*/cac:AllowanceCharge[cbc:ChargeIndicator='true']) and not(cbc:ChargeTotalAmount)
		if len(g.docCharge) > 0 && !ptExists(t, "ChargeTotalAmount") {
			add("BR-CIUS-PT-63", "an Invoice with a Document level charge (BG-21) shall contain the "+
				"Sum of charges on document level (BT-108)")
		}
	}
}

// ptPartyRules holds the rules bound to $Seller, $Buyer, $Payee and the three
// postal-address contexts.
//
// Every rule here is a `<report>` of the shape "exists(G) and not(G/child)": the
// group is optional, and an invoice that omits it entirely is conforming. The
// second conjunct is a node-set negation over *all* instances of the group, so a
// party with two cac:PartyName elements of which one carries a cbc:Name does not
// trip -35. That is what the artefact says and it is why these read as one
// ptExists over a set rather than as a loop.
func ptPartyRules(g *ptNodes, add func(rule, msg string)) {
	seller, buyer := g.seller, g.buyer

	// BR-CIUS-PT-34/35/36, report, $Seller = cac:AccountingSupplierParty.
	if ptExists(seller, "Party", "PartyIdentification") && !ptExists(seller, "Party", "PartyIdentification", "ID") {
		add("BR-CIUS-PT-34", "the Seller identifier (BT-29) or the Bank assigned creditor identifier (BT-90) shall be filled")
	}
	if ptExists(seller, "Party", "PartyName") && !ptExists(seller, "Party", "PartyName", "Name") {
		add("BR-CIUS-PT-35", "the Seller trading name (BT-28) shall be filled")
	}
	if sc := ptPath(seller, "Party", "Contact"); len(sc) > 0 &&
		!ptExistsFrom(sc, "Name") && !ptExistsFrom(sc, "Telephone") && !ptExistsFrom(sc, "ElectronicMail") {
		add("BR-CIUS-PT-36", "the Seller contact point (BT-41), the Seller contact telephone number (BT-42) or "+
			"the Seller contact email address (BT-43) shall be filled")
	}
	// BR-CIUS-PT-38/39/40, report, $Buyer = cac:AccountingCustomerParty. The same
	// three rules for the other party.
	if ptExists(buyer, "Party", "PartyIdentification") && !ptExists(buyer, "Party", "PartyIdentification", "ID") {
		add("BR-CIUS-PT-38", "the Buyer identifier (BT-46) shall be filled")
	}
	if ptExists(buyer, "Party", "PartyName") && !ptExists(buyer, "Party", "PartyName", "Name") {
		add("BR-CIUS-PT-39", "the Buyer trading name (BT-45) shall be filled")
	}
	if bc := ptPath(buyer, "Party", "Contact"); len(bc) > 0 &&
		!ptExistsFrom(bc, "Name") && !ptExistsFrom(bc, "Telephone") && !ptExistsFrom(bc, "ElectronicMail") {
		add("BR-CIUS-PT-40", "the Buyer contact point (BT-56), the Buyer contact telephone number (BT-57) or "+
			"the Buyer contact email address (BT-58) shall be filled")
	}

	// BR-CIUS-PT-32/42/43, report, $Payee = cac:PayeeParty. The context repeats:
	// UBL allows one PayeeParty, but the rule is bound to the element and not to
	// the document, so each is judged on its own.
	for _, p := range g.payees {
		if ptExists(p, "PartyName") && !ptExists(p, "PartyName", "Name") {
			add("BR-CIUS-PT-32", "the Payee name (BT-59) shall be filled")
		}
		if ptExists(p, "PartyIdentification") && !ptExists(p, "PartyIdentification", "ID") {
			add("BR-CIUS-PT-42", "the Payee identifier (BT-60) or the Bank assigned creditor identifier (BT-90) shall be filled")
		}
		if ptExists(p, "PartyLegalEntity") && !ptExists(p, "PartyLegalEntity", "CompanyID") {
			add("BR-CIUS-PT-43", "the Payee legal registration identifier (BT-61) shall be filled")
		}
	}

	// BR-CIUS-PT-05/06/07, assert, $Seller_postal_address. The context is the
	// address, so an invoice with no Seller postal address at all trips BR-08 in the
	// core and none of these three.
	for _, a := range g.sellerAddr {
		if !ptExists(a, "StreetName") {
			add("BR-CIUS-PT-05", "the Seller postal address (BG-5) shall contain a Seller address line 1 (BT-35)")
		}
		if !ptExists(a, "CityName") {
			add("BR-CIUS-PT-06", "the Seller postal address (BG-5) shall contain a Seller city (BT-37)")
		}
		if !ptExists(a, "PostalZone") {
			add("BR-CIUS-PT-07", "the Seller postal address (BG-5) shall contain a Seller post code (BT-38)")
		}
		// BR-CIUS-PT-37, report, $Seller_postal_address:
		//   exists(cac:AddressLine) and not(cac:AddressLine/cbc:Line)
		if ptExists(a, "AddressLine") && !ptExists(a, "AddressLine", "Line") {
			add("BR-CIUS-PT-37", "the Seller address line 3 (BT-162) shall be filled")
		}
	}
	// BR-CIUS-PT-41, report, $Buyer_postal_address.
	for _, a := range g.buyerAddr {
		if ptExists(a, "AddressLine") && !ptExists(a, "AddressLine", "Line") {
			add("BR-CIUS-PT-41", "the Buyer address line 3 (BT-163) shall be filled")
		}
	}
	// BR-CIUS-PT-44, report, $Tax_Representative_postal_address.
	for _, a := range g.taxRepAddr {
		if ptExists(a, "AddressLine") && !ptExists(a, "AddressLine", "Line") {
			add("BR-CIUS-PT-44", "the Tax representative address line 3 (BT-164) shall be filled")
		}
	}
}

// ptReferenceRules holds the six document-reference contexts and the additional
// supporting documents.
//
// All seven are the same shape and all seven are conditional on the optional group
// being present: CIUS-PT does not require a purchase order, it requires that an
// order reference, once written, carries an identifier.
func ptReferenceRules(g *ptNodes, add func(rule, msg string)) {
	// BR-CIUS-PT-24, assert, $Order_Reference:
	//   exists(cbc:ID) or exists(cbc:SalesOrderID)
	for _, r := range g.root.all("OrderReference") {
		if !ptExists(r, "ID") && !ptExists(r, "SalesOrderID") {
			add("BR-CIUS-PT-24", "the Purchase order reference (BT-13) or the Sales order reference (BT-14) shall be filled")
		}
	}
	// BR-CIUS-PT-26/27/28/29/33, assert on their group: exists(cbc:ID).
	for _, r := range []struct{ group, rule, msg string }{
		{"DespatchDocumentReference", "BR-CIUS-PT-26", "the Despatch advice reference (BT-16) shall be filled"},
		{"ReceiptDocumentReference", "BR-CIUS-PT-27", "the Receiving advice reference (BT-15) shall be filled"},
		{"OriginatorDocumentReference", "BR-CIUS-PT-28", "the Tender or lot reference (BT-17) shall be filled"},
		{"ContractDocumentReference", "BR-CIUS-PT-29", "the Contract reference (BT-12) shall be filled"},
		{"ProjectReference", "BR-CIUS-PT-33", "the Project reference (BT-11) shall be filled"},
	} {
		for _, n := range g.root.all(r.group) {
			if !ptExists(n, "ID") {
				add(r.rule, r.msg)
			}
		}
	}
	// BR-CIUS-PT-30, report, $Additional_supporting_documents:
	//   exists(cac:Attachment) and (not(cac:Attachment/cbc:EmbeddedDocumentBinaryObject)
	//     and not(cac:Attachment/cac:ExternalReference/cbc:URI))
	for _, d := range g.addDocRefs {
		att := ptPath(d, "Attachment")
		if len(att) > 0 && !ptExistsFrom(att, "EmbeddedDocumentBinaryObject") &&
			!ptExistsFrom(att, "ExternalReference", "URI") {
			add("BR-CIUS-PT-30", "an Additional supporting document (BG-24) shall carry the External document "+
				"location (BT-124) or the Attached document (BT-125), or both")
		}
	}
}

// ptDeliveryRules holds $Delivery and $Deliver_to_address.
func ptDeliveryRules(g *ptNodes, add func(rule, msg string)) {
	for _, d := range g.deliveries {
		// BR-CIUS-PT-64, assert, $Delivery: four alternatives, and the rule applies
		// only where a cac:Delivery exists.
		if !ptExists(d, "ActualDeliveryDate") && !ptExists(d, "DeliveryParty") &&
			!ptExists(d, "DeliveryLocation", "ID") && !ptExists(d, "DeliveryLocation", "Address") {
			add("BR-CIUS-PT-64", "the Actual delivery date (BT-72), the Deliver to party name (BT-70), the Deliver "+
				"to location identifier (BT-71) or the Deliver to address (BG-15) shall be present")
		}
		// BR-CIUS-PT-46, report, $Delivery:
		//   (exists(cac:DeliveryParty) and not(cac:DeliveryParty/cac:PartyName))
		//     or (exists(cac:DeliveryParty/cac:PartyName) and not(cac:DeliveryParty/cac:PartyName/cbc:Name))
		dp := ptPath(d, "DeliveryParty")
		if (len(dp) > 0 && !ptExistsFrom(dp, "PartyName")) ||
			(ptExistsFrom(dp, "PartyName") && !ptExistsFrom(dp, "PartyName", "Name")) {
			add("BR-CIUS-PT-46", "the Deliver to party name (BT-70) shall be filled")
		}
	}
	// BR-CIUS-PT-21/22/23 assert and -45 reports on every Deliver to address, not
	// only the first: the context is the address element.
	for _, a := range g.deliverTo {
		if !ptExists(a, "StreetName") {
			add("BR-CIUS-PT-21", "each Deliver to address (BG-15) shall contain an address line 1 (BT-75)")
		}
		if !ptExists(a, "CityName") {
			add("BR-CIUS-PT-22", "each Deliver to address (BG-15) shall contain a city (BT-77)")
		}
		if !ptExists(a, "PostalZone") {
			add("BR-CIUS-PT-23", "each Deliver to address (BG-15) shall contain a post code (BT-78)")
		}
		if ptExists(a, "AddressLine") && !ptExists(a, "AddressLine", "Line") {
			add("BR-CIUS-PT-45", "the Deliver to address line 3 (BT-165) shall be filled")
		}
	}
}

// ptPaymentRules holds $Payment_instructions (cac:PaymentMeans) and
// $Payment_terms.
func ptPaymentRules(g *ptNodes, add func(rule, msg string)) {
	for _, m := range g.payMeans {
		// BR-CIUS-PT-47, report: a payment account that identifies itself in none of
		// the three ways UBL offers.
		if acc := ptPath(m, "PayeeFinancialAccount"); len(acc) > 0 &&
			!ptExistsFrom(acc, "ID") && !ptExistsFrom(acc, "Name") &&
			!ptExistsFrom(acc, "FinancialInstitutionBranch", "ID") {
			add("BR-CIUS-PT-47", "the Payment account identifier (BT-84), the Payment account name (BT-85) or "+
				"the Payment service provider identifier (BT-86) shall be filled")
		}
		// BR-CIUS-PT-48, report:
		//   exists(cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch)
		//     and not(cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID)
		if ptExists(m, "PayeeFinancialAccount", "FinancialInstitutionBranch") &&
			!ptExists(m, "PayeeFinancialAccount", "FinancialInstitutionBranch", "ID") {
			add("BR-CIUS-PT-48", "the Payment service provider identifier (BT-86) shall be filled")
		}
		// BR-CIUS-PT-49, report: a direct-debit mandate with neither a mandate
		// reference nor a debited account.
		if mand := ptPath(m, "PaymentMandate"); len(mand) > 0 &&
			!ptExistsFrom(mand, "ID") && !ptExistsFrom(mand, "PayerFinancialAccount", "ID") {
			add("BR-CIUS-PT-49", "the Mandate reference identifier (BT-89) or the Debited account identifier (BT-91) "+
				"shall be filled")
		}
		// BR-CIUS-PT-50, report.
		if ptExists(m, "PaymentMandate", "PayerFinancialAccount") &&
			!ptExists(m, "PaymentMandate", "PayerFinancialAccount", "ID") {
			add("BR-CIUS-PT-50", "the Debited account identifier (BT-91) shall be filled")
		}
		// BR-CIUS-PT-60, report:
		//   exists(cac:CardAccount) and (not(cac:CardAccount/cbc:PrimaryAccountNumberID)
		//     or not(cac:CardAccount/cbc:NetworkID))
		// An `or`, not an `and`: a card account missing either half trips it.
		if card := ptPath(m, "CardAccount"); len(card) > 0 &&
			(!ptExistsFrom(card, "PrimaryAccountNumberID") || !ptExistsFrom(card, "NetworkID")) {
			add("BR-CIUS-PT-60", "the Payment card primary account number (BT-87) and the card network identifier "+
				"shall be filled")
		}
	}
	// BR-CIUS-PT-61, assert, $Payment_terms: exists(cbc:Note).
	for _, t := range g.payTerms {
		if !ptExists(t, "Note") {
			add("BR-CIUS-PT-61", "the Payment terms (BT-20) shall be filled")
		}
	}
}

// ptLineRules holds $Invoice_Line, $Invoice_Line_Item and $Invoice_Line_Price,
// and the two document-level allowance/charge contexts.
func ptLineRules(g *ptNodes, add func(rule, msg string)) {
	for _, l := range g.lines {
		// BR-CIUS-PT-09, assert: exists(cac:Item/cac:ClassifiedTaxCategory/cac:TaxScheme/cbc:ID)
		if !ptExists(l, "Item", "ClassifiedTaxCategory", "TaxScheme", "ID") {
			add("BR-CIUS-PT-09", "each Invoice line (BG-25) shall have a tax scheme")
		}
		// BR-CIUS-PT-51/52, report.
		if ptExists(l, "OrderLineReference") && !ptExists(l, "OrderLineReference", "LineID") {
			add("BR-CIUS-PT-51", "the Referenced purchase order line reference (BT-132) shall be filled")
		}
		if ptExists(l, "DocumentReference") && !ptExists(l, "DocumentReference", "ID") {
			add("BR-CIUS-PT-52", "the Invoice line object identifier (BT-128) shall be filled")
		}
	}
	// BR-CIUS-PT-53/54/55/56/57, report, $Invoice_Line_Item.
	for _, it := range g.lineItems {
		for _, r := range []struct{ group, leaf, rule, msg string }{
			{"BuyersItemIdentification", "ID", "BR-CIUS-PT-53", "the Item Buyer's identifier (BT-156) shall be filled"},
			{"SellersItemIdentification", "ID", "BR-CIUS-PT-54", "the Item Seller's identifier (BT-155) shall be filled"},
			{"StandardItemIdentification", "ID", "BR-CIUS-PT-55", "the Item standard identifier (BT-157) shall be filled"},
			{"OriginCountry", "IdentificationCode", "BR-CIUS-PT-56", "the Item country of origin (BT-159) shall be filled"},
			{"CommodityClassification", "ItemClassificationCode", "BR-CIUS-PT-57", "the Item classification identifier (BT-158) shall be filled"},
		} {
			if ptExists(it, r.group) && !ptExists(it, r.group, r.leaf) {
				add(r.rule, r.msg)
			}
		}
	}
	// BR-CIUS-PT-58/59, $Invoice_Line_Price.
	for _, p := range g.linePrices {
		// -58, assert: not(cac:AllowanceCharge[cbc:ChargeIndicator='true']). CIUS-PT
		// forbids a charge on the price detail; a discount (BT-147) is allowed and is
		// what -59 is about.
		acs := p.all("AllowanceCharge")
		if len(ptCharge(acs, "true")) > 0 {
			add("BR-CIUS-PT-58", "an Item price charge is not allowed in the Price details (BG-29)")
		}
		// -59, report: exists(cac:AllowanceCharge[cbc:ChargeIndicator='false'])
		//   and not(cac:AllowanceCharge/cbc:Amount)
		// The second conjunct is over *every* AllowanceCharge on the price, not only
		// the discounts, which is what the artefact says.
		if len(ptCharge(acs, "false")) > 0 && !ptExistsFrom(acs, "Amount") {
			add("BR-CIUS-PT-59", "the Item price discount (BT-147) shall be filled")
		}
	}
	// BR-CIUS-PT-19/20, assert, $Document_level_allowances and $Document_level_charges:
	//   exists(cac:TaxCategory/cac:TaxScheme/cbc:ID)
	for _, a := range g.docAllowance {
		if !ptExists(a, "TaxCategory", "TaxScheme", "ID") {
			add("BR-CIUS-PT-19", "each Document level allowance (BG-20) shall have a tax scheme")
		}
	}
	for _, c := range g.docCharge {
		if !ptExists(c, "TaxCategory", "TaxScheme", "ID") {
			add("BR-CIUS-PT-20", "each Document level charge (BG-21) shall have a tax scheme")
		}
	}
}

// ptVATRules holds $VAT_breakdown, the three code-filtered breakdown contexts
// ($VATAA, $VATS, $VATE) and the three item-attribute contexts that carry BT-160.
//
// These are the rules the coverage table used to call "the Portuguese VAT-category
// rate rules", and they are where the 2.0.0/2.1.1 divergence lives: six of the
// eight select on a category-code set that 2.1.1 widened. See the file comment.
func ptVATRules(g *ptNodes, add func(rule, msg string)) {
	// BR-CIUS-PT-08, assert, $VAT_breakdown: exists(cac:TaxCategory/cac:TaxScheme/cbc:ID)
	for _, b := range g.breakdowns {
		if !ptExists(b, "TaxCategory", "TaxScheme", "ID") {
			add("BR-CIUS-PT-08", "each VAT breakdown (BG-23) shall have a tax scheme")
		}
	}
	// BR-CIUS-PT-12/14/16, assert on the code-filtered breakdown category.
	//
	// An absent cbc:Percent makes the comparison false in XPath — an empty sequence
	// compares equal to nothing — so a breakdown with no rate trips the rule. That
	// is the artefact's behaviour and it is not the same as BR-48's, which asks
	// whether the element is there.
	for _, c := range ptCategoriesIn(g.bdCategories, ptCatLower...) {
		if !ptPercentGreaterThanZero(c) {
			add("BR-CIUS-PT-12", "a VAT breakdown (BG-23) with the VAT category code (BT-118) \"Lower rate\" shall "+
				"have a VAT category rate (BT-119) greater than zero")
		}
	}
	for _, c := range ptCategoriesIn(g.bdCategories, ptCatStandard...) {
		if !ptPercentGreaterThanZero(c) {
			add("BR-CIUS-PT-14", "a VAT breakdown (BG-23) with the VAT category code (BT-118) \"Standard rated\" "+
				"shall have a VAT category rate (BT-119) greater than zero")
		}
	}
	for _, c := range ptCategoriesIn(g.bdCategories, ptCatExempt...) {
		if !ptPercentEqualsZero(c) {
			add("BR-CIUS-PT-16", "a VAT breakdown (BG-23) with the VAT category code (BT-118) \"Exempt from VAT\" "+
				"shall have a VAT category rate (BT-119) of zero")
		}
	}

	// BR-CIUS-PT-13/15/17, on $VATxx_AdditionalLine — every
	// cac:AdditionalItemProperty/cbc:Name of a line item in the given category.
	//
	// CIUS-PT carries BT-160, the VAT exemption reason, as an item attribute whose
	// name is one of two literals, because UBL's invoice line has nowhere else to
	// put it. -13 and -15 forbid that attribute on a line that is not exempt; -17
	// requires it on a line that is.
	//
	// The switch is exclusive on purpose, and the order is the artefact's. The three
	// are separate <rule> elements of the *same* Schematron pattern — at lines 211,
	// 217 and 223 of urn_feap.gov.pt_CIUS-PT_2.1.1-model.sch, in that order — and ISO
	// Schematron gives a node to the first rule in a pattern whose context matches it
	// and to no other. So a cbc:Name under an item that somehow carried both an 'AA'
	// and an 'E' category answers to -13 and not to -17, and a chain of independent
	// `if`s would report a rule no processor reaches. It is the same reading that
	// makes CEN's CII-DT-010/011/012 unreachable (D10).
	//
	// Note the three category sets are not the same shape: -13's is 'AA' alone,
	// -15's differs between an invoice line and a credit-note line, and only -17's is
	// the plain alias pair. See ptCatLowerItemAttr and ptStandardRateCodes.
	for _, it := range g.lineItems {
		names := ptPath(it, "AdditionalItemProperty", "Name")
		cats := it.all("ClassifiedTaxCategory")
		switch {
		case len(ptCategoriesIn(cats, ptCatLowerItemAttr...)) > 0:
			for _, n := range names {
				if ptStartsWithExemptionName(n) {
					add("BR-CIUS-PT-13", "an Invoice line (BG-25) with the Invoiced item VAT category code (BT-151) "+
						"\"Lower rate\" shall not have a VAT exemption reason code or text (BT-160)")
				}
			}
		case len(ptCategoriesIn(cats, ptStandardRateCodes(g, it)...)) > 0:
			for _, n := range names {
				if ptStartsWithExemptionName(n) {
					add("BR-CIUS-PT-15", "an Invoice line (BG-25) with the Invoiced item VAT category code (BT-151) "+
						"\"Standard rated\" shall not have a VAT exemption reason code or text (BT-160)")
				}
			}
		case len(ptCategoriesIn(cats, ptCatExempt...)) > 0:
			// -17's test counts across the whole line item rather than reading the
			// context node, so the verdict is the same for every cbc:Name under it —
			// but the rule is bound to the name, so it fires once per name, and this
			// transcribes that.
			if len(names) > 0 && !ptHasExactExemptionName(names) {
				for range names {
					add("BR-CIUS-PT-17", "an Invoice line (BG-25) with the Invoiced item VAT category code (BT-151) "+
						"\"Exempt from VAT\" shall have a VAT exemption reason code or text (BT-160)")
				}
			}
		}
		// BR-CIUS-PT-18, report, $Invoice_Line_Item:
		//   ((cac:ClassifiedTaxCategory/cbc:ID) = 'E' or … = 'ISE') and not(cac:AdditionalItemProperty)
		// The companion to -17 for the line that carries no item attribute at all, in
		// which case -17's context matches nothing and only this fires.
		if len(ptCategoriesIn(cats, ptCatExempt...)) > 0 && !ptExists(it, "AdditionalItemProperty") {
			add("BR-CIUS-PT-18", "an Invoice line (BG-25) with the Invoiced item VAT category code (BT-151) "+
				"\"Exempt from VAT\" shall have a VAT exemption reason code or text (BT-160)")
		}
	}
}

// ptLowerRateRules holds BR-AA-01..07 and BR-AA-10 — the "Lower rate" VAT
// category family.
//
// # What this family is
//
// It looks like a CEN family and it is not. EN 16931 publishes a rule family per
// VAT category code in its restricted BT-118 list — BR-S-*, BR-Z-*, BR-E-*,
// BR-AE-*, BR-IC-*, BR-G-*, BR-O-*, BR-IG-*, BR-IP-*, BR-L-*, BR-M-* — and 'AA'
// ("Lower rate") is a UNCL5305 code that list leaves out. So CEN publishes no
// BR-AA-* family and no CEN artefact in this repository contains the string:
// verified by decoding every vendored EN 16931 Schematron, UBL and CII, and by
// grep over their bytes.
//
// AT/eSPap wrote it. Portugal levies reduced VAT rates that the Portuguese profile
// carries as category 'AA' (and, from 2.1.1, 'RED' and 'INT'), so AT cloned CEN's
// BR-S-* template into a family for that code, keeping CEN's numbering convention:
// -01 "a line/allowance/charge in this category needs a breakdown in it", -02/03/04
// "…needs the Seller VAT identifier", -05/06/07 "…the rate shall be greater than
// zero", -10 "…shall carry no exemption reason". AT publishes eight of the ten
// slots; -08 and -09, CEN's two arithmetic rules, have no BR-AA counterpart —
// their Portuguese equivalents are DT-CIUS-PT-171 and -172, which is where AT put
// every summation rule.
//
// The identifiers are therefore AT's, they are flagged fatal like the rest of the
// Portuguese set, and they are reported under SourceCIUSPT. Nothing named them
// before this PR: ciusArtefacts filtered CIUS-PT's identifiers on
// `^(?:BR|DT)-CIUS-PT-`, so the guard that exists to catch a published rule that is
// in neither the code nor the coverage table could not see this family at all —
// the same shape as C38's DT-CIUS-PT-*, one prefix further out.
//
// # Why they read `//`
//
// Six of the eight reach document-wide. BR-AA-01 counts every
// `//cac:AllowanceCharge/cac:TaxCategory` in the AA set, line-level allowances
// included, while BR-AA-06 and -07 are bound to the *document-level* ones. The two
// populations are transcribed separately for that reason.
func ptLowerRateRules(g *ptNodes, add func(rule, msg string)) {
	lower := func(cats []*ciiNode) []*ciiNode { return ptCategoriesIn(cats, ptCatLower...) }

	acCats := ptPathFrom(g.allAllowCharges, "TaxCategory")
	lineOrACLower := len(lower(acCats)) + len(lower(g.allClassifiedCats))
	breakdownLower := len(lower(g.bdCategories))

	// BR-AA-01, assert, $Invoice: the two counts are both positive or both zero.
	// An invoice that categorises something as "Lower rate" and reports no such
	// breakdown fails it, and so does the mirror case.
	if (lineOrACLower > 0) != (breakdownLower > 0) {
		add("BR-AA-01", "an Invoice with a \"Lower rate\" (AA) line, allowance or charge shall contain a "+
			"VAT breakdown (BG-23) with that category, and only then")
	}

	// BR-AA-02/03/04, assert, $Invoice: a "Lower rate" line, document-level
	// allowance or document-level charge requires the Seller VAT identifier (BT-31)
	// or the Seller tax representative's (BT-63).
	//
	// AT's antecedent for -02 is `//cac:ClassifiedTaxCategory[AA]` with no tax-scheme
	// predicate, where CEN's BR-S-02 adds `[cac:TaxScheme/cbc:ID='VAT']`. That is one
	// of the CEN conditions AT modified, and it is transcribed as AT wrote it.
	sellerVATIdentified := ptExists(g.root, "AccountingSupplierParty", "Party", "PartyTaxScheme", "CompanyID") ||
		ptTaxRepVATID(g.root)
	const vatIDMsg = " shall contain the Seller VAT identifier (BT-31), the Seller tax registration " +
		"identifier (BT-32) or the Seller tax representative VAT identifier (BT-63)"
	if len(lower(g.allClassifiedCats)) > 0 && !sellerVATIdentified {
		add("BR-AA-02", "an Invoice with a \"Lower rate\" (AA) Invoice line (BG-25)"+vatIDMsg)
	}
	if len(lower(ptPathFrom(ptCharge(g.allAllowCharges, "false"), "TaxCategory"))) > 0 && !sellerVATIdentified {
		add("BR-AA-03", "an Invoice with a \"Lower rate\" (AA) Document level allowance (BG-20)"+vatIDMsg)
	}
	if len(lower(ptPathFrom(ptCharge(g.allAllowCharges, "true"), "TaxCategory"))) > 0 && !sellerVATIdentified {
		add("BR-AA-04", "an Invoice with a \"Lower rate\" (AA) Document level charge (BG-21)"+vatIDMsg)
	}

	// BR-AA-05, assert, $VATAA_Line: every line item's "Lower rate" category shall
	// carry a rate greater than zero. The context is the line's category element, so
	// a document with no such line is outside it.
	for _, c := range lower(ptPathFrom(g.lineItems, "ClassifiedTaxCategory")) {
		if !ptPercentGreaterThanZero(c) {
			add("BR-AA-05", "an Invoice line (BG-25) with the Invoiced item VAT category code (BT-151) "+
				"\"Lower rate\" shall have an Invoiced item VAT rate (BT-152) greater than zero")
		}
	}
	// BR-AA-06/07, assert, $VATAA_Allowance and $VATAA_Charge — the *document-level*
	// allowance and charge categories only.
	for _, c := range lower(ptPathFrom(g.docAllowance, "TaxCategory")) {
		if !ptPercentGreaterThanZero(c) {
			add("BR-AA-06", "a Document level allowance (BG-20) with the VAT category code (BT-95) \"Lower rate\" "+
				"shall have a Document level allowance VAT rate (BT-96) greater than zero")
		}
	}
	for _, c := range lower(ptPathFrom(g.docCharge, "TaxCategory")) {
		if !ptPercentGreaterThanZero(c) {
			add("BR-AA-07", "a Document level charge (BG-21) with the VAT category code (BT-102) \"Lower rate\" "+
				"shall have a Document level charge VAT rate (BT-103) greater than zero")
		}
	}
	// BR-AA-10, assert, $VATAA: a "Lower rate" breakdown carries no exemption
	// reason. It is the mirror of CEN's BR-E-10, which requires one for "Exempt".
	for _, c := range lower(g.bdCategories) {
		if ptExists(c, "TaxExemptionReason") || ptExists(c, "TaxExemptionReasonCode") {
			add("BR-AA-10", "a VAT breakdown (BG-23) with the VAT category code (BT-118) \"Lower rate\" shall not "+
				"have a VAT exemption reason code (BT-121) or a VAT exemption reason text (BT-120)")
		}
	}
}

// ptTaxRepVATID is the second disjunct BR-AA-02/03/04 share:
// exists(//cac:TaxRepresentativeParty/cac:PartyTaxScheme[cac:TaxScheme/cbc:ID = 'VAT']/cbc:CompanyID).
//
// The tax-scheme predicate is present here and absent from the Seller disjunct,
// which is asymmetric and is what AT wrote.
func ptTaxRepVATID(root *ciiNode) bool {
	for _, rep := range root.findAll("TaxRepresentativeParty") {
		for _, pts := range rep.all("PartyTaxScheme") {
			if ptAnyText(ptPath(pts, "TaxScheme", "ID"), "VAT") && ptExists(pts, "CompanyID") {
				return true
			}
		}
	}
	return false
}

// ptStandardRateCodes returns the category codes BR-CIUS-PT-15's context matches
// for one line item, which is 'S' and 'NOR' on an invoice line and 'S' alone on a
// credit-note line.
//
// That asymmetry is a defect in AT's artefact, and it is transcribed rather than
// repaired. $VATS_AdditionalLine reads
//
//	cac:InvoiceLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or cac:ClassifiedTaxCategory/cbc:ID = 'NOR']/…
//	| cac:CreditNoteLine/cac:Item[cac:ClassifiedTaxCategory/cbc:ID = 'S' or normalize-space(cbc:ID) = 'NOR']/…
//
// and in the second branch the alias alternative lost its cac:ClassifiedTaxCategory
// step: `cbc:ID` there is a child of cac:Item, which UBL's Item has no such element
// for, so the alternative is unsatisfiable and a 'NOR'-category credit-note line is
// outside the rule's context. It is the only one of the six $VAT*_AdditionalLine and
// $VAT* parameters whose two branches disagree — the AA and E ones are symmetric.
//
// Matching the invoice branch on a credit note would be an improvement on AT's rule
// and therefore a finding AT's own validator does not produce, which is the
// definition of a false positive here. C37's fifteen rules were all written by
// improving on, or paraphrasing, what an authority actually published.
func ptStandardRateCodes(g *ptNodes, item *ciiNode) []string {
	if g.creditNoteItems[item] {
		return ptCatStandard[:1]
	}
	return ptCatStandard
}

// ptPercentGreaterThanZero is AT's `(cbc:Percent) > 0`, empty sequence included:
// a category with no cbc:Percent compares false, as it does in XPath.
func ptPercentGreaterThanZero(cat *ciiNode) bool {
	for _, p := range cat.all("Percent") {
		if v, ok := parseAmount(strings.TrimSpace(p.text)); ok && v > 0 {
			return true
		}
	}
	return false
}

// ptPercentEqualsZero is AT's `(cbc:Percent = 0)`.
func ptPercentEqualsZero(cat *ciiNode) bool {
	for _, p := range cat.all("Percent") {
		if v, ok := parseAmount(strings.TrimSpace(p.text)); ok && v == 0 {
			return true
		}
	}
	return false
}

// ptStartsWithExemptionName is the test BR-CIUS-PT-13 and -15 share:
// starts-with(normalize-space(.), …) against either magic attribute name.
func ptStartsWithExemptionName(name *ciiNode) bool {
	v := strings.Join(strings.Fields(name.text), " ")
	return strings.HasPrefix(v, ptExemptionReasonCode) || strings.HasPrefix(v, ptExemptionReason)
}

// ptHasExactExemptionName is BR-CIUS-PT-17's test: at least one item attribute
// name matching `^(#(TAXEXEMPTIONREASONCODE@CLASSIFIEDTAXCATEGORY)#)$` or its
// reason-text twin. Unlike -13/-15 this is an exact match with no normalisation,
// which is what the artefact's matches() anchors say.
func ptHasExactExemptionName(names []*ciiNode) bool {
	for _, n := range names {
		if n.text == ptExemptionReasonCode || n.text == ptExemptionReason {
			return true
		}
	}
	return false
}
