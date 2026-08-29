package blob

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrepareStagingWriteAndSyncFailuresFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*Store)
		want   error
	}{
		{
			name: "partial write then ENOSPC",
			inject: func(store *Store) {
				calls := 0
				store.writeStaging = func(file *os.File, data []byte) (int, error) {
					calls++
					if calls == 1 {
						return file.Write(data[:len(data)/2])
					}
					return 0, syscall.ENOSPC
				}
			},
			want: syscall.ENOSPC,
		},
		{
			name: "zero byte short write",
			inject: func(store *Store) {
				store.writeStaging = func(*os.File, []byte) (int, error) { return 0, nil }
			},
			want: io.ErrShortWrite,
		},
		{
			name: "staging file sync ENOSPC",
			inject: func(store *Store) {
				store.syncStaging = func(*os.File) error { return syscall.ENOSPC }
			},
			want: syscall.ENOSPC,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore(t)
			content := []byte("BLOB-001 deterministic staging failure and exact retry")
			stagingSyncs := 0
			productionSyncDir := store.syncDir
			store.syncDir = func(path string) error {
				if filepath.Clean(path) == filepath.Clean(store.stagingDir) {
					stagingSyncs++
				}
				return productionSyncDir(path)
			}
			test.inject(store)

			prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), int64(len(content)))
			if prepared != nil || !errors.Is(err, test.want) {
				t.Fatalf("Prepare = %#v, %v; want nil and %v", prepared, err, test.want)
			}
			if stagingSyncs == 0 {
				t.Fatal("failed Prepare did not sync the staging directory after cleanup")
			}
			assertStagingEmpty(t, store)
			assertNoObjects(t, store)

			store.writeStaging = writeStagingFile
			store.syncStaging = syncStagingFile
			result, err := store.Import(t.Context(), bytes.NewReader(content), int64(len(content)))
			if err != nil || !result.Created || result.ID.String() != blobIDForTest(content) {
				t.Fatalf("exact retry = %#v, %v", result, err)
			}
			assertBlobContent(t, store, result.ID, content)
			assertStagingEmpty(t, store)
		})
	}
}

func blobIDForTest(content []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(content))
}
