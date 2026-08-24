package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

type backupFixture struct {
	root        string
	database    *store.Store
	blobs       *blob.Store
	workbench   *workbench.Service
	coordinator *Coordinator
}

func newBackupFixture(t *testing.T) *backupFixture {
	t.Helper()
	root := t.TempDir()
	database, err := store.Open(t.Context(), filepath.Join(root, "live.db"), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	blobs, err := blob.OpenStore(filepath.Join(root, "live-blobs"))
	if err != nil {
		t.Fatal(err)
	}
	workbenchService, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	return &backupFixture{root, database, blobs, workbenchService, coordinator}
}

func (fixture *backupFixture) addDocument(t *testing.T, key, content string) workbench.UploadResult {
	t.Helper()
	return fixture.addDocumentContext(t.Context(), key, content, func(err error) { t.Fatal(err) })
}

func (fixture *backupFixture) addDocumentContext(ctx context.Context, key, content string, fail func(error)) workbench.UploadResult {
	upload, err := fixture.workbench.Upload(ctx, workbench.UploadRequest{
		IdempotencyKey: key,
		Title:          "Backup " + key,
		Filename:       key + ".txt",
		Source:         strings.NewReader(content),
	})
	if err != nil {
		fail(fmt.Errorf("upload %s: %w", key, err))
		return workbench.UploadResult{}
	}
	job, err := fixture.workbench.RunOne(ctx, "backup-writer", time.Minute)
	if err != nil || job.Status != store.JobSucceeded {
		fail(fmt.Errorf("run %s: status=%s error=%w", key, job.Status, err))
		return workbench.UploadResult{}
	}
	return upload
}

func TestBackupUnderWritesRestoresSnapshotAndEveryBlob(t *testing.T) {
	fixture := newBackupFixture(t)
	seed := fixture.addDocument(t, "seed", "seed backup consistency searchable phrase")

	writerContext := t.Context()
	firstWrite := make(chan struct{})
	writerDone := make(chan error, 1)
	var writes atomic.Int64
	go func() {
		for index := 0; index < 100; index++ {
			if err := writerContext.Err(); err != nil {
				writerDone <- nil
				return
			}
			var writeErr error
			fixture.addDocumentContext(writerContext, fmt.Sprintf("concurrent-%02d", index),
				fmt.Sprintf("concurrent backup payload number %02d", index),
				func(err error) { writeErr = err })
			if writeErr != nil {
				writerDone <- writeErr
				return
			}
			if writes.Add(1) == 1 {
				close(firstWrite)
			}
		}
		writerDone <- nil
	}()
	<-firstWrite

	backupPath := filepath.Join(fixture.root, "backup-under-writes")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if err := <-writerDone; err != nil {
		t.Fatalf("concurrent writer: %v", err)
	}
	if writes.Load() == 0 {
		t.Fatal("concurrent writer made no progress")
	}
	if len(manifest.Blobs) == 0 || manifest.Database.Path != databasePath {
		t.Fatalf("manifest = %#v", manifest)
	}

	restoredPath := filepath.Join(fixture.root, "restored-vault")
	if err := Restore(t.Context(), backupPath, restoredPath); err != nil {
		t.Fatalf("restore: %v", err)
	}
	owned, err := vault.Open(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	restoredDatabase, err := store.Open(t.Context(), filepath.Join(owned.Paths().Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDatabase.Close()
	restoredBlobStore, err := blob.OpenStore(owned.Paths().Blobs)
	if err != nil {
		t.Fatal(err)
	}
	restoredWorkbench, err := workbench.New(restoredDatabase, restoredBlobStore)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := restoredWorkbench.Search(t.Context(), "consistency", 10)
	if err != nil || len(hits) != 1 || hits[0].DocumentID != seed.DocumentID {
		t.Fatalf("restored seed search = %#v, err=%v", hits, err)
	}
	for _, artifact := range manifest.Blobs {
		id, err := blob.ParseID(artifact.BlobID)
		if err != nil {
			t.Fatal(err)
		}
		file, err := restoredBlobStore.Open(id)
		if err != nil {
			t.Fatalf("open restored blob %s: %v", id, err)
		}
		_ = file.Close()
	}
}

func TestRestoreRejectsCorruptionTraversalFutureAndExistingTarget(t *testing.T) {
	t.Run("database corruption", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "database-tamper", "database tamper searchable text")
		backupPath := filepath.Join(fixture.root, "backup")
		manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
		if err != nil {
			t.Fatal(err)
		}
		path, err := resolveArtifactPath(backupPath, manifest.Database.Path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteAt([]byte("CORRUPT"), 128); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := Restore(t.Context(), backupPath, filepath.Join(fixture.root, "restored")); err == nil {
			t.Fatal("corrupt database restore unexpectedly succeeded")
		}
	})

	t.Run("blob corruption", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "tamper", "tamper detection searchable text")
		backupPath := filepath.Join(fixture.root, "backup")
		manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
		if err != nil {
			t.Fatal(err)
		}
		path, err := resolveArtifactPath(backupPath, manifest.Blobs[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(fixture.root, "must-not-exist")
		if err := Restore(t.Context(), backupPath, destination); err == nil {
			t.Fatal("corrupt blob restore unexpectedly succeeded")
		}
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed restore published destination: %v", err)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, path string, manifest Manifest)
	}{
		{
			name: "path traversal",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				manifest.Blobs[0].Path = "../outside"
				rewriteManifest(t, path, manifest)
			},
		},
		{
			name: "future schema",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				manifest.SchemaVersion = 1_000_000
				rewriteManifest(t, path, manifest)
			},
		},
		{
			name: "schema label mismatch",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				manifest.SchemaVersion = 1
				rewriteManifest(t, path, manifest)
			},
		},
		{
			name: "unknown field",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				data, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data[:len(data)-1], []byte(`,"unexpected":true}`)...)
				if err := os.WriteFile(filepath.Join(path, manifestFileName), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "duplicate field",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				data, err := os.ReadFile(filepath.Join(path, manifestFileName))
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), "{", `{"format_version":1,`, 1))
				if err := os.WriteFile(filepath.Join(path, manifestFileName), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBackupFixture(t)
			fixture.addDocument(t, "manifest", "manifest validation searchable text")
			backupPath := filepath.Join(fixture.root, "backup")
			manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, backupPath, manifest)
			if err := Restore(t.Context(), backupPath, filepath.Join(fixture.root, "restored")); err == nil {
				t.Fatalf("%s restore unexpectedly succeeded", test.name)
			}
		})
	}

	t.Run("existing destination", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "existing", "existing target searchable text")
		backupPath := filepath.Join(fixture.root, "backup")
		if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(fixture.root, "existing-target")
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(destination, "marker")
		if err := os.WriteFile(marker, []byte("untouched"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Restore(t.Context(), backupPath, destination); err == nil {
			t.Fatal("restore over existing target unexpectedly succeeded")
		}
		data, err := os.ReadFile(marker)
		if err != nil || string(data) != "untouched" {
			t.Fatalf("existing target changed: %q, %v", data, err)
		}
	})
}

func TestReadManifestRequiresExactCompleteSchema(t *testing.T) {
	schemaVersion, err := store.SupportedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	id, err := blob.ParseID("sha256:" + digest)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(Manifest{
		FormatVersion:       FormatVersion,
		SchemaVersion:       schemaVersion,
		CreatedAtUnixMicros: 1,
		Database: Artifact{
			Path:   databasePath,
			Size:   1,
			SHA256: strings.Repeat("0", 64),
		},
		Blobs: []Artifact{{
			Path:   blobArtifactPath(id),
			Size:   1,
			SHA256: digest,
			BlobID: id.String(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeAndRead := func(t *testing.T, data []byte) error {
		t.Helper()
		path := filepath.Join(t.TempDir(), manifestFileName)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := readManifest(path)
		return err
	}
	if err := writeAndRead(t, canonical); err != nil {
		t.Fatalf("canonical manifest rejected: %v", err)
	}

	for _, test := range []struct {
		name    string
		wantErr string
		mutate  func(t *testing.T, data []byte) []byte
	}{
		{
			name:    "top-level case alias",
			wantErr: `manifest contains unknown field "FORMAT_VERSION"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["FORMAT_VERSION"] = object["format_version"]
					delete(object, "format_version")
				})
			},
		},
		{
			name:    "top-level case alias alongside canonical field",
			wantErr: `manifest contains unknown field "FORMAT_VERSION"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["FORMAT_VERSION"] = json.RawMessage("999")
				})
			},
		},
		{
			name:    "missing required top-level field",
			wantErr: `manifest requires non-null field "created_at_unix_micros"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					delete(object, "created_at_unix_micros")
				})
			},
		},
		{
			name:    "zero creation time",
			wantErr: "invalid creation time",
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["created_at_unix_micros"] = json.RawMessage("0")
				})
			},
		},
		{
			name:    "missing blobs",
			wantErr: `manifest requires non-null field "blobs"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					delete(object, "blobs")
				})
			},
		},
		{
			name:    "null blobs",
			wantErr: `manifest requires non-null field "blobs"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["blobs"] = json.RawMessage("null")
				})
			},
		},
		{
			name:    "database case alias",
			wantErr: `database artifact contains unknown field "SHA256"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					object["SHA256"] = object["sha256"]
					delete(object, "sha256")
				})
			},
		},
		{
			name:    "database missing required field",
			wantErr: `database artifact requires non-null field "size"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					delete(object, "size")
				})
			},
		},
		{
			name:    "database null blob ID",
			wantErr: `database artifact field "blob_id" must not be null`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					object["blob_id"] = json.RawMessage("null")
				})
			},
		},
		{
			name:    "database non-empty blob ID",
			wantErr: "database artifact must not have a blob ID",
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					object["blob_id"] = json.RawMessage(`"sha256:` + digest + `"`)
				})
			},
		},
		{
			name:    "blob case alias",
			wantErr: `blob artifact 0 contains unknown field "Blob_ID"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateBlobJSONObject(t, data, 0, func(object map[string]json.RawMessage) {
					object["Blob_ID"] = object["blob_id"]
					delete(object, "blob_id")
				})
			},
		},
		{
			name:    "blob missing required field",
			wantErr: `blob artifact 0 requires non-null field "blob_id"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateBlobJSONObject(t, data, 0, func(object map[string]json.RawMessage) {
					delete(object, "blob_id")
				})
			},
		},
		{
			name:    "blob unknown nested field",
			wantErr: `blob artifact 0 contains unknown field "unexpected"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateBlobJSONObject(t, data, 0, func(object map[string]json.RawMessage) {
					object["unexpected"] = json.RawMessage("true")
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := writeAndRead(t, test.mutate(t, canonical))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("readManifest error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestBackupVerificationRejectsEveryUnlistedArtifactAndSidecarFailure(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "exact-tree", "exact backup tree content")
	destination := filepath.Join(t.TempDir(), "backup")
	manifest, err := fixture.coordinator.Create(t.Context(), destination)
	if err != nil {
		t.Fatal(err)
	}

	extra := filepath.Join(destination, "data", store.DatabaseFileName+"-wal")
	if err := os.WriteFile(extra, []byte("unlisted user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupTree(t.Context(), destination, manifest); err == nil || !strings.Contains(err.Error(), "unlisted artifact") {
		t.Fatalf("verify extra artifact error = %v", err)
	}

	blocked := filepath.Join(t.TempDir(), "snapshot")
	if err := os.Mkdir(blocked+"-wal", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked+"-wal", "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeSQLiteSidecars(blocked); err == nil {
		t.Fatal("sidecar removal failure was ignored")
	}
}

func rewriteManifest(t *testing.T, root string, manifest Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, manifestFileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mutateJSONObject(t *testing.T, data []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	mutate(object)
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mutateNestedJSONObject(t *testing.T, data []byte, field string, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
		object[field] = mutateJSONObject(t, object[field], mutate)
	})
}

func mutateBlobJSONObject(t *testing.T, data []byte, index int, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
		var blobs []json.RawMessage
		if err := json.Unmarshal(object["blobs"], &blobs); err != nil {
			t.Fatal(err)
		}
		blobs[index] = mutateJSONObject(t, blobs[index], mutate)
		encoded, err := json.Marshal(blobs)
		if err != nil {
			t.Fatal(err)
		}
		object["blobs"] = encoded
	})
}
