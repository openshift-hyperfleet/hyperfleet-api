FROM registry.access.redhat.com/ubi9/ubi-minimal:9.2-750.1697534106

RUN \
    microdnf install -y \
    util-linux \
    && \
    microdnf clean all

COPY \
    hyperfleet-api \
    /usr/local/bin/

EXPOSE 8000

ENTRYPOINT ["/usr/local/bin/hyperfleet-api", "serve"]

LABEL name="hyperfleet-api" \
      vendor="Red Hat" \
      version="0.0.1" \
      summary="HyperFleet API" \
      description="HyperFleet API"
