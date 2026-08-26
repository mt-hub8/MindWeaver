package formalis

// Coverage: what this package checked, and what it did not.
//
// # Why a validator has to say what it did not check
//
// This package already draws one epistemic line with unusual care. limits.go
// spends eighty lines arguing that "the checker stopped" must never look like
// "the invoice is clean", and it backs the argument with RuleLimit,
// IsCheckerViolation and tests that pin the property. But there is a third
// state, and until this file existed it was folded into "clean": *the checks
// that exist all passed, and the rule set they come from is a documented
// subset*.
//
// Every rule set in this package is such a subset. Four CIUS say so in a file
// comment; the twelve national formats check "the mandatory structure and code
// lists" rather than the XSD or Schematron their authority publishes; and the
// EN 16931 core itself — the one rule set that looked complete, because the
// CEN unit-test oracle reports 198/198 — implemented, when this table was
// written, none of the CII datatype bindings and five of the fifty-four fatal
// UBL syntax rules. None of that reached a caller. Both bindings' fatal halves
// are evaluated now, and the table below is how that became a statement someone
// can check rather than a claim in a commit message. A Portuguese integrator
// could run ValidateCIUSPT, read
// len(v) == 0, file with AT, and be rejected on BR-CIUS-PT-13: a rule this
// package never evaluated and never said it had not evaluated.
//
// The consequence of confusing "unknown" with "clean" is the same in both
// cases — an invoice ships that should not have — so the two belong behind one
// question. Report.Complete is that question and Report.Conformant is the
// single predicate a caller can safely key on: it is false for a run that
// stopped early *and* for a run whose rule set has holes an authority would
// reject an invoice over.
//
// The severity on each family is what keeps the second half usable. A rule set
// with no fatal gaps left can be conformant while still not complete, so
// implementing an authority's advisory tier improves the report without being
// the price of a verdict, and a fatal gap can never be filed inside an entry
// whose prose reads as advisory — which had already happened once (C27).
//
// The evaluability of each family is what keeps it *answerable*. An authority can
// publish a rule its own reference implementation cannot honour, and CEN publishes
// seven; a table that could only say "not evaluated" about those made the strict
// question permanently unanswerable, for a reason no work could change. Saying so
// in a field of its own is what lets one rule set here finally report that it saw
// everything a reference validator could. See RuleFamily.Unevaluable, and read its
// boundary carefully: it is the field that would do the most damage if it widened.
//
// # Why the table is static
//
// Coverage takes a Source and reads a table. It parses nothing, allocates one
// copy of a slice of constants, and cannot fail, because what a rule set omits
// is a property of this package rather than of any document. That is what lets
// a caller ask the question *before* deciding to trust an answer, and it is
// what lets the table be the single source of truth: newReport builds
// Report.NotEvaluated from it, so the strings a caller reads are the strings
// the table holds and there is no second list to drift.
//
// # Why Source and not CIUS
//
// The obvious key is CIUS, since the CIUS validators are where the problem was
// first noticed. It does not work: CIUS names seven rule sets, and the thirteen
// tree-reading validators (FatturaPA, Facturae, ebInterface, KSeF, Finvoice,
// TEAPPS, OIOUBL, Svefaktura, ZATCA, NAV OSA, UBL-TR, PINT, Order-X) are not
// CIUS and are the *most* partial rule sets in the package — CIUS would have
// left every one of them unable to declare its coverage. Source already names
// every rule-authoring authority here, the seven CIUS among them, so it is the
// key that makes the table total.

// RuleFamily names a group of rules an authority publishes and this package does
// not evaluate, in a form a caller can look up in that authority's own
// documentation.
//
// # Three facts, three fields
//
// A gap has three independent properties, and the type carries one field for
// each because collapsing any two of them has already gone wrong here.
//
// Severity is the authority's published flag, quoted, unconditionally. It is not
// this package's estimate of what the gap costs. Every entry in the table can be
// checked against the artefact its authority publishes wherever this repository
// vendors one, and TestCoverageSeveritiesMatchThePublishedFlag does exactly that
// with no exceptions at all.
//
// Unevaluable is whether the authority published a rule *nobody* can evaluate —
// see the field's own comment, which is where the boundary is drawn, because it
// is the field most likely to be abused.
//
// Reason is the prose, and for an unevaluable family it is where the evidence
// goes.
//
// # Why they cannot be collapsed
//
// Severity and Unevaluable were one column until D10. To keep Report.Conformant
// from being false forever over rules CEN itself cannot honour, six entries CEN
// flags fatal were recorded at SeverityWarning, and the contradiction was kept
// legal by a hand-maintained list of those six identifiers inside the test that
// checks the column against the published flag. A column that needs an excuse
// list is answering two questions, and the excuse list is the load-bearing part:
// it had to be edited by hand every time such a rule was found, which is the
// same failure mode as a coverage claim in a file comment.
//
// The cost of that collapse was not only tidiness. Report.Complete could never
// be true for any document, because CEN publishes seven rules nobody can
// evaluate and the table could not say so in a form a predicate could read — so
// the question "did this package see everything a reference validator would"
// had a permanent answer of no, for a reason a caller could not tell apart from
// "not implemented yet". That is the trap Conformant was in before severity
// arrived, one level down.
//
// A third Severity value was rejected rather than overlooked. Unevaluability is
// orthogonal to severity, not a further point on the same scale, and Severity's
// own comment argues against a third value for a reason that applies here too:
// it would stop Report.Fatal and Report.Warnings being a partition of
// Violations.
type RuleFamily struct {
	// Rules is the identifier or range the authority uses — "BR-DE-23-a/b,
	// BR-DE-24-a/b", "BR-CIUS-PT-24..63", "BR-NL-19..35". It is the machine-ish
	// half, written the way the authority writes it, ranges included, so that the
	// tests holding this table to the published Schematron can read identifiers
	// out of it and a caller can search for one.
	Rules string

	// Severity is the flag the authority put on these rules: fatal when it
	// rejects a document for breaking one, warning when it does not. It is a
	// quotation and never an estimate — not of how much the gap matters, and not
	// of what it costs a verdict. Where the authority publishes no flag because it
	// publishes no rule identifier (a format checked against its own schema), the
	// severity is this package's classification for the reason Severity documents.
	Severity Severity

	// Unevaluable reports that the authority published these rules and no
	// validator can evaluate them — the authority's own reference implementation
	// included. It is a fact about the published artefact, not about this package,
	// and it is what lets Report.Complete be reachable: a rule nobody can check is
	// not a rule this package skipped.
	//
	// It is deliberately narrow, and the narrowness is the point. It means the
	// artefact makes the rule unreachable or vacuous, demonstrably, from the
	// artefact itself: CEN binds BR-CO-05..08 to the XPath expression true(), so
	// the assertion cannot fail; CEN's ISO Schematron gives a node to the first
	// matching rule in a pattern and //ram:TypeCode precedes the rule
	// CII-DT-010/011/012 are bound to, so no processor reaches them; CEN's UBL
	// test for BR-51 is a string-length bound that a correctly masked card number
	// trips, so honouring it would mean accusing conforming invoices. Each of
	// those is checkable against a file in this repository by a reviewer who
	// disagrees, and Reason has to say which file and which construct.
	//
	// It does not mean hard, expensive, low-value, out of scope, or not yet. A
	// rule this package could implement and has not is a gap with Unevaluable
	// false, whatever the excuse — including the four CIUS whose authority
	// publishes no Schematron this repository vendors, which are unimplemented and
	// not unevaluable. TestUnevaluableFamiliesNameTheirEvidence and
	// TestOnlyEN16931HasUnevaluableFamilies keep that boundary from eroding as
	// later work adds entries, because a field that quietly widens to "we did not
	// do this one" would make Complete a lie in exactly the way the old Complete
	// field was.
	Unevaluable bool

	// Reason is why it is not evaluated, in prose. It is where a judgement gets
	// written down — that a rule is unenforceable by construction, that a
	// sub-profile is out of scope, that an authority publishes a schema rather
	// than a rule set — and for an Unevaluable family it is where the evidence
	// goes, specific enough to check against the vendored artefact without
	// re-deriving it.
	Reason string
}

// Report is the outcome of one validation: what the checker found, and whether
// it was in a position to find everything.
//
// The second half is the point. A bare []Violation can say "here is what is
// wrong" and "here is nothing", and a caller reasonably reads the second as
// "this invoice is fine" — but "nothing" is produced by four quite different
// runs: one that checked everything and found nothing; one that was cancelled;
// one that hit a resource budget; and one that ran a rule set which does not
// implement every rule its authority publishes. Complete separates the first
// from the other three, and Conformant is the predicate that gets it right.
//
// The zero Report is deliberately neither Conformant nor Complete, so a Report
// that was never filled in — a var nobody assigned, a struct decoded from JSON
// that had no such field, the value returned alongside an error a caller chose
// to ignore — cannot pass for a clean invoice. The unexported ran field is what
// holds that: see its comment, because the guard is easy to lose.
type Report struct {
	// Violations is every finding: the ways the document departs from the rules
	// that were evaluated, plus any statement by this checker about its own run
	// (RuleLimit, RuleProfile — see IsCheckerViolation) or about the file it was
	// handed (RuleRoot).
	Violations []Violation

	// NotEvaluated names the rule families the rule set that ran does not
	// implement. A validator that composes rule sets reports the union:
	// ValidateCIUSPT runs the EN 16931 core and the CIUS-PT rules, so its
	// NotEvaluated holds both sources' gaps.
	//
	// It is empty when no rule set was selected — an unknown Profile — because
	// naming the gaps of a rule set that was never chosen would say something
	// about the document, and nothing was checked. Complete is false there too,
	// through the RuleProfile finding.
	NotEvaluated []RuleFamily

	// ran records that this Report came out of a validation in this package.
	//
	// It exists for one reason, and the reason is not obvious enough to leave
	// implicit. Complete and Conformant are computed from the two fields above,
	// so a Report with no findings and no gaps answers true to both — and the
	// zero Report is exactly that. Every earlier shape of this type made the
	// zero value non-conformant (a Complete field defaulting to false did it for
	// free), and the contract that a Report nobody filled in cannot read as a
	// clean invoice is load-bearing: it is what makes it safe for a caller to
	// ignore an error return, decode a Report from a wire format, or copy one
	// out of a zero-valued struct field. newReport is the only thing that sets
	// this, so every one of those paths yields false and TestZeroReportIsNotConformant
	// and TestReportFromAnIgnoredErrorIsNotConformant pin both directions.
	ran bool
}

// Conformant reports whether this document may be treated as conforming to the
// rule set it was validated against.
//
// It is the conjunction the caller almost always means: nothing found that an
// authority rejects a document for, and nothing left unexamined that could have
// been. len(Violations) == 0 alone is the claim this package exists to stop
// people making, because it is equally true of a run that checked everything, a
// run that was cancelled, and a run whose rule set has fatal holes.
//
// Three things make it false:
//
//   - a fatal finding — the document breaks a rule its authority rejects for;
//   - a finding IsCheckerViolation recognises — the run was cut short, or never
//     chose a rule set, so the answer is "unknown" rather than "conformant";
//   - a fatal family in NotEvaluated that is evaluable — a rule that could have
//     rejected this document, that a validator could have checked, and that this
//     package did not check.
//
// Warnings do not: an advisory finding, and an advisory gap in the rule set, are
// both things a reference validator would report and no authority would reject
// an invoice for. Read them with Warnings and Complete respectively.
//
// Nor does a fatal family its authority made unevaluable, and that exception is
// narrower than it sounds. It is not "we decided this one does not count": it
// means the published artefact cannot fail — CEN binds four of these to the XPath
// expression true() — or cannot be reached, so no validator anywhere reports it
// and no gateway rejects a document over it. Not checking a rule nothing can
// check cannot put a verdict in doubt. RuleFamily.Unevaluable is where the
// boundary is documented and two tests hold the table to it.
//
// The checker's own findings are tested for by IsCheckerViolation and not merely
// by their severity, deliberately. They are fatal — see Severity — but a
// stopped run must be non-conformant because the checker did not look, which is
// a different fact from the finding's weight, and this predicate should not
// quietly start depending on a severity someone could reclassify.
//
// A consequence worth stating plainly: every rule set in this package has gaps
// today, and a rule set with a *fatal* gap makes Conformant false for every
// document it validates, however clean. One rule set no longer has one. The EN
// 16931 core's fatal half is complete — every fatal rule of the semantic model,
// of the UBL binding and of the CII binding is evaluated, bar the handful CEN's
// own reference implementation cannot report — so a clean invoice validated
// against the core alone, by ValidateEN16931 or by ValidateCIUS on a document
// declaring no CIUS, is conformant — and Complete as well, which no rule
// set in this package had ever been: everything left in Coverage(SourceEN16931) is
// a rule CEN itself cannot honour. Every other Source still names a fatal gap it
// could close and has not, so Conformant is still false for every document a CIUS
// or national validator is handed.
//
// Validate is now among them, and the change is worth stating rather than leaving
// to be discovered. It used to return the sentence above because it applied CEN's
// binding to a Factur-X document; Factur-X publishes a binding of its own and does
// not adopt CEN's, so what it was reporting Conformant on was a verdict against
// the wrong authority's rules. That binding — the per-profile data model, 2,159
// assertions across the five tiers — is evaluated now, and Validate stays here on
// the strength of one smaller entry: the 42 BR-FXEXT-* rules that restate a CEN
// identifier the EXTENDED profile drops, whose stricter CEN originals this package
// does evaluate. A caller who wants the old answer wants ValidateEN16931, which is
// what the old call actually was.
// Coverage says why for any Source, and Report.NotEvaluated says why for any
// particular run. A caller who wants the older, weaker claim writes
// len(r.Fatal()) == 0 and now has r.NotEvaluated sitting beside it, naming
// exactly what that claim omits.
func (r Report) Conformant() bool {
	if !r.ran {
		return false
	}
	for _, v := range r.Violations {
		if IsCheckerViolation(v) || v.Severity == SeverityFatal {
			return false
		}
	}
	for _, f := range r.NotEvaluated {
		if f.Severity == SeverityFatal && !f.Unevaluable {
			return false
		}
	}
	return true
}

// Complete reports whether every rule that *can* be evaluated was evaluated,
// advisory rules included. It is the stricter of the two questions, and it is
// false when either kind of gap is present:
//
//   - the rule set that ran does not implement some family its authority
//     publishes and a validator could check, which is a static fact about this
//     package and is named in NotEvaluated, whatever that family's severity; or
//   - this run stopped before it had seen everything — a cancelled context or a
//     tripped resource budget — or never chose a rule set at all, which
//     Violations reports as a finding IsCheckerViolation recognises.
//
// It was a field until severity arrived and is a method now, because with
// severity on the family the two questions genuinely differ: Conformant asks
// whether the verdict is trustworthy and passes over advisory gaps, Complete
// asks whether this package saw as much as a reference validator would. A single
// boolean could answer only one of them, and it answered the stricter one, which
// is why a residue of advisory families would have kept Conformant false forever.
//
// A family its authority made unevaluable does not make a run incomplete, and
// that clause is what makes this predicate answerable at all. Without it the
// question is "did this package evaluate every rule its authorities published",
// and for CEN the answer is permanently no — not because of anything this package
// could fix, but because CEN publishes seven rules it cannot honour itself, four
// of them bound to the XPath expression true(). A predicate whose answer no work
// can change tells a caller nothing, which is the state Conformant was in before
// severity moved onto the family. With the clause the question becomes "did this
// package see everything a reference validator could see", which is answerable,
// and as of the advisory bindings the EN 16931 core answers yes. See
// RuleFamily.Unevaluable for why that clause cannot be stretched.
//
// A document that is not an invoice at all (RuleRoot) does not make a run
// incomplete on its own account: that is a definite finding about the file, and
// Conformant is false because of the finding rather than because of any doubt.
//
// The zero Report is not Complete, for the same reason it is not Conformant and
// through the same unexported field. That mattered less while no rule set could
// reach Complete at all; now that one can, it is the only thing standing between
// a Report nobody filled in and the strongest claim this package makes.
// TestZeroReportIsNotComplete pins it directly rather than leaving it to follow
// from Conformant.
func (r Report) Complete() bool {
	if !r.ran {
		return false
	}
	for _, f := range r.NotEvaluated {
		if !f.Unevaluable {
			return false
		}
	}
	return !anyCheckerViolation(r.Violations)
}

// Fatal returns the findings whose rules their authority rejects a document for,
// plus this checker's own findings, which are fatal for the reason Severity
// gives. It returns nil when there are none.
//
// Fatal and Warnings partition Violations, so a caller that handles both handles
// everything. Neither is a substitute for Conformant: a document can have no
// fatal findings and still not be conformant, because a rule that would have
// rejected it was never evaluated.
func (r Report) Fatal() []Violation { return r.bySeverity(SeverityFatal) }

// Warnings returns the findings whose rules their authority reports without
// rejecting the document — CEN's flag="warning" and its equivalents. It returns
// nil when there are none.
//
// These are information, not a verdict. A document whose only findings are
// warnings is conformant to the rule set that ran.
func (r Report) Warnings() []Violation { return r.bySeverity(SeverityWarning) }

// bySeverity returns a fresh slice of the findings at one severity, so that
// neither accessor hands back an alias of Violations that a caller could sort or
// truncate under the Report it came from.
func (r Report) bySeverity(s Severity) []Violation {
	var out []Violation
	for _, v := range r.Violations {
		if v.Severity == s {
			out = append(out, v)
		}
	}
	return out
}

// Coverage returns the rule families that src publishes and this package does
// not evaluate, each with what its absence costs a conformance claim. It returns
// nil for a Source whose rule set is implemented in full, and for SourceChecker,
// which publishes no rules — its identifiers (RuleLimit, RuleProfile, RuleRoot)
// are this package's statements about its own run and about the file.
//
// The result is a fresh slice: the table is package state read by every
// validator, and a caller that sorted or appended to it in place would change
// what every later Report says.
//
// It parses nothing and takes no document, so it answers before a call is made
// — "is this validator good enough for what I am about to trust it with?" —
// as well as after, through Report.NotEvaluated, which is built from this same
// table.
func Coverage(src Source) []RuleFamily {
	g := notEvaluated[src]
	if len(g) == 0 {
		return nil
	}
	return append(make([]RuleFamily, 0, len(g)), g...)
}

// notEvaluated is the coverage table: for each authority, the rule families
// this package does not evaluate. It is the single source of truth — Coverage
// returns from it and newReport builds Report.NotEvaluated from it, so a
// validator cannot state a coverage claim of its own that drifts from this one.
//
// The entries are derived from what the code emits, not from what the file
// comments claim. Three tests hold them to that:
// TestCoverageNamesNoRuleThePackageEmits sweeps the corpus and fails if an entry
// claims a rule that a validator does in fact report;
// TestEN16931CoverageNamesRulesCENPublishes checks every identifier under
// SourceEN16931 against the vendored CEN Schematron so the table cannot invent
// a rule family; and TestCoverageSeveritiesMatchThePublishedFlag checks the
// severity of every entry whose authority ships a Schematron this repository
// vendors — EN 16931, XRechnung and Peppol — against the flag on the assertion
// itself, with no exceptions. Two more hold RuleFamily.Unevaluable to its
// definition: TestUnevaluableFamiliesNameTheirEvidence and
// TestOnlyEN16931HasUnevaluableFamilies.
//
// Two writing conventions, which those tests read:
//
//   - The Rules field is the claim and the Reason field is prose. Both are read
//     by both guards, so moving a claim into the prose cannot dodge either; what
//     the split buys is that the severity and the identifiers sit where a caller
//     can use them without parsing a sentence.
//
//   - An entry that carves out the implemented part of a family says "other
//     than", "emits only" or "binding only" in Rules and then lists it. Those
//     three phrases mark the *entry* as containing identifiers this package does
//     report, so the over-claim test skips both its fields — a Reason that
//     explains such a carve-out has to be able to name the evaluated half (BR-51
//     in the CII binding is the example). Any other phrasing is taken as a plain
//     claim that everything the entry names is unevaluated.
//
//     "binding only" is the third of them and the newest. It is what an entry says
//     when a rule is evaluable in one of an authority's syntax bindings and not in
//     the other, which NLCIUS is the first rule set here to have: SI-UBL's BR-NL-9
//     is reached and NLCIUS-CII's copy of it is not, so the identifier is both
//     evaluated and named as a gap, and the entry has to be able to say which is
//     which.
//
// A family whose members do not share one flag is split into one entry per
// severity rather than recorded at the stronger of the two. That is not
// cosmetic: the two fatal UBL-CR rules sat inside an entry describing 678 rules
// as "all but two advisory" (C27), where a fatal gap was accounted for in a line
// a reader would skim as advisory, and four XRechnung provisional rules were
// filed together as "provisional/advisory" when KoSIT flags two of them fatal.
// Splitting is how the severity column stays a fact rather than a rounding.
//
// A Source absent from the map is claimed complete. Only SourceChecker is,
// today, and it is absent because it publishes no rules rather than because it
// implements all of them.
var notEvaluated = map[Source][]RuleFamily{
	// EN 16931 is the rule set that most looked complete and is not. The CEN
	// unit-test suite has error fragments for 198 rules and this package catches
	// all 198 (TestEN16931ConformanceSuite), but the suite exercises the
	// semantic model and eight of the syntax bindings; it says nothing about the
	// remaining bindings, and it has no error fragment at all for thirty model
	// rules. Reading the published Schematron instead of the oracle gives the
	// list below.
	//
	// All three entries below are Unevaluable, and the reason each gives is meant
	// to be checked rather than believed. BR-CO-05..08 are bound to true();
	// CII-DT-010/011/012 sit behind an earlier matching rule in the same ISO
	// Schematron pattern; BR-51's UBL binding is a string-length bound a correctly
	// masked card number exceeds. Each names the vendored file.
	//
	// No fatal gap is left here that a validator could close, and this is the only
	// Source of which that is true. All 54 fatal UBL-SR-* rules are evaluated, and so are the two
	// fatal UBL-CR-* rules and the three fatal UBL-DT-* ones
	// (en16931_ubl_rules.go, en16931_model.go); so are all 42 fatal CII-SR-* and
	// 67 of the 70 fatal CII-DT-* rules (en16931_cii_rules.go).
	//
	// The advisory halves of both bindings are evaluated too, as of the generated
	// tables in en16931_syntax_advisory.go: 676 UBL-CR-*, 21 UBL-DT-*, 440
	// CII-SR-* and 31 CII-DT-* rules, reported at SeverityWarning. They used to be
	// four entries in this table. They are gone from it because they are checked,
	// not because they were reclassified.
	//
	// What is left is three entries covering seven rules, and every one of them is
	// a rule CEN published that no validator can evaluate. Each carries
	// Unevaluable, each names the file and the construct that makes it so, and
	// each carries the flag CEN actually published rather than the flag that would
	// keep a predicate quiet — which is the whole of D10. Six of the seven were
	// recorded here at SeverityWarning against a published flag of fatal, and the
	// contradiction was held open by a list of those six identifiers inside the
	// test that checks this column. That list is gone.
	//
	// So this is the first Source for which Report.Complete is true. Both
	// predicates now say something a caller can act on: a clean document validated
	// against this rule set is Conformant, and it is Complete, because everything
	// left is a rule a reference validator does not evaluate either. Every other
	// Source still names a fatal gap it could close and has not.
	SourceEN16931: {
		{
			Rules:       "BR-51 other than in the CII binding",
			Severity:    SeverityWarning,
			Unevaluable: true,
			Reason: "one assertion in the abstract model with two flags in the two bindings. ubl/schematron/abstract/EN16931-model.sch flags it " +
				"warning and cii/schematron/abstract/EN16931-CII-model.sch flags it fatal, and this package evaluates it in the binding that " +
				"makes it fatal (facturx_en16931.go). Unevaluable applies to the UBL half only, and the evidence is the test CEN binds it to: " +
				"EN16931-UBL-model.sch gives BR-51 the value string-length(normalize-space(.))<=10 over cbc:PrimaryAccountNumberID, and PCI DSS " +
				"permits showing the first six and last four digits of a PAN — which for a 16-digit card is a 10-character masked value plus at " +
				"least one mask character, so a correctly masked number exceeds the bound. Honouring the assertion means reporting conforming " +
				"invoices, which is the one thing this table exists to keep the package from doing",
		},
		{
			Rules:       "BR-CO-05..08 (allowance/charge reason code agrees with reason text: BT-97/98, BT-104/105, BT-139/140, BT-144/145)",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "CEN flags all four fatal in ubl/schematron/abstract/EN16931-model.sch and binds all four to the XPath expression true() — " +
				"EN16931-UBL-model.sch and EN16931-CII-model.sch both carry <param name=\"BR-CO-05\" value=\"true()\"/> and its three siblings — " +
				"so the assertion cannot fail, no conforming validator reports them, and the CEN unit-test suite ships no fragment for them. " +
				"\"The reason code and the free-text reason indicate the same type of allowance\" is a judgement about prose in an arbitrary " +
				"language, and any mechanical stand-in would accuse conforming invoices, which is presumably why CEN bound them to true()",
		},
		{
			Rules:       "CII-DT-010, CII-DT-011, CII-DT-012",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "CEN flags these three fatal and binds them to the context /rsm:CrossIndustryInvoice/rsm:ExchangedDocument/ram:TypeCode, which " +
				"in cii/schematron/abstract/EN16931-CII-syntax.sch is preceded in the same pattern by the rule whose context is //ram:TypeCode " +
				"(CII-DT-008/009). ISO Schematron gives a node to the first matching rule in a pattern only, so no processor reaches these three; " +
				"the generated EN16931-CII-validation.xslt makes the same reading mechanical, giving the wildcard template priority 1009 and the " +
				"specific one 1008. en16931_syntax_advisory.go models that ordering, so this package does not report them either, and " +
				"TestAdvisoryRulesCENCannotReportAreNotReported pins it",
		},
	},

	// Factur-X. FNFE-MPE and FeRD bind EN 16931 with a rule set of their own and
	// do not adopt CEN's CII syntax binding, which is why this Source exists at
	// all; facturx.go argues it at length and SourceFacturX states it. What the
	// five profile Schematrons publish, and what of it is here, is:
	//
	//   - CEN's BR-* core, republished verbatim. Evaluated, under SourceEN16931,
	//     because CEN minted those identifiers. Nothing to record here.
	//   - four of CEN's 583 CII-SR-*/CII-DT-* binding assertions. Three are
	//     evaluated; the fourth is the first entry below.
	//   - 51 BR-FXEXT-* rules of FNFE's own. 33 are evaluated: the nine that are
	//     new ground rather than a restatement (facturx.go) and the 24 restatements
	//     FNFE leaves unflagged, which within this artefact means fatal
	//     (facturx_restatements.go). The other 18 are the second entry.
	//   - one OpenPEPPOL rule FNFE merges in, the third entry.
	//   - a per-profile data model of 2,159 unnamed assertions, from 48 in MINIMUM
	//     to 1,241 in EXTENDED. All of them are evaluated — see
	//     facturx_datamodel.go — and the fourth entry is the three of them no
	//     processor can report.
	//
	// Nothing evaluable and fatal is left, which is the point of it: Report.Conformant
	// is true again for a clean document under a Factur-X profile, having been false
	// for every Validate call since the binding was scoped. Report.Complete is not,
	// and the entry that keeps it false is the eighteen advisory -08ini/-08rev
	// identifiers — an advisory gap, which is exactly the distinction Complete
	// exists to draw against Conformant.
	//
	// The 41 CEN identifiers the EXTENDED profile drops are not an entry here.
	// This package evaluates CEN's original for all 41 — see facturXCENOmissions,
	// which records the omission, the 23 that have a BR-FXEXT-* replacement and the
	// 18 that have none — so there is no gap to name.
	//
	// It does not always *report* all 41 at EXTENDED, and the difference is a
	// verdict rather than a coverage claim. For 21 of the 23 that have a
	// replacement, FNFE's restatement can be satisfied on a document CEN's rule
	// reports — a logistics service charge folded into BT-108, a third-party
	// charge folded into BT-115, a GROUP line CEN counts and FNFE does not, a cent
	// inside a tolerance FNFE has and CEN does not — and on the Factur-X path the
	// authority that governs the document decides. facturXAuthorityParity drops
	// the CEN finding in exactly that case and nowhere else, and the rule is
	// evaluated either way: what is dropped is a finding whose authority
	// overruled it, not a check that was skipped.
	//
	// Both identifiers still report on a document that breaks both, so a document
	// failing BR-S-08 and BR-FXEXT-S08b is reported twice, once under each
	// authority's identifier. Source separates them; a caller who wants exactly
	// FNFE's verdict filters on it. facturx_restatements.go argues the decision
	// and TestFacturXRestatementsDuplicateTheirCENOriginal pins it.
	SourceFacturX: {
		{
			Rules:       "CII-SR-464",
			Severity:    SeverityWarning,
			Unevaluable: true,
			Reason: "CEN's condition for this identifier is not(ram:PayerSpecifiedDebtorFinancialInstitution) and Factur-X rewrote it. All four " +
				"profile Schematrons that carry it — testdata/facturx/schematron/FACTUR-X_EN16931.sch and its BASIC-WL, BASIC and EXTENDED " +
				"siblings — bind it to " +
				"(A or B) or (not(A) and not(B)) over A = ram:PayeeSpecifiedCreditorFinancialInstitution and " +
				"B = ram:PayerSpecifiedDebtorFinancialInstitution, which is a tautology: the first disjunct holds unless both are absent, and then " +
				"the second does. No processor can report it from these files. The severity is CEN's published flag for the identifier, which is " +
				"warning; TestFacturXInertBindingIsStillInert re-derives the XPath and fails if FNFE fixes it",
		},
		{
			Rules:    "BR-FXEXT-{AE,E,G,IC,AF,AG,O,S,Z}-08ini and -08rev: 18 advisory identifiers",
			Severity: SeverityWarning,
			Reason: "the two readings of the VAT breakdown summation FNFE publishes side by side for the 2017 and 2026 texts of EN 16931 — " +
				"\"Without Exemption reason EN16931_2017\" and \"With Exemption reason EN16931_2026\" — and flags warning in " +
				"FACTUR-X_EXTENDED.sch, which is presumably why the -08b of each family, which is the two joined by \"or\" and is therefore the " +
				"weaker of them, carries no flag. This package evaluates the nine -08b (facturx_restatements.go), so the ground is covered by the " +
				"rule FNFE makes fatal; what is not reported is which of the two readings a document failed, which is what these eighteen say. " +
				"They are the whole of what remains evaluable under this Source",
		},
		{
			Rules:    "PEPPOL-EN16931-R008",
			Severity: SeverityWarning,
			Reason: "OpenPEPPOL's \"Document MUST not contain empty elements\", which FNFE merges into FACTUR-X_{BASIC,EN16931,EXTENDED}.sch under " +
				"the context //*[not(*) and not(normalize-space())] with flag=\"warning\" and a message reading \"(still status warning)\". " +
				"ValidatePeppol evaluates it and Validate does not, because Validate declares the rule sets that ran and OpenPEPPOL's is not one " +
				"of them; a caller who wants it has ValidatePeppol, and a Factur-X document declaring the Peppol identifier routes there through " +
				"ValidateCIUS anyway",
		},
		{
			Rules:       "three data-model assertions of the EXTENDED profile: report @listID on the context ram:ReasonCode[not (@listID)]",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "FNFE forbids @listID on a node it selects by that attribute being absent. testdata/facturx/schematron/FACTUR-X_EXTENDED.sch " +
				"carries the rule three times — under the document-level, line-level and gross-price allowance charges whose ChargeIndicator is " +
				"true — and in each the condition and the rule's own context predicate are the same test at opposite polarities, so the nodes that " +
				"would break the assertion are exactly the nodes the rule does not claim. No processor can report it. The severity is the one every " +
				"unflagged Factur-X assertion carries, for the reason facturx.go gives; fxDMContradictory derives the contradiction from the table " +
				"rather than listing it, and TestEveryFacturXDataModelAssertionFires requires these three to stay silent on the same document that " +
				"makes the other 2,156 report",
		},
	},

	// XRechnung. Every identifier KoSIT publishes in its own Schematron is now
	// evaluated: BR-DE-1..11, 14..22, 23-a/b, 24-a/b, 25-a/b, 26..28, 30, 31, the
	// seven BR-DE-CVD-*, the fifteen BR-DEX-*, BR-DE-TMP-32, BR-TMP-2, BR-TMP-3
	// and BR-TMP-CVD-01. That is 57 and not the 54 this table used to claim: the
	// harness that counted them read <assert ...> with a regular expression that
	// stops at the first '>', and three of KoSIT's assertions carry a '>' inside
	// an attribute value — BR-DE-19 and BR-DE-20 in their IBAN arithmetic
	// ("if($cp > 64)") and BR-DEX-02 in "count(cac:SubInvoiceLine) > 0". They were
	// invisible to every check that read the flags, which is also how BR-DEX-02
	// came to be filed as fatal when KoSIT flags it warning.
	//
	// What was left was not KoSIT's own rules but the ones it imports, and those
	// are evaluated now: the 21 Peppol BIS Billing rules src/xsl/rule-list.xml
	// whitelists and src/xsl/peppol-into-xr.xsl merges into the released artefact,
	// in KoSIT's wording of them and gated on that file. So SourceXRechnung is
	// absent from this table, which is the claim that its rule set — all 78
	// identifiers of the Schematron a German buyer validates against — is evaluated
	// in full. completeSources in report_test.go is where that claim is registered.
	//
	// The imported findings carry SourcePeppol, because Source names the authority
	// that wrote the rule and OpenPEPPOL wrote these. They are nonetheless
	// XRechnung's coverage and not Peppol's, and the distinction is not a dodge: the
	// Sources a validator hands newReport name *the rule sets that ran*, and the
	// rule set that ran is XRechnung's, which happens to include 21 rules of
	// OpenPEPPOL's authorship. ValidateXRechnung therefore does not declare
	// SourcePeppol, and a caller reading an XRechnung Report's NotEvaluated is told
	// what the XRechnung Schematron leaves unchecked rather than what OpenPEPPOL
	// publishes and KoSIT never imported — which is most of Peppol's rule set,
	// including every PEPPOL-COMMON-* identifier and all 101 country-specific rules.
	// Both tables are empty now, so the two coverage answers coincide; the
	// distinction is still the one that decides which Sources a validator declares.

	// Peppol BIS Billing 3.0. All 59 PEPPOL-COMMON-* and PEPPOL-EN16931-*
	// identifiers the vendored Schematron publishes are evaluated — 58 in the UBL
	// binding, 44 in the CII one, each in the binding that publishes it.
	//
	// The count used to read "60", and both halves of that number were wrong. It
	// counted PEPPOL-COMMON-R048, which is inside an XML comment in both binding
	// files and is therefore a rule OpenPEPPOL withdrew rather than a gap; and it
	// counted PEPPOL-EN16931-R045, which no binding publishes at all — the entry
	// wrote the family as "R040..R046" and the artefact has no R045 in the
	// PEPPOL-EN16931 series. The severity test could not catch either, because it
	// skips an identifier the artefact does not carry.
	//
	// The other rule set in the same two files is evaluated too. PEPPOL-EN16931-UBL.sch
	// and -CII.sch each carry, after the PEPPOL-* patterns and under a comment
	// reading "National rules", the country-specific rules OpenPEPPOL publishes for
	// eight member states — 101 identifiers, of which the UBL binding holds all 101
	// and the CII binding 41, for 142 (identifier, binding) pairs. No survey of this
	// rule set in this repository had counted them, this table included, because
	// every one of them matched on the prefix "PEPPOL-" and stopped.
	//
	// They belong here rather than behind an option, and the artefacts say so three
	// times over: neither binding file declares a <phase>, so ISO Schematron
	// activates every pattern; buildconfig.xml's base configurations are the whole
	// file with no phase attribute; and each rule is gated inside itself on the
	// supplier's country, and the domestic ones on the customer's too, so evaluating
	// them unconditionally accuses nobody. peppol_country_rules.go sets out the five
	// different spellings of that gate, which are not interchangeable.
	//
	// They are not the NLCIUS or CIUS-BE rule sets under their own Sources: NL-R-*
	// here is OpenPEPPOL's Dutch rule set, distinct from the BR-NL-* of
	// SourceNLCIUS, and DE-R-* is OpenPEPPOL's German set, distinct from KoSIT's
	// BR-DE-*. And KoSIT imports none of the 101, so an XRechnung validation does not
	// acquire them — which matters most for the German family, since OpenPEPPOL's
	// DE-R-NNN is a re-publication of KoSIT's BR-DE-NNN and reporting both would name
	// one defect twice.
	//
	// SourcePeppol is therefore absent from this table, which is the claim that its
	// rule set — all 160 identifiers of the two files, in the bindings that publish
	// them — is evaluated in full. completeSources in report_test.go is where that
	// claim is registered, and TestEveryPublishedPeppolRuleHasBothVerdicts is what
	// holds it up: every one of the 244 published (identifier, binding) pairs has a
	// document that trips it and one that does not.

	// NLCIUS is finished in both halves and in both bindings — 34 BR-NL identifiers
	// in UBL, 33 in CII, plus the eight BR-GA-* of the G-account extension and the
	// empty-element rule each binding ends with — and it was the last rule set in
	// this package whose only gap was advisory. Its "not recommended" tier is
	// evaluated now, as warnings, so a Dutch invoice reports Complete() as well as
	// Conformant().
	//
	// The gap that entry used to describe read "BR-NL-19..35", which is a range and
	// not a set: SI-UBL publishes no BR-NL-22, -23 or -34 at all, and its BR-NL-27,
	// -28 and -32 exist only as numbered sub-rules (BR-NL-27-1 … -4).
	//
	// What is left is four identifiers neither authority's own validator can report,
	// all for the same reason and all derived from the artefacts rather than argued:
	// a rule whose context an earlier rule of the same pattern has already claimed is
	// never reached, because ISO Schematron processes a node with the first matching
	// rule and no other. TestNLCIUSUnevaluableRulesAreDerivedFromTheArtefacts
	// re-derives all four.
	//
	// BR-NL-9 is the consequential one and it is per binding rather than per rule
	// set: it is evaluated for a UBL document, where SI-UBL binds it in the same
	// Schematron rule as BR-NL-7 and BR-NL-8, and it is unevaluable for a CII one,
	// where the NLCIUS-CII file gives it a rule of its own against a context BR-NL-7's
	// rule already holds. This package reported it for CII until that was read.
	//
	// The one entry here that was not Unevaluable is gone, and it was the newest:
	// SI-UBL-2 in the UBL binding and empty-element-check in the CII one, one rule
	// published under two names. PR 28 found it with the enumeration guard it built
	// — the same guard that surfaced the eight BR-GA-* rules of the G-account
	// extension — and named it rather than evaluating it, because it is the only
	// rule in either binding that carries no gate and evaluating it therefore
	// changes what ValidateNLCIUS says about a document that is not an NLCIUS
	// invoice.
	//
	// It is evaluated now, ungated, which is what SimplerInvoicing's own validator
	// does; nlciusEmptyElements is where that decision and the rule-order arithmetic
	// it needs are argued. Both identifiers are emitted, each in the binding that
	// publishes it, at the flag="warning" both artefacts carry — so it cannot move
	// Conformant(), and Complete() is true for a Dutch invoice again. That is not a
	// return to the earlier claim: the earlier claim rested on a survey that had
	// never seen this rule, and this one rests on the rule being evaluated.
	SourceNLCIUS: {
		{
			Rules:       "BR-NL-9, in the CII binding only",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "NLCIUS-CII-validation.sch gives it a rule of its own with the context " +
				"`/*/rsm:ExchangedDocument/ram:TypeCode[$si]`, which the immediately preceding rule of the same " +
				"pattern already claims, so no CII invoice type code ever reaches it. The UBL binding puts the " +
				"same assertion in a rule that is reached, and this package evaluates it there",
		},
		{
			Rules:       "BR-NL-31, in the CII binding only",
			Severity:    SeverityWarning,
			Unevaluable: true,
			Reason: "NLCIUS-CII-validation.sch gives it a rule of its own with the context " +
				"`ram:SpecifiedTradeSettlementPaymentMeans[$s]`, which the immediately preceding rule of the " +
				"same pattern already claims, so no CII payment means group ever reaches it. The UBL binding " +
				"puts the same assertion in a rule that is reached, and this package evaluates it there",
		},
		{
			Rules:       "BR-NL-32-2, BR-NL-32-3, in the UBL binding only",
			Severity:    SeverityWarning,
			Unevaluable: true,
			Reason: "si-ubl-2.0-nlcius.sch binds them to the contexts " +
				"`cac:InvoiceLine/cac:AllowanceCharge/cbc:AllowanceChargeReasonCode` and " +
				"`cac:CreditNoteLine/cac:AllowanceCharge/cbc:AllowanceChargeReasonCode`, and an earlier rule of " +
				"the same pattern matches every node either of them does: an XSLT match pattern is anchored at " +
				"its last step, so `cac:AllowanceCharge/cbc:AllowanceChargeReasonCode` claims the line-level " +
				"reason codes as well as the document-level ones. The identifier bound to that earlier rule is " +
				"what this package reports for a reason code at either level",
		},
	},

	// CIUS-PT has no entry: it is the second rule set in this package, after
	// NLCIUS's fatal half and Peppol's, whose published inventory is evaluated in
	// full — all 65 BR-CIUS-PT-* identifiers (the family is 65 and not the 66 its
	// numbering suggests, because AT/eSPap publishes no BR-CIUS-PT-31), all 8 of
	// AT's own BR-AA-*, and all 290 DT-CIUS-PT-* identifiers over the 291 assertions
	// that carry them. See completeSources in report_test.go, which is where that
	// claim is registered.
	//
	// The DT-CIUS-PT-* family is four fifths of the Portuguese rule set by count and
	// no entry in this table named it until PR 22 vendored the Schematron. It is
	// generated from that Schematron rather than transcribed — see
	// cius_pt_datatype.go — and cius_pt_datatype_test.go holds the generated tables
	// to the artefact in both directions.

	// CIUS-RO's fatal half is finished: all 121 identifiers ANAF publishes in
	// release 1.0.9 are accounted for — 115 evaluated (the 25 BR-RO-NNN business
	// rules by hand in cius_ro.go, and 90 length, decimal, date-format and
	// occurrence rules generated into cius_ro_rules_table.go) and the six below,
	// which ANAF published and no conforming Schematron processor can report.
	//
	// Every entry here is Unevaluable, so none of them holds Conformant down; each
	// records a defect in somebody else's artefact rather than a gap in this
	// package, and TestCIUSROUnevaluableAssertsAreDerivedFromTheArtefact re-derives
	// all six from the Schematron so the claims cannot rot.
	//
	// Not listed, deliberately: BR-RO-020, BR-RO-A999, BR-RO-L0301 and BR-RO-L0309,
	// which releases 1.0.3 and 1.0.4 publish and which ANAF withdrew. A withdrawn
	// rule is not a coverage gap — the same reading that removed two phantom Peppol
	// entries in PR 20 — and TestCIUSROVersionsDiffer names each one's successor.
	SourceCIUSRO: {
		{
			Rules:       "BR-DEC-RO-13, BR-DEC-RO-15",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "cius-ro/RO16931-rules.sch binds their rule to the context `/ubl:Invoice | cac:CreditNote`, " +
				"and both branches are dead: " +
				"the invoice is claimed by the earlier rule `/ubl:Invoice | /cn:CreditNote` and ISO " +
				"Schematron gives a node to the first matching rule only, and cac:CreditNote is a name " +
				"UBL does not define in the CommonAggregateComponents namespace",
		},
		{
			Rules:       "BR-DEC-RO-23",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "in cius-ro/RO16931-rules.sch its rule repeats the context " +
				"`cac:InvoiceLine | cac:CreditNoteLine` of an earlier rule in the same pattern, so no " +
				"invoice line ever reaches it",
		},
		{
			Rules:       "BR-RO-L1019",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "in cius-ro/RO16931-rules.sch its rule's context " +
				"`/ubl:Invoice/cac:TaxTotal/cac:TaxSubtotal` selects a subset of the earlier rule's " +
				"`cac:TaxTotal/cac:TaxSubtotal`, which claims every VAT breakdown first",
		},
		{
			Rules:       "BR-RO-A051, BR-RO-A052",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "cius-ro/RO16931-rules.sch binds both to `count(.) <= 50`, and count(.) counts the " +
				"context node, which is one " +
				"node: the assertion cannot be false. ANAF wrote a document-wide occurrence limit as a " +
				"per-occurrence one",
		},
	},

	// UBL.BE's fatal half is finished: all 15 identifiers the ubl-model-BE pattern
	// of GLOBALUBL.BE.sch publishes are accounted for — 14 evaluated in cius_be.go
	// and the one below, which the authority published and no conforming Schematron
	// processor can report.
	//
	// The four that closed the gap were ubl-BE-01 and -04 (the two on the
	// AdditionalDocumentReference group, which the file comment never mentioned
	// until PR 22 read the artefact) and ubl-BE-06 and -12, the two bilingual
	// free-text lists. The entry that named those last two had called them "not
	// enforced", which described the rule rather than gave a reason: both are
	// exact-match membership tests over sixteen and eighteen quoted sentences, and
	// TestUBLBECodeListsQuoteTheArtefact re-derives all four of this rule set's
	// tokenize() lists from the file.
	SourceUBLBE: {
		{
			// The one unevaluable family outside CEN's and CIUS-RO's, and the reason
			// it is here rather than as a gap: it is not a rule this package has not
			// got to, it is a rule the authority cannot report either.
			Rules:       "ubl-BE-13",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason:      ublBE13Reason,
		},
	},

	// SRBDT's fatal half is finished: all 46 identifiers the Ministry of Finance
	// publishes are accounted for — 31 evaluated in cius_rs.go (21 RSR business
	// rules, the 3 RSE extension rules, and the 7 assertions of the abstract pdvcat
	// pattern, which the validation schema instantiates four times) and the 15
	// below, which the Ministry published and no conforming Schematron processor can
	// report.
	//
	// The two families that had no entry until the Schematron was vendored are
	// evaluated rather than named now: RSK-X-* is the Serbian zero-rate VAT-category
	// tier and RSE-* the srbdtext extension tier.
	//
	// Every entry here is Unevaluable, so none of them holds Conformant down. All
	// fifteen have the same cause and it is not a judgement:
	// EN16931-UBL-srbdt.sch is one pattern, eleven of its rules repeat the context
	// `/ubl:Invoice | /cn:CreditNote` and four more repeat three other contexts, and
	// ISO Schematron gives a node to the first matching rule in a pattern and to no
	// other. TestSRBDTUnevaluableRulesAreDerivedFromTheArtefact re-derives the whole
	// list from the file, so a Ministry that splits its pattern turns these back into
	// gaps on the day the artefact is re-fetched.
	//
	// Eight of them — RSR-09, 10, 13, 16, 17, 20, 22 and 25 — were being *emitted*
	// until this was read, so recording them here is a false-positive fix and not a
	// coverage reduction: they are findings the Ministry's own validator cannot
	// produce. It is the same reading that put ubl-BE-13 in Coverage(SourceUBLBE).
	SourceSRBDT: {
		{
			Rules:       "RSR-08, RSR-09, RSR-10, RSR-13, RSR-16, RSR-17, RSR-20, RSR-22, RSR-25, RSR-26, RSR-33",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "EN16931-UBL-srbdt.sch binds all eleven, in the single pattern UBL-srbdt, to the context " +
				"`/ubl:Invoice | /cn:CreditNote`, which an earlier rule of that pattern has already claimed. " +
				"ISO Schematron processes a node with the first rule in a pattern whose context matches it and " +
				"with no other, so the document element never reaches any of them",
		},
		{
			Rules:       "RSR-15",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "in EN16931-UBL-srbdt.sch its rule repeats the context " +
				"`cac:AccountingSupplierParty/cac:Party/cbc:EndpointID` of the immediately preceding rule of the " +
				"same pattern, so no Seller electronic address ever reaches it",
		},
		{
			Rules:       "RSR-24",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "in EN16931-UBL-srbdt.sch its rule repeats the context " +
				"`cac:AccountingCustomerParty/cac:Party/cbc:EndpointID` of the immediately preceding rule of the " +
				"same pattern, so no Buyer electronic address ever reaches it",
		},
		{
			Rules:       "RSR-31, RSR-32",
			Severity:    SeverityFatal,
			Unevaluable: true,
			Reason: "in EN16931-UBL-srbdt.sch both rules repeat the context " +
				"`/ubl:Invoice/cac:TaxRepresentativeParty | /cn:CreditNote/cac:TaxRepresentativeParty` of an " +
				"earlier rule of the same pattern, so no tax representative ever reaches either",
		},
	},

	// The national formats below publish an XSD (and, for OIOUBL, a Schematron)
	// rather than a business-rule set this package could quote, and each
	// validator checks the mandatory structure and code lists rather than the
	// whole schema. The identifiers under these Sources were minted here, so
	// there is no published family to name: the entry says what the authority
	// checks that this package does not.
	//
	// Every one of these gaps is fatal, and that is the least interesting
	// severity in the table rather than a default nobody thought about. What is
	// unevaluated here is a schema: a document that violates it is rejected at
	// the border by the authority's own gateway, which is the definition of fatal
	// this table uses. None of these authorities publishes an advisory tier for
	// its schema, so there is no warning half to split off.
	SourceFatturaPA: {{
		Rules:    "the SdI FatturaPA XSD and the SdI's consistency checks",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure and Italian code lists the FPA-* rules cover",
	}},
	SourceFacturae: {{
		Rules:    "the Facturae XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure and Spanish code lists the FE-* rules cover",
	}},
	SourceEbInterface: {{
		Rules:    "the ebInterface XSD (schema versions 3.x..6.x)",
		Severity: SeverityFatal,
		Reason:   "beyond the version-independent mandatory structure the EB-* rules cover",
	}},
	SourceKSeF: {{
		Rules:    "the KSeF FA XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure and Polish code lists the KS-* rules cover",
	}},
	SourceFinvoice: {{
		Rules:    "the Finvoice XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the FI-* rules cover",
	}},
	SourceTEAPPS: {{
		Rules:    "the TEAPPSXML XSD",
		Severity: SeverityFatal,
		Reason:   "beyond each invoice's type and customer information the TP-* rules cover",
	}},
	SourceOIOUBL: {{
		Rules:    "the OIOUBL Schematron",
		Severity: SeverityFatal,
		Reason:   "beyond the profile, core document terms, electronic addresses and seller name the OIO-* rules cover",
	}},
	SourceSvefaktura: {{
		Rules:    "the SFTI Svefaktura 1.0 (UBL 1.0) XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the SV-* rules cover",
	}},
	SourceZATCA: {{
		Rules:    "the ZATCA XSD and the Fatoora platform's reporting/clearance checks",
		Severity: SeverityFatal,
		Reason: "including the cryptographic stamp, the QR payload and the invoice hash chain — beyond the mandatory structure the ZA-* rules " +
			"cover",
	}},
	SourceOSA: {{
		Rules:    "the NAV Online Számla XSD",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the HU-* rules cover",
	}},
	SourceUBLTR: {{
		Rules:    "the UBL-TR XSD, and the non-invoice UBL-TR document types (despatch advice, response)",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure the TR-* rules cover; the other document types this validator does not accept at all",
	}},
	SourcePINT: {{
		Rules:    "the PINT core rule set and every jurisdiction rule set (AE, AUNZ, EU, JP, MY, OM, SG)",
		Severity: SeverityFatal,
		Reason:   "beyond the mandatory structure every jurisdiction shares that the PINT-* rules cover",
	}},
	SourceOrderX: {{
		Rules:    "the Order-X document rules",
		Severity: SeverityFatal,
		Reason:   "beyond the five mandatory head terms this package checks",
	}},
}

// newReport assembles one validator's answer: the findings it gathered, and the
// coverage of the rule sets it ran.
//
// sources is the authorities whose rules were applied, in the order they were
// applied, and it is what makes the coverage claim follow the call rather than
// the entry point: ValidateCIUSPT passes SourceEN16931 and SourceCIUSPT because
// it runs both, and ValidateCIUS passes whatever the arbitration routed the
// document to — a pair for a CIUS layered on the core, one Source for a national
// format with a rule set of its own. Passing none says no rule set was chosen —
// the unknown-Profile
// case — and yields an empty NotEvaluated with Complete still false, because
// the RuleProfile finding is a checker violation.
// It is also the only thing that sets Report.ran, which is what keeps a Report
// nobody produced from reading as a clean invoice. Every Report a caller can
// obtain from this package comes from here or is the zero value, and the zero
// value answers false to both questions.
func newReport(vs []Violation, sources ...Source) Report {
	return Report{Violations: vs, NotEvaluated: coverageUnion(sources), ran: true}
}

// coverageUnion concatenates the coverage gaps of sources, in order, dropping
// the repeats a composed rule set would otherwise produce. Families are deduped
// on the whole value, so two authorities that happened to word one gap alike
// would still both be reported unless they agree on its severity and reason too.
func coverageUnion(sources []Source) []RuleFamily {
	var out []RuleFamily
	seen := map[RuleFamily]bool{}
	for _, src := range sources {
		for _, g := range notEvaluated[src] {
			if seen[g] {
				continue
			}
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
}

// anyCheckerViolation reports whether any finding is this checker speaking about
// its own run rather than about the document — the stopped-run half of Complete.
func anyCheckerViolation(vs []Violation) bool {
	for _, v := range vs {
		if IsCheckerViolation(v) {
			return true
		}
	}
	return false
}
