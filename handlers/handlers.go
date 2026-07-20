package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/randsw/cascadescenariocontroller/cascadescenario"
	k8sClient "github.com/randsw/cascadescenariocontroller/k8sclient"
	"github.com/randsw/cascadescenariocontroller/logger"
	"github.com/randsw/cascadescenariocontroller/process"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	tag  string
	hash string
	date string
)

// Server holds all HTTP server state, replacing package-level globals.
// All fields are protected by mu for concurrent access.
type Server struct {
	mu          sync.RWMutex
	isShutDown  bool
	HTTPRequest map[string][]map[string]string
	rateLimiter *rate.Limiter
}

// NewServer creates a new Server with default settings.
// Default rate limit: 100 requests per second with burst of 200.
func NewServer() *Server {
	return &Server{
		HTTPRequest: make(map[string][]map[string]string),
		rateLimiter: rate.NewLimiter(rate.Limit(100), 200),
	}
}

// SetShutDown marks the server as shutting down or resets the flag.
func (s *Server) SetShutDown(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isShutDown = v
}

// IsShutDown returns whether the server is in shutdown state.
func (s *Server) IsShutDown() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isShutDown
}

// RateLimitMiddleware is an HTTP middleware that limits request rate
// using a token bucket algorithm.
func (s *Server) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.rateLimiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ChHandler struct {
	Ch chan map[string]string
}

type RunConfig struct {
	CascadeScenarioConfig []cascadescenario.CascadeScenarios
	JobNamespace          string
	ScenarioName          string
	SID                   string
	OutMinioAddress       string
	K8sClient             *kubernetes.Clientset
	K8sDynClient          dynamic.Interface
	GlobalChannel         chan map[string]string
}

// RunHandler handles /run HTTP requests.
// IsShuttingDown is a function that returns true when the server is shutting down,
// enabling dependency injection instead of a global variable.
type RunHandler struct {
	Config         RunConfig
	IsShuttingDown func() bool
}

// StartPayload represents the JSON payload for the /run endpoint.
type StartPayload struct {
	TURLInfo string `json:"turlinfo"`
	IUid     string `json:"iuid"`
	OName    string `json:"oname"`
	SName    string `json:"sname"`
}

// Validate checks that required fields are present and returns
// a descriptive error if validation fails.
func (p *StartPayload) Validate() error {
	if p.TURLInfo == "" {
		return fmt.Errorf("turlinfo is required")
	}
	if p.IUid == "" {
		return fmt.Errorf("iuid is required")
	}
	return nil
}

type moduleStatus struct {
	RunName      string `json:"runname"`
	ModuleName   string `json:"modulename"`
	ModuleResult string `json:"moduleresult"`
}

// Validate checks that required fields are present and returns
// a descriptive error if validation fails.
func (m *moduleStatus) Validate() error {
	if m.RunName == "" {
		return fmt.Errorf("runname is required")
	}
	if m.ModuleName == "" {
		return fmt.Errorf("modulename is required")
	}
	if m.ModuleResult == "" {
		return fmt.Errorf("moduleresult is required")
	}
	return nil
}

// GetHealth returns the health status of the application.
func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := map[string]string{
		"app_name": "cascadescenariocontroller-auto",
		"status":   "OK",
		"tag":      tag,
		"hash":     hash,
		"date":     date,
	}
	if err := enc.Encode(resp); err != nil {
		logger.Error("Error while encoding JSON response", zap.String("err", err.Error()))
	}
}

// GetReadiness returns 200 OK for readiness probes.
func (s *Server) GetReadiness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// Metrics exposes Prometheus metrics via the promhttp handler.
func (s *Server) Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

// GetStatusFromModules receives module execution results and forwards them
// to the processing channel. Includes input validation.
func (h *ChHandler) GetStatusFromModules(w http.ResponseWriter, r *http.Request) {
	var status moduleStatus
	logger.Info("Received post request")
	err := json.NewDecoder(r.Body).Decode(&status)
	if err != nil {
		switch err.(type) {
		case *json.SyntaxError:
			http.Error(w, "Invalid JSON syntax", http.StatusBadRequest)
		default:
			http.Error(w, "Failed to parse JSON", http.StatusInternalServerError)
		}
		return
	}

	// Input validation
	if err := status.Validate(); err != nil {
		logger.Error("Validation failed for module status", zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logger.Info("Received module result", zap.String("ScenarioName", status.RunName), zap.String("ModuleName", status.ModuleName), zap.String("Module result", status.ModuleResult))
	resp := map[string]string{status.ModuleName: status.ModuleResult, "runname": status.RunName}
	logger.Info("Send data to channel", zap.String("ScenarioName", status.RunName), zap.String("ModuleName", status.ModuleName), zap.String("Module result", status.ModuleResult))
	h.Ch <- resp
	w.WriteHeader(http.StatusOK)
}

// StartRun handles incoming cascade scenario run requests.
// It validates the payload, checks shutdown state via dependency injection,
// and launches the processing in a goroutine.
func (run *RunHandler) StartRun(w http.ResponseWriter, r *http.Request) {
	var Payload StartPayload

	if run.IsShuttingDown != nil && run.IsShuttingDown() {
		logger.Info("Server is shutting down. Ignoring new request", zap.String("Namespace", run.Config.JobNamespace), zap.String("ScenarioName", run.Config.ScenarioName))
		http.Error(w, "Server is shutting down. Ignoring new request", http.StatusServiceUnavailable)
		return
	}

	err := json.NewDecoder(r.Body).Decode(&Payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Input validation
	if err := Payload.Validate(); err != nil {
		logger.Error("Validation failed for start payload", zap.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logger.Info("Received request", zap.String("TURL", Payload.TURLInfo), zap.String("UUID", Payload.IUid), zap.String("OName", Payload.OName), zap.String("SName", Payload.SName))

	w.WriteHeader(http.StatusOK)

	processingConfig := k8sClient.K8sScenarioConfig{ScenarioNamespace: run.Config.JobNamespace, ScenarioName: run.Config.ScenarioName, OutMinioAddress: run.Config.OutMinioAddress}

	processingConfig.S3Path = Payload.TURLInfo
	processingConfig.TUUID = Payload.IUid
	processingConfig.Ob = Payload.OName
	processingConfig.SName = Payload.SName

	logger.Info("Start Cascade run")
	// Only launch processing if K8s clients are available (defensive check)
	if run.Config.K8sClient != nil && run.Config.K8sDynClient != nil {
		go process.ImageProcessing(run.Config.CascadeScenarioConfig, run.Config.K8sClient, run.Config.K8sDynClient, processingConfig, run.Config.GlobalChannel)
	} else {
		logger.Error("Cannot start cascade run: K8s clients are not initialized")
	}
}
