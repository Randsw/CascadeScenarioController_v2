package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/randsw/cascadescenariocontroller/cascadescenario"
	k8sClient "github.com/randsw/cascadescenariocontroller/k8sclient"
	"github.com/randsw/cascadescenariocontroller/logger"
	"github.com/randsw/cascadescenariocontroller/process"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	tag        string
	hash       string
	date       string
	IsShutDown = false
)

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

type RunHandler struct {
	Config RunConfig
}

type StartPayload struct {
	TURLInfo string `json:"turlinfo"`
	IUid     string `json:"iuid"`
	OName    string `json:"oname"`
	SName    string `json:"sname"`
}

var HTTPRequest map[string][]map[string]string

type moduleStatus struct {
	RunName      string `json:"runname"`
	ModuleName   string `json:"modulename"`
	ModuleResult string `json:"moduleresult"`
}

func GetHealth(w http.ResponseWriter, r *http.Request) {
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

func GetReadiness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

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

	logger.Info("Received module result", zap.String("ScenarioName", status.RunName), zap.String("ModuleName", status.ModuleName), zap.String("Module result", status.ModuleResult))
	resp := map[string]string{status.ModuleName: status.ModuleResult, "runname": status.RunName}
	logger.Info("Send data to channel", zap.String("ScenarioName", status.RunName), zap.String("ModuleName", status.ModuleName), zap.String("Module result", status.ModuleResult))
	h.Ch <- resp
	w.WriteHeader(http.StatusOK)
}

func (run *RunHandler) StartRun(w http.ResponseWriter, r *http.Request) {

	var Payload StartPayload
	if IsShutDown {
		logger.Info("Server is shutting down. Ignoring new request", zap.String("Namespace", run.Config.JobNamespace), zap.String("ScenarioName", run.Config.ScenarioName))
		http.Error(w, "Server is shutting down. Ignoring new request", http.StatusServiceUnavailable)
		return
	}
	err := json.NewDecoder(r.Body).Decode(&Payload)
	if err != nil {
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
	go process.ImageProcessing(run.Config.CascadeScenarioConfig, run.Config.K8sClient, run.Config.K8sDynClient, processingConfig, run.Config.GlobalChannel)
}
