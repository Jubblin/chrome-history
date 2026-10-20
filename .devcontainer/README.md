# Dev environment (DevPod on local k3s)

Runs the devcontainer as a pod in the local k3s cluster. No Docker needed.

## One-time setup
- Install the DevPod CLI to `~/.local/bin` (release binary from github.com/loft-sh/devpod, no sudo).
- Working kubeconfig at `~/.kube/config` (`KUBECONFIG=$HOME/.kube/config kubectl get nodes`).

## Usage
- `make dev-up`    – configure the `kubernetes` provider (namespace `history-timeline-dev`, 2Gi PVC, CPU/memory requests+limits) and start the workspace.
- `make dev-shell` – shell in the pod (Go available).
- `make dev-down`  – delete the workspace and the `history-timeline-dev` namespace.

Image: `mcr.microsoft.com/devcontainers/go:1.27-bookworm`; port 8080 is forwarded.
Note: `devpod ssh` may print a harmless "Error port forwarding 8080" when a session ends.
Do not put real browser history/profile data in the repo; it is copied into the pod.
