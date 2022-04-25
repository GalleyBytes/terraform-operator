FROM isaaguilar/terraform-arm64:${TF_IMAGE} as terraform

FROM docker.io/library/debian@sha256:e3bb8517d8dd28c789f3e8284d42bd8019c05b17d851a63df09fd9230673306f as k8s
RUN apt update -y && apt install curl -y
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/arm64/kubectl"
RUN curl -LO "https://dl.k8s.io/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/arm64/kubectl.sha256"
RUN ls -lah kubectl
RUN ls -lah kubectl.sha256
RUN echo "$(cat kubectl.sha256)  kubectl" | sha256sum --check
RUN install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

FROM ubuntu:20.04 as irsa-tokengen
WORKDIR /workdir
RUN mkdir bin
RUN apt update && apt install wget -y
RUN wget https://github.com/isaaguilar/irsa-tokengen/releases/download/v1.0.0/irsa-tokengen-v1.0.0-linux-arm64.tgz && \
    tar xzf irsa-tokengen-v1.0.0-linux-arm64.tgz && mv irsa-tokengen bin/irsa-tokengen

FROM ubuntu:latest as bin
WORKDIR /workdir
RUN mkdir bin
COPY --from=terraform /usr/local/bin/terraform bin/terraform
COPY --from=k8s /usr/local/bin/kubectl bin/kubectl
COPY --from=irsa-tokengen /workdir/bin/irsa-tokengen bin/irsa-tokengen

FROM docker.io/library/alpine@sha256:c74f1b1166784193ea6c8f9440263b9be6cae07dfe35e32a5df7a31358ac2060
RUN apk add bash jq git
COPY --from=bin /workdir/bin /usr/local/bin
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
