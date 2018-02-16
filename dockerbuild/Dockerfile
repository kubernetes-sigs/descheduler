FROM golang:1.9

WORKDIR /golang
ENV GOPATH /golang
WORKDIR /golang/src/github.com

RUN    git clone https://github.com/kubernetes-incubator/descheduler.git kubernetes-incubator/descheduler \
    && make -C kubernetes-incubator/descheduler \
    && cp kubernetes-incubator/descheduler/_output/bin/descheduler /bin/descheduler \
    && rm -rf kubernetes-incubator/descheduler

CMD ["/bin/descheduler", "--help"]
