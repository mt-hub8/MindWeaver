package supplychain

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha3"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const (
	sqliteUpstreamVersion        = "3.53.4"
	sqliteUpstreamUUID           = "bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc"
	sqliteUpstreamSourceIDPrefix = "2026-07-24 19:02:57 "
)

type sqliteUpstreamFileContract struct {
	name             string
	size             int64
	sha256           string
	fossilArtifactID string
}

var sqliteUpstreamFiles = []sqliteUpstreamFileContract{
	{
		name:             "LICENSE.md",
		size:             3850,
		sha256:           "ee6af51062b30d532991face5164136ae6f84e265ecf8abe89dc69dac45ca1e7",
		fossilArtifactID: "6bc480fc673fb4acbc4094e77edb326267dd460162d7723c7f30bee2d3d9e97d",
	},
	{
		name:   "manifest.uuid",
		size:   65,
		sha256: "c613b5f6581cba368618c41ccfc16e476f65efa963c9c7c9fc3b4519daea71d9",
	},
}

func TestSQLiteTranslationUpstreamProvenanceMutationsFailClosed(t *testing.T) {
	root := moduleRoot(t)
	source := filepath.Join(root, "third_party", "sqlite", sqliteUpstreamVersion)
	valid := func(t *testing.T) string {
		t.Helper()
		target := t.TempDir()
		evidence := filepath.Join(target, "third_party", "sqlite", sqliteUpstreamVersion)
		if err := os.MkdirAll(evidence, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, contract := range sqliteUpstreamFiles {
			contents, err := os.ReadFile(filepath.Join(source, contract.name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(evidence, contract.name), contents, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return target
	}

	t.Run("license bytes", func(t *testing.T) {
		target := valid(t)
		path := filepath.Join(target, "third_party", "sqlite", sqliteUpstreamVersion, "LICENSE.md")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		contents[0] ^= 1
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = validateSQLiteUpstreamFiles(target)
		requireSQLiteProvenanceError(t, err, "hash")
	})

	t.Run("extra file", func(t *testing.T) {
		target := valid(t)
		path := filepath.Join(target, "third_party", "sqlite", sqliteUpstreamVersion, "unreviewed.txt")
		if err := os.WriteFile(path, []byte("not evidence\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := validateSQLiteUpstreamFiles(target)
		requireSQLiteProvenanceError(t, err, "file exact set")
	})

	t.Run("extra version", func(t *testing.T) {
		target := valid(t)
		if err := os.Mkdir(filepath.Join(target, "third_party", "sqlite", "3.54.0"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := validateSQLiteUpstreamFiles(target)
		requireSQLiteProvenanceError(t, err, "version exact set")
	})

	t.Run("non regular file", func(t *testing.T) {
		target := valid(t)
		path := filepath.Join(target, "third_party", "sqlite", sqliteUpstreamVersion, "LICENSE.md")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := validateSQLiteUpstreamFiles(target)
		requireSQLiteProvenanceError(t, err, "regular non-link")
	})

	t.Run("runtime version", func(t *testing.T) {
		requireSQLiteProvenanceError(t, validateSQLiteRuntimeIdentity("3.53.3", sqliteUpstreamSourceIDPrefix+sqliteUpstreamUUID, sqliteUpstreamUUID), "version")
	})

	t.Run("runtime source id", func(t *testing.T) {
		requireSQLiteProvenanceError(t, validateSQLiteRuntimeIdentity(sqliteUpstreamVersion, "2026-07-24 19:02:57 "+strings.Repeat("0", 64), sqliteUpstreamUUID), "source ID")
	})

	t.Run("runtime manifest uuid", func(t *testing.T) {
		requireSQLiteProvenanceError(t, validateSQLiteRuntimeIdentity(sqliteUpstreamVersion, sqliteUpstreamSourceIDPrefix+sqliteUpstreamUUID, strings.Repeat("0", 64)), "source ID")
	})

	t.Run("vendor retention statement", func(t *testing.T) {
		requireSQLiteProvenanceError(t, validateSQLiteRetentionStatement([]byte("Everything else is licensed under MIT-0.\n")), "retention")
	})

	t.Run("upstream license statement", func(t *testing.T) {
		requireSQLiteProvenanceError(t, validateSQLiteLicenseStatement([]byte("unrelated license text\n")), "public-domain scope")
	})

	t.Run("fossil artifact id", func(t *testing.T) {
		contents, err := os.ReadFile(filepath.Join(source, "LICENSE.md"))
		if err != nil {
			t.Fatal(err)
		}
		requireSQLiteProvenanceError(t, validateSQLiteFossilArtifact(contents, strings.Repeat("0", 64)), "Fossil artifact")
	})
}

func validateSQLiteTranslationUpstreamProvenance(t *testing.T, root, moduleCache string) {
	t.Helper()
	uuid, err := validateSQLiteUpstreamFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteModuleEvidence(moduleCache); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()

	var version, sourceID string
	if err := database.QueryRowContext(t.Context(), "SELECT sqlite_version(), sqlite_source_id()").Scan(&version, &sourceID); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteRuntimeIdentity(version, sourceID, uuid); err != nil {
		t.Fatal(err)
	}
}

func validateSQLiteUpstreamFiles(root string) (string, error) {
	versionsRoot := filepath.Join(root, "third_party", "sqlite")
	versions, err := os.ReadDir(versionsRoot)
	if err != nil {
		return "", fmt.Errorf("read SQLite upstream versions: %w", err)
	}
	versionNames := make([]string, 0, len(versions))
	for _, version := range versions {
		versionNames = append(versionNames, version.Name())
	}
	if !slices.Equal(versionNames, []string{sqliteUpstreamVersion}) {
		return "", fmt.Errorf("SQLite upstream version exact set = %q, want %q", versionNames, []string{sqliteUpstreamVersion})
	}

	versionRoot := filepath.Join(versionsRoot, sqliteUpstreamVersion)
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		return "", fmt.Errorf("read SQLite upstream evidence: %w", err)
	}
	wantNames := make([]string, 0, len(sqliteUpstreamFiles))
	gotNames := make([]string, 0, len(entries))
	for _, contract := range sqliteUpstreamFiles {
		wantNames = append(wantNames, contract.name)
	}
	for _, entry := range entries {
		gotNames = append(gotNames, entry.Name())
	}
	slices.Sort(wantNames)
	slices.Sort(gotNames)
	if !slices.Equal(gotNames, wantNames) {
		return "", fmt.Errorf("SQLite upstream file exact set = %q, want %q", gotNames, wantNames)
	}

	var uuid string
	for _, contract := range sqliteUpstreamFiles {
		path := filepath.Join(versionRoot, contract.name)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != contract.size {
			return "", fmt.Errorf("SQLite upstream evidence %s is not the expected bounded regular non-link file", contract.name)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		sha256Digest := sha256.Sum256(contents)
		if hex.EncodeToString(sha256Digest[:]) != contract.sha256 {
			return "", fmt.Errorf("SQLite upstream evidence %s hash mismatch", contract.name)
		}
		if contract.fossilArtifactID != "" {
			if err := validateSQLiteFossilArtifact(contents, contract.fossilArtifactID); err != nil {
				return "", fmt.Errorf("SQLite upstream evidence %s: %w", contract.name, err)
			}
		}
		if contract.name == "manifest.uuid" {
			if len(contents) != 65 || contents[64] != '\n' || !bytes.Equal(contents[:64], []byte(sqliteUpstreamUUID)) {
				return "", errors.New("SQLite manifest UUID is not the exact lowercase source identity")
			}
			uuid = string(contents[:64])
		}
		if contract.name == "LICENSE.md" {
			if err := validateSQLiteLicenseStatement(contents); err != nil {
				return "", err
			}
		}
	}
	if uuid == "" {
		return "", errors.New("SQLite manifest UUID is missing")
	}
	return uuid, nil
}

func validateSQLiteModuleEvidence(moduleCache string) error {
	moduleRoot := filepath.Join(moduleCache, "github.com", "ncruces", "go-sqlite3-wasm", "v3@v3.2.35304")
	statement, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
	if err != nil {
		return err
	}
	return validateSQLiteRetentionStatement(statement)
}

func validateSQLiteFossilArtifact(contents []byte, expectedID string) error {
	digest := sha3.Sum256(contents)
	if hex.EncodeToString(digest[:]) != expectedID {
		return errors.New("SQLite upstream Fossil artifact mismatch")
	}
	return nil
}

func validateSQLiteRetentionStatement(statement []byte) error {
	for _, required := range [][]byte{
		[]byte("the original authors retain copyright"),
		[]byte("the original licenses remain in effect"),
	} {
		if !bytes.Contains(statement, required) {
			return errors.New("SQLite translation retention statement is missing")
		}
	}
	return nil
}

func validateSQLiteLicenseStatement(statement []byte) error {
	for _, required := range [][]byte{
		[]byte("SQLite Is Public Domain"),
		[]byte("All of the primary SQLite source code files found in the"),
		[]byte("All of the SQLite extension source code and test cases in the"),
		[]byte("All code that ends up in the \"sqlite3.c\" and \"sqlite3.h\" build products"),
	} {
		if !bytes.Contains(statement, required) {
			return errors.New("SQLite upstream public-domain scope statement is missing")
		}
	}
	return nil
}

func validateSQLiteRuntimeIdentity(version, sourceID, uuid string) error {
	if version != sqliteUpstreamVersion {
		return fmt.Errorf("SQLite runtime version = %q, want %q", version, sqliteUpstreamVersion)
	}
	if sourceID != sqliteUpstreamSourceIDPrefix+uuid {
		return fmt.Errorf("SQLite runtime source ID = %q, want exact reviewed source ID", sourceID)
	}
	return nil
}

func requireSQLiteProvenanceError(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("error = %v, want containing %q", err, contains)
	}
}
