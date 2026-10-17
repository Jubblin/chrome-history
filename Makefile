# Dev environment on local k3s via DevPod. See .devcontainer/README.md
export KUBECONFIG ?= $(HOME)/.kube/config
export PATH := $(HOME)/.local/bin:$(PATH)

DEVPOD_WS ?= chrome-history
DEV_NAMESPACE ?= chrome-history-dev

.PHONY: dev-provider dev-up dev-shell dev-down

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
