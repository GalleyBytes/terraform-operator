FROM scratch
COPY bin/gencert-amd64 /gencert
ENTRYPOINT ["/gencert"]
