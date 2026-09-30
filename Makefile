GO_VERSION := 1.26

# Target platform. These defaults apply to plain local builds; the Dockerfile
# overrides them with the BuildKit TARGETOS/TARGETARCH values during image
# builds.
GOOS   ?= linux
GOARCH ?= amd64

# Build metadata.
#
# Resolution order:
#   1. a value supplied through the environment, which includes Docker
#      build-args, since BuildKit exposes declared ARGs to RUN as environment
#      variables;
#   2. a value derived from git metadata, when a repository is available;
#   3. a static fallback.
#
# Step 2 is what makes a local `make build` report real version information.
# Inside the Docker build the .git directory is intentionally excluded from the
# build context, so the metadata is passed in as build-args instead and step 3
# keeps the build from failing when nothing is supplied.
VERSION ?=
COMMIT  ?=
DATE    ?=

ifeq ($(strip $(VERSION)),)
VERSION := $(shell git describe --tags --abbrev=0 --always 2>/dev/null || echo dev)
endif

ifeq ($(strip $(COMMIT)),)
COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
endif

ifeq ($(strip $(DATE)),)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
endif

LDFLAGS := -w -X github.com/randsw/cascadescenariocontroller/handlers.hash=$(COMMIT) \
			-X github.com/randsw/cascadescenariocontroller/handlers.tag=$(VERSION) \
			-X github.com/randsw/cascadescenariocontroller/handlers.date=$(DATE)

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -a -o cascadescenariocontroller_auto main.go
