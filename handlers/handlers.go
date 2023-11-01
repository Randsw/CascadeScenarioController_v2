package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/randsw/cascadescenariocontroller/logger"
	"go.uber.org/zap"
)

var (
	tag  string
	hash string
	date string
)

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

func Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}
