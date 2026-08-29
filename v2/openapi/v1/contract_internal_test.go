package v1

import "testing"

func TestForbiddenSegmentRecognizesCanonicalDeferredTokens(t *testing.T) {
	t.Parallel()
	for _, token := range []string{
		"boundedinstr", "cache", "caches", "expansion", "fusion", "hybrid", "naturalquestion",
		"profile", "profiles", "qdrant", "queryunderstanding", "rrf", "storage", "storages", "termindex",
	} {
		if got, forbidden := forbiddenSegment("internal/" + token); !forbidden || got != token {
			t.Errorf("canonical LATER token %q = %q/%t", token, got, forbidden)
		}
	}
}

func TestForbiddenSegmentRecognizesEveryDeferredCompound(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  string
	}{
		{"agentprofile", "agent"},
		{"agentservice", "agent"},
		{"createagent", "agent"},
		{"batchimporter", "batch"},
		{"batchrunner", "batch"},
		{"cachestore", "cache"},
		{"embeddingcache", "cache"},
		{"retrievalcache", "cache"},
		{"embeddingservice", "embedding"},
		{"evaluationrun", "evaluation"},
		{"fusionservice", "fusion"},
		{"fusionstage", "fusion"},
		{"hybridretriever", "hybrid"},
		{"hybridsearch", "hybrid"},
		{"memoryrecord", "memory"},
		{"memorystore", "memory"},
		{"notificationprojection", "notification"},
		{"notificationservice", "notification"},
		{"profilestore", "profile"},
		{"qdrantclient", "qdrant"},
		{"reindexdocument", "reindex"},
		{"queryexpansion", "expansion"},
		{"queryexpansionservice", "expansion"},
		{"queryunderstandingservice", "queryunderstanding"},
		{"rerankresults", "rerank"},
		{"rerankservice", "rerank"},
		{"reciprocalrankfusion", "rrf"},
		{"rrfscore", "rrf"},
		{"storagesummary", "storage"},
		{"vectorbackend", "vector"},
		{"vectorsearch", "vector"},
		{"boundedinstrretriever", "boundedinstr"},
		{"naturalquestionretriever", "naturalquestion"},
		{"relationaltermindex", "termindex"},
	} {
		t.Run(test.value, func(t *testing.T) {
			if got, forbidden := forbiddenSegment(test.value); !forbidden || got != test.want {
				t.Fatalf("forbiddenSegment(%q) = %q/%t, want %q/true", test.value, got, forbidden, test.want)
			}
		})
	}
}

func TestForbiddenSegmentRecognizesSeparatedAndCamelCaseDeferredNames(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  string
	}{
		{"internal/hybrid", "hybrid"},
		{"RRFScore", "rrf"},
		{"fusion-stage", "fusion"},
		{"query_expansion", "expansion"},
		{"query-understanding", "queryunderstanding"},
		{"queryUnderstanding", "queryunderstanding"},
		{"QdrantClient", "qdrant"},
		{"ProfileStore", "profile"},
		{"NaturalQuestionRetriever", "naturalquestion"},
		{"RelationalTermIndex", "termindex"},
		{"BoundedInstrRetriever", "boundedinstr"},
	} {
		t.Run(test.value, func(t *testing.T) {
			if got, forbidden := forbiddenSegment(test.value); !forbidden || got != test.want {
				t.Fatalf("forbiddenSegment(%q) = %q/%t, want %q/true", test.value, got, forbidden, test.want)
			}
		})
	}
	for _, value := range []string{"query", "understanding", "internal/store/sqlite", "runtime-boundary"} {
		if got, forbidden := forbiddenSegment(value); forbidden {
			t.Errorf("CORE spelling %q was rejected as %q", value, got)
		}
	}
}
