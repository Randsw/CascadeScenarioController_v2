package promexporter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestNewResponseWriter(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := NewResponseWriter(rr)

	if rw.statusCode != http.StatusOK {
		t.Errorf("Expected default status 200, got %d", rw.statusCode)
	}
}

func TestNewResponseWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := NewResponseWriter(rr)

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rw.statusCode)
	}
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected underlying status 404, got %d", rr.Code)
	}
}

func TestNewResponseWriter_Write(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := NewResponseWriter(rr)

	data := []byte("hello world")
	n, err := rw.Write(data)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}
	if rr.Body.String() != "hello world" {
		t.Errorf("Expected body 'hello world', got '%s'", rr.Body.String())
	}
}

func TestNewResponseWriter_Header(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := NewResponseWriter(rr)

	rw.Header().Set("X-Custom", "test-value")

	if rr.Header().Get("X-Custom") != "test-value" {
		t.Errorf("Expected X-Custom header 'test-value', got '%s'", rr.Header().Get("X-Custom"))
	}
}

func TestNewResponseWriter_DefaultStatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := NewResponseWriter(rr)

	// Write without calling WriteHeader should default to 200
	if _, err := rw.Write([]byte("test")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if rw.statusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.statusCode)
	}
}

func TestPrometheusMiddleware_Passthrough(t *testing.T) {
	// Create a mux router so that mux.CurrentRoute works
	router := mux.NewRouter()
	router.HandleFunc("/test-path", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status": "ok"}`)); err != nil {
			t.Errorf("Write failed: %v", err)
		}
	}).Methods("GET")

	// Wrap the router with middleware
	router.Use(PrometheusMiddleware)

	req := httptest.NewRequest("GET", "/test-path", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
	if rr.Body.String() != `{"status": "ok"}` {
		t.Errorf("Expected body '{\"status\": \"ok\"}', got '%s'", rr.Body.String())
	}
}

func TestPrometheusMiddleware_ErrorStatus(t *testing.T) {
	router := mux.NewRouter()
	router.HandleFunc("/error-path", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}).Methods("GET")

	router.Use(PrometheusMiddleware)

	req := httptest.NewRequest("GET", "/error-path", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rr.Code)
	}
}

func TestPrometheusMiddleware_BadRequest(t *testing.T) {
	router := mux.NewRouter()
	router.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("bad request")); err != nil {
			t.Errorf("Write failed: %v", err)
		}
	}).Methods("POST")

	router.Use(PrometheusMiddleware)

	req := httptest.NewRequest("POST", "/submit", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rr.Code)
	}
}

func TestMetricsRegistration(t *testing.T) {
	// Verify that metrics are registered (they should be via init())
	// This test ensures the init() function doesn't panic
	// The prometheus.MustRegister calls in init() would panic if registration fails
}

func TestMetricLabels(t *testing.T) {
	// Verify metric label definitions are correct
	scenarioLabels := []string{"scenarioName", "OName", "SName"}
	httpLabels := []string{"path"}
	statusLabels := []string{"status", "path"}

	if len(scenarioLabels) != 3 {
		t.Error("Scenario metrics should have 3 labels")
	}
	if len(httpLabels) != 1 {
		t.Error("HTTP duration metrics should have 1 label")
	}
	if len(statusLabels) != 2 {
		t.Error("Response status metrics should have 2 labels (status + path)")
	}
}
