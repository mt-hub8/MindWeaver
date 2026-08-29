package sqliteprobe

import (
	"errors"
	"strings"

	"github.com/ncruces/go-sqlite3"
)

// ErrorInfo is a stable, numeric view of an SQLite error. Application code
// must not branch on localized or version-dependent error strings.
type ErrorInfo struct {
	Message      string `json:"message"`
	PrimaryCode  int    `json:"primary_code"`
	ExtendedCode int    `json:"extended_code"`
	PrimaryName  string `json:"primary_name"`
	ExtendedName string `json:"extended_name"`
	Category     string `json:"category"`
	Retryable    bool   `json:"retryable"`
}

// ClassifySQLiteError extracts SQLite's primary and extended numeric result
// codes. The boolean is false when err is not an SQLite error.
func ClassifySQLiteError(err error) (ErrorInfo, bool) {
	if err == nil {
		return ErrorInfo{}, false
	}

	var primary sqlite3.ErrorCode
	var extended sqlite3.ExtendedErrorCode
	var sqliteErr *sqlite3.Error
	switch {
	case errors.As(err, &sqliteErr):
		primary = sqliteErr.Code()
		extended = sqliteErr.ExtendedCode()
	case errors.As(err, &extended):
		primary = extended.Code()
	case errors.As(err, &primary):
		extended = primary.ExtendedCode()
	default:
		return ErrorInfo{}, false
	}

	info := ErrorInfo{
		Message:      err.Error(),
		PrimaryCode:  int(primary),
		ExtendedCode: int(extended),
		PrimaryName:  sqliteCodeName(primary.Error()),
		ExtendedName: sqliteCodeName(extended.Error()),
	}
	switch primary {
	case sqlite3.BUSY, sqlite3.LOCKED:
		info.Category = "contention"
		info.Retryable = true
	case sqlite3.FULL:
		info.Category = "capacity"
	case sqlite3.INTERRUPT:
		info.Category = "interrupted"
	case sqlite3.CONSTRAINT:
		info.Category = "constraint"
	case sqlite3.CORRUPT, sqlite3.NOTADB:
		info.Category = "corruption"
	default:
		info.Category = "sqlite"
	}
	return info, true
}

func sqliteCodeName(s string) string {
	return strings.TrimPrefix(s, "sqlite3: ")
}
