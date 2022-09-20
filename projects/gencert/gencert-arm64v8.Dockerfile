FROM scratch
COPY bin/gencert-arm64 /gencert
ENTRYPOINT ["/gencert"]
