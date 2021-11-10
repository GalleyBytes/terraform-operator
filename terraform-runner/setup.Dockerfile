FROM alpine/git:user
USER root
RUN apk add gettext jq bash
COPY backend.tf /backend.tf
COPY setup.sh /runner/tfo_runner.sh

ENV TFO_RUNNER_SCRIPT=/runner/tfo_runner.sh \
    USER_UID=2000 \
    USER_NAME=tfo-runner \
    HOME=/home/tfo-runner
COPY usersetup.sh /usersetup.sh
RUN  /usersetup.sh
COPY entrypoint /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}
