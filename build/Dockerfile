FROM centos:latest as terraform
COPY build/bin /usr/local/bin
RUN /usr/local/bin/terraform_setup 

# --- Build main image --- 
FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/terraform-operator \
    USER_UID=1001 \
    USER_NAME=terraform-operator \
    HOME=/home/terraform-operator

# install operator binary
COPY build/_output/bin/terraform-operator ${OPERATOR}

COPY build/bin /usr/local/bin
# multi-verssion support?
COPY --from=terraform /usr/local/bin/terraform* /usr/local/bin/ 
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
