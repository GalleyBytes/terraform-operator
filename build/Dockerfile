# Build the manager binary
FROM golang:1.20 as builder
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -v -o manager cmd/manager/main.go


FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV OPERATOR=/usr/local/bin/terraform-operator \
    USER_UID=1001 \
    USER_NAME=terraform-operator \
    HOME=/home/terraform-operator
# install operator binary
COPY --from=builder /workspace/manager ${OPERATOR}
COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}
