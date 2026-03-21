package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Buildeployship/go-cicd-observability/internal/handler"
	"github.com/Buildeployship/go-cicd-observability/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize telemetry
	collectorEndpoint := getEnv("OTEL_COLLECTOR_ENDPOINT", "localhost:4318")
	shutdownTelemetry, err := telemetry.InitTelemetry(ctx, "webhook-relay", collectorEndpoint)
	if err != nil {
		slog.Error("failed to initialize telemetry", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry()

	// Create handler
	webhookHandler := handler.NewWebhookHandler()

	// Setup routes with OTel instrumentation
	mux := http.NewServeMux()
	mux.Handle("/webhook", otelhttp.NewHandler(http.HandlerFunc(webhookHandler.HandleWebhook), "/webhook"))
	mux.Handle("/health", otelhttp.NewHandler(http.HandlerFunc(healthHandler), "/health"))
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Info("shutting down server")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	slog.Info("starting server", "addr", ":8080")
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"healthy"}`))
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
