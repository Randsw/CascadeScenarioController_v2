package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
	"github.com/randsw/cascadescenariocontroller/handlers"
	k8sClient "github.com/randsw/cascadescenariocontroller/k8sclient"
	"github.com/randsw/cascadescenariocontroller/logger"
	"github.com/randsw/cascadescenariocontroller/process"
	prom "github.com/randsw/cascadescenariocontroller/prometheus-exporter"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// App holds all application dependencies, replacing package-level globals
// with dependency injection for better testability and maintainability.
type App struct {
	Server         *handlers.Server
	ProcessManager *process.ProcessManager
	GlobalChannel  chan map[string]string
	K8sClient      *kubernetes.Clientset
	K8sDynClient   dynamic.Interface
	Config         *handlers.RunConfig
}

func main() {
	// Logger Initialization
	logger.InitLogger()
	defer logger.CloseLogger()

	// Get Config from file - configurable via CONFIG_PATH environment variable
	configFilename := "/tmp/configuration"
	if envvar := os.Getenv("CONFIG_PATH"); len(envvar) > 0 {
		configFilename = envvar
	}

	CascadeScenarioConfig := scenarioconfig.ReadConfigJSON(configFilename)

	// Get pod namespace
	jobNamespace := "cascade-operator"
	if envvar := os.Getenv("POD_NAMESPACE"); len(envvar) > 0 {
		jobNamespace = envvar
	}

	// Get scenario name
	scenarioName := "cascadeautooperator-ip"
	if envvar := os.Getenv("SCENARIO_NAME"); len(envvar) > 0 {
		scenarioName = envvar
	}

	// Get sID
	sID := "UnderTest"
	if envvar := os.Getenv("SID"); len(envvar) > 0 {
		sID = envvar
	}

	outMinioAddress := "http://example.com/test-out/"
	if envvar := os.Getenv("OUT_MINIO_ADDRESS"); len(envvar) > 0 {
		outMinioAddress = envvar
	}

	// Create context that is cancelled on SIGTERM/SIGINT for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Initialize application components via dependency injection
	app := &App{
		Server:         handlers.NewServer(),
		ProcessManager: process.NewProcessManager(),
		GlobalChannel:  make(chan map[string]string),
	}

	k8sAPIClientset := k8sClient.ConnectToK8s()
	k8sAPIClientDyn := k8sClient.ConnectTOK8sDinamic()

	app.K8sClient = k8sAPIClientset
	app.K8sDynClient = k8sAPIClientDyn

	// Watch deletion timestamp appear in CRD metadata
	go app.watchDeletionTimestamp(ctx, k8sAPIClientDyn, jobNamespace, scenarioName)

	config := &handlers.RunConfig{
		CascadeScenarioConfig: CascadeScenarioConfig,
		JobNamespace:          jobNamespace,
		ScenarioName:          scenarioName,
		SID:                   sID,
		OutMinioAddress:       outMinioAddress,
		K8sClient:             k8sAPIClientset,
		K8sDynClient:          k8sAPIClientDyn,
		GlobalChannel:         app.GlobalChannel,
	}
	app.Config = config

	// Set up HTTP handlers with dependency injection
	h := &handlers.ChHandler{Ch: app.GlobalChannel}
	r := &handlers.RunHandler{
		Config:         *config,
		IsShuttingDown: app.Server.IsShutDown,
	}

	mux := mux.NewRouter()
	mux.HandleFunc("/", h.GetStatusFromModules)
	mux.HandleFunc("/run", r.StartRun)
	mux.HandleFunc("/healthz", app.Server.GetHealth)
	mux.HandleFunc("/metrics", app.Server.Metrics)
	mux.HandleFunc("/ready", app.Server.GetReadiness)

	// Apply middleware: Prometheus metrics, then rate limiting
	mux.Use(prom.PrometheusMiddleware)
	mux.Use(app.Server.RateLimitMiddleware)

	// Configure HTTP server with timeouts
	servingAddress := ":8080"
	srv := &http.Server{
		Addr:         servingAddress,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      mux,
	}

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("Start serving http request...", zap.String("address", servingAddress))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Fail to start http server", zap.String("err", err.Error()))
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("Shutdown signal received, starting graceful shutdown...")

	// Mark server as shutting down to reject new requests
	app.Server.SetShutDown(true)

	// Wait for in-flight processes to complete
	app.waitForProcesses()

	// Delete finalizer if we were shutting down due to CRD deletion
	if err := k8sClient.DeleteFinalizerCRD(k8sAPIClientDyn, jobNamespace, scenarioName, "shutdown.cascade.cascade.net/finalizer"); err != nil {
		logger.Error("Fail to delete finalizer during shutdown", zap.Error(err))
	}

	// Graceful HTTP server shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}

	logger.Info("Server gracefully stopped")
}

// watchDeletionTimestamp monitors the CRD for deletion timestamp and
// triggers graceful shutdown when detected.
func (app *App) watchDeletionTimestamp(ctx context.Context, k8sAPIClientDyn dynamic.Interface, jobNamespace, scenarioName string) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Deletion timestamp watcher stopped due to context cancellation")
			return
		default:
		}

		var err error
		isShutDown, err := k8sClient.WatchDeletetionTimeStampCRD(k8sAPIClientDyn, jobNamespace, scenarioName)
		if err != nil {
			logger.Error("Fail to read Deletion Timestamp", zap.Error(err))
		}

		if isShutDown {
			app.Server.SetShutDown(true)
			currval := app.ProcessManager.Load()
			logger.Info("Waiting for processes to finish...", zap.Int64("Running Processes", currval))

			// Wait for all in-flight processes to complete
			for app.ProcessManager.Load() > 0 {
				current := app.ProcessManager.Load()
				if current != currval {
					logger.Info("Waiting for processes to finish...", zap.Int64("Running Processes", current))
					currval = current
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Delete finalizer to allow CRD removal
			err = k8sClient.DeleteFinalizerCRD(k8sAPIClientDyn, jobNamespace, scenarioName, "shutdown.cascade.cascade.net/finalizer")
			if err != nil {
				logger.Error("Fail to delete finalizer", zap.Error(err))
			}
			return
		}

		if !isShutDown {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// waitForProcesses blocks until all in-flight processes have completed.
func (app *App) waitForProcesses() {
	currval := app.ProcessManager.Load()
	logger.Info("Waiting for processes to finish...", zap.Int64("Running Processes", currval))

	for app.ProcessManager.Load() > 0 {
		current := app.ProcessManager.Load()
		if current != currval {
			logger.Info("Waiting for processes to finish...", zap.Int64("Running Processes", current))
			currval = current
		}
		time.Sleep(100 * time.Millisecond)
	}
	logger.Info("All processes completed")
}
