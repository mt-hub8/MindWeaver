package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	residueReceiptVersion    = 1
	residueBindingVersion    = 2
	maxResidueReceiptBytes   = 4 << 10
	maxResidueBindingBytes   = 4 << 10
	maxResiduePageSize       = 256
	maxResidueScanEntries    = maxExactTreeEntries
	residueReceiptPrefix     = ".mindweaver-residue-"
	residueReceiptSuffix     = ".json"
	residueBindingSuffix     = ".identity.json"
	residueTempPrefix        = ".mindweaver-residue-temp-"
	startupVerifyScratchLeaf = "MindWeaver-Recovery-Verify-v1"
	liveVerifyScratchPrefix  = "MindWeaver-Backup-Live-Verify-v1-"
)

var (
	ErrResidueActive   = errors.New("backup: residue operation is still active")
	ErrResidueConflict = errors.New("backup: residue changed since it was listed")
)

// ResidueState is derived from the immutable receipt and the retained parent
// topology. The receipt itself is never rewritten, avoiding torn state updates.
type ResidueState string

const (
	ResidueStateStaging              ResidueState = "staging"
	ResidueStatePublicationUncertain ResidueState = "publication_uncertain"
	ResidueStatePublicationCleanup   ResidueState = "publication_cleanup_pending"
	ResidueStateReceiptOnly          ResidueState = "receipt_only"
	ResidueStateConflict             ResidueState = "conflict"
)

// Residue is an immutable compare-and-swap token returned by ListResidues.
// Identity and Revision are opaque and both are required by RecoverResidue.
type Residue struct {
	ID              string       `json:"id"`
	Revision        string       `json:"revision"`
	Identity        string       `json:"identity"`
	Kind            string       `json:"kind"`
	State           ResidueState `json:"state"`
	StagingName     string       `json:"staging_name"`
	DestinationName string       `json:"destination_name"`
}

// ResiduePage is bounded by both the requested page size and a fixed maximum
// number of direct parent entries scanned. No prefix-only directory is owned.
type ResiduePage struct {
	Items     []Residue `json:"items"`
	Truncated bool      `json:"truncated"`
}

type residueReceipt struct {
	Version         int    `json:"version"`
	ID              string `json:"id"`
	Revision        string `json:"revision"`
	ParentIdentity  string `json:"parent_identity"`
	Kind            string `json:"kind"`
	StagingName     string `json:"staging_name"`
	DestinationName string `json:"destination_name"`
}

// residueBinding is written only after the staging directory has been opened
// and assigned a persistent filesystem identity. It is immutable, canonical,
// and published atomically. A receipt authorizes discovery; this second record
// is what authorizes deletion of a staging, cleanup, or destination directory.
type residueBinding struct {
	Version         int    `json:"version"`
	ID              string `json:"id"`
	ReceiptRevision string `json:"receipt_revision"`
	ParentIdentity  string `json:"parent_identity"`
	Kind            string `json:"kind"`
	StagingName     string `json:"staging_name"`
	DestinationName string `json:"destination_name"`
	TreeIdentity    string `json:"tree_identity"`
}

type residueObservation struct {
	state           ResidueState
	identity        string
	stagingInfo     os.FileInfo
	destinationInfo os.FileInfo
	bindingInfo     os.FileInfo
}

func residueKindForPrefix(prefix string) (string, error) {
	switch prefix {
	case stagingPrefix:
		return "backup", nil
	case restorePrefix:
		return "restore", nil
	case verifyScratchPrefix:
		return "verify", nil
	default:
		return "", errors.New("backup: unsupported staging prefix")
	}
}

func validResidueLeaf(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\\`) && filepath.VolumeName(name) == "" && validPlatformLeaf(name)
}

func validDestinationLeaf(name string) bool {
	if !validResidueLeaf(name) {
		return false
	}
	return !managedDestinationLeaf(name)
}

func managedDestinationLeaf(name string) bool {
	for _, prefix := range []string{
		stagingPrefix,
		restorePrefix,
		verifyScratchPrefix,
		residueReceiptPrefix,
		residueTempPrefix,
		startupVerifyScratchLeaf,
		liveVerifyScratchPrefix,
	} {
		if hasPlatformReservedPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func rejectManagedDestinationAncestor(parent *retainedDirectory) error {
	if parent == nil || parent.root == nil || parent.identity.info == nil {
		return errors.New("backup: destination parent capability is unavailable")
	}
	return visitRetainedAncestorsFromWitness(parent, func(directory *retainedDirectory) error {
		managed, err := retainedManagedDestinationAncestor(directory)
		if err != nil {
			return err
		}
		if managed {
			return errors.New("backup: destination is inside a managed staging namespace")
		}
		return nil
	})
}

func validHexToken(value string, encodedBytes int) bool {
	if len(value) != encodedBytes*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == encodedBytes
}

func residueReceiptName(id string) string {
	return residueReceiptPrefix + id + residueReceiptSuffix
}

func residueBindingName(id string) string {
	return residueReceiptPrefix + id + residueBindingSuffix
}

func parseResidueReceiptName(name string) (string, bool) {
	if !strings.HasPrefix(name, residueReceiptPrefix) || !strings.HasSuffix(name, residueReceiptSuffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, residueReceiptPrefix), residueReceiptSuffix)
	return id, validHexToken(id, 16)
}

func parseResidueBindingName(name string) (string, bool) {
	if !strings.HasPrefix(name, residueReceiptPrefix) || !strings.HasSuffix(name, residueBindingSuffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, residueReceiptPrefix), residueBindingSuffix)
	return id, validHexToken(id, 16)
}

func receiptForStaging(staging *stagingDirectory) residueReceipt {
	return residueReceipt{
		Version:         residueReceiptVersion,
		ID:              staging.operationID,
		Revision:        staging.revision,
		ParentIdentity:  staging.parentIdentity,
		Kind:            staging.kind,
		StagingName:     staging.name,
		DestinationName: staging.destinationName,
	}
}

func residueReceiptRevision(receipt residueReceipt) (string, error) {
	receipt.Revision = ""
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateResidueReceipt(receipt residueReceipt) error {
	if receipt.Version != residueReceiptVersion || !validHexToken(receipt.ID, 16) ||
		!validHexToken(receipt.Revision, sha256.Size) ||
		(receipt.Kind != "backup" && receipt.Kind != "restore" && receipt.Kind != "verify") ||
		!validResidueLeaf(receipt.StagingName) || !validDestinationLeaf(receipt.DestinationName) ||
		len(receipt.ParentIdentity) == 0 || len(receipt.ParentIdentity) > 160 {
		return errors.New("backup: invalid residue receipt")
	}
	prefix := stagingPrefix
	if receipt.Kind == "restore" {
		prefix = restorePrefix
	} else if receipt.Kind == "verify" {
		prefix = verifyScratchPrefix
	}
	if receipt.StagingName != prefix+receipt.ID {
		return errors.New("backup: residue staging name differs from its operation ID")
	}
	expectedRevision, err := residueReceiptRevision(receipt)
	if err != nil || expectedRevision != receipt.Revision {
		return errors.New("backup: residue receipt revision mismatch")
	}
	return nil
}

type residueReceiptWriteFunc func(*os.File, []byte) (int, error)

type persistedResidueFile struct {
	file      *os.File
	info      os.FileInfo
	published bool
}

func createResidueReceiptWithHooks(
	parent *retainedDirectory,
	staging *stagingDirectory,
	write residueReceiptWriteFunc,
	afterTempCreate func(*os.File) error,
) error {
	if parent == nil || parent.root == nil || staging == nil {
		return errors.New("backup: invalid residue receipt creation")
	}
	parentIdentity, err := persistentDirectoryIdentityToken(parent)
	if err != nil {
		return fmt.Errorf("backup: retain residue parent identity: %w", err)
	}
	staging.parentIdentity = parentIdentity
	receipt := receiptForStaging(staging)
	receipt.Revision, err = residueReceiptRevision(receipt)
	if err != nil {
		return err
	}
	staging.revision = receipt.Revision
	if err := validateResidueReceipt(receipt); err != nil {
		return err
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxResidueReceiptBytes {
		return errors.New("backup: residue receipt exceeds size limit")
	}
	name := residueReceiptName(staging.operationID)
	persisted, persistErr := persistCanonicalResidueFile(
		parent,
		residueTempName(staging.operationID, "receipt"),
		name,
		encoded,
		true,
		write,
		afterTempCreate,
	)
	if persisted.published {
		staging.receiptName = name
		staging.receiptIdentity = persisted.info
		staging.receiptLease = persisted.file
	}
	if persistErr != nil {
		primary := fmt.Errorf("backup: persist residue receipt atomically: %w", persistErr)
		if persisted.published {
			return errors.Join(primary, removeResidueReceipt(parent, staging))
		}
		return primary
	}
	return nil
}

func residueTempName(id, kind string) string {
	return residueTempPrefix + id + "-" + kind + ".tmp"
}

func persistCanonicalResidueFile(
	parent *retainedDirectory,
	tempName, finalName string,
	encoded []byte,
	lease bool,
	write residueReceiptWriteFunc,
	afterTempCreate func(*os.File) error,
) (persisted persistedResidueFile, resultErr error) {
	if parent == nil || parent.root == nil || parent.syncHandle == nil ||
		!validResidueLeaf(tempName) || !validResidueLeaf(finalName) || len(encoded) == 0 {
		return persistedResidueFile{}, errors.New("backup: invalid canonical residue file")
	}
	file, err := parent.root.OpenFile(tempName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return persistedResidueFile{}, err
	}
	locked := false
	cleanupTemp := func(cause error) (persistedResidueFile, error) {
		info, statErr := file.Stat()
		var unlockErr error
		if locked {
			unlockErr = unlockResidueFile(file)
			locked = false
		}
		closeErr := file.Close()
		var removeErr error
		if info != nil {
			removeErr = removeOwnedReceiptFile(parent, tempName, info)
		}
		var syncErr error
		if removeErr == nil {
			syncErr = syncRetainedDirectory(parent)
		}
		cleanupErr := errors.Join(statErr, unlockErr, closeErr, removeErr, syncErr)
		if cleanupErr != nil {
			cleanupErr = errors.Join(ErrCleanupResidual, cleanupErr)
		}
		return persistedResidueFile{}, errors.Join(cause, cleanupErr)
	}
	if lease {
		if err := lockResidueFile(file, true); err != nil {
			return cleanupTemp(fmt.Errorf("lock residue temp file: %w", err))
		}
		locked = true
	}
	if afterTempCreate != nil {
		if err := afterTempCreate(file); err != nil {
			return cleanupTemp(err)
		}
	}
	if err := file.Truncate(0); err != nil {
		return cleanupTemp(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return cleanupTemp(err)
	}
	if write == nil {
		write = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	}
	if err := writeFullResidueReceipt(file, encoded, write); err != nil {
		return cleanupTemp(err)
	}
	if err := file.Sync(); err != nil {
		return cleanupTemp(err)
	}
	info, err := file.Stat()
	if err != nil {
		return cleanupTemp(err)
	}
	current, err := parent.root.Lstat(tempName)
	if err != nil || !info.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 ||
		!current.Mode().IsRegular() || !os.SameFile(info, current) {
		if err == nil {
			err = errors.New("backup: residue temp identity changed before publication")
		}
		return cleanupTemp(err)
	}
	consumed, err := publishResidueFile(parent, tempName, finalName)
	if err != nil || !consumed {
		if err == nil {
			err = errors.New("backup: residue publication did not consume temp file")
		}
		return cleanupTemp(err)
	}
	persisted = persistedResidueFile{file: file, info: info, published: true}
	finalInfo, finalErr := parent.root.Lstat(finalName)
	_, tempErr := parent.root.Lstat(tempName)
	if tempErr == nil {
		tempErr = errors.New("backup: residue temp name remains after publication")
	} else if errors.Is(tempErr, os.ErrNotExist) {
		tempErr = nil
	}
	if finalErr == nil && (finalInfo.Mode()&os.ModeSymlink != 0 || !finalInfo.Mode().IsRegular() ||
		!os.SameFile(info, finalInfo)) {
		finalErr = errors.New("backup: residue final identity differs from published temp")
	}
	if err := errors.Join(finalErr, tempErr); err != nil {
		return persisted, err
	}
	if err := syncRetainedDirectory(parent); err != nil {
		return persisted, fmt.Errorf("sync residue publication parent: %w", err)
	}
	return persisted, nil
}

func writeFullResidueReceipt(file *os.File, encoded []byte, write residueReceiptWriteFunc) error {
	for offset := 0; offset < len(encoded); {
		written, err := write(file, encoded[offset:])
		if written < 0 || written > len(encoded)-offset {
			return errors.New("backup: invalid residue receipt write count")
		}
		offset += written
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func bindingForStaging(staging *stagingDirectory) residueBinding {
	return residueBinding{
		Version:         residueBindingVersion,
		ID:              staging.operationID,
		ReceiptRevision: staging.revision,
		ParentIdentity:  staging.parentIdentity,
		Kind:            staging.kind,
		StagingName:     staging.name,
		DestinationName: staging.destinationName,
		TreeIdentity:    staging.identityToken,
	}
}

func validateResidueBinding(binding residueBinding, receipt residueReceipt) error {
	if binding.Version != residueBindingVersion || binding.ID != receipt.ID ||
		binding.ReceiptRevision != receipt.Revision || binding.ParentIdentity != receipt.ParentIdentity ||
		binding.Kind != receipt.Kind || binding.StagingName != receipt.StagingName ||
		binding.DestinationName != receipt.DestinationName ||
		len(binding.TreeIdentity) == 0 || len(binding.TreeIdentity) > 256 {
		return errors.New("backup: residue identity binding differs from its receipt")
	}
	return nil
}

func createResidueBinding(parent *retainedDirectory, staging *stagingDirectory) error {
	if parent == nil || parent.root == nil || staging == nil || staging.identityToken == "" {
		return errors.New("backup: invalid residue identity binding creation")
	}
	receipt := receiptForStaging(staging)
	binding := bindingForStaging(staging)
	if err := validateResidueBinding(binding, receipt); err != nil {
		return err
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxResidueBindingBytes {
		return errors.New("backup: residue identity binding exceeds size limit")
	}
	name := residueBindingName(staging.operationID)
	persisted, persistErr := persistCanonicalResidueFile(
		parent,
		residueTempName(staging.operationID, "identity"),
		name,
		encoded,
		false,
		nil,
		nil,
	)
	if persisted.published {
		staging.bindingName = name
		staging.bindingIdentity = persisted.info
	}
	var closeErr error
	if persisted.file != nil {
		closeErr = persisted.file.Close()
	}
	return errors.Join(persistErr, closeErr)
}

func closeResidueLease(staging *stagingDirectory) error {
	if staging == nil || staging.receiptLease == nil {
		return nil
	}
	file := staging.receiptLease
	staging.receiptLease = nil
	return errors.Join(unlockResidueFile(file), file.Close())
}

func removeResidueReceipt(parent *retainedDirectory, staging *stagingDirectory) (resultErr error) {
	if staging == nil || staging.receiptName == "" {
		return nil
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(ErrCleanupResidual, resultErr)
		}
	}()
	if parent == nil || parent.root == nil || staging.receiptIdentity == nil {
		return errors.New("backup: residue receipt identity is unavailable")
	}
	if err := parent.acquireSyncHandle(); err != nil {
		return fmt.Errorf("backup: acquire residue parent sync capability: %w", err)
	}
	info := staging.receiptIdentity
	// Creation keeps a cooperative lease open, while recovery keeps an
	// identity-bound lease. Close either and reacquire the final mutation
	// capability with write sharing denied. The exact canonical receipt and
	// binding are then re-read under those capabilities immediately before the
	// identity-bound deletes.
	if err := closeResidueLease(staging); err != nil {
		return err
	}
	receiptLease, err := openAndLockResidueReceipt(parent, staging.receiptName, info)
	if err != nil {
		return err
	}
	receiptLeaseOpen := true
	defer func() {
		if receiptLeaseOpen {
			resultErr = errors.Join(resultErr, unlockResidueFile(receiptLease), receiptLease.Close())
		}
	}()
	receipt, lockedInfo, err := readLockedResidueReceipt(context.Background(), receiptLease, staging.receiptName)
	if err != nil || receipt != receiptForStaging(staging) || !os.SameFile(info, lockedInfo) {
		return errors.Join(ErrResidueConflict, err)
	}

	var bindingLease *os.File
	bindingLeaseOpen := false
	if staging.bindingName != "" {
		if staging.bindingIdentity == nil {
			return errors.New("backup: residue identity binding identity is unavailable")
		}
		bindingLease, err = openAndLockResidueFile(parent, staging.bindingName, staging.bindingIdentity)
		if err != nil {
			return err
		}
		bindingLeaseOpen = true
		defer func() {
			if bindingLeaseOpen {
				resultErr = errors.Join(resultErr, unlockResidueFile(bindingLease), bindingLease.Close())
			}
		}()
		binding, bindingInfo, readErr := readLockedResidueBinding(
			context.Background(), bindingLease, receipt,
		)
		if readErr != nil || binding != bindingForStaging(staging) ||
			!os.SameFile(bindingInfo, staging.bindingIdentity) {
			return errors.Join(ErrResidueConflict, readErr)
		}
	}
	receiptLeaseOpen = false
	if err := deleteResidueMutationFile(parent, staging.receiptName, info, receiptLease); err != nil {
		return err
	}
	staging.receiptName = ""
	staging.receiptIdentity = nil
	if err := syncRetainedDirectory(parent); err != nil {
		return fmt.Errorf("backup: sync residue receipt removal: %w", err)
	}
	if bindingLease != nil {
		bindingLeaseOpen = false
		if err := deleteResidueMutationFile(
			parent, staging.bindingName, staging.bindingIdentity, bindingLease,
		); err != nil {
			return err
		}
		staging.bindingName = ""
		staging.bindingIdentity = nil
		if err := syncRetainedDirectory(parent); err != nil {
			return fmt.Errorf("backup: sync residue identity binding removal: %w", err)
		}
	}
	return nil
}

func readResidueReceipt(ctx context.Context, parent *retainedDirectory, name string) (residueReceipt, os.FileInfo, error) {
	if ctx == nil || parent == nil || parent.root == nil {
		return residueReceipt{}, nil, errors.New("backup: invalid residue receipt read")
	}
	_, ok := parseResidueReceiptName(name)
	if !ok {
		return residueReceipt{}, nil, errors.New("backup: invalid residue receipt filename")
	}
	file, info, err := openRootRegularFile(parent, name)
	if err != nil {
		return residueReceipt{}, nil, err
	}
	if info.Size() > maxResidueReceiptBytes {
		_ = file.Close()
		return residueReceipt{}, nil, errors.New("backup: residue receipt exceeds size limit")
	}
	var encoded bytes.Buffer
	_, readErr := copyContext(ctx, &encoded, io.LimitReader(file, maxResidueReceiptBytes+1))
	stableErr := verifyOpenedRootFile(parent, name, file, info)
	closeErr := file.Close()
	if err := errors.Join(readErr, stableErr, closeErr); err != nil {
		return residueReceipt{}, nil, err
	}
	if encoded.Len() > maxResidueReceiptBytes {
		return residueReceipt{}, nil, errors.New("backup: residue receipt exceeds size limit")
	}
	receipt, err := decodeResidueReceipt(ctx, name, encoded.Bytes())
	return receipt, info, err
}

func readLockedResidueReceipt(
	ctx context.Context,
	file *os.File,
	name string,
) (residueReceipt, os.FileInfo, error) {
	if ctx == nil || file == nil {
		return residueReceipt{}, nil, errors.New("backup: invalid locked residue receipt read")
	}
	info, err := file.Stat()
	if err != nil {
		return residueReceipt{}, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxResidueReceiptBytes {
		return residueReceipt{}, nil, errors.New("backup: invalid locked residue receipt")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return residueReceipt{}, nil, err
	}
	var encoded bytes.Buffer
	if _, err := copyContext(ctx, &encoded, io.LimitReader(file, maxResidueReceiptBytes+1)); err != nil {
		return residueReceipt{}, nil, err
	}
	if encoded.Len() > maxResidueReceiptBytes {
		return residueReceipt{}, nil, errors.New("backup: residue receipt exceeds size limit")
	}
	receipt, err := decodeResidueReceipt(ctx, name, encoded.Bytes())
	return receipt, info, err
}

func readLockedResidueBinding(
	ctx context.Context,
	file *os.File,
	receipt residueReceipt,
) (residueBinding, os.FileInfo, error) {
	if ctx == nil || file == nil {
		return residueBinding{}, nil, errors.New("backup: invalid locked residue identity-binding read")
	}
	info, err := file.Stat()
	if err != nil {
		return residueBinding{}, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxResidueBindingBytes {
		return residueBinding{}, nil, errors.New("backup: invalid locked residue identity binding")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return residueBinding{}, nil, err
	}
	var encoded bytes.Buffer
	if _, err := copyContext(ctx, &encoded, io.LimitReader(file, maxResidueBindingBytes+1)); err != nil {
		return residueBinding{}, nil, err
	}
	if encoded.Len() > maxResidueBindingBytes {
		return residueBinding{}, nil, errors.New("backup: residue identity binding exceeds size limit")
	}
	binding, err := decodeResidueBinding(ctx, encoded.Bytes(), receipt)
	return binding, info, err
}

func decodeResidueReceipt(ctx context.Context, name string, encoded []byte) (residueReceipt, error) {
	id, ok := parseResidueReceiptName(name)
	if !ok {
		return residueReceipt{}, errors.New("backup: invalid residue receipt filename")
	}
	if err := rejectDuplicateJSONFields(ctx, encoded); err != nil {
		return residueReceipt{}, fmt.Errorf("backup: invalid residue receipt JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var receipt residueReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return residueReceipt{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return residueReceipt{}, errors.New("backup: residue receipt has trailing JSON")
	}
	if err := validateResidueReceipt(receipt); err != nil {
		return residueReceipt{}, err
	}
	if receipt.ID != id {
		return residueReceipt{}, errors.New("backup: receipt ID differs from its filename")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil {
		return residueReceipt{}, err
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(encoded, canonical) {
		return residueReceipt{}, errors.New("backup: residue receipt is not canonical")
	}
	return receipt, nil
}

func readResidueBinding(
	ctx context.Context,
	parent *retainedDirectory,
	receipt residueReceipt,
) (binding residueBinding, info os.FileInfo, exists bool, resultErr error) {
	if ctx == nil || parent == nil || parent.root == nil {
		return residueBinding{}, nil, false, errors.New("backup: invalid residue identity binding read")
	}
	name := residueBindingName(receipt.ID)
	entry, err := parent.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return residueBinding{}, nil, false, nil
	}
	if err != nil {
		return residueBinding{}, nil, false, err
	}
	if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() || entry.Size() > maxResidueBindingBytes {
		return residueBinding{}, entry, true, errors.New("backup: invalid residue identity binding file")
	}
	file, opened, err := openRootRegularFile(parent, name)
	if err != nil {
		return residueBinding{}, entry, true, err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	if !os.SameFile(entry, opened) || opened.Size() > maxResidueBindingBytes {
		return residueBinding{}, opened, true, errors.New("backup: residue identity binding changed while opening")
	}
	var encoded bytes.Buffer
	if _, err := copyContext(ctx, &encoded, io.LimitReader(file, maxResidueBindingBytes+1)); err != nil {
		return residueBinding{}, opened, true, err
	}
	if encoded.Len() > maxResidueBindingBytes {
		return residueBinding{}, opened, true, errors.New("backup: residue identity binding exceeds size limit")
	}
	if err := verifyOpenedRootFile(parent, name, file, opened); err != nil {
		return residueBinding{}, opened, true, err
	}
	binding, err = decodeResidueBinding(ctx, encoded.Bytes(), receipt)
	return binding, opened, true, err
}

func decodeResidueBinding(ctx context.Context, encoded []byte, receipt residueReceipt) (residueBinding, error) {
	if err := rejectDuplicateJSONFields(ctx, encoded); err != nil {
		return residueBinding{}, fmt.Errorf("backup: invalid residue identity binding JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var binding residueBinding
	if err := decoder.Decode(&binding); err != nil {
		return residueBinding{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return residueBinding{}, errors.New("backup: residue identity binding has trailing JSON")
	}
	if err := validateResidueBinding(binding, receipt); err != nil {
		return residueBinding{}, err
	}
	canonical, err := json.Marshal(binding)
	if err != nil {
		return residueBinding{}, err
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(encoded, canonical) {
		return residueBinding{}, errors.New("backup: residue identity binding is not canonical")
	}
	return binding, nil
}

func readStandaloneResidueBinding(
	ctx context.Context,
	parent *retainedDirectory,
	name string,
) (residueBinding, residueReceipt, os.FileInfo, error) {
	if ctx == nil || parent == nil || parent.root == nil {
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: invalid standalone residue binding read")
	}
	id, ok := parseResidueBindingName(name)
	if !ok {
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: invalid standalone residue binding filename")
	}
	file, info, err := openRootRegularFile(parent, name)
	if err != nil {
		return residueBinding{}, residueReceipt{}, nil, err
	}
	if info.Size() > maxResidueBindingBytes {
		_ = file.Close()
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: standalone residue binding exceeds size limit")
	}
	var encoded bytes.Buffer
	_, readErr := copyContext(ctx, &encoded, io.LimitReader(file, maxResidueBindingBytes+1))
	stableErr := verifyOpenedRootFile(parent, name, file, info)
	closeErr := file.Close()
	if err := errors.Join(readErr, stableErr, closeErr); err != nil {
		return residueBinding{}, residueReceipt{}, nil, err
	}
	if encoded.Len() > maxResidueBindingBytes {
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: standalone residue binding exceeds size limit")
	}
	if err := rejectDuplicateJSONFields(ctx, encoded.Bytes()); err != nil {
		return residueBinding{}, residueReceipt{}, nil, fmt.Errorf("backup: invalid standalone residue binding JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded.Bytes()))
	decoder.DisallowUnknownFields()
	var binding residueBinding
	if err := decoder.Decode(&binding); err != nil {
		return residueBinding{}, residueReceipt{}, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: standalone residue binding has trailing JSON")
	}
	receipt := residueReceipt{
		Version: residueReceiptVersion, ID: binding.ID, Revision: binding.ReceiptRevision,
		ParentIdentity: binding.ParentIdentity, Kind: binding.Kind,
		StagingName: binding.StagingName, DestinationName: binding.DestinationName,
	}
	if binding.ID != id {
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: standalone binding ID differs from its filename")
	}
	if err := validateResidueReceipt(receipt); err != nil {
		return residueBinding{}, residueReceipt{}, nil, err
	}
	if err := validateResidueBinding(binding, receipt); err != nil {
		return residueBinding{}, residueReceipt{}, nil, err
	}
	canonical, err := json.Marshal(binding)
	if err != nil {
		return residueBinding{}, residueReceipt{}, nil, err
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(encoded.Bytes(), canonical) {
		return residueBinding{}, residueReceipt{}, nil, errors.New("backup: standalone residue binding is not canonical")
	}
	return binding, receipt, info, nil
}

// ListResidues performs a bounded direct-child scan of one retained target
// parent. Staging-looking directories without strict receipts are ignored.
func ListResidues(ctx context.Context, parentPath string, limit int) (page ResiduePage, resultErr error) {
	if ctx == nil {
		return ResiduePage{}, errors.New("backup: nil residue listing context")
	}
	if limit < 1 || limit > maxResiduePageSize {
		return ResiduePage{}, fmt.Errorf("backup: residue page size must be between 1 and %d", maxResiduePageSize)
	}
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		return ResiduePage{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	return listResiduesRoot(ctx, parent, limit)
}

func listResiduesRoot(
	ctx context.Context,
	parent *retainedDirectory,
	limit int,
) (page ResiduePage, resultErr error) {
	if ctx == nil || parent == nil || parent.root == nil {
		return ResiduePage{}, errors.New("backup: invalid retained residue listing")
	}
	if limit < 1 || limit > maxResiduePageSize {
		return ResiduePage{}, fmt.Errorf("backup: residue page size must be between 1 and %d", maxResiduePageSize)
	}
	parentIdentity, err := persistentDirectoryIdentityToken(parent)
	if err != nil {
		return ResiduePage{}, err
	}
	rootInfo, err := parent.root.Stat(".")
	if err != nil {
		return ResiduePage{}, err
	}
	cursor, err := openExactTreeDirectory(parent, rootInfo, ".")
	if err != nil {
		return ResiduePage{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, closeExactTreeDirectory(parent, rootInfo, cursor)) }()
	receiptNames := make(map[string]string, limit)
	bindingNames := make(map[string]string, limit)
	scanned := 0
	finished := false
	for {
		if err := ctx.Err(); err != nil {
			return ResiduePage{}, err
		}
		entries, readErr := cursor.file.ReadDir(directoryReadBatch)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return ResiduePage{}, readErr
		}
		for _, entry := range entries {
			scanned++
			if scanned > maxResidueScanEntries {
				page.Truncated = true
				finished = true
				break
			}
			if id, ok := parseResidueReceiptName(entry.Name()); ok {
				if _, duplicate := receiptNames[id]; duplicate {
					return ResiduePage{}, fmt.Errorf("backup: duplicate residue receipt ID %s", id)
				}
				receiptNames[id] = entry.Name()
				continue
			}
			if id, ok := parseResidueBindingName(entry.Name()); ok {
				if _, duplicate := bindingNames[id]; duplicate {
					return ResiduePage{}, fmt.Errorf("backup: duplicate residue binding ID %s", id)
				}
				bindingNames[id] = entry.Name()
			}
		}
		if finished || errors.Is(readErr, io.EOF) {
			break
		}
		if len(entries) == 0 {
			return ResiduePage{}, errors.New("backup: zero-progress residue directory read")
		}
	}
	ids := make([]string, 0, len(receiptNames)+len(bindingNames))
	seen := make(map[string]struct{}, len(receiptNames)+len(bindingNames))
	for id := range receiptNames {
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for id := range bindingNames {
		if _, exists := seen[id]; !exists {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	page.Items = make([]Residue, 0, min(limit, len(ids)))
	for _, id := range ids {
		if len(page.Items) == limit {
			page.Truncated = true
			break
		}
		if name := receiptNames[id]; name != "" {
			receipt, _, err := readResidueReceipt(ctx, parent, name)
			if err != nil {
				return ResiduePage{}, fmt.Errorf("backup: inspect residue receipt %s: %w", name, err)
			}
			if receipt.ParentIdentity != parentIdentity {
				return ResiduePage{}, fmt.Errorf("backup: residue receipt %s belongs to a different parent", name)
			}
			observation, err := classifyResidue(ctx, parent, receipt)
			if err != nil {
				return ResiduePage{}, err
			}
			page.Items = append(page.Items, residueFromReceipt(receipt, observation))
			continue
		}
		binding, receipt, info, err := readStandaloneResidueBinding(ctx, parent, bindingNames[id])
		if err != nil {
			return ResiduePage{}, fmt.Errorf("backup: inspect standalone residue binding %s: %w", bindingNames[id], err)
		}
		if binding.ParentIdentity != parentIdentity {
			return ResiduePage{}, fmt.Errorf("backup: standalone residue binding %s belongs to a different parent", bindingNames[id])
		}
		observation, err := classifyStandaloneResidueBinding(parent, binding, info)
		if err != nil {
			return ResiduePage{}, err
		}
		page.Items = append(page.Items, residueFromReceipt(receipt, observation))
	}
	return page, nil
}

func residueFromReceipt(receipt residueReceipt, observation residueObservation) Residue {
	return Residue{
		ID: receipt.ID, Revision: receipt.Revision, Identity: observation.identity, Kind: receipt.Kind,
		State: observation.state, StagingName: receipt.StagingName, DestinationName: receipt.DestinationName,
	}
}

// RecoverResidue requires a value returned by ListResidues. It acquires the
// receipt lease non-blockingly, rechecks revision, parent and tree identity,
// then removes only the matching staging tree or receipt. The Coordinator's
// retained active-Vault identity rejects recovery of any tree that is equal
// to, contains, or is contained by the active Vault. A publication-uncertain
// destination and its evidence are never deleted or acknowledged here.
func (c *Coordinator) RecoverResidue(ctx context.Context, parentPath string, expected Residue) (resultErr error) {
	if c == nil {
		return errors.New("backup: coordinator is not initialized")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return errors.New("backup: coordinator is not initialized")
	}
	return recoverResidue(ctx, parentPath, expected, c.rejectActiveVaultOverlap, nil)
}

// ConfirmPublishedBackup resolves an uncertain publication receipt or its
// cleanup-pending binding-only redo state. It first performs the complete
// semantic Verify against an identity-bound scratch parent, then CAS-reopens
// the exact remaining evidence and destination, rechecks the complete immutable
// tree, and removes only that evidence. It never deletes or rewrites the
// published destination.
func (c *Coordinator) ConfirmPublishedBackup(
	ctx context.Context,
	parentPath string,
	expected Residue,
	options VerifyOptions,
) (Outcome, error) {
	return c.confirmPublishedBackup(ctx, parentPath, expected, options, nil)
}

func (c *Coordinator) confirmPublishedBackup(
	ctx context.Context,
	parentPath string,
	expected Residue,
	options VerifyOptions,
	afterFullVerify func() error,
) (Outcome, error) {
	if c == nil || ctx == nil || expected.Kind != "backup" ||
		(expected.State != ResidueStatePublicationUncertain && expected.State != ResidueStatePublicationCleanup) {
		return failedOutcome(FailureInvalid, errors.New("backup: invalid published-backup confirmation input"))
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return failedOutcome(FailureInvalid, errors.New("backup: coordinator is not initialized"))
	}
	var summary Summary
	err := recoverResidue(
		ctx,
		parentPath,
		expected,
		c.rejectActiveVaultOverlap,
		func(ctx context.Context, published *retainedDirectory) error {
			cleanup, err := c.cleanupLiveVerifyScratch(ctx, published, options.ScratchParent)
			if err == nil && cleanup.Truncated {
				err = errors.Join(ErrCleanupResidual, errors.New("backup: live verification scratch cleanup is truncated"))
			}
			if err != nil {
				return err
			}
			manifest, wire, err := readManifestWireRoot(ctx, published, manifestFileName)
			if err != nil {
				return err
			}
			commitment := sha256.Sum256(wire)
			verified, err := verifyBackup(
				ctx,
				published.path,
				options,
				verifyHooks{},
				func(scratch *retainedDirectory) error {
					return errors.Join(
						validateOwnerOnlyVerifyScratch(scratch),
						rejectRetainedDirectoryOverlap(c.activeVault, scratch, ErrActiveVaultOverlap),
					)
				},
			)
			if err != nil {
				return err
			}
			if afterFullVerify != nil {
				if err := afterFullVerify(); err != nil {
					return err
				}
			}
			if err := published.verifyPath(); err != nil {
				return err
			}
			_, currentWire, err := readManifestWireRoot(ctx, published, manifestFileName)
			if err != nil || sha256.Sum256(currentWire) != commitment {
				return errors.Join(errors.New("backup: published manifest changed during confirmation"), err)
			}
			if err := verifyBackupPublicationRoot(ctx, published, manifest); err != nil {
				return err
			}
			summary = verified
			return nil
		},
	)
	if err == nil {
		return Outcome{Succeeded: true, Summary: summary}, nil
	}
	class := classifyBackupFailure(err)
	outcome := Outcome{Failure: class, CleanupRequired: errors.Is(err, ErrCleanupResidual), Summary: summary}
	return outcome, &classifiedFailure{class: class, cause: err}
}

type residueRecoveryGuard func(*destinationTarget) error
type publishedBackupConfirmation func(context.Context, *retainedDirectory) error

func recoverResidue(
	ctx context.Context,
	parentPath string,
	expected Residue,
	guard residueRecoveryGuard,
	confirmPublishedBackup publishedBackupConfirmation,
) (resultErr error) {
	if ctx == nil {
		return errors.New("backup: nil residue recovery context")
	}
	if err := ensureResidueRecoverySupported(); err != nil {
		return err
	}
	if !validHexToken(expected.ID, 16) || !validHexToken(expected.Revision, sha256.Size) ||
		expected.Identity == "" || !validResidueLeaf(expected.StagingName) ||
		!validDestinationLeaf(expected.DestinationName) {
		return errors.New("backup: invalid expected residue")
	}
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	return recoverResidueRoot(ctx, parent, expected, guard, confirmPublishedBackup)
}

func recoverResidueRoot(
	ctx context.Context,
	parent *retainedDirectory,
	expected Residue,
	guard residueRecoveryGuard,
	confirmPublishedBackup publishedBackupConfirmation,
) (resultErr error) {
	if ctx == nil || parent == nil || parent.root == nil {
		return errors.New("backup: invalid retained residue recovery")
	}
	if err := ensureResidueRecoverySupported(); err != nil {
		return err
	}
	if !validHexToken(expected.ID, 16) || !validHexToken(expected.Revision, sha256.Size) ||
		expected.Identity == "" || !validResidueLeaf(expected.StagingName) ||
		!validDestinationLeaf(expected.DestinationName) {
		return errors.New("backup: invalid expected residue")
	}
	name := residueReceiptName(expected.ID)
	receipt, receiptInfo, err := readResidueReceipt(ctx, parent, name)
	if errors.Is(err, os.ErrNotExist) {
		return recoverStandaloneResidueBindingRoot(ctx, parent, expected, guard, confirmPublishedBackup)
	}
	if err != nil {
		return err
	}
	lease, err := openAndLockResidueReceipt(parent, name, receiptInfo)
	if err != nil {
		return err
	}
	staging := &stagingDirectory{receiptLease: lease}
	defer func() { resultErr = errors.Join(resultErr, closeResidueLease(staging)) }()
	receipt, lockedInfo, err := readLockedResidueReceipt(ctx, lease, name)
	if err != nil || !os.SameFile(receiptInfo, lockedInfo) {
		return errors.Join(ErrResidueConflict, err)
	}
	parentIdentity, err := persistentDirectoryIdentityToken(parent)
	if err != nil {
		return err
	}
	if receipt.ParentIdentity != parentIdentity {
		return ErrResidueConflict
	}
	observation, err := classifyResidue(ctx, parent, receipt)
	if err != nil {
		return err
	}
	actual := residueFromReceipt(receipt, observation)
	if actual != expected {
		return ErrResidueConflict
	}
	staging.name = receipt.StagingName
	staging.operationID = receipt.ID
	staging.revision = receipt.Revision
	staging.kind = receipt.Kind
	staging.destinationName = receipt.DestinationName
	staging.parentIdentity = receipt.ParentIdentity
	staging.identityToken = observation.identity
	staging.receiptName = name
	staging.receiptIdentity = lockedInfo
	if observation.bindingInfo != nil {
		staging.bindingName = residueBindingName(receipt.ID)
		staging.bindingIdentity = observation.bindingInfo
	}
	guardName := receipt.StagingName
	if observation.state == ResidueStatePublicationUncertain {
		guardName = receipt.DestinationName
	}
	if guard != nil {
		if err := guard(&destinationTarget{
			finalPath: filepath.Join(parent.path, guardName),
			finalName: guardName,
			parent:    parent,
		}); err != nil {
			return fmt.Errorf("backup: residue overlaps protected root: %w", err)
		}
	}
	switch observation.state {
	case ResidueStateStaging:
		directory, err := openExpectedDirectory(parent, receipt.StagingName, directoryIdentity{info: observation.stagingInfo})
		if err != nil {
			return errors.Join(ErrCleanupResidual, ErrCleanupIdentityLost, err)
		}
		staging.directory = directory
		return cleanupStagingTree(staging, parent, receipt.Kind)
	case ResidueStatePublicationUncertain:
		if confirmPublishedBackup == nil || receipt.Kind != "backup" {
			return ErrPublicationUncertain
		}
		published, err := openExpectedDirectory(
			parent, receipt.DestinationName, directoryIdentity{info: observation.destinationInfo},
		)
		if err != nil {
			return errors.Join(ErrPublicationUncertain, err)
		}
		verifyErr := confirmPublishedBackup(ctx, published)
		closeErr := published.Close()
		if err := errors.Join(verifyErr, closeErr); err != nil {
			return errors.Join(ErrPublicationUncertain, err)
		}
		return removeResidueReceipt(parent, staging)
	case ResidueStateReceiptOnly:
		return removeResidueReceipt(parent, staging)
	case ResidueStateConflict:
		return ErrResidueConflict
	default:
		return errors.New("backup: unknown residue state")
	}
}

func recoverStandaloneResidueBindingRoot(
	ctx context.Context,
	parent *retainedDirectory,
	expected Residue,
	guard residueRecoveryGuard,
	confirmPublishedBackup publishedBackupConfirmation,
) (resultErr error) {
	// The binding is the durable redo marker for the receipt-removal gap. Hold
	// the parent's sync capability before any verification or mutation so a
	// successful binding deletion can be made durable before returning.
	if err := parent.acquireSyncHandle(); err != nil {
		return fmt.Errorf("backup: acquire cleanup-pending parent sync capability: %w", err)
	}
	name := residueBindingName(expected.ID)
	binding, receipt, info, err := readStandaloneResidueBinding(ctx, parent, name)
	if err != nil {
		return err
	}
	parentIdentity, err := persistentDirectoryIdentityToken(parent)
	if err != nil || binding.ParentIdentity != parentIdentity {
		return errors.Join(ErrResidueConflict, err)
	}
	observation, err := classifyStandaloneResidueBinding(parent, binding, info)
	if err != nil {
		return err
	}
	if residueFromReceipt(receipt, observation) != expected {
		return ErrResidueConflict
	}
	lease, err := openAndLockResidueFile(parent, name, info)
	if err != nil {
		return err
	}
	leaseOpen := true
	defer func() {
		if leaseOpen {
			resultErr = errors.Join(resultErr, unlockResidueFile(lease), lease.Close())
		}
	}()
	lockedBinding, lockedInfo, err := readLockedResidueBinding(ctx, lease, receipt)
	if err != nil || lockedBinding != binding || !os.SameFile(lockedInfo, info) {
		return errors.Join(ErrResidueConflict, err)
	}
	if _, err := parent.root.Lstat(residueReceiptName(expected.ID)); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrResidueConflict, err)
	}
	guardName := binding.StagingName
	if observation.state == ResidueStatePublicationCleanup {
		guardName = binding.DestinationName
	}
	if guard != nil {
		if err := guard(&destinationTarget{
			finalPath: filepath.Join(parent.path, guardName), finalName: guardName, parent: parent,
		}); err != nil {
			return fmt.Errorf("backup: cleanup-pending residue overlaps protected root: %w", err)
		}
	}
	switch observation.state {
	case ResidueStateReceiptOnly:
		// The owned tree is already absent. The binding is the sole redo marker.
	case ResidueStatePublicationCleanup:
		if confirmPublishedBackup == nil || binding.Kind != "backup" {
			return ErrPublicationUncertain
		}
		published, err := openExpectedDirectory(
			parent, binding.DestinationName, directoryIdentity{info: observation.destinationInfo},
		)
		if err != nil {
			return errors.Join(ErrPublicationUncertain, err)
		}
		verifyErr := confirmPublishedBackup(ctx, published)
		closeErr := published.Close()
		if err := errors.Join(verifyErr, closeErr); err != nil {
			return errors.Join(ErrPublicationUncertain, err)
		}
	default:
		return ErrResidueConflict
	}
	// Reclassify both admissible states immediately before deleting the sole
	// redo marker. This prevents a tree created after the first observation
	// from being orphaned by a receipt-only cleanup, and also revalidates the
	// published destination identity after the full verification pass.
	current, classifyErr := classifyStandaloneResidueBinding(parent, binding, info)
	if classifyErr != nil || current.state != observation.state ||
		current.identity != observation.identity {
		return errors.Join(ErrResidueConflict, classifyErr)
	}
	lockedBinding, lockedInfo, err = readLockedResidueBinding(ctx, lease, receipt)
	if err != nil || lockedBinding != binding || !os.SameFile(lockedInfo, info) {
		return errors.Join(ErrResidueConflict, err)
	}
	if _, err := parent.root.Lstat(residueReceiptName(expected.ID)); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrResidueConflict, err)
	}
	leaseOpen = false
	if err := deleteResidueMutationFile(parent, name, info, lease); err != nil {
		return errors.Join(ErrCleanupResidual, err)
	}
	if err := syncRetainedDirectory(parent); err != nil {
		return errors.Join(ErrCleanupResidual, err)
	}
	return nil
}

func openAndLockResidueReceipt(parent *retainedDirectory, name string, expected os.FileInfo) (*os.File, error) {
	return openAndLockResidueFile(parent, name, expected)
}

func openAndLockResidueFile(parent *retainedDirectory, name string, expected os.FileInfo) (*os.File, error) {
	file, err := openResidueMutationFile(parent, name, expected)
	if err != nil {
		return nil, err
	}
	fail := func(cause error) (*os.File, error) {
		_ = file.Close()
		return nil, cause
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(opened, expected) {
		if err != nil {
			return fail(err)
		}
		return fail(ErrResidueConflict)
	}
	if err := lockResidueFile(file, true); err != nil {
		return fail(err)
	}
	current, err := parent.root.Lstat(name)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() ||
		!os.SameFile(opened, current) {
		_ = unlockResidueFile(file)
		if err != nil {
			return fail(err)
		}
		return fail(ErrResidueConflict)
	}
	return file, nil
}

func classifyResidue(ctx context.Context, parent *retainedDirectory, receipt residueReceipt) (residueObservation, error) {
	binding, bindingInfo, bindingExists, bindingErr := readResidueBinding(ctx, parent, receipt)
	if err := ctx.Err(); err != nil {
		return residueObservation{}, err
	}
	stagingExists, stagingToken, stagingInfo, err := observeResidueDirectory(parent, receipt.StagingName)
	if err != nil {
		return residueObservation{}, err
	}
	destinationExists, destinationToken, destinationInfo, err := observeResidueDirectory(parent, receipt.DestinationName)
	if err != nil {
		return residueObservation{}, err
	}
	observation := residueObservation{
		state: ResidueStateReceiptOnly, identity: "none", stagingInfo: stagingInfo,
		destinationInfo: destinationInfo, bindingInfo: bindingInfo,
	}
	existing := 0
	for _, exists := range []bool{stagingExists, destinationExists} {
		if exists {
			existing++
		}
	}
	if existing == 0 {
		if bindingErr != nil || bindingExists {
			observation.state = ResidueStateConflict
			observation.identity = "conflict"
		}
		return observation, nil
	}
	if bindingErr != nil || !bindingExists || existing > 1 ||
		(stagingExists && stagingToken == "") ||
		(destinationExists && destinationToken == "") {
		observation.state = ResidueStateConflict
		observation.identity = "conflict"
		return observation, nil
	}
	currentToken := destinationToken
	if stagingExists {
		currentToken = stagingToken
	}
	if currentToken != binding.TreeIdentity {
		observation.state = ResidueStateConflict
		observation.identity = "conflict"
		return observation, nil
	}
	switch {
	case stagingExists:
		observation.state = ResidueStateStaging
		observation.identity = stagingToken
	case destinationExists:
		observation.state = ResidueStatePublicationUncertain
		observation.identity = destinationToken
	}
	return observation, nil
}

func classifyStandaloneResidueBinding(
	parent *retainedDirectory,
	binding residueBinding,
	bindingInfo os.FileInfo,
) (residueObservation, error) {
	stagingExists, _, stagingInfo, err := observeResidueDirectory(parent, binding.StagingName)
	if err != nil {
		return residueObservation{}, err
	}
	destinationExists, destinationToken, destinationInfo, err := observeResidueDirectory(parent, binding.DestinationName)
	if err != nil {
		return residueObservation{}, err
	}
	observation := residueObservation{
		state: ResidueStateReceiptOnly, identity: "none", stagingInfo: stagingInfo,
		destinationInfo: destinationInfo, bindingInfo: bindingInfo,
	}
	if stagingExists || (destinationExists && destinationToken == "") ||
		(stagingExists && destinationExists) {
		observation.state = ResidueStateConflict
		observation.identity = "conflict"
		return observation, nil
	}
	if destinationExists {
		if destinationToken != binding.TreeIdentity {
			observation.state = ResidueStateConflict
			observation.identity = "conflict"
			return observation, nil
		}
		observation.state = ResidueStatePublicationCleanup
		observation.identity = destinationToken
	}
	return observation, nil
}

func observeResidueDirectory(
	parent *retainedDirectory,
	name string,
) (exists bool, token string, info os.FileInfo, resultErr error) {
	info, err := parent.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, "", nil, nil
	}
	if err != nil {
		return false, "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return true, "", info, nil
	}
	directory, err := openExpectedDirectory(parent, name, directoryIdentity{info: info})
	if err != nil {
		return true, "", info, err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	token, err = persistentDirectoryIdentityToken(directory)
	if err != nil {
		return true, "", info, err
	}
	return true, token, info, nil
}
