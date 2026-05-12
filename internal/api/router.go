package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lohitakshgupta/GoApp/internal/job"
	"github.com/lohitakshgupta/GoApp/internal/observability"
)

type JobService interface {
	Submit(ctx context.Context, req job.CreateRequest) (job.Job, error)
	Get(ctx context.Context, id string) (job.Job, error)
	List(ctx context.Context) ([]job.Job, error)
}

type Queue interface {
	Enqueue(ctx context.Context, id string) error
}

type Config struct {
	Service JobService
	Pool    Queue
	Metrics *observability.Metrics
	Logger  *slog.Logger
}

type Handler struct {
	service JobService
	pool    Queue
	metrics *observability.Metrics
	logger  *slog.Logger
}

func NewRouter(cfg Config) http.Handler {
	h := &Handler{
		service: cfg.Service,
		pool:    cfg.Pool,
		metrics: cfg.Metrics,
		logger:  cfg.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/jobs", h.createJob)
	mux.HandleFunc("GET /v1/jobs", h.listJobs)
	mux.HandleFunc("GET /v1/jobs/", h.getJob)
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("GET /metrics", h.metricsEndpoint)
	return h.withLogging(mux)
}

func (h *Handler) createJob(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req job.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	created, err := h.service.Submit(r.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, job.ErrInvalidJob) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	if err := h.pool.Enqueue(r.Context(), created.ID); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, created)
}

func (h *Handler) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job.Page{Jobs: jobs})
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	j, err := h.service.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, job.ErrJobNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) metricsEndpoint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(h.metrics.Prometheus()))
}

func (h *Handler) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		h.logger.Info("request completed", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
