FROM isaaguilar/terraform-arm64:${TF_IMAGE} as terraform

FROM docker.io/library/debian@sha256:e3bb8517d8dd28c789f3e8284d42bd8019c05b17d851a63df09fd9230673306f as k8s
RUN apt update -y && apt install curl -y
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
RUN curl -LO "https://dl.k8s.io/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl.sha256"
RUN ls -lah kubectl
RUN ls -lah kubectl.sha256
RUN echo "$(cat kubectl.sha256)  kubectl" | sha256sum --check
RUN install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

FROM docker.io/library/alpine@sha256:c74f1b1166784193ea6c8f9440263b9be6cae07dfe35e32a5df7a31358ac2060
RUN apk add bash jq
COPY --from=terraform /usr/local/bin/terraform /usr/local/bin/terraform
COPY --from=k8s /usr/local/bin/kubectl /usr/local/bin/kubectl
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
