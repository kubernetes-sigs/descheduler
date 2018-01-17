FROM rhel7:7-released

MAINTAINER Avesh Agarwal <avagarwa@redhat.com>

ENV container=docker

LABEL vendor="Red Hat, Inc." \
      com.redhat.component="atomic-openshift-descheduler-docker" \
      name="openshift3/descheduler" \
      version="0.4.0" \
      release="1" \
      architecture="x86_64" \
      Summary="Descheduler for Opensift and Kubernetes"

RUN yum -y update && \
    yum -y install atomic-openshift-descheduler && \
    yum clean all

CMD /usr/bin/descheduler
