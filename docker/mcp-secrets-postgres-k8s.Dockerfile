# syntax=docker/dockerfile:1.4

ARG GOLANG_IMAGE_VERSION=1.25.6-bookworm
FROM golang:${GOLANG_IMAGE_VERSION}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV DEBIAN_FRONTEND=noninteractive \
    TZ=UTC \
    GOPATH=/root/go \
    GOBIN=/usr/local/bin \
    PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

RUN apt-get update -y \
  && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg lsb-release \
  && rm -rf /var/lib/apt/lists/*

RUN install -m 0755 -d /etc/apt/keyrings \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
      -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
 && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
      | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
 && apt-get update -y \
 && apt-get install -y gh \
 && rm -rf /var/lib/apt/lists/*

ARG KUBECTL_VERSION=v1.34.1
RUN curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
  && chmod +x /usr/local/bin/kubectl \
  && kubectl version --client --output=yaml || true

ARG YAML_MCP_SERVER_VERSION=latest
RUN GOBIN=/usr/local/bin go install github.com/codex-k8s/yaml-mcp-server/cmd/yaml-mcp-server@${YAML_MCP_SERVER_VERSION}

ENTRYPOINT ["/usr/local/bin/yaml-mcp-server"]
