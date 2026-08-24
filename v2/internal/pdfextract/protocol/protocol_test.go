package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func TestResultFrameIsCanonical(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := WriteResult(&output, Result{Text: "x", Pages: 1}); err != nil {
		t.Fatal(err)
	}
	want := []byte("MWPDF1\npages=1\nbytes=1\nx")
	if !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("result frame = %q, want %q", output.Bytes(), want)
	}
	if result, err := DecodeResult(want); err != nil || result != (Result{Text: "x", Pages: 1}) {
		t.Fatalf("DecodeResult = %#v, %v", result, err)
	}
}

func TestDecodeResultRejectsMalformedFrames(t *testing.T) {
	t.Parallel()
	for _, frame := range [][]byte{
		[]byte("wrong\npages=1\nbytes=1\nx"),
		[]byte("MWPDF1\npages=0\nbytes=1\nx"),
		[]byte("MWPDF1\npages=1\nbytes=2\nx"),
		[]byte("MWPDF1\npages=1\nbytes=1\n\xff"),
	} {
		if _, err := DecodeResult(frame); !errors.Is(err, ErrHelperProtocol) {
			t.Fatalf("frame %q error = %v, want ErrHelperProtocol", frame, err)
		}
	}
}

func TestDecodeProbeRequiresOneCanonicalBoundedFrame(t *testing.T) {
	t.Parallel()
	want := []byte("MWPDF-PROBE/1\nhelper=mindweaver-pdf\nextract=MWPDF1\n")
	if !bytes.Equal(probeResponse, want) {
		t.Fatalf("probe frame = %q, want %q", probeResponse, want)
	}
	if err := DecodeProbe(probeResponse); err != nil {
		t.Fatalf("canonical probe: %v", err)
	}
	for _, test := range []struct {
		name  string
		frame []byte
	}{
		{name: "protocol alias", frame: []byte("MWPDF-PROBE/01\nhelper=mindweaver-pdf\nextract=MWPDF1\n")},
		{name: "field alias", frame: []byte("MWPDF-PROBE/1\nname=mindweaver-pdf\nextract=MWPDF1\n")},
		{name: "duplicate", frame: []byte("MWPDF-PROBE/1\nhelper=mindweaver-pdf\nhelper=mindweaver-pdf\nextract=MWPDF1\n")},
		{name: "trailing", frame: append(append([]byte(nil), probeResponse...), []byte("trailing")...)},
		{name: "extra newline", frame: append(append([]byte(nil), probeResponse...), '\n')},
		{name: "over limit", frame: bytes.Repeat([]byte("x"), MaxProbeResponseBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := DecodeProbe(test.frame); !errors.Is(err, ErrHelperProtocol) {
				t.Fatalf("DecodeProbe error = %v, want ErrHelperProtocol", err)
			}
		})
	}
}

func TestExitCodeRoundTripIsClosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		err  error
		code int
	}{
		{err: ErrInvalidPDF, code: 20},
		{err: ErrEncryptedPDF, code: 21},
		{err: ErrResourceLimit, code: 22},
		{err: ErrNoExtractedText, code: 23},
	} {
		if code := ExitCode(test.err); code != test.code {
			t.Errorf("ExitCode(%v) = %d, want %d", test.err, code, test.code)
		}
		if got := ErrorForExitCode(test.code); !errors.Is(got, test.err) {
			t.Errorf("ErrorForExitCode(%d) = %v, want %v", test.code, got, test.err)
		}
	}
	if ExitCode(errors.New("unknown")) != 1 || ErrorForExitCode(1) != nil {
		t.Fatal("unknown failure was classified as a controlled parser category")
	}
}
