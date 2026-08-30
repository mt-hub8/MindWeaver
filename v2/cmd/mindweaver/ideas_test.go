package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/ideashook"
	"github.com/mt-hub8/MindWeaver/v2/internal/ideasollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/ollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/sessiondistill"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

func TestIdeasExtractNoteCreatesCompleteReport(t *testing.T) {
	outputDirectory := filepath.Join(t.TempDir(), "report")
	var stdout bytes.Buffer
	err := runWithIO(
		context.Background(),
		[]string{"ideas", "extract", "-input-format", "note", "-input", "-", "-output", outputDirectory},
		strings.NewReader("我的想法是先交付一个可用 CLI。\n下一步补充 Codex Hook。"),
		&stdout,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "created ideas report (2 user items, 0 assistant context items)\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	jsonData, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.UserItems) != 2 || len(result.AssistantContext) != 0 || !result.UntrustedVisibleContent {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(outputDirectory, "report.md")); err != nil {
		t.Fatal(err)
	}
}

func TestIdeasExtractChatJSONLKeepsAssistantSeparate(t *testing.T) {
	input := strings.Join([]string{
		`{"role":"user","text":"目标是提炼用户思想。"}`,
		`{"role":"assistant","text":"我的想法是先用规则引擎。"}`,
	}, "\n")
	outputDirectory := filepath.Join(t.TempDir(), "report")
	var stdout bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "extract", "-input-format", "chat-jsonl", "-output", outputDirectory}, strings.NewReader(input), &stdout); err != nil {
		t.Fatal(err)
	}
	jsonData, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.UserItems) != 1 || result.UserItems[0].Attribution != sessiondistill.RoleUser ||
		len(result.AssistantContext) != 1 || result.AssistantContext[0].Attribution != sessiondistill.RoleAssistant {
		t.Fatalf("result=%+v", result)
	}
}

func TestIdeasExtractSessionJSONFromFile(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "session.json")
	input := fmt.Sprintf(
		`{"schema_version":1,"session_id":"sha256:%s","turns":[`+
			`{"event_id":"sha256:%s","ordinal":1,"role":"user","text":"我的目标是完成 Codex CLI。"},`+
			`{"event_id":"sha256:%s","ordinal":2,"role":"assistant","text":"下一步可以安装 Hook。"}]}`,
		strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64),
	)
	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	outputDirectory := filepath.Join(t.TempDir(), "report")
	var stdout bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "extract", "-input", inputPath, "-output", outputDirectory}, strings.NewReader("ignored"), &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.UserItems) != 1 || result.UserItems[0].Kind != sessiondistill.KindGoal || len(result.AssistantContext) != 1 {
		t.Fatalf("result=%+v", result)
	}
}

func TestIdeasExtractInvalidInputIsZeroOutputAndZeroArtifact(t *testing.T) {
	outputDirectory := filepath.Join(t.TempDir(), "report")
	input := `{"schema_version":1,"session_id":"sha256:` + strings.Repeat("a", 64) + `","turns":[],"hidden_reasoning":"secret"}`
	var stdout bytes.Buffer
	err := runWithIO(context.Background(), []string{"ideas", "extract", "-output", outputDirectory}, strings.NewReader(input), &stdout)
	if err == nil || apperror.KindOf(err) != apperror.KindInvalid {
		t.Fatalf("err=%v kind=%s", err, apperror.KindOf(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if _, statErr := os.Stat(outputDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("artifact exists: %v", statErr)
	}
	if strings.Contains(apperror.PublicMessage(err), "secret") {
		t.Fatalf("public error leaked input: %q", apperror.PublicMessage(err))
	}
}

func TestIdeasExtractNeverOverwritesExistingDestination(t *testing.T) {
	outputDirectory := filepath.Join(t.TempDir(), "report")
	if err := os.Mkdir(outputDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outputDirectory, "owner-data")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := runWithIO(context.Background(), []string{"ideas", "extract", "-input-format", "note", "-output", outputDirectory}, strings.NewReader("我的想法是保留原目录。"), &stdout)
	if err == nil || apperror.KindOf(err) != apperror.KindConflict || stdout.Len() != 0 {
		t.Fatalf("err=%v kind=%s stdout=%q", err, apperror.KindOf(err), stdout.String())
	}
	content, readErr := os.ReadFile(marker)
	if readErr != nil || string(content) != "keep" {
		t.Fatalf("marker=%q err=%v", content, readErr)
	}
}

func TestIdeasExtractOptionalOllamaRanksOnlyExistingUserItems(t *testing.T) {
	previousRank := ideasRankCandidates
	ideasRankCandidates = func(ctx context.Context, options ollama.Options, candidates []ideasollama.Candidate) (ideasollama.Result, error) {
		if ctx == nil || options.BaseURL != ideasollama.DefaultBaseURL || options.Model != "local-ranker" || options.Timeout != 1250*time.Millisecond {
			t.Fatalf("options=%+v", options)
		}
		if len(candidates) != 2 || candidates[0].Kind != sessiondistill.KindIdea || candidates[1].Kind != sessiondistill.KindNextAction {
			t.Fatalf("candidates=%+v", candidates)
		}
		return ideasollama.Result{
			ProviderConfigDigest: "sha256:" + strings.Repeat("d", 64),
			RankedItemIDs:        []string{candidates[1].ItemID, candidates[0].ItemID},
		}, nil
	}
	t.Cleanup(func() { ideasRankCandidates = previousRank })

	outputDirectory := filepath.Join(t.TempDir(), "report")
	var stdout bytes.Buffer
	err := runWithIO(context.Background(), []string{
		"ideas", "extract", "-input-format", "note", "-output", outputDirectory,
		"-ollama-model", "local-ranker", "-ollama-timeout", "1250ms",
	}, strings.NewReader("我的想法是先交付 CLI。\n下一步补充测试。"), &stdout)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.ModelAssistance == nil || result.ModelAssistance.Status != "completed" ||
		len(result.ModelAssistance.RankedItemIDs) != 2 || result.ModelAssistance.RankedItemIDs[0] != result.UserItems[1].ID ||
		result.ModelAssistance.RankedItemIDs[1] != result.UserItems[0].ID {
		t.Fatalf("result=%+v", result)
	}
	markdown, err := os.ReadFile(filepath.Join(outputDirectory, "report.md"))
	if err != nil || !bytes.Contains(markdown, []byte("Optional local-model priority (non-authoritative)")) {
		t.Fatalf("markdown=%q err=%v", markdown, err)
	}
}

func TestIdeasExtractOptionalOllamaLiteralLoopbackEndToEnd(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/api/chat" {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		var providerRequest struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil || len(providerRequest.Messages) != 1 {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		var prompt struct {
			Candidates []struct {
				ItemID string `json:"item_id"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(providerRequest.Messages[0].Content), &prompt); err != nil || len(prompt.Candidates) != 1 {
			http.Error(writer, "invalid prompt", http.StatusBadRequest)
			return
		}
		ranking, _ := json.Marshal(struct {
			SchemaVersion int      `json:"schema_version"`
			RankedItemIDs []string `json:"ranked_item_ids"`
		}{SchemaVersion: 1, RankedItemIDs: []string{prompt.Candidates[0].ItemID}})
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"message": map[string]string{"role": "assistant", "content": string(ranking)},
			"done":    true,
		})
	}))
	t.Cleanup(server.Close)
	t.Setenv("HTTP_PROXY", "http://203.0.113.1:9")
	t.Setenv("HTTPS_PROXY", "http://203.0.113.1:9")

	outputDirectory := filepath.Join(t.TempDir(), "report")
	if err := runWithIO(context.Background(), []string{
		"ideas", "extract", "-input-format", "note", "-output", outputDirectory,
		"-ollama-model", "literal-loopback-ranker", "-ollama-endpoint", server.URL,
	}, strings.NewReader("我的想法是用本地模型排列已有条目。"), io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || result.ModelAssistance == nil || result.ModelAssistance.Status != "completed" ||
		len(result.ModelAssistance.RankedItemIDs) != 1 || result.ModelAssistance.RankedItemIDs[0] != result.UserItems[0].ID {
		t.Fatalf("calls=%d result=%+v", calls.Load(), result)
	}
	assertExactIdeasModelAssistanceFields(t, data, false)
}

func TestIdeasExtractOptionalOllamaFallbackStillPublishesDeterministicEvidence(t *testing.T) {
	const (
		modelCanary    = "ollama-model-canary-7c0da2"
		promptCanary   = "likely long-term usefulness"
		responseCanary = "ollama-response-canary-922f31"
		errorCanary    = "ollama-error-canary-b4a80e"
	)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var providerRequest struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/chat" ||
			json.NewDecoder(request.Body).Decode(&providerRequest) != nil ||
			providerRequest.Model != modelCanary || len(providerRequest.Messages) != 1 ||
			!strings.Contains(providerRequest.Messages[0].Content, promptCanary) {
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"response": responseCanary,
			"error":    errorCanary,
		})
	}))
	t.Cleanup(server.Close)

	outputDirectory := filepath.Join(t.TempDir(), "report")
	var stdout bytes.Buffer
	if err := runWithIO(context.Background(), []string{
		"ideas", "extract", "-input-format", "note", "-output", outputDirectory,
		"-ollama-model", modelCanary, "-ollama-endpoint", server.URL,
	}, strings.NewReader("我的想法是保留确定性证据。"), &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := os.ReadFile(filepath.Join(outputDirectory, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(result.UserItems) != 1 || result.ModelAssistance == nil || result.ModelAssistance.Status != "fallback" ||
		result.ModelAssistance.LimitationCode != ideasollama.LimitationUnavailable ||
		result.ModelAssistance.RankedItemIDs == nil || len(result.ModelAssistance.RankedItemIDs) != 0 {
		t.Fatalf("calls=%d result=%+v", calls.Load(), result)
	}
	for _, artifact := range [][]byte{data, markdown, stdout.Bytes()} {
		assertIdeasCanariesAbsent(t, artifact, server.URL, modelCanary, promptCanary, responseCanary, errorCanary)
	}
	assertExactIdeasModelAssistanceFields(t, data, true)
}

func TestIdeasExtractOptionalOllamaNoUserCandidatesNeverCallsProvider(t *testing.T) {
	const modelCanary = "ollama-empty-candidates-model-canary-3fe126"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(writer, "provider must not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	note := "这是普通背景信息。"
	var firstJSON, firstMarkdown []byte
	for run := 0; run < 2; run++ {
		outputDirectory := filepath.Join(t.TempDir(), fmt.Sprintf("report-%d", run))
		if err := runWithIO(context.Background(), []string{
			"ideas", "extract", "-input-format", "note", "-output", outputDirectory,
			"-ollama-model", modelCanary, "-ollama-endpoint", server.URL,
		}, strings.NewReader(note), io.Discard); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		markdown, err := os.ReadFile(filepath.Join(outputDirectory, "report.md"))
		if err != nil {
			t.Fatal(err)
		}
		if run == 0 {
			firstJSON, firstMarkdown = data, markdown
		} else if !bytes.Equal(data, firstJSON) || !bytes.Equal(markdown, firstMarkdown) {
			t.Fatal("no-candidate fallback output is not byte deterministic")
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls=%d", calls.Load())
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(firstJSON, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.UserItems) != 0 || result.ModelAssistance == nil || result.ModelAssistance.Status != "fallback" ||
		result.ModelAssistance.LimitationCode != ideasollama.LimitationNoItems ||
		result.ModelAssistance.RankedItemIDs == nil || len(result.ModelAssistance.RankedItemIDs) != 0 {
		t.Fatalf("result=%+v", result)
	}
	assertIdeasCanariesAbsent(t, firstJSON, server.URL, modelCanary)
	assertIdeasCanariesAbsent(t, firstMarkdown, server.URL, modelCanary)
	assertExactIdeasModelAssistanceFields(t, firstJSON, true)
}

func TestIdeasExtractWithoutModelNeverCallsProviderOrChangesBaseShape(t *testing.T) {
	previousRank := ideasRankCandidates
	ideasRankCandidates = func(context.Context, ollama.Options, []ideasollama.Candidate) (ideasollama.Result, error) {
		t.Fatal("provider called without -ollama-model")
		return ideasollama.Result{}, errors.New("unreachable")
	}
	t.Cleanup(func() { ideasRankCandidates = previousRank })

	note := "我的想法是默认不调用模型。"
	request, err := sessiondistill.RequestFromNote(strings.NewReader(note))
	if err != nil {
		t.Fatal(err)
	}
	base, err := sessiondistill.NewV1().Distill(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	outputDirectory := filepath.Join(t.TempDir(), "report")
	if err := runWithIO(context.Background(), []string{
		"ideas", "extract", "-input-format", "note", "-output", outputDirectory,
	}, strings.NewReader(note), io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := os.ReadFile(filepath.Join(outputDirectory, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.ModelAssistance != nil || bytes.Contains(data, []byte("model_assistance")) ||
		!bytes.Equal(data, base.JSON()) || !bytes.Equal(markdown, base.Markdown()) {
		t.Fatalf("no-model report changed base bytes:\nJSON=%s\nMarkdown=%s", data, markdown)
	}
}

func TestIdeasOllamaFlagsAndCancellationFailBeforePublication(t *testing.T) {
	tests := []struct {
		name string
		args []string
		kind apperror.Kind
	}{
		{name: "endpoint without model", args: []string{"-ollama-endpoint", "http://127.0.0.1:11434"}, kind: apperror.KindInvalid},
		{name: "timeout without model", args: []string{"-ollama-timeout", "2s"}, kind: apperror.KindInvalid},
		{name: "empty model", args: []string{"-ollama-model", ""}, kind: apperror.KindInvalid},
		{name: "too long timeout", args: []string{"-ollama-model", "ranker", "-ollama-timeout", "61s"}, kind: apperror.KindInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputDirectory := filepath.Join(t.TempDir(), "report")
			args := append([]string{"ideas", "extract", "-input-format", "note", "-output", outputDirectory}, test.args...)
			var stdout bytes.Buffer
			err := runWithIO(context.Background(), args, strings.NewReader("我的想法是不能发布。"), &stdout)
			if err == nil || apperror.KindOf(err) != test.kind || stdout.Len() != 0 {
				t.Fatalf("err=%v kind=%s stdout=%q", err, apperror.KindOf(err), stdout.String())
			}
			if _, statErr := os.Stat(outputDirectory); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("output exists: %v", statErr)
			}
		})
	}

	previousRank := ideasRankCandidates
	ideasRankCandidates = func(context.Context, ollama.Options, []ideasollama.Candidate) (ideasollama.Result, error) {
		return ideasollama.Result{}, context.Canceled
	}
	t.Cleanup(func() { ideasRankCandidates = previousRank })
	outputDirectory := filepath.Join(t.TempDir(), "report")
	var stdout bytes.Buffer
	err := runWithIO(context.Background(), []string{
		"ideas", "extract", "-input-format", "note", "-output", outputDirectory, "-ollama-model", "ranker",
	}, strings.NewReader("我的想法是取消后不能发布。"), &stdout)
	if err == nil || apperror.KindOf(err) != apperror.KindCanceled || stdout.Len() != 0 {
		t.Fatalf("err=%v kind=%s stdout=%q", err, apperror.KindOf(err), stdout.String())
	}
	if public := apperror.PublicMessage(err); public != "ideas extraction was canceled" {
		t.Fatalf("cancellation public message=%q", public)
	}
	if _, statErr := os.Stat(outputDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output exists: %v", statErr)
	}

	const (
		endpointCanary = "http://127.0.0.1:54321"
		modelCanary    = "ollama-public-error-model-canary-f3d6a1"
		promptCanary   = "ollama-public-error-prompt-canary-019a8c"
		responseCanary = "ollama-public-error-response-canary-a5b417"
		errorCanary    = "ollama-public-error-cause-canary-d92e60"
	)
	ideasRankCandidates = func(context.Context, ollama.Options, []ideasollama.Candidate) (ideasollama.Result, error) {
		return ideasollama.Result{}, errors.New(strings.Join(
			[]string{endpointCanary, modelCanary, promptCanary, responseCanary, errorCanary}, " ",
		))
	}
	internalOutput := filepath.Join(t.TempDir(), "report")
	stdout.Reset()
	err = runWithIO(context.Background(), []string{
		"ideas", "extract", "-input-format", "note", "-output", internalOutput,
		"-ollama-model", modelCanary, "-ollama-endpoint", endpointCanary,
	}, strings.NewReader("我的想法是内部错误也不能泄漏。"), &stdout)
	if err == nil || apperror.KindOf(err) != apperror.KindInternal || apperror.CodeOf(err) != "ideas.ollama_internal" || stdout.Len() != 0 {
		t.Fatalf("err=%v kind=%s code=%s stdout=%q", err, apperror.KindOf(err), apperror.CodeOf(err), stdout.String())
	}
	public := apperror.PublicMessage(err)
	if public != "local-model ranking failed" {
		t.Fatalf("internal public message=%q", public)
	}
	assertIdeasCanariesAbsent(t, []byte(public), endpointCanary, modelCanary, promptCanary, responseCanary, errorCanary)
	if _, statErr := os.Stat(internalOutput); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("internal error published output: %v", statErr)
	}
}

func TestUsageAdvertisesIdeasExtract(t *testing.T) {
	var output bytes.Buffer
	if err := printUsage(&output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"mindweaver ideas extract",
		"mindweaver ideas extract -session",
		"[-ollama-model <model>]",
		"mindweaver ideas sessions",
		"mindweaver ideas hooks print|install|status",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("usage missing %q: %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "ideas hook codex") {
		t.Fatalf("usage exposed internal callback: %q", output.String())
	}
	if !strings.Contains(output.String(), "mindweaver ideas extract") {
		t.Fatalf("usage=%q", output.String())
	}
}

func TestCodexHookProcessContract(t *testing.T) {
	tests := []struct {
		name       string
		capture    func(context.Context, io.Reader) error
		wantExit   int
		wantStderr string
	}{
		{
			name: "success is silent",
			capture: func(context.Context, io.Reader) error {
				return nil
			},
			wantExit: 0,
		},
		{
			name: "failure is stable and content free",
			capture: func(context.Context, io.Reader) error {
				return errors.New("secret-canary")
			},
			wantExit:   1,
			wantStderr: "ideas.hook_storage_failed\n",
		},
		{
			name: "panic is contained",
			capture: func(context.Context, io.Reader) error {
				panic("secret-canary")
			},
			wantExit:   1,
			wantStderr: "ideas.hook_internal\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			exit := runCodexHookProcessWith(context.Background(), strings.NewReader("secret-canary"), &stderr, test.capture)
			if exit != test.wantExit || stderr.String() != test.wantStderr {
				t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
			}
			if exit == 2 || strings.Contains(stderr.String(), "secret-canary") {
				t.Fatalf("unsafe hook result exit=%d stderr=%q", exit, stderr.String())
			}
		})
	}
}

func TestCodexHookDispatchIsExactAndNeverPublic(t *testing.T) {
	if !isCodexHookCommand([]string{"ideas", "hook", "codex"}) {
		t.Fatal("exact internal callback was not recognized")
	}
	for _, args := range [][]string{
		{"ideas", "hook"},
		{"ideas", "hook", "Codex"},
		{"ideas", "hook", "codex", "extra"},
		{"ideas", "hooks", "codex"},
	} {
		if isCodexHookCommand(args) {
			t.Fatalf("variant reached internal callback: %q", args)
		}
		var stdout bytes.Buffer
		err := runWithIO(context.Background(), args, strings.NewReader("secret-canary"), &stdout)
		if err == nil || apperror.KindOf(err) != apperror.KindInvalid || stdout.Len() != 0 {
			t.Fatalf("variant=%q err=%v kind=%s stdout=%q", args, err, apperror.KindOf(err), stdout.String())
		}
	}
}

func TestIdeasExtractRequiresOneInputAuthority(t *testing.T) {
	sessionID := "sha256:" + strings.Repeat("a", 64)
	for _, args := range [][]string{
		{"ideas", "extract", "-session", sessionID, "-input", "owner.json", "-output", "report"},
		{"ideas", "extract", "-session", sessionID, "-input-format", "note", "-output", "report"},
		{"ideas", "extract", "-session=", "-output", "report"},
		{"ideas", "extract", "-session=", "-input", "owner.json", "-output", "report"},
	} {
		var stdout bytes.Buffer
		err := runWithIO(context.Background(), args, strings.NewReader("secret-canary"), &stdout)
		if err == nil || apperror.KindOf(err) != apperror.KindInvalid || stdout.Len() != 0 {
			t.Fatalf("args=%q err=%v kind=%s stdout=%q", args, err, apperror.KindOf(err), stdout.String())
		}
	}
}

func TestIdeasCurrentExtractUsesOnlyTheCurrentWorkingDirectorySelector(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "repo")
	outputDirectory := filepath.Join(t.TempDir(), "report")
	request := sessiondistill.Request{
		SchemaVersion: sessiondistill.SchemaVersion,
		SessionID:     "sha256:" + strings.Repeat("a", 64),
		Turns: []sessiondistill.VisibleTurn{{
			EventID: "sha256:" + strings.Repeat("b", 64), Ordinal: 1,
			Role: sessiondistill.RoleUser, Text: "我的想法是使用安全的当前会话选择。",
		}},
	}
	previousWorkingDirectory := ideasWorkingDirectory
	previousCurrentRequest := ideasCurrentSessionRequest
	ideasWorkingDirectory = func() (string, error) { return workingDirectory, nil }
	ideasCurrentSessionRequest = func(ctx context.Context, gotWorkingDirectory string) (sessiondistill.Request, error) {
		if ctx == nil || gotWorkingDirectory != workingDirectory {
			t.Fatalf("working directory = %q", gotWorkingDirectory)
		}
		return request, nil
	}
	t.Cleanup(func() {
		ideasWorkingDirectory = previousWorkingDirectory
		ideasCurrentSessionRequest = previousCurrentRequest
	})

	var stdout bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "current", "extract", "-output", outputDirectory}, strings.NewReader("ignored"), &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "created ideas report (1 user items, 0 assistant context items)\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(outputDirectory, "report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestIdeasCurrentExtractSupportsOptionalOllamaOnTheSelectedSession(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "repo")
	outputDirectory := filepath.Join(t.TempDir(), "report")
	request := sessiondistill.Request{
		SchemaVersion: sessiondistill.SchemaVersion,
		SessionID:     "sha256:" + strings.Repeat("a", 64),
		Turns: []sessiondistill.VisibleTurn{{
			EventID: "sha256:" + strings.Repeat("b", 64), Ordinal: 1,
			Role: sessiondistill.RoleUser, Text: "我的想法是只增强当前会话。",
		}},
	}
	previousWorkingDirectory := ideasWorkingDirectory
	previousCurrentRequest := ideasCurrentSessionRequest
	previousRank := ideasRankCandidates
	ideasWorkingDirectory = func() (string, error) { return workingDirectory, nil }
	ideasCurrentSessionRequest = func(ctx context.Context, gotWorkingDirectory string) (sessiondistill.Request, error) {
		if ctx == nil || gotWorkingDirectory != workingDirectory {
			t.Fatalf("working directory = %q", gotWorkingDirectory)
		}
		return request, nil
	}
	ideasRankCandidates = func(_ context.Context, options ollama.Options, candidates []ideasollama.Candidate) (ideasollama.Result, error) {
		if options.Model != "current-ranker" || len(candidates) != 1 {
			t.Fatalf("options=%+v candidates=%+v", options, candidates)
		}
		return ideasollama.Result{
			ProviderConfigDigest: "sha256:" + strings.Repeat("f", 64),
			RankedItemIDs:        []string{candidates[0].ItemID},
		}, nil
	}
	t.Cleanup(func() {
		ideasWorkingDirectory = previousWorkingDirectory
		ideasCurrentSessionRequest = previousCurrentRequest
		ideasRankCandidates = previousRank
	})

	if err := runWithIO(context.Background(), []string{
		"ideas", "current", "extract", "-output", outputDirectory, "-ollama-model", "current-ranker",
	}, strings.NewReader("ignored"), io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDirectory, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result sessiondistill.Distillation
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.ModelAssistance == nil || result.ModelAssistance.Status != "completed" ||
		len(result.ModelAssistance.RankedItemIDs) != 1 || result.ModelAssistance.RankedItemIDs[0] != result.UserItems[0].ID {
		t.Fatalf("result=%+v", result)
	}
}

func TestIdeasCurrentExtractRejectsAmbiguousCommandShapesWithZeroOutput(t *testing.T) {
	for _, args := range [][]string{
		{"ideas", "current"},
		{"ideas", "current", "unknown"},
		{"ideas", "current", "extract"},
		{"ideas", "current", "extract", "-output", "report", "extra"},
		{"ideas", "current", "extract", "-output", "report", "-input", "session.json"},
		{"ideas", "current", "extract", "-output", "report", "-session", "sha256:" + strings.Repeat("a", 64)},
	} {
		var stdout bytes.Buffer
		err := runWithIO(context.Background(), args, strings.NewReader("secret-canary"), &stdout)
		if err == nil || apperror.KindOf(err) != apperror.KindInvalid || stdout.Len() != 0 {
			t.Fatalf("args=%q err=%v kind=%s stdout=%q", args, err, apperror.KindOf(err), stdout.String())
		}
	}
}

func TestIdeasCurrentExtractMapsSelectionFailureWithoutOutputOrIdentityLeak(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "secret-project")
	previousWorkingDirectory := ideasWorkingDirectory
	previousCurrentRequest := ideasCurrentSessionRequest
	previousCodeOf := ideasHookCodeOf
	ideasWorkingDirectory = func() (string, error) { return workingDirectory, nil }
	t.Cleanup(func() {
		ideasWorkingDirectory = previousWorkingDirectory
		ideasCurrentSessionRequest = previousCurrentRequest
		ideasHookCodeOf = previousCodeOf
	})

	tests := []struct {
		name     string
		code     string
		wantKind apperror.Kind
	}{
		{name: "not found", code: ideashook.CodeNotFound, wantKind: apperror.KindNotFound},
		{name: "identity conflict", code: ideashook.CodeIdentityConflict, wantKind: apperror.KindConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canary := "secret-session-project-canary"
			selectorErr := errors.New(canary + " " + workingDirectory)
			ideasCurrentSessionRequest = func(context.Context, string) (sessiondistill.Request, error) {
				return sessiondistill.Request{}, selectorErr
			}
			ideasHookCodeOf = func(err error) string {
				if err != selectorErr {
					t.Fatalf("unexpected selector error: %v", err)
				}
				return test.code
			}
			outputDirectory := filepath.Join(t.TempDir(), "report")
			var stdout bytes.Buffer
			err := runWithIO(context.Background(), []string{"ideas", "current", "extract", "-output", outputDirectory}, strings.NewReader("ignored"), &stdout)
			if err == nil || apperror.KindOf(err) != test.wantKind || stdout.Len() != 0 {
				t.Fatalf("err=%v kind=%s stdout=%q", err, apperror.KindOf(err), stdout.String())
			}
			if _, statErr := os.Stat(outputDirectory); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("output exists: %v", statErr)
			}
			public := apperror.PublicMessage(err)
			if strings.Contains(public, canary) || strings.Contains(public, workingDirectory) || strings.Contains(public, "sha256:") {
				t.Fatalf("public error leaked selector identity: %q", public)
			}
		})
	}
}

func TestIdeasHooksRequiresExplicitScopeAndKnownCommand(t *testing.T) {
	for _, args := range [][]string{
		{"ideas", "hooks", "status"},
		{"ideas", "hooks", "status", "--scope", "workspace"},
		{"ideas", "hooks", "unknown", "--scope", "user"},
	} {
		var stdout bytes.Buffer
		err := runWithIO(context.Background(), args, strings.NewReader(""), &stdout)
		if err == nil || apperror.KindOf(err) != apperror.KindInvalid || stdout.Len() != 0 {
			t.Fatalf("args=%q err=%v kind=%s stdout=%q", args, err, apperror.KindOf(err), stdout.String())
		}
	}
}

func TestIdeasHookConfigResultIsContentFreeAndWarnsForRepoScope(t *testing.T) {
	var output bytes.Buffer
	err := writeIdeasHookConfigResult(&output, "install", ideashook.HookConfigResult{
		Scope: ideashook.HookScopeRepo, State: ideashook.HookConfigInstalled,
		Activation: ideashook.HookActivationState, ManagedEvents: 4, Changed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["state"] != "installed" || decoded["activation"] != "unknown" || decoded["managed_events"] != float64(4) {
		t.Fatalf("decoded=%#v", decoded)
	}
	if decoded["warning"] != "repository_hook_configuration_contains_a_machine_local_executable_path_do_not_commit" ||
		decoded["next_step"] != "review_and_trust_with_slash_hooks_then_start_a_new_codex_task_to_verify_activation" {
		t.Fatalf("decoded=%#v", decoded)
	}
	if strings.Contains(output.String(), `D:\\`) || strings.Contains(output.String(), "24281") {
		t.Fatalf("machine path leaked: %q", output.String())
	}
}

func TestIdeasHooksUserScopePrintInstallAndStatus(t *testing.T) {
	codexHome := filepath.Join(t.TempDir(), "codex-home")
	if err := os.Mkdir(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	executable := filepath.Join(t.TempDir(), "mindweaver.exe")
	if err := os.WriteFile(executable, []byte("test executable identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousExecutablePath := ideasExecutablePath
	ideasExecutablePath = func() (string, error) { return executable, nil }
	t.Cleanup(func() { ideasExecutablePath = previousExecutablePath })

	var statusOutput bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "hooks", "status", "--scope", "user"}, strings.NewReader(""), &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil || status["state"] != "absent" || status["activation"] != "unknown" {
		t.Fatalf("status=%#v err=%v", status, err)
	}

	var preview bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "hooks", "print", "--scope", "user"}, strings.NewReader(""), &preview); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("print mutated hooks.json: %v", err)
	}
	assertExactIdeasHookPreview(t, preview.Bytes(), executable)

	var installOutput bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "hooks", "install", "--scope", "user"}, strings.NewReader(""), &installOutput); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(codexHome, "hooks.json"))
	if err != nil || !bytes.Equal(installed, preview.Bytes()) {
		t.Fatalf("installed differs from preview err=%v", err)
	}
	var install map[string]any
	if err := json.Unmarshal(installOutput.Bytes(), &install); err != nil || install["state"] != "installed" || install["changed"] != true {
		t.Fatalf("install=%#v err=%v", install, err)
	}

	var replayOutput bytes.Buffer
	if err := runWithIO(context.Background(), []string{"ideas", "hooks", "install", "--scope", "user"}, strings.NewReader(""), &replayOutput); err != nil {
		t.Fatal(err)
	}
	var replay map[string]any
	if err := json.Unmarshal(replayOutput.Bytes(), &replay); err != nil || replay["changed"] != false {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
}

func assertExactIdeasHookPreview(t *testing.T, preview []byte, executable string) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(preview, &top); err != nil || len(top) != 1 {
		t.Fatalf("preview top-level shape=%v err=%v", top, err)
	}
	var hooks map[string][]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooks); err != nil || len(hooks) != 4 {
		t.Fatalf("preview hooks=%v err=%v", hooks, err)
	}
	expectedCommand := `"` + executable + `" ideas hook codex`
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd"} {
		groups, exists := hooks[event]
		if !exists || len(groups) != 1 {
			t.Fatalf("preview event %s groups=%d exists=%v", event, len(groups), exists)
		}
		var group map[string]json.RawMessage
		if err := json.Unmarshal(groups[0], &group); err != nil || len(group) != 1 {
			t.Fatalf("preview event %s group=%v err=%v", event, group, err)
		}
		var handlers []map[string]json.RawMessage
		if err := json.Unmarshal(group["hooks"], &handlers); err != nil || len(handlers) != 1 {
			t.Fatalf("preview event %s handlers=%v err=%v", event, handlers, err)
		}
		handler := handlers[0]
		if len(handler) != 6 {
			t.Fatalf("preview event %s handler fields=%v", event, handler)
		}
		var actual struct {
			Type           string `json:"type"`
			Command        string `json:"command"`
			CommandWindows string `json:"commandWindows"`
			Timeout        uint64 `json:"timeout"`
			Async          bool   `json:"async"`
			StatusMessage  string `json:"statusMessage"`
		}
		encoded, err := json.Marshal(handler)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &actual); err != nil ||
			actual.Type != "command" || actual.Command != expectedCommand ||
			actual.CommandWindows != expectedCommand || actual.Timeout != 3 ||
			actual.Async || actual.StatusMessage != ideashook.HookConfigMarker {
			t.Fatalf("preview event %s handler=%+v err=%v", event, actual, err)
		}
	}
}

func assertIdeasCanariesAbsent(t *testing.T, artifact []byte, canaries ...string) {
	t.Helper()
	for _, canary := range canaries {
		if canary == "" || bytes.Contains(artifact, []byte(canary)) {
			t.Fatalf("artifact retained raw canary %q: %s", canary, artifact)
		}
	}
}

func assertExactIdeasModelAssistanceFields(t *testing.T, reportJSON []byte, withLimitation bool) {
	t.Helper()
	var report map[string]json.RawMessage
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		t.Fatal(err)
	}
	wantReportFields := []string{
		"schema_version", "policy_version", "session_id", "input_digest", "untrusted_visible_content",
		"redacted_count", "user_items", "assistant_context", "relations", "model_assistance",
	}
	if len(report) != len(wantReportFields) {
		t.Fatalf("report fields=%v", report)
	}
	for _, field := range wantReportFields {
		if _, exists := report[field]; !exists {
			t.Fatalf("report missing field %q: %v", field, report)
		}
	}

	var assistance map[string]json.RawMessage
	if err := json.Unmarshal(report["model_assistance"], &assistance); err != nil {
		t.Fatal(err)
	}
	wantAssistanceFields := []string{
		"schema_version", "provider", "purpose", "status", "provider_config_digest", "input_digest", "ranked_item_ids",
	}
	if withLimitation {
		wantAssistanceFields = append(wantAssistanceFields, "limitation_code")
	}
	if len(assistance) != len(wantAssistanceFields) {
		t.Fatalf("model assistance fields=%v", assistance)
	}
	for _, field := range wantAssistanceFields {
		if _, exists := assistance[field]; !exists {
			t.Fatalf("model assistance missing field %q: %v", field, assistance)
		}
	}
}
