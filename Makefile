# Dev environment on local k3s via DevPod. See .devcontainer/README.md
export KUBECONFIG ?= $(HOME)/.kube/config
export PATH := $(HOME)/.local/bin:$(PATH)

DEVPOD_WS ?= chrome-history
DEV_NAMESPACE ?= chrome-history-dev

BIN ?= bin/chrome-history
export CGO_ENABLED ?= 0
LDFLAGS := -s -w

.PHONY: build test cross fixture run-fixture dev-provider dev-up dev-shell dev-down extension-test

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/chrome-history

test:
	go vet ./...
	go test -count=1 ./...

cross:
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/chrome-history-linux-amd64 ./cmd/chrome-history
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/chrome-history-darwin-arm64 ./cmd/chrome-history
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/chrome-history-darwin-amd64 ./cmd/chrome-history
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/chrome-history-windows-amd64.exe ./cmd/chrome-history

# Chrome extension tests: Node >= 22 on the host, no dependencies.
extension-test:
	cd extension && node --test "test/*.test.js"

fixture:
	go run ./cmd/make-fixture -out fixture

run-fixture: build
	@[ -f fixture/Default/History ] || $(MAKE) fixture
	$(BIN) --profile fixture --no-open

dev-provider:
	@devpod provider list 2>/dev/null | grep -q kubernetes || devpod provider add kubernetes
	devpod provider set-options kubernetes \
		-o KUBERNETES_NAMESPACE=$(DEV_NAMESPACE) \
		-o KUBERNETES_CONFIG=$(KUBECONFIG) \
		-o KUBERNETES_CONTEXT=default \
		-o DISK_SIZE=2Gi \
		-o RESOURCES=requests.cpu=250m,requests.memory=512Mi,limits.cpu=2,limits.memory=2Gi \
		-o CREATE_NAMESPACE=true

dev-up: dev-provider
	devpod up . --id $(DEVPOD_WS) --provider kubernetes --ide none

dev-shell:
	devpod ssh $(DEVPOD_WS)

dev-down:
	-devpod delete $(DEVPOD_WS) --force
	-kubectl delete namespace $(DEV_NAMESPACE) --ignore-not-found
