package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_PreflightOptions(t *testing.T) {
	r := newTestRouter(seedFake())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, APIBasePath+"/flows", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allow-origin = %q, want http://localhost:3000", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("allow-methods empty")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Errorf("allow-headers empty")
	}
}

func TestCORS_GetEchoesArbitraryOrigin(t *testing.T) {
	r := newTestRouter(seedFake())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, APIBasePath+"/flows", nil)
	req.Header.Set("Origin", "http://some-other-host:9999")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://some-other-host:9999" {
		t.Errorf("GET should echo Origin, got %q", got)
	}
}

func TestCORS_NoOriginHeaderNoCORSHeaders(t *testing.T) {
	r := newTestRouter(seedFake())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, APIBasePath+"/flows", nil)
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no Origin request should not set allow-origin, got %q", got)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
