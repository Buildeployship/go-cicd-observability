package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWebhook_Success(t *testing.T) {
	h := NewWebhookHandler()

	body := strings.NewReader(`{"event": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.HandleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestHandleWebhook_MethodNotAllowed(t *testing.T) {
	h := NewWebhookHandler()

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()
	h.HandleWebhook(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr.Code)
	}
}
