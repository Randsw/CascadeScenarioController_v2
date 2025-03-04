package promexporter

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

var totalRequests = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Number of get requests.",
	},
	[]string{"path"},
)

var responseStatus = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "response_status",
		Help: "Status of HTTP response",
	},
	[]string{"status", "path"},
)

var TotalSucceedScenario = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "success_scenario_total",
		Help: "Number of successful scenario run",
	},
	[]string{"scenarioName", "OName", "SName"},
)

var TotalFailedScenario = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "failed_scenario_total",
		Help: "Number of failed scenario run",
	},
	[]string{"scenarioName", "OName", "SName"},
)

var CurrentRuns = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "scenario_current_runs",
		Help: "Number of current runs on scenario",
	},
	[]string{"scenarioName", "OName", "SName"},
)

var httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_response_time_seconds",
	Help:    "Duration of HTTP requests.",
	Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.0025},
}, []string{"path"})

var ScenarioDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "scenarion_execution_time_seconds",
	Help:    "Duration of full scenarion.",
	Buckets: []float64{1, 5, 20, 60, 100, 180},
}, []string{"scenarioName", "OName", "SName"})

var JobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "job_execution_time_seconds",
	Help:    "Duration of one job in scenarion.",
	Buckets: []float64{1, 5, 20, 60, 100, 180},
}, []string{"scenarioName", "moduleName", "OName", "SName", "status"})

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()

		timer := prometheus.NewTimer(httpDuration.WithLabelValues(path))
		rw := NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		statusCode := rw.statusCode

		responseStatus.WithLabelValues(strconv.Itoa(statusCode)).Inc()
		totalRequests.WithLabelValues(path).Inc()

		timer.ObserveDuration()
	})
}

func init() {
	prometheus.MustRegister(totalRequests)
	prometheus.MustRegister(responseStatus)
	prometheus.MustRegister(httpDuration)
	prometheus.MustRegister(ScenarioDuration)
	prometheus.MustRegister(JobDuration)
}
