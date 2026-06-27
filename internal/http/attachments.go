package http

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/work"
)

//	@Summary	List issue attachments
//	@Tags		attachments
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Success	200			{array}	model.Attachment
//	@Router		/orgs/{orgID}/issues/{issueKey}/attachments [get]
func (s *Server) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	if _, ok := s.requireIssueAccess(w, r, scope); !ok {
		return
	}
	items, err := s.work.ListAttachments(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if items == nil {
		items = []model.Attachment{}
	}
	respond(w, http.StatusOK, items)
}

//	@Summary	Upload an attachment
//	@Description	multipart/form-data with a single "file" field.
//	@Tags		attachments
//	@Security	BearerAuth
//	@Accept		multipart/form-data
//	@Param		orgID		path		string	true	"Organization ID or slug"
//	@Param		issueKey	path		string	true	"Issue key, e.g. ENG-123"
//	@Param		file		formData	file	true	"File to upload"
//	@Success	201			{object}	model.Attachment
//	@Router		/orgs/{orgID}/issues/{issueKey}/attachments [post]
func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionAttachmentCreate) {
		return
	}
	if _, ok := s.requireIssueAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())

	// Cap the request body to the configured limit (plus headroom for multipart
	// framing) so a huge upload can't exhaust memory/disk.
	if s.cfg.MaxUploadBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+1<<20)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "expected a multipart 'file' field")
		return
	}
	defer file.Close()

	att, err := s.work.CreateAttachment(r.Context(), scope.Org.ID, user.ID,
		chi.URLParam(r, "issueKey"), header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		if errors.Is(err, work.ErrFileTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "the file is too large")
			return
		}
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, att)
}

//	@Summary	Download an attachment
//	@Tags		attachments
//	@Security	BearerAuth
//	@Param		orgID			path	string	true	"Organization ID or slug"
//	@Param		attachmentID	path	string	true	"Attachment ID"
//	@Success	200
//	@Router		/orgs/{orgID}/attachments/{attachmentID} [get]
func (s *Server) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	att, f, err := s.work.OpenAttachment(r.Context(), scope.Org.ID, chi.URLParam(r, "attachmentID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	defer f.Close()

	// Gate on the attachment's issue space (the route has no space/issue key).
	issue, err := s.work.GetIssueByID(r.Context(), scope.Org.ID, att.IssueID)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, issue.SpaceID) {
		return
	}

	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(att.SizeBytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+att.Filename+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

//	@Summary	Delete an attachment
//	@Tags		attachments
//	@Security	BearerAuth
//	@Param		orgID			path	string	true	"Organization ID or slug"
//	@Param		attachmentID	path	string	true	"Attachment ID"
//	@Success	204
//	@Router		/orgs/{orgID}/attachments/{attachmentID} [delete]
func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionAttachmentDelete) {
		return
	}
	user, _ := userFromContext(r.Context())
	// Admins may delete anyone's attachment; members only their own.
	canModerate := authz.Can(scope.Role, authz.ActionMemberRemove)
	if err := s.work.DeleteAttachment(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "attachmentID"), canModerate); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
