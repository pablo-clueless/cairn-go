package work

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cairn/internal/model"
	"cairn/internal/store"
)

var ErrFileTooLarge = errors.New("work: file exceeds the maximum upload size")

// ListAttachments returns an issue's attachments.
func (s *Service) ListAttachments(ctx context.Context, orgID, issueKey string) ([]model.Attachment, error) {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListAttachmentsByIssue(ctx, orgID, issue.ID)
}

// CreateAttachment streams an uploaded file to disk and records its metadata.
// It enforces the configured size cap while copying.
func (s *Service) CreateAttachment(ctx context.Context, orgID, actorID, issueKey, filename, contentType string, src io.Reader) (*model.Attachment, error) {
	filename = sanitizeFilename(filename)
	if filename == "" {
		return nil, fmt.Errorf("%w: filename is required", ErrValidation)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}

	key, err := storageKey(orgID, filename)
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(s.attachmentsDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, fmt.Errorf("work: prepare storage dir: %w", err)
	}

	size, err := s.writeFile(dest, src)
	if err != nil {
		_ = os.Remove(dest)
		return nil, err
	}

	att, err := s.store.CreateAttachment(ctx, orgID, issue.ID, actorID, filename, contentType, key, size)
	if err != nil {
		_ = os.Remove(dest)
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "attachment.created", "attachment", att.ID, map[string]any{
		"issue_id": issue.ID, "filename": filename,
	})
	s.autoWatch(ctx, orgID, issue.ID, actorID)
	return att, nil
}

// writeFile copies src to dest, failing with ErrFileTooLarge past the cap.
func (s *Service) writeFile(dest string, src io.Reader) (int64, error) {
	f, err := os.Create(dest)
	if err != nil {
		return 0, fmt.Errorf("work: create file: %w", err)
	}
	defer f.Close()

	// Read one byte past the limit to detect oversize without trusting headers.
	limit := s.maxUploadBytes
	written, err := io.Copy(f, io.LimitReader(src, limit+1))
	if err != nil {
		return 0, fmt.Errorf("work: write file: %w", err)
	}
	if limit > 0 && written > limit {
		return 0, ErrFileTooLarge
	}
	return written, nil
}

// OpenAttachment returns metadata and an open file handle for download. The
// caller must close the returned file.
func (s *Service) OpenAttachment(ctx context.Context, orgID, id string) (*model.Attachment, *os.File, error) {
	att, err := s.store.GetAttachmentByID(ctx, orgID, id)
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(s.attachmentsDir, filepath.FromSlash(att.StorageKey))
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, store.ErrNotFound
		}
		return nil, nil, fmt.Errorf("work: open attachment: %w", err)
	}
	return att, f, nil
}

// DeleteAttachment removes an attachment's row and file. Only the uploader or an
// admin (canModerate) may delete it.
func (s *Service) DeleteAttachment(ctx context.Context, orgID, actorID, id string, canModerate bool) error {
	att, err := s.store.GetAttachmentByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if !canModerate && (att.UploadedBy == nil || *att.UploadedBy != actorID) {
		return ErrForbidden
	}
	if err := s.store.DeleteAttachment(ctx, orgID, id); err != nil {
		return err
	}
	// Best-effort file removal; the row is the source of truth.
	path := filepath.Join(s.attachmentsDir, filepath.FromSlash(att.StorageKey))
	_ = os.Remove(path)
	s.audit(ctx, orgID, actorID, "attachment.deleted", "attachment", att.ID, map[string]any{"issue_id": att.IssueID})
	return nil
}

// storageKey builds a collision-resistant relative path: <orgID>/<random>-<name>.
func storageKey(orgID, filename string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("work: generate storage key: %w", err)
	}
	return orgID + "/" + hex.EncodeToString(buf) + "-" + filename, nil
}

// sanitizeFilename strips path separators and control characters so a client
// filename can't escape the storage directory.
func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}
