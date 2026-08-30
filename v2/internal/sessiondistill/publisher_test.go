package sessiondistill

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishBundlePublishesOneCompleteDirectoryWithoutOverwrite(t *testing.T) {
	if !atomicDirectoryPublicationSupported {
		t.Skip("atomic publication is not supported on this platform")
	}
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	if err := PublishBundle(context.Background(), destination, bundle); err != nil {
		t.Fatal(err)
	}
	assertPublishedBundle(t, destination, bundle)
	if err := PublishBundle(context.Background(), destination, bundle); CodeOf(err) != CodeDestinationExists {
		t.Fatalf("second publish err=%v code=%s", err, CodeOf(err))
	}
	assertPublishedBundle(t, destination, bundle)
	assertNoStagingDirectories(t, parent)
}

func TestPublishBundleFailuresBeforeRenameExposeNoPartialTarget(t *testing.T) {
	tests := []struct {
		name string
		ops  func(string) publishOps
	}{
		{
			name: "second write",
			ops: func(_ string) publishOps {
				calls := 0
				return publishOps{
					writeFile: func(path string, data []byte) error {
						calls++
						if calls == 2 {
							return errors.New("write fault")
						}
						return writeBundleFile(path, data)
					},
					syncDirectory: func(string) error { return nil },
					rename:        os.Rename,
				}
			},
		},
		{
			name: "staging sync",
			ops: func(_ string) publishOps {
				return publishOps{
					writeFile:     writeBundleFile,
					syncDirectory: func(string) error { return errors.New("sync fault") },
					rename:        os.Rename,
				}
			},
		},
		{
			name: "rename",
			ops: func(_ string) publishOps {
				return publishOps{
					writeFile:     writeBundleFile,
					syncDirectory: func(string) error { return nil },
					rename:        func(string, string) error { return errors.New("rename fault") },
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			destination := filepath.Join(parent, "ideas")
			bundle := testPublishedBundle(t)
			err := publishBundleWithOps(context.Background(), destination, bundle, test.ops(parent))
			if err == nil || CodeOf(err) != CodeOutputFailed {
				t.Fatalf("err=%v code=%s", err, CodeOf(err))
			}
			if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("partial target visible: %v", statErr)
			}
			assertNoStagingDirectories(t, parent)
		})
	}
}

func TestPublishBundleReportsPostRenameSyncAsUncertainButKeepsCompleteBundle(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	syncCalls := 0
	err := publishBundleWithOps(context.Background(), destination, bundle, publishOps{
		writeFile: writeBundleFile,
		syncDirectory: func(string) error {
			syncCalls++
			if syncCalls == 2 {
				return errors.New("parent sync fault")
			}
			return nil
		},
		rename: os.Rename,
	})
	if CodeOf(err) != CodePublicationUncertain {
		t.Fatalf("err=%v code=%s", err, CodeOf(err))
	}
	assertPublishedBundle(t, destination, bundle)
	assertNoStagingDirectories(t, parent)
}

func TestPublishBundleCancellationBeforeWorkIsZeroWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	err := PublishBundle(ctx, destination, testPublishedBundle(t))
	if CodeOf(err) != CodeCanceled {
		t.Fatalf("err=%v code=%s", err, CodeOf(err))
	}
	if entries, readErr := os.ReadDir(parent); readErr != nil || len(entries) != 0 {
		t.Fatalf("canceled publish wrote files: entries=%v err=%v", entries, readErr)
	}
}

func TestPublishBundleRejectsForgedOrMutatedBundle(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	forged := Bundle{json: []byte("{}\n"), markdown: []byte("# report\n")}
	if err := PublishBundle(context.Background(), destination, forged); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("forged bundle err=%v code=%s", err, CodeOf(err))
	}
	valid := testPublishedBundle(t)
	valid.json[0] ^= 1
	if err := PublishBundle(context.Background(), destination, valid); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("mutated bundle err=%v code=%s", err, CodeOf(err))
	}
}

func TestBundleAccessorsReturnDeepCopies(t *testing.T) {
	bundle := testPublishedBundle(t)
	wantJSON := bundle.JSON()
	wantMarkdown := bundle.Markdown()
	wantResult := bundle.Result()
	jsonCopy := bundle.JSON()
	markdownCopy := bundle.Markdown()
	resultCopy := bundle.Result()
	jsonCopy[0] ^= 1
	markdownCopy[0] ^= 1
	resultCopy.UserItems[0].Statement = "forged"
	resultCopy.UserItems[0].Sources[0].Hash = testID('f')
	if !validBundle(bundle) || !bytes.Equal(bundle.JSON(), wantJSON) || !bytes.Equal(bundle.Markdown(), wantMarkdown) || bundle.Result().InputDigest != wantResult.InputDigest || bundle.Result().UserItems[0].Statement != wantResult.UserItems[0].Statement {
		t.Fatalf("bundle accessor mutation changed opaque bundle")
	}
}

func TestPublishBundleConcurrentDestinationCreationNeverOverwrites(t *testing.T) {
	if !atomicDirectoryPublicationSupported {
		t.Skip("atomic publication is not supported on this platform")
	}
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			<-start
			results <- PublishBundle(context.Background(), destination, bundle)
		}()
	}
	close(start)
	succeeded, existed := 0, 0
	for index := 0; index < 2; index++ {
		err := <-results
		switch CodeOf(err) {
		case "sessiondistill.internal":
			if err != nil {
				t.Fatalf("unexpected internal publication failure: %v", err)
			}
			succeeded++
		case CodeDestinationExists:
			existed++
		default:
			t.Fatalf("unexpected publication result: err=%v code=%s", err, CodeOf(err))
		}
	}
	if succeeded != 1 || existed != 1 {
		t.Fatalf("succeeded=%d destination_exists=%d", succeeded, existed)
	}
	assertPublishedBundle(t, destination, bundle)
	assertNoStagingDirectories(t, parent)
}

func testPublishedBundle(t *testing.T) Bundle {
	t.Helper()
	bundle, err := NewV1().Distill(context.Background(), Request{
		SchemaVersion: SchemaVersion,
		SessionID:     testID('a'),
		Turns:         []VisibleTurn{{EventID: testID('b'), Ordinal: 1, Role: RoleUser, Text: "我的想法是输出报告。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func assertPublishedBundle(t *testing.T, destination string, bundle Bundle) {
	t.Helper()
	jsonData, err := os.ReadFile(filepath.Join(destination, "report.json"))
	if err != nil || string(jsonData) != string(bundle.JSON()) {
		t.Fatalf("JSON data=%q err=%v", jsonData, err)
	}
	markdown, err := os.ReadFile(filepath.Join(destination, "report.md"))
	if err != nil || string(markdown) != string(bundle.Markdown()) {
		t.Fatalf("Markdown data=%q err=%v", markdown, err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil || len(entries) != 2 || entries[0].Name() != "report.json" || entries[1].Name() != "report.md" {
		t.Fatalf("published entries=%v err=%v", entries, err)
	}
}

func assertNoStagingDirectories(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mindweaver-ideas-") {
			t.Fatalf("staging residue %q", entry.Name())
		}
	}
}
