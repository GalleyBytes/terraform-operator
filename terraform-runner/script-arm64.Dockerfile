FROM docker.io/library/ubuntu@sha256:64162ac111b666daf1305de1888eb67a3033f62000f5ff781fe529aff8f88b09
COPY script.sh /runner/tfo_runner.sh

ENV TFO_RUNNER_SCRIPT=/runner/tfo_runner.sh \
    USER_UID=2000 \
    USER_NAME=tfo-runner \
    HOME=/home/tfo-runner
COPY usersetup.sh script-toolset.sh /
RUN  /script-toolset.sh && /usersetup.sh
COPY entrypoint /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}