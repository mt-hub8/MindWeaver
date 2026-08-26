package formalis

// Per-CIUS condition overrides: the mechanism by which a document validated under a
// national CIUS is judged by that authority's reading of a CEN rule and not by
// CEN's.
//
// # The problem this exists for
//
// A CIUS narrows EN 16931. Most of them do it by adding rules of their own, which
// this package evaluates alongside CEN's and reports under the authority's Source.
// But five of the seven do something else as well: instead of *referencing* CEN's
// Schematron they ship a **copy** of it, and a copy can be edited. Where an
// authority has edited one, its own validator evaluates the edited condition — so a
// package that evaluates CEN's condition and reports it under a CEN identifier says
// something about the document that the authority does not.
//
// That is the audit's C40, found in PR 23, confirmed in PR 26 and deliberately left
// alone until the shape below was decided.
//
// # Telling a national reading apart from a stale vendored copy
//
// The hard half is not applying an override, it is knowing which differences are
// overrides. A copy that differs from CEN's *current* file differs for one of two
// reasons, and they call for opposite treatment:
//
//   - the authority edited it. That is a national reading and this package should
//     honour it under that CIUS.
//   - CEN changed the file after the copy was taken. That is a lag, not a rule.
//     Honouring it would mean reporting BR-51 with a four-to-six-character
//     card-number test because a directory has not been refreshed since 2018, and
//     it would silently change what a CEN identifier means for every caller — the
//     objection on which PR 23 and PR 26 declined to act at all.
//
// testdata/cius-condition-overrides/gen.py separates them by asking CEN's own git
// history whether CEN ever published what a copy carries, at any commit, for that
// identifier. A condition CEN published is CEN's whenever it was written; one CEN
// never published is the authority's. That is a decision procedure and not a
// judgement, which matters here more than usual: the differences are overwhelmingly
// of a kind that looks cosmetic, and the audit's own reading of them was wrong.
// `exists(cbc:ID)` for BR-02 — recorded in C40 as AT/eSPap being more permissive
// than CEN, and as this package's one live false positive under CIUS-PT — is CEN's
// own text, published on 2017-03-14 and changed later. So are the dropped
// `cac:TaxScheme/cbc:ID='VAT'` predicates, BR-48's missing `or category='O'` escape,
// BR-CO-19's missing DescriptionCode alternative and UBL-SR-27 counting
// cbc:InstructionNote. None of them is Portuguese.
//
// What survives that filter is small and specific. See ciusCENCopyVerdicts for the
// per-authority counts and cius_overrides_test.go for the checks that re-derive
// them.
//
// # What an override is, mechanically
//
// The authority's condition is *evaluated*, not translated. Each override set
// carries the authority's own Schematron pattern — every <rule> of it, in the
// authority's order, with its context — and the overridden assertions with the
// authority's XPath, polarity and message verbatim. It is run by the same evaluator
// the generated CIUS-PT datatype and CIUS-RO tiers use (cius_pt_datatype_eval.go).
//
// Every rule of the pattern is present even when it carries no overridden
// assertion, because under ISO Schematron a node goes to the first rule whose
// context matches it: a rule dropped for carrying nothing would hand its nodes to a
// rule below it and change which nodes the override sees. Rule order has decided a
// rule's meaning four times in this repository already (PR 14, PR 23, PR 25, PR 26).
//
// Because the authority's condition is evaluated directly, the question "is this
// difference cosmetic" never arises at the point of evaluation. It arises once, in
// the generator, and is answered against CEN's history rather than by eye.
//
// # Where an override applies, and where it does not
//
// Only on the path that asked for that CIUS. ValidateCIUSPT applies AT/eSPap's
// reading; Validate, ValidatePeppol, ValidateXRechnung and everything else keep
// CEN's, and so does ValidateCIUS when it routes to a validator with no override
// set. A caller who asked for EN 16931 gets EN 16931.
//
// And only in the syntax the authority publishes for. CIUS-PT ships a UBL binding
// and no CII one, so a Factur-X invoice through ValidateCIUSPT is judged by CEN's
// conditions throughout — the same gate PR 22 put on the BR-CIUS-PT-* rules
// themselves, for the same reason (C36).
//
// # The other half: what a copy leaves out
//
// A copy can differ from CEN by carrying a different condition, which is what the
// overrides above are for, and it can differ by not carrying the identifier at all.
// The second is the larger difference and it has the same two causes, so it gets the
// same treatment: ciusCENCopyOmissions records, per authority, the CEN release the
// copy was taken from and which CEN identifiers it lacks, split into ones CEN had
// published by then and ones CEN has added since.
//
// Nothing is suppressed on the strength of it, and the reason is worth stating here
// rather than only in the commit that decided it. Suppressing a rule an authority
// dropped is right only when the authority put something in its place that covers the
// same ground; CIUS-PT is the only one of the six that drops a CEN rule at all, and
// what it put in place of CEN's arithmetic identities carries a ±1.00 € acceptance
// range, which is a relaxation rather than a replacement. Suppressing on that basis
// would convert a divergence from one authority's validator into a class of invoice
// nothing checks, and a false negative is worse than a false positive because nothing
// reports it. The table is therefore a record and not a switch — cius_omissions_test.go
// re-derives it, measures what it is worth across the corpus, and holds the decision
// with a guard that goes red if a suppression ever lands without the argument.
//
// # How a caller can tell
//
// Violation.Reading. An overridden finding keeps SourceEN16931 and CEN's identifier,
// because the identifier is CEN's and re-stamping it would make one identifier mean
// two things — the collision TestNoRuleIdentifierIsClaimedByTwoSources exists to
// prevent, and the reason PR 23 rejected the obvious fix. Reading names the
// authority whose condition was evaluated, is SourceNone on every finding judged by
// its own Source, and is rendered by Violation.Error, so the distinction reaches a
// caller who only logs the finding as well as one who reads the field.

// ciusOverrides is one authority's set of re-conditioned CEN rules.
//
// rules is the identifiers overridden and the severity to report them at, which is
// the authority's own flag rather than CEN's — an authority that re-wrote the
// condition may have re-flagged it too, and the flag is read from the same file as
// the condition.
//
// patterns is the authority's Schematron, and compiled is the same thing parsed once
// at load. syntax is the document syntax the authority publishes the binding for;
// an override never applies to the other one.
type ciusOverrides struct {
	authority Source
	syntax    string
	rules     map[string]Severity
	patterns  []ptDTPattern

	compiled  []*ptDTCompiledPattern
	rootNames map[string]bool
}

// ciusCENCopyVerdict is what one authority's copy of CEN's Schematron turned out to
// be, derived from the copy and from CEN's history rather than asserted.
//
// ships is false for an authority that publishes no copy at all. same, stale and own
// partition the CEN identifiers a copy does carry: identical to CEN's current file;
// different from it but published by CEN at some release; and carrying an axis —
// context, polarity or test — that CEN never published. own maps each such
// identifier to the axes that are the authority's.
//
// applied says whether this package evaluates that authority's conditions. It is
// true for an authority with nothing of its own, and notApplied carries the reason
// when it is false. An authority whose overrides are known and not applied is a
// stated gap with its identifiers named; the alternative — leaving them out of the
// table — is how a rule set comes to have a hole nobody can see (C27, C33, C39).
type ciusCENCopyVerdict struct {
	authority  string
	source     Source
	ships      bool
	shared     int
	same       int
	stale      int
	applied    bool
	notApplied string
	own        map[string]string
}

// ciusCENCopyOmission is the absence half of the same question, for one authority:
// which CEN identifiers its published rule set does not carry, and whether CEN had
// published them when the copy was taken.
//
// # Why absence needs the same discriminator as a differing condition
//
// A CIUS whose copy of CEN's Schematron lacks an identifier lacks it for one of two
// reasons, and PR 27 established that telling them apart by eye gets it wrong:
//
//   - CEN had not published the identifier yet. The copy is old, and the identifier
//     is one CEN has added since. Nothing about it is national, and a CIUS is by
//     construction a restriction of EN 16931, so evaluating it under that CIUS is
//     evaluating the standard the CIUS narrows.
//   - CEN had published it and the authority left it out. That is a decision, of the
//     same kind as re-writing a condition, and it is the only half that could
//     justify not reporting the identifier under that CIUS.
//
// release is the CEN release the copy was taken from, derived from the copy's own
// content rather than from a version string: the release that publishes every
// identifier the copy carries and whose assertions the copy reproduces most closely.
// releaseThrough is set when CEN republished those files unchanged across several
// releases and the evidence therefore pins a run of them rather than one.
//
// classified is false for a copy this derivation cannot speak about, with
// notClassified saying why. carried is the CEN identifiers the copy holds and
// differing how many of them depart from that release, which is what makes the pin
// readable: a copy that differs from its own release in 26 assertions of 774 is a
// copy, and one that differed in half of them would not be.
type ciusCENCopyOmission struct {
	authority      string
	source         Source
	classified     bool
	notClassified  string
	release        string
	releaseThrough string
	releaseDate    string
	carried        int
	differing      int
	overlay        bool
	files          []ciusCENFileOmission
}

// ciusCENFileOmission is one of CEN's Schematron files as one authority treats it.
//
// copied is whether this repository holds the authority's copy of that file, so its
// contents can be compared. fetched separates the two ways copied can be false:
// the authority's master Schematron does not <include> a copy of this CEN file at
// all, so its rule set does not run it (fetched true, dropped and postdates
// populated over the whole file); or it does include one and this repository does
// not vendor it, so nothing is claimed either way (fetched false, both lists empty).
//
// Keeping those apart is the point of the field. CIUS-RO's master and both NLCIUS
// masters <include> CEN's code-list file; CIUS-PT's includes no code-list file of
// any name. Collapsing the two would either invent 22 Romanian omissions or hide 19
// Portuguese ones.
type ciusCENFileOmission struct {
	cenFile   string
	copied    bool
	fetched   bool
	dropped   []string
	postdates []string
}

// mustCompileOverrides parses an override set's patterns once, at load.
//
// A parse failure panics, for the reason ptDTMustCompile gives: the table is
// generated and committed, so a row this evaluator cannot read is a defect in the
// build and not anything a caller did, and the alternative — dropping the row — is
// an override that silently stops being applied while the finding it was meant to
// replace goes on being reported.
func mustCompileOverrides(o *ciusOverrides) *ciusOverrides {
	o.rootNames = map[string]bool{}
	for i := range o.patterns {
		c := ptDTMustCompile(o.patterns[i])
		o.compiled = append(o.compiled, c)
		for n := range c.rootNames {
			o.rootNames[n] = true
		}
	}
	return o
}

// ptOverrides is AT/eSPap's reading of the nine CEN identifiers CIUS-PT 2.1.1
// re-conditions. See ciusCENCopyVerdicts for how the nine were arrived at.
var ptOverrides = mustCompileOverrides(ptConditionOverrides)

// applyConditionOverrides replaces this package's verdict on the identifiers o's
// authority re-conditioned with that authority's own verdict on the same document.
//
// It is two halves and both are necessary. Dropping the package's findings without
// evaluating the authority's would turn a national reading into a silent exemption;
// evaluating the authority's without dropping the package's would report the
// identifier twice, once each way.
//
// The findings it emits keep SourceEN16931 — the identifier is CEN's — and carry
// Reading, so the substitution is visible rather than silent. They are appended
// after the core findings that survived rather than in the position the replaced
// ones held: an overridden rule fires at the nodes the *authority's* context claims,
// which is not in general the order this package's own rules ran in, and inventing a
// position for it would be a fact about nothing.
func applyConditionOverrides(r *run, o *ciusOverrides, p *parsed, out []Violation) []Violation {
	if o == nil || p.inv == nil || p.inv.syntax != o.syntax {
		return out
	}
	kept := make([]Violation, 0, len(out))
	for _, v := range out {
		if v.Source == SourceEN16931 {
			if _, ok := o.rules[v.Rule]; ok {
				continue
			}
		}
		kept = append(kept, v)
	}
	doc := &ptDTDoc{root: p.root, want: o.rootNames}
	add := func(rule, msg string) {
		kept = append(kept, Violation{
			Source:   SourceEN16931,
			Rule:     rule,
			Severity: o.rules[rule],
			Message:  msg,
			Reading:  o.authority,
		})
	}
	for _, c := range o.compiled {
		ptDTRun(c, r, p.root, doc, add)
	}
	return kept
}
