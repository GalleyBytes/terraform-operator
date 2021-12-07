FROM alpine/k8s:1.20.7 as k8s

FROM hashicorp/terraform:${TF_IMAGE}
RUN apk add bash jq
COPY --from=k8s /usr/bin/kubectl /usr/local/bin/kubectl
COPY tf.sh /runner/tfo_runner.sh

ENV TFO_RUNNER_SCRIPT=/runner/tfo_runner.sh \
    USER_UID=2000 \
    USER_NAME=tfo-runner \
    HOME=/home/tfo-runner
COPY usersetup.sh /usersetup.sh
RUN  /usersetup.sh
COPY entrypoint /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}
