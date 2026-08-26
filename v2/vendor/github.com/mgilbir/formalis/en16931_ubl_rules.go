package formalis

import (
	"fmt"
	"strings"
)

// This file evaluates the fatal half of CEN's UBL syntax binding — the 54
// UBL-SR-* rules of EN16931-UBL-syntax.sch and the two UBL-CR-* rules of the
// same file that CEN flags fatal — against the parsed element tree.
//
// Why not on the syntax-neutral model, which is where every BR-* rule in this
// package lives? Because these are not statements about business terms. A BR-*
// rule says something an invoice means ("the total with VAT is the total
// without VAT plus the VAT total"), which is true or false of a Factur-X
// document and of a Peppol document alike, and the shared model exists so that
// one sentence of Go can say it for both. A UBL-SR-* rule says something about a
// UBL document tree ("cac:PaymentTerms shall carry at most one cbc:Note"), and
// it is inapplicable to CII by construction: CEN publishes a separate CII-SR-*
// binding whose rules are numbered differently, phrased differently, and do not
// correspond one-to-one. Restating CEN's element counts as fields on
// en16931Invoice would fill the shared model with facts that are meaningless for
// half its readers, and would still not make a CII invoice answerable to them.
//
// So the syntax rules read the tree, which parsed keeps alongside the model for
// exactly this reason, and they live in their own file where a reader looking
// for "the UBL binding" finds all of it in one place. cius_be.go is the
// precedent: the ubl-BE-* rules are UBL-structural and are evaluated the same
// way.
//
// Fidelity. The rules below are transcribed from the assertions in
// ubl/schematron/UBL/EN16931-UBL-syntax.sch as resolved in
// ubl/schematron/preprocessed/EN16931-UBL-validation-preprocessed.sch, which is
// the form a reference validator runs. Each rule cites its XPath. The Schematron
// contexts are element patterns, not fixed paths: `cac:PaymentMeans` matches a
// cac:PaymentMeans anywhere in the document, not only under the document
// element, and the transcription follows that. The rules whose context is the
// document element (`/ubl:Invoice | /cn:CreditNote`) are the exception and are
// applied to the root only.
//
// Severity. Every one of CEN's 54 UBL-SR-* assertions is flagged fatal — the
// binding publishes no advisory cardinality rule — so none of them is filtered
// out here. The UBL-CR-* family is the other way round: 676 of its 678 rules are
// flagged warning and two, UBL-CR-666 and UBL-CR-673, are flagged fatal, and it
// is the two that are here. That split is the whole of C27. While this file
// evaluated "the fatal UBL rules" meaning the UBL-SR-* ones, the two fatal
// UBL-CR-* rules sat unevaluated inside a coverage entry that described their
// family as advisory, so a fatal gap was filed in a line a reader would skim
// past. The remaining 676 advisory UBL-CR-* rules and the 21 advisory UBL-DT-*
// rules are evaluated too, but not here: they are generated from the same
// Schematron and run by en16931_syntax_advisory.go, which argues why a thousand
// mechanical forbidden-path rules belong in a table rather than in this file.
//
// Source. These findings are stamped SourceEN16931, not a source of their own:
// CEN publishes the syntax bindings as normative parts of EN 16931 itself, and
// a caller filtering on the authority is asking who wrote the rule, not which
// file it came out of.

// validateUBLSyntaxRules evaluates the fatal rules of CEN's UBL syntax binding —
// all 54 UBL-SR-* and the two fatal UBL-CR-* — against a UBL document tree. It
// is a no-op for any other root, so a CII invoice passing through the same entry
// point is never asked to answer a UBL rule.
//
// The rule bodies are grouped by the node population they read, which is also
// how the Schematron groups them: one <rule context=...> per group, named in
// each function's own comment.
func validateUBLSyntaxRules(r *run, root *ciiNode) []Violation {
	if root == nil || (root.name != "Invoice" && root.name != "CreditNote") {
		return nil
	}
	var out []Violation
	add := adder(&out, SourceEN16931)
	g := gatherUBLSyntaxNodes(root)

	ublSyntaxDocumentRules(g, add)
	if r.stopped() {
		return out
	}
	ublSyntaxPartyRules(g, add)
	if r.stopped() {
		return out
	}
	ublSyntaxPaymentRules(g, add)
	if r.stopped() {
		return out
	}
	ublSyntaxLineRules(g, add)
	if r.stopped() {
		return out
	}
	ublSyntaxReferenceRules(g, add)
	if r.stopped() {
		return out
	}
	ublCoreRestrictionRules(g, add)
	return out
}

// ublSyntaxNodes is every node population the rules in this file read, gathered
// in one pass.
//
// The alternative — one findAll per rule — would walk the tree once per rule,
// four dozen times over, and this package validates 1,690 UBL documents on every
// test run. One walk costs what the largest population costs. The struct also
// makes the rule bodies read as what they are: a count against a bound, with no
// traversal in the way.
type ublSyntaxNodes struct {
	// root is the document element, and the context of the twenty-eight rules
	// CEN binds to `/ubl:Invoice | /cn:CreditNote`.
	root *ciiNode
	// isCreditNote is read by UBL-SR-43, the one rule whose test names the
	// document element.
	isCreditNote bool

	addresses       []*ciiNode // cac:PostalAddress | cac:Address
	supplierParties []*ciiNode // cac:AccountingSupplierParty/cac:Party
	// addDocRefs is `//cac:AdditionalDocumentReference`, document-wide, which is
	// what UBL-SR-33/43 (context: the reference) and UBL-CR-666/673 (context: the
	// document element, reaching down with `//`) both want. collectCommon in
	// facturx_en16931.go walks the same element for BR-CL-07's scheme
	// identifiers, but it collects a code-list value off the model rather than
	// nodes, and this walk is already paid for.
	addDocRefs       []*ciiNode
	deliveries       []*ciiNode // cac:Delivery
	allowanceCharges []*ciiNode // cac:AllowanceCharge
	partyTaxSchemes  []*ciiNode // cac:PartyTaxScheme
	lines            []*ciiNode // cac:InvoiceLine | cac:CreditNoteLine
	paymentMeans     []*ciiNode // cac:PaymentMeans
	billingRefs      []*ciiNode // cac:BillingReference
	taxReps          []*ciiNode // cac:TaxRepresentativeParty
	taxSubtotals     []*ciiNode // cac:TaxSubtotal

	// payees carries each cac:PayeeParty with its parent element, because
	// UBL-SR-19/20/21 compare the payee name against `../cac:AccountingSupplier
	// Party/...` — a step up out of the context node.
	payees []ublParented

	// sepaCreditorIDs is `//cac:PartyIdentification/cbc:ID[upper-case(@schemeID)
	// = 'SEPA']`, counted document-wide for UBL-SR-29.
	sepaCreditorIDs int
	// paymentIDValues and paymentMeansCodeValues are the string values of
	// `//cbc:PaymentID` and `//cbc:PaymentMeansCode`, document-wide, for the two
	// rules that bound the number of *distinct* values rather than of elements.
	paymentIDValues        []string
	paymentMeansCodeValues []string
}

// ublParented is a node together with its parent, for the rules whose XPath
// steps up out of the context node.
type ublParented struct {
	node   *ciiNode
	parent *ciiNode
}

// gatherUBLSyntaxNodes walks the tree once and collects every population the
// rules read.
func gatherUBLSyntaxNodes(root *ciiNode) *ublSyntaxNodes {
	g := &ublSyntaxNodes{root: root, isCreditNote: root.name == "CreditNote"}
	var rec func(n, parent *ciiNode)
	rec = func(n, parent *ciiNode) {
		switch n.name {
		case "PostalAddress", "Address":
			g.addresses = append(g.addresses, n)
		case "Party":
			if parent != nil && parent.name == "AccountingSupplierParty" {
				g.supplierParties = append(g.supplierParties, n)
			}
		case "AdditionalDocumentReference":
			g.addDocRefs = append(g.addDocRefs, n)
		case "Delivery":
			g.deliveries = append(g.deliveries, n)
		case "AllowanceCharge":
			g.allowanceCharges = append(g.allowanceCharges, n)
		case "PartyTaxScheme":
			g.partyTaxSchemes = append(g.partyTaxSchemes, n)
		case "InvoiceLine", "CreditNoteLine":
			g.lines = append(g.lines, n)
		case "PayeeParty":
			g.payees = append(g.payees, ublParented{node: n, parent: parent})
		case "PaymentMeans":
			g.paymentMeans = append(g.paymentMeans, n)
		case "BillingReference":
			g.billingRefs = append(g.billingRefs, n)
		case "TaxRepresentativeParty":
			g.taxReps = append(g.taxReps, n)
		case "TaxSubtotal":
			g.taxSubtotals = append(g.taxSubtotals, n)
		case "PartyIdentification":
			for _, id := range n.all("ID") {
				if strings.EqualFold(id.attr("schemeID"), "SEPA") {
					g.sepaCreditorIDs++
				}
			}
		case "PaymentID":
			g.paymentIDValues = append(g.paymentIDValues, strings.TrimSpace(n.text))
		case "PaymentMeansCode":
			g.paymentMeansCodeValues = append(g.paymentMeansCodeValues, strings.TrimSpace(n.text))
		}
		for _, c := range n.children {
			rec(c, n)
		}
	}
	rec(root, nil)
	return g
}

// nodesAt returns every node reachable from n by the given chain of local
// names, following every branch. It is `count(a/b/c)`'s node set: the XPath step
// is over all matching children at each level, not the first.
func nodesAt(n *ciiNode, path ...string) []*ciiNode {
	if n == nil {
		return nil
	}
	cur := []*ciiNode{n}
	for _, name := range path {
		var next []*ciiNode
		for _, c := range cur {
			for _, ch := range c.children {
				if ch.name == name {
					next = append(next, ch)
				}
			}
		}
		if len(next) == 0 {
			return nil
		}
		cur = next
	}
	return cur
}

// countAt is `count(path)` relative to n.
func countAt(n *ciiNode, path ...string) int { return len(nodesAt(n, path...)) }

// atMostOnce is the shape forty of these rules share: a path that may occur once
// and does not.
func atMostOnce(n *ciiNode, path ...string) bool { return countAt(n, path...) > 1 }

// countPartyTaxSchemeIDs is `count(<party>/cac:PartyTaxScheme[cac:TaxScheme/
// upper-case(cbc:ID)='VAT']/cbc:CompanyID)` when vat is true, and the same
// predicate negated to `!='VAT'` when it is false. It is how the binding tells
// the VAT identifier (BT-31/48/63) apart from any other tax registration
// (BT-32), which share the one UBL element.
//
// The predicate is a general comparison over `cac:TaxScheme/upper-case(cbc:ID)`,
// which has three outcomes rather than two, and the third is why this is not
// simply `isVAT` and its negation:
//
//   - a cac:TaxScheme naming VAT satisfies the first predicate and not the
//     second;
//   - a cac:TaxScheme naming anything else, including one with no cbc:ID at all
//     (upper-case(()) is the empty string, which is not 'VAT'), satisfies the
//     second and not the first;
//   - a cac:PartyTaxScheme with no cac:TaxScheme at all yields an empty sequence
//     and satisfies *neither*, so it is counted by neither rule. UBL-SR-53 is
//     the rule that has something to say about that group.
func countPartyTaxSchemeIDs(party *ciiNode, vat bool) int {
	n := 0
	for _, pts := range party.all("PartyTaxScheme") {
		schemes := pts.all("TaxScheme")
		if len(schemes) == 0 {
			continue
		}
		isVAT := false
		for _, ts := range schemes {
			if strings.EqualFold(strings.TrimSpace(ts.str("ID")), "VAT") {
				isVAT = true
			}
		}
		if isVAT == vat {
			n += countAt(pts, "CompanyID")
		}
	}
	return n
}

// distinctValues counts how many distinct strings a slice holds. It is
// `count(//x[not(preceding::x/. = .)])` — the number of distinct string values
// of an element, which is what UBL-SR-44 and UBL-SR-47 bound rather than the
// number of elements.
func distinctValues(vals []string) int {
	seen := make(map[string]bool, len(vals))
	for _, v := range vals {
		seen[v] = true
	}
	return len(seen)
}

// ublSyntaxDocumentRules are the rules CEN binds to the document element itself
// — `<rule context="/ubl:Invoice | /cn:CreditNote">` — including the three whose
// test reaches document-wide from there with a `//` step. Each bounds how often
// a business term may appear on one invoice.
func ublSyntaxDocumentRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	root := g.root
	for _, c := range []struct {
		rule string
		term string
		path []string
	}{
		// count(cac:ContractDocumentReference/cbc:ID) <= 1
		{"UBL-SR-01", "The Contract reference (BT-12)", []string{"ContractDocumentReference", "ID"}},
		// count(cac:ReceiptDocumentReference/cbc:ID) <= 1
		{"UBL-SR-02", "The Receiving advice reference (BT-15)", []string{"ReceiptDocumentReference", "ID"}},
		// count(cac:DespatchDocumentReference/cbc:ID) <= 1
		{"UBL-SR-03", "The Despatch advice reference (BT-16)", []string{"DespatchDocumentReference", "ID"}},
		// count(cac:PaymentTerms/cbc:Note) <= 1
		{"UBL-SR-05", "The Payment terms (BT-20)", []string{"PaymentTerms", "Note"}},
		// count(cac:InvoicePeriod) <= 1
		{"UBL-SR-08", "The Invoicing period (BG-14)", []string{"InvoicePeriod"}},
		// count(cac:Delivery) <= 1
		{"UBL-SR-24", "The Deliver to information (BG-13)", []string{"Delivery"}},
		// count(cac:ProjectReference/cbc:ID) <= 1
		{"UBL-SR-39", "The Project reference (BT-11)", []string{"ProjectReference", "ID"}},
		// count(cac:PaymentMeans/cbc:PaymentDueDate) <= 1
		{"UBL-SR-45", "The Payment due date (BT-9)", []string{"PaymentMeans", "PaymentDueDate"}},
		// count(cac:InvoicePeriod/cbc:DescriptionCode) <= 1
		{"UBL-SR-49", "The Value added tax point date code (BT-8)", []string{"InvoicePeriod", "DescriptionCode"}},
		// count(cac:PaymentMeans/cac:CardAccount) <= 1
		{"UBL-SR-54", "The Payment card information group (BG-18)", []string{"PaymentMeans", "CardAccount"}},
		// count(cac:PaymentMeans/cac:PaymentMandate) <= 1
		{"UBL-SR-55", "The Direct debit group (BG-19)", []string{"PaymentMeans", "PaymentMandate"}},
		// count(cac:OriginatorDocumentReference/cbc:ID) <= 1
		{"UBL-SR-56", "The Tender or lot reference (BT-17)", []string{"OriginatorDocumentReference", "ID"}},
	} {
		if atMostOnce(root, c.path...) {
			add(c.rule, c.term+" shall occur at most once")
		}
	}

	// UBL-SR-04: count(cac:AdditionalDocumentReference[cbc:DocumentTypeCode='130']
	// /cbc:ID) <= 1. The Invoiced object identifier (BT-18) is the supporting
	// document whose type code is 130; the other cac:AdditionalDocumentReference
	// entries are BG-24 attachments and are not bounded here.
	//
	// The path is relative to the context node, which for this rule is the
	// document element, so the references on it are the ones counted — unlike
	// UBL-SR-33 and UBL-SR-43, whose context is the reference itself and which
	// therefore reach every one in the document.
	objectIDs := 0
	for _, d := range root.all("AdditionalDocumentReference") {
		if !ublDocRefHasTypeCode(d, "130") {
			continue
		}
		objectIDs += countAt(d, "ID")
	}
	if objectIDs > 1 {
		add("UBL-SR-04", "The Invoiced object identifier (BT-18) shall occur at most once")
	}

	// UBL-SR-46: count(cac:PaymentMeans/cbc:PaymentMeansCode/@name) <= 1. The
	// Payment means text (BT-82) is an attribute of the code, not an element of
	// its own, so this counts attributes rather than elements.
	names := 0
	for _, pmc := range nodesAt(root, "PaymentMeans", "PaymentMeansCode") {
		if pmc.hasAttr("name") {
			names++
		}
	}
	if names > 1 {
		add("UBL-SR-46", "The Payment means text (BT-82) shall occur at most once")
	}

	// UBL-SR-29: count(//cac:PartyIdentification/cbc:ID[upper-case(@schemeID) =
	// 'SEPA']) <= 1. The Bank assigned creditor identifier (BT-90) is written as
	// a party identifier tagged SEPA, and may be written on the Seller or on the
	// Payee — hence the document-wide count.
	if g.sepaCreditorIDs > 1 {
		add("UBL-SR-29", "The Bank assigned creditor identifier (BT-90) shall occur at most once")
	}

	// UBL-SR-44: count(//cbc:PaymentID[not(preceding::cbc:PaymentID/. = .)]) <= 1.
	// The bound is on distinct values, not on elements: one payment reference may
	// legitimately be repeated on every cac:PaymentMeans.
	if distinctValues(g.paymentIDValues) > 1 {
		add("UBL-SR-44", "The Payment reference (BT-83) shall occur at most once, though one value may be repeated on several payment means")
	}
	// UBL-SR-47: the same shape for the payment means code (BT-81) — several
	// cac:PaymentMeans are allowed, but they shall agree.
	if distinctValues(g.paymentMeansCodeValues) > 1 {
		add("UBL-SR-47", "All Payment means type codes (BT-81) shall have the same value")
	}
}

// ublDocRefHasTypeCode reports whether a cac:AdditionalDocumentReference carries
// a cbc:DocumentTypeCode with the given value. CEN writes the predicate
// `cbc:DocumentTypeCode='130'`, an existential over the children, so a reference
// with several codes matches if any of them does.
func ublDocRefHasTypeCode(d *ciiNode, code string) bool {
	for _, tc := range d.all("DocumentTypeCode") {
		if strings.TrimSpace(tc.text) == code {
			return true
		}
	}
	return false
}

// ublSyntaxPartyRules are the rules about the parties: the seller and buyer
// terms CEN counts from the document element, and the rules whose context is a
// party element in its own right — cac:AccountingSupplierParty/cac:Party,
// cac:PartyTaxScheme, cac:TaxRepresentativeParty, cac:PayeeParty, cac:Delivery
// and every cac:PostalAddress or cac:Address.
func ublSyntaxPartyRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	root := g.root
	for _, c := range []struct {
		rule string
		term string
		path []string
	}{
		// count(cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName) <= 1
		{"UBL-SR-09", "The Seller name (BT-27)", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"}},
		// count(cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name) <= 1
		{"UBL-SR-10", "The Seller trading name (BT-28)", []string{"AccountingSupplierParty", "Party", "PartyName", "Name"}},
		// count(cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID) <= 1
		{"UBL-SR-11", "The Seller legal registration identifier (BT-30)", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "CompanyID"}},
		// count(cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm) <= 1
		{"UBL-SR-14", "The Seller additional legal information (BT-33)", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "CompanyLegalForm"}},
		// count(cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:RegistrationName) <= 1
		{"UBL-SR-15", "The Buyer name (BT-44)", []string{"AccountingCustomerParty", "Party", "PartyLegalEntity", "RegistrationName"}},
		// count(cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification/cbc:ID) <= 1
		{"UBL-SR-16", "The Buyer identifier (BT-46)", []string{"AccountingCustomerParty", "Party", "PartyIdentification", "ID"}},
		// count(cac:AccountingCustomerParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyID) <= 1
		{"UBL-SR-17", "The Buyer legal registration identifier (BT-47)", []string{"AccountingCustomerParty", "Party", "PartyLegalEntity", "CompanyID"}},
		// count(cac:AccountingCustomerParty/cac:Party/cac:PartyName/cbc:Name) <= 1
		{"UBL-SR-40", "The Buyer trading name (BT-45)", []string{"AccountingCustomerParty", "Party", "PartyName", "Name"}},
	} {
		if atMostOnce(root, c.path...) {
			add(c.rule, c.term+" shall occur at most once")
		}
	}

	// UBL-SR-12/13/18: the VAT identifier and the other tax registration are the
	// same UBL element (cac:PartyTaxScheme/cbc:CompanyID) told apart by the tax
	// scheme name, so each is counted under its own predicate.
	seller := root.child("AccountingSupplierParty", "Party")
	buyer := root.child("AccountingCustomerParty", "Party")
	if countPartyTaxSchemeIDs(seller, true) > 1 {
		add("UBL-SR-12", "The Seller VAT identifier (BT-31) shall occur at most once")
	}
	if countPartyTaxSchemeIDs(seller, false) > 1 {
		add("UBL-SR-13", "The Seller tax registration identifier (BT-32) shall occur at most once")
	}
	if countPartyTaxSchemeIDs(buyer, true) > 1 {
		add("UBL-SR-18", "The Buyer VAT identifier (BT-48) shall occur at most once")
	}

	// UBL-SR-42: count(cac:PartyTaxScheme) <= 2, on cac:AccountingSupplierParty/
	// cac:Party. Two, not one: the seller may carry a VAT identifier (BT-31) and
	// one other tax registration (BT-32), and each is its own cac:PartyTaxScheme.
	for _, p := range g.supplierParties {
		if countAt(p, "PartyTaxScheme") > 2 {
			add("UBL-SR-42", "The Seller shall carry at most two party tax schemes (BT-31 and BT-32)")
		}
	}

	// UBL-SR-53: exists(cac:TaxScheme/cbc:ID) and exists(cbc:CompanyID), on every
	// cac:PartyTaxScheme. A tax scheme group says "this party is registered under
	// this scheme with this identifier"; either half alone says nothing. The test
	// is existence, not content, so an empty element satisfies it — the rule is
	// about the shape of the group and BR-CO-09 is about the value.
	for _, pts := range g.partyTaxSchemes {
		if countAt(pts, "TaxScheme", "ID") == 0 || countAt(pts, "CompanyID") == 0 {
			add("UBL-SR-53", "A party tax scheme shall carry both a tax scheme identifier and a company identifier (BT-31/32/48/63)")
		}
	}

	// UBL-SR-22/23, on cac:TaxRepresentativeParty.
	for _, tr := range g.taxReps {
		if atMostOnce(tr, "PartyName", "Name") {
			add("UBL-SR-22", "The Seller tax representative name (BT-62) shall occur at most once")
		}
		if atMostOnce(tr, "PartyTaxScheme", "CompanyID") {
			add("UBL-SR-23", "The Seller tax representative VAT identifier (BT-63) shall occur at most once")
		}
	}

	// UBL-SR-25, on cac:Delivery.
	for _, d := range g.deliveries {
		if atMostOnce(d, "DeliveryParty", "PartyName", "Name") {
			add("UBL-SR-25", "The Deliver to party name (BT-70) shall occur at most once")
		}
	}

	// UBL-SR-51: not(cac:AddressLine) or count(cac:AddressLine) = 1, on every
	// cac:PostalAddress and cac:Address in the document. EN 16931 gives an
	// address three lines; UBL spends the first two on cbc:StreetName and
	// cbc:AdditionalStreetName, which leaves one cac:AddressLine for the third.
	for _, a := range g.addresses {
		if countAt(a, "AddressLine") > 1 {
			add("UBL-SR-51", "An address shall carry at most one additional address line (BT-163/165/168/172)")
		}
	}

	// UBL-SR-19/20/21, on cac:PayeeParty. These three carry a second conjunct
	// that the other fifty-one do not:
	//
	//   (cac:PartyName/cbc:Name) != (../cac:AccountingSupplierParty/cac:Party/
	//                                cac:PartyLegalEntity/cbc:RegistrationName)
	//
	// EN 16931 admits the Payee group (BG-10) only when the payee is someone
	// other than the seller, and CEN enforces that here rather than as a BR-*
	// rule. The XPath is a general comparison over two node sequences, so it is
	// true when some payee name differs from some seller name — and false, which
	// fails the assertion, when either sequence is empty. That is deliberate on
	// the seller side (BR-06 makes the seller name mandatory) and on the payee
	// side (BR-17 makes the payee name mandatory once the group is present), so
	// an empty operand is already a defect the core reports; this rule adds its
	// own finding rather than staying silent, exactly as the reference validator
	// does.
	for _, p := range g.payees {
		differs := ublPayeeDiffersFromSeller(p)
		if atMostOnce(p.node, "PartyName", "Name") || !differs {
			add("UBL-SR-19", "The Payee name (BT-59) shall occur at most once, and the Payee group (BG-10) shall only be used when the payee differs from the seller")
		}
		if ublCountPayeeIdentifiers(p.node) > 1 || !differs {
			add("UBL-SR-20", "The Payee identifier (BT-60) shall occur at most once, and the Payee group (BG-10) shall only be used when the payee differs from the seller")
		}
		if atMostOnce(p.node, "PartyLegalEntity", "CompanyID") || !differs {
			add("UBL-SR-21", "The Payee legal registration identifier (BT-61) shall occur at most once, and the Payee group (BG-10) shall only be used when the payee differs from the seller")
		}
	}
}

// ublPayeeDiffersFromSeller evaluates
// `(cac:PartyName/cbc:Name) != (../cac:AccountingSupplierParty/cac:Party/
// cac:PartyLegalEntity/cbc:RegistrationName)` for one cac:PayeeParty: a general
// comparison, true when some payee name differs from some seller name and false
// when either side is empty.
func ublPayeeDiffersFromSeller(p ublParented) bool {
	payee := nodesAt(p.node, "PartyName", "Name")
	seller := nodesAt(p.parent, "AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName")
	for _, a := range payee {
		for _, b := range seller {
			if strings.TrimSpace(a.text) != strings.TrimSpace(b.text) {
				return true
			}
		}
	}
	return false
}

// ublCountPayeeIdentifiers is `count(cac:PartyIdentification/cbc:ID[
// upper-case(@schemeID) != 'SEPA'])`. The SEPA-tagged identifier is BT-90, the
// bank assigned creditor identifier, which UBL-SR-29 bounds separately; what is
// counted here is BT-60.
func ublCountPayeeIdentifiers(payee *ciiNode) int {
	n := 0
	for _, id := range nodesAt(payee, "PartyIdentification", "ID") {
		if !strings.EqualFold(id.attr("schemeID"), "SEPA") {
			n++
		}
	}
	return n
}

// ublSyntaxPaymentRules are the rules whose context is a cac:PaymentMeans, plus
// UBL-SR-32 on cac:TaxSubtotal and the two allowance/charge reason rules, which
// share one test and differ only in which of the two contexts a group falls in.
func ublSyntaxPaymentRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	for _, pm := range g.paymentMeans {
		// count(cbc:PaymentID) <= 1 — within one payment means group, unlike
		// UBL-SR-44, which bounds the distinct values across the document.
		if atMostOnce(pm, "PaymentID") {
			add("UBL-SR-26", "The Payment reference (BT-83) shall occur at most once in a payment instructions group")
		}
		// count(cbc:PaymentMeansCode) <= 1
		if atMostOnce(pm, "PaymentMeansCode") {
			add("UBL-SR-27", "The Payment means type code (BT-81) shall occur at most once in a payment instructions group")
		}
		// count(cac:PaymentMandate/cbc:ID) <= 1
		if atMostOnce(pm, "PaymentMandate", "ID") {
			add("UBL-SR-28", "The Mandate reference identifier (BT-89) shall occur at most once")
		}
	}

	// UBL-SR-32: count(cac:TaxCategory/cbc:TaxExemptionReason) <= 1, on every
	// cac:TaxSubtotal.
	for _, ts := range g.taxSubtotals {
		if atMostOnce(ts, "TaxCategory", "TaxExemptionReason") {
			add("UBL-SR-32", "The VAT exemption reason text (BT-120) shall occur at most once per VAT breakdown")
		}
	}

	// UBL-SR-30/31: count(cbc:AllowanceChargeReason) <= 1, on every
	// cac:AllowanceCharge, with the identifier chosen by cbc:ChargeIndicator.
	// CEN writes the two contexts as `[cbc:ChargeIndicator = false()]` and
	// `[... = true()]`, which under XPath 2.0 casts the element's text to
	// xs:boolean; a group whose indicator is neither matches no context, and so
	// is not checked here at all.
	for _, ac := range g.allowanceCharges {
		if !atMostOnce(ac, "AllowanceChargeReason") {
			continue
		}
		switch ublChargeIndicator(ac) {
		case ublIndicatorCharge:
			add("UBL-SR-31", "The Charge reason (BT-104/144) shall occur at most once")
		case ublIndicatorAllowance:
			add("UBL-SR-30", "The Allowance reason (BT-97/139) shall occur at most once")
		}
	}
}

// The three states of a cac:ChargeIndicator as the two Schematron contexts see
// it: a charge, an allowance, or a value neither context matches.
const (
	ublIndicatorNeither = iota
	ublIndicatorCharge
	ublIndicatorAllowance
)

func ublChargeIndicator(ac *ciiNode) int {
	switch strings.ToLower(strings.TrimSpace(ac.str("ChargeIndicator"))) {
	case "true", "1":
		return ublIndicatorCharge
	case "false", "0":
		return ublIndicatorAllowance
	}
	return ublIndicatorNeither
}

// ublSyntaxLineRules are the rules whose context is an invoice or credit note
// line.
func ublSyntaxLineRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	for _, ln := range g.lines {
		for _, c := range []struct {
			rule string
			term string
			path []string
		}{
			// count(cbc:Note) <= 1
			{"UBL-SR-34", "The Invoice line note (BT-127)", []string{"Note"}},
			// count(cac:OrderLineReference/cbc:LineID) <= 1
			{"UBL-SR-35", "The Referenced purchase order line reference (BT-132)", []string{"OrderLineReference", "LineID"}},
			// count(cac:InvoicePeriod) <= 1
			{"UBL-SR-36", "The Invoice line period (BG-26)", []string{"InvoicePeriod"}},
			// count(cac:Price/cac:AllowanceCharge/cbc:Amount) <= 1
			{"UBL-SR-37", "The Item price discount (BT-147)", []string{"Price", "AllowanceCharge", "Amount"}},
			// count(cac:Item/cbc:Description) <= 1
			{"UBL-SR-50", "The Item description (BT-154)", []string{"Item", "Description"}},
			// count(cac:DocumentReference) <= 1
			{"UBL-SR-52", "The Invoice line object identifier (BT-128)", []string{"DocumentReference"}},
		} {
			if atMostOnce(ln, c.path...) {
				add(c.rule, c.term+" shall occur at most once per invoice line")
			}
		}
		// UBL-SR-48: count(cac:Item/cac:ClassifiedTaxCategory) = 1. The only
		// UBL-SR-* rule that bounds a term from below as well as from above: a
		// line's VAT category (BT-151) and rate (BT-152) are mandatory, and a
		// second classified tax category would make the line's VAT ambiguous.
		if n := countAt(ln, "Item", "ClassifiedTaxCategory"); n != 1 {
			add("UBL-SR-48", fmt.Sprintf("An invoice line shall have exactly one classified tax category (BT-151), not %d", n))
		}
	}
}

// ublSyntaxReferenceRules are the rules on cac:BillingReference and
// cac:AdditionalDocumentReference.
func ublSyntaxReferenceRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	for _, br := range g.billingRefs {
		// count(cac:InvoiceDocumentReference) <= 1
		if atMostOnce(br, "InvoiceDocumentReference") {
			add("UBL-SR-06", "A preceding invoice reference group (BG-3) shall carry at most one preceding invoice")
		}
		// (cac:InvoiceDocumentReference/cbc:ID) — an existence test, so a billing
		// reference that names no preceding invoice number is a violation.
		//
		// This is the same defect BR-55 reports on the model, and CEN reports it
		// twice for the same reason: the semantic rule says the term is required
		// and the binding says which element carries it. A reference validator
		// emits both, and so does this one.
		if countAt(br, "InvoiceDocumentReference", "ID") == 0 {
			add("UBL-SR-07", "A preceding invoice reference (BG-3) shall carry the preceding invoice number (BT-25)")
		}
	}

	for _, d := range g.addDocRefs {
		// count(cbc:DocumentDescription) <= 1
		if atMostOnce(d, "DocumentDescription") {
			add("UBL-SR-33", "The Supporting document description (BT-123) shall occur at most once")
		}
		// (cbc:DocumentTypeCode='130') or ((local-name(/*) = 'CreditNote') and
		// (cbc:DocumentTypeCode='50')) or (not(cbc:ID/@schemeID) and
		// not(cbc:DocumentTypeCode)).
		//
		// cac:AdditionalDocumentReference carries two different business terms:
		// the Invoiced object identifier (BT-18), which is marked by document
		// type code 130 and is the only one entitled to a scheme identifier, and
		// a supporting document (BG-24), which carries neither. The rule keeps
		// the two apart.
		switch {
		case ublDocRefHasTypeCode(d, "130"):
		case g.isCreditNote && ublDocRefHasTypeCode(d, "50"):
		case len(d.all("DocumentTypeCode")) == 0 && !ublHasIDScheme(d):
		default:
			add("UBL-SR-43", "A supporting document reference (BG-24) shall carry no scheme identifier and no document type code; only the Invoiced object identifier (BT-18, type code 130) may")
		}
	}
}

// ublCoreRestrictionRules are the two rules of CEN's 678-rule UBL-CR-* family
// that CEN flags fatal:
//
//	UBL-CR-666  not(//cac:AdditionalDocumentReference[cbc:DocumentTypeCode = '130']/cac:Attachment)
//	UBL-CR-673  not(//cac:AdditionalDocumentReference[cbc:DocumentTypeCode  = '130']/cbc:DocumentDescription)
//
// Both are asserted on the document element (`<rule context="/ubl:Invoice |
// /cn:CreditNote">`) and reach document-wide from there with a `//` step, so each
// is one verdict about the whole invoice however many references are at fault —
// hence the two booleans below rather than a finding per reference. UBL-SR-33 and
// UBL-SR-43, whose context is the reference itself, are the contrast.
//
// What they forbid. cac:AdditionalDocumentReference carries two unrelated
// business terms, told apart by cbc:DocumentTypeCode: with the code 130 it is the
// Invoiced object identifier (BT-18), a bare identifier of the thing being billed
// for — a meter, a subscription, a contract line — and with no code it is a
// supporting document (BG-24), which may carry a description (BT-123) and an
// attachment (BT-125). The two must not be the same element. A reference that
// says "type code 130" and also carries an attachment or a description is
// claiming to be both terms at once, and a receiver has no way to tell which term
// the cbc:ID was meant to be. UBL-SR-43 polices the same boundary from the other
// direction, refusing a scheme identifier or a type code on a supporting
// document; these two refuse the payload of a supporting document on an invoiced
// object identifier.
//
// Why CEN needs two rules rather than one. They are not the Invoice and
// CreditNote bindings of one constraint — the context `$Invoice` resolves to
// `/ubl:Invoice | /cn:CreditNote`, so one assertion already covers both roots,
// and UBL-CR-672 is what a CreditNote-specific rule looks like in this family.
// They are two genuinely different shapes: cac:Attachment is an aggregate holding
// the attachment itself (BT-125, or the external URI BT-124), cbc:DocumentDescription
// is the free-text description (BT-123), and they are separate children of
// cac:AdditionalDocumentReference. Written as one rule with an `or`, a violating
// document would be told only that its reference is malformed; as two, it is told
// which of the two things it has to remove. The family is generated one forbidden
// path at a time — that is what makes 678 of them — so one path per rule is also
// simply the shape of the family.
//
// Corpus. Neither rule fires on any of the 1,690 documents, and that is the
// rules being right rather than the rules being wired to nothing. Of the 785
// documents with a UBL Invoice or CreditNote root, 58 carry a BT-18 reference,
// 235 carry a cac:AdditionalDocumentReference/cac:Attachment and 182 a
// cbc:DocumentDescription — and in 50 of them a BT-18 reference and an attachment
// or a description occur in the same invoice, on different
// cac:AdditionalDocumentReference elements. So the corpus exercises both halves of
// each predicate and the per-reference scoping that keeps them apart; a
// transcription that lost that scoping — asking whether the document has a 130
// reference and, anywhere, an attachment — would accuse those 50 conforming
// invoices, which is how "fires on nothing" was told apart from "is wired to
// nothing" here.
func ublCoreRestrictionRules(g *ublSyntaxNodes, add func(rule, msg string)) {
	var attachment, description bool
	for _, d := range g.addDocRefs {
		if !ublDocRefHasTypeCode(d, "130") {
			continue
		}
		// Child steps, as CEN writes them: an attachment nested deeper (a
		// supporting document's own cac:Attachment, in a document that has one)
		// is a different reference's business.
		if len(d.all("Attachment")) > 0 {
			attachment = true
		}
		if len(d.all("DocumentDescription")) > 0 {
			description = true
		}
	}
	if attachment {
		add("UBL-CR-666", "The Invoiced object identifier (BT-18) shall not be carried on a document reference that also carries an attachment (BG-24): "+
			"a cac:AdditionalDocumentReference with document type code 130 shall have no cac:Attachment")
	}
	if description {
		add("UBL-CR-673", "The Invoiced object identifier (BT-18) shall not be carried on a document reference that also carries a supporting document "+
			"description (BT-123): a cac:AdditionalDocumentReference with document type code 130 shall have no cbc:DocumentDescription")
	}
}

// ublHasIDScheme reports whether any cbc:ID child carries a @schemeID. CEN
// writes it `not(cbc:ID/@schemeID)`, an existence test on the attribute node, so
// an empty schemeID counts as present — hence hasAttr rather than a comparison
// against "".
func ublHasIDScheme(d *ciiNode) bool {
	for _, id := range d.all("ID") {
		if id.hasAttr("schemeID") {
			return true
		}
	}
	return false
}
