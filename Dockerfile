FROM quay.io/prometheus/busybox-${TARGETOS}-${TARGETARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

ARG TARGETARCH
ARG TARGETOS
COPY .build/${TARGETOS}-${TARGETARCH}/graphite_exporter /bin/graphite_exporter

USER        nobody
EXPOSE      9108 9109 9109/udp
ENTRYPOINT  [ "/bin/graphite_exporter" ]
