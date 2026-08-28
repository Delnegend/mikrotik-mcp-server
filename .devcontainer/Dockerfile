FROM debian:13-slim

ARG GO_VERSION=1.27.0
ARG GO_ARCH=linux-amd64

ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

RUN --mount=type=cache,target=/var/cache/apt \
    apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git sudo \
    qemu-system-x86 qemu-utils unzip netcat-openbsd just && \
    rm -rf /var/lib/apt/lists/*

# Go toolchain from the official tarball (pinned to go.mod's version).
RUN curl -fsSL -o /tmp/go.tgz \
        "https://go.dev/dl/go${GO_VERSION}.${GO_ARCH}.tar.gz" && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

ENV PATH="/usr/local/go/bin:${PATH}"

RUN groupadd -g 1000 mikrotik && \
    useradd -m -u 1000 -g mikrotik mikrotik && \
    echo "mikrotik ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/mikrotik && \
    chmod 0440 /etc/sudoers.d/mikrotik

USER mikrotik
