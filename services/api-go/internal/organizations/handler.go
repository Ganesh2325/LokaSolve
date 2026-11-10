package organizations

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/auth"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/validate"
)

type Handler struct {
	Service *Service
}

func (h Handler) Routes(r chi.Router) {
	r.Get("/me/organizations", h.mine)
	r.Post("/organizations", h.create)
	r.Get("/organizations/{organizationID}", h.get)
	r.Get("/organizations/{organizationID}/members", h.members)
	r.Post("/organizations/{organizationID}/members", h.addMember)
	r.Delete("/organizations/{organizationID}/members/{userID}", h.removeMember)
	r.Get("/organizations/{organizationID}/departments", h.departments)
	r.Post("/organizations/{organizationID}/departments", h.createDepartment)
	r.Post("/organizations/{organizationID}/teams", h.createTeam)
}

func (h Handler) mine(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgs, err := h.Service.Mine(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": orgs})
}

func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	var req struct {
		Name string `json:"name"`
		Type string `json:"org_type"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	org, err := h.Service.Create(r.Context(), principal, CreateInput{
		Name: req.Name, Type: req.Type, RequestID: httpx.RequestID(r.Context()),
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, org)
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	org, err := h.Service.Get(r.Context(), principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, org)
}

func (h Handler) members(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	members, err := h.Service.Members(r.Context(), principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

func (h Handler) addMember(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	var req struct {
		Email      string `json:"email"`
		MemberRole string `json:"member_role"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.AddMember(r.Context(), principal, orgID, MemberInput{
		Email: req.Email, MemberRole: req.MemberRole, RequestID: httpx.RequestID(r.Context()),
	}); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	userID, ok := pathID(w, r, "userID")
	if !ok {
		return
	}
	if err := h.Service.RemoveMember(r.Context(), principal, orgID, userID, httpx.RequestID(r.Context())); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) departments(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	departments, err := h.Service.Departments(r.Context(), principal, orgID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": departments})
}

func (h Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	department, err := h.Service.CreateDepartment(r.Context(), principal, orgID, req.Name, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, department)
}

func (h Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	orgID, ok := pathID(w, r, "organizationID")
	if !ok {
		return
	}
	var req struct {
		Name         string `json:"name"`
		DepartmentID string `json:"department_id"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var departmentID *uuid.UUID
	if req.DepartmentID != "" {
		id, message := validate.ID(req.DepartmentID)
		if message != "" {
			httpx.WriteError(w, r, httpx.Validation(map[string]string{"department_id": message}))
			return
		}
		departmentID = &id
	}
	team, err := h.Service.CreateTeam(r.Context(), principal, orgID, req.Name, departmentID, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, team)
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, message := validate.ID(chi.URLParam(r, name))
	if message != "" {
		httpx.WriteError(w, r, httpx.New(http.StatusNotFound, "not_found", "That record was not found."))
		return uuid.Nil, false
	}
	return id, true
}
