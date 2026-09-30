# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: builder
#
# Compiles a fully static (CGO disabled) Linux binary. The builder always runs
# on the build platform ($BUILDPLATFORM) and cross-compiles natively for the
# requested target platform ($TARGETOS/$TARGETARCH), so multi-architecture
# builds do not require emulating the Go toolchain.
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d AS builder

# Target platform, injected automatically by BuildKit.
ARG TARGETOS
ARG TARGETARCH

# Build metadata injected at build time. When these are left empty the
# Makefile falls back to git metadata or a safe default, so a plain
# `make build` inside this stage keeps working.
ARG VERSION
ARG COMMIT
ARG DATE

WORKDIR /workspace

# Copy the module manifests first so the dependency layer is cached
# independently of source changes.
COPY go.mod go.sum ./

# Persist the module cache between builds via a BuildKit cache mount.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the remaining build context (see .dockerignore for exclusions).
COPY . .

# Build with persistent module and build caches. GOOS/GOARCH and the version
# metadata are exported as environment variables so the Makefile reads them
# instead of shelling out to git (which is not part of the build context).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    VERSION="${VERSION}" COMMIT="${COMMIT}" DATE="${DATE}" \
    make build

# ---------------------------------------------------------------------------
# Stage 2: runtime
#
# Minimal, non-root, static distroless image.
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3 AS runtime

# OCI image metadata. The build workflow supplies the real values; the
# defaults keep a local `docker build` useful.
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

LABEL org.opencontainers.image.title="cascade-scenario-controller" \
      org.opencontainers.image.description="Kubernetes-native sequential cascade scenario controller" \
      org.opencontainers.image.source="https://github.com/Randsw/CascadeScenarioController_v2" \
      org.opencontainers.image.url="https://github.com/Randsw/CascadeScenarioController_v2" \
      org.opencontainers.image.documentation="https://github.com/Randsw/CascadeScenarioController_v2/blob/main/README.md" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${DATE}"

# Port used by the health, readiness, metrics and run endpoints.
EXPOSE 8080

WORKDIR /

# CA certificates are required to reach the Kubernetes API server and any
# HTTPS webhook endpoint. distroless/static ships without them, so the bundle
# is copied from the builder image.
COPY --link --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --link --from=builder /workspace/cascadescenariocontroller_auto /cascadescenariocontroller_auto

# distroless/static:nonroot already runs as 65532:65532; the user is set
# explicitly to document and enforce the intent even if the base tag changes.
USER 65532:65532

ENTRYPOINT ["/cascadescenariocontroller_auto"]
