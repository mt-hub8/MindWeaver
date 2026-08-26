# Every recipe runs under bash with `set -e -u -o pipefail`. This is load-bearing
# rather than tidiness: the oracle fetches below are ~600 downloads driven by
# `gh api | while read`, and under /bin/sh's defaults a failed `gh api` produces
# an empty list, the loop body never runs, and the target still succeeds. A
# corpus that half-arrived would then be indistinguishable from a corpus that
# arrived, and the FP=0 oracles would report a green verdict backed by whatever
# happened to land (C8). With pipefail, a failed `gh api` fails the recipe line.
SHELL := /bin/bash
.SHELLFLAGS := -e -u -o pipefail -c

# -f is the other half of the same problem: without it curl writes the server's
# error body to the output file and exits 0, so a 404 or a rate-limit response
# becomes an .xml file containing "404: Not Found". --retry rides out the
# transient 5xx that a few hundred sequential requests will otherwise hit.
CURL := curl -fsSL --retry 3 --retry-delay 2 --retry-connrefused

# Percent-encodes a repository path one segment at a time, so it can be pasted
# into a raw.githubusercontent URL. Several sample filenames carry spaces or
# non-ASCII characters — "Finvoice 2.01 example.xml", the Portuguese and
# Romanian samples — and curl refuses a URL containing them outright ("URL
# rejected: Malformed input to a URL function"). Under the old unchecked `curl
# -sSL` that refusal was invisible: the file was simply never fetched, the loop
# carried on, and the corpus was quietly one document short. Every loop that
# builds a URL from a path goes through this.
URLENC := python3 -c "import urllib.parse,sys;print('/'.join(urllib.parse.quote(x) for x in sys.argv[1].split('/')))"

.PHONY: test check-deps en16931-artefacts en16931-codelists en16931-genericode \
	en16931-syntax-rules en16931-ubl cius-oracles cius-schematron cius-pt-rules \
	cius-ro-rules cius-condition-overrides facturx-schematron facturx-datamodel facturx-examples \
	clean-en16931-artefacts clean-en16931-codelists clean-en16931-ubl clean-cius-oracles \
	clean-facturx-schematron

# Run the tests. Oracle-backed tests skip when their (gitignored) data is absent;
# fetch it with the targets below. When it is present, each oracle also ratchets
# the number of documents it saw — see corpus_test.go — so a partial fetch is a
# red build rather than a quiet one.
test:
	go test ./... -count=1

# Prerequisites for the fetch targets, checked up front rather than 200 lines
# into a download. `gh` must be authenticated: the fetches make ~15 `gh api`
# calls, and the unauthenticated rate limit is 60/hour, which is not enough.
# python3 URL-encodes filenames — several Romanian and Portuguese samples carry
# non-ASCII characters. `gh --jq` uses gh's embedded gojq, so a separate jq is
# not needed.
REQUIRED_TOOLS := git bash curl gh python3
check-deps:
	@missing=""; \
	for t in $(REQUIRED_TOOLS); do command -v "$$t" >/dev/null 2>&1 || missing="$$missing $$t"; done; \
	if [ -n "$$missing" ]; then \
		echo "make: missing required tool(s):$$missing" >&2; \
		echo "  the oracle fetches need: $(REQUIRED_TOOLS)" >&2; \
		exit 1; \
	fi; \
	if ! gh auth status >/dev/null 2>&1; then \
		echo "make: gh is installed but not authenticated" >&2; \
		echo "  run \`gh auth login\`, or set GH_TOKEN — the fetches make ~15 gh api calls" >&2; \
		echo "  and the unauthenticated rate limit (60/hour) is not enough for them" >&2; \
		exit 1; \
	fi

UBL_DIR := testdata/en16931-ubl
EN16931_DIR := testdata/en16931-artefacts
CODELISTS_DIR := testdata/en16931-codelists
CIUS_STAMP := testdata/.cius-oracles.ok
CIUS_SCH_STAMP := testdata/.cius-schematron.ok
FACTURX_SCH_STAMP := testdata/.facturx-schematron.ok

# git-sync clones $(1) into $(2), or refreshes it if it is already there, so the
# fetch targets can be re-run without `git clone` failing on an existing
# directory (C21). One shell line: each recipe line gets its own shell.
git-sync = if [ -d "$(2)/.git" ]; then git -C "$(2)" fetch --depth 1 origin HEAD && git -C "$(2)" reset --hard --quiet FETCH_HEAD; else rm -rf "$(2)" && git clone --quiet --depth 1 "$(1)" "$(2)"; fi

# git-sync-full is git-sync for a repository whose *history* is the oracle rather
# than its tip. There is one: CEN/TC 434's, because the only way to tell a CIUS's
# own edit to a CEN condition apart from a CEN condition the CIUS copied before CEN
# changed it is to ask which strings CEN has ever published (cius_overrides.go). A
# shallow clone answers "never" to every condition CEN has since edited, which would
# turn three stale vendored copies into nine hundred national overrides — so the
# depth is load-bearing and an already-shallow clone is unshallowed rather than left.
# It costs about 60 MB and four seconds once.
git-sync-full = if [ -d "$(2)/.git" ]; then \
		if [ -f "$(2)/.git/shallow" ]; then git -C "$(2)" fetch --unshallow --quiet; fi; \
		git -C "$(2)" fetch --quiet origin HEAD && git -C "$(2)" reset --hard --quiet FETCH_HEAD; \
	else rm -rf "$(2)" && git clone --quiet "$(1)" "$(2)"; fi

# EN 16931 UBL example invoices (CEN TC 434 + OpenPEPPOL) — the UBL FP=0 oracle.
en16931-ubl: $(UBL_DIR)/.ok
$(UBL_DIR)/.ok: $(UBL_DIR)/sources.tsv $(UBL_DIR)/download.sh
	bash $(UBL_DIR)/download.sh
	touch $@
clean-en16931-ubl:
	rm -f $(UBL_DIR)/*.xml $(UBL_DIR)/.ok

# Official CEN/TC 434 EN 16931 artefacts (Schematron + per-rule unit-test suite);
# differential oracle for the rule engine, and — with its history — the oracle for
# which conditions in a CIUS's copy of those files that CIUS wrote itself.
en16931-artefacts: $(EN16931_DIR)/.ok
$(EN16931_DIR)/.ok:
	$(call git-sync-full,https://github.com/ConnectingEurope/eInvoicing-EN16931,$(EN16931_DIR))
	touch $@
clean-en16931-artefacts:
	rm -rf $(EN16931_DIR)

# Official code lists (genericode + EAS/VATEX), in two halves. The bundle is the
# oracle en16931_codelists_test.go checks the committed tables against, and
# fetching it needs nothing but curl and unzip; gen.py *regenerates* those
# tables, which is a deliberate act and not something a test run should do. CI
# wants the first half only.
en16931-genericode: $(CODELISTS_DIR)/genericode/.ok
$(CODELISTS_DIR)/genericode/.ok: $(CODELISTS_DIR)/download.sh
	bash $(CODELISTS_DIR)/download.sh
	touch $@
en16931-codelists: en16931-genericode
	python3 $(CODELISTS_DIR)/gen.py
clean-en16931-codelists:
	rm -rf $(CODELISTS_DIR)/genericode $(CODELISTS_DIR)/*.zip $(CODELISTS_DIR)/*.xlsx

# The advisory halves of CEN's two syntax bindings, generated from the vendored
# Schematron into en16931_syntax_advisory_table.go. Same arrangement as the code
# lists above: the generator is a deliberate act, and the test that re-derives the
# same table from the Schematron runs on every `make test`.
#
# The test run at the end is not belt-and-braces. gen.py refuses an assertion
# whose XPath is outside the subset it recognises, and the package refuses to
# compile a table its own parser cannot read; those are two independent gates on
# the same property, and the second one is only reached by running the tests. A
# regeneration that wrote a file the package cannot read should fail here rather
# than in someone's build.
SYNTAX_RULES_DIR := testdata/en16931-syntax-rules
en16931-syntax-rules: en16931-artefacts
	python3 $(SYNTAX_RULES_DIR)/gen.py
	go test -count=1 -run 'TestAdvisory' .

# AT/eSPap's CIUS-PT datatype tier, generated from the vendored Schematron into
# cius_pt_datatype_table.go. Same arrangement as the two targets above: the
# generator is a deliberate act, and the test that re-derives the same tables from
# the Schematron runs on every `make test`.
#
# The test run at the end is not belt-and-braces. gen.py refuses an assertion whose
# XPath is outside the subset it recognises and refuses to write a table short of
# what the artefact publishes; the package refuses to compile a table its own parser
# cannot read. Those are independent gates on the same property, and the second is
# only reached by running the tests.
CIUS_PT_RULES_DIR := testdata/cius-pt-rules
cius-pt-rules: cius-schematron
	python3 $(CIUS_PT_RULES_DIR)/gen.py
	go test -count=1 -run 'TestCIUSPTDatatype|TestEveryCIUSPTDatatype' .

# ANAF's CIUS-RO length, decimal, date-format and occurrence tier, generated from
# the vendored Schematron into cius_ro_rules_table.go by the same arrangement, and
# with one gate the two targets above do not need: this generator also decides which
# published assertions no Schematron processor can report, and writes them out with
# the reason. gofmt runs because the emitter writes keyed struct literals whose
# alignment gofmt owns; the drift test compares decoded fields and not bytes, so the
# formatting is cosmetic, but a committed file `gofmt -l` complains about is not.
# The CEN conditions each CIUS re-wrote, generated from every authority's copy of
# CEN's Schematron and from CEN's own git history into cius_overrides_table.go. Same
# arrangement as the three targets above, and it needs both fetches: the copies come
# from cius-schematron and the history from en16931-artefacts, which clones full
# depth for this reason. A shallow clone stops the generator rather than being read
# as "CEN never published any of this".
CIUS_OVERRIDES_DIR := testdata/cius-condition-overrides
cius-condition-overrides: cius-schematron en16931-artefacts
	python3 $(CIUS_OVERRIDES_DIR)/gen.py
	gofmt -w cius_overrides_table.go
	go test -count=1 -run 'TestCIUSCopiesOfCEN|TestCIUSCopyOmissions|TestConditionOverride|TestEveryConditionOverride|TestOverriddenIdentifiers|TestUnappliedConditionOverrides|TestOmittedCENIdentifiers' .

CIUS_RO_RULES_DIR := testdata/cius-ro-rules
cius-ro-rules: cius-schematron
	python3 $(CIUS_RO_RULES_DIR)/gen.py
	gofmt -w cius_ro_rules_table.go
	go test -count=1 -run 'TestCIUSRO|TestEveryCIUSRO' .

# National CIUS oracles: KoSIT XRechnung, OpenPEPPOL BIS 3, the Dutch NLCIUS
# (SimplerInvoicing SI-UBL) instance test suite, and the national-format sample
# sets from phax/phive-rules.
#
# Guarded by a stamp file so a second `make cius-oracles` is a no-op instead of
# ~600 repeated downloads; `make clean-cius-oracles` removes it. check-deps is an
# order-only prerequisite: it runs first every time, and never makes the stamp
# look stale.
cius-oracles: $(CIUS_STAMP) $(CIUS_SCH_STAMP)
$(CIUS_STAMP): | check-deps
	$(call git-sync,https://github.com/itplr-kosit/xrechnung-schematron,testdata/xrechnung/schematron)
	$(call git-sync,https://github.com/itplr-kosit/xrechnung-testsuite,testdata/xrechnung/testsuite)
	$(call git-sync,https://github.com/OpenPEPPOL/peppol-bis-invoice-3,testdata/peppol/repo)
	mkdir -p testdata/nlcius/testsuite
	gh api repos/phax/phive-rules/contents/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/SI-UBL-2.0.3.2 --jq '.[].name' \
		| grep '\.xml$$' \
		| while read f; do $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/SI-UBL-2.0.3.2/$$f" -o "testdata/nlcius/testsuite/$$f"; done
	@# The G-account extension's own instances. They are a *separate* directory
	@# upstream, which is why the SI-UBL-2.0.3.2 fetch above never brought them in and
	@# why none of the 95 instances that oracle reads exercises the extension. Kept in
	@# a directory of their own here for the same reason: TestNLCIUSConformanceSuite
	@# and TestNLCIUSPerRuleFixtures read the file name for the rule it exercises, and
	@# these are named for the term they break rather than for a BR-GA identifier.
	mkdir -p testdata/nlcius/gaccount
	gh api repos/phax/phive-rules/contents/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/si-ubl-2.0-ext-gaccount-1.0 --jq '.[].name' \
		| grep '\.xml$$' \
		| while read f; do $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-simplerinvoicing/src/test/resources/external/test-files/simplerinvoicing/si-ubl-2.0-ext-gaccount-1.0/$$f" -o "testdata/nlcius/gaccount/$$f"; done
	@# CIUS-PT (Portuguese AT/eSPap) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-pt/testsuite
	for ver in 2.0.0 2.1.1; do \
		gh api "repos/phax/phive-rules/contents/phive-rules-cius-pt/src/test/resources/external/test-files/$$ver" --jq '.[] | select(.name|endswith(".xml")) | .name' \
		| while read -r name; do \
			enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
			$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-cius-pt/src/test/resources/external/test-files/$$ver/$$enc" -o "testdata/cius-pt/testsuite/$${ver}_$$name"; \
		done; \
	done
	@# CIUS-RO (Romanian ANAF RO e-Factura) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-ro/testsuite
	for ver in 1.0.3 1.0.4 1.0.8 1.0.9; do \
		gh api "repos/phax/phive-rules/contents/phive-rules-cius-ro/src/test/resources/external/test-files/$$ver" --jq '.[] | select(.name|endswith(".xml")) | .name' \
		| while read -r name; do \
			enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
			$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-cius-ro/src/test/resources/external/test-files/$$ver/$$enc" -o "testdata/cius-ro/testsuite/$${ver}_$$name"; \
		done; \
	done
	@# UBL.BE (Belgian) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-be/testsuite
	gh api "repos/phax/phive-rules/contents/phive-rules-ublbe/src/test/resources/external/test-files/en16931/v1.31" --jq '.[] | select(.name|endswith(".xml")) | .name' \
	| while read -r name; do \
		enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$name"); \
		$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/phive-rules-ublbe/src/test/resources/external/test-files/en16931/v1.31/$$enc" -o "testdata/cius-be/testsuite/$$name"; \
	done
	@# SRBDT (Serbian) sample instances, from phax/phive-rules.
	mkdir -p testdata/cius-rs/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-serbia/src/test/resources/external/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$$(basename "$$p")"); \
		$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$(dirname "$$p")/$$enc" -o "testdata/cius-rs/testsuite/$$(basename "$$p")"; \
	done
	@# FatturaPA (Italian FatturaElettronica) sample instances, from phax/phive-rules.
	mkdir -p testdata/fatturapa/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-fatturapa/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/fatturapa/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# Facturae (Spanish) sample instances, from phax/phive-rules.
	mkdir -p testdata/facturae/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-facturae/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/facturae/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# ebInterface (Austrian) sample instances, from phax/phive-rules.
	mkdir -p testdata/ebinterface/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-ebinterface/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/ebinterface/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# KSeF (Polish FA structured invoice) samples — current FA(3) version — from phax/phive-rules.
	mkdir -p testdata/ksef/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-ksef/") and contains("/test-files/fa3/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/ksef/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# Finvoice (Finnish) sample instances, from phax/phive-rules.
	mkdir -p testdata/finvoice/testsuite
	gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-finvoice/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/finvoice/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
	done
	@# ZATCA (Saudi Fatoora) UBL sample instances, from phax/phive-rules.
	mkdir -p testdata/zatca/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-zatca/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		i=$$((i+1)); enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/zatca/testsuite/f$$i.xml"; \
	done
	@# Svefaktura (Swedish) and TEAPPS (Finnish) sample instances, from phax/phive-rules.
	for m in svefaktura teapps; do \
		mkdir -p testdata/$$m/testsuite; \
		gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq ".tree[].path | select(contains(\"phive-rules-$$m/\") and contains(\"/test-files/\") and endswith(\".xml\"))" \
		| while read -r p; do \
			enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/$$m/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#/#_#g')"; \
		done; \
	done
	@# OIOUBL (Danish) invoices — content-filtered to Invoice-rooted OIOUBL files.
	mkdir -p testdata/oioubl/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-oioubl/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		enc=$$($(URLENC) "$$p"); b=$$($(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc"); \
		if echo "$$b" | grep -qE "<Invoice[ >]" && echo "$$b" | grep -q "CustomizationID>OIOUBL"; then \
			i=$$((i+1)); echo "$$b" > "testdata/oioubl/testsuite/inv$$i.xml"; \
		fi; \
	done
	@# UBL-TR (Turkish) sample instances, from phax/phive-rules.
	mkdir -p testdata/turkey/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-turkey/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		i=$$((i+1)); enc=$$($(URLENC) "$$p"); $(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/turkey/testsuite/f$$i.xml"; \
	done
	@# OSA (Hungarian NAV Online Szamla) sample instances, from phax/phive-rules.
	mkdir -p testdata/osa/testsuite
	i=0; gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq '.tree[].path | select(contains("phive-rules-osa/") and contains("/test-files/") and endswith(".xml"))' \
	| while read -r p; do \
		i=$$((i+1)); enc=$$($(URLENC) "$$p"); \
		$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/osa/testsuite/f$$i.xml"; \
	done
	@# Peppol PINT (all jurisdictions) sample instances, from phax/phive-rules.
	@# awk rather than `head -8`: head closes the pipe after the eighth line, and
	@# under pipefail the SIGPIPE it delivers upstream would fail the recipe.
	mkdir -p testdata/pint/testsuite
	for j in pint-ae pint-aunz pint-eu pint-jp pint-jp-ntr pint-my pint-om pint-sg; do \
		gh api "repos/phax/phive-rules/git/trees/master?recursive=1" --jq ".tree[].path | select(contains(\"phive-rules-peppol-pint/\") and contains(\"/test-files/$$j/\") and endswith(\".xml\"))" \
		| grep -iv invalid | awk 'NR<=8' | while read -r p; do \
			enc=$$($(URLENC) "$$p"); \
			$(CURL) "https://raw.githubusercontent.com/phax/phive-rules/master/$$enc" -o "testdata/pint/testsuite/$$(echo "$$p"|sed -E 's#.*/test-files/##;s#[/ ]#_#g')"; \
		done; \
	done
	touch $@

# The five national rule sets whose *Schematron* this repository needs and used
# not to fetch. `cius-oracles` above pulls each authority's sample instances out
# of phax/phive-rules under src/test/resources/external/test-files/ and walked
# straight past src/test/resources/external/rule-source/, which is where the same
# repository keeps the authorities' published rule sets. The consequence was not
# cosmetic: with no artefact to quote, every Coverage severity for CIUS-PT,
# CIUS-RO, UBL.BE, SRBDT and NLCIUS was this package's fail-safe guess, no
# implemented rule's XPath had ever been checked against the published one, and
# those five Sources had to be held permanently ineligible for
# RuleFamily.Unevaluable — a claim about a published file cannot be checked for a
# file that is not here (C35). They are .sch only, so this adds no documents to
# the instance corpora.
#
# Which version of each, and why: the one that matches the sample instances
# `cius-oracles` already fetches, so the artefact and the documents validated
# against it are the same release of the same rule set.
#
#   CIUS-PT   2.0.0 and 2.1.1     — test-files/2.0.0 and test-files/2.1.1
#   CIUS-RO   1.0.3/4/8/9         — test-files/1.0.3, 1.0.4, 1.0.8, 1.0.9
#   UBL.BE    en16931/v1.31       — test-files/en16931/v1.31
#   SRBDT     1.0.0               — the only rule-source version published (the
#                                   instances cover 1.0.0 and 1.1.0)
#   NLCIUS    SI-UBL 2.0.3.2      — test-files/simplerinvoicing/SI-UBL-2.0.3.2
#             NLCIUS-CII 1.0.3    — test-files/simplerinvoicing/NLCIUS-CII-1.0.3;
#                                   the CII binding is a separate version series
#                                   and this is the one whose instances are named
#
# What is deliberately *not* fetched: the copies of CEN's own EN16931-*.sch that
# the CIUS-RO and NLCIUS-CII trees vendor as dependencies. Those are already here
# under testdata/en16931-artefacts, and pulling them in beside a national rule set
# is how a survey comes to count CEN's rules as Portuguese or Romanian ones.
# CIUS-PT is the exception that proves the point: its abstract files are a
# *modified* copy of CEN's, in which the UBL-SR-19/20/21 CEN flags fatal are
# flagged warning, so they are fetched and the readers filter on the identifier
# prefix rather than on the file.
#
# Every fetch is one `$(CURL)` (which is -f, so a 404 is a failure and not an .sch
# containing "404: Not Found") on an explicit path under `set -euo pipefail`, so a
# file that moved upstream fails this recipe rather than quietly shrinking the
# oracle — C8's lesson, and C26's. The Go-side ratchets are the second half: each
# reader in cius_artefacts_test.go asserts a floor on the number of identifiers it
# decoded, so a file that arrives truncated or empty is a red build too.
PHIVE := https://raw.githubusercontent.com/phax/phive-rules/master
PT_RULESRC := phive-rules-cius-pt/src/test/resources/external/rule-source
RO_RULESRC := phive-rules-cius-ro/src/test/resources/external/rule-source
BE_RULESRC := phive-rules-ublbe/src/test/resources/external/rule-source
RS_RULESRC := phive-rules-serbia/src/test/resources/external/rule-source
NL_RULESRC := phive-rules-simplerinvoicing/src/test/resources/external/rule-source

cius-schematron: $(CIUS_SCH_STAMP)
$(CIUS_SCH_STAMP): | check-deps
	@# CIUS-PT (Portuguese AT/eSPap). The assertions live in the abstract files and
	@# the UBL files bind them to XPath through <param>, exactly as CEN's do, so
	@# both halves are needed to read a rule.
	for ver in 2.0.0 2.1.1; do \
		for f in "abstract/urn_feap.gov.pt_CIUS-PT_$$ver-model.sch" \
			"abstract/urn_feap.gov.pt_CIUS-PT_$$ver-syntax.sch" \
			"abstract/urn_feap.gov.pt_CIUS-PT_$$ver-condition.sch" \
			"UBL/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-model.sch" \
			"UBL/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-syntax.sch" \
			"UBL/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-condition.sch" \
			"datatype/urn_feap.gov.pt_CIUS-PT_$$ver-UBL-datatype.sch" \
			"urn_feap.gov.pt_CIUS-PT_$$ver.sch"; do \
			mkdir -p "$$(dirname "testdata/cius-pt/schematron/$$ver/$$f")"; \
			$(CURL) "$(PHIVE)/$$($(URLENC) "$(PT_RULESRC)/$$ver/$$f")" -o "testdata/cius-pt/schematron/$$ver/$$f"; \
		done; \
	done
	@# CIUS-RO (Romanian ANAF RO e-Factura). cius-ro/RO16931-rules.sch is the whole
	@# national rule set; the validation file beside it is the wrapper that says
	@# which patterns are active. The abstract/ and UBL/ siblings are ANAF's own copy
	@# of CEN's rules, and they are fetched for the version this package evaluates:
	@# a CIUS that ships a copy of CEN's files can have edited them, and the only way
	@# to know whether it did is to read the copy. codelist/ is not fetched — the code
	@# lists are checked against CEN's genericode by en16931_codelists_test.go.
	for ver in 1.0.3 1.0.4 1.0.8 1.0.9; do \
		mkdir -p "testdata/cius-ro/schematron/$$ver/cius-ro"; \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RO_RULESRC)/$$ver/cius-ro/RO16931-rules.sch")" -o "testdata/cius-ro/schematron/$$ver/cius-ro/RO16931-rules.sch"; \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RO_RULESRC)/$$ver/EN16931-CIUS_RO-UBL-validation.sch")" -o "testdata/cius-ro/schematron/$$ver/EN16931-CIUS_RO-UBL-validation.sch"; \
	done
	mkdir -p testdata/cius-ro/schematron/1.0.9/abstract testdata/cius-ro/schematron/1.0.9/UBL
	for f in abstract/EN16931-model.sch abstract/EN16931-syntax.sch \
		UBL/EN16931-UBL-model.sch UBL/EN16931-UBL-syntax.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RO_RULESRC)/1.0.9/$$f")" -o "testdata/cius-ro/schematron/1.0.9/$$f"; \
	done
	@# UBL.BE (Belgian). One file, and it is a merged artefact: alongside the 15
	@# ubl-BE-* rules it carries CEN's, OpenPEPPOL's and five of OpenPEPPOL's
	@# country sets. The reader filters on the ubl-BE- prefix.
	mkdir -p testdata/cius-be/schematron/v1.31
	$(CURL) "$(PHIVE)/$$($(URLENC) "$(BE_RULESRC)/en16931/v1.31/GLOBALUBL.BE.sch")" -o testdata/cius-be/schematron/v1.31/GLOBALUBL.BE.sch
	@# SRBDT (Serbian Ministry of Finance). The -pdvcat-* files are the per-VAT-
	@# category halves of one rule set and only -gen carries assertions, but all
	@# five are fetched: which of them is empty is upstream's business and not a
	@# fact this fetch should bake in.
	mkdir -p testdata/cius-rs/schematron/1.0.0
	for f in EN16931-UBL-srbdt.sch EN16931-UBL-srbdt-validation.sch \
		EN16931-UBL-srbdt-pdvcat-gen.sch EN16931-UBL-srbdt-pdvcat-n.sch \
		EN16931-UBL-srbdt-pdvcat-oe.sch EN16931-UBL-srbdt-pdvcat-r.sch \
		EN16931-UBL-srbdt-pdvcat-ss.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(RS_RULESRC)/1.0.0/$$f")" -o "testdata/cius-rs/schematron/1.0.0/$$f"; \
	done
	@# NLCIUS (Dutch SimplerInvoicing), both bindings. This is the one CIUS of the
	@# five that genuinely publishes a CII binding as well as a UBL one, and the two
	@# do not publish the same identifiers or gate them the same way, which is the
	@# fact a single-engine implementation is most likely to get wrong.
	mkdir -p testdata/nlcius/schematron/ubl testdata/nlcius/schematron/cii
	for f in si-ubl-2.0.3.2.sch si-ubl-2.0/si-ubl-2.0-nlcius.sch si-ubl-2.0-ext-gaccount-1.0.2.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(NL_RULESRC)/simplerinvoicing/2.0.3.2/$$f")" -o "testdata/nlcius/schematron/ubl/$$(basename "$$f")"; \
	done
	for f in nlcius-cii-1.0.3.sch nlcius-cii/NLCIUS-CII-validation.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(NL_RULESRC)/nlcius/1.0.3/$$f")" -o "testdata/nlcius/schematron/cii/$$(basename "$$f")"; \
	done
	@# NLCIUS's copy of CEN's files. si-ubl-2.0.3.2.sch <include>s them from a
	@# CenPC434/ directory beside it and nlcius-cii-1.0.3.sch from one of its own, and
	@# neither was fetched — so whether SimplerInvoicing ships CEN's rules verbatim,
	@# an older release of them, or an edited copy was unmeasured. It is measurable
	@# only by reading the copy. EN16931-syntax-modified.sch is the G-account
	@# extension's own replacement for CEN's abstract syntax file, and the name says
	@# what it is.
	mkdir -p testdata/nlcius/schematron/ubl/cen testdata/nlcius/schematron/cii/cen
	for f in si-ubl-2.0/CenPC434/abstract/EN16931-model.sch \
		si-ubl-2.0/CenPC434/abstract/EN16931-syntax.sch \
		si-ubl-2.0/CenPC434/UBL/EN16931-UBL-model.sch \
		si-ubl-2.0/CenPC434/UBL/EN16931-UBL-syntax.sch \
		si-ubl-2.0-ext-gaccount/EN16931-syntax-modified.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(NL_RULESRC)/simplerinvoicing/2.0.3.2/$$f")" -o "testdata/nlcius/schematron/ubl/cen/$$(basename "$$f")"; \
	done
	@# The CII half keeps its two directories: abstract/EN16931-CII-model.sch and
	@# CII/EN16931-CII-model.sch differ only in path, and flattening them to a
	@# basename would silently leave one copy of each pair on disk.
	mkdir -p testdata/nlcius/schematron/cii/cen/abstract testdata/nlcius/schematron/cii/cen/CII
	for f in abstract/EN16931-CII-model.sch abstract/EN16931-CII-syntax.sch \
		CII/EN16931-CII-model.sch CII/EN16931-CII-syntax.sch; do \
		$(CURL) "$(PHIVE)/$$($(URLENC) "$(NL_RULESRC)/nlcius/1.0.3/nlcius-cii/CenPC434/$$f")" -o "testdata/nlcius/schematron/cii/cen/$$f"; \
	done
	@# The fetch-side ratchet. Every file above is fetched with curl -f under
	@# pipefail, so a missing one has already failed the recipe; this catches the
	@# case a per-file check cannot see — a list that was edited down.
	@n=$$(find testdata/cius-pt/schematron testdata/cius-ro/schematron testdata/cius-be/schematron \
		testdata/cius-rs/schematron testdata/nlcius/schematron -name '*.sch' | wc -l); \
	if [ "$$n" -lt 50 ]; then \
		echo "make: fetched only $$n CIUS Schematron files, expected at least 50" >&2; \
		exit 1; \
	fi; \
	echo "make: vendored $$n CIUS Schematron files"
	touch $@
# Factur-X 1.09 / ZUGFeRD 2.5 profile Schematrons.
#
# Factur-X binds EN 16931 with its own rule set rather than adopting CEN's CII
# syntax binding, so the two disagree about which rules a Factur-X document is
# judged by: EXTENDED publishes one CII-DT-* rule (CII-DT-097) where CEN's
# binding has 101, and it adds 50 BR-FXEXT-* rules of its own that CEN has no
# counterpart for. Reading that from the artefact is the only way to know it.
#
# FNFE publishes these inside a ~33 MB specification bundle that is not
# individually addressable, so they are fetched from mustangproject, which
# vendors the same five files. Checked: its EXTENDED copy and FNFE's agree
# exactly — 217 identifiers over 1458 assertions, same set both directions.
#
# Note for the reader: these Schematrons carry no `id` attribute. A rule is
# identified by an "[ID]-" prefix on its message text, so an enumeration written
# the way CEN's, KoSIT's and OpenPEPPOL's are written finds nothing here.
MUSTANG := https://raw.githubusercontent.com/ZUGFeRD/mustangproject/master/validator/src/main/resources/schematron/ZF_250

facturx-schematron: $(FACTURX_SCH_STAMP)
$(FACTURX_SCH_STAMP): | check-deps
	mkdir -p testdata/facturx/schematron
	@# Both halves of each profile, because they are one artefact. 366 of the
	@# 2,159 data-model assertions are a lookup in FACTUR-X_<PROFILE>_codedb.xml —
	@# `document('FACTUR-X_EN16931_codedb.xml')/codedb/cl[@id=23]/enumeration` — and
	@# a .sch fetched without its code database is a rule set a sixth of which
	@# cannot be evaluated. mustangproject carries the two side by side, which is
	@# what makes the code-list tier implementable at all; PR 57 recorded these
	@# files as obtainable only inside FNFE's ~33 MB specification bundle.
	for f in MINIMUM BASIC-WL BASIC EN16931 EXTENDED; do \
		$(CURL) "$(MUSTANG)/FACTUR-X_$$f.sch" -o "testdata/facturx/schematron/FACTUR-X_$$f.sch"; \
		$(CURL) "$(MUSTANG)/FACTUR-X_$${f}_codedb.xml" -o "testdata/facturx/schematron/FACTUR-X_$${f}_codedb.xml"; \
	done
	@# The fetch-side ratchet, as for the CIUS Schematrons above: curl -f under
	@# pipefail has already failed on a missing file, so this catches only an
	@# edited-down list.
	@n=$$(find testdata/facturx/schematron -name '*.sch' | wc -l); \
	c=$$(find testdata/facturx/schematron -name '*_codedb.xml' | wc -l); \
	if [ "$$n" -ne 5 ] || [ "$$c" -ne 5 ]; then \
		echo "make: fetched $$n Factur-X Schematron files and $$c code databases, expected 5 of each" >&2; \
		exit 1; \
	fi; \
	echo "make: vendored $$n Factur-X profile Schematron files and $$c code databases"
	touch $@

# The Factur-X profile data model, generated from the five Schematrons and the
# five code databases into facturx_datamodel_table.go. Same arrangement as the
# four generator targets above — the generator is a deliberate act, and the tests
# that re-derive the table from the artefact run on every `make test` — with one
# difference worth stating: this generator is tracked Go under internal/ rather
# than a gen.py under testdata/. All five generators in this repository are
# tracked; this one is Go so that `gofmt -l .` and `go vet ./...` reach the
# program that decides what 2,159 fatal assertions mean.
#
# The test run at the end is not belt-and-braces. The generator refuses an
# assertion outside the six shapes it can express and renders every decomposition
# back to XPath before emitting it; the package refuses to build an index over a
# table row it cannot read. The tests are the third gate and the only one that
# makes each of the 2,159 assertions report on a document written to break it.
facturx-datamodel: facturx-schematron
	go run ./internal/gen/facturx
	go test -count=1 -run 'TestFacturXDataModel|TestFacturXCodeLists|TestEveryFacturXDataModelAssertionFires' .
# Factur-X example invoices and the authority's own validation reports.
#
# These are not fetchable individually. FNFE-MPE publishes them inside the
# Factur-X 1.09 / ZUGFeRD 2.5 specification bundle (a ~33 MB zip, downloaded
# from fnfe-mpe.org / ferd-net.de), and ZUGFeRD/corpus on GitHub carries only
# the EN 16931 subset as bare XML — 3 EXTENDED documents against the bundle's
# 25, which is the profile the two rule sets actually disagree about.
#
# So this corpus is vendored from the bundle rather than fetched, and this
# target only reports whether it is present. The oracle tests skip without it,
# the way every other corpus-backed test here does; what they must not do is
# quietly pass on a subset (C8/C26), which is what the ratchets in
# corpus_test.go are for.
#
#   testdata/facturx/examples/  bare CII invoice XML, one per published example
#   testdata/facturx/reports/   *_fx_validation_report.xml, FNFE's own verdict
#                               per document — the same class of oracle as
#                               KoSIT's <?xmute?> fixtures and OpenPEPPOL's
#                               unit-test sets
facturx-examples:
	@e=$$(ls testdata/facturx/examples/*.xml 2>/dev/null | wc -l); \
	r=$$(ls testdata/facturx/reports/*.xml 2>/dev/null | wc -l); \
	if [ "$$e" -eq 0 ]; then \
		echo "make: no Factur-X examples under testdata/facturx/examples/" >&2; \
		echo "  unpack the Factur-X 1.09 / ZUGFeRD 2.5 bundle and copy the example" >&2; \
		echo "  CII XML there; *_fx_validation_report.xml go in testdata/facturx/reports/" >&2; \
		exit 1; \
	fi; \
	echo "make: $$e Factur-X example invoices, $$r validation reports"

clean-facturx-schematron:
	rm -rf testdata/facturx/schematron $(FACTURX_SCH_STAMP)

clean-cius-oracles:
	rm -rf testdata/xrechnung testdata/peppol testdata/nlcius testdata/cius-pt testdata/cius-ro testdata/cius-be testdata/cius-rs testdata/fatturapa testdata/facturae testdata/ebinterface testdata/ksef testdata/finvoice testdata/zatca testdata/svefaktura testdata/teapps testdata/oioubl testdata/turkey testdata/osa testdata/pint $(CIUS_STAMP) $(CIUS_SCH_STAMP)
