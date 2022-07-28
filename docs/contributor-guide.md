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
make kind-multi-node
kind load docker-image <image name>
kind get kubeconfig > /tmp/admin.conf
export KUBECONFIG=/tmp/admin.conf
make test-unit
make test-e2e
```

## Build Helm Package locally

If you made some changes in the chart, and just want to check if templating is ok, or if the chart is buildable, you can run this command to have a package built from the `./charts` directory.

```
make build-helm
```

## Lint Helm Chart locally

To check linting of your changes in the helm chart locally you can run:

```
make lint-helm
```

## Test helm changes locally with kind and ct

You will need kind and docker (or equivalent) installed. We can use ct public image to avoid installing ct and all its dependencies.


```
make kind-multi-node
make ct-helm
```


### Miscellaneous
See the [hack directory](https://github.com/kubernetes-sigs/descheduler/tree/master/hack) for additional tools and scripts used for developing the descheduler.
