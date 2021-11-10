#!/bin/bash -e
apt-get update
apt-get upgrade -y

apt-get install -y \
    software-properties-common \
    vim \
    wget \
    curl \
    sudo

wget https://packages.cloud.google.com/apt/doc/apt-key.gpg
sudo apt-key add apt-key.gpg
echo "deb https://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee -a /etc/apt/sources.list.d/kubernetes.list
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

apt-get update
apt-cache policy docker-ce
apt-get install -y \
    kubectl \
    docker-ce \
    git \
    zip \
    unzip \
    apt-transport-https \
    jq \
    python3-pip \
    mysql-client \
    postgresql-client \
    musl-dev \
    gcc \
    python-dev \
    python3 \
    python3-dev \
    libffi-dev \
    python-yaml \
    libssl-dev \
    gettext \
    dnsutils

pip install awscli cfn_flip ruamel.yaml
pip install urllib3[secure]

rm $0