FROM docker.io/ubuntu:latest
COPY script.sh /tfo_runner.sh

ENV TFO_RUNNER_SCRIPT=/tfo_runner.sh \
    USER_UID=1000 \
    USER_NAME=tfo-runner \
    HOME=/home/tfo-runner
COPY usersetup.sh /usersetup.sh
RUN  /usersetup.sh
COPY entrypoint /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}