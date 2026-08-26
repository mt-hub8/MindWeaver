package formalis

import (
	"context"
	"fmt"
)

// Cancellation and resource limits.
//
// This package validates invoice XML that arrives from outside — a Factur-X
// attachment pulled out of a PDF, a file posted to an endpoint — so the cost of
// a call is set by the document, not by the caller. Two separate mechanisms
// bound that cost, and the distinction is the same one pdf0 draws in
// docs/limits.md: a *limit* says how much this document may cost, a *context*
// says how long this operation may take.
//
// # Why both, and why the limits came first
//
// The limits are not a fallback for the context; they answer a question the
// context cannot. Measuring validation time against invoice size turned up
// three defects that no amount of cancellation would have fixed:
//
//   - parseCII accumulated an element's character data with `text += string(t)`,
//     one reallocation per token. 6.4 MB of well-formed XML took 33 s and grew
//     quadratically; the same shape at this repository's practical 100 MB
//     ceiling would have run for hours. Fixed in parseCII by appending to a
//     []byte (there is no budget for it — it is simply linear now).
//   - validateVATTaxableSums re-scanned and re-parsed every invoice line for
//     every VAT breakdown. 7.3 MB took 1.7 s, also quadratic. Fixed by parsing
//     each operand once and memoising per distinct (category, rate), with
//     maxVATSumWork as the backstop for input that defeats the memoisation.
//   - The tree walks (findAll, collectAttr, mapCII) recurse once per level, and
//     a deeply nested document overflowed the goroutine stack at around 90 MB.
//     A stack overflow is a *fatal* runtime error: it is not a panic, so no
//     recover() anywhere up the stack — including the one pdf0 wraps
//     ValidateFacturX in — can turn it back into a finding. maxDepth stops the
//     parse before the tree can get that deep, which protects every walk at one
//     point rather than each of them separately.
//
// A caller with no deadline at all still pays those costs, which is what makes
// them limits rather than latency.
//
// # What the budgets bound, and what they deliberately do not
//
// maxDepth bounds how deep a document nests and maxNodes bounds how many
// elements it has. Neither bounds the character data *inside* an element, and
// nothing else does either: a document of two elements whose inner one holds
// 40 MB of text parses cleanly, spending 2 of the 1,000,000-element budget. That
// is a deliberate gap rather than an oversight, and the measurement is why.
//
// Cost is stated as a multiple of the input, because the input is the one thing
// a caller can bound before the call. Measured with runtime.MemStats around
// parseCII, at 10 MB and again at 40 MB, the multiple is the same at both sizes
// — it is a constant, not a growth rate:
//
//	shape of the character data          allocated   retained
//	one contiguous run                      5.2x       1.00x
//	one run split into 4-byte tokens        6.7x       0.36x
//	many 4 KB text nodes                    2.1x       1.02x
//	900 nested elements, all open at once   2.1x       1.06x
//
// Retained never exceeds about 1x, whatever the shape, because every string in
// the finished tree is a copy of a stretch of the input and closeNode releases
// each accumulator as its element ends. Allocated is higher because the run is
// buffered on the way in, appended to a second time, and copied once at the end,
// and because append grows by doubling.
//
// Most of that is not this package's to bound. scanShape reads the same 40 MB
// document building no tree and capturing nothing, and still allocates 3.2x it:
// that is encoding/xml's own buffer, which materialises a whole contiguous
// character-data run before parseCII is ever handed the token. parseCII adds
// 2.0x on top of that — the textBuf and the string() copy — and no budget here
// can reach the rest.
//
// Set that against the element cost maxNodes exists for. The same measurement
// over a document of `<a/>` siblings gives 83x allocated and 26x retained, 105
// bytes per element. Character data is between twelve and forty times cheaper
// per input byte than markup, and — this is the part that decides it — it cannot
// exceed the input, while the element count can multiply it. At the practical
// 100 MB ceiling cited above, all-text is roughly 110 MB retained and under
// 700 MB allocated across the parse; all-markup was the multi-gigabyte case
// maxNodes was written to stop.
//
// So there is no text budget, for three reasons rather than only the size:
//
//   - A cap could not recover most of the cost. The decoder has already
//     materialised the run before parseCII sees the token, so bounding textBuf
//     removes the 2.0x this package adds and leaves the 3.2x it does not own.
//   - It would have to stop the parse rather than truncate, because every other
//     guard here stops rather than hand the rule engine a partial document — and
//     stopping means refusing a document outright.
//   - The number could not be justified the way maxDepth and maxNodes are, which
//     is by sitting a hundredfold above the largest real document. A text cap has
//     no such headroom, because one legitimate business term genuinely does carry
//     megabytes in a single element: BT-125, the attached document, embedded as
//     base64. Across the 1613 documents in testdata the largest text node is
//     3,285,640 bytes of EmbeddedDocumentBinaryObject — and it is 99.8% of the
//     largest document in the corpus, a Romanian B2G invoice that must validate.
//     A cap tight enough to be worth having would refuse it.
//
// TestElementTextIsBoundedOnlyByTheInput pins the table above, so it is checked
// rather than asserted, and TestBigTextDoesNotSpendTheNodeBudget pins the
// property that motivates it.
//
// # What a stopped run reports
//
// A cancelled run, and a run that trips a budget, have the same problem: the
// checker stopped before it had seen everything, so "no violations" would be a
// lie. Both are therefore reported as a Violation under the reserved rule
// identifier RuleLimit, and IsCheckerViolation tells those apart from a real
// non-conformance.
//
// This is deliberately the same convention pdf0 uses for its own guards and for
// cancellation — the same rule name, the same meaning, the same predicate shape
// — so that a caller draining ValidateFacturX's mixed slice of container and
// invoice findings has one rule to look for and not two.
//
// The property that matters: a stopped run never returns an empty Violations
// slice. A caller testing len(r.Violations) == 0 for "valid" gets "not valid";
// a caller filtering with IsCheckerViolation gets "unknown". Neither gets a
// clean bill of health from a run that did not look.
//
// This is also why a stopped run is not the error return. Every exported
// validator can answer with an error, for a document it could not read, and it
// would have been tidier to fold "and one it did not finish reading" into the
// same channel. It would also have broken the property above: an error carries no
// findings, so the checks that *did* complete — which are true, just incomplete —
// would be discarded, and pdf0, which drains its own container guards and this
// package's findings from one mixed slice looking for one rule name, would have
// to learn a second mechanism for the same event.
//
// Report.Complete is where this generalises. A stopped run is one of two ways a
// validator can fail to have seen everything — the other is a rule set that
// does not implement every rule its authority publishes — and Complete is false
// for both, so Report.Conformant is the one predicate that is safe to key on.
// The RuleLimit finding stays exactly as it is: Complete reads it, rather than
// replacing it, so a caller that already routes on RuleLimit keeps working and
// pdf0's own container guards keep the same meaning inside a mixed slice.
//
// This is also why a parse failure has to be told apart from a stopped parse.
// Returning ErrMalformedXML when the run was merely cancelled would be the same
// lie in the other direction — refusing a document the checker never finished
// reading. readFailure draws that line, and every exported validator goes through
// it, by way of one of the two harnesses (modelValidate and treeValidator.validate)
// rather than by twenty-two validators each remembering to.
//
// # Why the Is* predicates take no context
//
// The exported detection predicates (IsZATCA, IsFinvoice, ...) report whether
// they could read the document, but take no context. There is nothing to
// cancel: they do not build a tree, so there is no budget for them to trip and
// no long-running phase for a deadline to interrupt. scanShape reads the
// document once and retains only the open elements — bounded by maxDepth — and
// the text of the handful of elements it captures, so its cost is a single
// linear pass whose memory is set by the nesting and not by the element count.
//
// It is not independent of the document's size, and the section above is why:
// a CustomizationID holding 40 MB of text is retained in full, at the same 1x
// and 5.2x the tree pays for the same bytes. What the scan removes is the
// per-element amplification, which is the term that made detection the cheapest
// way to reach a gigabyte; capturing less is not on offer, because the
// predicates match substrings of those strings and a truncated capture would let
// the scan and the tree route one document two ways. See detect.go.
//
// Their error therefore reports what the document is — malformed XML, an
// encoding this package does not implement, nesting past the cap — and never
// "the checker gave up", which is the distinction RuleLimit exists to carry on
// the validation side.

// RuleLimit is the rule identifier carried by a Violation that reports the
// checker stopping early — a cancelled context or a tripped resource budget —
// rather than a defect in the invoice. Such a finding carries SourceChecker,
// because it is a statement by this package and not by any rule authority.
//
// It matches the identifier pdf0 uses for the same event, so a caller that
// already separates "the file is bad" from "the checker could not finish" needs
// only one name for the second.
const RuleLimit = "limit"

// RuleRoot is the rule identifier for a document this package read and that is
// not an EN 16931 invoice in either syntax — a root element that is neither a
// CrossIndustryInvoice nor a UBL Invoice or CreditNote. Unlike RuleLimit this
// *is* a statement about the document, but it is still this checker's statement
// rather than a rule authority's, so it too carries SourceChecker.
//
// It replaces the RuleSyntax constant, which meant two things: "this file is not
// well-formed XML" and "this file is not an invoice document at all". The first
// is now an error — see ErrMalformedXML — because there is no document to make a
// finding about, and the second stays a finding because there is. Narrowing
// RuleSyntax to the surviving half would have been the quieter change and the
// worse one: a caller filtering on it to catch malformed files would still
// compile and would never match one again. Being made to look at the call site is
// the point, and "root" is what the surviving meaning is actually about — the
// same word the thirteen tree-reading validators already use for it (FPA-root,
// ZA-root, ORDER-root, …), whose findings are unchanged.
//
// It is not an IsCheckerViolation, for the reason RuleSyntax was not: "you handed
// me a Facturae and I check EN 16931 invoices" is a definite answer, not a
// confession that the checker did not look.
const RuleRoot = "root"

// RuleProfile is the rule identifier carried by a Violation that reports the
// caller naming a Profile this package does not implement. Like RuleLimit and
// RuleRoot it carries SourceChecker, because it is a statement by this
// checker; unlike either it is a statement about the *request*, not about the
// document, which is innocent and was not examined.
//
// It exists because the alternatives are all worse. Returning an error, as this
// package does for a document it could not read, would say the input was
// unreadable when the input was never looked at and the caller's argument was the
// problem. Reporting it as RuleRoot would accuse a document that may be perfectly
// conformant of being the wrong kind of document. Reporting it as
// RuleLimit would overload an identifier documented as a resource-budget or
// cancellation event and shared verbatim with pdf0, so a caller that routes
// "the checker ran out of room, retry it smaller" would retry forever on input
// no retry can fix. And returning no findings at all would be the one outcome
// this package refuses everywhere else: a caller testing len(v) == 0 for
// "valid" would get a clean bill of health from a run that never chose a rule
// set. So a run that rejects the profile validates nothing and returns exactly
// this one finding — never mixed with document findings, because there are
// none to mix it with.
//
// It is a reserved word, like RuleLimit, rather than an identifier in anyone's
// numbering scheme, and IsCheckerViolation recognises it: "I did not judge this
// document" is exactly what that predicate exists to keep separate from
// "conformant".
const RuleProfile = "profile"

// IsCheckerViolation reports whether v describes the checker not having judged
// the document, rather than a way in which the invoice departs from the rules.
//
// A cancelled context, a tripped resource budget (both RuleLimit) and a Profile
// this package does not implement (RuleProfile) all produce one. Treat it as
// "unknown", never as "conformant" and never as "non-conformant".
//
// RuleRoot is deliberately *not* one of them: "this document is not an EN 16931
// invoice" is a finding about the document, and a definite one. Neither is the
// error a document this package cannot read produces, which does not arrive as a
// finding at all.
//
// It tests Rule alone, deliberately, even though every finding this package
// emits now carries a Source and the pair a caller should think in is
// (SourceChecker, RuleLimit). Two reasons. Both identifiers are reserved words
// rather than identifiers in anyone's numbering scheme — no rule authority
// mints a rule called "limit" or "profile" — so there is nothing for the scope
// to disambiguate here. And RuleLimit is shared with pdf0, which constructs
// that same finding for its own container guards and hands it back in one mixed
// slice; requiring SourceChecker would silently reclassify every one of those as
// a business-rule violation the moment this package added the field. A caller
// that wants the strict pair can still write it — the Source is there — but the
// predicate that exists to keep "unknown" from being read as "conformant" must
// not start returning false for a finding it has always covered.
//
// Widening it to RuleProfile is safe in the direction that mattered there:
// pdf0 never emits RuleProfile, so no finding that exists today changes
// classification. What would not be safe is the reverse — leaving it out, so
// that a caller filtering with this predicate to count document defects counted
// its own bad argument as one.
func IsCheckerViolation(v Violation) bool {
	return v.Rule == RuleLimit || v.Rule == RuleProfile
}

// maxDepth is the deepest element nesting the parser will build.
//
// Real invoices in every syntax this package reads nest around a dozen levels;
// the deepest document in the oracle suites is far below this. The cap exists
// for the fatal-stack-overflow case described above, so it is set generously
// enough that no genuine invoice can reach it and low enough that the recursive
// walks stay far from the 1 GB goroutine stack limit.
const maxDepth = 1000

// maxNodes is the largest number of elements the parser will build a tree from.
//
// maxDepth bounds how *deep* a document nests; it says nothing about how *many*
// elements it has, and the two failure shapes are unrelated. A document that is
// millions of shallow siblings has a depth of 2, so maxDepth never engages,
// while every one of those siblings becomes a ciiNode — a name string, a
// children slice, a text string and an accumulator. That is about 105 bytes
// retained per element, and around 165 bytes of peak RSS, for an element
// written as four (`<a/>`): a 60 MB document of them reached 2.5 GB, and the
// 100 MB this package can be handed projected to roughly 4.2 GB. Like the stack
// overflow maxDepth exists for, an OOM kill is a process death rather than a
// finding the caller can report, which is what makes a bound necessary rather
// than merely tidy.
//
// A context does not substitute for it. parseCII polls cancellation every
// cancelParseTokens tokens, so a deadline does stop the parse — but only after
// whatever has already been allocated, so a short deadline still admits
// gigabytes.
//
// The number is set the way maxDepth was: measure real documents and leave a
// wide margin. Across the 1613 documents in testdata the largest is 8300
// elements, and that is a UN/ECE code list rather than an invoice; the largest
// actual invoice is 1803. The largest XML pdf0 has found embedded in a PDF is
// 1.3 MB. A million elements is over a hundred times the largest document here,
// and it holds the whole call flat: validating 15, 30 and 60 MB of siblings now
// peaks at 167, 203 and 236 MB rather than 610 MB, 1.24 GB and 2.56 GB, and
// reports one RuleLimit finding instead of eighteen invented business-rule
// violations.
//
// The budget is a property of the *document*, not of the entry point the caller
// reached for. That has to be arranged rather than assumed: run.nodes is spent
// one element at a time by parseCII, so a call that read the same bytes twice
// would halve the ceiling. Every exported validator therefore parses exactly
// once, at its own boundary, and threads the parsed artefacts down — see parsed
// in facturx_en16931.go — rather than the raw bytes. Before that, ValidateCIUS
// (which reads BT-24 to choose a validator) reached UBL.BE through three parses
// and refused, as too large, documents that ValidateUBLBE validated and
// Validate validated: a third of the number below. Nothing about the document
// decided that; only how many layers the call passed through did.
// TestNodeBudgetIsPerDocumentNotPerEntryPoint pins the property.
const maxNodes = 1_000_000

// maxVATSumWork bounds the (breakdown x operand) pairs validateVATTaxableSums
// may examine across one invoice.
//
// After memoisation a real invoice uses a few thousand: the pair count is
// (distinct VAT category/rate combinations) x (lines + allowances/charges), and
// an invoice has single-digit combinations. The budget is three orders of
// magnitude above that, so it is reached only by a document built to reach it.
const maxVATSumWork = 10_000_000

// canceler is one call's cancellation signal.
//
// The zero value never cancels, which is what lets internal helpers take one
// unconditionally instead of branching around a missing context.
//
// ctx.Done() is hoisted into a field at construction because the parser's token
// loop tests it: Context.Done and Context.Err both take the context's mutex on a
// cancellable context, while a receive-with-default on an already-obtained
// channel does not. A context that can never be cancelled (context.Background)
// has a nil Done channel, and a receive on a nil channel blocks, so the default
// case is taken — the zero value and a background context behave alike, for
// free.
type canceler struct {
	ctx  context.Context
	done <-chan struct{}
}

func newCanceler(ctx context.Context) canceler {
	if ctx == nil {
		return canceler{}
	}
	return canceler{ctx: ctx, done: ctx.Done()}
}

// stopped reports whether the call should stop now.
func (c canceler) stopped() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c canceler) err() error {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}

// run is the per-call state of one validation: the caller's cancellation signal
// and the resource budgets. It is threaded explicitly rather than stored
// anywhere, because its lifetime is exactly one call.
type run struct {
	cancel canceler
	// vatWork is the remaining validateVATTaxableSums pair budget.
	vatWork int
	// nodes is the remaining element budget for the tree parseCII builds. One
	// call reads its document once, so this is the document's element count and
	// not a per-parse allowance; see maxNodes.
	nodes int
	// trips accumulates the RuleLimit findings this run has to report. A run
	// records at most one trip per distinct cause, since repeating "the checker
	// stopped" tells the caller nothing new.
	trips []Violation
	seen  map[string]bool
}

func newRun(ctx context.Context) *run {
	return &run{cancel: newCanceler(ctx), vatWork: maxVATSumWork, nodes: maxNodes}
}

// note records a limit trip once per guard.
func (r *run) note(guard, msg string) {
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if r.seen[guard] {
		return
	}
	r.seen[guard] = true
	r.trips = append(r.trips, Violation{
		Source:   SourceChecker,
		Rule:     RuleLimit,
		Severity: SeverityFatal,
		Message:  fmt.Sprintf("%s (%s); the checks that had not yet run were skipped, so this invoice is neither confirmed valid nor invalid", msg, guard),
	})
}

// stopped reports whether the caller's context has ended, recording the trip the
// first time it has. Every loop that can run long consults it.
func (r *run) stopped() bool {
	if r == nil || !r.cancel.stopped() {
		return false
	}
	err := r.cancel.err()
	if err == nil {
		err = context.Canceled
	}
	r.note("context-canceled", "the run was cancelled before it finished: "+err.Error())
	return true
}

// spendNode draws one element from the tree budget, reporting whether the parse
// may proceed.
func (r *run) spendNode() bool {
	if r == nil {
		return true
	}
	if r.nodes <= 0 {
		r.note("xml-node-count", fmt.Sprintf("the invoice XML has more than %d elements", maxNodes))
		return false
	}
	r.nodes--
	return true
}

// spendVAT draws n pairs from the VAT summation budget, reporting whether the
// work may proceed.
func (r *run) spendVAT(n int) bool {
	if r == nil {
		return true
	}
	if r.vatWork <= 0 {
		r.note("vat-sum-work", "the VAT breakdown taxable-amount checks (BR-*-08) exceeded their work budget and were not completed")
		return false
	}
	r.vatWork -= n
	return true
}

// finish appends this run's limit trips to the findings it gathered.
//
// It is the single place a stopped run is turned into output, so no exported
// validator can forget it — and because the trips are appended rather than
// replacing the findings, the checks that did complete are still reported. They
// are true, just incomplete.
func (r *run) finish(out []Violation) []Violation {
	if r == nil || len(r.trips) == 0 {
		return out
	}
	return append(out, r.trips...)
}
