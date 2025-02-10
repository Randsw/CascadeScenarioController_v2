package main

import (
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
)

type Payload struct {
	PId           int    `json:"pId"`
	OName         string `json:"oName"`
	SName         string `json:"sName"`
	ITId          int    `json:"itId"`
	ITUid         string `json:"iitUid"`
	IncomingCount int    `json:"incomingCount"`
	IDate         string `json:"idate"`
	UserId        int    `json:"userId"`
	Path          string `json:"path"`
	Crc32         string `json:"crc32"`
}

type PayloadOut struct {
	UsId      int    `json:"usId"`
	ITUid     string `json:"iitUid"` // This used as uid
	FCount    int    `json:"fCount"`
	IFCount   int    `json:"iFCount"`
	OPath     string `json:"oPath"`
	OTId      int    `json:"oTId"`
	IAOut     bool   `json:"iAOut"`
	OName     string `json:"oName"`
	IDate     string `json:"iDate"`
	OTUid     string `json:"oTUid"`
	Crc32     string `json:"crc32"`
	ICE       bool   `json:"iCE"`
	PId       int    `json:"pId"`
	ITId      int    `json:"iTId"`
	ODateTime string `json:"oDateTime"`
	IPath     string `json:"iPath"`
	SName     string `json:"sName"`
	IDateTime string `json:"iDateTime"`
}

var GlobalChannel chan map[string]string = make(chan map[string]string)

func main() {
	//Loger Initialization
	logger.InitLogger()
	defer logger.CloseLogger()

	//Get Config from file mounted in tmp folder
	configFilename := "/tmp/configuration"

	CascadeScenarioConfig := scenarioconfig.ReadConfigJSON(configFilename)
	//Get pod namespace
	jobNamespace := "cascade-operator"
	if envvar := os.Getenv("POD_NAMESPACE"); len(envvar) > 0 {
		jobNamespace = envvar
	}
	// //Get scenario name
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

	//Create channel for signal
	cancelChan := make(chan os.Signal, 1)
	// catch SIGETRM or SIGINTERRUPT
	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan bool, 1)
	go func() {
		sig := <-cancelChan
		logger.Info("Caught signal", zap.String("Signal", sig.String()))
		logger.Info("Wait for 1 second to finish processing")
		time.Sleep(1 * time.Second)
		logger.Info("Exiting.....")
		// shutdown other goroutines gracefully
		// close other resources
		done <- true
		os.Exit(0)
	}()
	k8sAPIClientset := k8sClient.ConnectToK8s()

	k8sAPIClientDyn := k8sClient.ConnectTOK8sDinamic()

	// watch deletetion timestamp appear in CRD metadate
	go func() {
		for {
			var err error
			handlers.IsShutDown, err = k8sClient.WatchDeletetionTimeStampCRD(k8sAPIClientDyn, jobNamespace, scenarioName)
			if err != nil {
				logger.Error("Fail to read Deletion Timestamp", zap.Error(err))
			}
			if !handlers.IsShutDown {
				time.Sleep(100 * time.Millisecond)
			} else {
				currval := process.CurrProcess
				logger.Info("Waiting for processes to finish...", zap.Int64("Running Processes", process.CurrProcess))
				//Delete finalizer
				for process.CurrProcess > 0 {
					if process.CurrProcess != currval {
						logger.Info("Waiting for processes to finish...", zap.Int64("Running Processes", process.CurrProcess))
						currval = process.CurrProcess
					}
					time.Sleep(100 * time.Millisecond)
				}
				err = k8sClient.DeleteFinalizerCRD(k8sAPIClientDyn, jobNamespace, scenarioName, "shutdown.cascade.cascade.net/finalizer")
				if err != nil {
					logger.Error("Fail to delete finalizer", zap.Error(err))
				}
				break
			}
		}
	}()
	config := &handlers.RunConfig{
		CascadeScenarioConfig: CascadeScenarioConfig,
		JobNamespace:          jobNamespace,
		ScenarioName:          scenarioName,
		SID:                   sID,
		OutMinioAddress:       outMinioAddress,
		K8sClient:             k8sAPIClientset,
		K8sDynClient:          k8sAPIClientDyn,
		GlobalChannel:         GlobalChannel,
	}

	h := &handlers.ChHandler{Ch: GlobalChannel}
	r := &handlers.RunHandler{Config: *config}
	mux := mux.NewRouter()
	mux.HandleFunc("/", h.GetStatusFromModules)
	mux.HandleFunc("/run", r.StartRun)
	mux.HandleFunc("/healthz", handlers.GetHealth)
	mux.HandleFunc("/metrics", handlers.Metrics)
	mux.HandleFunc("/ready", handlers.GetReadiness)
	mux.Use(prom.PrometheusMiddleware)
	//Start healthz http server
	servingAddress := ":8080"
	handlers.HTTPRequest = make(map[string][]map[string]string)
	srv := &http.Server{
		Addr: servingAddress,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      mux, // Pass our instance of gorilla/mux in.
	}
	logger.Info("Start serving http request...", zap.String("address", servingAddress))
	err := srv.ListenAndServe()
	if err != nil {
		logger.Error("Fail to start http server", zap.String("err", err.Error()))
	}
	<-done
	// shutdown other goroutines gracefully
	// close other resources
}
