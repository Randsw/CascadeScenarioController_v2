# CascadeScenarioController v2

A Kubernetes-native cascade scenario controller that orchestrates **sequential execution** of containerized processing modules. The controller receives HTTP requests, creates Kubernetes Jobs for each module in a scenario pipeline, and tracks their execution status via Custom Resource Definitions (CRDs).

---

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Project Structure](#project-structure)
- [API Endpoints](#api-endpoints)
  - [`POST /` - Receive Module Status](#post----receive-module-status)
  - [`POST /run` - Start Scenario Run](#post-run---start-scenario-run)
  - [`GET /healthz` - Health Check](#get-healthz---health-check)
  - [`GET /ready` - Readiness Probe](#get-ready---readiness-probe)
  - [`GET /metrics` - Prometheus Metrics](#get-metrics---prometheus-metrics)
- [Configuration](#configuration)
  - [Scenario Configuration File (JSON)](#scenario-configuration-file-json)
  - [Environment Variables](#environment-variables)
- [Deployment Guide](#deployment-guide)
  - [Prerequisites](#prerequisites)
  - [Building](#building)
  - [Docker](#docker)
  - [Kubernetes Custom Resources](#kubernetes-custom-resources)
  - [Kubernetes Deployment](#kubernetes-deployment)
- [Metrics & Observability](#metrics--observability)
- [Graceful Shutdown](#graceful-shutdown)
- [Rate Limiting](#rate-limiting)
- [Logging](#logging)
- [Semantic Release](#semantic-release)

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                       CascadeScenarioController                   │
│                                                                   │
│  ┌──────────┐    ┌──────────┐    ┌────────────┐    ┌──────────┐ │
│  │  HTTP     │───▶│ Handlers │───▶│  Process   │───▶│ k8sclient│ │
│  │  Server   │    │ package  │    │  package   │    │ package  │ │
│  └──────────┘    └──────────┘    └────────────┘    └──────────┘ │
│       │               │                │                │         │
│       ▼               ▼                ▼                ▼         │
│  ┌──────────────────────────────────────────────────────────┐     │
│  │                    Prometheus Exporter                     │     │
│  └──────────────────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────────────────┘
         │                        │                      │
         ▼                        ▼                      ▼
   ┌──────────┐          ┌──────────────┐      ┌──────────────┐
   │ External │          │ Kubernetes   │      │ CRDs:        │
   │ Modules  │          │ Jobs (Batch) │      │ - CascadeRun │
   │ (POST /) │          │              │      │ - CascadeAuto│
   └──────────┘          └──────────────┘      │   Operator   │
                                               └──────────────┘
```

### Flow Description

1. **External caller** sends a `POST /run` request with processing parameters (TURL, IUID, OName, SName).
2. The **Handler** validates the payload, checks server shutdown state, and launches the scenario via `process.ImageProcessing()`.
3. **Process** iterates through the cascade scenario modules sequentially:
   - For each module, it creates a **Kubernetes Job** via the `k8sclient` package.
   - It monitors the Job status (Running → Succeeded / Failed).
   - On success, it deletes the job and moves to the next module.
   - On failure, it marks the scenario as failed and stops.
4. **Modules** report their individual results back via `POST /` with module status (run name, module name, result).
5. Results are forwarded through a **channel** to update the `CascadeRun` CRD status.
6. **Prometheus metrics** track request counts, durations, scenario execution times, and job-level metrics.
7. On **SIGTERM/SIGINT**, the controller performs graceful shutdown: stops accepting new requests, waits for in-flight processes to complete, then shuts down the HTTP server.

### Key Features

- **Sequential Module Execution** — each module waits for the previous one to succeed
- **S3 Stage Transfer** — intermediate results are passed between stages via S3 paths
- **CRD-Based State Tracking** — scenario execution status persisted in Kubernetes CRDs
- **Graceful Shutdown** — waits for running processes to finish before exiting
- **Prometheus Metrics** — comprehensive observability for HTTP requests and scenario runs
- **Rate Limiting** — configurable token-bucket rate limiter (default: 100 req/s, burst 200)
- **Retry with Exponential Backoff** — resilient K8s API calls with jitter

---

## Project Structure

```
├── main.go                              # Application entry point, DI setup, HTTP server
├── handlers/
│   ├── handlers.go                      # HTTP handlers, request validation, rate limiting
│   └── handlers_test.go                 # Handler unit tests
├── process/
│   ├── process.go                       # Scenario orchestration logic
│   ├── retry.go                         # Reusable retry helper with exponential backoff
│   ├── retry_test.go                    # Retry logic tests
│   └── retry_test.go                    # Process package tests
├── k8sclient/
│   ├── k8sclient.go                     # Kubernetes client operations (Jobs, CRDs)
│   └── k8sclient_test.go                # K8s client unit tests
├── cascadescenario/
│   ├── cascadeScenario.go               # Config parsing (JSON scenario definitions)
│   ├── cascadeScenario_test.go          # Scenario config tests
│   └── test/
│       ├── test_success.json            # Test config: 3 modules (success path)
│       └── test_fail_first.json         # Test config: fail on 2nd module
├── api/v1alpha1/
│   ├── cascadeautooperator_types.go     # CascadeAutoOperator CRD types
│   ├── cascaderun_types.go              # CascadeRun CRD types
│   └── groupversion_info.go             # API group version registration
├── prometheus-exporter/
│   ├── prometheus-exporter.go           # Prometheus metrics, middleware
│   └── prometheus-exporter_test.go      # Metrics tests
├── webhook/
│   ├── webhook.go                       # Generic webhook sender
│   └── webhook_test.go                  # Webhook tests
├── logger/
│   └── logger.go                        # Structured logging (zap)
├── Dockerfile                           # Multi-stage distroless Docker build
├── Makefile                             # Build automation
├── go.mod / go.sum                      # Go modules
└── .releaserc.yaml                      # Semantic release configuration
```

---

## API Endpoints

### `POST /` — Receive Module Status

Receives execution results from individual processing modules and forwards them to the scenario controller's channel for CRD status updates.

#### Request Body

```json
{
  "runname": "cascadeautooperator-ip-abc12",
  "modulename": "grayscale",
  "moduleresult": "success"
}
```

| Field        | Type   | Required | Description                              |
|-------------|--------|----------|------------------------------------------|
| `runname`   | string | ✅       | Name of the CascadeRun CR (generated)     |
| `modulename`| string | ✅       | Name of the module that completed         |
| `moduleresult`| string| ✅      | Result of the module execution           |

#### Responses

| Status Code | Description                                    |
|------------|------------------------------------------------|
| `200 OK`    | Module status received and forwarded           |
| `400 Bad Request` | Invalid JSON or missing required fields  |

#### Validation Rules

- `runname` is required (non-empty)
- `modulename` is required (non-empty)
- `moduleresult` is required (non-empty)

---

### `POST /run` — Start Scenario Run

Initiates a new cascade scenario execution. The controller validates the input and launches the scenario pipeline.

#### Request Body

```json
{
  "turlinfo": "s3://bucket/path/to/input.tgz",
  "iuid": "33e65374-ee8e-4da6-a2e7-71a9cc39d222",
  "oname": "organization-name",
  "sname": "source-name"
}
```

| Field     | Type   | Required | Description                              |
|-----------|--------|----------|------------------------------------------|
| `turlinfo`| string | ✅       | S3 path to the input data                |
| `iuid`    | string | ✅       | Unique UUID for the transfer/run         |
| `oname`   | string | ❌       | Organization name (tagged in metrics)    |
| `sname`   | string | ❌       | Source name (tagged in metrics)          |

#### Responses

| Status Code | Description                                    |
|------------|------------------------------------------------|
| `200 OK`    | Scenario run initiated (async processing)      |
| `400 Bad Request` | Invalid JSON or missing required fields  |
| `503 Service Unavailable` | Server is shutting down           |

#### Validation Rules

- `turlinfo` is required (non-empty)
- `iuid` is required (non-empty)
- If the server is in shutdown state, returns `503 Service Unavailable`

---

### `GET /healthz` — Health Check

Returns the application health status including build metadata.

#### Response

```json
{
  "app_name": "cascadescenariocontroller-auto",
  "status": "OK",
  "tag": "v1.2.3",
  "hash": "abcdef1234567890",
  "date": "2026-07-20.17:30:00"
}
```

The `tag`, `hash`, and `date` fields are injected at build time via LD flags in the [`Makefile`](Makefile:7-9).

#### Responses

| Status Code | Description          |
|------------|----------------------|
| `200 OK`    | Application is healthy |

---

### `GET /ready` — Readiness Probe

Simple readiness check for Kubernetes liveness/readiness probes.

#### Responses

| Status Code | Description            |
|------------|------------------------|
| `200 OK`    | Application is ready   |

---

### `GET /metrics` — Prometheus Metrics

Exposes Prometheus metrics in the standard text format. Served via the standard [`promhttp.Handler`](handlers/handlers.go:156-158).

#### Responses

| Status Code | Description                  |
|------------|------------------------------|
| `200 OK`    | Prometheus metrics output    |

---

## Configuration

### Scenario Configuration File (JSON)

The controller reads a JSON configuration file that defines the cascade scenario modules. Default path: `/tmp/configuration`. Can be overridden via the `CONFIG_PATH` environment variable.

#### Example (`cascadescenario/test/test_success.json`):

```json
{
  "cascademodules": [
    {
      "modulename": "grayscale",
      "configuration": {
        "foo": "bar",
        "spamm": "eggs"
      },
      "backoffLimit": 0,
      "template": {
        "spec": {
          "containers": [
            {
              "name": "grayscale",
              "image": "ghcr.io/randsw/grayscale:0.1.1",
              "imagePullPolicy": "IfNotPresent"
            }
          ],
          "restartPolicy": "OnFailure"
        }
      }
    }
  ]
}
```

| Field                     | Type              | Description                                      |
|---------------------------|-------------------|--------------------------------------------------|
| `cascademodules`          | array             | Ordered list of modules to execute sequentially   |
| ├ `modulename`            | string            | Module name (appended with UUID for uniqueness)   |
| ├ `configuration`         | map[string]string | Key-value pairs injected as pod environment vars  |
| ├ `activeDeadlineSeconds` | int64 (optional)  | Job active deadline in seconds                    |
| ├ `backoffLimit`          | int32 (optional)  | Number of retries before marking job failed       |
| ├ `ttlSecondsAfterFinished`| int32 (optional) | Seconds after which to auto-delete finished job   |
| └ `template`              | PodTemplateSpec   | Standard Kubernetes PodTemplateSpec for the job   |

### Environment Variables

| Variable           | Default                    | Description                                        |
|--------------------|----------------------------|----------------------------------------------------|
| `CONFIG_PATH`      | `/tmp/configuration`       | Path to scenario configuration JSON file           |
| `POD_NAMESPACE`    | `cascade-operator`         | Kubernetes namespace for Jobs and CRDs             |
| `SCENARIO_NAME`    | `cascadeautooperator-ip`   | Name of the CascadeAutoOperator CRD resource       |
| `SID`              | `UnderTest`                | Scenario identifier                                 |
| `OUT_MINIO_ADDRESS`| `http://example.com/test-out/` | Base URL for output S3/Minio storage           |
| `KUBECONFIG`       | (in-cluster or `~/.kube/config`) | Path to kubeconfig file (for out-of-cluster) |

---

## Deployment Guide

### Prerequisites

- **Go 1.26+** for building
- **Kubernetes cluster** (tested with v1.30+)
- **CRDs installed** (see below)
- **Docker** (for containerized deployment)
- **Access to container registry** (e.g., GitHub Container Registry)

### Building

```bash
# Build the binary
make build

# The output binary: cascadescenariocontroller_auto
```

Build flags include:
- `CGO_ENABLED=0` — static binary for Alpine/distroless
- `GOOS=linux GOARCH=amd64` — Linux AMD64 target
- LD flags inject `tag`, `hash`, `date` into the binary at [`handlers/handlers.go:21-23`](handlers/handlers.go:21-23)

### Docker

```bash
# Build Docker image
docker build -t ghcr.io/randsw/cascadescenariocontroller:latest .

# Push to registry
docker push ghcr.io/randsw/cascadescenariocontroller:latest
```

The [`Dockerfile`](Dockerfile) uses a multi-stage build:
1. **Builder stage**: `golang:1.26` — compiles the binary via `make build`
2. **Runtime stage**: `gcr.io/distroless/static:nonroot` — minimal, secure runtime

### Kubernetes Custom Resources

The controller requires two CRDs to be installed in the cluster:

#### `CascadeAutoOperator`

- **API Group**: `cascade.cascade.net/v1alpha1`
- **Resource**: `cascadeautooperators`
- **Purpose**: Tracks overall scenario execution state (active/succeeded/failed counts)
- **Status Fields**: `active`, `succeeded`, `failed`, `result`

#### `CascadeRun`

- **API Group**: `cascade.cascade.net/v1alpha1`
- **Resource**: `cascaderuns`
- **Purpose**: Tracks individual scenario run details (modules, results, final output)
- **Spec Fields**: `ob`, `src`, `pid`, `scenarioname`, `modules`
- **Status Fields**: `result` (module-level results), `info` (final output address or "Failed")

### Kubernetes Deployment

#### 1. Install CRDs

Apply the CRD manifests to your cluster:

```bash
kubectl apply -f config/crd/
```

#### 2. Create Namespace

```bash
kubectl create namespace cascade-operator
```

#### 3. Deploy the Controller

Example deployment manifest:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cascade-scenario-controller
  namespace: cascade-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cascade-scenario-controller
  template:
    metadata:
      labels:
        app: cascade-scenario-controller
    spec:
      serviceAccountName: cascade-controller
      containers:
      - name: controller
        image: ghcr.io/randsw/cascadescenariocontroller:latest
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: "/etc/cascade/config.json"
        - name: POD_NAMESPACE
          value: "cascade-operator"
        - name: OUT_MINIO_ADDRESS
          value: "http://minio-service:9000/output/"
        - name: SCENARIO_NAME
          value: "cascadeautooperator-ip"
        - name: SID
          value: "production"
        volumeMounts:
        - name: config
          mountPath: /etc/cascade
      volumes:
      - name: config
        configMap:
          name: cascade-scenario-config
---
apiVersion: v1
kind: Service
metadata:
  name: cascade-scenario-controller
  namespace: cascade-operator
spec:
  selector:
    app: cascade-scenario-controller
  ports:
  - port: 8080
    targetPort: 8080
  type: ClusterIP
```

#### 4. Create ConfigMap with Scenario Configuration

```bash
kubectl create configmap cascade-scenario-config \
  --namespace cascade-operator \
  --from-file=config.json=./scenario-config.json
```

#### 5. Verify Deployment

```bash
# Check pod status
kubectl get pods -n cascade-operator

# Check endpoints
kubectl get svc -n cascade-operator

# Test health endpoint
kubectl port-forward -n cascade-operator svc/cascade-scenario-controller 8080:8080 &
curl http://localhost:8080/healthz
```

---

## Metrics & Observability

The controller exposes comprehensive Prometheus metrics via the `GET /metrics` endpoint.

### HTTP Metrics

| Metric                          | Type      | Labels                     | Description                       |
|--------------------------------|-----------|----------------------------|-----------------------------------|
| `http_requests_total`          | Counter   | `path`                     | Total HTTP requests by path       |
| `response_status`              | Counter   | `status`, `path`           | HTTP response status codes        |
| `http_response_time_seconds`   | Histogram | `path`                     | HTTP request duration             |

### Scenario Metrics

| Metric                          | Type      | Labels                                  | Description                         |
|--------------------------------|-----------|-----------------------------------------|-------------------------------------|
| `success_scenario_total`       | Counter   | `scenarioName`, `OName`, `SName`        | Successful scenario executions      |
| `failed_scenario_total`        | Counter   | `scenarioName`, `OName`, `SName`        | Failed scenario executions          |
| `scenario_current_runs`        | Gauge     | `scenarioName`, `OName`, `SName`        | Currently running scenarios         |
| `scenarion_execution_time_seconds`| Histogram| `scenarioName`, `OName`, `SName`      | Full scenario execution duration    |
| `job_execution_time_seconds`   | Histogram | `scenarioName`, `moduleName`, `OName`, `SName`, `status` | Per-job execution duration |

### Prometheus Middleware

The [`PrometheusMiddleware`](prometheus-exporter/prometheus-exporter.go:83-98) wraps all HTTP routes to automatically capture:
- Request count per path
- Response status distribution per path
- Request duration histograms

### Grafana Dashboard

You can create a Grafana dashboard using the metric names above. Key panels:
- Request rate and latency (per endpoint)
- Scenario success/failure ratio
- Active scenario runs
- Job execution time distribution

---

## Graceful Shutdown

The controller implements graceful shutdown on `SIGTERM` / `SIGINT` signals (see [`main.go:71`](main.go:71)):

1. **Signal received** — cancel the root context
2. **Reject new requests** — [`SetShutDown(true)`](handlers/handlers.go:44-49) causes `POST /run` to return `503`
3. **Wait for in-flight processes** — polls [`ProcessManager.Load()`](process/process.go:45-47) until all goroutines complete
4. **Delete CRD finalizer** — removes `shutdown.cascade.cascade.net/finalizer` from the CRD
5. **Shutdown HTTP server** — [`srv.Shutdown()`](main.go:157) with 10-second timeout
6. **Exit** — clean process termination

Additionally, a background goroutine ([`watchDeletionTimestamp`](main.go:166-208)) monitors CRD deletion timestamps and triggers the same shutdown sequence if the CRD is deleted externally.

---

## Rate Limiting

The controller includes a token-bucket rate limiter ([`golang.org/x/time/rate`](handlers/handlers.go:60-68)):

- **Default**: 100 requests per second, burst of 200
- **Behavior**: Returns `429 Too Many Requests` when limit exceeded
- **Scope**: Applied globally via middleware on all routes

Configured in [`NewServer()`](handlers/handlers.go:37-42):

```go
rateLimiter: rate.NewLimiter(rate.Limit(100), 200)
```

---

## Logging

The controller uses [`go.uber.org/zap`](logger/logger.go) for structured logging:

- **Production configuration** with RFC3339 timestamps
- **Log levels**: `Info`, `Warn`, `Debug`, `Error`, `Fatal`
- **Structured fields**: all log entries include relevant context (namespace, job name, UUID, etc.)

Example log format:
```json
{"level":"info","ts":"2026-07-20T17:30:00+03:00","func":"main.main","msg":"Start serving http request...","address":":8080"}
```

---

## Semantic Release

The project uses [`semantic-release`](.releaserc.yaml) with conventional commits for automated versioning and changelog generation:

| Commit Type  | Release Impact |
|-------------|---------------|
| `fix`        | Patch release  |
| `feat`       | Minor release  |
| `breaking`   | Major release  |
| `docs`, `chore`, `test`, `refactor`, `style` | No release |

The changelog is maintained in [`CHANGELOG.md`](CHANGELOG.md) and updated automatically on each release.

---

## License

Apache License 2.0 — See [`api/v1alpha1/cascadeautooperator_types.go`](api/v1alpha1/cascadeautooperator_types.go) for details.
