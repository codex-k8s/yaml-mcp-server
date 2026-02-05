<div align="center">
  <img src="docs/media/logo.png" alt="yaml-mcp-server logo" width="120" height="120" />
  <h1>yaml-mcp-server</h1>
  <p>🔐 MCP gateway with declarative YAML tools and a pluggable approval system for safe model actions.</p>
</div>

![Go Version](https://img.shields.io/github/go-mod/go-version/codex-k8s/yaml-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/codex-k8s/yaml-mcp-server.svg)](https://pkg.go.dev/github.com/codex-k8s/yaml-mcp-server)

🇷🇺 Русская версия: [README_RU.md](README_RU.md)

`yaml-mcp-server` is a single MCP server for a cluster that reads a YAML‑DSL to define tools and resources,
executes approver chains, and returns strictly structured responses.

## 🎯 Idea and Motivation

The server enables **safe execution** of high‑risk operations (secrets, infra/repo changes, etc.) by requiring
explicit approval through pluggable approvers (HTTP/Shell/limits).

## ✅ Key Features

- MCP server (HTTP/stdio) with tools created from YAML‑DSL.
- Ordered approval chains per tool (limits → shell → HTTP, etc.).
- HTTP executors (sync/async) with webhook callbacks.
- Optional idempotency cache for repeated calls.
- Strict response contract: `status`, `decision`, `reason`, `correlation_id`.
- Health endpoints: `/healthz`, `/readyz`.
- YAML templating with env checks before startup.

## 🔗 Related repositories

- `telegram-approver` — Telegram approver for approval flow: https://github.com/codex-k8s/telegram-approver
- `codexctl` — CLI orchestrator for environments and Codex workflows: https://github.com/codex-k8s/codexctl
- `project-example` — Kubernetes project example with ready manifests: https://github.com/codex-k8s/project-example

## 📦 Installation

Go **>= 1.25.5** is required (see `go.mod`).

```bash
go install github.com/codex-k8s/yaml-mcp-server/cmd/yaml-mcp-server@latest
```

## 🚀 Quick Start

```bash
export YAML_MCP_CONFIG=/path/to/config.yaml
export YAML_MCP_LANG=en
export YAML_MCP_LOG_LEVEL=info

yaml-mcp-server
```

Default MCP HTTP endpoint: `http://localhost:8080/mcp`.

### Embedded configs

To use a config embedded from `configs/`, pass:

```bash
yaml-mcp-server --embedded-config github_secrets_postgres_k8s.yaml
yaml-mcp-server --embedded-config github_review.yaml
yaml-mcp-server --embedded-config telegram_feedback.yaml
```

## 🔌 Connect to Codex (CLI/IDE)

Codex stores MCP configuration in `~/.codex/config.toml`. You can also scope it per project with `.codex/config.toml`
for trusted projects. The CLI and IDE extension share the same configuration.

### Option 1 — via CLI

```bash
codex mcp add github_secrets_postgres_k8s_mcp --url http://localhost:8080/mcp
codex mcp list
```

After adding, make sure to set `tool_timeout_sec` in `config.toml` so Codex does not terminate long approval flows
on the client side (seconds).

### Option 2 — via config.toml

```toml
[mcp_servers.github_secrets_postgres_k8s_mcp]
url = "http://localhost:8080/mcp"
tool_timeout_sec = 3600
```

If the server is deployed in a cluster, use an ingress/port‑forward URL (or service DNS).

You can also attach the built-in review workflow config:

```toml
[mcp_servers.github_review_mcp]
url = "http://localhost:8080/mcp"
tool_timeout_sec = 600
```

## 🧩 YAML‑DSL (short)

YAML defines server settings, tools, and resources. See `configs/`.

### Server

```yaml
server:
  name: github_secrets_postgres_k8s_mcp
  version: "0.1.0"
  transport: "http"   # http | stdio
  shutdown_timeout: "10s"
  idempotency_cache:
    enabled: true
    ttl: "24h"
    max_entries: 2000
    key_strategy: "auto"
  startup_hooks:
    - timeout: "10s"
      command: |
        command -v gh >/dev/null
        command -v kubectl >/dev/null
    - timeout: "30s"
      command: |
        printf %s "$YAML_MCP_GH_PAT" | gh auth login --with-token
  http:
    host: "127.0.0.1"
    port: 8080
    path: "/mcp"
    read_timeout: "1h"
    write_timeout: "1h"
    idle_timeout: "1h"
  approval_webhook_url: "http://yaml-mcp-server.local/approvals/webhook" # optional, async HTTP approvers
  executor_webhook_url: "http://yaml-mcp-server.local/executors/webhook" # optional, async HTTP executors
```

`server.http.host` is required. For local testing you can use `0.0.0.0`,
but this is **unsafe** — only use it in an isolated environment.

### Idempotency

If `server.idempotency_cache` is enabled, the server returns cached responses
for repeated tool calls. Cache keys are derived from `correlation_id`/`request_id`
(if provided) or from a hash of arguments.

### Tool

Use `snake_case` tool names with a service prefix (for example, `github_*` or `k8s_*`)
to avoid collisions with other MCP servers.

```yaml
tools:
  - name: github_create_env_secret_k8s
    title: "Create GitHub secret and K8s secret"
    description: |
      Creates a GitHub environment secret and injects it into Kubernetes after approval.
      Input fields:
      - secret_name: secret name (uppercase, digits, underscores).
      - environment: target environment, allowed values: ai-staging or staging.
      - namespace: Kubernetes namespace for secret injection.
      - k8s_secret_name: Kubernetes Secret name (kebab-case).
      - justification: required; write in language "{{ envOr "YAML_MCP_LANG" "en" }}".
      - approval_request: required; concise action summary in the same language.
      - risk_assessment: required; describe possible risks/side-effects in the same language.
      - correlation_id (optional): provide a stable id to enable idempotent responses.
      - links_to_code (optional): list of code references (text/url).
      Notes:
      - GitHub repository is fixed by server configuration.
      - The secret value is generated by the server, do NOT provide secret_value.
    annotations:
      read_only_hint: false
      destructive_hint: true
      idempotent_hint: false
      open_world_hint: true
      title: "Create GitHub env secret + K8s secret"
    requires_approval: true
    timeout: "1h"
    timeout_message: "approval timeout"
    input_schema:
      type: object
      additionalProperties: false
      required: ["secret_name", "environment", "namespace", "k8s_secret_name", "justification", "approval_request", "risk_assessment"]
      properties:
        correlation_id: { type: string }
        secret_name: { type: string, pattern: "^[A-Z0-9_]+$" }
        environment: { type: string, enum: ["ai-staging", "staging"] }
        namespace: { type: string, pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" } # DNS-1123
        k8s_secret_name: { type: string, pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" } # DNS-1123
        justification: { type: string, minLength: 10, maxLength: 500 }
        approval_request: { type: string, minLength: 10, maxLength: 500 }
        risk_assessment: { type: string, minLength: 10, maxLength: 500 }
        links_to_code:
          type: array
          maxItems: 5
          items:
            type: object
            additionalProperties: false
            required: ["text", "url"]
            properties:
              text: { type: string }
              url: { type: string }
    approvers:
      - type: limits
        fields:
          secret_name: { regex: "^[A-Z0-9_]+$" }
          environment: { regex: "^(ai-staging|staging)$" }
          namespace: { regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" }
          k8s_secret_name: { regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" }
          justification: { min_length: 10, max_length: 500 }
          approval_request: { min_length: 10, max_length: 500 }
          risk_assessment: { min_length: 10, max_length: 500 }
      - type: shell
        timeout: "1m"
        command: |
          repo="{{ env "YAML_MCP_GITHUB_REPO" }}"
          if gh secret list -R "$repo" | awk '{print $1}' | grep -qx "{{ "{{ .Args.secret_name }}" }}"; then
            echo "secret already exists"; exit 1; fi
    executor:
      type: shell
      timeout: "1h"
      command: |
        secret_value="$(head -c 32 /dev/urandom | base64)"
        repo="{{ env "YAML_MCP_GITHUB_REPO" }}"
        gh api -X PUT "repos/$repo/environments/{{ "{{ .Args.environment }}" }}" >/dev/null
        gh secret set {{ "{{ .Args.secret_name }}" }} -R "$repo" --env {{ "{{ .Args.environment }}" }} --body "$secret_value"
        kubectl -n {{ "{{ .Args.namespace }}" }} create secret generic {{ "{{ .Args.k8s_secret_name }}" }} \
          --from-literal={{ "{{ .Args.secret_name }}" }}="$secret_value" \
          --dry-run=client -o yaml | kubectl apply -f -
        echo "secret {{ "{{ .Args.secret_name }}" }} created in $repo env {{ "{{ .Args.environment }}" }} and injected into {{ "{{ .Args.namespace }}" }}/{{ "{{ .Args.k8s_secret_name }}" }}"
```

### Resources

```yaml
resources:
  - name: Welcome
    uri: static:welcome
    description: Welcome message
    mime_type: text/plain
    text: "Hello from yaml-mcp-server"
```

## 🔄 End‑to‑end DB flow (github_create_env_secret_k8s → k8s_create_postgres_db)

1) The model requests secrets such as `PG_USER` and `PG_PASSWORD` via
   `github_create_env_secret_k8s` (two separate calls).
   Secrets are created in GitHub and **immediately injected** into Kubernetes.
2) The model calls `k8s_create_postgres_db`, passing only secret names and keys:
   - `k8s_pg_user_secret_name` / `pg_user_secret_name`
   - `k8s_pg_password_secret_name` / `pg_password_secret_name`
3) The tool reads values from K8s secrets and creates the database inside the PostgreSQL pod.

### Benefits of this approach

- **The model never sees secret values**, but can still execute an approved workflow.
- **Secrets are immediately available** to services via Kubernetes Secret.
- **Unified approval chain and audit** through yaml-mcp-server.

### k8s_create_postgres_db request example

```json
{
  "correlation_id": "corr-...",
  "tool": "k8s_create_postgres_db",
  "arguments": {
    "namespace": "project-ai-staging",
    "db_name": "billing",
    "k8s_pg_user_secret_name": "db-credentials",
    "pg_user_secret_name": "PG_USER",
    "k8s_pg_password_secret_name": "db-credentials",
    "pg_password_secret_name": "PG_PASSWORD",
    "justification": "New database required for billing service",
    "approval_request": "Create a DB and set the owner using Kubernetes secrets.",
    "risk_assessment": "May create an extra DB if the name is wrong; requires careful review."
  }
}
```

### Response example

```json
{
  "status": "success",
  "decision": "approve",
  "reason": "database billing created in namespace project-ai-staging",
  "correlation_id": "corr-..."
}
```

## 🧪 Approvers

Supported approvers:

- `limits` — rate limits and field validation (regex, min/max, length).
- `shell` — approval based on a shell command.
- `http` — approval via external HTTP service.

**Order is exactly as in YAML.** Chain stops on first `deny`.

For `http` you can set:
`async` (true/false), `markup` (markdown/html), `webhook_url` (override).

`markup: markdown` uses **MarkdownV2** (Telegram).

### HTTP‑approver: request

An HTTP approver can be **any** service that implements the contract below.
You can build an approver via Telegram (see `telegram-approver`: https://github.com/codex-k8s/telegram-approver),
or via Mattermost/Slack, or a more complex Jira workflow.

```json
{
  "correlation_id": "corr-...",
  "tool": "github_create_env_secret_k8s",
  "arguments": {
    "secret_name": "POSTGRES_PASSWORD",
    "environment": "ai-staging",
    "namespace": "project-ai-staging",
    "k8s_secret_name": "db-credentials"
  },
  "justification": "Need a new password for the billing service.",
  "approval_request": "Create a secret and inject it into Kubernetes.",
  "risk_assessment": "May affect DB access if the new secret is misused.",
  "links_to_code": [
    { "text": "PR #42", "url": "https://github.com/org/repo/pull/42" }
  ],
  "lang": "en",
  "markup": "markdown",
  "timeout_sec": 3600,
  "callback": {
    "url": "http://yaml-mcp-server.codex-system.svc.cluster.local/approvals/webhook"
  }
}
```

Fields:
- `justification`, `approval_request`, `risk_assessment`: 10–500 chars (**required**).
- `links_to_code`: up to 5 links (`text`, `url`).
- `lang`: `ru`/`en`.
- `markup`: `markdown`/`html`.

### HTTP‑approver: response

```json
{ "decision": "approve", "reason": "ok" }
```

`decision` is: `approve | deny | error` (for async, `pending` is also allowed).

### HTTP‑approver (async)

If `approver.async: true`, the approver may return:

```json
{ "decision": "pending", "reason": "queued" }
```

Then it sends a webhook to `server.approval_webhook_url`:

```json
{
  "correlation_id": "corr-...",
  "decision": "deny",
  "reason": "Not enough context"
}
```

⚠️ Security: the webhook has no shared secret. Restrict access at the network level
(Kubernetes NetworkPolicy, service mesh/mTLS, private Service + no public Ingress).

## 📡 Tool Response Protocol

```json
{
  "status": "success|denied|error",
  "decision": "approve|deny|error",
  "reason": "secret POSTGRES_PASSWORD created in owner/repo env ai-staging and injected into project-ai-staging/db-credentials",
  "correlation_id": "corr-..."
}
```

## 🔧 YAML templating

Available template functions:

- `env`, `envOr`, `default`, `ternary`, `join`, `lower`, `upper`, `trimPrefix`, `trimSuffix`, `replace`.

The server checks that all referenced env vars exist **before** startup.

⚠️ Important: the config is rendered **at startup**. Any `{{ .Args.* }}` expressions must be
**escaped** so they are evaluated at tool call time, not during startup.
Use a nested expression:

```
{{ "{{ .Args.secret_name }}" }}
```

## ❤️ Health endpoints

- `GET /healthz` — liveness
- `GET /readyz` — readiness

## ⚙️ Environment Variables

- `YAML_MCP_CONFIG` — path to YAML config (default `config.yaml`).
- `YAML_MCP_GITHUB_REPO` — GitHub repo in `owner/name` format (for tools with fixed repo).
- `YAML_MCP_APPROVAL_WEBHOOK_URL` — external URL for async callbacks (when async HTTP approvers are used).
- `YAML_MCP_EXECUTOR_WEBHOOK_URL` — external URL for async callbacks (when async HTTP executors are used).
- `YAML_MCP_LOG_LEVEL` — `debug|info|warn|error`.
- `YAML_MCP_LANG` — `en` (default) or `ru`.
- `YAML_MCP_SHUTDOWN_TIMEOUT` — graceful shutdown timeout.

### Embedded config envs & secrets

**configs/github_secrets_postgres_k8s.yaml**
- Required: `YAML_MCP_GH_PAT`, `YAML_MCP_GITHUB_REPO`, `YAML_MCP_APPROVER_URL`, `YAML_MCP_APPROVAL_WEBHOOK_URL`
- Optional: `YAML_MCP_LANG`, `YAML_MCP_LOG_LEVEL`, `YAML_MCP_POSTGRES_POD_SELECTOR`

**configs/github_review.yaml**
- Required: `YAML_MCP_GH_PAT`, `YAML_MCP_GITHUB_REPO`, `YAML_MCP_GH_USERNAME`
- Optional: `YAML_MCP_LANG`, `YAML_MCP_LOG_LEVEL`

**configs/telegram_feedback.yaml**
- Required: `YAML_MCP_EXECUTOR_URL`, `YAML_MCP_EXECUTOR_WEBHOOK_URL`
- Optional: `YAML_MCP_LANG`, `YAML_MCP_LOG_LEVEL`

## 📄 Examples

- `configs/github_secrets_postgres_k8s.yaml`
  (contains two tools: github_create_env_secret_k8s and k8s_create_postgres_db)
- `configs/github_review.yaml`
  (tools for deterministic PR review/comment workflows)
- `configs/telegram_feedback.yaml`
  (tool `telegram_request_feedback` executed via async HTTP executor)

## 🧷 Security notes

`yaml-mcp-server` is a **general MCP gateway** that isolates risky actions from the model and only allows execution
through explicit approval. The GitHub secret flow is just an example: the model does not know tokens or secret values,
but can request creation via an approved flow.

There is **no built-in access control yet**. Run the service either locally or in a cluster with strict network access
restrictions to the `yaml-mcp-server`.
