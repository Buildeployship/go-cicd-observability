package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_requests_total",
			Help: "Total number of webhook requests",
		},
		[]string{"status"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "webhook_request_duration_seconds",
			Help:    "Duration of webhook requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	payloadSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "webhook_payload_size_bytes",
			Help:    "Size of webhook payloads in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000},
		},
	)
)

var eventCounter uint64

type WebhookHandler struct{}

func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{}
}

type WebhookResponse struct {
	Status  string `json:"status"`
	EventID string `json:"event_id"`
	Message string `json:"message"`
}

func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		h.respondError(w, "method not allowed", http.StatusMethodNotAllowed, start)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.respondError(w, "failed to read body", http.StatusBadRequest, start)
		return
	}
	defer func() { _ = r.Body.Close() }()

	payloadSize.Observe(float64(len(body)))

	eventID := fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&eventCounter, 1))

	slog.Info("webhook received",
		"event_id", eventID,
		"content_type", r.Header.Get("Content-Type"),
		"payload_size", len(body),
		"source_ip", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"),
	)

	duration := time.Since(start).Seconds()
	requestCounter.WithLabelValues("success").Inc()
	requestDuration.WithLabelValues("success").Observe(duration)

	response := WebhookResponse{
		Status:  "received",
		EventID: eventID,
		Message: "webhook processed successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *WebhookHandler) respondError(w http.ResponseWriter, message string, status int, start time.Time) {
	duration := time.Since(start).Seconds()
	requestCounter.WithLabelValues("error").Inc()
	requestDuration.WithLabelValues("error").Observe(duration)

	slog.Error("webhook error", "message", message, "status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
