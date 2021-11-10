FROM golang:1.15 as go
ENV CGO_ENABLED=0
RUN go get -u github.com/isaaguilar/tfvar-consolidate
# RUN git clone https://github.com/isaaguilar/tfvar-consolidate.git &&\
#     cd tfvar-consolidate &&\
#     CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -v -o tfvar-consolidate main.go &&\
#     mv tfvar-consolidate /go/bin

FROM alpine/git:user
USER root
RUN apk add bash
COPY backend.tf /backend.tf
COPY export.sh /runner/tfo_runner.sh
COPY --from=go /go/bin/tfvar-consolidate /usr/local/bin/tfvar-consolidate
ENV TFO_RUNNER_SCRIPT=/runner/tfo_runner.sh \
    USER_UID=2000 \
    USER_NAME=tfo-runner \
    HOME=/home/tfo-runner
COPY usersetup.sh /usersetup.sh
RUN  /usersetup.sh
COPY entrypoint /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}
