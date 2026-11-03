package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LocalWORM is a filesystem-backed ObjectStore that refuses overwrites and
// refuses deletes until an object's retention period has elapsed.
//
// Honest caveat: on a local disk this is cooperative enforcement - root (or
// ransomware running as root) can still chmod and delete. Real immutability
// requires an enforcement point the attacker cannot reach, e.g. S3 Object
// Lock in COMPLIANCE mode, which even the bucket owner cannot bypass. That is
// what the S3 backend (roadmap milestone 2) is for.
type LocalWORM struct {
	root      string
	retention time.Duration
	now       func() time.Time
}

// NewLocalWORM creates the store rooted at dir.
func NewLocalWORM(dir string, retention time.Duration) (*LocalWORM, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &LocalWORM{root: dir, retention: retention, now: time.Now}, nil
}

func (s *LocalWORM) path(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || filepath.IsAbs(key) {
		return "", fmt.Errorf("store: invalid key %q", key)
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}

// Put writes atomically: write to a temp file, fsync, then hard-link into
// place. link(2) fails if the target exists, which gives us an atomic
// create-if-absent with no race between concurrent writers.
func (s *LocalWORM) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o444); err != nil {
		return err
	}
	if err := os.Link(tmpName, p); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrExists
		}
		return err
	}
	return nil
}

func (s *LocalWORM) Get(ctx context.Context, key string) ([]byte, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *LocalWORM) Exists(ctx context.Context, key string) (bool, error) {
	p, err := s.path(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *LocalWORM) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	sort.Strings(keys)
	return keys, err
}

func (s *LocalWORM) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if s.now().Before(fi.ModTime().Add(s.retention)) {
		return ErrImmutable
	}
	return os.Remove(p)
}
