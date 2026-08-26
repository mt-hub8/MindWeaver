package formalis

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Format detection, and why it does not build a tree.
//
// The twelve Is* predicates answer "which format is this", which every one of
// them decides from the root element name plus a handful of things immediately
// under it — the presence of a distinguishing child, the text of a
// CustomizationID or ProfileID, or (ZATCA alone) an AdditionalDocumentReference
// carrying a particular ID.
//
// They used to get that by calling parseCII and reading two fields off the
// result, which meant materialising every element in the document to look at a
// name. That made them the cheapest way to reach the memory amplification
// maxNodes now bounds: a 60 MB document cost about 2.4 GB to answer a question
// about its first tag.
//
// docShape is what the predicates actually consult, and scanShape fills it in a
// single pass that retains only the open elements — bounded by maxDepth — and
// the few strings below. Nothing accumulates per element, so the cost is set by
// the nesting rather than by the element count, and detection no longer needs a
// node budget at all: there is no tree to bound.
//
// "Rather than by the element count" is the exact claim, and the qualifier
// matters. The scan is not free of the document's *size*: the strings below are
// short in every document anyone has written, but nothing makes them short, and
// a CustomizationID carrying 40 MB of character data is accumulated and retained
// in full — measured at 1.0x the input retained and 5.2x allocated, the same
// figures parseCII pays for the same bytes, because most of both is
// encoding/xml's own buffering of a contiguous run. limits.go argues why that is
// acceptable unbounded for the tree; it is acceptable here for the same reason
// and one more: text cannot exceed the input, whereas the per-element cost the
// scan exists to remove multiplies it by twenty-six.
//
// Capping the capture was considered and rejected. The predicates match
// substrings of these strings (IsOIOUBL and IsPINT both use strings.Contains),
// so a truncated capture could answer differently from the tree that
// TestScanMatchesTreeDetection holds it identical to — which would mean one
// document routed two ways, the single failure this scan's parity contract
// exists to make impossible. TestScanRetainsOnlyWhatItCaptures pins what is and
// is not retained.
//
// Note what does *not* work: parsing into a tree but capping it at depth 2. In
// the document that motivates this, the millions of siblings *are* the root's
// direct children, so a depth-capped tree retains all of them. The scan has to
// inspect and discard, which is what it does.
//
// The scan reads to end of input rather than stopping once it has what it
// needs. Detection has to be able to say "I could not read this" — that is what
// the error return of every predicate is for — and a document whose first tag
// is fine but whose remainder is truncated or malformed is exactly such a case.
// Stopping early would report it as readable, which is the collapse the error
// return exists to prevent.
//
// One capture is not a direct child of the root: the CII Specification
// identifier (BT-24) sits three elements down, under
// ExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter, and Detect
// needs it to answer "which CIUS is this" for the CII syntax the way the
// CustomizationID answers it for UBL. The scan follows that path with two flags
// on frames it was going to open anyway and keeps the one string at the end of
// it, so the property above is unchanged: what is retained is set by the nesting
// and by a fixed list of short strings, never by the element count.
//
// Everything below the scan reads that one shape. The twelve Is* predicates ask
// it one question each; Detect asks it all of them in a documented order and
// returns a single routing answer.

// docShape is the part of a document the Is* predicates and Detect examine.
type docShape struct {
	// root is the local name of the root element.
	root string
	// The distinguishing children the predicates test for by presence.
	hasNaglowek    bool
	hasBiller      bool
	hasSellerParty bool
	// The text of the first direct child of the root with each name, matching
	// what root.str(name) returned from the tree.
	customizationID string
	profileID       string
	gotCustomID     bool
	gotProfileID    bool
	// icvDocRef records an AdditionalDocumentReference, at any depth, whose
	// first direct ID child reads "ICV" — what zatcaDocRef looked for.
	icvDocRef bool
	// ciiSpecID is the text of the CII Specification identifier (BT-24):
	// ExchangedDocumentContext / GuidelineSpecifiedDocumentContextParameter /
	// ID, which mapCII reads with exactly that path from the root.
	//
	// It is the one thing the scan captures that is not a direct child of the
	// root, and it is why the scan follows a path rather than only watching
	// depth 1. It is still one string: the frames it walks through are frames
	// the scan was going to open anyway, and nothing below the ID is retained.
	ciiSpecID string
	// gotContext marks the root's first ExchangedDocumentContext, so a second
	// one is ignored the way child() ignored it.
	gotContext bool
}

// The elements whose text the scan keeps. Anything else is discarded as it is
// read.
const (
	captureNone = iota
	captureCustomizationID
	captureProfileID
	captureDocRefID
	captureCIISpecID
)

// The steps of the CII specification-identifier path, as a frame's position
// along root / ExchangedDocumentContext / GuidelineSpecifiedDocumentContextParameter
// / ID. specStepNone is every other element.
const (
	specStepNone = iota
	specStepContext
	specStepParameter
)

// scanFrame is one open element. It holds the name, because a child's meaning
// depends on its parent, and a text accumulator only for the elements listed
// above — so a document of millions of siblings allocates nothing per sibling.
type scanFrame struct {
	name string
	// capture says which docShape field this element's text feeds, or
	// captureNone.
	capture int
	buf     []byte
	// specStep says where this element sits on the CII specification-identifier
	// path, or specStepNone.
	specStep int
	// gotChild marks a frame whose first interesting child has already been
	// taken, so a second one is ignored the way child() ignored it. Two frames
	// watch for one: an AdditionalDocumentReference for its first ID, and a step
	// of the specification-identifier path for its first step down. No element
	// is both, because each is decided by the frame's own name.
	gotChild bool
}

// scanShape reads xmlData once and reports what the Is* predicates need to know
// about it, or an error if it could not be read.
//
// It mirrors parseCII's acceptance exactly — the same decoder, the same charset
// reader, the same depth cap, the same treatment of an empty document — so the
// predicates changed only in what they cost, not in which documents they
// accept. An element's text is the character data directly inside it, excluding
// its descendants', which is what ciiNode.text held.
func scanShape(xmlData []byte) (*docShape, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	// The same trap the tree parser uses, for the same reason and with the same
	// result: a caller can tell "this file is corrupt" from "this producer emits
	// an encoding we do not read" with errors.Is, and gets the same answer whether
	// it asked a predicate or a validator.
	var trap charsetTrap
	dec.CharsetReader = trap.reader

	var d docShape
	var stack []scanFrame
	seenRoot := false

	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, trap.classify(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) >= maxDepth {
				// Neither sentinel: a document this deep may be perfectly
				// well-formed and in an encoding this package reads. It is refused
				// for what it would cost the tree, which is why the validators
				// answer the same document with a RuleLimit finding instead. See
				// ErrMalformedXML.
				return nil, fmt.Errorf("the XML nests deeper than %d elements", maxDepth)
			}
			name := t.Name.Local
			f := scanFrame{name: name}

			switch {
			case len(stack) == 0:
				// Go's decoder accepts more than one top-level element, and
				// parseCII overwrites its root each time it sees one, so the
				// tree the validators check is built from the *last*. Detection
				// has to land on the same document or a predicate would route
				// to a validator that then reports a different root — so
				// everything gathered from an earlier top-level element is
				// discarded here, exactly as the tree discarded it.
				d = docShape{root: name}
				seenRoot = true
			case len(stack) == 1:
				// A direct child of the root.
				switch name {
				case "Naglowek":
					d.hasNaglowek = true
				case "Biller":
					d.hasBiller = true
				case "SellerParty":
					d.hasSellerParty = true
				case "CustomizationID":
					if !d.gotCustomID {
						f.capture = captureCustomizationID
					}
				case "ProfileID":
					if !d.gotProfileID {
						f.capture = captureProfileID
					}
				case "ExchangedDocumentContext":
					if !d.gotContext {
						d.gotContext = true
						f.specStep = specStepContext
					}
				}
			}
			if len(stack) > 0 {
				p := &stack[len(stack)-1]
				switch {
				// zatcaDocRef searched the whole tree, so this is not restricted
				// to a particular depth.
				case name == "ID" && p.name == "AdditionalDocumentReference" && !p.gotChild:
					p.gotChild = true
					f.capture = captureDocRefID
				// The next two steps of the CII specification-identifier path.
				// child() takes the first match at every step and gives up if it
				// is missing, so a second parameter group — or an ID under a
				// later one — is not consulted even when the first carried none.
				case name == "GuidelineSpecifiedDocumentContextParameter" && p.specStep == specStepContext && !p.gotChild:
					p.gotChild = true
					f.specStep = specStepParameter
				case name == "ID" && p.specStep == specStepParameter && !p.gotChild:
					p.gotChild = true
					f.capture = captureCIISpecID
				}
			}
			stack = append(stack, f)

		case xml.EndElement:
			if len(stack) > 0 {
				d.close(&stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			if len(stack) > 0 {
				if f := &stack[len(stack)-1]; f.capture != captureNone {
					f.buf = append(f.buf, t...)
				}
			}
		}
	}
	// Elements left open at end of input never saw their EndElement. parseCII
	// materialised their text too.
	for i := range stack {
		d.close(&stack[i])
	}
	if !seenRoot {
		return nil, fmt.Errorf("%w: no root element", ErrMalformedXML)
	}
	return &d, nil
}

// close stores a finished element's captured text.
func (d *docShape) close(f *scanFrame) {
	if f.capture == captureNone {
		return
	}
	text := string(f.buf)
	f.buf = nil
	switch f.capture {
	case captureCustomizationID:
		d.customizationID = text
		d.gotCustomID = true
	case captureProfileID:
		d.profileID = text
		d.gotProfileID = true
	case captureDocRefID:
		if strings.TrimSpace(text) == "ICV" {
			d.icvDocRef = true
		}
	case captureCIISpecID:
		d.ciiSpecID = text
	}
	f.capture = captureNone
}

// str reports the trimmed text of the named direct child of the root, matching
// what root.str(name) returned.
func (d *docShape) str(name string) string {
	switch name {
	case "CustomizationID":
		return strings.TrimSpace(d.customizationID)
	case "ProfileID":
		return strings.TrimSpace(d.profileID)
	}
	return ""
}

// specID reports the document's Specification identifier (BT-24), read from
// whichever place the syntax it is written in puts it.
//
// The two syntaxes are the two parseEN16931 dispatches on, and the accessor is
// the mapper's: mapUBL reads root.str("CustomizationID") for an Invoice or
// CreditNote root, mapCII reads the ExchangedDocumentContext path for a
// CrossIndustryInvoice root. Any other root carries no BT-24 that this package
// could quote — parseEN16931 refuses it before a mapper runs — so this reports
// "" rather than guessing from an element that happens to share a name.
//
// The result is therefore the same string ValidateCIUS routes on, which is what
// makes it usable for the question that motivated it: a caller can now ask which
// CIUS a document declares without validating it, and get the answer the
// dispatcher would have got.
func (d *docShape) specID() string {
	switch d.root {
	case "Invoice", "CreditNote":
		return d.str("CustomizationID")
	case "CrossIndustryInvoice":
		return strings.TrimSpace(d.ciiSpecID)
	}
	return ""
}

// detectShape reads xmlData far enough to identify what kind of document it is.
//
// It exists so the Is* predicates share one answer to "could I read this at
// all". They report three outcomes between them — yes, definitively not, and
// could not tell — and the third is what the error carries: XML that is not
// well-formed, an encoding this package does not implement, or a document that
// nests past the cap. Folding any of those into a plain false would tell a
// caller that a truncated Facturae invoice is not a Facturae invoice, which is
// not the same statement and misroutes anything dispatching on the answer.
func detectShape(xmlData []byte) (*docShape, error) {
	d, err := scanShape(xmlData)
	if err != nil {
		return nil, fmt.Errorf("the XML could not be read: %w", err)
	}
	return d, nil
}

// Ordered detection lives below, next to the scan it reads. The precedence it
// applies is documented on Detect itself, because it is part of that function's
// contract: a caller routing on the answer has to know why one format won.

// Detection is what Detect concluded about one document.
//
// The zero Detection is a "recognised nothing" answer — Source is SourceNone,
// Recognised is false and Validator is nil — so a Detection that was never
// filled in cannot pass for a format.
type Detection struct {
	// Source is the arbitrated answer: the authority whose format this document
	// is in, and therefore whose rules apply to it. It is SourceNone when the
	// document was read and matched no format this package validates.
	//
	// It is Source and not a Format type of its own because the two would name
	// the same twenty-one things. Source already means "the authority that
	// defines a rule", and for every value Detect can return that authority and
	// the format are one — FatturaPA is a format and a rule set, XRechnung is a
	// CIUS and a rule set, and a UBL invoice declaring no national profile is
	// judged by CEN. Reusing it buys two things a parallel taxonomy would not:
	// Coverage(det.Source) answers "what would that validator not check?"
	// *before* the call is made, and the Source on every Violation the call
	// returns is comparable with the Source the routing was done on. The values
	// Detect never returns are SourceChecker, which is this package speaking
	// about its own run rather than a format, and SourceNone.
	Source Source

	// CIUS is the Core Invoice Usage Specification the Specification identifier
	// declares — DetectCIUS(SpecID) and nothing else. It is CIUSNone when the
	// identifier names none, and for every root that carries no BT-24 at all.
	//
	// It is not always the same answer as Source, and where the two differ
	// Source is the one to route on: CIUS answers only "which CIUS", so it is
	// CIUSNone for a document Source recognises by a national format that is not
	// a CIUS (an OIOUBL or UBL-TR identifier), by evidence outside BT-24 (a
	// ZATCA profile identifier, an ebInterface Biller), or by its root element.
	// The two no longer disagree about which rule set applies, which they did
	// while a PINT invoice reported CIUSPeppol and SourcePINT.
	CIUS CIUS

	// SpecID is the Specification identifier (BT-24) as the document wrote it,
	// with nothing removed but the surrounding whitespace: the
	// cbc:CustomizationID of a UBL Invoice or CreditNote, or the
	// ExchangedDocumentContext/GuidelineSpecifiedDocumentContextParameter/ID of
	// a CrossIndustryInvoice. It is "" for any other root, and for a document
	// that omits the term.
	//
	// This is the string DetectCIUS was always documented to take and that
	// nothing exported could produce. It is kept verbatim so a caller can log
	// it, meter it, or match it against a profile this package does not know
	// about.
	SpecID string

	// Root is the local name of the document's root element, namespace
	// discarded — the fact most of the detection rests on, kept so a caller can
	// tell an Invoice from a CreditNote and see why the answer came out as it
	// did.
	Root string
}

// Recognised reports whether Detect matched a format at all. It is false for
// the third of Detect's three answers — the document was read, and is no format
// this package validates.
func (d Detection) Recognised() bool { return d.Source != SourceNone }

// Validator returns the exported entry point that validates a document of this
// format, or nil when Detect recognised nothing.
//
// It exists so that routing on a Detection needs no table of the caller's own,
// which is the other half of owning the precedence: an order nobody can act on
// without re-deriving the format-to-validator map has only moved the problem.
//
//	det, err := formalis.Detect(data)
//	if err != nil { ... }          // could not read it
//	v := det.Validator()
//	if v == nil { ... }            // read it, recognised nothing
//	report, err := v(ctx, data)
//
// The validator's own error is for the same three inputs Detect's is — malformed
// XML, an encoding this package does not implement — so a caller that got a
// Detection has usually already ruled it out. It is returned anyway rather than
// dropped, because the two passes read the bytes independently and nothing
// guarantees the caller passed the same ones.
//
// Two of the mappings are worth stating. SourceEN16931 maps to ValidateCIUS
// rather than to Validate: Detect has already established that the document
// declares no CIUS, which is the branch on which ValidateCIUS runs the EN 16931
// core, and it avoids inventing a Profile. A caller who knows the Factur-X
// data-richness profile from a PDF container's XMP should call Validate with it
// instead, since a leaner profile is excused rules the core applies — Detect
// reads the invoice, and the invoice does not carry that metadata. Each CIUS
// maps to its own validator rather than to ValidateCIUS, which is the direct
// route to the same rule set: ValidateCIUS now applies this very arbitration, so
// either call runs what Detect named.
//
// The returned function re-reads xmlData. Detection is a separate pass by
// design — it builds no tree and spends no budget, which is what lets it run on
// input the validator may go on to refuse.
func (d Detection) Validator() func(context.Context, []byte) (Report, error) {
	switch d.Source {
	case SourceEN16931, SourceFacturX:
		// Both map to ValidateCIUS, and for the same reason: that entry point
		// applies this very arbitration, so it reaches the rule set Detect named.
		// Factur-X is the one Source whose rule set needs a second fact — which
		// data-richness tier — and ValidateCIUS reads it out of the same BT-24
		// this detection came from, so nothing has to be invented here. A caller
		// who knows the tier from a PDF container's XMP and does not trust the
		// invoice to agree calls Validate with it; DetectCIUS's own comment on
		// which of the two to believe applies unchanged.
		return ValidateCIUS
	case SourceXRechnung:
		return ValidateXRechnung
	case SourcePeppol:
		return ValidatePeppol
	case SourceNLCIUS:
		return ValidateNLCIUS
	case SourceCIUSPT:
		return ValidateCIUSPT
	case SourceCIUSRO:
		return ValidateCIUSRO
	case SourceUBLBE:
		return ValidateUBLBE
	case SourceSRBDT:
		return ValidateSRBDT
	case SourceFatturaPA:
		return ValidateFatturaPA
	case SourceFacturae:
		return ValidateFacturae
	case SourceEbInterface:
		return ValidateEbInterface
	case SourceKSeF:
		return ValidateKSeF
	case SourceFinvoice:
		return ValidateFinvoice
	case SourceTEAPPS:
		return ValidateTEAPPS
	case SourceOIOUBL:
		return ValidateOIOUBL
	case SourceSvefaktura:
		return ValidateSvefaktura
	case SourceZATCA:
		return ValidateZATCA
	case SourceOSA:
		return ValidateOSA
	case SourceUBLTR:
		return ValidateTurkishInvoice
	case SourcePINT:
		return ValidatePINT
	case SourceOrderX:
		return ValidateOrderXML
	}
	return nil
}

// Detect reports which of the formats this package validates a document is in,
// and which CIUS it declares, in one streaming pass that builds no tree.
//
// It is the routing entry point, and the order it applies is part of its
// contract rather than an implementation detail: a caller that routes on the
// answer needs to know why one format won.
//
// # The three answers
//
// None of them is folded into another:
//
//   - a non-nil error means the document could not be read — malformed XML, an
//     encoding this package does not implement, or nesting past the cap — and
//     the Detection is the zero value and means nothing;
//   - a Detection whose Recognised is false means the document was read and is
//     no format this package validates;
//   - otherwise Source names the format, Validator returns the entry point that
//     checks it, and Coverage(Source) says in advance what that entry point
//     will not look at.
//
// Detect also answers the CIUS question on its own account: Detection.SpecID is
// the Specification identifier (BT-24) that DetectCIUS takes and that nothing
// exported could otherwise extract from XML, and Detection.CIUS is DetectCIUS
// applied to it.
//
// # Why an ordered answer is needed
//
// The twelve Is* predicates are independent tests, not a partition. Six of them
// key on a root element named Invoice or CreditNote — a name four national
// formats and every EN 16931 UBL document share — and disambiguate on a child
// that no other format forbids, so more than one of them says true about the
// same bytes:
//
//	<Invoice><Biller/><SellerParty/></Invoice>                     IsEbInterface, IsSvefaktura
//	<Invoice><CustomizationID>TR-OIOUBL-2.02</CustomizationID>…     IsOIOUBL, IsTurkishInvoice
//	<Invoice><CustomizationID>urn:peppol:pint:x</CustomizationID>
//	        <ProfileID>reporting:1.0</ProfileID></Invoice>          IsZATCA, IsPINT
//
// Each of those answers is individually correct — IsOIOUBL means "the
// specification identifier says OIOUBL", not "this is OIOUBL and nothing else"
// — and each is worth being able to ask on its own. What was missing is the
// arbitration. A caller routing a mailbox has to pick an order, the package
// documented none, and so every caller picked a different one and got a
// different answer to the same question. Detect is that order, written down
// once, tested, and shipped as part of the API rather than left in a README
// example that reads like a partition.
//
// # The order
//
// Evidence is ranked by how much of the document it consumes and how narrowly
// the thing it matches identifies a format.
//
//  1. A root element that belongs to exactly one format — Facturae,
//     FatturaElettronica, InvoiceData, Finvoice, INVOICE_CENTER,
//     SCRDMCCBDACIOMessageStructure, Faktura with a Naglowek, and
//     CrossIndustryInvoice. These cannot compete with one another: a document
//     has one root and no two of these formats claim the same name. Nothing
//     later can overturn them, because a root name that only one vocabulary
//     uses is the strongest evidence a document offers.
//
//  2. Within the shared UBL root (Invoice, CreditNote), the Specification
//     identifier — BT-24, the cbc:CustomizationID. This is the one business
//     term whose entire purpose is to name the rule set the document follows,
//     so it outranks every structural hint. The tests that read it are the
//     ordered list specIDRules in cius.go, most specific first, and that list is
//     the same one DetectCIUS, the Is* predicates and ValidateCIUS read, so no
//     two of them can answer differently. Three entries are ordered rather than
//     merely listed:
//
//     a. "peppol:pint" (PINT) before the bare "peppol" (Peppol BIS Billing
//     3.0). "urn:peppol:pint:billing-1@my-1" is a Malaysian PINT invoice and
//     contains the substring "peppol"; PINT and BIS Billing are different rule
//     sets, and the identifier names PINT. The pre-release Japanese identifier
//     "urn:fdc:peppol:jp:billing:3.0" is read as PINT too, for the reason
//     written out at that entry.
//
//     b. "xrechnung" before "peppol", because an XRechnung identifier may also
//     reference the Peppol base.
//
//     c. "OIOUBL" before the "TR" prefix. Denmark's identifier carries a brand
//     name that appears in no other profile; the Turkish test is a
//     two-character prefix over a namespace everyone shares, which is the
//     weakest of the identifier tests and therefore the last of them. That is
//     what decides the audit's "TR-OIOUBL-2.02": OIOUBL.
//
//  3. ZATCA, which reads a ProfileID value and an AdditionalDocumentReference
//     rather than BT-24, so it runs after everything that reads BT-24. Real
//     ZATCA invoices carry no CustomizationID at all, so this costs nothing in
//     practice; the contrived document that carries "urn:peppol:pint:x" and
//     "reporting:1.0" together is PINT, because the specification identifier
//     is a claim about the rule set and a profile identifier is not.
//
//  4. The presence of a distinguishing child — Biller (ebInterface), then
//     SellerParty (Svefaktura). This is the weakest evidence in the table and
//     the pair is the weakest arbitration in it: a document carrying both is
//     not a real document of either format, and the order exists so the answer
//     is at least fixed and stated. ebInterface goes first because Biller
//     belongs to ebInterface's own vocabulary and appears in no UBL schema,
//     while SellerParty is an ordinary UBL 1.0 party role that a UBL 1.0
//     document other than a Svefaktura could also carry.
//
//  5. Anything still rooted Invoice or CreditNote is reported as EN 16931. The
//     root name is exactly what parseEN16931 dispatches the UBL mapper on, so
//     it is real evidence rather than a shrug, and the alternative — refusing
//     to answer — would leave the caller with nothing for the single most
//     common document this package sees.
//
// Everything else is SourceNone: a document that was read and recognised as no
// format this package validates. That is a third answer, distinct from the
// error, and the distinction is the same one the Is* predicates draw between
// (false, nil) and a non-nil error. Collapsing "I read this and it is not a
// format I know" into "I could not read this" would say something false about
// the file, and collapsing it the other way would route a truncated invoice to
// a validator that then reports another format's rules against it.
//
// # What it costs
//
// One streaming pass, the same one the Is* predicates make, over the same
// docShape. Detect builds no tree and spends no element budget, so the document
// that exhausts the parser's budget is still routable — which is the point of
// routing before validating.
func Detect(xmlData []byte) (Detection, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return Detection{}, err
	}
	return d.detect(), nil
}

// detect applies the precedence to a scanned shape.
func (d *docShape) detect() Detection {
	det := Detection{Root: d.root, SpecID: d.specID()}
	det.CIUS = DetectCIUS(det.SpecID)
	det.Source = route(d.facts())
	return det
}

// routeFacts is everything the arbitration reads about a document, and the
// whole of the coupling between the two callers that have to make this decision.
//
// There are two of them because they arrive by different roads. Detect has a
// streaming scan and no tree, which is what lets it route a document the parser
// would refuse; ValidateCIUS has already parsed the document and must not read
// the bytes a second time, because the element budget belongs to the document
// rather than to the number of layers a call passed through. Neither can use the
// other's reading. What they can share — and now do — is the decision, so each
// fills this in from what it already has and hands it to route.
type routeFacts struct {
	// root is the local name of the root element, namespace discarded.
	root string
	// specID is the Specification identifier (BT-24) as the document wrote it.
	specID string
	// profileID is the text of the root's first ProfileID child, which is what
	// ZATCA is recognised by.
	profileID string
	// The distinguishing direct children, each belonging to one format's
	// vocabulary.
	hasNaglowek    bool
	hasBiller      bool
	hasSellerParty bool
	// zatcaMarked reports an AdditionalDocumentReference whose ID reads "ICV".
	//
	// It is a function rather than a bool because the two callers pay very
	// differently for it: the scan has the answer already, while the model-fed
	// path has to walk the whole tree for it. route asks only for a UBL document
	// whose Specification identifier named nothing, so on every document that
	// declares a profile — which is most of them — the walk never happens.
	zatcaMarked func() bool
}

// facts reads the arbitration's inputs off a scanned shape.
func (d *docShape) facts() routeFacts {
	return routeFacts{
		root:           d.root,
		specID:         d.specID(),
		profileID:      d.str("ProfileID"),
		hasNaglowek:    d.hasNaglowek,
		hasBiller:      d.hasBiller,
		hasSellerParty: d.hasSellerParty,
		zatcaMarked:    func() bool { return d.icvDocRef },
	}
}

// facts reads the same inputs off a document that has already been parsed, so
// that ValidateCIUS routes on the arbitration Detect applies without reading the
// bytes again. Each field is the tree expression of the scan's capture above:
// the specification identifier is the mapper's own, and the three children are
// direct children of the root, which is what the scan retains.
func (p *parsed) facts() routeFacts {
	return routeFacts{
		root:           p.root.name,
		specID:         p.inv.specID,
		profileID:      p.root.str("ProfileID"),
		hasNaglowek:    p.root.child("Naglowek") != nil,
		hasBiller:      p.root.child("Biller") != nil,
		hasSellerParty: p.root.child("SellerParty") != nil,
		zatcaMarked:    func() bool { return zatcaDocRef(p.root, "ICV") },
	}
}

// route is step 1 of the order: the roots that belong to exactly one format, and
// the two shared roots that delegate.
func route(f routeFacts) Source {
	switch f.root {
	case "Facturae":
		return SourceFacturae
	case "FatturaElettronica":
		return SourceFatturaPA
	case "InvoiceData":
		return SourceOSA
	case "Finvoice":
		return SourceFinvoice
	case "INVOICE_CENTER":
		return SourceTEAPPS
	case "SCRDMCCBDACIOMessageStructure":
		// Order-X has no Is* predicate — it is an order, not an invoice, so no
		// caller asked "is this an invoice of format X" about it. It has a root
		// no other format uses and a validator of its own, so leaving it out of
		// the routing would have been an omission rather than a decision.
		return SourceOrderX
	case "Faktura":
		// The Polish root without its Naglowek head is not a KSeF FA document,
		// and nothing else claims the name.
		if f.hasNaglowek {
			return SourceKSeF
		}
		return SourceNone
	case "CrossIndustryInvoice":
		// The CII syntax is used by Factur-X/ZUGFeRD and by the CIUS that
		// publish a CII binding, and by no national format in this package, so
		// the specification identifier is the only question left to ask — and
		// only the part of it that names a CIUS, since OIOUBL and UBL-TR are UBL
		// vocabularies that no CII document is written in.
		//
		// PINT is a CIUS constant and so is reached here, though it publishes no
		// CII binding either. That is deliberate: a CrossIndustryInvoice
		// declaring a PINT identifier is a contradiction, and answering PINT
		// sends it to a validator that says so — "the document root shall be a
		// UBL Invoice or CreditNote" — rather than quietly validating it against
		// a rule set it did not claim.
		//
		// Factur-X is asked about first and separately, because it is not a CIUS
		// and so has no CIUS constant for DetectCIUS to answer with — it is a
		// profile family with a rule set of its own, which is what SourceFacturX
		// says. specIDRules is consulted rather than a test written here, so the
		// order this shares with routeUBL cannot drift: a ZUGFeRD document
		// declaring the XRechnung identifier matches that rule first and comes
		// back SourceXRechnung, which falls through to the CIUS question below
		// and is answered there.
		if specIDSource(f.specID) == SourceFacturX {
			return SourceFacturX
		}
		if src := ciusSource(DetectCIUS(f.specID)); src != SourceNone {
			return src
		}
		return SourceEN16931
	case "Invoice", "CreditNote":
		return routeUBL(f)
	}
	return SourceNone
}

// routeUBL is steps 2 to 5: arbitration inside the root name that four national
// formats, the CIUS and the EN 16931 UBL binding all share.
func routeUBL(f routeFacts) Source {
	// Step 2 — the Specification identifier, most specific test first. The order
	// within it is specIDRules, which DetectCIUS and the Is* predicates read
	// too, so this cannot drift from what ValidateCIUS routes on.
	if src := specIDSource(f.specID); src != SourceNone {
		return src
	}
	switch {
	// Step 3 — ZATCA, which reads a profile identifier and a document reference
	// rather than BT-24.
	case strings.Contains(strings.ToLower(f.profileID), "reporting") || f.zatcaMarked():
		return SourceZATCA
	// Step 4 — a distinguishing child, the weakest evidence here.
	case f.hasBiller:
		return SourceEbInterface
	case f.hasSellerParty:
		return SourceSvefaktura
	}
	// Step 5 — a UBL invoice declaring no national profile.
	return SourceEN16931
}
