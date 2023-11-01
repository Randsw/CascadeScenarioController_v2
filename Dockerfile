# Build the manager binary
FROM golang:1.21 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY process.go process.go
COPY Makefile Makefile
COPY logger/ logger/
COPY cascadescenario/ cascadescenario/
COPY webhook/ webhook/
COPY k8sclient/ k8sclient/
COPY .git/ ./.git/

# Build
RUN make build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/cascadescenariocontroller_auto .
USER 65532:65532

ENTRYPOINT ["/cascadescenariocontroller_auto"]