// Package httpapi exposes the T02 HTTP contract through application services.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"scale-challenge/internal/application"
	"scale-challenge/internal/domain"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Router() http.Handler {
	router := chi.NewRouter()
	router.Route("/v1", func(router chi.Router) {
		router.Route("/branches", h.branches)
		router.Route("/scales", h.scales)
		router.Route("/trucks", h.trucks)
		router.Route("/grain-types", h.grainTypes)
		router.Route("/transport-transactions", h.transportTransactions)
	})
	return router
}

func (h *Handler) branches(router chi.Router) {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.ListBranches(r.Context())
		respond(w, value, err)
	})
	router.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var input application.BranchInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.CreateBranch(r.Context(), input)
		respondCreated(w, value, err)
	})
	router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.GetBranch(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
	router.Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input application.BranchInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.UpdateBranch(r.Context(), chi.URLParam(r, "id"), input)
		respond(w, value, err)
	})
	router.Post("/{id}/deactivate", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.DeactivateBranch(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
}
func (h *Handler) scales(router chi.Router) {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.ListScales(r.Context())
		respond(w, value, err)
	})
	router.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var input application.ScaleInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.CreateScale(r.Context(), input)
		respondCreated(w, value, err)
	})
	router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.GetScale(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
	router.Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input application.ScaleInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.UpdateScale(r.Context(), chi.URLParam(r, "id"), input)
		respond(w, value, err)
	})
	router.Post("/{id}/deactivate", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.DeactivateScale(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
}
func (h *Handler) trucks(router chi.Router) {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.ListTrucks(r.Context())
		respond(w, value, err)
	})
	router.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var input application.TruckInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.CreateTruck(r.Context(), input)
		respondCreated(w, value, err)
	})
	router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.GetTruck(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
	router.Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input application.TruckInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.UpdateTruck(r.Context(), chi.URLParam(r, "id"), input)
		respond(w, value, err)
	})
	router.Post("/{id}/deactivate", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.DeactivateTruck(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
}
func (h *Handler) grainTypes(router chi.Router) {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.ListGrainTypes(r.Context())
		respond(w, value, err)
	})
	router.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var input application.GrainTypeInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.CreateGrainType(r.Context(), input)
		respondCreated(w, value, err)
	})
	router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.GetGrainType(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
	router.Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input application.GrainTypeInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.UpdateGrainType(r.Context(), chi.URLParam(r, "id"), input)
		respond(w, value, err)
	})
	router.Post("/{id}/deactivate", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.DeactivateGrainType(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
}
func (h *Handler) transportTransactions(router chi.Router) {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.ListTransportTransactions(r.Context())
		respond(w, value, err)
	})
	router.Post("/", func(w http.ResponseWriter, r *http.Request) {
		var input application.TransportInput
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.CreateTransportTransaction(r.Context(), input)
		respondCreated(w, value, err)
	})
	router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.GetTransportTransaction(r.Context(), chi.URLParam(r, "id"))
		respond(w, value, err)
	})
	router.Patch("/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Status string `json:"status"`
		}
		if !decode(w, r, &input) {
			return
		}
		value, err := h.service.TransitionTransportTransaction(r.Context(), chi.URLParam(r, "id"), input.Status)
		respond(w, value, err)
	})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		respondError(w, http.StatusBadRequest, "malformed request body")
		return false
	}
	return true
}
func respondCreated(w http.ResponseWriter, value any, err error) {
	if err != nil {
		respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func respond(w http.ResponseWriter, value any, err error) {
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			respondError(w, http.StatusNotFound, "resource not found")
		case errors.Is(err, domain.ErrConflict):
			respondError(w, http.StatusConflict, "operation conflicts with current state")
		case errors.Is(err, domain.ErrValidation):
			respondError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			respondError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func respondError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"code": http.StatusText(status), "message": message})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
