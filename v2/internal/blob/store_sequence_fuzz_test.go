package blob

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

const (
	maxSequenceFuzzContentBytes = 128
	maxSequenceFuzzOperations   = 12
)

// FuzzStoreOperationSequence exercises the real filesystem-backed Blob state
// machine rather than one parser call. The model deliberately stays within the
// Store's ownership boundary: database references and crash recovery remain in
// their owning qualification suites.
func FuzzStoreOperationSequence(f *testing.F) {
	for _, seed := range []struct {
		left       []byte
		right      []byte
		operations []byte
	}{
		{[]byte("alpha"), []byte("beta"), []byte{0, 2, 1, 2, 4, 5, 8, 0, 2}},
		{nil, nil, []byte{0, 6, 7, 2, 8, 9, 4, 3}},
		{[]byte{0, 0xff}, []byte{0xff, 0}, []byte{4, 0, 8, 2, 5, 1, 3, 7, 6}},
	} {
		f.Add(seed.left, seed.right, seed.operations)
	}

	f.Fuzz(func(t *testing.T, left, right, operations []byte) {
		if len(left) > maxSequenceFuzzContentBytes || len(right) > maxSequenceFuzzContentBytes ||
			len(operations) > maxSequenceFuzzOperations {
			t.Skip()
		}

		root := t.TempDir()
		store, err := OpenStore(root)
		if err != nil {
			t.Fatalf("OpenStore: %v", err)
		}
		contents := [2][]byte{
			append([]byte{0}, left...),
			append([]byte{1}, right...),
		}
		ids := [2]BlobID{sequenceBlobID(contents[0]), sequenceBlobID(contents[1])}
		published := [2]bool{}
		var prepared PreparedImport
		preparedIndex := -1

		abortPrepared := func() {
			t.Helper()
			if prepared == nil {
				return
			}
			if err := prepared.Abort(); err != nil {
				t.Fatalf("Abort: %v", err)
			}
			prepared = nil
			preparedIndex = -1
		}
		t.Cleanup(abortPrepared)

		for step, rawOperation := range operations {
			operation := rawOperation % 10
			switch operation {
			case 0, 1:
				abortPrepared()
				preparedIndex = int(operation)
				prepared, err = store.Prepare(t.Context(), bytes.NewReader(contents[preparedIndex]), int64(len(contents[preparedIndex])))
				if err != nil {
					t.Fatalf("step %d Prepare(%d): %v", step, preparedIndex, err)
				}
				if prepared.ID() != ids[preparedIndex] || prepared.Size() != int64(len(contents[preparedIndex])) {
					t.Fatalf("step %d prepared identity = %q/%d, want %q/%d", step, prepared.ID(), prepared.Size(), ids[preparedIndex], len(contents[preparedIndex]))
				}
			case 2:
				if prepared != nil {
					wasPublished := published[preparedIndex]
					result, publishErr := prepared.Publish(t.Context())
					if publishErr != nil {
						t.Fatalf("step %d Publish: %v", step, publishErr)
					}
					if result.ID != ids[preparedIndex] || result.Size != int64(len(contents[preparedIndex])) || result.Created == wasPublished {
						t.Fatalf("step %d Publish = %#v, previously published=%t", step, result, wasPublished)
					}
					published[preparedIndex] = true
					prepared = nil
					preparedIndex = -1
				}
			case 3:
				abortPrepared()
			case 4, 5:
				index := int(operation - 4)
				wasPublished := published[index]
				result, importErr := store.Import(t.Context(), bytes.NewReader(contents[index]), int64(len(contents[index])))
				if importErr != nil {
					t.Fatalf("step %d Import(%d): %v", step, index, importErr)
				}
				if result.ID != ids[index] || result.Size != int64(len(contents[index])) || result.Created == wasPublished {
					t.Fatalf("step %d Import(%d) = %#v, previously published=%t", step, index, result, wasPublished)
				}
				published[index] = true
			case 6:
				removed, cleanupErr := store.CleanupStaging(t.Context(), time.Now().Add(time.Hour))
				if cleanupErr != nil || removed != 0 {
					t.Fatalf("step %d CleanupStaging = %d, %v; active staging must be retained", step, removed, cleanupErr)
				}
			case 7:
				reopened, reopenErr := OpenStore(root)
				if reopenErr != nil {
					t.Fatalf("step %d reopen Store: %v", step, reopenErr)
				}
				store = reopened
			case 8:
				index := int(rawOperation>>4) % len(contents)
				guard, guardErr := store.BeginDeletionContext(t.Context())
				if guardErr != nil {
					t.Fatalf("step %d BeginDeletion: %v", step, guardErr)
				}
				removed, deleteErr := guard.Delete(t.Context(), ids[index])
				guard.Release()
				if deleteErr != nil || removed != published[index] {
					t.Fatalf("step %d Delete(%d) = %t, %v; published=%t", step, index, removed, deleteErr, published[index])
				}
				published[index] = false
			case 9:
				// The invariant check below is the operation for this opcode.
			}
			assertSequenceBlobState(t, store, contents, ids, published, prepared != nil)
		}

		abortPrepared()
		removed, err := store.CleanupStaging(t.Context(), time.Now().Add(time.Hour))
		if err != nil || removed != 0 {
			t.Fatalf("final CleanupStaging = %d, %v", removed, err)
		}
		assertSequenceBlobState(t, store, contents, ids, published, false)
	})
}

func sequenceBlobID(content []byte) BlobID {
	digest := sha256.Sum256(content)
	return BlobID(fmt.Sprintf("sha256:%x", digest))
}

func assertSequenceBlobState(t *testing.T, store *Store, contents [2][]byte, ids [2]BlobID, published [2]bool, hasPrepared bool) {
	t.Helper()
	wantObjects := 0
	for index, id := range ids {
		file, err := store.Open(id)
		if !published[index] {
			if !errors.Is(err, os.ErrNotExist) {
				if file != nil {
					_ = file.Close()
				}
				t.Fatalf("Open unpublished object %d error = %v, want not-exist", index, err)
			}
			continue
		}
		wantObjects++
		if err != nil {
			t.Fatalf("Open published object %d: %v", index, err)
		}
		got, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(got, contents[index]) {
			t.Fatalf("published object %d content/read/close = %x, %v", index, got, errors.Join(readErr, closeErr))
		}
	}

	objects := 0
	for _, prefix := range mustReadSequenceDirectory(t, store.objectsDir) {
		if !prefix.IsDir() {
			t.Fatalf("unexpected non-directory in objects root: %s", prefix.Name())
		}
		for _, object := range mustReadSequenceDirectory(t, store.objectsDir+string(os.PathSeparator)+prefix.Name()) {
			if !object.Type().IsRegular() {
				t.Fatalf("unexpected non-regular object: %s", object.Name())
			}
			objects++
		}
	}
	if objects != wantObjects {
		t.Fatalf("published object count = %d, want %d", objects, wantObjects)
	}

	staging := mustReadSequenceDirectory(t, store.stagingDir)
	wantStaging := 0
	if hasPrepared {
		wantStaging = 1
	}
	if len(staging) != wantStaging {
		t.Fatalf("staging entry count = %d, want %d", len(staging), wantStaging)
	}
	for _, entry := range staging {
		if !entry.Type().IsRegular() {
			t.Fatalf("unexpected non-regular staging entry: %s", entry.Name())
		}
	}
}

func mustReadSequenceDirectory(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", path, err)
	}
	return entries
}
