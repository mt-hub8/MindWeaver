package formalis

import (
	"fmt"
	"math"
)

// This file implements the EN 16931 VAT-category rule families (BR-S/Z/E/AE/IC/
// G/O-*). Each VAT category code (BT-118, and its line/allowance/charge form
// BT-151/95/102) constrains: which VAT breakdown groups must exist (-01), the
// permitted VAT rate on the lines and document allowances/charges that carry it
// (-05/-06/-07), the breakdown taxable-amount sum (-08), the breakdown tax
// amount (-09), and the presence of an exemption reason (-10). "Not subject to
// VAT" (O) is additionally exclusive (BR-O-11..14).

// vatCatSpec captures the EN 16931 constraints of one VAT category code.
type vatCatSpec struct {
	fam           string // rule-id family (S, Z, E, AE, IC, G, O, AF, AG)
	name          string // category name used in messages
	rate          byte   // rate on lines/allowances/charges: '+' >0, 'g' >=0, '0' ==0, '-' absent
	requireReason bool   // breakdown must (true) / must not (false) carry an exemption reason
	taxZero       bool   // breakdown VAT tax amount (BT-117) must be 0 (else tax = taxable x rate)
}

// vatCatSpecs maps the UNCL 5305 category code to its constraints. The taxed
// categories (S, and the Canary/Ceuta-Melilla IGIC/IPSI) group their breakdown
// taxable-amount sums by rate; the others carry a single (zero) rate.
var vatCatSpecs = map[string]vatCatSpec{
	"S":  {"S", "Standard rated", '+', false, false},
	"Z":  {"Z", "Zero rated", '0', false, true},
	"E":  {"E", "Exempt from VAT", '0', true, true},
	"AE": {"AE", "Reverse charge", '0', true, true},
	"K":  {"IC", "Intra-community supply", '0', true, true},
	"G":  {"G", "Export outside the EU", '0', true, true},
	"O":  {"O", "Not subject to VAT", '-', true, true},
	"L":  {"AF", "IGIC", 'g', false, false},
	"M":  {"AG", "IPSI", 'g', false, false},
}

// vatCatByRate reports whether a category's breakdown taxable-amount sum is
// grouped per VAT rate (the categories that permit more than one positive rate).
func vatCatByRate(fam string) bool {
	return fam == "S" || fam == "AF" || fam == "AG"
}

func validateVATCategories(r *run, inv *en16931Invoice, add func(rule, msg string)) {
	if r.stopped() {
		return
	}
	// Categories actually used on lines and document allowances/charges. Each is
	// checked against the UNCL 5305 category code list (BR-CL-18; the breakdown
	// category is BR-CL-17).
	used := map[string]bool{}
	clSeen := map[string]bool{}
	useCat := func(cat string) {
		if cat == "" {
			return
		}
		used[cat] = true
		if !validEN16931VATCategory(cat) && !clSeen[cat] {
			clSeen[cat] = true
			add("BR-CL-18", fmt.Sprintf("VAT category code (BT-151/95/102=%q) is not a valid UNCL 5305 value", cat))
		}
	}
	lineCats, allowCats, chargeCats := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, li := range inv.lines {
		useCat(li.vatCategory)
		if li.vatCategory != "" {
			lineCats[li.vatCategory] = true
		}
	}
	for _, ac := range inv.allowCharges {
		useCat(ac.category)
		if ac.category != "" {
			if ac.isCharge {
				chargeCats[ac.category] = true
			} else {
				allowCats[ac.category] = true
			}
		}
	}
	// BR-{fam}-02/03/04: VAT-identifier requirements for a line / allowance /
	// charge carrying a category. The standard, zero-rated, exempt, IGIC and IPSI
	// categories need a Seller VAT/tax registration or tax-representative VAT
	// identifier; export needs the Seller or tax-representative VAT identifier;
	// reverse charge and intra-community additionally need a Buyer identifier;
	// "Not subject to VAT" forbids the VAT identifiers.
	validateVATIdentifiers(inv, lineCats, allowCats, chargeCats, add)

	// Breakdown groups tallied by category. A group missing its VAT category code
	// (BT-118) is deliberately tallied under "" rather than dropped: it is still a
	// BG-23 group, and BR-O-11 below counts groups, not codes. See the note there.
	bdCats := map[string]int{}
	for _, b := range inv.vatBreakdowns {
		bdCats[b.category]++
	}

	// BR-{fam}-01: a category used on a line/allowance/charge must appear in the
	// VAT breakdown (BG-23).
	for cat := range used {
		spec, ok := vatCatSpecs[cat]
		if ok && bdCats[cat] == 0 {
			add("BR-"+spec.fam+"-01", fmt.Sprintf("an Invoice with a %q (%s) line, allowance or charge shall contain a VAT breakdown (BG-23) with that category", spec.name, cat))
		}
	}

	// BR-{fam}-05/06/07: the VAT rate on each line / allowance / charge carrying a
	// category must satisfy that category's rate constraint.
	for _, li := range inv.lines {
		checkVATRate(li.vatCategory, li.vatRate, "05", "the Invoiced item VAT rate (BT-152)", add)
	}
	for _, ac := range inv.allowCharges {
		if ac.isCharge {
			checkVATRate(ac.category, ac.rate, "07", "the Document level charge VAT rate (BT-103)", add)
		} else {
			checkVATRate(ac.category, ac.rate, "06", "the Document level allowance VAT rate (BT-96)", add)
		}
	}

	// Breakdown-level rules per category.
	for _, b := range inv.vatBreakdowns {
		spec, ok := vatCatSpecs[b.category]
		if !ok {
			continue
		}
		// -10: exemption reason present exactly when the category requires it.
		if spec.requireReason && !b.hasReason {
			add("BR-"+spec.fam+"-10", fmt.Sprintf("a VAT breakdown with category %q (%s) shall have a VAT exemption reason", spec.name, b.category))
		} else if !spec.requireReason && b.hasReason {
			add("BR-"+spec.fam+"-10", fmt.Sprintf("a VAT breakdown with category %q (%s) shall not have a VAT exemption reason", spec.name, b.category))
		}
		// -09: tax amount is zero for the non-taxed categories; for the standard
		// category it equals taxable amount x rate.
		calc, okC := parseAmount(b.calc)
		if spec.taxZero {
			if okC && math.Abs(calc) > 0.005 {
				add("BR-"+spec.fam+"-09", fmt.Sprintf("the VAT category tax amount (BT-117) for category %q (%s) shall be 0", spec.name, b.category))
			}
		} else {
			basis, okB := parseAmount(b.basis)
			rate, okR := parseAmount(b.rate)
			if okB && okC && okR && math.Abs(round2(basis*rate/100)-calc) >= vatAmountTolerance {
				add("BR-"+spec.fam+"-09", fmt.Sprintf("the VAT category tax amount (BT-117=%.2f) for category %q shall equal taxable amount x rate", calc, b.category))
			}
		}
	}

	// BR-{fam}-08: each breakdown's taxable amount equals the sum of matching
	// line net amounts + charges - allowances of the same category (and, for the
	// standard category, the same rate).
	//
	// Two preconditions, both properties of the document rather than of its
	// profile: at least one line with every line and document allowance/charge
	// carrying a VAT category — without which the operands cannot be selected —
	// and BT-107/BT-108 accounted for by the BG-20/21 entries, without which the
	// allowance/charge arm of the sum is not the quantity the totals describe and
	// the finding would accuse a conforming invoice. That second one is exactly
	// what BR-CO-11 and BR-CO-12 assert, and it is now their finding to make: this
	// rule stays quiet about a document those two already report.
	//
	// A third precondition used to sit here — !hasSubLines(inv), which removed the
	// check from every document carrying an EXTENDED sub-line structure. That is
	// C45's second gate, and like the first it answered a real hazard with far too
	// blunt an instrument. The hazard is double counting: a sub-invoice line's net
	// amount is rolled up into its parent's BT-131, so summing both counts it
	// twice. The answer is the one BR-CO-10 four hundred lines above already uses —
	// sum the top-level lines only — and newVATSummands now applies it, so a
	// document with sub-lines is summed rather than skipped.
	if len(inv.lines) > 0 && vatCategoriesComplete(inv) && docAllowanceChargesItemized(inv) {
		validateVATTaxableSums(r, inv, add)
	}

	// BR-B-01/02: the "Split payment" category (B), an Italian domestic mechanism.
	validateSplitPayment(inv, add)

	// BR-O-11..14: "Not subject to VAT" (O) is exclusive of every other category.
	//
	// A breakdown group carrying no category code at all counts as another group
	// here, so an invoice with one O group and one group missing BT-118 reports
	// both BR-47 (the missing code) and BR-O-11 (the exclusivity). That is two
	// findings for one input error, and it is deliberate: the EN 16931 UBL binding
	// counts the same way. Its BR-O-11 parameter is
	//
	//	count(cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory
	//	        [normalize-space(cbc:ID) != 'O'][...TaxScheme is VAT]) = 0
	//
	// and for a cac:TaxCategory with no cbc:ID, normalize-space(cbc:ID) is "",
	// which is != 'O', so the group is counted and the assert fails. Matching the
	// normative binding is worth more than suppressing the second finding, and the
	// rule text agrees with it: an uncategorised BG-23 group is another BG-23
	// group.
	//
	// The CII binding reads not(//ram:ApplicableTradeTax[ram:CategoryCode != 'O']),
	// where the same comparison against an absent element is a node-set test that
	// is false, so CII does not count it. The two normative bindings genuinely
	// disagree; this syntax-neutral engine follows the UBL one, which is also the
	// binding the CEN unit-test oracle exercises.
	if bdCats["O"] > 0 {
		if len(bdCats) > 1 {
			add("BR-O-11", "an Invoice with a \"Not subject to VAT\" (O) VAT breakdown shall not contain other VAT breakdown groups")
		}
		for _, li := range inv.lines {
			if li.vatCategory != "" && li.vatCategory != "O" {
				add("BR-O-12", "an Invoice with a \"Not subject to VAT\" (O) VAT breakdown shall not contain a line with another VAT category")
				break
			}
		}
		var badAllow, badCharge bool
		for _, ac := range inv.allowCharges {
			if ac.category != "" && ac.category != "O" {
				if ac.isCharge {
					badCharge = true
				} else {
					badAllow = true
				}
			}
		}
		if badAllow {
			add("BR-O-13", "an Invoice with a \"Not subject to VAT\" (O) VAT breakdown shall not contain an allowance with another VAT category")
		}
		if badCharge {
			add("BR-O-14", "an Invoice with a \"Not subject to VAT\" (O) VAT breakdown shall not contain a charge with another VAT category")
		}
	}
}

// validateSplitPayment applies the "Split payment" VAT category rules (BR-B-01/
// 02). Category B (an Italian domestic mechanism) has no breakdown/rate/reason
// family of its own, only these two constraints, so it is handled here rather
// than through vatCatSpecs. hasB/hasS consider lines, document allowances and
// charges, and the VAT breakdown — matching the Schematron bindings.
func validateSplitPayment(inv *en16931Invoice, add func(rule, msg string)) {
	hasB, hasS := false, false
	note := func(cat string) {
		switch cat {
		case "B":
			hasB = true
		case "S":
			hasS = true
		}
	}
	for _, li := range inv.lines {
		note(li.vatCategory)
	}
	for _, ac := range inv.allowCharges {
		note(ac.category)
	}
	for _, b := range inv.vatBreakdowns {
		note(b.category)
	}
	if !hasB {
		return
	}
	// BR-B-01: an Invoice using "Split payment" shall be domestic Italian — every
	// country code present shall be IT. We check the country codes we extract
	// (Seller BT-40, Buyer BT-55, tax representative BT-69, Deliver-to BT-80); a
	// non-IT value among them is a definite violation.
	for _, c := range []string{inv.sellerCountry, inv.buyerCountry, inv.taxRepCountry, inv.deliverToCountry} {
		if c != "" && c != "IT" {
			add("BR-B-01", "an Invoice using the \"Split payment\" (B) VAT category shall be a domestic Italian invoice (all country codes shall be IT)")
			break
		}
	}
	// BR-B-02: "Split payment" (B) and "Standard rated" (S) are mutually exclusive.
	if hasS {
		add("BR-B-02", "an Invoice using the \"Split payment\" (B) VAT category shall not also use the \"Standard rated\" (S) category")
	}
}

// validateVATIdentifiers applies the per-category VAT-identifier requirements
// (BR-{fam}-02 for lines, -03 for allowances, -04 for charges).
func validateVATIdentifiers(inv *en16931Invoice, lineCats, allowCats, chargeCats map[string]bool, add func(rule, msg string)) {
	sellerAny := inv.sellerVATID || inv.sellerTaxReg || inv.taxRepVATID
	sellerVATorRep := inv.sellerVATID || inv.taxRepVATID
	// idFail reports whether a category's identifier requirement is unmet.
	idFail := func(cat string) (bool, bool) {
		switch cat {
		case "S", "Z", "E", "L", "M": // seller VAT / tax registration / tax rep
			return !sellerAny, true
		case "G": // seller or tax representative VAT identifier
			return !sellerVATorRep, true
		case "AE": // seller identifier and a buyer identifier
			return !(sellerAny && (inv.buyerVATID || inv.buyerLegalReg)), true
		case "K": // seller/tax-rep VAT and buyer VAT identifier
			return !(sellerVATorRep && inv.buyerVATID), true
		case "O": // must NOT contain seller/tax-rep/buyer VAT identifiers
			return inv.sellerVATID || inv.taxRepVATID || inv.buyerVATID, true
		}
		return false, false
	}
	emit := func(cats map[string]bool, suffix string) {
		for cat := range cats {
			spec, ok := vatCatSpecs[cat]
			if !ok {
				continue
			}
			if fail, applies := idFail(cat); applies && fail {
				add("BR-"+spec.fam+"-"+suffix, fmt.Sprintf("the VAT identifier requirement for category %q (%s) is not met", spec.name, cat))
			}
		}
	}
	emit(lineCats, "02")
	emit(allowCats, "03")
	emit(chargeCats, "04")
}

// checkVATRate enforces a category's rate constraint on one line/allowance/charge.
func checkVATRate(cat, rate, suffix, label string, add func(rule, msg string)) {
	spec, ok := vatCatSpecs[cat]
	if !ok {
		return
	}
	switch spec.rate {
	case '+': // must be greater than zero
		r, ok := parseAmount(rate)
		if rate == "" || (ok && r <= 0) {
			add("BR-"+spec.fam+"-"+suffix, fmt.Sprintf("for category %q (%s), %s shall be greater than zero", spec.name, cat, label))
		}
	case 'g': // must be present and zero or greater
		r, ok := parseAmount(rate)
		if rate == "" || (ok && r < 0) {
			add("BR-"+spec.fam+"-"+suffix, fmt.Sprintf("for category %q (%s), %s shall be zero or greater", spec.name, cat, label))
		}
	case '0': // must be zero
		if r, ok := parseAmount(rate); ok && math.Abs(r) > 0.005 {
			add("BR-"+spec.fam+"-"+suffix, fmt.Sprintf("for category %q (%s), %s shall be 0", spec.name, cat, label))
		}
	case '-': // must be absent
		if rate != "" {
			add("BR-"+spec.fam+"-"+suffix, fmt.Sprintf("for category %q (%s), %s shall not be present", spec.name, cat, label))
		}
	}
}

// vatCategoriesComplete reports whether every line and document allowance/charge
// carries a VAT category code (the precondition for the -08 sum checks).
//
// Every line means every line. A sub-invoice line used to be excused here, which
// was consistent with the gate that skipped these documents outright and is not
// consistent with summing them: a sub-line with no category is an amount CEN's
// operand filter does not select and the taxable amount does count, so the sum
// would understate and the finding would accuse a conforming invoice. BR-CO-04
// is what reports that line, and it is the finding a reader can act on.
func vatCategoriesComplete(inv *en16931Invoice) bool {
	for _, li := range inv.lines {
		if li.vatCategory == "" {
			return false
		}
	}
	for _, ac := range inv.allowCharges {
		if ac.category == "" {
			return false
		}
	}
	return true
}

// docAllowanceChargesItemized reports whether every document-level allowance and
// charge the invoice claims is present as an itemizable BG-20/21 entry — the
// precondition for the BR-{fam}-08 sums, which add charges and subtract
// allowances per VAT category.
//
// The operands of that sum are the BG-20/21 entries. Some producers — EXTENDED
// in particular — carry allowance and charge amounts only in the summation
// totals BT-107/BT-108, with no entry to attribute to a category, and summing
// the entries then understates the taxable base and accuses a conforming invoice
// of BR-{fam}-08. So the question the gate has to answer is not "are there any
// allowances or charges" but "do the entries account for the totals", which is
// the same arithmetic BR-CO-11/BR-CO-12 perform in the rule engine.
//
// Which makes this the one gate in this file that is deliberately redundant with
// a rule: a document it closes on is a document BR-CO-11 or BR-CO-12 reports,
// and since the EXTENDED exemption on those two was removed (C45) that is true
// at every profile. The finding a reader needs is "your BT-107 is not the sum of
// your allowances", once, and not that plus a taxable-amount discrepancy derived
// from it. Silence here is a choice about which of two identifiers says it, not
// a rule left unevaluated.
//
// Both totals absent and no entries is the common case and passes, as before.
// A total with no entries fails, as before. An invoice whose entries sum to its
// totals now passes, where the previous gate — len(inv.allowCharges) > 0 —
// rejected it, taking the whole allowance/charge arm of the sum with it: that
// arm was unreachable, so the rule's own message promised arithmetic it could
// not perform.
//
// An entry whose amount will not parse is not accounted for by anything, so the
// gate closes rather than guessing; BR-31/BR-36 report the missing amount.
func docAllowanceChargesItemized(inv *en16931Invoice) bool {
	var allowSum, chargeSum float64
	for _, ac := range inv.allowCharges {
		v, ok := parseAmount(ac.amount)
		if !ok {
			return false
		}
		if ac.isCharge {
			chargeSum += v
		} else {
			allowSum += v
		}
	}
	// An absent total is zero: BR-CO-13 already reads it that way, and an invoice
	// with no allowances and no BT-107 is the shape this gate must let through.
	allowTotal, _ := parseAmount(inv.totals.allowanceTotal)
	chargeTotal, _ := parseAmount(inv.totals.chargeTotal)
	return math.Abs(round2(allowSum)-allowTotal) <= 0.005 &&
		math.Abs(round2(chargeSum)-chargeTotal) <= 0.005
}

// validateVATTaxableSums checks BR-{fam}-08 for every breakdown of a known
// category. For the standard category the sum is grouped by VAT rate; the other
// categories have a single (zero) rate.
func validateVATTaxableSums(r *run, inv *en16931Invoice, add func(rule, msg string)) {
	// Each breakdown sums over the same two lists, so the operands are parsed once
	// here rather than once per breakdown. The original parsed every line rate and
	// amount inside the per-breakdown scan, so an invoice with B breakdowns and L
	// lines cost B*L float parses: 7.3 MB of well-formed XML took 1.7 s and grew
	// as the square. Parsing up front reduces the scan to a float compare.
	sums := newVATSummands(inv)

	// Two breakdowns of the same category and rate select the same operands, and
	// so produce the same sum. Memoising on that key collapses the other half of
	// the quadratic — many breakdowns over one rate — to a single scan.
	type key struct {
		cat  string
		rate float64
	}
	cache := map[key]vatSum{}

	for _, b := range inv.vatBreakdowns {
		spec, ok := vatCatSpecs[b.category]
		if !ok {
			continue
		}
		basis, okBasis := parseAmount(b.basis)
		if !okBasis {
			continue
		}
		byRate := vatCatByRate(spec.fam)
		bRate, _ := parseAmount(b.rate)
		if !byRate {
			// These categories have a single (zero) rate, so the rate does not
			// select operands and must not split the cache.
			bRate = 0
		}
		k := key{b.category, bRate}
		res, hit := cache[k]
		if !hit {
			if r.stopped() || !r.spendVAT(len(sums.lines)+len(sums.allowCharges)) {
				// The caller stopped us, or the budget ran out. Report nothing
				// further: a partial sum would accuse a conforming invoice of
				// BR-*-08. The trip is already recorded on the run.
				return
			}
			res = sums.total(b.category, bRate, byRate)
			cache[k] = res
		}
		if res.complete && math.Abs(round2(res.value)-basis) > 0.005 {
			add("BR-"+spec.fam+"-08", fmt.Sprintf("the VAT category taxable amount (BT-116=%.2f) for category %q shall equal the sum of matching line net amounts + charges - allowances (%.2f)", basis, b.category, res.value))
		}
	}
}

// vatSummand is one pre-parsed operand of a VAT category sum: an invoice line's
// net amount, or a document-level allowance or charge.
type vatSummand struct {
	category string
	rate     float64
	rateOK   bool
	amount   float64
	amountOK bool
	isCharge bool
}

// vatSummands holds an invoice's summation operands with amounts and rates
// already parsed.
type vatSummands struct {
	lines        []vatSummand
	allowCharges []vatSummand
}

// vatSum is the outcome of one category/rate summation. complete is false when an
// operand's amount would not parse, in which case value says nothing.
type vatSum struct {
	value    float64
	complete bool
}

func newVATSummands(inv *en16931Invoice) *vatSummands {
	s := &vatSummands{
		lines:        make([]vatSummand, 0, len(inv.lines)),
		allowCharges: make([]vatSummand, 0, len(inv.allowCharges)),
	}
	for _, li := range inv.lines {
		// Every line, sub-lines included, and that is CEN's node set rather than a
		// simplification of it. EN16931-CII-model.sch sums
		// /rsm:CrossIndustryInvoice/rsm:SupplyChainTradeTransaction/
		// ram:IncludedSupplyChainTradeLineItem/…/ram:LineTotalAmount, and a CII
		// sub-invoice line is a *sibling* of the line it names in BT-X-9, not a
		// child of it, so CEN's step selects it. The UBL binding reaches the same
		// place from the other side: a UBL sub-line is cac:SubInvoiceLine, which
		// //cac:InvoiceLine does not match and mapUBL does not collect.
		//
		// This is worth stating because the obvious model — a parent rolls its
		// children up, so summing both double counts — is a claim about the
		// producer and not about the syntax, and the corpus contradicts it.
		// testdata/xrechnung/schematron/test/instances/cii/
		// cii-br-dex-15-test-on-sub-invoice-lines.xml carries a parent at 288,79 and
		// a sub-line at 26,07 with BT-106 and BT-116 both 314,86, which is the sum
		// of the two: nothing is rolled up. Excluding the sub-line there produces a
		// finding CEN's own validator does not, which is why neither this sum nor
		// BR-CO-10 excludes one.
		o := vatSummand{category: li.vatCategory}
		o.rate, o.rateOK = parseAmount(li.vatRate)
		o.amount, o.amountOK = parseAmount(li.netAmount)
		s.lines = append(s.lines, o)
	}
	for _, ac := range inv.allowCharges {
		o := vatSummand{category: ac.category, isCharge: ac.isCharge}
		o.rate, o.rateOK = parseAmount(ac.rate)
		o.amount, o.amountOK = parseAmount(ac.amount)
		s.allowCharges = append(s.allowCharges, o)
	}
	return s
}

// total sums the operands matching a category — and, for the rate-grouped
// standard category, a rate. The matching and the abandon-on-unparseable-amount
// behaviour reproduce the original scan exactly: an operand whose amount will not
// parse abandons the sum rather than being skipped, so no violation is reported.
func (s *vatSummands) total(cat string, rate float64, byRate bool) vatSum {
	match := func(o vatSummand) bool {
		if o.category != cat {
			return false
		}
		if byRate {
			return o.rateOK && math.Abs(o.rate-rate) < 0.005
		}
		return true
	}
	sum := 0.0
	for _, o := range s.lines {
		if match(o) {
			if !o.amountOK {
				return vatSum{}
			}
			sum += o.amount
		}
	}
	for _, o := range s.allowCharges {
		if match(o) {
			if !o.amountOK {
				return vatSum{}
			}
			if o.isCharge {
				sum += o.amount
			} else {
				sum -= o.amount
			}
		}
	}
	return vatSum{value: sum, complete: true}
}
