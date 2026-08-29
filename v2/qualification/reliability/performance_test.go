package reliability

import (
	"bytes"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

// BenchmarkBlobDurablePublication measures the real content-addressed Prepare,
// fsync, no-replace publication, and verification boundary. Cleanup is outside
// the timed region and uses the production deletion barrier.
func BenchmarkBlobDurablePublication(b *testing.B) {
	root := b.TempDir()
	blobs, err := blob.OpenStore(root)
	qualificationRequire(b, "BENCH_OPEN_BLOB_STORE", err == nil)
	b.ReportAllocs()
	b.SetBytes(qualificationSourceBytes)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		b.StopTimer()
		payload := qualificationPayload(uint64(index))
		b.StartTimer()
		result, err := blobs.Import(b.Context(), bytes.NewReader(payload), int64(len(payload)))
		b.StopTimer()
		qualificationRequire(b, "BENCH_BLOB_IMPORT", err == nil && result.Created && result.Size == int64(len(payload)))
		qualificationDeleteUnreferencedBlob(b, blobs, result.ID)
	}
}

// BenchmarkWorkbenchUploadAndIngest measures the real workbench boundary from
// bounded blob publication through the SQLite job claim and ingestion commit.
// Qualification cleanup is excluded from timing to keep resource use bounded.
func BenchmarkWorkbenchUploadAndIngest(b *testing.B) {
	runtime := newQualificationRuntime(b)
	b.ReportAllocs()
	b.SetBytes(qualificationSourceBytes)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		b.StopTimer()
		payload := qualificationPayload(uint64(index))
		b.StartTimer()
		upload, err := runtime.service.Upload(b.Context(), uploadRequest(index, payload))
		if err == nil {
			_, err = runtime.service.RunOne(b.Context(), "benchmark-worker", qualificationLease)
		}
		b.StopTimer()
		qualificationRequire(b, "BENCH_WORKBENCH_INGEST", err == nil && upload.Created)
		qualificationPurge(b, runtime, upload)
	}
}

// BenchmarkSQLiteFTSSearch measures the production literal-phrase FTS path
// over a frozen 32-document active corpus. It deliberately declares no
// cross-machine latency threshold; benchstat or release infrastructure owns
// comparisons between controlled runs.
func BenchmarkSQLiteFTSSearch(b *testing.B) {
	const corpusDocuments = 32
	runtime := newQualificationRuntime(b)
	want := make(map[string]struct{}, corpusDocuments)
	for index := 0; index < corpusDocuments; index++ {
		payload := append(qualificationPayload(uint64(index)), []byte(" stable search needle")...)
		upload, err := runtime.service.Upload(b.Context(), uploadRequest(index, payload))
		qualificationRequire(b, "BENCH_SEARCH_SETUP_UPLOAD", err == nil && upload.Created)
		job, err := runtime.service.RunOne(b.Context(), "benchmark-worker", qualificationLease)
		qualificationRequire(b, "BENCH_SEARCH_SETUP_INGEST", err == nil && job.Status == store.JobSucceeded)
		want[upload.DocumentID] = struct{}{}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		hits, err := runtime.service.Search(b.Context(), "stable search needle", corpusDocuments)
		b.StopTimer()
		qualificationRequire(b, "BENCH_FTS_SEARCH", err == nil && len(hits) == corpusDocuments)
		for _, hit := range hits {
			_, expected := want[hit.DocumentID]
			qualificationRequire(b, "BENCH_FTS_IDENTITY", expected)
		}
		b.StartTimer()
	}
	b.StopTimer()
}

func uploadRequest(sequence int, payload []byte) workbench.UploadRequest {
	return workbench.UploadRequest{
		IdempotencyKey: qualificationSequence("benchmark", sequence),
		Title:          "Reliability benchmark",
		Filename:       "benchmark.txt",
		Source:         bytes.NewReader(payload),
	}
}
