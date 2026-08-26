package formalis

import (
	"context"
	"strings"
)

// This file validates the Hungarian NAV Online Számla (OSA) invoice-data format —
// the real-time invoice report submitted to the Hungarian tax authority (root
// InvoiceData). It is XSD-validated rather than rule-validated, so this checks the
// mandatory structure directly against the parsed tree.
//
// Rule identifiers are HU-* (this package's own). Not vendored: the OSA sample
// instances (phax/phive-rules) are used only as the oracle.

// firstNonEmptyText returns the first non-empty trimmed text among the nodes.
func firstNonEmptyText(ns []*ciiNode) string {
	for _, n := range ns {
		if t := strings.TrimSpace(n.text); t != "" {
			return t
		}
	}
	return ""
}

// IsOSA reports whether the XML is a Hungarian OSA InvoiceData document.
//
// A non-nil error means the document could not be read — malformed XML, an
// unsupported character encoding, or a guard that tripped — and the bool is
// meaningless. It is distinct from (false, nil), which says the document was
// read and is some other format.
//
// The Is* predicates are independent tests, not a partition: several of them key
// on a root element name that four national formats, seven CIUS and the EN 16931
// UBL binding all share, and more than one can report true about one document.
// This one keys on a root no other format claims, so nothing overlaps it today.
// Detect owns the precedence for the whole set and returns a single answer; route
// with it.
func IsOSA(xmlData []byte) (bool, error) {
	d, err := detectShape(xmlData)
	if err != nil {
		return false, err
	}
	return d.root == "InvoiceData", nil
}

// ValidateOSA validates a Hungarian OSA invoice-data document against its
// mandatory structure.
//
// ctx bounds how long the call may take; the work itself is bounded by this
// package's own limits. A cancelled run reports a RuleLimit violation rather
// than an empty Violations slice, so a run that stopped early cannot be read
// as a clean invoice-data report.
//
// This validator checks the mandatory structure and code lists rather than the
// whole schema its authority publishes, so the Report is never Conformant even
// for a document with no findings: Report.NotEvaluated, from Coverage(SourceOSA),
// says what was not checked.
func ValidateOSA(ctx context.Context, xmlData []byte) (Report, error) {
	return osaValidator.validate(ctx, xmlData)
}

var osaValidator = treeValidator{
	source:   SourceOSA,
	rootRule: "HU-root",
	rootMsg:  "the document root shall be InvoiceData",
	accepts:  rootNamed("InvoiceData"),
	check:    checkOSA,
}

func checkOSA(root *ciiNode, add func(rule, msg string)) {
	if strings.TrimSpace(root.str("invoiceNumber")) == "" {
		add("HU-number", "the document shall contain an invoiceNumber")
	}
	if strings.TrimSpace(root.str("invoiceIssueDate")) == "" {
		add("HU-date", "the document shall contain an invoiceIssueDate")
	}
	if firstNonEmptyText(root.findAll("supplierName")) == "" {
		add("HU-supplier-name", "the supplier information shall contain a supplierName")
	}
	if firstNonEmptyText(root.findAll("taxpayerId")) == "" {
		add("HU-supplier-tax", "the supplier information shall contain a tax number (supplierTaxNumber/taxpayerId)")
	}
	if len(root.findAll("customerInfo")) == 0 {
		add("HU-customer", "the invoice shall contain customerInfo")
	}
}
