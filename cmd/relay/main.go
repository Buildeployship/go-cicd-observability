package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/Buildeployship/go-cicd-observability/internal/handler"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting webhook relay", "port", "8080")

	webhookHandler := handler.NewWebhookHandler()

	http.HandleFunc("/webhook", webhookHandler.HandleWebhook)
	http.HandleFunc("/health", handleHealth)
	http.Handle("/metrics", promhttp.Handler())

	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}
