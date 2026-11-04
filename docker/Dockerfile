FROM docker.io/library/golang:1.26-alpine3.23@sha256:34414731924cf2263160f0021f5cf6e1807382d85248f7cb419d4187b96d5e43 AS builder_golang

ARG appVer="appVer"
ARG commitId="commitId"

ENV TZ=Asia/Shanghai
ENV GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org GOTOOLCHAIN=local GOFLAGS=-mod=readonly

WORKDIR /server
RUN apk add --no-cache build-base tzdata
COPY server/go.mod server/go.sum ./
RUN sha256sum go.mod go.sum > /tmp/dependencies.sha256 \
    && go mod download \
    && go mod verify \
    && sha256sum -c /tmp/dependencies.sha256
COPY docker/init_build.sh /tmp/
COPY server/ /server/
COPY web/ui /server/ui
RUN --network=none sh /tmp/init_build.sh

FROM docker.io/library/alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63
LABEL maintainer="github.com/bjdgyc"

ENV TZ=Asia/Shanghai
ENV ANYLINK_IN_CONTAINER="on"
ENV IPTABLES_LEGACY="off"

WORKDIR /app
COPY docker/init_release.sh /tmp/
COPY --from=builder_golang /server/anylink /app/
COPY docker/docker_entrypoint.sh server/bridge-init.sh ./README.md ./LICENSE version_info /app/
COPY ./deploy /app/deploy
COPY ./index_template /app/index_template
COPY server/conf/server-sample.toml server/conf/profile.xml /app/conf/
RUN sh /tmp/init_release.sh

EXPOSE 443 8800 443/udp
ENTRYPOINT ["/app/docker_entrypoint.sh"]
