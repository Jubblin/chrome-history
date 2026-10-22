# Dev environment on local k3s via DevPod. See .devcontainer/README.md
export KUBECONFIG ?= $(HOME)/.kube/config
export PATH := $(HOME)/.local/bin:$(PATH)

DEVPOD_WS ?= history-timeline
DEV_NAMESPACE ?= history-timeline-dev

BIN ?= bin/history-timeline
export CGO_ENABLED ?= 0
LDFLAGS := -s -w

.PHONY: build test cross fixture run-fixture dev-provider dev-up dev-shell dev-down extension-test extension-sync extension-check ui-sync ui-check extension-lint extension-zip

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/history-timeline

test:
	go vet ./...
	go test -count=1 ./...

cross:
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/history-timeline-linux-amd64 ./cmd/history-timeline
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/history-timeline-darwin-arm64 ./cmd/history-timeline
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/history-timeline-darwin-amd64 ./cmd/history-timeline
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/history-timeline-windows-amd64.exe ./cmd/history-timeline

# Chrome extension tests: Node >= 22 on the host, no dependencies.
extension-test:
	cd extension && node --test "test/*.test.js"

# Copy internal/classify/rules.json (source of truth) into the extension.
extension-sync:
	node extension/scripts/sync-rules.mjs

# Fail if the extension's copy of the rules is out of date.
extension-check:
	node extension/scripts/sync-rules.mjs --check

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

# shared/ui-common.js is the source of truth for helpers used by both front ends.
ui-sync:
	node extension/scripts/sync-ui-common.mjs

ui-check:
	node extension/scripts/sync-ui-common.mjs --check

# Validate the extension manifest (permissions, CSP, icons, version format).
extension-lint:
	node extension/scripts/build-zip.mjs --lint

# Reproducible Web Store / release zip in dist/ (allowlist; no tests, scripts or fixtures).
# Pass TAG=vX.Y.Z to require the manifest version to match.
extension-zip: extension-check ui-check extension-lint
	node extension/scripts/build-zip.mjs --out dist $(if $(TAG),--tag "$$TAG",)
