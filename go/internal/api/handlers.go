package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"podsentinel/internal/models"
	"podsentinel/internal/store"
)

// handlers holds the dependencies shared by the API's HTTP handlers.
type handlers struct {
	logger *slog.Logger
	store  *store.Store
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
	pods, err := h.store.ListPods(r.Context())
	if err != nil {
		h.logger.Error("listing pods failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list pods"})
		return
	}

	summaries := make([]models.PodSummary, 0, len(pods))
	for _, p := range pods {
		summaries = append(summaries, models.PodSummary{
			Namespace:    p.Namespace,
			Name:         p.Pod,
			Status:       p.Status,
			RestartCount: int(p.RestartCount),
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *handlers) getPod(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	rows, err := h.store.GetPodMetrics(r.Context(), namespace, pod)
	if err != nil {
		h.logger.Error("getting pod metrics failed", "namespace", namespace, "pod", pod, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get pod metrics"})
		return
	}

	detail := models.PodDetail{
		Namespace: namespace,
		Name:      pod,
		Metrics:   make([]models.MetricSample, 0, len(rows)),
	}
	for i, row := range rows {
		if i == len(rows)-1 {
			detail.Status = row.Status
			detail.RestartCount = int(row.RestartCount)
		}
		detail.Metrics = append(detail.Metrics, models.MetricSample{
			Timestamp: row.Time,
			CPU:       row.CPU,
			Memory:    row.Memory,
		})
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *handlers) listPodAnomalies(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	pod := chi.URLParam(r, "pod")

	rows, err := h.store.ListAnomalies(r.Context(), namespace, pod)
	if err != nil {
		h.logger.Error("listing anomalies failed", "namespace", namespace, "pod", pod, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list anomalies"})
		return
	}

	anomalies := make([]models.Anomaly, 0, len(rows))
	for _, a := range rows {
		anomalies = append(anomalies, models.Anomaly{
			Timestamp: a.Time,
			Namespace: a.Namespace,
			Pod:       a.Pod,
			Metric:    a.Metric,
			Value:     a.Value,
			Baseline:  a.Baseline,
			Severity:  a.Severity,
		})
	}
	writeJSON(w, http.StatusOK, anomalies)
}
