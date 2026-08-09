package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/JustinGarvida/PodSentinel/go/internal/models"
)

// handlers holds the dependencies shared by the API's HTTP handlers.
// Placeholder data is returned today; a data source (Postgres) will
// replace it once the agent's ingestion path is implemented.
type handlers struct {
	logger *slog.Logger
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) listPods(w http.ResponseWriter, r *http.Request) {
	pods := []models.PodSummary{
		{Namespace: "default", Name: "example-pod", Status: "Running", RestartCount: 0},
	}
	writeJSON(w, http.StatusOK, pods)
}

func (h *handlers) getPod(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	detail := models.PodDetail{
		Namespace:    namespace,
		Name:         pod,
		Status:       "Running",
		RestartCount: 0,
		Metrics: []models.MetricSample{
			{Timestamp: time.Now().UTC(), CPU: 0, Memory: 0},
		},
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *handlers) listPodAnomalies(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	anomalies := []models.Anomaly{
		{
			Timestamp: time.Now().UTC(),
			Namespace: namespace,
			Pod:       pod,
			Metric:    "cpu",
			Value:     0,
			Baseline:  0,
			Severity:  "none",
		},
	}
	writeJSON(w, http.StatusOK, anomalies)
}
