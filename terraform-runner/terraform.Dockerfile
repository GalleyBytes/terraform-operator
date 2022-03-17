FROM docker.io/library/alpine@sha256:c74f1b1166784193ea6c8f9440263b9be6cae07dfe35e32a5df7a31358ac2060
RUN apk add bash curl
RUN wget "https://releases.hashicorp.com/terraform/${TF_IMAGE}/terraform_${TF_IMAGE}_linux_${ARM_V}.zip"
RUN unzip "terraform_${TF_IMAGE}_linux_${ARM_V}.zip"
RUN mv terraform /usr/local/bin/terraform