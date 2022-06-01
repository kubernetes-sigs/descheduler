# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
ARG GORUNNER_VERSION=v2.3.1-go1.18.2-bullseye.0

FROM golang:1.18.2

WORKDIR /go/src/sigs.k8s.io/descheduler
COPY . .
ARG ARCH
ARG VERSION
RUN VERSION=${VERSION} make build.$ARCH

FROM k8s.gcr.io/build-image/go-runner:${GORUNNER_VERSION} as build-base

FROM scratch

LABEL maintainer="Kubernetes SIG Scheduling <kubernetes-sig-scheduling@googlegroups.com>"

USER 1000

COPY --from=build-base /go-runner /
COPY --from=0 /go/src/sigs.k8s.io/descheduler/_output/bin/descheduler /bin/descheduler

ENTRYPOINT [ "/go-runner" ]
CMD ["/bin/descheduler", "--help"]
