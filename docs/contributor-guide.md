# Contributor Guide

## Required Tools

- [Git](https://git-scm.com/downloads)
- [Go 1.16+](https://golang.org/dl/)
- [Docker](https://docs.docker.com/install/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl)
- [kind v0.10.0+](https://kind.sigs.k8s.io/)

## Build and Run

Build descheduler.
```sh
cd $GOPATH/src/sigs.k8s.io
git clone https://github.com/kubernetes-sigs/descheduler.git
cd descheduler
make
```

Run descheduler.
```sh
./_output/bin/descheduler --kubeconfig <path to kubeconfig> --policy-config-file <path-to-policy-file> --v 1
```

View all CLI options.
```
./_output/bin/descheduler --help
```

## Run Tests
```
GOOS=linux make dev-image
kind create cluster --config hack/kind_config.yaml
kind load docker-image <image name>
kind get kubeconfig > /tmp/admin.conf
export KUBECONFIG=/tmp/admin.conf
make test-unit
make test-e2e
```

## Run Helm Tests
Run the helm test for a particular descheduler release by setting below variables,
```
HELM_IMAGE_REPO="descheduler"
HELM_IMAGE_TAG="helm-test"
HELM_CHART_LOCATION="./charts/descheduler"
```
The helm tests runs as part of descheduler CI. But, to run it manually from the descheduler root,

```
make test-helm
```

### Miscellaneous
See the [hack directory](https://github.com/kubernetes-sigs/descheduler/tree/master/hack) for additional tools and scripts used for developing the descheduler.
