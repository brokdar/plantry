package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// ProfileHandler holds the HTTP handlers for the user profile.
type ProfileHandler struct {
	svc *profile.Service
}

// NewProfileHandler creates a ProfileHandler.
func NewProfileHandler(svc *profile.Service) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

type profileRequest struct {
	KcalTarget          *float64       `json:"kcal_target"`
	ProteinPct          *float64       `json:"protein_pct"`
	FatPct              *float64       `json:"fat_pct"`
	CarbsPct            *float64       `json:"carbs_pct"`
	DietaryRestrictions []string       `json:"dietary_restrictions"`
	Preferences         map[string]any `json:"preferences"`
	SystemPrompt        *string        `json:"system_prompt"`
	Locale              string         `json:"locale"`
}

type profileResponse struct {
	KcalTarget          *float64       `json:"kcal_target"`
	ProteinPct          *float64       `json:"protein_pct"`
	FatPct              *float64       `json:"fat_pct"`
	CarbsPct            *float64       `json:"carbs_pct"`
	DietaryRestrictions []string       `json:"dietary_restrictions"`
	Preferences         map[string]any `json:"preferences"`
	SystemPrompt        *string        `json:"system_prompt"`
	Locale              string         `json:"locale"`
	UpdatedAt           string         `json:"updated_at"`
}

func toProfileResponse(p *profile.Profile) profileResponse {
	restrictions := p.DietaryRestrictions
	if restrictions == nil {
		restrictions = []string{}
	}
	prefs := p.Preferences
	if prefs == nil {
		prefs = map[string]any{}
	}
	var updatedAt string
	if !p.UpdatedAt.IsZero() {
		updatedAt = p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return profileResponse{
		KcalTarget:          p.KcalTarget,
		ProteinPct:          p.ProteinPct,
		FatPct:              p.FatPct,
		CarbsPct:            p.CarbsPct,
		DietaryRestrictions: restrictions,
		Preferences:         prefs,
		SystemPrompt:        p.SystemPrompt,
		Locale:              p.Locale,
		UpdatedAt:           updatedAt,
	}
}

// Get handles GET /api/profile.
func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, toProfileResponse(p))
}

// Update handles PUT /api/profile.
func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}

	restrictions := req.DietaryRestrictions
	if restrictions == nil {
		restrictions = []string{}
	}
	prefs := req.Preferences
	if prefs == nil {
		prefs = map[string]any{}
	}
	locale := req.Locale
	if locale == "" {
		locale = "en"
	}

	p := &profile.Profile{
		KcalTarget:          req.KcalTarget,
		ProteinPct:          req.ProteinPct,
		FatPct:              req.FatPct,
		CarbsPct:            req.CarbsPct,
		DietaryRestrictions: restrictions,
		Preferences:         prefs,
		SystemPrompt:        req.SystemPrompt,
		Locale:              locale,
	}

	updated, err := h.svc.Update(r.Context(), p)
	if err != nil {
		status, key := toHTTPWithResource(err, "profile")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toProfileResponse(updated))
}
