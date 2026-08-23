// Package blob stores immutable, content-addressed files inside a local vault.
//
// A Store owns only file bytes. Reference truth belongs to the database layer.
// Imported content is written to a same-volume staging directory, hashed while it
// is copied, synced, and then renamed to its SHA-256 address. A successful Import
// means the destination file and the destination directory were synced where the
// operating system exposes that operation. It is not a promise against faulty
// storage hardware, hostile modification of the vault, or a filesystem that lies
// about flush completion.
//
// On Windows the publishing rename uses MOVEFILE_WRITE_THROUGH and directory
// handles are opened with GENERIC_WRITE before FlushFileBuffers. A flush failure,
// including access denied or an unsupported filesystem, is returned to the caller
// rather than reported as durable success. Retrying an Import is always safe
// because its address is derived from content.
package blob
