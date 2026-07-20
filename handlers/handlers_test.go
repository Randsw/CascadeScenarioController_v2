package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
	"github.com/randsw/cascadescenariocontroller/logger"
)

func init() {
	logger.InitLogger()
}

func TestGetHealth(t *testing.T) {
	srv := NewServer()

	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(srv.GetHealth)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status 200, got %d", status)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var resp map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	if resp["app_name"] != "cascadescenariocontroller-auto" {
		t.Errorf("Expected app_name 'cascadescenariocontroller-auto', got '%s'", resp["app_name"])
	}
	if resp["status"] != "OK" {
		t.Errorf("Expected status 'OK', got '%s'", resp["status"])
	}
}

func TestGetReadiness(t *testing.T) {
	srv := NewServer()

	req, err := http.NewRequest("GET", "/ready", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(srv.GetReadiness)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status 200, got %d", status)
	}
}

func TestGetStatusFromModules_ValidJSON(t *testing.T) {
	ch := make(chan map[string]string, 1)
	h := &ChHandler{Ch: ch}

	payload := moduleStatus{
		RunName:      "test-run-123",
		ModuleName:   "grayscale",
		ModuleResult: "success",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(h.GetStatusFromModules)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status 200, got %d", status)
	}

	// Verify channel received the data
	select {
	case result := <-ch:
		if result["grayscale"] != "success" {
			t.Errorf("Expected module result 'success', got '%s'", result["grayscale"])
		}
		if result["runname"] != "test-run-123" {
			t.Errorf("Expected runname 'test-run-123', got '%s'", result["runname"])
		}
	default:
		t.Error("Expected data on channel, got nothing")
	}
}

func TestGetStatusFromModules_InvalidJSON(t *testing.T) {
	ch := make(chan map[string]string, 1)
	h := &ChHandler{Ch: ch}

	body := []byte(`{"runname": invalid json`)

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(h.GetStatusFromModules)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", status)
	}
}

func TestGetStatusFromModules_EmptyBody(t *testing.T) {
	ch := make(chan map[string]string, 1)
	h := &ChHandler{Ch: ch}

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer([]byte{}))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(h.GetStatusFromModules)
	handler.ServeHTTP(rr, req)

	// Empty body should result in an error (EOF or parse error)
	if status := rr.Code; status < 400 {
		t.Errorf("Expected error status for empty body, got %d", status)
	}
}

func TestGetStatusFromModules_MissingFields(t *testing.T) {
	ch := make(chan map[string]string, 1)
	h := &ChHandler{Ch: ch}

	// Missing runname and moduleresult
	payload := moduleStatus{
		ModuleName: "grayscale",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(h.GetStatusFromModules)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing fields, got %d", status)
	}
}

func TestStartRun_ShuttingDown(t *testing.T) {
	run := &RunHandler{
		Config: RunConfig{
			JobNamespace: "test-ns",
			ScenarioName: "test-scenario",
		},
		IsShuttingDown: func() bool { return true },
	}

	payload := StartPayload{
		TURLInfo: "s3://bucket/path",
		IUid:     "test-uuid",
		OName:    "test-o",
		SName:    "test-s",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(run.StartRun)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when shutting down, got %d", status)
	}
}

func TestStartRun_InvalidJSON(t *testing.T) {
	run := &RunHandler{
		Config: RunConfig{
			JobNamespace: "test-ns",
			ScenarioName: "test-scenario",
		},
		IsShuttingDown: func() bool { return false },
	}

	body := []byte(`{invalid}`)

	req, err := http.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(run.StartRun)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", status)
	}
}

func TestStartRun_ValidPayload(t *testing.T) {
	ch := make(chan map[string]string, 10)

	run := &RunHandler{
		Config: RunConfig{
			CascadeScenarioConfig: []scenarioconfig.CascadeScenarios{},
			JobNamespace:          "test-ns",
			ScenarioName:          "test-scenario",
			SID:                   "test-sid",
			OutMinioAddress:       "http://minio:9000/",
			GlobalChannel:         ch,
		},
		IsShuttingDown: func() bool { return false },
	}

	payload := StartPayload{
		TURLInfo: "s3://bucket/test.tgz",
		IUid:     "test-uuid-12345",
		OName:    "test-o",
		SName:    "test-s",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(run.StartRun)
	handler.ServeHTTP(rr, req)

	// Should return 200 immediately (processing happens in goroutine)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status 200, got %d", status)
	}
}

func TestStartRun_EmptyBody(t *testing.T) {
	run := &RunHandler{
		Config: RunConfig{
			JobNamespace: "test-ns",
			ScenarioName: "test-scenario",
		},
		IsShuttingDown: func() bool { return false },
	}

	req, err := http.NewRequest("POST", "/run", bytes.NewBuffer([]byte{}))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(run.StartRun)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty body, got %d", status)
	}
}

func TestStartRun_MissingRequiredFields(t *testing.T) {
	run := &RunHandler{
		Config: RunConfig{
			JobNamespace: "test-ns",
			ScenarioName: "test-scenario",
		},
		IsShuttingDown: func() bool { return false },
	}

	// Missing TURLInfo (required)
	payload := StartPayload{
		IUid:  "test-uuid",
		OName: "test-o",
		SName: "test-s",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(run.StartRun)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing required fields, got %d", status)
	}
}

func TestStartRun_MissingIUid(t *testing.T) {
	run := &RunHandler{
		Config: RunConfig{
			JobNamespace: "test-ns",
			ScenarioName: "test-scenario",
		},
		IsShuttingDown: func() bool { return false },
	}

	// Missing IUid (required)
	payload := StartPayload{
		TURLInfo: "s3://bucket/path",
		OName:    "test-o",
		SName:    "test-s",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(run.StartRun)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing iuid, got %d", status)
	}
}

func TestModuleStatus_JSONSerialization(t *testing.T) {
	status := moduleStatus{
		RunName:      "run-001",
		ModuleName:   "grayscale",
		ModuleResult: "success",
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal moduleStatus: %v", err)
	}

	var decoded moduleStatus
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal moduleStatus: %v", err)
	}

	if decoded.RunName != status.RunName {
		t.Errorf("Expected RunName '%s', got '%s'", status.RunName, decoded.RunName)
	}
	if decoded.ModuleName != status.ModuleName {
		t.Errorf("Expected ModuleName '%s', got '%s'", status.ModuleName, decoded.ModuleName)
	}
	if decoded.ModuleResult != status.ModuleResult {
		t.Errorf("Expected ModuleResult '%s', got '%s'", status.ModuleResult, decoded.ModuleResult)
	}
}

func TestStartPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload StartPayload
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid payload",
			payload: StartPayload{TURLInfo: "s3://bucket/path", IUid: "uuid-123"},
			wantErr: false,
		},
		{
			name:    "missing turlinfo",
			payload: StartPayload{IUid: "uuid-123"},
			wantErr: true,
			errMsg:  "turlinfo is required",
		},
		{
			name:    "missing iuid",
			payload: StartPayload{TURLInfo: "s3://bucket/path"},
			wantErr: true,
			errMsg:  "iuid is required",
		},
		{
			name:    "empty payload",
			payload: StartPayload{},
			wantErr: true,
			errMsg:  "turlinfo is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got '%s'", err.Error())
				}
			}
		})
	}
}

func TestModuleStatus_Validate(t *testing.T) {
	tests := []struct {
		name    string
		status  moduleStatus
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid status",
			status:  moduleStatus{RunName: "run-1", ModuleName: "mod-1", ModuleResult: "success"},
			wantErr: false,
		},
		{
			name:    "missing runname",
			status:  moduleStatus{ModuleName: "mod-1", ModuleResult: "success"},
			wantErr: true,
			errMsg:  "runname is required",
		},
		{
			name:    "missing modulename",
			status:  moduleStatus{RunName: "run-1", ModuleResult: "success"},
			wantErr: true,
			errMsg:  "modulename is required",
		},
		{
			name:    "missing moduleresult",
			status:  moduleStatus{RunName: "run-1", ModuleName: "mod-1"},
			wantErr: true,
			errMsg:  "moduleresult is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.status.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got '%s'", err.Error())
				}
			}
		})
	}
}

func TestServer_SetAndIsShutDown(t *testing.T) {
	srv := NewServer()

	if srv.IsShutDown() {
		t.Error("Expected IsShutDown to be false initially")
	}

	srv.SetShutDown(true)
	if !srv.IsShutDown() {
		t.Error("Expected IsShutDown to be true after SetShutDown(true)")
	}

	srv.SetShutDown(false)
	if srv.IsShutDown() {
		t.Error("Expected IsShutDown to be false after SetShutDown(false)")
	}
}

func TestServer_RateLimitMiddleware(t *testing.T) {
	srv := NewServer()

	// Create a simple handler that returns 200
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := srv.RateLimitMiddleware(nextHandler)

	// Send requests up to burst limit (200) - all should pass
	for i := 0; i < 200; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// The next request should be rate limited (bucket exhausted)
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 after burst limit, got %d", rr.Code)
	}
}