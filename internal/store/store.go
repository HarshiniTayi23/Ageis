// Package store defines the object-storage abstraction Aegis writes to and a
// local write-once-read-many (WORM) implementation.
//
// The interface is deliberately the shape of S3/GCS/Azure Blob so a cloud
// backend (S3 with Object Lock in COMPLIANCE mode) can be dropped in without
// touching the backup engine.
package store

import (
	"context"
	"errors"
)

var (
	ErrNotFound  = errors.New("store: object not found")
	ErrExists    = errors.New("store: object already exists (write-once)")
	ErrImmutable = errors.New("store: object is under retention lock")
)

// ObjectStore is a flat key/value blob store.
type ObjectStore interface {
	// Put writes a new object. It MUST fail with ErrExists if key exists:
	// backups are append-only, nothing is ever overwritten.
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Exists(ctx context.Context, key string) (bool, error)
	List(ctx context.Context, prefix string) ([]string, error)
	// Delete MUST fail with ErrImmutable while the object is retained.
	Delete(ctx context.Context, key string) error
}
