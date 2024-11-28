FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.22 as builder
WORKDIR /go/src/github.com/openshift/descheduler
COPY . .
RUN make build --warn-undefined-variables

FROM registry.redhat.io/rhel9-4-els/rhel-minimal:9.4
COPY --from=builder /go/src/github.com/openshift/descheduler/descheduler /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/descheduler/LICENSE /licenses/.

LABEL io.k8s.display-name="Descheduler based on RHEL 9" \
      io.k8s.description="This is a component of OpenShift for the Descheduler based on RHEL 9" \
      com.redhat.component="descheduler-container" \
      name="descheduler-rhel-9" \
      version="${CI_CONTAINER_VERSION}" \
      summary="descheduler" \
      io.openshift.expose-services="" \
      io.openshift.tags="openshift,descheduler" \
      description="descheduler-container" \
      maintainer="AOS workloads team, <aos-workloads@redhat.com>"


USER nobody
