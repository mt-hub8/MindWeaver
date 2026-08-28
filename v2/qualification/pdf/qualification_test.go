package pdfqualification_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/mgilbir/pdf0"
	pdfclient "github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/client"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
)

const (
	pdfTextLayerSchema         = "mindweaver.qualification.pdf-text-layer/v1"
	qualificationCIDFontObject = "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 0 >> /DW 1000 /FontDescriptor << /Type /FontDescriptor /FontName /STSongStd-Light /Flags 6 /FontBBox [-25 -254 1000 880] /ItalicAngle 0 /Ascent 752 /Descent -271 /CapHeight 737 /StemV 58 /MissingWidth 500 >> /W [1 [207 270 342 467 462 797 710 239 374] 10 [374 423 605 238 375 238 334 462] 18 26 462 27 28 238 29 31 605 32 [344 748 684 560 695 739 563 511 729 793 318 312 666 526 896 758 772 544 772 628 465 607 753 711 972 647 620 607 374 333 374 606 500 239 417 503 427 529 415 264 444 518 241 230 495 228 793 527 524] 81 [524 504 338 336 277 517 450 652 466 452 407 370 258 370 605]] >>"
	readyEnvironment           = "MWQ_PDF_PROBE_READY"
	sentinelEnvironment        = "MWQ_PDF_PROBE_SENTINEL"
	sentinelDelayEnvironment   = "MWQ_PDF_PROBE_SENTINEL_DELAY_MS"
)

type pdfTextLayerSpec struct {
	SchemaVersion              string    `json:"schema_version"`
	DocumentID                 string    `json:"document_id"`
	MediaBox                   [4]int    `json:"media_box"`
	ExpectedGeneratedPDFSHA256 string    `json:"expected_generated_pdf_sha256"`
	Pages                      []pdfPage `json:"pages"`
}

type pdfPage struct {
	Lines []string `json:"lines"`
}

func TestProductionPDFQualificationMatrix(t *testing.T) {
	helper := buildPackage(t, "./cmd/mindweaver-pdf", "mindweaver-pdf")
	client, err := pdfclient.New(helper, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	spec := loadPDFSpec(t)
	representative, err := generateUniGBPDF(spec)
	if err != nil {
		t.Fatal(err)
	}
	if digest(representative) != spec.ExpectedGeneratedPDFSHA256 {
		t.Fatal("representative UniGB PDF digest drifted from the committed corpus spec")
	}
	root := t.TempDir()
	representativePath := writeFile(t, root, "representative-unigb.pdf", representative)

	t.Run("representative UniGB exact", func(t *testing.T) {
		result, extractErr := client.Extract(t.Context(), representativePath)
		exact := extractErr == nil && result.Pages == len(spec.Pages) && normalize(result.Text) == expectedText(spec)
		if !exact {
			t.Fatalf("representative UniGB extraction = category %s, pages %d, text %q", safeCategory(extractErr), result.Pages, normalize(result.Text))
		}
	})

	t.Run("UniGB identity is fail closed", func(t *testing.T) {
		spoofed := bytes.Replace(representative, []byte("/Ordering (GB1)"), []byte("/Ordering (XX1)"), 1)
		if bytes.Equal(spoofed, representative) {
			t.Fatal("qualification fixture no longer exposes CIDSystemInfo")
		}
		path := writeFile(t, root, "unigb-spoofed-ordering.pdf", spoofed)
		if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrInvalidPDF) {
			t.Fatalf("spoofed UniGB category = %s", safeCategory(err))
		}
	})

	t.Run("explicit ToUnicode CMap", func(t *testing.T) {
		data, err := generateToUnicodePDF(spec, len(spec.Pages))
		if err != nil {
			t.Fatal(err)
		}
		path := writeFile(t, root, "explicit-tounicode.pdf", data)
		result, extractErr := client.Extract(t.Context(), path)
		if extractErr != nil || result.Pages != len(spec.Pages) || normalize(result.Text) != expectedText(spec) {
			t.Fatalf("ToUnicode extraction = category %s, pages %d, text %q", safeCategory(extractErr), result.Pages, normalize(result.Text))
		}
	})

	t.Run("Flate content and CMap", func(t *testing.T) {
		data, err := generateFlateToUnicodePDF(spec)
		if err != nil {
			t.Fatal(err)
		}
		path := writeFile(t, root, "flate-tounicode.pdf", data)
		result, extractErr := client.Extract(t.Context(), path)
		if extractErr != nil || result.Pages != len(spec.Pages) || normalize(result.Text) != expectedText(spec) {
			t.Fatalf("Flate extraction = category %s, pages %d, text %q", safeCategory(extractErr), result.Pages, normalize(result.Text))
		}
	})

	t.Run("multiple pages", func(t *testing.T) {
		const want = "English page one\n\n中文第二页"
		path := writeFile(t, root, "multiple-pages.pdf", generateMultiPageUniGBPDF([]string{"English page one", "中文第二页"}))
		result, extractErr := client.Extract(t.Context(), path)
		if extractErr != nil || result.Pages != 2 || normalize(result.Text) != want {
			t.Fatalf("multipage extraction = category %s, pages %d, text %q", safeCategory(extractErr), result.Pages, normalize(result.Text))
		}
	})

	t.Run("object and xref streams", func(t *testing.T) {
		const want = "对象流中文"
		path := writeFile(t, root, "object-xref-stream.pdf", generateObjectXRefStreamPDF(t, want))
		result, extractErr := client.Extract(t.Context(), path)
		if extractErr != nil || result.Pages != 1 || normalize(result.Text) != want {
			t.Fatalf("object/xref stream extraction = category %s, pages %d, text %q", safeCategory(extractErr), result.Pages, normalize(result.Text))
		}
	})

	t.Run("Form XObject text", func(t *testing.T) {
		const want = "表单对象中文"
		path := writeFile(t, root, "form-xobject.pdf", generateUniGBFormPDF(want))
		result, extractErr := client.Extract(t.Context(), path)
		if extractErr != nil || result.Pages != 1 || normalize(result.Text) != want {
			t.Fatalf("Form extraction = category %s, pages %d, text %q", safeCategory(extractErr), result.Pages, normalize(result.Text))
		}
	})

	t.Run("English text", func(t *testing.T) {
		const text = "English calibration torque sensor 12.5"
		path := writeFile(t, root, "english.pdf", generateSimpleEnglishPDF(text))
		result, err := client.Extract(t.Context(), path)
		if err != nil || result.Pages != 1 || normalize(result.Text) != text {
			t.Fatalf("English extraction = pages %d, category %s", result.Pages, safeCategory(err))
		}
	})

	t.Run("empty and no text", func(t *testing.T) {
		empty := writeFile(t, root, "empty.pdf", nil)
		if _, err := client.Extract(t.Context(), empty); !errors.Is(err, protocol.ErrInvalidPDF) {
			t.Fatalf("empty source category = %s", safeCategory(err))
		}
		noText := writeFile(t, root, "no-text.pdf", generateSimpleEnglishPDF(""))
		if _, err := client.Extract(t.Context(), noText); !errors.Is(err, protocol.ErrNoExtractedText) {
			t.Fatalf("no-text category = %s", safeCategory(err))
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		const protectedText = "protected text"
		const password = "secret"
		encrypted := writeFile(t, root, "encrypted.pdf", generateEncryptedEnglishPDF(protectedText, password))
		assertEncryptedFixture(t, encrypted, password)
		if _, err := client.Extract(t.Context(), encrypted); !errors.Is(err, protocol.ErrEncryptedPDF) {
			t.Fatalf("encrypted category = %s", safeCategory(err))
		}
	})

	t.Run("malformed", func(t *testing.T) {
		valid := generateSimpleEnglishPDF("malformed matrix")
		xref := bytes.LastIndex(valid, []byte("xref\n"))
		if xref < 0 {
			t.Fatal("fixture has no xref")
		}
		broken := bytes.Replace(valid, []byte("4 0 obj\n"), []byte("4 0 bad\n"), 1)
		cases := map[string][]byte{
			"invalid-signature": []byte("NOTPDF-1.7\nsource"),
			"truncated-xref":    append([]byte(nil), valid[:xref+5]...),
			"broken-object":     broken,
		}
		for name, data := range cases {
			t.Run(name, func(t *testing.T) {
				path := writeFile(t, root, name+".pdf", data)
				if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrInvalidPDF) {
					t.Fatalf("malformed category = %s", safeCategory(err))
				}
			})
		}
	})

	t.Run("declared page count", func(t *testing.T) {
		bomb, err := generateToUnicodePDF(spec, protocol.MaxPages+1)
		if err != nil {
			t.Fatal(err)
		}
		path := writeFile(t, root, "page-count.pdf", bomb)
		if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrResourceLimit) {
			t.Fatalf("page-count category = %s", safeCategory(err))
		}
	})

	t.Run("actual page count", func(t *testing.T) {
		path := writeFile(t, root, "actual-page-count.pdf", generateManyPagePDF(protocol.MaxPages+1))
		if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrResourceLimit) {
			t.Fatalf("actual page-count category = %s", safeCategory(err))
		}
	})

	t.Run("decoded stream and operand limits", func(t *testing.T) {
		for _, test := range []struct {
			name string
			data []byte
		}{
			{name: "output-allocation", data: generateCompressedTextOutputBomb(9 << 20)},
			{name: "operand-stack", data: generateCompressedContentPDF(bytes.Repeat([]byte("0 "), 4097))},
		} {
			t.Run(test.name, func(t *testing.T) {
				path := writeFile(t, root, test.name+".pdf", test.data)
				if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrResourceLimit) {
					t.Fatalf("%s category = %s", test.name, safeCategory(err))
				}
			})
		}
	})

	t.Run("repeated Form expansion", func(t *testing.T) {
		path := writeFile(t, root, "repeated-form-expansion.pdf", generateRepeatedFormOutputBomb(1<<20, 400))
		if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrResourceLimit) {
			t.Fatalf("repeated Form category = %s", safeCategory(err))
		}
	})

	t.Run("source size", func(t *testing.T) {
		path := filepath.Join(root, "oversize.pdf")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		truncateErr := file.Truncate(protocol.MaxSourceBytes + 1)
		closeErr := file.Close()
		if truncateErr != nil || closeErr != nil {
			t.Fatal("materialize oversize source")
		}
		if _, err := client.Extract(t.Context(), path); !errors.Is(err, protocol.ErrResourceLimit) {
			t.Fatalf("oversize category = %s", safeCategory(err))
		}
	})

	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := client.Extract(ctx, representativePath); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel category = %s", safeCategory(err))
		}
	})
}

func assertEncryptedFixture(t *testing.T, path, password string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	document, err := pdf0.ReadWithPasswordContext(t.Context(), file, info.Size(), password)
	if err != nil {
		t.Fatalf("encrypted fixture did not open with its qualification password: %T", err)
	}
	if !document.Encrypted || document.Locked() {
		t.Fatal("encrypted fixture did not exercise an unlocked standard-security document")
	}
}

func TestPDFHelperTimeoutQualification(t *testing.T) {
	const (
		helperStartupBudget    = 2 * time.Second
		helperTimeout          = 3 * time.Second
		helperReapGrace        = 2 * time.Second
		sentinelDelay          = 3100 * time.Millisecond
		sentinelScheduleMargin = 500 * time.Millisecond
	)
	if helperStartupBudget >= helperTimeout || helperTimeout >= sentinelDelay ||
		helperReapGrace <= 0 || sentinelScheduleMargin <= 0 {
		t.Fatal("invalid PDF timeout qualification timing contract")
	}
	probe := buildPackage(t, "./qualification/pdf/adversarialprobe", "pdf-adversarial-probe")
	root := t.TempDir()
	source := writeFile(t, root, "source.pdf", []byte("%PDF-1.7\nqualification probe"))
	ready := filepath.Join(root, "ready")
	sentinel := filepath.Join(root, "must-not-survive")
	t.Setenv(readyEnvironment, ready)
	t.Setenv(sentinelEnvironment, sentinel)
	t.Setenv(sentinelDelayEnvironment, fmt.Sprintf("%d", sentinelDelay/time.Millisecond))
	client, err := pdfclient.New(probe, helperTimeout)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	callStarted := make(chan time.Time, 1)
	go func() {
		callStarted <- time.Now()
		_, extractErr := client.Extract(ctx, source)
		done <- extractErr
	}()
	started := <-callStarted
	startupDeadline := started.Add(helperStartupBudget)
	var readyObservedAt time.Time
	for readyObservedAt.IsZero() {
		if _, statErr := os.Lstat(ready); statErr == nil {
			readyObservedAt = time.Now()
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			cancel()
			select {
			case <-done:
			case <-time.After(helperReapGrace):
				t.Fatal("qualification helper startup failure did not reap the process")
			}
			t.Fatal(statErr)
		}
		select {
		case extractErr := <-done:
			t.Fatalf("qualification helper terminated before ready: %s", safeCategory(extractErr))
		default:
		}
		if !time.Now().Before(startupDeadline) {
			cancel()
			select {
			case <-done:
			case <-time.After(helperReapGrace):
				t.Fatal("qualification helper startup cancellation did not reap the process")
			}
			t.Fatal("qualification helper did not start within the bounded startup window")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reapDeadline := started.Add(helperTimeout + helperReapGrace)
	reapWait := time.Until(reapDeadline)
	if reapWait <= 0 {
		t.Fatal("timed-out helper exceeded its bounded reap window")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout category = %s", safeCategory(err))
		}
	case <-time.After(reapWait):
		t.Fatal("timed-out helper was not reaped")
	}
	if observationWait := time.Until(readyObservedAt.Add(sentinelDelay + sentinelScheduleMargin)); observationWait > 0 {
		time.Sleep(observationWait)
	}
	if _, err := os.Lstat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("timed-out helper survived: %v", err)
	}
}

func TestPDFQualificationDependencyAndLicenseEvidence(t *testing.T) {
	root := moduleRoot(t)
	goTool := goTool(t)
	command := exec.Command(goTool, "list", "-deps", "-f", "{{if and (not .Standard) .Module}}{{.Module.Path}}@{{.Module.Version}}{{end}}", "./internal/pdfextract/parser")
	command.Dir = root
	command.Env = hermeticEnvironment(runtime.GOOS, runtime.GOARCH)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, line := range strings.Fields(string(output)) {
		got[line] = true
	}
	for _, want := range []string{
		"github.com/mt-hub8/MindWeaver/v2@",
		"github.com/mgilbir/pdf0@v0.1.0",
		"github.com/mgilbir/formalis@v0.3.1",
		"github.com/mgilbir/gopenjpeg@v0.0.0-20260727163526-8a139bc479b2",
		"github.com/mgilbir/golittlecms@v0.0.0-20260727161601-f6af7cfe1556",
	} {
		if !got[want] {
			t.Errorf("selected parser dependency closure is missing %s", want)
		}
	}
	if len(got) != 5 {
		t.Fatalf("selected parser dependency closure = %v, want exactly five modules including MindWeaver", got)
	}

	licenses := map[string]string{
		filepath.Join(root, "vendor", "github.com", "mgilbir", "pdf0", "LICENSE"):        "4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a",
		filepath.Join(root, "vendor", "github.com", "mgilbir", "formalis", "LICENSE"):    "4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a",
		filepath.Join(root, "vendor", "github.com", "mgilbir", "gopenjpeg", "LICENSE"):   "958dc940b3916ca8b4d373f24027e26e29623828f41205de09e9c680e5539f78",
		filepath.Join(root, "vendor", "github.com", "mgilbir", "golittlecms", "LICENSE"): "4b0b89edd67872e0507e20e03032e4dc4eb194f88082f80acee13a13fb73317c",
	}
	for path, hash := range licenses {
		assertFileSHA256(t, path, hash)
	}
}

func safeCategory(err error) string {
	switch {
	case err == nil:
		return "NONE"
	case errors.Is(err, protocol.ErrInvalidPDF):
		return "INVALID_PDF"
	case errors.Is(err, protocol.ErrEncryptedPDF):
		return "ENCRYPTED_PDF"
	case errors.Is(err, protocol.ErrResourceLimit):
		return "RESOURCE_LIMIT"
	case errors.Is(err, protocol.ErrHelperProtocol):
		return "HELPER_PROTOCOL"
	case errors.Is(err, pdfclient.ErrHelperFailed):
		return "HELPER_FAILED"
	case errors.Is(err, protocol.ErrNoExtractedText):
		return "NO_EXTRACTED_TEXT"
	case errors.Is(err, context.DeadlineExceeded):
		return "DEADLINE"
	default:
		return "UNCLASSIFIED"
	}
}

func loadPDFSpec(t *testing.T) pdfTextLayerSpec {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "testdata", "qualification", "pdf", "text-layer-cn-mixed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec pdfTextLayerSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatal("qualification spec contains trailing data")
	}
	if spec.SchemaVersion != pdfTextLayerSchema || len(spec.Pages) != 1 || len(spec.Pages[0].Lines) == 0 {
		t.Fatal("qualification spec is outside the frozen schema")
	}
	return spec
}

func expectedText(spec pdfTextLayerSpec) string {
	pages := make([]string, 0, len(spec.Pages))
	for _, page := range spec.Pages {
		pages = append(pages, strings.Join(page.Lines, "\n"))
	}
	return normalize(strings.Join(pages, "\n\n"))
}

func normalize(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
}

func generateUniGBPDF(spec pdfTextLayerSpec) ([]byte, error) {
	if len(spec.Pages) != 1 {
		return nil, errors.New("qualification UniGB PDF requires one page")
	}
	var content strings.Builder
	content.WriteString("BT\n/F0 11 Tf\n72 790 Td\n")
	for index, line := range spec.Pages[0].Lines {
		if index > 0 {
			content.WriteString("0 -18 Td\n")
		}
		fmt.Fprintf(&content, "<%s> Tj\n", utf16BEHex(line))
	}
	content.WriteString("ET\n")
	mediaBox := fmt.Sprintf("[%d %d %d %d]", spec.MediaBox[0], spec.MediaBox[1], spec.MediaBox[2], spec.MediaBox[3])
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox %s /Resources << /Font << /F0 5 0 R >> >> /Contents 4 0 R >>", mediaBox),
		pdfStream([]byte(content.String())),
		"<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [6 0 R] >>",
		qualificationCIDFontObject,
	}
	return serializePDF(objects, ""), nil
}

func generateToUnicodePDF(spec pdfTextLayerSpec, declaredPages int) ([]byte, error) {
	if len(spec.Pages) != 1 || declaredPages < 1 {
		return nil, errors.New("qualification ToUnicode PDF requires one page")
	}
	var content strings.Builder
	for index, line := range spec.Pages[0].Lines {
		fmt.Fprintf(&content, "BT\n/F0 11 Tf\n72 %d Td\n<%s> Tj\nET\n", 790-index*18, utf16BEHex(line))
	}
	mediaBox := fmt.Sprintf("[%d %d %d %d]", spec.MediaBox[0], spec.MediaBox[1], spec.MediaBox[2], spec.MediaBox[3])
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [3 0 R] /Count %d >>", declaredPages),
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox %s /Resources << /Font << /F0 5 0 R >> >> /Contents 4 0 R >>", mediaBox),
		pdfStream([]byte(content.String())),
		"<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
		qualificationCIDFontObject,
		pdfStream([]byte(toUnicodeCMap(spec.Pages[0].Lines))),
	}
	return serializePDF(objects, ""), nil
}

func generateSimpleEnglishPDF(text string) []byte {
	escaped := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(text)
	content := "BT /F1 12 Tf 72 720 Td (" + escaped + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		pdfStream([]byte(content)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	return serializePDF(objects, "")
}

func generateEncryptedEnglishPDF(text, userPassword string) []byte {
	const permissions int32 = -4
	fileID := md5.Sum([]byte("MindWeaver encrypted qualification fixture"))
	owner := standardOwnerEntry("owner", userPassword)
	key := standardFileKey(userPassword, owner, permissions, fileID[:])
	user := standardUserEntry(key)
	content := []byte("BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET")
	encryptedContent := cryptObject(content, key, 4, 0)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		pdfStream(encryptedContent),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Filter /Standard /V 1 /R 2 /O <%X> /U <%X> /P %d >>", owner, user, permissions),
	}
	trailer := fmt.Sprintf(" /Encrypt 6 0 R /ID [<%X><%X>]", fileID, fileID)
	return serializePDF(objects, trailer)
}

var standardPasswordPadding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func standardOwnerEntry(ownerPassword, userPassword string) []byte {
	digest := md5.Sum(padPassword(ownerPassword))
	return rc4Bytes(digest[:5], padPassword(userPassword))
}

func standardFileKey(userPassword string, owner []byte, permissions int32, fileID []byte) []byte {
	hash := md5.New()
	_, _ = hash.Write(padPassword(userPassword))
	_, _ = hash.Write(owner)
	var encodedPermissions [4]byte
	binary.LittleEndian.PutUint32(encodedPermissions[:], uint32(permissions))
	_, _ = hash.Write(encodedPermissions[:])
	_, _ = hash.Write(fileID)
	return hash.Sum(nil)[:5]
}

func standardUserEntry(key []byte) []byte {
	return rc4Bytes(key, standardPasswordPadding)
}

func padPassword(password string) []byte {
	result := make([]byte, 32)
	length := copy(result, []byte(password))
	if length < len(result) {
		copy(result[length:], standardPasswordPadding[:len(result)-length])
	}
	return result
}

func cryptObject(data, fileKey []byte, objectNumber, generation int) []byte {
	hash := md5.New()
	_, _ = hash.Write(fileKey)
	_, _ = hash.Write([]byte{byte(objectNumber), byte(objectNumber >> 8), byte(objectNumber >> 16), byte(generation), byte(generation >> 8)})
	digest := hash.Sum(nil)
	keyLength := len(fileKey) + 5
	if keyLength > 16 {
		keyLength = 16
	}
	return rc4Bytes(digest[:keyLength], data)
}

func rc4Bytes(key, data []byte) []byte {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		panic(err)
	}
	output := make([]byte, len(data))
	cipher.XORKeyStream(output, data)
	return output
}

func serializePDF(objects []string, trailerExtra string) []byte {
	var output bytes.Buffer
	output.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(offsets))
	output.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R%s >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), trailerExtra, xref)
	return output.Bytes()
}

func pdfStream(content []byte) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content)
}

func utf16BEHex(text string) string {
	var encoded strings.Builder
	for _, unit := range utf16.Encode([]rune(text)) {
		fmt.Fprintf(&encoded, "%04X", unit)
	}
	return encoded.String()
}

func toUnicodeCMap(lines []string) string {
	units := make(map[uint16]struct{})
	for _, line := range lines {
		for _, unit := range utf16.Encode([]rune(line)) {
			units[unit] = struct{}{}
		}
	}
	ordered := make([]int, 0, len(units))
	for unit := range units {
		ordered = append(ordered, int(unit))
	}
	sort.Ints(ordered)
	var cmap strings.Builder
	cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	cmap.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	cmap.WriteString("/CMapName /MindWeaverQualification-UCS def\n/CMapType 2 def\n")
	cmap.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	fmt.Fprintf(&cmap, "%d beginbfchar\n", len(ordered))
	for _, unit := range ordered {
		fmt.Fprintf(&cmap, "<%04X> <%04X>\n", unit, unit)
	}
	cmap.WriteString("endbfchar\nendcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return cmap.String()
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for depth := 0; depth < 8; depth++ {
		module, readErr := os.ReadFile(filepath.Join(current, "go.mod"))
		if readErr == nil && bytes.Contains(module, []byte("module github.com/mt-hub8/MindWeaver/v2")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	t.Fatal("locate v2 module root")
	return ""
}

func goTool(t *testing.T) string {
	t.Helper()
	if configured := strings.TrimSpace(os.Getenv("MW_GO")); configured != "" {
		absolute, err := filepath.Abs(configured)
		if err != nil {
			t.Fatal(err)
		}
		return absolute
	}
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(runtime.GOROOT(), "bin", name)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatal("qualification Go tool is unavailable")
	}
	return path
}

func buildPackage(t *testing.T, packagePath, baseName string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		baseName += ".exe"
	}
	output := filepath.Join(t.TempDir(), baseName)
	command := exec.Command(goTool(t), "build", "-trimpath", "-buildvcs=false", "-o", output, packagePath)
	command.Dir = moduleRoot(t)
	command.Env = hermeticEnvironment(runtime.GOOS, runtime.GOARCH)
	if buildOutput, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, buildOutput)
	}
	return output
}

func hermeticEnvironment(goos, goarch string) []string {
	overrides := map[string]string{
		"CGO_ENABLED": "0", "GOARCH": goarch, "GOAMD64": "v1", "GOOS": goos,
		"GOENV": "off", "GOEXPERIMENT": "", "GOFIPS140": "off", "GOFLAGS": "-mod=vendor -buildvcs=false",
		"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOTELEMETRY": "off", "GOVCS": "*:off", "GOWORK": "off",
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func writeFile(t *testing.T, directory, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertFileSHA256(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(data); got != want {
		t.Fatalf("license evidence %s SHA256 = %s, want %s", filepath.Base(path), got, want)
	}
}
