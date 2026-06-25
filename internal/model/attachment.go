package model

import "time"

// Attachment is a file uploaded to an issue. The bytes live on disk under
// StorageKey (relative to ATTACHMENTS_DIR); StorageKey is never serialized.
type Attachment struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	IssueID        string    `json:"issue_id"`
	UploadedBy     *string   `json:"uploaded_by"`
	UploaderName   *string   `json:"uploader_name"`
	Filename       string    `json:"filename"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	StorageKey     string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}
