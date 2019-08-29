FROM busybox as kubectl
RUN wget https://storage.googleapis.com/kubernetes-release/release/`wget -qO- https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl &&\
    chmod +x kubectl

FROM hashicorp/terraform:0.11.14
RUN apk add bash
COPY --from=kubectl /kubectl /usr/local/bin/kubectl
COPY run.sh /run.sh
ENTRYPOINT ["/bin/bash"]
CMD ["run.sh"]