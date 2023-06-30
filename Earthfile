VERSION 0.6
FROM alpine

ARG BASE_IMAGE=quay.io/kairos/core-opensuse-leap:v2.1.2
ARG IMAGE_REPOSITORY=quay.io/kairos

ARG LUET_VERSION=0.34.0
ARG GOLINT_VERSION=v1.52.2
ARG GOLANG_VERSION=1.19.10

ARG RKE2_VERSION=latest
ARG BASE_IMAGE_NAME=$(echo $BASE_IMAGE | grep -o [^/]*: | rev | cut -c2- | rev)
ARG BASE_IMAGE_TAG=$(echo $BASE_IMAGE | grep -o :.* | cut -c2-)
ARG RKE2_VERSION_TAG=$(echo $RKE2_VERSION | sed s/+/-/)

luet:
    FROM quay.io/luet/base:$LUET_VERSION
    SAVE ARTIFACT /usr/bin/luet /luet

build-cosign:
    FROM gcr.io/projectsigstore/cosign:v1.13.1
    SAVE ARTIFACT /ko-app/cosign cosign

go-deps:
    FROM golang:$GOLANG_VERSION
    WORKDIR /build
    COPY go.mod go.sum ./
    RUN go mod download
    RUN apt-get update
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

BUILD_GOLANG:
    COMMAND
    WORKDIR /build
    COPY . ./
    ARG BIN
    ARG SRC

    RUN go build -ldflags "-s -w" -o ${BIN} ./${SRC}
    SAVE ARTIFACT ${BIN} ${BIN} AS LOCAL build/${BIN}

VERSION:
    COMMAND
    FROM alpine
    RUN apk add git

    COPY . ./

    RUN echo $(git describe --exact-match --tags || echo "v0.0.0-$(git log --oneline -n 1 | cut -d" " -f1)") > VERSION

    SAVE ARTIFACT VERSION VERSION

build-provider:
    FROM +go-deps
    DO +BUILD_GOLANG --BIN=agent-provider-rke2 --SRC=main.go

build-provider-package:
    DO +VERSION
    ARG VERSION=$(cat VERSION)
    FROM scratch
    COPY +build-provider/agent-provider-rke2 /system/providers/agent-provider-rke2
    COPY scripts /opt/rke2/scripts
    SAVE IMAGE --push $IMAGE_REPOSITORY/provider-rke2:${VERSION}

lint:
    FROM golang:$GOLANG_VERSION
    RUN wget -O- -nv https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $GOLINT_VERSION
    WORKDIR /build
    COPY . .
    RUN golangci-lint run

docker:
    DO +VERSION
    ARG VERSION=$(cat VERSION)

    FROM $BASE_IMAGE

    IF [ "$RKE2_VERSION" = "latest" ]
    ELSE
        ENV INSTALL_RKE2_VERSION=${RKE2_VERSION}
    END

    COPY install_rke2.sh .
     COPY +luet/luet /usr/bin/luet

    ENV INSTALL_RKE2_METHOD="tar"
    ENV INSTALL_RKE2_SKIP_RELOAD="true"
    ENV INSTALL_RKE2_TAR_PREFIX="/usr"
    RUN ./install_rke2.sh && rm install_rke2.sh
    COPY +build-provider/agent-provider-rke2 /system/providers/agent-provider-rke2

    ENV OS_ID=${BASE_IMAGE_NAME}-rke2
    ENV OS_NAME=$OS_ID:${BASE_IMAGE_TAG}
    ENV OS_REPO=${IMAGE_REPOSITORY}
    ENV OS_VERSION=${RKE2_VERSION_TAG}_${VERSION}
    ENV OS_LABEL=${BASE_IMAGE_TAG}_${RKE2_VERSION_TAG}_${VERSION}
    RUN envsubst >>/etc/os-release </usr/lib/os-release.tmpl
    RUN echo "export PATH=/var/lib/rancher/rke2/bin:$PATH" >> /etc/profile
    RUN mkdir -p /opt/rke2/scripts/
    COPY scripts/* /opt/rke2/scripts/

    RUN mkdir -p /var/lib/rancher/rke2/agent/images
    RUN curl -L --output /var/lib/rancher/rke2/agent/images/images.tar.zst "https://github.com/rancher/rke2/releases/download/$RKE2_VERSION/rke2-images-core.linux-amd64.tar.zst"

    SAVE IMAGE --push $IMAGE_REPOSITORY/${BASE_IMAGE_NAME}-rke2:${RKE2_VERSION_TAG}
    SAVE IMAGE --push $IMAGE_REPOSITORY/${BASE_IMAGE_NAME}-rke2:${RKE2_VERSION_TAG}_${VERSION}

cosign:
    ARG --required ACTIONS_ID_TOKEN_REQUEST_TOKEN
    ARG --required ACTIONS_ID_TOKEN_REQUEST_URL

    ARG --required REGISTRY
    ARG --required REGISTRY_USER
    ARG --required REGISTRY_PASSWORD

    DO +VERSION
    ARG VERSION=$(cat VERSION)

    FROM docker

    ENV ACTIONS_ID_TOKEN_REQUEST_TOKEN=${ACTIONS_ID_TOKEN_REQUEST_TOKEN}
    ENV ACTIONS_ID_TOKEN_REQUEST_URL=${ACTIONS_ID_TOKEN_REQUEST_URL}

    ENV REGISTRY=${REGISTRY}
    ENV REGISTRY_USER=${REGISTRY_USER}
    ENV REGISTRY_PASSWORD=${REGISTRY_PASSWORD}

    ENV COSIGN_EXPERIMENTAL=1
    COPY +build-cosign/cosign /usr/local/bin/

    RUN echo $REGISTRY_PASSWORD | docker login -u $REGISTRY_USER --password-stdin $REGISTRY

    RUN cosign sign $IMAGE_REPOSITORY/${BASE_IMAGE_NAME}-rke2:${RKE2_VERSION_TAG}
    RUN cosign sign $IMAGE_REPOSITORY/${BASE_IMAGE_NAME}-rke2:${RKE2_VERSION_TAG}_${VERSION}

provider-package-all-platforms:
     BUILD --platform=linux/amd64 +build-provider-package
     BUILD --platform=linux/arm64 +build-provider-package
