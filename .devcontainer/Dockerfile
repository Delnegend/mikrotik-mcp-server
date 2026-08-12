FROM debian:13-slim

ARG GO_VERSION=1.25.5
ARG GO_ARCH=linux-amd64
ARG JUST_VERSION=1.36.0
ARG JUST_ARCH=x86_64

ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

RUN apt-get update && apt-get install -y \
    ca-certificates curl git sudo \
    qemu-system-x86 qemu-utils unzip netcat-openbsd && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Go toolchain from the official tarball (pinned to go.mod's version).
RUN curl -fsSL -o /tmp/go.tgz \
        "https://go.dev/dl/go${GO_VERSION}.${GO_ARCH}.tar.gz" && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

ENV PATH="/usr/local/go/bin:${PATH}"

# just (command runner): prefer the distro package, fall back to an official
# GitHub release binary if the base image's repos lack it.
RUN apt-get install -y --no-install-recommends just || \
    ( curl -fsSL -o /tmp/just.tgz \
        "https://github.com/casey/just/releases/download/${JUST_VERSION}/just-${JUST_VERSION}-${JUST_ARCH}-unknown-linux-musl.tar.gz" && \
      tar -C /usr/local/bin -xzf /tmp/just.tgz just && \
      rm /tmp/just.tgz )

RUN groupadd -g 1000 mikrotik-mcp && \
    useradd -m -u 1000 -g mikrotik-mcp mikrotik-mcp && \
    echo "mikrotik-mcp ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/mikrotik-mcp && \
    chmod 0440 /etc/sudoers.d/mikrotik-mcp

USER mikrotik-mcp
