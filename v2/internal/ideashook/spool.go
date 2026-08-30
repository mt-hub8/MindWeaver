package ideashook

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/sessiondistill"
)

const (
	spoolSchemaVersion           = 1
	maxSessionEvents             = 1000
	maxSessionBytes              = 32 << 20
	maxGlobalSessions            = 256
	maxGlobalEvents              = 4096
	maxGlobalBytes               = 256 << 20
	maxPendingRecords            = 64
	maxPendingSessionDirectories = 64
	maxSessionDirectoryEntries   = maxSessionEvents + maxPendingRecords
	maxGlobalDirectoryEntries    = maxGlobalSessions + maxPendingSessionDirectories
	maxSpoolRecordBytes          = maxVisibleBytes + maxMetadataBytes
	spoolKeyBytes                = 32
)

var eventFilePattern = regexp.MustCompile(`^([0-9]{5})-([0-9a-f]{64})\.json$`)
var sessionDirectoryPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type spoolRecord struct {
	SchemaVersion int    `json:"schema_version"`
	EventID       string `json:"event_id"`
	SessionID     string `json:"session_id"`
	ProjectID     string `json:"project_id"`
	TurnID        string `json:"turn_id,omitempty"`
	Ordinal       uint32 `json:"ordinal"`
	HookEvent     string `json:"hook_event"`
	Kind          string `json:"kind"`
	Role          string `json:"role,omitempty"`
	Content       string `json:"content,omitempty"`
	Boundary      string `json:"boundary,omitempty"`
	CapturedAt    string `json:"captured_at"`
	RecordMAC     string `json:"record_mac"`
}

type recordCandidate struct {
	sessionID string
	projectID string
	turnID    string
	hookEvent string
	kind      string
	role      string
	content   string
	boundary  string
	dedupeID  string
}

type globalSpoolUsage struct {
	sessions        int
	events          int
	bytes           int64
	candidateExists bool
}

type spoolAdmissionLimits struct {
	sessionEvents  int
	sessionBytes   int64
	globalSessions int
	globalEvents   int
	globalBytes    int64
}

type captureSpoolHooks struct {
	afterSessionsRootRetained     func() error
	afterSessionDirectoryRetained func() error
	beforeRecordRename            func(string) error
	beforeDirectorySync           func() error
}

type captureSpoolOptions struct {
	limits spoolAdmissionLimits
	hooks  captureSpoolHooks
}

type retainedSpoolInspection struct {
	usage            globalSpoolUsage
	sessionsRoot     *os.File
	sessionDirectory *os.File
}

func productionSpoolAdmissionLimits() spoolAdmissionLimits {
	return spoolAdmissionLimits{
		sessionEvents:  maxSessionEvents,
		sessionBytes:   maxSessionBytes,
		globalSessions: maxGlobalSessions,
		globalEvents:   maxGlobalEvents,
		globalBytes:    maxGlobalBytes,
	}
}

func Capture(ctx context.Context, reader io.Reader) error {
	event, err := decodeHookInput(reader)
	if err != nil || event.ignored {
		return err
	}
	root, err := resolveSpoolRoot()
	if err != nil {
		return hookError(CodeUnsupported, err)
	}
	return captureAtRoot(ctx, root, event, time.Now)
}

func captureAtRoot(ctx context.Context, root string, event captureEvent, now func() time.Time) error {
	return captureAtRootWithOptions(ctx, root, event, now, captureSpoolOptions{limits: productionSpoolAdmissionLimits()})
}

func captureAtRootWithOptions(ctx context.Context, root string, event captureEvent, now func() time.Time, options captureSpoolOptions) (resultErr error) {
	if event.ignored {
		return nil
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if root == "" || now == nil {
		return hookError(CodeStorage, fmt.Errorf("spool root"))
	}
	if event.role != "" {
		safe, _, err := sessiondistill.RedactVisibleText(event.content)
		if err != nil {
			return hookError(CodeInvalidInput, err)
		}
		event.content = safe
	}
	if err := prepareSpoolRoot(root); err != nil {
		return hookError(CodeStorage, err)
	}
	lock, err := acquireSpoolLock(ctx, root)
	if err != nil {
		return err
	}
	defer lock.Close()
	key, err := loadOrCreateSpoolKey(root)
	if err != nil {
		return hookError(CodeStorage, err)
	}
	candidate := candidateForEvent(key, event)
	sessionsRoot := filepath.Join(root, "sessions")
	if err := ensureOwnerOnlyDirectory(sessionsRoot); err != nil {
		return hookError(CodeStorage, err)
	}
	inspection, err := inspectGlobalSpoolRetained(sessionsRoot, candidate.sessionID, options.hooks.afterSessionsRootRetained)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := inspection.Close(); resultErr == nil && closeErr != nil {
			resultErr = hookError(CodeStorage, closeErr)
		}
	}()
	usage := inspection.usage
	sessionDirectory := filepath.Join(sessionsRoot, strings.TrimPrefix(candidate.sessionID, "sha256:"))
	var records []spoolRecord
	var totalBytes int64
	if usage.candidateExists {
		if inspection.sessionDirectory == nil {
			return hookError(CodeIntegrity, fmt.Errorf("session directory witness"))
		}
		if hook := options.hooks.afterSessionDirectoryRetained; hook != nil {
			if err := hook(); err != nil {
				return hookError(CodeStorage, err)
			}
		}
		records, totalBytes, err = readSessionRecords(sessionDirectory, candidate.sessionID, key)
		if err != nil {
			return err
		}
	}
	for _, record := range records {
		if recordDedupeID(key, record) != candidate.dedupeID {
			continue
		}
		if !recordMatchesCandidate(record, candidate) {
			return hookError(CodeIdentityConflict, fmt.Errorf("spool replay conflict"))
		}
		return nil
	}
	eventID, err := randomOpaqueID()
	if err != nil {
		return hookError(CodeStorage, err)
	}
	record := spoolRecord{
		SchemaVersion: spoolSchemaVersion,
		EventID:       eventID,
		SessionID:     candidate.sessionID,
		ProjectID:     candidate.projectID,
		TurnID:        candidate.turnID,
		Ordinal:       uint32(len(records) + 1),
		HookEvent:     candidate.hookEvent,
		Kind:          candidate.kind,
		Role:          candidate.role,
		Content:       candidate.content,
		Boundary:      candidate.boundary,
		CapturedAt:    now().UTC().Format(time.RFC3339Nano),
	}
	record.RecordMAC = recordIntegrityMAC(key, record)
	encoded, err := encodeRecord(record)
	if err != nil {
		return hookError(CodeStorage, err)
	}
	if len(encoded) == 0 || len(encoded) > maxSpoolRecordBytes {
		return hookError(CodeLimitExceeded, fmt.Errorf("spool record size"))
	}
	if err := validateSpoolAdmission(options.limits, usage, len(records), totalBytes, int64(len(encoded))); err != nil {
		return err
	}
	created := false
	if !usage.candidateExists {
		created, err = ensureOwnerOnlyDirectoryCreated(sessionDirectory)
		if err != nil {
			if created {
				err = errors.Join(err, removeEmptyOwnerOnlyDirectory(sessionDirectory))
			}
			return hookError(CodeStorage, err)
		}
		if !created {
			return hookError(CodeIntegrity, fmt.Errorf("session directory changed after inspection"))
		}
		inspection.sessionDirectory, err = openOwnerOnlyDirectory(sessionDirectory)
		if err != nil {
			cleanupErr := removeEmptyOwnerOnlyDirectory(sessionDirectory)
			return hookError(CodeStorage, errors.Join(err, cleanupErr))
		}
	}
	if !usage.candidateExists && options.hooks.afterSessionDirectoryRetained != nil {
		hook := options.hooks.afterSessionDirectoryRetained
		if err := hook(); err != nil {
			if created {
				closeErr := inspection.closeSessionDirectory()
				removeErr := removeEmptyOwnerOnlyDirectory(sessionDirectory)
				err = errors.Join(err, closeErr, removeErr)
			}
			return hookError(CodeStorage, err)
		}
	}
	fileName := fmt.Sprintf("%05d-%s.json", record.Ordinal, candidate.dedupeID)
	if err := writeOwnerOnlyRecord(ctx, inspection.sessionDirectory, sessionDirectory, fileName, encoded, options.hooks); err != nil {
		if created {
			closeErr := inspection.closeSessionDirectory()
			err = errors.Join(err, closeErr, removeEmptyOwnerOnlyDirectory(sessionDirectory))
		}
		if errors.Is(err, os.ErrExist) {
			return hookError(CodeIdentityConflict, err)
		}
		return hookError(CodeStorage, err)
	}
	return nil
}

func validateSpoolAdmission(limits spoolAdmissionLimits, usage globalSpoolUsage, sessionEvents int, sessionBytes, recordBytes int64) error {
	if limits.sessionEvents <= 0 || limits.sessionBytes <= 0 || limits.globalSessions <= 0 || limits.globalEvents <= 0 || limits.globalBytes <= 0 ||
		usage.sessions < 0 || usage.events < 0 || usage.bytes < 0 || sessionEvents < 0 || sessionBytes < 0 || recordBytes <= 0 {
		return hookError(CodeStorage, fmt.Errorf("spool admission limits"))
	}
	if !usage.candidateExists && usage.sessions+1 > limits.globalSessions {
		return hookError(CodeLimitExceeded, fmt.Errorf("global session limit"))
	}
	if sessionEvents+1 > limits.sessionEvents {
		return hookError(CodeLimitExceeded, fmt.Errorf("session event limit"))
	}
	if sessionBytes+recordBytes > limits.sessionBytes {
		return hookError(CodeLimitExceeded, fmt.Errorf("session byte limit"))
	}
	if usage.events+1 > limits.globalEvents || usage.bytes+recordBytes > limits.globalBytes {
		return hookError(CodeLimitExceeded, fmt.Errorf("global spool limit"))
	}
	return nil
}

func candidateForEvent(key []byte, event captureEvent) recordCandidate {
	candidate := recordCandidate{
		sessionID: "sha256:" + keyedDigest(key, "mindweaver/ideas-hook/session/v1", event.rawSession),
		projectID: projectIDForWorkingDirectory(key, event.cwd),
		hookEvent: event.event,
		role:      event.role,
		content:   event.content,
		boundary:  event.boundary,
	}
	if event.rawTurn != "" {
		candidate.turnID = "sha256:" + keyedDigest(key, "mindweaver/ideas-hook/turn/v1", event.rawSession, event.rawTurn)
	}
	if event.role == "" {
		candidate.kind = "boundary"
	} else {
		candidate.kind = "message"
	}
	candidate.dedupeID = keyedDigest(
		key,
		"mindweaver/ideas-hook/event/v1",
		candidate.sessionID,
		candidate.turnID,
		candidate.projectID,
		candidate.hookEvent,
		candidate.role,
		candidate.content,
		candidate.boundary,
	)
	return candidate
}

func projectIDForWorkingDirectory(key []byte, workingDirectory string) string {
	canonical := filepath.Clean(workingDirectory)
	return "sha256:" + keyedDigest(key, "mindweaver/ideas-hook/project/v1", canonical)
}

func recordDedupeID(key []byte, record spoolRecord) string {
	return keyedDigest(
		key,
		"mindweaver/ideas-hook/event/v1",
		record.SessionID,
		record.TurnID,
		record.ProjectID,
		record.HookEvent,
		record.Role,
		record.Content,
		record.Boundary,
	)
}

func recordIntegrityMAC(key []byte, record spoolRecord) string {
	return keyedDigest(
		key,
		"mindweaver/ideas-hook/record-integrity/v1",
		strconv.Itoa(record.SchemaVersion),
		record.EventID,
		record.SessionID,
		record.ProjectID,
		record.TurnID,
		strconv.FormatUint(uint64(record.Ordinal), 10),
		record.HookEvent,
		record.Kind,
		record.Role,
		record.Content,
		record.Boundary,
		record.CapturedAt,
	)
}

func keyedDigest(key []byte, domain string, values ...string) string {
	hash := hmac.New(sha256.New, key)
	hash.Write([]byte(domain))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		hash.Write(length[:])
		hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func randomOpaqueID() (string, error) {
	value := make([]byte, sha256.Size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(value), nil
}

func loadOrCreateSpoolKey(root string) ([]byte, error) {
	path := filepath.Join(root, "dedupe.key")
	file, err := createOwnerOnlyFile(path)
	if err == nil {
		key := make([]byte, spoolKeyBytes)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return nil, err
		}
		writeErr := writeAll(file, key)
		secureErr := secureOwnerOnlyFile(file)
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(writeErr, secureErr, syncErr, closeErr); err != nil {
			_ = os.Remove(path)
			return nil, err
		}
		if err := syncOwnerDirectory(root); err != nil {
			return nil, err
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return loadSpoolKey(root)
}

func loadSpoolKey(root string) ([]byte, error) {
	path := filepath.Join(root, "dedupe.key")
	file, err := openOwnerOnlyFile(path)
	if err != nil {
		return nil, err
	}
	key, readErr := io.ReadAll(io.LimitReader(file, spoolKeyBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(key) != spoolKeyBytes {
		return nil, errors.Join(readErr, closeErr, fmt.Errorf("spool key"))
	}
	return key, nil
}

func inspectGlobalSpool(sessionsRoot, candidateSessionID string) (globalSpoolUsage, error) {
	inspection, err := inspectGlobalSpoolRetained(sessionsRoot, candidateSessionID, nil)
	if err != nil {
		return globalSpoolUsage{}, err
	}
	usage := inspection.usage
	if err := inspection.Close(); err != nil {
		return globalSpoolUsage{}, hookError(CodeIntegrity, err)
	}
	return usage, nil
}

func inspectGlobalSpoolRetained(sessionsRoot, candidateSessionID string, afterRootRetained func() error) (result retainedSpoolInspection, resultErr error) {
	directoryHandle, entries, err := openBoundedOwnerOnlyDirectory(sessionsRoot, maxGlobalDirectoryEntries)
	if err != nil {
		return retainedSpoolInspection{}, hookError(CodeIntegrity, err)
	}
	result.sessionsRoot = directoryHandle
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, result.Close())
		}
	}()
	if afterRootRetained != nil {
		if err := afterRootRetained(); err != nil {
			return result, hookError(CodeStorage, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	wanted := strings.TrimPrefix(candidateSessionID, "sha256:")
	for _, sessionEntry := range entries {
		if !sessionDirectoryPattern.MatchString(sessionEntry.Name()) || !sessionEntry.IsDir() || sessionEntry.Type()&os.ModeSymlink != 0 {
			return result, hookError(CodeIntegrity, fmt.Errorf("spool session entry"))
		}
		directory := filepath.Join(sessionsRoot, sessionEntry.Name())
		sessionHandle, files, err := openBoundedOwnerOnlyDirectory(directory, maxSessionDirectoryEntries)
		if err != nil {
			return result, hookError(CodeIntegrity, err)
		}
		files, err = cleanupPendingRecords(directory, files)
		if err != nil {
			return result, hookError(CodeIntegrity, errors.Join(err, sessionHandle.Close()))
		}
		if len(files) == 0 {
			if err := sessionHandle.Close(); err != nil {
				return result, hookError(CodeIntegrity, err)
			}
			if err := removeEmptyOwnerOnlyDirectory(directory); err != nil {
				return result, hookError(CodeIntegrity, err)
			}
			continue
		}
		result.usage.sessions++
		if result.usage.sessions > maxGlobalSessions {
			return result, hookError(CodeIntegrity, errors.Join(sessionHandle.Close(), fmt.Errorf("global session limit")))
		}
		if sessionEntry.Name() == wanted {
			result.usage.candidateExists = true
			result.sessionDirectory = sessionHandle
		} else if err := sessionHandle.Close(); err != nil {
			return result, hookError(CodeIntegrity, err)
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
		if len(files) > maxSessionEvents {
			return result, hookError(CodeIntegrity, fmt.Errorf("session event limit"))
		}
		var inspectedSessionBytes int64
		for index, fileEntry := range files {
			match := eventFilePattern.FindStringSubmatch(fileEntry.Name())
			if len(match) != 3 || fileEntry.IsDir() || fileEntry.Type()&os.ModeSymlink != 0 {
				return result, hookError(CodeIntegrity, fmt.Errorf("spool event entry"))
			}
			ordinal, parseErr := strconv.Atoi(match[1])
			info, infoErr := fileEntry.Info()
			if parseErr != nil || infoErr != nil || ordinal != index+1 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSpoolRecordBytes {
				return result, hookError(CodeIntegrity, errors.Join(parseErr, infoErr, fmt.Errorf("spool event shape")))
			}
			result.usage.events++
			result.usage.bytes += info.Size()
			inspectedSessionBytes += info.Size()
			if inspectedSessionBytes > maxSessionBytes {
				return result, hookError(CodeIntegrity, fmt.Errorf("session byte limit"))
			}
			if result.usage.events > maxGlobalEvents || result.usage.bytes > maxGlobalBytes {
				return result, hookError(CodeIntegrity, fmt.Errorf("global spool limit"))
			}
		}
	}
	return result, nil
}

func (inspection *retainedSpoolInspection) closeSessionDirectory() error {
	if inspection == nil || inspection.sessionDirectory == nil {
		return nil
	}
	err := inspection.sessionDirectory.Close()
	inspection.sessionDirectory = nil
	return err
}

func (inspection *retainedSpoolInspection) Close() error {
	if inspection == nil {
		return nil
	}
	sessionErr := inspection.closeSessionDirectory()
	var rootErr error
	if inspection.sessionsRoot != nil {
		rootErr = inspection.sessionsRoot.Close()
		inspection.sessionsRoot = nil
	}
	return errors.Join(sessionErr, rootErr)
}

func readSessionRecords(directory, sessionID string, key []byte) ([]spoolRecord, int64, error) {
	directoryHandle, entries, err := openBoundedOwnerOnlyDirectory(directory, maxSessionDirectoryEntries)
	if err != nil {
		return nil, 0, hookError(CodeIntegrity, err)
	}
	defer directoryHandle.Close()
	entries, err = cleanupPendingRecords(directory, entries)
	if err != nil {
		return nil, 0, hookError(CodeIntegrity, err)
	}
	if len(entries) > maxSessionEvents {
		return nil, 0, hookError(CodeIntegrity, fmt.Errorf("session event limit"))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	records := make([]spoolRecord, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		match := eventFilePattern.FindStringSubmatch(entry.Name())
		if len(match) != 3 || entry.IsDir() {
			return nil, 0, hookError(CodeIntegrity, fmt.Errorf("spool entry"))
		}
		ordinal, err := strconv.Atoi(match[1])
		if err != nil || ordinal != len(records)+1 {
			return nil, 0, hookError(CodeIntegrity, fmt.Errorf("spool ordinal"))
		}
		path := filepath.Join(directory, entry.Name())
		file, err := openOwnerOnlyFile(path)
		if err != nil {
			return nil, 0, hookError(CodeIntegrity, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxSpoolRecordBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) == 0 || len(data) > maxSpoolRecordBytes {
			return nil, 0, hookError(CodeIntegrity, errors.Join(readErr, closeErr, fmt.Errorf("spool record size")))
		}
		var record spoolRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, 0, hookError(CodeIntegrity, err)
		}
		canonical, err := encodeRecord(record)
		if err != nil || !hmac.Equal(canonical, data) || !validStoredRecord(record, sessionID, uint32(ordinal)) ||
			!hmac.Equal([]byte(recordDedupeID(key, record)), []byte(match[2])) ||
			!hmac.Equal([]byte(recordIntegrityMAC(key, record)), []byte(record.RecordMAC)) {
			return nil, 0, hookError(CodeIntegrity, errors.Join(err, fmt.Errorf("spool record")))
		}
		totalBytes += int64(len(data))
		if totalBytes > maxSessionBytes {
			return nil, 0, hookError(CodeIntegrity, fmt.Errorf("spool byte limit"))
		}
		records = append(records, record)
	}
	return records, totalBytes, nil
}

func validStoredRecord(record spoolRecord, sessionID string, ordinal uint32) bool {
	if record.SchemaVersion != spoolSchemaVersion || record.SessionID != sessionID ||
		record.Ordinal != ordinal ||
		!validOpaque(record.EventID, "sha256:") || !validOpaque(record.SessionID, "sha256:") ||
		!validOpaque(record.ProjectID, "sha256:") ||
		(record.TurnID != "" && !validOpaque(record.TurnID, "sha256:")) ||
		!sessionDirectoryPattern.MatchString(record.RecordMAC) || record.CapturedAt == "" {
		return false
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, record.CapturedAt)
	if err != nil || capturedAt.Location() != time.UTC || capturedAt.Format(time.RFC3339Nano) != record.CapturedAt {
		return false
	}
	if record.Kind == "boundary" {
		return record.Role == "" && record.Content == "" && record.TurnID == "" &&
			(record.HookEvent == "SessionStart" && oneOf(record.Boundary, "startup", "resume", "clear", "compact") ||
				record.HookEvent == "SessionEnd" && record.Boundary == "other")
	}
	return record.Kind == "message" && oneOf(record.Role, "user", "assistant") && validVisible(record.Content) && record.Content == normalizeLines(record.Content) && record.TurnID != "" && record.Boundary == "" &&
		(record.HookEvent == "UserPromptSubmit" && record.Role == "user" || record.HookEvent == "Stop" && record.Role == "assistant")
}

func validOpaque(value, prefix string) bool {
	if len(value) != len(prefix)+sha256.Size*2 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func recordMatchesCandidate(record spoolRecord, candidate recordCandidate) bool {
	return record.SessionID == candidate.sessionID && record.ProjectID == candidate.projectID && record.TurnID == candidate.turnID &&
		record.HookEvent == candidate.hookEvent && record.Kind == candidate.kind && record.Role == candidate.role &&
		record.Content == candidate.content && record.Boundary == candidate.boundary
}

func encodeRecord(record spoolRecord) ([]byte, error) {
	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}

func openBoundedOwnerOnlyDirectory(path string, maximum int) (*os.File, []os.DirEntry, error) {
	directory, err := openOwnerOnlyDirectory(path)
	if err != nil {
		return nil, nil, err
	}
	entries, err := readBoundedDirectoryEntries(directory, maximum)
	if err != nil {
		_ = directory.Close()
		return nil, nil, err
	}
	return directory, entries, nil
}

func readBoundedDirectoryEntries(directory *os.File, maximum int) ([]os.DirEntry, error) {
	if directory == nil || maximum < 0 {
		return nil, fmt.Errorf("directory enumeration")
	}
	entries := make([]os.DirEntry, 0, min(maximum, 64))
	for {
		batch, err := directory.ReadDir(64)
		entries = append(entries, batch...)
		if len(entries) > maximum {
			return nil, fmt.Errorf("directory entry limit")
		}
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func writeAll(writer io.Writer, content []byte) error {
	for len(content) != 0 {
		written, err := writer.Write(content)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return hookError(CodeCanceled, ctx.Err())
	default:
		return nil
	}
}
