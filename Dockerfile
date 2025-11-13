FROM registry.access.redhat.com/ubi9/ubi-minimal:9.2-750.1697534106

RUN \
    microdnf install -y \
    util-linux \
    && \
    microdnf clean all

COPY \
    hyperfleet \
    /usr/local/bin/

EXPOSE 8000

ENTRYPOINT ["/usr/local/bin/hyperfleet", "serve"]

LABEL name="hyperfleet" \
      vendor="Red Hat" \
      version="0.0.1" \
      summary="hyperfleet API" \
      description="hyperfleet API"
