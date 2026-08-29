FROM golang:1.26.0-bookworm@sha256:2a0ba12e116687098780d3ce700f9ce3cb340783779646aafbabed748fa6677c AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        gcc \
        libx11-dev \
        libxtst-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod LICENSE ./
COPY cmd ./cmd
COPY internal ./internal

ARG BUILD_VERSION=devel
ARG BUILD_REVISION=unknown
ARG BUILD_DIRTY=unknown

RUN CGO_ENABLED=1 go build \
    -mod=readonly \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w -X main.buildVersion=${BUILD_VERSION} -X main.buildRevision=${BUILD_REVISION} -X main.buildDirty=${BUILD_DIRTY}" \
    -o /out/xtest-server \
    ./cmd/xtest-server

FROM debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        libx11-6 \
        libxtst6 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 xtest \
    && useradd --uid 10001 --gid 10001 --no-create-home \
        --home-dir /nonexistent --shell /usr/sbin/nologin xtest \
    && install -d -o 10001 -g 10001 -m 0700 /run/x11-input \
    && install -d -o root -g root -m 0755 /run/secrets

COPY --from=build /out/xtest-server /usr/local/bin/xtest-server
COPY --from=build /src/LICENSE /usr/share/doc/x11-input-daemon/LICENSE

USER 10001:10001
WORKDIR /run/x11-input

ENV XAUTHORITY=/run/secrets/xauthority

STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/local/bin/xtest-server", "-vnext", "unix:/run/x11-input/input.sock", "-vnext-allow", "euid", "-lock-file", "/run/x11-input/authority.lock"]
