package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/store"
	"cairn/internal/work"
)

type createDocumentRequest struct {
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Status   string  `json:"status"`
	Content  string  `json:"content"`
	ParentID *string `json:"parent_id"`
}

type updateDocumentRequest struct {
	Title    *string `json:"title"`
	Content  *string `json:"content"`
	Status   *string `json:"status"`
	ParentID *string `json:"parent_id"`
}

//	@Summary	List documents
//	@Tags		documents
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.Document
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/documents [get]
func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	docs, err := s.work.ListDocuments(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if docs == nil {
		docs = []model.Document{}
	}
	respond(w, http.StatusOK, docs)
}

//	@Summary	Create document
//	@Tags		documents
//	@Security	BearerAuth
//	@Param		orgID		path		string					true	"Organization ID or slug"
//	@Param		spaceKey	path		string					true	"Space key"
//	@Param		body		body		createDocumentRequest	true	"Document"
//	@Success	201			{object}	model.Document
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/documents [post]
func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionDocumentCreate) {
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())

	var req createDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	doc, err := s.work.CreateDocument(r.Context(), scope.Org.ID, user.ID, work.CreateDocumentInput{
		SpaceKey: chi.URLParam(r, "spaceKey"),
		Title:    req.Title,
		Type:     req.Type,
		Status:   req.Status,
		Content:  req.Content,
		ParentID: req.ParentID,
	})
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, doc)
}

//	@Summary	Get document
//	@Tags		documents
//	@Security	BearerAuth
//	@Param		orgID	path		string	true	"Organization ID or slug"
//	@Param		docID	path		string	true	"Document ID"
//	@Success	200		{object}	model.Document
//	@Router		/orgs/{orgID}/documents/{docID} [get]
func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	doc, err := s.work.GetDocument(r.Context(), scope.Org.ID, chi.URLParam(r, "docID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, doc.SpaceID) {
		return
	}
	respond(w, http.StatusOK, doc)
}

//	@Summary	Update document
//	@Tags		documents
//	@Security	BearerAuth
//	@Param		orgID	path		string					true	"Organization ID or slug"
//	@Param		docID	path		string					true	"Document ID"
//	@Param		body	body		updateDocumentRequest	true	"Fields to change"
//	@Success	200		{object}	model.Document
//	@Router		/orgs/{orgID}/documents/{docID} [patch]
func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionDocumentUpdate) {
		return
	}
	existing, err := s.work.GetDocument(r.Context(), scope.Org.ID, chi.URLParam(r, "docID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, existing.SpaceID) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req updateDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	doc, err := s.work.UpdateDocument(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "docID"), store.DocumentUpdate{
		Title:    req.Title,
		Content:  req.Content,
		Status:   req.Status,
		ParentID: req.ParentID,
	})
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, doc)
}

//	@Summary	Delete document
//	@Tags		documents
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID or slug"
//	@Param		docID	path	string	true	"Document ID"
//	@Success	204
//	@Router		/orgs/{orgID}/documents/{docID} [delete]
func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionDocumentDelete) {
		return
	}
	existing, err := s.work.GetDocument(r.Context(), scope.Org.ID, chi.URLParam(r, "docID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, existing.SpaceID) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.DeleteDocument(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "docID")); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
