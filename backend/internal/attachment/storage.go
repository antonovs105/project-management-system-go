package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// LocalStore is a content-checksummed filesystem object store for a mounted persistent volume.
type LocalStore struct{ root string }

// NewLocalStore prepares a private object root.
func NewLocalStore(root string) (*LocalStore, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return &LocalStore{root: absolute}, nil
}

// Put writes an immutable object atomically while computing its checksum.
func (s *LocalStore) Put(ctx context.Context, key string, source io.Reader) (int64, string, error) {
	target, err := s.path(key)
	if err != nil {
		return 0, "", err
	}
	temporary, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return 0, "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	limited := io.LimitReader(source, MaxSizeBytes+1)
	written, copyErr := copyWithContext(ctx, io.MultiWriter(temporary, hash), limited)
	closeErr := temporary.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	if written < 1 || written > MaxSizeBytes {
		return 0, "", ErrInvalidFile
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(hash.Sum(nil)), nil
}

// Open streams one immutable object.
func (s *LocalStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Delete removes one immutable object.
func (s *LocalStore) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// List returns immutable UUID-named objects for reconciliation.
func (s *LocalStore) List(ctx context.Context) ([]StoredObject, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	objects := make([]StoredObject, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() {
			continue
		}
		if _, err := uuid.Parse(entry.Name()); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		objects = append(objects, StoredObject{Key: entry.Name(), ModifiedAt: info.ModTime().UTC()})
	}
	return objects, nil
}

// path rejects non-UUID object keys before joining them to the storage root.
func (s *LocalStore) path(key string) (string, error) {
	if _, err := uuid.Parse(key); err != nil {
		return "", fmt.Errorf("invalid attachment object key")
	}
	return filepath.Join(s.root, key), nil
}

// copyWithContext makes large writes cancelable.
func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
