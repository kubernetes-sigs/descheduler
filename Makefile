# Copyright 2017 The Kubernetes Authors.
# #
# # Licensed under the Apache License, Version 2.0 (the "License");
# # you may not use this file except in compliance with the License.
# # You may obtain a copy of the License at
# #
# #     http://www.apache.org/licenses/LICENSE-2.0
# #
# # Unless required by applicable law or agreed to in writing, software
# # distributed under the License is distributed on an "AS IS" BASIS,
# # WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# # See the License for the specific language governing permissions and
# # limitations under the License.

.PHONY: test

# VERSION is based on a date stamp plus the last commit
VERSION?=v$(shell date +%Y%m%d)-$(shell git describe --tags --match "v*")
BUILD=$(shell date +%FT%T%z)
LDFLAG_LOCATION=sigs.k8s.io/descheduler/cmd/descheduler/app

LDFLAGS=-ldflags "-X ${LDFLAG_LOCATION}.version=${VERSION} -X ${LDFLAG_LOCATION}.buildDate=${BUILD}"

GOLANGCI_VERSION := v1.30.0
HAS_GOLANGCI := $(shell ls _output/bin/golangci-lint)

# REGISTRY is the container registry to push
# into. The default is to push to the staging
# registry, not production.
REGISTRY?=gcr.io/k8s-staging-descheduler

# IMAGE is the image name of descheduler
IMAGE:=descheduler:$(VERSION)

# IMAGE_GCLOUD is the image name of descheduler in the remote registry
IMAGE_GCLOUD:=$(REGISTRY)/descheduler:$(VERSION)

# TODO: upload binaries to GCS bucket
#
# In the future binaries can be uploaded to
# GCS bucket gs://k8s-staging-descheduler.

HAS_HELM := $(shell which helm)

all: build

build:
	CGO_ENABLED=0 go build ${LDFLAGS} -o _output/bin/descheduler sigs.k8s.io/descheduler/cmd/descheduler

dev-image: build
	docker build -f Dockerfile.dev -t $(IMAGE) .

image:
	docker build --build-arg VERSION="$(VERSION)" -t $(IMAGE) .

push-container-to-gcloud: image
	gcloud auth configure-docker
	docker tag $(IMAGE) $(IMAGE_GCLOUD)
	docker push $(IMAGE_GCLOUD)

push: push-container-to-gcloud

clean:
	rm -rf _output

verify: verify-gofmt verify-vendor lint lint-chart verify-spelling

verify-spelling:
	./hack/verify-spelling.sh

verify-gofmt:
	./hack/verify-gofmt.sh

verify-vendor:
	./hack/verify-vendor.sh

test-unit:
	./test/run-unit-tests.sh

test-e2e:
	./test/run-e2e-tests.sh

gen:
	./hack/update-generated-conversions.sh
	./hack/update-generated-deep-copies.sh
	./hack/update-generated-defaulters.sh

lint:
ifndef HAS_GOLANGCI
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b ./_output/bin ${GOLANGCI_VERSION}
endif
	./_output/bin/golangci-lint run

lint-chart:
ifndef HAS_HELM
	curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 && chmod 700 ./get_helm.sh && ./get_helm.sh
endif
	helm lint ./charts/descheduler
