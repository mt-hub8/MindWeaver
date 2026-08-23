// Package config owns the small versioned bootstrap configuration.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

const (
	CurrentSchemaVersion = 1
	DefaultFileName      = "mindweaver.v1.json"
	maxConfigBytes       = 64 << 10
)

var errDuplicateJSONKey = errors.New("duplicate JSON object key")

// Config contains only choices the first release actually lets a user make.
// Database names, locking and connection policy are implementation details of
// the Vault and deliberately do not appear here.
type Config struct {
	SchemaVersion int         `json:"schema_version"`
	Vault         VaultConfig `json:"vault"`
}

type VaultConfig struct {
	Root string `json:"root"`
}

func Default(vaultRoot string) Config {
	return Config{
		SchemaVersion: CurrentSchemaVersion,
		Vault:         VaultConfig{Root: vaultRoot},
	}
}

func (c Config) Validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return apperror.New(apperror.KindInvalid, "config.unsupported_version", fmt.Sprintf("configuration schema_version must be %d", CurrentSchemaVersion))
	}
	root := strings.TrimSpace(c.Vault.Root)
	if root == "" {
		return apperror.New(apperror.KindInvalid, "config.vault_root_required", "vault.root is required")
	}
	if strings.IndexByte(root, 0) >= 0 {
		return apperror.New(apperror.KindInvalid, "config.vault_root_invalid", "vault.root contains an invalid character")
	}
	return nil
}

// ResolveVault returns the absolute Vault path. Relative roots are resolved
// next to the configuration file rather than against the process working
// directory, so launching from a shortcut cannot silently select another
// Vault.
func ResolveVault(configPath string, cfg Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	root := filepath.Clean(cfg.Vault.Root)
	if !filepath.IsAbs(root) {
		absoluteConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return "", apperror.Wrap(err, apperror.KindInvalid, "config.path_invalid", "config.resolve_vault", "configuration path is invalid")
		}
		root = filepath.Join(filepath.Dir(absoluteConfigPath), root)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", apperror.Wrap(err, apperror.KindInvalid, "config.vault_root_invalid", "config.resolve_vault", "vault.root is invalid")
	}
	return filepath.Clean(absoluteRoot), nil
}

// Load performs one bounded typed JSON decode. Unknown fields and trailing
// values are rejected; callers never consume a partially validated config.
func Load(ctx context.Context, path string) (Config, error) {
	const op = "config.load"
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, apperror.Wrap(err, apperror.KindNotFound, "config.not_found", op, "configuration file was not found")
		}
		return Config{}, apperror.Wrap(err, apperror.KindUnavailable, "config.open_failed", op, "configuration could not be opened")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return Config{}, apperror.Wrap(err, apperror.KindUnavailable, "config.read_failed", op, "configuration could not be read")
	}
	if len(data) > maxConfigBytes {
		return Config{}, apperror.New(apperror.KindInvalid, "config.too_large", "configuration file exceeds 64 KiB")
	}
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	if err := rejectDuplicateKeys(data); err != nil {
		code := "config.invalid_json"
		message := "configuration is not valid versioned JSON"
		if errors.Is(err, errDuplicateJSONKey) {
			code = "config.duplicate_key"
			message = "configuration contains a duplicate object key"
		}
		return Config{}, apperror.Wrap(err, apperror.KindInvalid, code, op, message)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, apperror.Wrap(err, apperror.KindInvalid, "config.invalid_json", op, "configuration is not valid versioned JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return Config{}, apperror.Wrap(err, apperror.KindInvalid, "config.trailing_data", op, "configuration contains trailing data")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return scanJSONValue(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("%w: %q", errDuplicateJSONKey, key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	if end != matchingDelimiter(delimiter) {
		return errors.New("mismatched JSON delimiter")
	}
	return nil
}

func matchingDelimiter(open json.Delim) json.Delim {
	if open == '{' {
		return '}'
	}
	return ']'
}

// WriteNew durably creates a config and never overwrites an existing one.
func WriteNew(ctx context.Context, path string, cfg Config) (err error) {
	const op = "config.write_new"
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return apperror.Wrap(err, apperror.KindInternal, "config.encode_failed", op, "configuration could not be encoded")
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "config.directory_failed", op, "configuration directory could not be created")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return apperror.Wrap(err, apperror.KindConflict, "config.already_exists", op, "configuration file already exists")
		}
		return apperror.Wrap(err, apperror.KindUnavailable, "config.create_failed", op, "configuration file could not be created")
	}
	complete := false
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = apperror.Wrap(closeErr, apperror.KindUnavailable, "config.close_failed", op, "configuration file could not be finalized")
		}
		if !complete {
			_ = os.Remove(path)
		}
	}()

	if _, err := file.Write(data); err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "config.write_failed", op, "configuration file could not be written")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "config.sync_failed", op, "configuration file could not be synchronized")
	}
	complete = true
	return nil
}
