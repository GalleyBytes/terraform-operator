ARG TF_IMAGE
FROM busybox as kubectl
RUN wget https://storage.googleapis.com/kubernetes-release/release/`wget -qO- https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl &&\
    chmod +x kubectl

FROM hashicorp/terraform:${TF_IMAGE}
RUN apk add bash gettext jq
COPY --from=kubectl /kubectl /usr/local/bin/kubectl
COPY run.sh /run.sh
COPY backend.tf /backend.tf
ENTRYPOINT ["/bin/bash"]
CMD ["run.sh"]
