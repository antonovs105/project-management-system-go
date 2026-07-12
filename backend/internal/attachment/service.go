package attachment

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service coordinates authorization, validation, scanning, storage, and metadata.
type Service struct {
	repository  *Repository
	store       ObjectStore
	permissions PermissionChecker
	scanner     MalwareScanner
}

// NewService returns an attachment service.
func NewService(repository *Repository, store ObjectStore, permissions PermissionChecker, scanner MalwareScanner) *Service {
	return &Service{repository: repository, store: store, permissions: permissions, scanner: scanner}
}

// Upload validates and stores one immutable ticket attachment.
func (s *Service) Upload(ctx context.Context, ticketID, userID, filename string, source io.ReadSeeker) (*Attachment, error) {
	projectID, err := s.repository.TicketProjectID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.require(ctx, projectID, userID, "comments.create"); err != nil {
		return nil, err
	}
	filename = cleanFilename(filename)
	if filename == "" {
		return nil, ErrInvalidFile
	}
	header := make([]byte, 512)
	count, err := source.Read(header)
	if err != nil && err != io.EOF {
		return nil, err
	}
	contentType := http.DetectContentType(header[:count])
	if !allowedContentType(contentType) {
		return nil, ErrInvalidFile
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if s.scanner != nil {
		if err := s.scanner.Scan(ctx, io.LimitReader(source, MaxSizeBytes+1)); err != nil {
			return nil, err
		}
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}
	objectKey := uuid.NewString()
	size, checksum, err := s.store.Put(ctx, objectKey, source)
	if err != nil {
		return nil, err
	}
	value := &Attachment{TicketID: ticketID, UploaderID: userID, ObjectKey: objectKey, Filename: filename, ContentType: contentType, SizeBytes: size, SHA256: checksum}
	if err := s.repository.Create(ctx, value); err != nil {
		_ = s.store.Delete(ctx, objectKey)
		return nil, err
	}
	return value, nil
}

// List authorizes and returns attachment metadata.
func (s *Service) List(ctx context.Context, ticketID, userID string) ([]Attachment, error) {
	projectID, err := s.repository.TicketProjectID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if err := s.require(ctx, projectID, userID, "project.read"); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, ticketID)
}

// Open authorizes and opens one stored attachment.
func (s *Service) Open(ctx context.Context, attachmentID, userID string) (*Attachment, io.ReadCloser, error) {
	value, projectID, err := s.repository.Get(ctx, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	if err := s.require(ctx, projectID, userID, "project.read"); err != nil {
		return nil, nil, err
	}
	reader, err := s.store.Open(ctx, value.ObjectKey)
	return value, reader, err
}

// Delete removes metadata and its object for the uploader or a ticket moderator.
func (s *Service) Delete(ctx context.Context, attachmentID, userID string) error {
	value, projectID, err := s.repository.Get(ctx, attachmentID)
	if err != nil {
		return err
	}
	permission := "project.read"
	if value.UploaderID != userID {
		permission = "tickets.delete"
	}
	if err := s.require(ctx, projectID, userID, permission); err != nil {
		return err
	}
	key, err := s.repository.Delete(ctx, attachmentID)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, key)
}

// CleanupOrphans deletes old objects that no longer have metadata, including
// objects orphaned by cascaded project or ticket deletion.
func (s *Service) CleanupOrphans(ctx context.Context, cutoff time.Time) (int, error) {
	lister, ok := s.store.(ObjectLister)
	if !ok {
		return 0, nil
	}
	referenced, err := s.repository.ObjectKeys(ctx)
	if err != nil {
		return 0, err
	}
	objects, err := lister.List(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, object := range objects {
		if _, ok := referenced[object.Key]; ok || object.ModifiedAt.After(cutoff) {
			continue
		}
		if err := s.store.Delete(ctx, object.Key); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// StartOrphanCleanupLoop reconciles old unreferenced objects until stopped.
func (s *Service) StartOrphanCleanupLoop(ctx context.Context, interval, gracePeriod time.Duration, report func(int, error)) context.CancelFunc {
	loopContext, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			deleted, err := s.CleanupOrphans(loopContext, time.Now().UTC().Add(-gracePeriod))
			if report != nil {
				report(deleted, err)
			}
			select {
			case <-loopContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

// require checks one project permission.
func (s *Service) require(ctx context.Context, projectID, userID, permission string) error {
	allowed, err := s.permissions.HasPermission(ctx, projectID, userID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// cleanFilename strips path components and control characters.
func cleanFilename(value string) string {
	value = path.Base(strings.ReplaceAll(strings.TrimSpace(value), `\`, "/"))
	value = strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return -1
		}
		return character
	}, value)
	if value == "." || value == ".." || len([]byte(value)) > 255 {
		return ""
	}
	return value
}

// allowedContentType is a conservative product attachment allow-list.
func allowedContentType(value string) bool {
	value = strings.Split(value, ";")[0]
	if strings.HasPrefix(value, "image/") {
		return value != "image/svg+xml"
	}
	switch value {
	case "application/pdf", "application/json", "application/zip", "text/plain", "text/csv":
		return true
	default:
		return false
	}
}
