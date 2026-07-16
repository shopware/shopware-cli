ARG PHP_VERSION

FROM ghcr.io/shopware/shopware-cli-base:${PHP_VERSION}

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/shopware-cli /usr/local/bin/

ENTRYPOINT ["/entrypoint", "/usr/local/bin/entrypoint.sh", "/usr/local/bin/shopware-cli"]
CMD ["--help"]
