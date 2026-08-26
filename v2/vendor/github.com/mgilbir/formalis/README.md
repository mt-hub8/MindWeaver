# formalis

A dependency-free (Go standard library only) validator for electronic invoices.

It checks **EN 16931** and the **Core Invoice Usage Specifications (CIUS)**
layered on it, and it also checks the national invoice formats that are not
EN 16931 profiles at all — Italian FatturaPA, Spanish Facturae, Austrian
ebInterface, Polish KSeF, Finnish Finvoice and TEAPPS, Danish OIOUBL, Swedish
Svefaktura, Hungarian NAV Online Számla, Turkish UBL-TR, Saudi ZATCA, and Peppol
PINT for AE / AUNZ / EU / JP / MY / OM / SG. The Franco-German Order-X order
document is validated too. Twenty-one rule sets in all; the table below names
each one, the entry point that runs it, and the `Source` that scopes its
findings.

One syntax-neutral rule engine validates both EN 16931 syntaxes — UN/CEFACT
Cross Industry Invoice (CII, used by Factur-X/ZUGFeRD) and OASIS UBL (Peppol
BIS, XRechnung, NLCIUS) — and each CIUS adds its own rule layer on top of it.
The formats that are not EN 16931 profiles are checked against their own
mandatory structure and code lists instead.

Every exported function is safe to call from any number of goroutines at once:
the package holds no mutable global state, and `TestValidatorsAreSafeForConcurrentUse`
pins that under `-race`.

## Usage

```go
report, err := formalis.ValidateCIUS(ctx, xml)
if err != nil {
    // The input could not be read at all: malformed XML, or an encoding this
    // package does not implement. Nothing was judged.
    return err
}

for _, v := range report.Fatal() {
    fmt.Printf("%s %s: %s\n", v.Source, v.Rule, v.Message)
}

fmt.Println("nothing found:", len(report.Violations) == 0)
fmt.Println("conformant:   ", report.Conformant())
```

`ValidateCIUS` routes the document against whichever rule set it declares. Every
exported validator has this shape — `func(context.Context, []byte) (Report, error)`
— and `Report` is the whole answer:

```go
type Report struct {
    Violations   []Violation  // what was found
    NotEvaluated []RuleFamily // the rule families that were not evaluated
}

func (r Report) Conformant() bool  // no fatal finding, no closeable fatal gap, run not cut short
func (r Report) Complete() bool    // every rule anyone *can* evaluate was evaluated
func (r Report) Fatal() []Violation
func (r Report) Warnings() []Violation
```

The zero `Report` is deliberately neither `Conformant` nor `Complete`, so a value
nobody filled in — including the one returned beside an error — cannot pass for a
clean invoice.

## Four answers, and the difference between them

The distinctions this package exists for are all in how a call can end:

| Outcome | Means | Read it with |
|---|---|---|
| `error`, zero `Report` | the input could not be read at all — malformed XML, an unimplemented encoding. A statement about the *file* | `errors.Is(err, formalis.ErrMalformedXML)`, `ErrUnsupportedEncoding` |
| a `RuleLimit` finding | the run stopped before it had seen everything — cancelled context, tripped budget. A statement about the *run* | `formalis.IsCheckerViolation(v)` |
| findings | the document departs from rules that were evaluated | `report.Fatal()`, `report.Warnings()` |
| no findings | everything that *was* evaluated passed | `report.NotEvaluated`, `report.Conformant()` |

A run that stopped stays a finding rather than becoming an error, deliberately:
the checks that did complete are still true, and an error would discard them.
A well-formed document that is simply not an invoice is a finding too — `RuleRoot`
for the EN 16931 entry points, `FPA-root`, `ZA-root`, `ORDER-root` … for the
national ones — because the document *was* read.

## Severity

Every `Violation` carries the severity its authority gave the rule: CEN's
`flag="fatal"` or `flag="warning"` in the Schematron, and the national
authorities' equivalents. Every one of those flags is a quotation checked against
the artefact that publishes it — for CEN, KoSIT and OpenPEPPOL, and since
`make cius-schematron` for the Portuguese, Romanian, Belgian, Serbian and Dutch
rule sets as well, whose severities used to be this package's fail-safe reading of
prose. `SeverityFatal` is the zero value, so an unstamped
finding reads as blocking rather than as advisory — the fail-safe direction.

Four rule sets report warnings, and everything else this package implements is
fatal:

- the advisory halves of CEN's two EN 16931 syntax bindings — 676 `UBL-CR-*`, 21
  `UBL-DT-*`, 440 `CII-SR-*` and 31 `CII-DT-*` assertions, generated from CEN's
  Schematron into a table this package evaluates. Their job is to hold a document
  down to the EN 16931 core subset of UBL and CII, which is a thing a reference
  validator reports and no authority rejects an invoice for;
- the advisory "not recommended" tier of both NLCIUS bindings — `BR-NL-19` to
  `BR-NL-35`, twenty identifiers in the UBL binding and twenty-one in the CII one,
  which SimplerInvoicing flags `warning` — plus the empty-element rule each binding
  ends with, `SI-UBL-2` and `empty-element-check`, also flagged `warning`;
- eleven of XRechnung's fifty-seven — the invoice type code, the specification
  identifier, the two IBAN checks, the telephone and email formats and five more,
  which KoSIT flags `warning` or `information`;
- six of OpenPEPPOL's — the Italian, Danish and Swedish participant-identifier
  format checks `PEPPOL-COMMON-R044/R045/R046/R047/R052/R053` — plus
  `PEPPOL-EN16931-R120`, which OpenPEPPOL flags fatal and KoSIT re-flags `warning`
  when it merges it into XRechnung, so the same rule is a non-conformance on the
  Peppol path and a warning on the German one;
- eighteen of OpenPEPPOL's 101 country-specific rules, among them all six Swedish
  Bankgiro/Plusgiro checks `SE-R-007..012`, the Greek `GR-S-008-1` and `GR-S-011`,
  the Danish `DK-R-003`/`DK-R-017`, the Norwegian `NO-R-002`, the Icelandic
  `IS-R-001` and six German ones.

(This section said "one rule set and one only" until the second and third arrived,
and "three" until the Dutch advisory tier landed; the KoSIT half had been true for a
release before anyone corrected the sentence.)

That is checked rather than assumed, in both directions: one test sweeps the whole
corpus and fails on any finding whose severity does not match the half of the
package it came from, and another reads the flag off each of the EN 16931 rules the
corpus exercises straight from the vendored CEN Schematron and compares it with the
severity the finding carried.

Where the identifier was minted here rather than quoted — `FPA-*`, `ZA-*`,
`ORDER-*` and the rest — there is no flag to quote, and fatal is a decision: each
of those rules checks a mandatory element of the format's own schema or a value
outside its own code list, so a document that breaks one is a document the
authority's gateway rejects. `go doc formalis.Severity` says so.

`Coverage`'s families carry the authority's flag too, and that is what makes
`Conformant()` answerable at all: an advisory gap leaves the verdict intact and
only makes the report less informative than a reference validator's, while a
fatal gap means a rule that could have rejected this document was never run.

They also carry `Unevaluable`, which is a different fact and not a softer
severity. It means the authority published a rule **nobody** can evaluate — CEN
binds `BR-CO-05..08` to the XPath expression `true()`, so the assertion cannot
fail, and `CII-DT-010/011/012` sit behind an earlier matching rule in CEN's own
ISO Schematron pattern, so no processor reaches them. That second shape turns out to
be common: three CIUS-RO rules, fifteen of SRBDT's thirty-six `RSR-*` and four
NLCIUS assertions are unreachable for exactly that reason, and eight of the Serbian
ones were being reported here until the rule order was read. A rule nothing can check is
not a rule this package skipped, so those do not cost a verdict and do not make a
run incomplete. The field is deliberately narrow: it does not mean hard, low
value, or not yet, and `go doc formalis.RuleFamily` spells out the boundary and
the two tests that hold the table to it.

## What a clean report does and does not mean

**`Conformant()` returns false for most documents, whatever they contain.** That is
not a bug and it is the first thing to understand about this package. The EN 16931
core, Factur-X and the seven CIUS layered on it are the exceptions today; every
national format validator here returns false for a clean invoice.

`len(report.Violations) == 0` means only *the checks that ran found nothing*. It
is equally true of a run that checked everything, a run that was cancelled or hit
a resource budget, and a run whose rule set does not implement every rule its
authority publishes. Every rule set here is a documented subset, and
`Coverage(src)` is where each one says so.

`Conformant()` is the weaker and more useful question, because it passes over the
gaps an authority would not reject a document for. Nine rule sets have no **fatal**
gap left — the core, Factur-X and every CIUS:

- the **EN 16931 core**: every fatal rule of the semantic model, of the UBL binding
  and of the CII binding is evaluated, bar the few CEN's own reference
  implementation cannot report, so `ValidateEN16931` — and
  `ValidateCIUS` on a document that declares no CIUS — returns
  `Conformant() == true` for a clean invoice;
- **Factur-X / ZUGFeRD**: everything the five profile Schematrons publish that is
  fatal — the four CEN-minted binding assertions they carry, the nine `BR-FXEXT-*`
  rules that are Factur-X's own new ground, the 24 `BR-FXEXT-*` that restate a CEN
  identifier the profile drops, and all 2,159 assertions of the per-profile element
  data model. `Validate` returns `Conformant() == true` for a clean document at
  every tier, and does so for all 32 profile-declaring documents FNFE ships;
- **XRechnung**: the Schematron a German buyer validates against is 78 identifiers,
  KoSIT's own 57 plus 21 it merges in from Peppol BIS Billing 3.0, and all 78 are
  evaluated. The imported findings carry `SourcePeppol`, because `Source` names the
  authority that wrote the rule;
- **Peppol BIS Billing 3.0**: both rule sets of the two vendored OpenPEPPOL
  Schematron files are evaluated — the 59 `PEPPOL-COMMON-*` and `PEPPOL-EN16931-*`
  identifiers, and the 101 country-specific rules published in the same files under
  a comment reading "National rules" (`DE-R-*`, `DK-R-*`, `GR-R-*`/`GR-S-*`,
  `IS-R-*`, `IT-R-*`, `NL-R-*`, `NO-R-*`, `SE-R-*`). That is 244 `(identifier,
  binding)` pairs, each evaluated in the binding that publishes it, and every one of
  them has a document in the suite that trips it and one that does not. The country
  rules are gated the way OpenPEPPOL gates them — on the supplier's country and, for
  the domestic ones, the customer's — so a French invoice answers to none of them.
- **CIUS-PT**: all 363 identifiers AT/eSPap publishes are evaluated — the 65
  `BR-CIUS-PT-*` business rules, AT's own eight `BR-AA-*` for the "Lower rate" VAT
  category, and the 290 `DT-CIUS-PT-*` datatype and arithmetic rules over the 291
  assertions that carry them. That last family is four fifths of the Portuguese
  rule set by count and no coverage entry here named it until the Schematron was
  vendored; it is generated from that Schematron rather than transcribed, the way
  CEN's advisory binding rules are. It is the first CIUS whose *datatype* tier is
  implemented at all.
- **CIUS-RO**: all 121 identifiers ANAF publishes in release 1.0.9 — the 25
  `BR-RO-NNN` business rules by hand, and 90 length, decimal, date-format and
  occurrence rules generated from `RO16931-rules.sch`. The six that no Schematron
  processor can report are marked `Unevaluable`;
- **UBL.BE**: all 15 `ubl-BE-*` identifiers of the `ubl-model-BE` pattern, including
  the two on the `cac:AdditionalDocumentReference` group and the two bilingual
  free-text code lists. `ubl-BE-13` is `Unevaluable`: the authority binds it to
  `abs($TaxAmount) >= 0` over a variable that falls back to `-1`, so it cannot fail;
- **SRBDT**: all 46 identifiers the Serbian Ministry of Finance publishes — 21
  reachable `RSR-*` business rules, the 3 `RSE-*` srbdtext extension rules, and the
  7 assertions of the abstract `pdvcat` pattern, which the validation schema
  instantiates once per zero-rate VAT category. Fifteen `RSR-*` are `Unevaluable`:
  `EN16931-UBL-srbdt.sch` is a single pattern in which eleven rules repeat the
  context `/ubl:Invoice | /cn:CreditNote` and four more repeat three other contexts,
  and ISO Schematron gives a node to the first matching rule of a pattern only;
- **NLCIUS**: every identifier of both bindings — 43 in UBL, 34 in CII — in both
  halves: the twelve fatal rules, the twenty-two "not recommended" ones, which are
  reported as warnings, the eight `BR-GA-*` of the **G-account extension**, which
  the UBL binding alone publishes, and the empty-element rule each binding ends
  with, which each names differently (`SI-UBL-2` in UBL, `empty-element-check` in
  CII). Four are `Unevaluable` for the rule-order reason above, and one of those was
  a live false positive: `BR-NL-9` has a rule of its own in the CII binding, against
  a context `BR-NL-7`'s rule already holds, so no CII document can be reported for
  it. This package reported it until the file was read that way. Nothing else is a
  gap — `Coverage(SourceNLCIUS)` holds `Unevaluable` entries only.

  The empty-element rule is the one rule in either binding whose context carries no
  gate, so `ValidateNLCIUS` reports it for a document that does not declare the
  NLCIUS specification identifier at all, which is what SimplerInvoicing's own
  validator does. It is also *last* in its pattern, so an empty element an earlier
  rule of that pattern already claims is reported under that rule's identifier and
  not as an empty element: an empty `cbc:TaxCurrencyCode` in a Dutch NLCIUS invoice
  is `BR-NL-19`, not `SI-UBL-2`.

  The G-account extension is the invoice form for a Dutch *g-rekening*: a blocked
  account into which a contractor pays the payroll-tax share of a subcontractor's
  invoice. Such an invoice carries two payment instructions and two payment terms
  rather than one of each, and the eight rules are about that split. It is opt-in —
  the document declares `…#conformant#urn:fdc:nen.nl:gaccount:v1.0` — and it is part
  of NLCIUS rather than a profile beside it, because both NLCIUS binding files fold
  the extension's identifier into their own `$si` and `$s` gates and the extension's
  Schematron `<include>`s the whole of SI-UBL 2.0. `ValidateNLCIUS` applies it to a
  UBL **Invoice** that declares the extension or carries a `GACCOUNT` payment
  instruction, and to nothing else: SimplerInvoicing publishes no CII binding of it
  and none of its rules for a credit note.

Every national format validator still names a gap its authority flags fatal and a
validator could close, so those return false whatever the document.

`Complete()` is the stricter question — "did this package see everything a
reference validator could see" — and all eight of those rule sets answer yes. The EN 16931
core's 1,168 advisory binding rules used to be the reason it could not;
they are evaluated now, and what is left in `Coverage(SourceEN16931)` is seven
rules **CEN itself cannot evaluate**: four bound to the XPath expression `true()`,
three unreachable in CEN's own Schematron rule ordering, and one whose UBL test a
correctly PCI-masked card number trips. Those are marked `Unevaluable`, so they no
longer hold the answer down — a rule nobody can check is not a rule this package
skipped. The thirteen national format validators still name gaps they could close,
so `Complete()` is false there.

The XRechnung path answers yes for a different reason: it had exactly one gap, and
it was a rule set it *imports* rather than one of its own — the twenty-one Peppol
rules the released artefact merges in, one of which (`PEPPOL-EN16931-R061`) had
replaced KoSIT's withdrawn `BR-DE-29`, so BG-19's mandate reference was checked by
nothing on the German path at all.

The CIUS-PT path answers yes for a fourth reason, and it is the one this package
had been carrying longest: its only gap was 290 fatal rules that had never been
named anywhere. `ValidateCIUSPT` reported `Conformant() == false` for every
Portuguese invoice, whatever it contained, until they were implemented.

The NLCIUS path answers yes for a fifth reason, and it is the one worth reading.
Implementing its "not recommended" tier made it the first rule set here whose last
gap was advisory, and `Complete()` was true for a Dutch invoice on that basis — but
the survey behind that had never counted the whole rule set. Every guard over these
five national rule sets asked its question about the identifiers a *prefix*
admitted, `^BR-NL-` in this case, and the two bindings publish three things it does
not match: the eight `BR-GA-*` of the G-account extension, and one advisory rule
each binding names differently, `SI-UBL-2` and `empty-element-check`. Naming the
last of those made `Complete()` false again; evaluating it has made it true, and
this time over an inventory that was enumerated rather than pattern-matched.

The general defect is fixed rather than the instance. Every identifier a vendored
national Schematron publishes is now enumerated and then classified — its
authority's own, another vendored authority's by lookup, or a named withdrawn one —
and one that no classifier accounts for fails the build. A prefix could only ever
enumerate what its author anticipated, which is how AT/eSPap's eight `BR-AA-*` rules
went unnoticed before it and SimplerInvoicing's eight `BR-GA-*` after.

The Peppol path answers yes for a third: its only gap was a rule set nobody had
counted. Both binding files hold a second family of 101 country-specific rules
beside the 59 `PEPPOL-*` ones, and every coverage survey here had matched on the
prefix `PEPPOL-` and stopped — so `ValidatePeppol` reported `Conformant() == false`
for every document, for that reason alone.

Note what `Complete()` is *not*: it says nothing about what was found. A document
with twenty fatal findings can be `Complete` — every rule ran, and twenty of them
failed. `Conformant()` is the verdict; `Complete()` is the statement about the
checker.

Rather than hide that behind a number in this file that would drift as rules
land, the package makes it machine-readable:

- **`Coverage(src Source) []RuleFamily`** names the rule families `src` publishes
  and this package does not evaluate, with the authority's flag on each and
  whether anyone could evaluate it at all. It takes no document, parses nothing
  and cannot fail, so you can ask *before* deciding to trust a validator.
- **`Report.NotEvaluated`** is the same information for the run that just
  happened — the union across every authority that call applied. A validator that
  layers a CIUS on the core reports both sets.
- **`Report.Complete()`** is false when a rule a validator *could* have evaluated
  went unevaluated, whatever its severity, or when the run stopped early.
  **`Conformant()`** passes over the advisory holes as well.

```go
for _, gap := range formalis.Coverage(formalis.SourceFatturaPA) {
    fmt.Printf("not evaluated: %s [%s] unevaluable=%t\n", gap.Rules, gap.Severity, gap.Unevaluable)
}
// not evaluated: the SdI FatturaPA XSD and the SdI's consistency checks [fatal] unevaluable=false

for _, gap := range formalis.Coverage(formalis.SourceNLCIUS) {
    fmt.Printf("not evaluated: %s [%s] unevaluable=%t\n", gap.Rules, gap.Severity, gap.Unevaluable)
}
// not evaluated: BR-NL-9, in the CII binding only [fatal] unevaluable=true
// not evaluated: BR-NL-31, in the CII binding only [warning] unevaluable=true
// not evaluated: BR-NL-32-2, BR-NL-32-3, in the UBL binding only [warning] unevaluable=true
```

The answers are two kinds and the two lists show one each. The Italian entry is
work: a rule the SdI would reject a document over, which this package does not run,
so it costs `Conformant()`. Every Dutch entry is `unevaluable=true` — a fact about
somebody else's file, assertions SimplerInvoicing publishes that no validator, its
own included, ever reaches — so they cost neither `Conformant()` nor `Complete()`.
That is why an Italian invoice with no findings is reported neither conformant nor
complete and a Dutch one with no findings is reported both.

Use `Conformant()` when you need the strong claim, and `len(r.Fatal()) == 0` when
the weaker one will do — with `r.NotEvaluated` beside it saying exactly what it
omits.

## Coverage

| Format | Entry point | `Is*` predicate | `Source` |
|---|---|---|---|
| EN 16931 core (CII + UBL) | `ValidateEN16931`, `ValidateCIUS` | — | `SourceEN16931` |
| Factur-X / ZUGFeRD (FR/DE), five profiles | `Validate` (with a `Profile`), `ValidateCIUS` | — | `SourceFacturX` |
| XRechnung (DE) | `ValidateXRechnung` | — | `SourceXRechnung` |
| Peppol BIS Billing 3.0 | `ValidatePeppol` | — | `SourcePeppol` |
| NLCIUS / SimplerInvoicing (NL) | `ValidateNLCIUS` | — | `SourceNLCIUS` |
| CIUS-PT (PT) | `ValidateCIUSPT` | — | `SourceCIUSPT` |
| CIUS-RO / RO e-Factura (RO) | `ValidateCIUSRO` | — | `SourceCIUSRO` |
| UBL.BE (BE) | `ValidateUBLBE` | — | `SourceUBLBE` |
| SRBDT (RS) | `ValidateSRBDT` | — | `SourceSRBDT` |
| Peppol PINT (AE/AUNZ/EU/JP/MY/OM/SG) | `ValidatePINT` | `IsPINT` | `SourcePINT` |
| FatturaPA / FatturaElettronica (IT) | `ValidateFatturaPA` | `IsFatturaPA` | `SourceFatturaPA` |
| Facturae (ES) | `ValidateFacturae` | `IsFacturae` | `SourceFacturae` |
| ebInterface (AT) | `ValidateEbInterface` | `IsEbInterface` | `SourceEbInterface` |
| KSeF FA (PL) | `ValidateKSeF` | `IsKSeF` | `SourceKSeF` |
| Finvoice (FI) | `ValidateFinvoice` | `IsFinvoice` | `SourceFinvoice` |
| TEAPPSXML (FI) | `ValidateTEAPPS` | `IsTEAPPS` | `SourceTEAPPS` |
| OIOUBL (DK) | `ValidateOIOUBL` | `IsOIOUBL` | `SourceOIOUBL` |
| Svefaktura (SE) | `ValidateSvefaktura` | `IsSvefaktura` | `SourceSvefaktura` |
| ZATCA (SA) | `ValidateZATCA` | `IsZATCA` | `SourceZATCA` |
| NAV Online Számla / OSA (HU) | `ValidateOSA` | `IsOSA` | `SourceOSA` |
| UBL-TR e-Fatura (TR) | `ValidateTurkishInvoice` | `IsTurkishInvoice` | `SourceUBLTR` |
| Order-X (order, not invoice) | `ValidateOrderXML` | — | `SourceOrderX` |

Eight of these are `CIUS` constants — XRechnung, Peppol, NLCIUS, CIUS-PT,
CIUS-RO, UBL.BE, SRBDT and PINT — and `ValidateCIUS` routes to them on the
document's **Specification identifier** (BT-24, what `DetectCIUS` reads). It
routes to OIOUBL and UBL-TR on that same identifier though neither is a CIUS, to
ZATCA on a profile identifier or a document reference, to ebInterface and
Svefaktura on a distinguishing child element, to the remaining formats on their
root element, and otherwise to the EN 16931 core. There is no `Is*` predicate for the EN 16931 core, for the
seven CIUS layered on it, or for Order-X, and none is needed: the CIUS are told
apart by that identifier (`DetectCIUS`, or `Detection.CIUS`), and Order-X by a
root element no other format uses.

`SourceChecker` is a `Source` but not a format: it carries this package's
statements about its own run, or about the file it was handed — `RuleLimit`,
`RuleProfile` and `RuleRoot` — and
`Coverage(SourceChecker)` is nil because it publishes no rules. `SourceNone` is
the zero value, which `Detect` reports for a document it read and recognised as
no format here, and which no `Violation` ever carries.

## Routing

`Detect` is the routing entry point. It reads the document once, without building
a tree, and arbitrates between the formats in an order that is part of its
documented contract:

```go
det, err := formalis.Detect(xml)
switch {
case err != nil:
    // Could not read it. Do not dispatch on this.
case !det.Recognised():
    // Read it; no format this package validates.
default:
    report, err := det.Validator()(ctx, xml)
}
```

`Detection` also carries `SpecID` (BT-24 as the document wrote it, trimmed),
`CIUS` and `Root`, and `Coverage(det.Source)` answers what the validator it named
will not check — before the call.

The twelve `Is*` predicates remain, and each answers about **one** format. They
are independent tests, **not a partition**: more than one can be true of the same
bytes (an `<Invoice>` with both a `Biller` and a `SellerParty` is `IsEbInterface`
*and* `IsSvefaktura`). That is why `Detect` exists — it is the arbitration,
written down once. Each predicate answers three ways:

```go
ok, err := formalis.IsFacturae(xml)
switch {
case err != nil:
    // Malformed XML, an unsupported encoding, or a tripped guard.
    // Not the same thing as "not a Facturae invoice".
case ok:
    report, err := formalis.ValidateFacturae(ctx, xml)
}
```

`Profile` is a Factur-X/ZUGFeRD **data-richness** tier (`MINIMUM`, `BASIC WL`,
`BASIC`, `EN 16931`, `EXTENDED`). It never selects a *national* rule set, and it
does select Factur-X's: naming one says "judge this as Factur-X", which means the
EN 16931 core, the rules that tier is expected to satisfy, and the CII syntax
binding FNFE-MPE publishes for that profile rather than CEN's. `ProfileFor` maps a
PDF's XMP `ConformanceLevel` to one; `CIUSFor` maps the levels that name a CIUS
instead (today, `"XRECHNUNG"`). A `Profile` this package does not implement is
refused with a `RuleProfile` violation rather than silently read as EN 16931.

`ValidateCIUS` and `Detect` read the tier back out of BT-24 for a document that
did not come with a `Profile`, and they accept **both** identifiers each tier is
published under: Factur-X 1.0 and ZUGFeRD 2.x are one specification under two
brands, and FNFE's own code database enumerates
`urn:factur-x.eu:1p0:minimum` and `urn:zugferd.de:2p0:minimum` for the same tier.
Only the EN 16931 tier has a single value — CEN's own `urn:cen.eu:en16931:2017`,
which is the document claiming to be exactly EN 16931 and is deliberately not
routed to Factur-X.

Factur-X binds EN 16931 with its own rule set and does not adopt CEN's CII syntax
binding: its five profile Schematrons carry four of CEN's 583 `CII-SR-*`/`CII-DT-*`
assertions and, in their place, a per-profile data model of between 48 and 1,241
assertions of their own — 2,159 across the five tiers, one per element of that
tier's element table. Judging a Factur-X document by CEN's binding reported 76
fatal findings on 13 of FNFE-MPE's own 59 published examples — documents FNFE's own
validator passes. So `Validate` is the **Factur-X** verdict, and `ValidateEN16931`
is CEN's, with CEN's binding and no `Profile` because CEN publishes none.

All 2,159 of those assertions are evaluated, the 366 code-list lookups included.
None of them carries an identifier in the artefact — FNFE names a rule by an
`[ID]-` prefix on its message and these have none — so each is reported under a
key this package mints, `FX-DM-<PROFILE>-<NNNN>`, documented in
`facturx_datamodel.go`.

The 42 `BR-FXEXT-*` rules that restate a CEN identifier the EXTENDED profile drops
were the last fatal gap, and the 24 of them FNFE leaves unflagged — which within
these files means fatal — are evaluated now. They were expected to be duplicates
of a stricter CEN rule this package already applies, and more than half of them
are not: `BR-FXEXT-CO-12` adds the logistics service charges (BT-X-272) EXTENDED
introduces to the charge total, `BR-FXEXT-CO-16` adds the charges collected on
behalf of a third party (BT-179), the nine `-08b` summations restrict the base to
`DETAIL` sub-lines and tighten CEN's ±1 tolerance to 0,01 × the operand count, and
`BR-FXEXT-CO-15` closes an escape clause CEN leaves open.
`facturx_restatements.go` states, rule by rule, what CEN asserts, what FNFE
asserts, and which fires where the other cannot.

They also run the other way, and that is a verdict rather than a list: for 21 of
the 24, FNFE's reading can be *satisfied* on a document CEN's rule reports — a
logistics charge folded into BT-108, a third-party charge folded into BT-115, a
`GROUP` line CEN counts and FNFE does not, a cent inside a tolerance FNFE has and
CEN has not. Two of FNFE's own published examples are in that position. Where a
document is being validated *as* Factur-X, the authority that governs it decides,
so the CEN finding is dropped when its Factur-X replacement holds — and only
then. `facturXAuthorityParity` implements it,
`TestAuthoritySamplesDrawNoFatalFinding` states the principle over every
authority in the tree that ships its own conformant samples, and
`ValidateEN16931` is unaffected: a caller who wants CEN's own verdict on a
Factur-X document still gets it, findings included.

So `Validate` reports `Conformant()` for a clean document again, at every tier.
`Coverage(SourceFacturX)` names what is left, and none of it is fatal and
evaluable: `CII-SR-464`, which FNFE rewrote into a tautology; three data-model
assertions that contradict their own context; one OpenPEPPOL rule FNFE merges in;
and the eighteen `-08ini`/`-08rev` readings FNFE flags warning, which keep
`Complete()` false and are an advisory gap — exactly the distinction the two
methods exist to draw.

One consequence is worth knowing before you count findings: this package reports
the **union** of CEN's rule set and Factur-X's, so a document that breaks both
`BR-S-08` and `BR-FXEXT-S08b` is reported twice, once under each authority's
identifier. FNFE's own validator prints one line, because its EXTENDED Schematron
does not carry `BR-S-08` at all. `Source` is the discriminator; filter on it for
exactly FNFE's verdict.

## Rule identity is `(Source, Rule)`

`Source` names the authority that defines a rule; two authorities may mint the
same string. Aggregate and suppress on the pair, never on `Rule` alone:

```go
type key struct {
    Source formalis.Source
    Rule   string
}
counts := map[key]int{}
for _, v := range report.Violations {
    counts[key{v.Source, v.Rule}]++
}
```

An identifier that *looks* like CEN's may not be CEN's. `SourceCIUSPT` carries
`BR-AA-01`…`BR-AA-07` and `BR-AA-10`, eight rules AT/eSPap wrote for the "Lower
rate" (`AA`) VAT category by cloning CEN's `BR-S-*` template — for a category code
EN 16931 leaves out of BT-118's restricted list, so CEN publishes no `BR-AA-*`
family at all. Keying on `Rule` alone would file them under the standard.

Most national formats publish no rule identifier this package could quote, so the
identifiers under those `Source`s — `FPA-*`, `FE-*`, `ZA-*`, `ORDER-*`, … — were
minted here. The `Source` is still the format the document was judged against,
which is what a caller routing or suppressing by format needs; it is not a claim
that the format's own documentation uses these names. The `Source`s whose
identifiers *are* quoted from a published rule set are EN 16931, XRechnung,
Peppol, NLCIUS, CIUS-PT, CIUS-RO, UBL.BE, SRBDT and Factur-X. `BR-GA-*` is SimplerInvoicing's
too — the G-account extension is published in the same repository under the same
customization identifier as NLCIUS, so its findings carry `SourceNLCIUS`.

## A CIUS may re-write a CEN rule's condition

Some national CIUS ship a **copy** of CEN's Schematron rather than referencing it,
and a copy can be edited. Where it has been, the authority's own validator evaluates
its edited condition — so a `BR-S-02` reported under that CIUS is CEN's identifier
judged by that authority's reading. This package honours that, and says so:

```go
report, _ := formalis.ValidateCIUSPT(ctx, xml)
for _, v := range report.Violations {
    if v.Reading != formalis.SourceNone {
        // v.Rule is CEN's, v.Source is SourceEN16931, and v.Reading names the
        // authority whose condition decided it. This finding will not reproduce
        // under a plain EN 16931 validation, and that is correct.
    }
}
```

`Violation.Reading` is `SourceNone` on every other finding, and `Violation.Error`
renders it, so a caller who only logs findings can still see it.

Which conditions count as the authority's own is derived rather than judged. A
copy that differs from CEN's *current* file usually differs because CEN changed the
file afterwards, which is a stale directory and not a national rule; the generator
in `testdata/cius-condition-overrides/` separates the two by asking CEN's own git
history whether CEN ever published what the copy carries. The result today:

| Authority | CEN identifiers in its copy | Same as CEN's current file | A CEN condition from an earlier release | The authority's own |
|---|---|---|---|---|
| CIUS-PT 2.1.1 (UBL) | 771 | 27 | 735 | **9**, applied |
| CIUS-RO 1.0.9 (UBL) | 930 | 904 | 26 | 0 |
| NLCIUS SI-UBL 2.0.3.2 | 929 | 866 | 63 | 0 |
| NLCIUS G-account 1.0.2 | 745 | 700 | 45 | 0 — but see below |
| NLCIUS 1.0.3 (CII) | 733 | 628 | 105 | 0 |
| UBL.BE v1.31 | 250 | 205 | 38 | **7**, recorded and not applied |
| SRBDT 1.0.0 | ships no copy of CEN's files | | | |

The nine Portuguese ones are the VAT category aliases `NOR`≡`S` and `ISE`≡`E`
across `BR-S-02/03/04/10` and `BR-E-02/03/04/10`, and `BR-23`, which AT/eSPap
inverts from an assertion into a report. Only `ValidateCIUSPT` applies them, only
to UBL documents, and only to those nine identifiers.

A copy can also **remove** a rule, which is an axis the table above does not have: it
compares the condition of each identifier a copy carries, and an assertion that is not
carried has no condition to compare. One copy removes a rule by commenting it out.
The G-account extension's `EN16931-syntax-modified.sch` comments out `UBL-CR-411`,
`UBL-CR-453` and `UBL-CR-459` — "a UBL invoice should not include the PaymentMeans
ID / the PaymentTerms PaymentMeansID / the PaymentTerms Amount" — because those
three elements are precisely what the extension carries its payment split in. All
three are advisory, and `ValidateNLCIUS` suppresses them for a document inside the
extension and for no other. The set is derived by comparing the modified file against
the unmodified one **in the same release directory**, which is what makes it a
measurement of SimplerInvoicing's edit rather than of the gap between two release
dates.

### What a copy leaves out

A copy can also simply **not carry** a rule, and absence has the same two causes as a
differing condition. The authority left the rule out, or CEN had not written it yet.
Telling them apart needs the release the copy was taken from, and that is derived
from the copy's own content: the CEN release that publishes every identifier the copy
carries and whose assertions the copy reproduces most closely. Version strings are
not consulted — a CIUS says "EN 16931" in its title whichever release it copied.

| Authority | CEN release it vendored | Identifiers CEN had published and it left out | Identifiers CEN has added since |
|---|---|---|---|
| CIUS-PT 2.1.1 (UBL) | `validation-1.1.0` (2018-06-26) | **114** | 78 |
| CIUS-RO 1.0.9 (UBL) | `validation-1.3.8` (2022-04-08) | 0 | 26 |
| NLCIUS SI-UBL 2.0.3.2 | `validation-1.3.6` (2021-05-30) | 0 | 28 |
| NLCIUS 1.0.3 (CII) | `validation-1.3.1` … `1.3.4` (2020-02-25) | 0 | 50 |

A release range means CEN republished those files unchanged and the evidence cannot
pin them any finer, which is said rather than rounded away. Three copies are not in
the table and each says why in `ciusCENCopyOmissions`: SRBDT ships no copy of CEN's
files at all, the G-account extension `<include>`s the whole of SI-UBL 2.0 so its
omissions are that row's, and UBL.BE's merged file re-cases 671 of CEN's identifiers
(`UBL-CR-001` as `ubl-CR-001`), which is a question about its identifier namespace
rather than about absence.

CIUS-PT is the only one that leaves a CEN rule out at all. What it leaves out is the
whole `BR-CL-*` code-list tier — its master Schematron includes no code-list file of
any name, where CIUS-RO's and both NLCIUS masters include CEN's — the `BR-AE-*`,
`BR-G-*`, `BR-IC-*`, `BR-O-*` and `BR-Z-*` VAT category families, `BR-CO-09..17`,
`BR-DEC-*`, and a handful more.

**None of them is suppressed**, and that is a decision rather than an omission.
Suppressing a rule the authority dropped is only right when the authority put
something in its place: AT/eSPap did, for the arithmetic tier — `DT-CIUS-PT-160..167`
answer the same questions as `BR-CO-10..17` — but with a **±1.00 € acceptance range**
where CEN's are exact identities. Across this repository's corpus the Portuguese rule
stays silent on 10 UBL documents where the CEN rule it displaced fires, so honouring
it instead would leave those documents reported by nothing. For the code-list tier and
the five VAT category families there is no Portuguese counterpart at all. Honouring
the deletion would turn a divergence from AT's validator into a class of invoice
nothing checks, and a false negative is worse than a false positive because nothing
reports it.

The consequence a caller should know: `ValidateCIUSPT` reports fatal EN 16931 findings
on all 20 instances AT/eSPap publishes as conformant — 216 of them — and **every one
is under an identifier AT's own rule set does not contain**. Not one is this package
over-reporting a rule AT publishes. If you need "what AT's validator would say", filter
the report against `Coverage` and the identifiers named in `ciusCENCopyOmissions`; if
you need "is this invoice EN 16931-conformant *and* CIUS-PT-conformant", which is what
`ValidateCIUSPT` answers, take it as it comes.

## Bounded work

Validation honours `ctx` and is bounded by the package's own limits on document
depth and element count, so the cost of a call is not set by a hostile document.
A run that stops early — a cancelled context, a tripped budget — reports a
`RuleLimit` violation rather than an empty `Violations` slice, so it can never be
read as a clean invoice:

```go
for _, v := range report.Violations {
    if formalis.IsCheckerViolation(v) {
        // The checker did not judge this document.
        // Neither conformant nor non-conformant: unknown.
    }
}
```

`RuleRoot` is deliberately not a checker violation: "this document is not an
EN 16931 invoice" is a definite answer about a document that *was* read. Neither
is the error an unreadable document produces, which does not arrive as a finding
at all.

## Examples

Runnable versions of everything above live in
[`example_test.go`](example_test.go) as Go `Example` functions, written against
the exported API from outside the package. `go test` compiles them and checks
their output, so an example that has drifted from the API fails the build rather
than misleading a reader. `go doc` shows them alongside the symbols they
document.

## Tests

`make test` runs the suite. Without the reference corpora it exercises the rule
engine against hand-written fixtures and skips the oracle-backed tests; that is
the mode a clean checkout and the fast CI job run in, and it is green.

The oracles need corpora this repository does not vendor (`testdata/` is
gitignored, with one carve-out: the six lean-tier Factur-X invoices under
`testdata/facturx/extracted/` are committed, because they exist only inside
PDF/A-3 containers and no fetch target can produce them — that directory's README
says why, and `facturx_lean_tiers_test.go` holds them as an expected-failure
table: what each draws, at which node, and a written reason that is itself
checked against the document and the artefact). Fetching the rest needs **`git`**, **`bash`**, **`curl`**, **`python3`**
— several Romanian and Portuguese sample filenames are non-ASCII and are
URL-encoded with it — and **`gh` authenticated against GitHub**, because the
fetch makes about fifteen `gh api` calls and the unauthenticated rate limit of 60
requests an hour is not enough for them. `make check-deps` verifies all of this
up front and names what is missing, rather than failing two hundred lines into a
download.

```
make check-deps                    # what the fetch targets need
make cius-oracles                  # ~600 documents: XRechnung, Peppol, NLCIUS,
                                   # the CIUS and the national-format samples,
                                   # plus (via cius-schematron) the five national
                                   # Schematrons the severity and coverage guards
                                   # read
make cius-schematron               # those Schematrons on their own: CIUS-PT,
                                   # CIUS-RO, UBL.BE, SRBDT and NLCIUS, plus each
                                   # authority's own copy of CEN's files
make en16931-artefacts             # the CEN/TC 434 per-rule unit-test suite, cloned
                                   # with full history (see condition overrides)
make en16931-ubl                   # the EN 16931 UBL example invoices
make en16931-genericode            # the official code lists (needs unzip)
make en16931-syntax-rules          # regenerate the advisory binding table
make cius-pt-rules                 # regenerate the CIUS-PT datatype table
make cius-ro-rules                 # regenerate the CIUS-RO length/decimal table
make cius-condition-overrides      # regenerate the per-CIUS condition-override table
make test
```

Each target is stamped, so re-running it is a no-op rather than an error; the
matching `clean-*` target removes the stamp and the data, and is how you force a
re-fetch.

Five targets go one step further and *regenerate* committed source. `make
en16931-codelists` rewrites the code-list tables from the genericode bundle,
`make en16931-syntax-rules` rewrites the advisory syntax-binding table from the CEN
Schematron, `make cius-pt-rules` rewrites the CIUS-PT datatype table from
AT/eSPap's, `make cius-ro-rules` rewrites the CIUS-RO length, decimal,
date-format and occurrence table from ANAF's, and `make cius-condition-overrides`
rewrites the table of CEN conditions each CIUS re-wrote — derived from every
authority's copy of CEN's Schematron and from CEN's own git history, which is why
`make en16931-artefacts` clones full depth. All five are deliberate acts and none is part of running the tests,
which is why fetching each oracle is its own target — and in both cases a test
re-derives the same data from the same source on every run and fails if the
committed table has drifted. The generators refuse to write anything they cannot
describe rather than skipping it: a rule quietly dropped by a generator is a rule
that silently stops being checked, with nothing to notice.

With the corpora present the suite runs with no skips, and each oracle ratchets
the number of documents it saw — the constants are collected in
[`corpus_test.go`](corpus_test.go). A corpus that arrives truncated therefore
fails the build rather than reporting a clean verdict over whatever landed, and
the fetch itself fails on the first download it cannot complete.
