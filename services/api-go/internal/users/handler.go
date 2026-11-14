package users

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"lokasolve/services/api-go/internal/auth"
	"lokasolve/services/api-go/internal/identitymode"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/validate"
)

type Handler struct {
	Service *Service
}

func (h Handler) Routes(r chi.Router) {
	r.Get("/me", h.me)
	r.Patch("/me", h.update)
	r.Get("/me/locations", h.locations)
	r.Post("/me/locations", h.createLocation)
	r.Delete("/me/locations/{locationID}", h.deleteLocation)
	r.Post("/me/deletion-request", h.deletion)
	r.Get("/feature-flags", h.flags)
}

func (h Handler) me(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	user, err := h.Service.Me(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, user)
}

func (h Handler) update(w http.ResponseWriter, r *http.Request) {
	if identitymode.Blocked(w, r) {
		return
	}
	principal, _ := auth.Current(r.Context())
	var req struct {
		DisplayName             *string          `json:"display_name"`
		PreferredLanguage       *string          `json:"preferred_language"`
		NotificationPreferences *map[string]bool `json:"notification_preferences"`
		PrivacyPreferences      *map[string]bool `json:"privacy_preferences"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	user, err := h.Service.Update(r.Context(), principal, ProfileUpdate{
		DisplayName: req.DisplayName, PreferredLanguage: req.PreferredLanguage,
		NotificationPreferences: req.NotificationPreferences, PrivacyPreferences: req.PrivacyPreferences,
		RequestID: httpx.RequestID(r.Context()),
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, user)
}

func (h Handler) locations(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	places, err := h.Service.Locations(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": places})
}

func (h Handler) createLocation(w http.ResponseWriter, r *http.Request) {
	if identitymode.Blocked(w, r) {
		return
	}
	principal, _ := auth.Current(r.Context())
	var req struct {
		Label        *string  `json:"label"`
		AddressText  *string  `json:"address_text"`
		LocalityText *string  `json:"locality_text"`
		Latitude     *float64 `json:"latitude"`
		Longitude    *float64 `json:"longitude"`
		Source       string   `json:"source"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	place, err := h.Service.CreateLocation(r.Context(), principal, LocationInput{
		Label: req.Label, AddressText: req.AddressText, LocalityText: req.LocalityText,
		Latitude: req.Latitude, Longitude: req.Longitude, Source: req.Source,
		RequestID: httpx.RequestID(r.Context()),
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, place)
}

func (h Handler) deleteLocation(w http.ResponseWriter, r *http.Request) {
	if identitymode.Blocked(w, r) {
		return
	}
	principal, _ := auth.Current(r.Context())
	locationID, message := validate.ID(chi.URLParam(r, "locationID"))
	if message != "" {
		httpx.WriteError(w, r, httpx.Validation(map[string]string{"location_id": message}))
		return
	}
	if err := h.Service.DeleteLocation(r.Context(), principal, locationID, httpx.RequestID(r.Context())); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) deletion(w http.ResponseWriter, r *http.Request) {
	if identitymode.Blocked(w, r) {
		return
	}
	principal, _ := auth.Current(r.Context())
	request, err := h.Service.RequestDeletion(r.Context(), principal, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, request)
}

func (h Handler) flags(w http.ResponseWriter, r *http.Request) {
	flags, err := h.Service.FeatureFlags(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"flags": flags})
}
