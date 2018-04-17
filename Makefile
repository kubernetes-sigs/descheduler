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

# VERSION is currently based on the last commit
VERSION=`git describe --tags`
COMMIT=`git rev-parse HEAD`
BUILD=`date +%FT%T%z`
REPO_ORG?=kubernetes-incubator
LDFLAG_LOCATION=github.com/${REPO_ORG}/descheduler/cmd/descheduler/app

LDFLAGS=-ldflags "-X ${LDFLAG_LOCATION}.version=${VERSION} -X ${LDFLAG_LOCATION}.buildDate=${BUILD} -X ${LDFLAG_LOCATION}.gitCommit=${COMMIT}"


# IMAGE is the image name of descheduler
# Should this be changed?
IMAGE:=descheduler:$(VERSION)

all: build

build:
	CGO_ENABLED=0 go build ${LDFLAGS} -o _output/bin/descheduler github.com/${REPO_ORG}/descheduler/cmd/descheduler

dev-image: build
	docker build -f Dockerfile.dev -t $(IMAGE) .

image:
	docker build -t $(IMAGE) .

clean:
	rm -rf _output

test-unit:
	./test/run-unit-tests.sh

test-e2e:
	./test/run-e2e-tests.sh

gen:
	./hack/update-codecgen.sh
	./hack/update-generated-conversions.sh
	./hack/update-generated-deep-copies.sh
	./hack/update-generated-defaulters.sh
