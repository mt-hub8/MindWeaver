package formalis

import (
	"fmt"
	"regexp"
	"strings"
)

// This file evaluates the fatal half of CEN's CII syntax binding — the CII-SR-*
// cardinality rules and the CII-DT-* datatype rules of EN16931-CII-syntax.sch —
// against the parsed element tree. It is the mirror of en16931_ubl_rules.go,
// which argues the design at length; the short form is that a CII-SR-* or
// CII-DT-* rule is a statement about a CrossIndustryInvoice document tree
// ("ram:BillingSpecifiedPeriod shall carry no ram:Description", "an amount other
// than ram:TaxTotalAmount shall carry no @currencyID"), inapplicable to UBL by
// construction, and restating a hundred element and attribute counts as fields
// on en16931Invoice would fill the shared semantic model with facts meaningless
// to half its readers.
//
// Fidelity. Every rule below is transcribed from the assertion in
// cii/schematron/CII/EN16931-CII-syntax.sch as resolved in
// cii/schematron/preprocessed/EN16931-CII-validation-preprocessed.sch, which is
// the form the vendored EN16931-CII-validation.xslt is generated from and the
// form a reference validator runs. Each rule cites its XPath, because several of
// CEN's rule *titles* describe a different rule than the XPath does — CII-SR-471
// is titled "shall contain a VAT category code (BT-95)" and tests
// `count(ram:RateApplicablePercent) <= 1`, and CII-SR-472 is titled "should
// contain a VAT rate (BT-96)" and tests `count(ram:CategoryTradeTax) <= 1`. The
// XPath is the rule.
//
// Severity. Of the 482 CII-SR-* assertions CEN publishes, 42 are flagged fatal
// and 440 advisory; of the 101 CII-DT-* assertions, 70 are fatal and 31
// advisory. Only the fatal ones are evaluated here. The advisory ones are
// evaluated as warnings by en16931_syntax_advisory.go, from a table generated
// from this same Schematron; the split between the two files is between the rules
// that needed judgement to transcribe and the rules that needed only accuracy.
//
// Three of the seventy fatal datatype rules are not evaluated, and cannot be —
// see ciiDatatypeIdentifierRules for CII-DT-010/011/012, which the Schematron's
// own rule ordering makes unreachable. Coverage(SourceEN16931) names them.
//
// Rule order matters, and this file honours it. Under ISO Schematron a node is
// processed by the *first* rule in a pattern whose context matches it, and every
// assertion here lives in one pattern, EN16931-CII-Syntax. Two of its contexts
// overlap:
//
//   - `//ram:*[ends-with(name(), 'ID')]` (CII-DT-101..104) is preceded by a rule
//     naming four specific identifiers (CII-DT-001..007), so those four are
//     checked under the CII-DT-00x identifiers and never under CII-DT-10x. The
//     generated XSLT gives the four-identifier template priority 1011 and the
//     wildcard 1010, which is that reading made mechanical.
//   - `//ram:TypeCode` (CII-DT-008/009, priority 1009) precedes
//     `/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:TypeCode`
//     (CII-DT-010/011/012, priority 1008), so the latter never runs.
//
// Source. These findings are stamped SourceEN16931, not a source of their own:
// CEN publishes the syntax bindings as normative parts of EN 16931 itself.

// validateCIISyntaxRules evaluates the fatal CII-SR-* and CII-DT-* rules against
// a CII document tree. It is a no-op for any other root, so a UBL invoice
// passing through the same entry point is never asked to answer a CII rule.
//
// The rule bodies are grouped by the node population they read, which is also
// how the Schematron groups them: one <rule context=...> per group, named in
// each function's own comment.
func validateCIISyntaxRules(r *run, root *ciiNode) []Violation {
	if root == nil || root.name != "CrossIndustryInvoice" {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceEN16931)
	g := gatherCIISyntaxNodes(root)

	ciiSyntaxDocumentRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxLineRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxAllowanceRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxHeaderRules(g, add)
	if r.stopped() {
		return out
	}
	ciiSyntaxTotalsRules(g, add)
	if r.stopped() {
		return out
	}
	ciiDatatypeIdentifierRules(g, add)
	if r.stopped() {
		return out
	}
	ciiDatatypeReferenceRules(g, add)
	if r.stopped() {
		return out
	}
	ciiDatatypeValueRules(g, add)
	if r.stopped() {
		return out
	}
	ciiDatatypeGroupRules(g, add)
	return out
}

// ciiSyntaxNodes is every node population the fatal CII rules read, gathered in
// one pass.
//
// The alternative — one findAll per rule — would walk the tree once per rule,
// a hundred and ten times over, on every CII document this package validates.
// One walk costs what the largest population costs. The struct also makes the
// rule bodies read as what they are: a count, or an attribute test, against a
// bound, with no traversal in the way.
//
// Two kinds of population live here and they are not interchangeable. The
// path-anchored ones (contexts, exchanged, lines, agreements, settlements,
// summations) are the contexts CEN writes as an absolute path from the document
// element, and are collected by following that path. The rest are the contexts
// CEN writes with a `//` step or a name predicate, and are collected by the
// walk, which matches CEN's own "anywhere in the document" reading.
type ciiSyntaxNodes struct {
	// root is the document element, and the context of the five rules CEN binds
	// to `/rsm:CrossIndustryInvoice`.
	root *ciiNode

	contexts    []*ciiNode // /rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext
	exchanged   []*ciiNode // /rsm:CrossIndustryInvoice/rsm:ExchangedDocument
	lines       []*ciiNode // .../ram:IncludedSupplyChainTradeLineItem
	agreements  []*ciiNode // .../ram:ApplicableHeaderTradeAgreement
	settlements []*ciiNode // .../ram:ApplicableHeaderTradeSettlement
	summations  []*ciiNode // .../ram:SpecifiedTradeSettlementHeaderMonetarySummation

	allowanceCharges []*ciiNode // //ram:SpecifiedTradeAllowanceCharge
	priceAllowances  []*ciiNode // //ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge

	// dueDateTypeCodes is `//ram:ApplicableTradeTax/ram:DueDateTypeCode`, read
	// document-wide because CII-SR-462's test reaches out of its own context
	// with a `//` step.
	dueDateTypeCodes []string
	// paymentMeansCodes and paymentMeansTexts are the values CII-SR-467 and
	// CII-SR-468 require to agree across every payment instruction.
	paymentMeansCodes []string
	paymentMeansTexts []string
	// paymentReferences is `count(//ram:ApplicableHeaderTradeSettlement/
	// ram:PaymentReference)` — elements, not distinct values, which is where the
	// CII binding differs from UBL-SR-44.
	paymentReferences int

	// scopedIDs are the four identifiers CEN gives a rule of their own
	// (CII-DT-001..007); otherIDs is every other ram element whose name ends in
	// "ID", which is what is left for the wildcard rule (CII-DT-101..104) once
	// the earlier rule has claimed the four.
	scopedIDs []*ciiNode
	otherIDs  []*ciiNode

	typeCodes  []*ciiNode // //ram:TypeCode
	refDocs    []*ciiNode // //ram:*[ends-with(name(), 'ReferencedDocument')]
	amounts    []*ciiNode // //ram:*[ends-with(name(), 'Amount') and not(self::ram:TaxTotalAmount)]
	quantities []*ciiNode // //ram:*[ends-with(name(), 'Quantity')]
	tradeTaxes []*ciiNode // //ram:*[ends-with(name(), 'TradeTax')]
	periods    []*ciiNode // //ram:BillingSpecifiedPeriod
	addresses  []*ciiNode // //ram:PostalTradeAddress
	dates102   []*ciiNode // //udt:DateTimeString[@format = '102']

	// billedQuantityUnitCode is the second operand of CII-DT-033, which permits
	// a @unitCode anywhere in the document as soon as the line's billed quantity
	// carries one.
	billedQuantityUnitCode bool
}

// gatherCIISyntaxNodes walks the tree once and collects every population the
// rules read.
func gatherCIISyntaxNodes(root *ciiNode) *ciiSyntaxNodes {
	g := &ciiSyntaxNodes{root: root}

	// The path-anchored contexts first, because the identity of the four scoped
	// identifiers is defined by their path and the walk below has to be able to
	// tell them from every other element whose name ends in "ID".
	g.contexts = root.all("ExchangedDocumentContext")
	g.exchanged = root.all("ExchangedDocument")
	for _, tx := range root.all("SupplyChainTradeTransaction") {
		g.lines = append(g.lines, tx.all("IncludedSupplyChainTradeLineItem")...)
		g.agreements = append(g.agreements, tx.all("ApplicableHeaderTradeAgreement")...)
		g.settlements = append(g.settlements, tx.all("ApplicableHeaderTradeSettlement")...)
	}
	for _, st := range g.settlements {
		g.summations = append(g.summations, st.all("SpecifiedTradeSettlementHeaderMonetarySummation")...)
	}

	// scoped is the walk's exclusion set; g.scopedIDs is the same nodes in the
	// order they were claimed, because a map's iteration order would make the
	// order of the findings vary from run to run.
	scoped := make(map[*ciiNode]bool)
	claim := func(ns []*ciiNode) {
		for _, n := range ns {
			if scoped[n] {
				continue
			}
			scoped[n] = true
			g.scopedIDs = append(g.scopedIDs, n)
		}
	}
	for _, c := range g.contexts {
		claim(nodesAt(c, "GuidelineSpecifiedDocumentContextParameter", "ID"))
	}
	for _, e := range g.exchanged {
		claim(nodesAt(e, "ID"))
	}
	for _, ln := range g.lines {
		claim(nodesAt(ln, "AssociatedDocumentLineDocument", "LineID"))
		claim(nodesAt(ln, "SpecifiedTradeProduct", "SellerAssignedID"))
		for _, d := range nodesAt(ln, "SpecifiedLineTradeDelivery", "BilledQuantity") {
			if d.hasAttr("unitCode") {
				g.billedQuantityUnitCode = true
			}
		}
	}

	var rec func(n, parent *ciiNode)
	rec = func(n, parent *ciiNode) {
		// The contexts CEN names by an exact element name.
		switch n.name {
		case "SpecifiedTradeAllowanceCharge":
			g.allowanceCharges = append(g.allowanceCharges, n)
		case "AppliedTradeAllowanceCharge":
			if parent != nil && parent.name == "GrossPriceProductTradePrice" {
				g.priceAllowances = append(g.priceAllowances, n)
			}
		case "ApplicableTradeTax":
			for _, d := range n.all("DueDateTypeCode") {
				g.dueDateTypeCodes = append(g.dueDateTypeCodes, normalizeSpace(d.text))
			}
		case "SpecifiedTradeSettlementPaymentMeans":
			for _, tc := range n.all("TypeCode") {
				g.paymentMeansCodes = append(g.paymentMeansCodes, normalizeSpace(tc.text))
			}
			for _, i := range n.all("Information") {
				g.paymentMeansTexts = append(g.paymentMeansTexts, normalizeSpace(i.text))
			}
		case "ApplicableHeaderTradeSettlement":
			g.paymentReferences += len(n.all("PaymentReference"))
		case "TypeCode":
			g.typeCodes = append(g.typeCodes, n)
		case "BillingSpecifiedPeriod":
			g.periods = append(g.periods, n)
		case "PostalTradeAddress":
			g.addresses = append(g.addresses, n)
		case "DateTimeString":
			// CEN's context is `//udt:DateTimeString`, and this tree is keyed by
			// local name, so the namespace has to be recovered from the position.
			// The CII schema puts a qdt:DateTimeString in exactly one place —
			// under ram:FormattedIssueDateTime, whose type is the qualified
			// FormattedDateTimeType — and a udt:DateTimeString everywhere else.
			// Excluding that one parent is what makes this rule the udt-only rule
			// CEN wrote rather than a broader one this package invented.
			if n.attr("format") == "102" && (parent == nil || parent.name != "FormattedIssueDateTime") {
				g.dates102 = append(g.dates102, n)
			}
		}
		// The contexts CEN names by a suffix of the element name. These are
		// disjoint from each other, and the switch above is a separate statement
		// because an element can be in both — ram:ApplicableTradeTax is read for
		// CII-SR-462 and is also a `//ram:*[ends-with(name(), 'TradeTax')]`.
		//
		// CEN restricts each to the ram namespace, which this tree does not carry.
		// It costs nothing here: across the 181 CII documents in the corpus, every
		// element whose local name ends in ID, Amount, Quantity, TypeCode,
		// ReferencedDocument or TradeTax is in the ram namespace, and the CII
		// schema puts nothing else in rsm, udt or qdt that could end in one of
		// them.
		switch {
		case strings.HasSuffix(n.name, "ID"):
			if !scoped[n] {
				g.otherIDs = append(g.otherIDs, n)
			}
		case strings.HasSuffix(n.name, "ReferencedDocument"):
			g.refDocs = append(g.refDocs, n)
		case n.name != "TaxTotalAmount" && strings.HasSuffix(n.name, "Amount"):
			g.amounts = append(g.amounts, n)
		case strings.HasSuffix(n.name, "Quantity"):
			g.quantities = append(g.quantities, n)
		case strings.HasSuffix(n.name, "TradeTax"):
			g.tradeTaxes = append(g.tradeTaxes, n)
		}
		for _, c := range n.children {
			rec(c, n)
		}
	}
	rec(root, nil)
	return g
}

// normalizeSpace is XPath's normalize-space(): leading and trailing whitespace
// removed and every internal run collapsed to one space. Three of these rules
// compare element values through it rather than raw, and a value that differs
// only in indentation is the same value to them.
// normalizeSpace is XPath's fn:normalize-space: leading and trailing whitespace
// removed and every internal run collapsed to one space.
//
// The common case by far is a string that is already normalised, and returning it
// unchanged rather than rebuilding it is worth the scan: the CIUS-PT datatype rules
// call this 321 times over a few hundred elements per invoice, and
// strings.Fields+Join allocated twice on every one of them.
func normalizeSpace(s string) string {
	if ptDTAlreadyNormalized(s) {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}

// ptDTAlreadyNormalized reports whether normalize-space would return s unchanged:
// no leading or trailing whitespace, no whitespace character other than a space,
// and no two spaces in a row.
func ptDTAlreadyNormalized(s string) bool {
	if s == "" {
		return true
	}
	if isXMLSpace(s[0]) || isXMLSpace(s[len(s)-1]) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isXMLSpace(s[i]) {
			continue
		}
		if s[i] != ' ' || s[i-1] == ' ' {
			return false
		}
	}
	return true
}

func isXMLSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// allNormalizeSpaceEqual is the shape CII-SR-467 and CII-SR-468 share:
//
//	count(//X[normalize-space(.) != normalize-space((//X)[1])]) = 0
//
// — every occurrence of a term, document-wide, agrees with the first. Vacuously
// true for none or one.
func allNormalizeSpaceEqual(vals []string) bool {
	for _, v := range vals {
		if v != vals[0] {
			return false
		}
	}
	return true
}

// anyHasAttr reports whether any node in the set carries the named attribute. It
// is how CEN's existence tests on an attribute node read when the step before
// them selects a sequence: `ram:GlobalID/@schemeID` is true when *some*
// ram:GlobalID has one.
func anyHasAttr(ns []*ciiNode, name string) bool {
	for _, n := range ns {
		if n.hasAttr(name) {
			return true
		}
	}
	return false
}

// hasChild is `(ram:X)` — an existence test on a child element, whatever its
// content.
func hasChild(n *ciiNode, name string) bool { return len(n.all(name)) > 0 }

// childValueIs is CEN's `ram:TypeCode='130'` — a general comparison over the
// children, so an element with several codes matches if any of them does.
func childValueIs(n *ciiNode, name, want string) bool {
	for _, c := range n.all(name) {
		if strings.TrimSpace(c.text) == want {
			return true
		}
	}
	return false
}

// forbiddenAttr is the shape thirty-one of the datatype rules share: an
// attribute the EN 16931 CII binding does not use, on a population of elements.
type forbiddenAttr struct {
	rule string
	attr string
}

// forbiddenChild is the shape thirty-two of them share: a child element the
// binding does not use.
type forbiddenChild struct {
	rule string
	name string
}

// ciiSyntaxDocumentRules are the rules CEN binds to the document element and to
// the two head groups under it — `/rsm:CrossIndustryInvoice`,
// `/rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext` and
// `/rsm:CrossIndustryInvoice/rsm:ExchangedDocument`.
//
// CII-DT-013 and CII-DT-014 share the document element's rule with CII-SR-467/
// 468/469 rather than living with the other datatype rules, and are stated here
// for the same reason the file is grouped this way at all: the group is the
// Schematron's, so a reader holding the .sch open finds the same five assertions
// in the same place.
func ciiSyntaxDocumentRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, c := range g.contexts {
		// count(ram:GuidelineSpecifiedDocumentContextParameter) = 1
		if n := countAt(c, "GuidelineSpecifiedDocumentContextParameter"); n != 1 {
			add("CII-SR-009", fmt.Sprintf("The document context shall carry exactly one guideline parameter group (BT-24), not %d", n))
		}
		// count(ram:GuidelineSpecifiedDocumentContextParameter/ram:ID) = 1
		if n := countAt(c, "GuidelineSpecifiedDocumentContextParameter", "ID"); n != 1 {
			add("CII-SR-010", fmt.Sprintf("The Specification identifier (BT-24) shall occur exactly once, not %d times", n))
		}
	}
	for _, e := range g.exchanged {
		// count(ram:TypeCode) = 1
		if n := countAt(e, "TypeCode"); n != 1 {
			add("CII-SR-014", fmt.Sprintf("The Invoice type code (BT-3) shall occur exactly once, not %d times", n))
		}
	}

	// CII-SR-467/468: every payment instruction shall agree on the payment means
	// type code (BT-81) and on the payment means text (BT-82). CEN writes both as
	// "no occurrence differs from the first", compared through normalize-space.
	if !allNormalizeSpaceEqual(g.paymentMeansCodes) {
		add("CII-SR-467", "All Payment means type codes (BT-81) shall have the same value")
	}
	if !allNormalizeSpaceEqual(g.paymentMeansTexts) {
		add("CII-SR-468", "All Payment means texts (BT-82) shall have the same value")
	}
	// CII-SR-469: count(//ram:ApplicableHeaderTradeSettlement/ram:PaymentReference)
	// <= 1. Elements, not distinct values: the CII binding gives the payment
	// reference (BT-83) one home, so repeating it is a defect even when the two
	// copies agree. UBL-SR-44 bounds the same term by distinct value, because the
	// UBL binding writes it once per cac:PaymentMeans.
	if g.paymentReferences > 1 {
		add("CII-SR-469", "The Payment reference (BT-83) shall occur at most once")
	}

	// CII-DT-013/014, on the document element.
	if g.root.hasAttr("languageID") {
		add("CII-DT-013", "The document element shall not carry a languageID attribute")
	}
	if g.root.hasAttr("languageLocaleID") {
		add("CII-DT-014", "The document element shall not carry a languageLocaleID attribute")
	}
}

// ciiSyntaxLineRules are the rules whose context is an invoice line or a group
// inside one: ram:SpecifiedTradeProduct, its ram:ApplicableProductCharacteristic
// and ram:SpecifiedLineTradeAgreement. All three contexts are absolute paths
// through ram:IncludedSupplyChainTradeLineItem, so a product group written
// somewhere else in the tree is not asked to answer them.
func ciiSyntaxLineRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ln := range g.lines {
		for _, p := range ln.all("SpecifiedTradeProduct") {
			// not(ram:GlobalID) or (ram:GlobalID/@schemeID)
			if ids := p.all("GlobalID"); len(ids) > 0 && !anyHasAttr(ids, "schemeID") {
				add("CII-SR-046", "The Item standard identifier (BT-157) shall carry a scheme identifier (BT-157-1)")
			}
			// not(ram:OriginTradeCountry) or (count(ram:OriginTradeCountry/ram:ID) = 1)
			if hasChild(p, "OriginTradeCountry") {
				if n := countAt(p, "OriginTradeCountry", "ID"); n != 1 {
					add("CII-SR-090", fmt.Sprintf("An item origin country group shall carry exactly one Item country of origin (BT-159), not %d", n))
				}
			}
			for _, ch := range p.all("ApplicableProductCharacteristic") {
				// count(ram:Description) = 1
				if n := countAt(ch, "Description"); n != 1 {
					add("CII-SR-069", fmt.Sprintf("An item attribute (BG-32) shall carry exactly one Item attribute name (BT-160), not %d", n))
				}
				// count(ram:Value) = 1
				if n := countAt(ch, "Value"); n != 1 {
					add("CII-SR-072", fmt.Sprintf("An item attribute (BG-32) shall carry exactly one Item attribute value (BT-161), not %d", n))
				}
			}
		}
		for _, ag := range ln.all("SpecifiedLineTradeAgreement") {
			// CEN publishes two assertions over one count, in one rule: CII-SR-439
			// requires it to be exactly one and CII-SR-441 requires it to be at
			// most one. The second is implied by the first and both are fatal, so a
			// line with two net prices is reported twice; that is what a reference
			// validator does and this transcribes it rather than deduplicating.
			n := countAt(ag, "NetPriceProductTradePrice", "ChargeAmount")
			if n != 1 {
				add("CII-SR-439", fmt.Sprintf("An invoice line shall carry exactly one Item net price (BT-146), not %d", n))
			}
			if n > 1 {
				add("CII-SR-441", "The Item net price (BT-146) shall occur at most once per invoice line")
			}
		}
	}
}

// ciiSyntaxAllowanceRules are the rules whose context is an allowance or charge
// group: `//ram:SpecifiedTradeAllowanceCharge`, which matches the document-level
// groups (BG-20/21) and the line-level ones (BG-27/28) alike, and
// `//ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge`, which is
// the item price discount (BT-147).
func ciiSyntaxAllowanceRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ac := range g.allowanceCharges {
		// (ram:ChargeIndicator) — an existence test. Without it nothing in the
		// document says whether the group is an allowance or a charge.
		if !hasChild(ac, "ChargeIndicator") {
			add("CII-SR-463", "An allowance or charge group (BG-20/21, BG-27/28) shall carry a charge indicator")
		}
		// count(ram:RateApplicablePercent) <= 1. CEN's title for this assertion
		// names the VAT category code (BT-95); its XPath counts a percentage
		// element, and the XPath is the rule.
		if atMostOnce(ac, "RateApplicablePercent") {
			add("CII-SR-471", "An allowance or charge group shall carry at most one applicable rate percentage")
		}
		// count(ram:CategoryTradeTax) <= 1
		if atMostOnce(ac, "CategoryTradeTax") {
			add("CII-SR-472", "An allowance or charge group shall carry at most one VAT category group (BT-95/96, BT-102/103)")
		}
		// count(ram:ActualAmount) <= 1
		if atMostOnce(ac, "ActualAmount") {
			add("CII-SR-473", "An allowance or charge group shall carry at most one amount (BT-92/99, BT-136/141)")
		}
	}
	for _, ap := range g.priceAllowances {
		// count(ram:ActualAmount) <= 1
		if atMostOnce(ap, "ActualAmount") {
			add("CII-SR-440", "The Item price discount (BT-147) shall occur at most once")
		}
	}
}

// ciiSyntaxHeaderRules are the rules whose context is one of the two header
// groups — ram:ApplicableHeaderTradeAgreement and
// ram:ApplicableHeaderTradeSettlement — reached by their absolute path from the
// document element.
func ciiSyntaxHeaderRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ag := range g.agreements {
		for _, c := range []struct {
			rule string
			term string
			path []string
		}{
			// count(ram:SellerTradeParty/ram:DefinedTradeContact) <= 1
			{"CII-SR-455", "The Seller contact group (BG-6)", []string{"SellerTradeParty", "DefinedTradeContact"}},
			// count(ram:BuyerTradeParty/ram:DefinedTradeContact) <= 1
			{"CII-SR-456", "The Buyer contact group (BG-9)", []string{"BuyerTradeParty", "DefinedTradeContact"}},
			// count(ram:SellerTradeParty/ram:URIUniversalCommunication) <= 1
			{"CII-SR-459", "The Seller electronic address (BT-34)", []string{"SellerTradeParty", "URIUniversalCommunication"}},
			// count(ram:BuyerTradeParty/ram:URIUniversalCommunication) <= 1
			{"CII-SR-460", "The Buyer electronic address (BT-49)", []string{"BuyerTradeParty", "URIUniversalCommunication"}},
		} {
			if atMostOnce(ag, c.path...) {
				add(c.rule, c.term+" shall occur at most once")
			}
		}
	}

	for _, st := range g.settlements {
		// count(ram:ApplicableTradeTax/ram:TaxPointDate) <= 1. BT-7 is one
		// document-level term, and the CII binding writes it inside a VAT breakdown
		// group; an invoice with several breakdowns therefore has to choose one to
		// carry it rather than repeating it in each.
		if n := countAt(st, "ApplicableTradeTax", "TaxPointDate"); n > 1 {
			add("CII-SR-461", fmt.Sprintf("The Value added tax point date (BT-7) shall occur at most once, not %d times", n))
		}
		// count(//ram:ApplicableTradeTax/ram:DueDateTypeCode) = 0 or
		// count(distinct-values(//ram:ApplicableTradeTax/ram:DueDateTypeCode)) = 1.
		// The counterpart of CII-SR-461 for BT-8, and CEN bounds it differently:
		// the code may be repeated on every breakdown as long as they agree.
		if len(g.dueDateTypeCodes) > 0 && distinctValues(g.dueDateTypeCodes) > 1 {
			add("CII-SR-462", "All Value added tax point date codes (BT-8) shall have the same value")
		}
		// count(ram:SpecifiedTradeSettlementPaymentMeans[(normalize-space(
		// ram:TypeCode) = '30' or normalize-space(ram:TypeCode) = '58') and
		// not(ram:PayeePartyCreditorFinancialAccount/ram:IBANID or
		// ram:PayeePartyCreditorFinancialAccount/ram:ProprietaryID)]) = 0.
		//
		// The count is of offending groups and the bound is zero, so the finding is
		// one per settlement however many payment instructions offend.
		for _, pm := range st.all("SpecifiedTradeSettlementPaymentMeans") {
			if !ciiIsCreditTransfer(pm) || ciiHasAccountIdentifier(pm) {
				continue
			}
			add("CII-SR-470", "A credit transfer (BG-16, BT-81 = 30 or 58) shall carry a Payment account identifier (BT-84) as an IBAN or a proprietary identifier")
			break
		}
	}
}

// ciiIsCreditTransfer is `normalize-space(ram:TypeCode) = '30' or
// normalize-space(ram:TypeCode) = '58'` — a general comparison over the
// children, so a group with several codes matches if any of them is a credit
// transfer.
func ciiIsCreditTransfer(pm *ciiNode) bool {
	for _, tc := range pm.all("TypeCode") {
		switch normalizeSpace(tc.text) {
		case "30", "58":
			return true
		}
	}
	return false
}

// ciiHasAccountIdentifier is `ram:PayeePartyCreditorFinancialAccount/ram:IBANID
// or ram:PayeePartyCreditorFinancialAccount/ram:ProprietaryID` — an existence
// test, so an empty element satisfies it. BR-50 is the rule about the value.
func ciiHasAccountIdentifier(pm *ciiNode) bool {
	for _, acc := range pm.all("PayeePartyCreditorFinancialAccount") {
		if hasChild(acc, "IBANID") || hasChild(acc, "ProprietaryID") {
			return true
		}
	}
	return false
}

// ciiSyntaxTotalsRules are the eighteen rules whose context is
// ram:SpecifiedTradeSettlementHeaderMonetarySummation. They bound every amount
// the CII summation group can carry at one occurrence each, including the eleven
// the EN 16931 core does not model — a document total this package has no
// business term for is still a document total, and two of them is still
// ambiguous.
func ciiSyntaxTotalsRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, ms := range g.summations {
		for _, c := range []struct {
			rule string
			elem string
			term string
		}{
			{"CII-SR-477", "LineTotalAmount", "The Sum of Invoice line net amount (BT-106)"},
			{"CII-SR-478", "ChargeTotalAmount", "The Sum of charges on document level (BT-108)"},
			{"CII-SR-479", "AllowanceTotalAmount", "The Sum of allowances on document level (BT-107)"},
			{"CII-SR-480", "TaxBasisTotalAmount", "The Invoice total amount without VAT (BT-109)"},
			{"CII-SR-481", "RoundingAmount", "The Rounding amount (BT-114)"},
			{"CII-SR-482", "GrandTotalAmount", "The Invoice total amount with VAT (BT-112)"},
			{"CII-SR-483", "InformationAmount", "The information amount"},
			{"CII-SR-484", "TotalPrepaidAmount", "The Paid amount (BT-113)"},
			{"CII-SR-485", "TotalDiscountAmount", "The total discount amount"},
			{"CII-SR-486", "TotalAllowanceChargeAmount", "The total allowance/charge amount"},
			{"CII-SR-487", "DuePayableAmount", "The Amount due for payment (BT-115)"},
			{"CII-SR-488", "RetailValueExcludingTaxInformationAmount", "The retail value excluding tax"},
			{"CII-SR-489", "TotalDepositFeeInformationAmount", "The total deposit fee"},
			{"CII-SR-490", "ProductValueExcludingTobaccoTaxInformationAmount", "The product value excluding tobacco tax"},
			{"CII-SR-491", "TotalRetailValueInformationAmount", "The total retail value"},
			{"CII-SR-492", "GrossLineTotalAmount", "The gross line total"},
			{"CII-SR-493", "NetLineTotalAmount", "The net line total"},
			{"CII-SR-494", "NetIncludingTaxesLineTotalAmount", "The net line total including taxes"},
		} {
			// count(ram:X) <= 1
			if atMostOnce(ms, c.elem) {
				add(c.rule, c.term+" shall occur at most once in the document totals (BG-22)")
			}
		}
	}
}

// ciiDatatypeIdentifierRules are the datatype rules about identifiers and codes:
// the seven attributes CEN forbids on the four identifiers that carry a bare
// value (BT-24, BT-1, BT-126, BT-155), the four it forbids on every other
// identifier in the document, and the two it forbids on every code.
//
// CII-DT-010, CII-DT-011 and CII-DT-012 are the three fatal datatype rules this
// package does not evaluate, and the reason is CEN's rather than this package's.
// They are bound to `/rsm:CrossIndustryInvoice/rsm:ExchangedDocument/
// ram:TypeCode`, and the rule immediately before them in the same pattern is
// bound to `//ram:TypeCode`, which matches the same element. Under ISO
// Schematron a node is processed by the first matching rule in a pattern and no
// later one, so the invoice type code is checked for @name and @listURI
// (CII-DT-008/009) and never for @listID, @listAgencyID or @listVersionID. CEN's
// own generated XSLT makes it mechanical: the `//ram:TypeCode` template has
// priority 1009 and the ExchangedDocument one 1008. Emitting them here would
// report a document no reference validator reports — the false positive this
// package exists not to produce — so Coverage(SourceEN16931) names them instead.
func ciiDatatypeIdentifierRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, id := range g.scopedIDs {
		for _, c := range []forbiddenAttr{
			{"CII-DT-001", "schemeName"},
			{"CII-DT-002", "schemeAgencyName"},
			{"CII-DT-003", "schemeDataURI"},
			{"CII-DT-004", "schemeURI"},
			{"CII-DT-005", "schemeID"},
			{"CII-DT-006", "schemeAgencyID"},
			{"CII-DT-007", "schemeVersionID"},
		} {
			if id.hasAttr(c.attr) {
				add(c.rule, fmt.Sprintf("The %s identifier shall not carry a @%s attribute", id.name, c.attr))
			}
		}
	}
	for _, id := range g.otherIDs {
		for _, c := range []forbiddenAttr{
			{"CII-DT-101", "schemeName"},
			{"CII-DT-102", "schemeAgencyName"},
			{"CII-DT-103", "schemeDataURI"},
			{"CII-DT-104", "schemeURI"},
		} {
			if id.hasAttr(c.attr) {
				add(c.rule, fmt.Sprintf("The %s identifier shall not carry a @%s attribute", id.name, c.attr))
			}
		}
	}
	for _, tc := range g.typeCodes {
		if tc.hasAttr("name") {
			add("CII-DT-008", "A code shall not carry a @name attribute")
		}
		if tc.hasAttr("listURI") {
			add("CII-DT-009", "A code shall not carry a @listURI attribute")
		}
	}
}

// ciiDatatypeReferenceRules are the sixteen rules on
// `//ram:*[ends-with(name(), 'ReferencedDocument')]` — every document reference
// in the invoice, whatever it references. EN 16931 gives a reference an
// identifier, and for a supporting document (BG-24) an attachment; the CII
// schema's ReferencedDocument type carries a great deal more, and these rules
// close all of it off.
//
// Four of the sixteen carve out an exception, and each exception is a business
// term: ram:AdditionalReferencedDocument with type code 916 is the supporting
// document (BG-24), which is entitled to its URI (BT-124), description (BT-123)
// and embedded object (BT-125); type code 130 is the invoiced object identifier
// (BT-18), which is entitled to a reference type code (BT-18-1); and
// ram:InvoiceReferencedDocument is the preceding invoice (BG-3), which is
// entitled to an issue date (BT-26).
func ciiDatatypeReferenceRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, d := range g.refDocs {
		additional := d.name == "AdditionalReferencedDocument"
		// The unconditional twelve.
		for _, c := range []forbiddenChild{
			{"CII-DT-016", "StatusCode"},
			{"CII-DT-017", "CopyIndicator"},
			{"CII-DT-019", "GlobalID"},
			{"CII-DT-020", "RevisionID"},
			{"CII-DT-023", "Information"},
			{"CII-DT-025", "SectionName"},
			{"CII-DT-026", "PreviousRevisionID"},
			{"CII-DT-028", "EffectiveSpecifiedPeriod"},
			{"CII-DT-029", "IssuerTradeParty"},
			{"CII-DT-030", "AttachedSpecifiedBinaryFile"},
		} {
			if hasChild(d, c.name) {
				add(c.rule, fmt.Sprintf("A document reference shall not carry a %s element", c.name))
			}
		}
		// not(ram:URIID) or (self::ram:AdditionalReferencedDocument and ram:TypeCode='916')
		if hasChild(d, "URIID") && !(additional && childValueIs(d, "TypeCode", "916")) {
			add("CII-DT-015", "Only a supporting document (BG-24, type code 916) may carry an external reference URI (BT-124)")
		}
		// not(ram:TypeCode) or (self::ram:AdditionalReferencedDocument) and
		// (ram:TypeCode='50' or ram:TypeCode='130' or ram:TypeCode='916')
		if hasChild(d, "TypeCode") && !(additional &&
			(childValueIs(d, "TypeCode", "50") || childValueIs(d, "TypeCode", "130") || childValueIs(d, "TypeCode", "916"))) {
			add("CII-DT-018", "Only an additional document reference may carry a document type code, and only 50, 130 or 916")
		}
		// not(ram:Name) or (self::ram:AdditionalReferencedDocument and ram:TypeCode='916')
		if hasChild(d, "Name") && !(additional && childValueIs(d, "TypeCode", "916")) {
			add("CII-DT-021", "Only a supporting document (BG-24, type code 916) may carry a description (BT-123)")
		}
		// not(ram:AttachmentBinaryObject) or (self::ram:AdditionalReferencedDocument and ram:TypeCode='916')
		if hasChild(d, "AttachmentBinaryObject") && !(additional && childValueIs(d, "TypeCode", "916")) {
			add("CII-DT-022", "Only a supporting document (BG-24, type code 916) may carry an attached document (BT-125)")
		}
		// not(ram:ReferenceTypeCode) or (self::ram:AdditionalReferencedDocument and ram:TypeCode='130')
		if hasChild(d, "ReferenceTypeCode") && !(additional && childValueIs(d, "TypeCode", "130")) {
			add("CII-DT-024", "Only the Invoiced object identifier (BT-18, type code 130) may carry a scheme identifier (BT-18-1)")
		}
		// not(ram:FormattedIssueDateTime) or self::ram:InvoiceReferencedDocument
		if hasChild(d, "FormattedIssueDateTime") && d.name != "InvoiceReferencedDocument" {
			add("CII-DT-027", "Only a preceding invoice reference (BG-3) may carry an issue date (BT-26)")
		}
	}
}

// ciiDate102 is the date form UN/EDIFACT format qualifier 102 names: CCYYMMDD,
// with a plausible month and day. CEN writes it as an XSD regular expression and
// this is the same expression; Go's syntax accepts it unchanged.
var ciiDate102 = regexp.MustCompile(`^\s*(\d{4})(1[0-2]|0[1-9]){1}(3[01]|[12][0-9]|0[1-9]){1}\s*$`)

// ciiDatatypeValueRules are the datatype rules on the value-carrying elements:
// the currency attributes on an amount, the unit attributes on a quantity, the
// tax type on a tax group, and the format of a date.
func ciiDatatypeValueRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	// CII-DT-031/032, on every amount but ram:TaxTotalAmount. The exception is
	// BT-110/BT-111, the only amounts EN 16931 lets a CII invoice denominate
	// explicitly, because an invoice with a VAT accounting currency (BT-6) carries
	// the VAT total twice, in two currencies. Every other amount is in the invoice
	// currency (BT-5) by construction and saying so again can only contradict it.
	for _, a := range g.amounts {
		if a.hasAttr("currencyID") {
			add("CII-DT-031", fmt.Sprintf("%s shall not carry a @currencyID attribute; only the VAT total (BT-110/111) may", a.name))
		}
		if a.hasAttr("currencyCodeListVersionID") {
			add("CII-DT-032", fmt.Sprintf("%s shall not carry a @currencyCodeListVersionID attribute", a.name))
		}
	}

	for _, q := range g.quantities {
		// not(@unitCode) or (/rsm:CrossIndustryInvoice/.../ram:BilledQuantity/@unitCode)
		//
		// The second operand does not mention the context node: one line's billed
		// quantity carrying a unit code satisfies the rule for every quantity in
		// the document. So this fires only on an invoice that puts a unit on some
		// quantity while no line states the unit it invoiced in.
		if q.hasAttr("unitCode") && !g.billedQuantityUnitCode {
			add("CII-DT-033", fmt.Sprintf("%s carries a @unitCode while no invoiced quantity (BT-130) states its unit of measure (BT-130-1)", q.name))
		}
		for _, c := range []forbiddenAttr{
			{"CII-DT-034", "unitCodeListID"},
			{"CII-DT-035", "unitCodeListAgencyID"},
			{"CII-DT-036", "unitCodeListAgencyName"},
		} {
			if q.hasAttr(c.attr) {
				add(c.rule, fmt.Sprintf("%s shall not carry a @%s attribute", q.name, c.attr))
			}
		}
	}

	// CII-DT-037: not(ram:TypeCode) or (ram:TypeCode = 'VAT'). A general
	// comparison, so a group naming several tax types passes if one of them is
	// VAT; a group naming none passes too, and BR-CO-09 and the category rules are
	// what have something to say about that.
	for _, tt := range g.tradeTaxes {
		if hasChild(tt, "TypeCode") && !childValueIs(tt, "TypeCode", "VAT") {
			add("CII-DT-037", fmt.Sprintf("The tax type code in %s shall be VAT", tt.name))
		}
	}

	// CII-DT-097, on every udt:DateTimeString declaring format 102.
	for _, d := range g.dates102 {
		if !ciiDate102.MatchString(d.text) {
			add("CII-DT-097", fmt.Sprintf("A date declaring format 102 shall be written YYYYMMDD, not %q", strings.TrimSpace(d.text)))
		}
	}
}

// ciiDatatypeGroupRules are the twenty-eight rules that close off the two CII
// groups whose schema type is far richer than the business terms EN 16931 binds
// to it: the invoicing period (ram:BillingSpecifiedPeriod, which carries BT-73/74
// and BT-134/135 and nothing else) and the postal address
// (ram:PostalTradeAddress, which carries BG-5/8/12/15 through ram:LineOne,
// ram:LineTwo, ram:LineThree, ram:CityName, ram:PostcodeCode,
// ram:CountrySubDivisionName and ram:CountryID, and nothing else).
func ciiDatatypeGroupRules(g *ciiSyntaxNodes, add func(rule, msg string)) {
	for _, p := range g.periods {
		// CII-DT-068 and CII-DT-072 are the two that reach a level deeper: the
		// period's start and end are dates, written as udt:DateString, and the
		// schema's alternative udt:DateTime is what the binding excludes.
		if len(nodesAt(p, "StartDateTime", "DateTime")) > 0 {
			add("CII-DT-068", "An invoicing period start date (BT-73/134) shall be written as a date string, not a DateTime")
		}
		if len(nodesAt(p, "EndDateTime", "DateTime")) > 0 {
			add("CII-DT-072", "An invoicing period end date (BT-74/135) shall be written as a date string, not a DateTime")
		}
		for _, c := range []forbiddenChild{
			{"CII-DT-069", "DurationMeasure"},
			{"CII-DT-070", "InclusiveIndicator"},
			{"CII-DT-071", "Description"},
			{"CII-DT-073", "CompleteDateTime"},
			{"CII-DT-074", "OpenIndicator"},
			{"CII-DT-075", "SeasonCode"},
			{"CII-DT-076", "ID"},
			{"CII-DT-077", "Name"},
			{"CII-DT-078", "SequenceNumeric"},
			{"CII-DT-079", "StartDateFlexibilityCode"},
			{"CII-DT-080", "ContinuousIndicator"},
			{"CII-DT-081", "PurposeCode"},
		} {
			if hasChild(p, c.name) {
				add(c.rule, fmt.Sprintf("An invoicing period (BG-14/BG-26) shall not carry a %s element", c.name))
			}
		}
	}

	for _, a := range g.addresses {
		// CII-DT-085 is absent from this table because CEN flags it advisory,
		// alone among the fifteen address rules.
		for _, c := range []forbiddenChild{
			{"CII-DT-082", "ID"},
			{"CII-DT-083", "PostOfficeBox"},
			{"CII-DT-084", "BuildingName"},
			{"CII-DT-086", "LineFour"},
			{"CII-DT-087", "LineFive"},
			{"CII-DT-088", "StreetName"},
			{"CII-DT-089", "CitySubDivisionName"},
			{"CII-DT-090", "CountryName"},
			{"CII-DT-091", "CountrySubDivisionID"},
			{"CII-DT-092", "AttentionOf"},
			{"CII-DT-093", "CareOf"},
			{"CII-DT-094", "BuildingNumber"},
			{"CII-DT-095", "DepartmentName"},
			{"CII-DT-096", "AdditionalStreetName"},
		} {
			if hasChild(a, c.name) {
				add(c.rule, fmt.Sprintf("A postal address (BG-5/8/12/15) shall not carry a %s element", c.name))
			}
		}
	}
}
