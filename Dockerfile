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
FROM golang:1.15.2

WORKDIR /go/src/sigs.k8s.io/descheduler
COPY . .
ARG VERSION
RUN VERSION=${VERSION} make

FROM scratch

MAINTAINER Avesh Agarwal <avesh.ncsu@gmail.com>

COPY --from=0 /go/src/sigs.k8s.io/descheduler/_output/bin/descheduler /bin/descheduler

CMD ["/bin/descheduler", "--help"]
