package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendWebHook_Success(t *testing.T) {
	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Verify content type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", contentType)
		}

		// Verify body
		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		if body["message"] != "test message" {
			t.Errorf("Expected message 'test message', got '%s'", body["message"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	statusCode, err := SendWebHook("test message", server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if statusCode != "200" {
		t.Errorf("Expected status '200', got '%s'", statusCode)
	}
}

func TestSendWebHook_ServerError(t *testing.T) {
	// Create a test server that returns 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	statusCode, err := SendWebHook("test message", server.URL)
	if err != nil {
		t.Fatalf("Expected no error (server error is not a client error), got: %v", err)
	}
	if statusCode != "500" {
		t.Errorf("Expected status '500', got '%s'", statusCode)
	}
}

func TestSendWebHook_NotFound(t *testing.T) {
	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	statusCode, err := SendWebHook("test message", server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if statusCode != "404" {
		t.Errorf("Expected status '404', got '%s'", statusCode)
	}
}

func TestSendWebHook_InvalidAddress(t *testing.T) {
	_, err := SendWebHook("test message", "http://invalid-host-that-does-not-exist.local:99999")
	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}
}

func TestSendWebHook_EmptyMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		if body["message"] != "" {
			t.Errorf("Expected empty message, got '%s'", body["message"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	statusCode, err := SendWebHook("", server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if statusCode != "200" {
		t.Errorf("Expected status '200', got '%s'", statusCode)
	}
}

func TestSendWebHook_LargeMessage(t *testing.T) {
	// Create a large message
	largeMsg := make([]byte, 10000)
	for i := range largeMsg {
		largeMsg[i] = 'A'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		if len(body["message"]) != 10000 {
			t.Errorf("Expected message length 10000, got %d", len(body["message"]))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	statusCode, err := SendWebHook(string(largeMsg), server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if statusCode != "200" {
		t.Errorf("Expected status '200', got '%s'", statusCode)
	}
}
