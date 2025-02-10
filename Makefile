GO_VERSION := 1.21

TAG := $(shell git describe --abbrev=0 --tags --always)
HASH := $(shell git rev-parse HEAD)
DATE := $(shell date +%Y-%m-%d.%H:%M:%S)

LDFLAGS := -w -X github.com/randsw/cascadescenariocontroller/handlers.hash=$(HASH) \
			-X github.com/randsw/cascadescenariocontroller/handlers.tag=$(TAG) \
			-X github.com/randsw/cascadescenariocontroller/handlers.date=$(DATE)

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -a -o cascadescenariocontroller_auto main.go
