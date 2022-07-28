#!/bin/bash

docker run -it --rm --network host --workdir=/data --volume ~/.kube/config:/root/.kube/config:ro --volume $(pwd):/data quay.io/helmpack/chart-testing:v3.7.0 /bin/bash -c "git config --global --add safe.directory /data; ct install --config=.github/ci/ct.yaml --helm-extra-set-args=\"--set=kind=Deployment --set=podSecurityPolicy.create=false\""