ARG PHP_VERSION

FROM ghcr.io/shopware/shopware-cli-base:${PHP_VERSION}

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/shopware-cli /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/shopware-cli"]
CMD ["--help"]
