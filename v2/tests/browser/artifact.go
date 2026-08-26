package browserqualification

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxApprovedArtifactBytes int64 = 1 << 30

type retainedArtifact struct {
	approval binaryApproval
	file     *os.File
	identity os.FileInfo
}

type approvedArtifactSet struct {
	approval   artifactApproval
	root       *os.File
	rootID     os.FileInfo
	browser    retainedArtifact
	driver     retainedArtifact
	mindweaver retainedArtifact
}

func validateArtifactApproval(approval artifactApproval) error {
	if !stableToken(approval.ID, 64) || approval.OS != "windows" || approval.Arch != "amd64" {
		return errors.New("invalid artifact approval")
	}
	binaries := []binaryApproval{approval.Browser, approval.Driver, approval.MindWeaver}
	seen := make(map[string]struct{}, len(binaries))
	for _, binary := range binaries {
		if !safeArtifactLeaf(binary.FileName) || !lowerSHA256(binary.SHA256) || binary.Size <= 0 ||
			binary.Size > maxApprovedArtifactBytes || !stableVersion(binary.Version) {
			return errors.New("invalid artifact approval")
		}
		folded := strings.ToLower(binary.FileName)
		if _, duplicate := seen[folded]; duplicate {
			return errors.New("invalid artifact approval")
		}
		seen[folded] = struct{}{}
	}
	return nil
}

func safeArtifactLeaf(name string) bool {
	if name == "" || len(name) > 128 || filepath.Base(name) != name || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	for _, character := range name {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" ||
		strings.HasPrefix(stem, "COM") && len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' ||
		strings.HasPrefix(stem, "LPT") && len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' {
		return false
	}
	return true
}

func openApprovedArtifacts(approval artifactApproval, rootPath string) (_ *approvedArtifactSet, result error) {
	if validateArtifactApproval(approval) != nil || runtime.GOOS != approval.OS || runtime.GOARCH != approval.Arch ||
		rootPath == "" || !filepath.IsAbs(rootPath) {
		return nil, errors.New("invalid artifact bundle")
	}
	cleanRoot := filepath.Clean(rootPath)
	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil || !sameArtifactPath(cleanRoot, resolvedRoot) {
		return nil, errors.New("invalid artifact bundle")
	}
	rootInfo, err := os.Lstat(cleanRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid artifact bundle")
	}
	root, err := os.Open(cleanRoot)
	if err != nil {
		return nil, errors.New("invalid artifact bundle")
	}
	openedRootInfo, err := root.Stat()
	if err != nil || !openedRootInfo.IsDir() || !os.SameFile(rootInfo, openedRootInfo) {
		_ = root.Close()
		return nil, errors.New("invalid artifact bundle")
	}
	set := &approvedArtifactSet{approval: approval, root: root, rootID: openedRootInfo}
	defer func() {
		if result != nil {
			result = errors.Join(result, set.Close())
		}
	}()
	entries, err := root.ReadDir(4)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid artifact bundle")
	}
	if len(entries) != 3 {
		return nil, errors.New("invalid artifact bundle")
	}
	expected := map[string]binaryApproval{
		strings.ToLower(approval.Browser.FileName):    approval.Browser,
		strings.ToLower(approval.Driver.FileName):     approval.Driver,
		strings.ToLower(approval.MindWeaver.FileName): approval.MindWeaver,
	}
	for _, entry := range entries {
		binary, ok := expected[strings.ToLower(entry.Name())]
		if !ok || entry.Name() != binary.FileName || entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("invalid artifact bundle")
		}
		retained, err := openRetainedArtifact(cleanRoot, binary)
		if err != nil {
			return nil, err
		}
		switch binary.FileName {
		case approval.Browser.FileName:
			set.browser = retained
		case approval.Driver.FileName:
			set.driver = retained
		case approval.MindWeaver.FileName:
			set.mindweaver = retained
		}
	}
	if set.browser.file == nil || set.driver.file == nil || set.mindweaver.file == nil || set.Reverify() != nil {
		return nil, errors.New("invalid artifact bundle")
	}
	return set, nil
}

func openRetainedArtifact(root string, approval binaryApproval) (retainedArtifact, error) {
	path := filepath.Join(root, approval.FileName)
	linkInfo, err := os.Lstat(path)
	if err != nil || linkInfo.Mode()&os.ModeSymlink != 0 || !linkInfo.Mode().IsRegular() || linkInfo.Size() != approval.Size {
		return retainedArtifact{}, errors.New("invalid artifact bundle")
	}
	file, err := os.Open(path)
	if err != nil {
		return retainedArtifact{}, errors.New("invalid artifact bundle")
	}
	identity, err := file.Stat()
	if err != nil || !identity.Mode().IsRegular() || identity.Size() != approval.Size || !os.SameFile(linkInfo, identity) {
		_ = file.Close()
		return retainedArtifact{}, errors.New("invalid artifact bundle")
	}
	if !singleLink(file) {
		_ = file.Close()
		return retainedArtifact{}, errors.New("invalid artifact bundle")
	}
	retained := retainedArtifact{approval: approval, file: file, identity: identity}
	if err := retained.verify(); err != nil {
		_ = file.Close()
		return retainedArtifact{}, err
	}
	return retained, nil
}

func (artifact *retainedArtifact) verify() error {
	if artifact.file == nil || artifact.identity == nil {
		return errors.New("invalid artifact bundle")
	}
	current, err := artifact.file.Stat()
	if err != nil || !os.SameFile(artifact.identity, current) || current.Size() != artifact.approval.Size || !singleLink(artifact.file) {
		return errors.New("invalid artifact bundle")
	}
	pathInfo, err := os.Lstat(artifact.file.Name())
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !os.SameFile(current, pathInfo) {
		return errors.New("invalid artifact bundle")
	}
	if _, err := artifact.file.Seek(0, io.SeekStart); err != nil {
		return errors.New("invalid artifact bundle")
	}
	hash := sha256.New()
	written, err := io.CopyN(hash, artifact.file, artifact.approval.Size)
	if err != nil || written != artifact.approval.Size {
		return errors.New("invalid artifact bundle")
	}
	one := make([]byte, 1)
	if count, err := artifact.file.Read(one); err != nil && !errors.Is(err, io.EOF) || count != 0 {
		return errors.New("invalid artifact bundle")
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != artifact.approval.SHA256 {
		return errors.New("invalid artifact bundle")
	}
	return nil
}

func (set *approvedArtifactSet) Reverify() error {
	if set == nil || set.root == nil || set.rootID == nil {
		return errors.New("invalid artifact bundle")
	}
	currentRoot, err := set.root.Stat()
	pathRoot, pathErr := os.Lstat(set.root.Name())
	if err != nil || pathErr != nil || !currentRoot.IsDir() || !pathRoot.IsDir() || pathRoot.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(set.rootID, currentRoot) || !os.SameFile(currentRoot, pathRoot) {
		return errors.New("invalid artifact bundle")
	}
	return errors.Join(set.browser.verify(), set.driver.verify(), set.mindweaver.verify())
}

func (set *approvedArtifactSet) Close() error {
	if set == nil {
		return nil
	}
	var result error
	for _, file := range []*os.File{set.mindweaver.file, set.driver.file, set.browser.file, set.root} {
		if file != nil {
			result = errors.Join(result, file.Close())
		}
	}
	return result
}

func sameArtifactPath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
