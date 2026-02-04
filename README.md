<div align="center">
  <img src="docs/media/logo.png" alt="yaml-mcp-server logo" width="120" height="120" />
  <h1>yaml-mcp-server</h1>
  <p>🔐 MCP‑gateway с декларативными инструментами из YAML и системой аппруверов для безопасных действий модели.</p>
</div>

![Go Version](https://img.shields.io/github/go-mod/go-version/codex-k8s/yaml-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/codex-k8s/yaml-mcp-server.svg)](https://pkg.go.dev/github.com/codex-k8s/yaml-mcp-server)

🇬🇧 English version: [README_EN.md](README_EN.md)

`yaml-mcp-server` — единый MCP‑сервер в кластере, который читает YAML‑DSL с описанием ресурсов и инструментов,
подключает цепочки аппруверов и возвращает модели строго структурированные ответы.

## 🎯 Идея и мотивация

Задача сервиса — безопасно исполнять потенциально опасные операции модели
(создание секретов, изменения инфраструктуры/репозиториев и т.д.)
**только после явного approval** через pluggable‑аппруверы (HTTP/Shell/лимиты).

## ✅ Ключевые возможности

- MCP‑сервер (HTTP/stdio) с динамическими инструментами из YAML‑DSL.
- Последовательные аппруверы на инструмент: лимиты → shell → HTTP и т.д.
- Идемпотентность (опционально): кэширование ответов на повторные запросы.
- Жёсткий контракт ответов для модели: `status`, `decision`, `reason`, `correlation_id`.
- Встроенные health endpoints: `/healthz`, `/readyz`.
- Шаблонизация YAML с проверкой всех используемых env до старта.

## 📦 Установка

Требуется Go **>= 1.25.5** (см. `go.mod`).

```bash
go install github.com/codex-k8s/yaml-mcp-server/cmd/yaml-mcp-server@latest
```

## 🚀 Быстрый старт

```bash
export YAML_MCP_CONFIG=/path/to/config.yaml
export YAML_MCP_LANG=ru
export YAML_MCP_LOG_LEVEL=info

yaml-mcp-server
```

По умолчанию HTTP‑endpoint MCP: `http://localhost:8080/mcp`.

### Встроенные конфиги

Если нужно использовать встроенный конфиг из `configs/`, укажите флаг:

```bash
yaml-mcp-server --embedded-config github_secrets_postgres_k8s.yaml
yaml-mcp-server --embedded-config github_review.yaml
```

## 🔌 Подключение к Codex (CLI/IDE)

Codex читает конфигурацию MCP из `~/.codex/config.toml`, либо из проектного `.codex/config.toml` (для trusted projects).
Есть два способа добавить сервер:

### Вариант 1 — через CLI

```bash
codex mcp add github_secrets_postgres_k8s_mcp --url http://localhost:8080/mcp
codex mcp list
```

После добавления обязательно выставьте `tool_timeout_sec` в `config.toml`, чтобы ожидание аппруверов не обрывалось
клиентом Codex (таймаут считается в секундах).

### Вариант 2 — через config.toml

```toml
[mcp_servers.github_secrets_postgres_k8s_mcp]
url = "http://localhost:8080/mcp"
tool_timeout_sec = 3600
```

Если сервер развёрнут в кластере, укажите URL ingress/port‑forward (или сервисный DNS) и добавьте его тем же способом.

Дополнительно можно подключить встроенный конфиг для review‑потоков:

```toml
[mcp_servers.github_review_mcp]
url = "http://localhost:8080/mcp"
tool_timeout_sec = 600
```

## 🧩 YAML‑DSL (кратко)

YAML описывает сервер, инструменты и ресурсы. Пример см. в `configs/`.

### Сервер

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
        printf %s "$CODEXCTL_GH_PAT" | gh auth login --with-token
  http:
    listen: ":8080"
    path: "/mcp"
    read_timeout: "1h"
    write_timeout: "1h"
    idle_timeout: "1h"
```

### Идемпотентность

Если включить `server.idempotency_cache`, сервер будет возвращать сохранённый ответ
для повторных вызовов одного и того же инструмента.
Ключ вычисляется по `correlation_id`/`request_id` (если задан) или по хэшу аргументов.

### Инструмент

Рекомендуем придерживаться нейминга `snake_case` с префиксом сервиса
(например, `github_*` или `k8s_*`), чтобы избегать коллизий между MCP‑сервером и внешними инструментами.

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
      - justification: required; write in Russian.
      - correlation_id (optional): provide a stable id to enable idempotent responses.
      - response_format (optional): json or markdown (default: json).
      Notes:
      - GitHub repository is fixed via YAML_MCP_GITHUB_REPO env variable.
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
      required: ["secret_name", "environment", "namespace", "k8s_secret_name", "justification"]
      properties:
        correlation_id: { type: string }
        response_format: { type: string, enum: ["json", "markdown"] }
        secret_name: { type: string, pattern: "^[A-Z0-9_]+$" }
        environment: { type: string, enum: ["ai-staging", "staging"] }
        namespace: { type: string, pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" } # DNS-1123
        k8s_secret_name: { type: string, pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" } # DNS-1123
        justification: { type: string }
    approvers:
      - type: limits
        fields:
          secret_name: { regex: "^[A-Z0-9_]+$" }
          environment: { regex: "^(ai-staging|staging)$" }
          namespace: { regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" }
          k8s_secret_name: { regex: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" }
      - type: shell
        timeout: "1m"
        command: |
          repo="{{ env "YAML_MCP_GITHUB_REPO" }}"
          if gh secret list -R "$repo" | awk '{print $1}' | grep -qx "{{ .Args.secret_name }}"; then
            echo "secret already exists"; exit 1; fi
    executor:
      type: shell
      timeout: "1h"
      command: |
        secret_value="$(head -c 32 /dev/urandom | base64)"
        repo="{{ env "YAML_MCP_GITHUB_REPO" }}"
        gh api -X PUT "repos/$repo/environments/{{ .Args.environment }}" >/dev/null
        gh secret set {{ .Args.secret_name }} -R "$repo" --env {{ .Args.environment }} --body "$secret_value"
        kubectl -n {{ .Args.namespace }} create secret generic {{ .Args.k8s_secret_name }} \
          --from-literal={{ .Args.secret_name }}="$secret_value" \
          --dry-run=client -o yaml | kubectl apply -f -
        echo "secret {{ .Args.secret_name }} created in $repo env {{ .Args.environment }} and injected into {{ .Args.namespace }}/{{ .Args.k8s_secret_name }}"
```

### Ресурсы

```yaml
resources:
  - name: Welcome
    uri: static:welcome
    description: Welcome message
    mime_type: text/plain
    text: "Hello from yaml-mcp-server"
```

## 🔄 Пример сквозного флоу для БД (github_create_env_secret_k8s → k8s_create_postgres_db)

1) Модель запрашивает создание секрета с именем, например `PG_USER` и `PG_PASSWORD` через
   `github_create_env_secret_k8s` (два отдельных вызова).
   Секреты создаются в GitHub и **сразу инъектятся** в Kubernetes в заданный namespace.
2) Модель вызывает `k8s_create_postgres_db`, передавая **только имена** секретов и ключей:
   - `k8s_pg_user_secret_name` / `pg_user_secret_name`
   - `k8s_pg_password_secret_name` / `pg_password_secret_name`
3) Инструмент сам читает значения из K8s secrets и создаёт БД внутри PostgreSQL Pod.

### Преимущества подхода

- **Модель не видит секреты**, но может запускать согласованный автоматизированный процесс.
- **Секреты сразу доступны сервисам** через Kubernetes Secret.
- **Единая цепочка аппруверов и аудит** — весь поток проходит через yaml-mcp-server.

### Пример запроса для k8s_create_postgres_db

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
    "justification": "Нужна новая БД для сервиса billing"
  }
}
```

### Пример ответа

```json
{
  "status": "success",
  "decision": "approve",
  "reason": "database billing created in namespace project-ai-staging",
  "correlation_id": "corr-..."
}
```

## 🧪 Аппруверы

Поддерживаются:

- `limits` — лимиты/валидации полей (regex, min/max, min/max length).
- `shell` — approval по результату shell‑команды.
- `http` — approval через внешний HTTP‑сервис.

**Порядок строго как в YAML.** На первом `deny` цепочка прерывается.

### HTTP‑approver: формат запроса

```json
{
  "correlation_id": "corr-...",
  "tool": "github_create_env_secret_k8s",
  "arguments": {
    "secret_name": "POSTGRES_PASSWORD",
    "environment": "ai-staging",
    "namespace": "project-ai-staging",
    "k8s_secret_name": "db-credentials"
  }
}
```

### HTTP‑approver: формат ответа

```json
{ "decision": "approve", "reason": "ok" }
```

`decision` принимает ровно: `approve | deny | error`.

## 📡 Протокол ответов инструмента

```json
{
  "status": "success|denied|error",
  "decision": "approve|deny|error",
  "reason": "secret POSTGRES_PASSWORD created in owner/repo env ai-staging and injected into project-ai-staging/db-credentials",
  "correlation_id": "corr-..."
}
```

## 🔧 Шаблонизация YAML

Поддерживаемые функции:

- `env`, `envOr`, `default`, `ternary`, `join`, `lower`, `upper`, `trimPrefix`, `trimSuffix`, `replace`.

Сервер проверяет, что все используемые env переменные заданы **до старта**.

## ❤️ Health endpoints

- `GET /healthz` — liveness
- `GET /readyz` — readiness

## ⚙️ Переменные окружения

- `YAML_MCP_CONFIG` — путь к YAML конфигу (по умолчанию `config.yaml`).
- `YAML_MCP_GITHUB_REPO` — GitHub repo в формате `owner/name` (для tool, где repo фиксирован).
- `YAML_MCP_LOG_LEVEL` — `debug|info|warn|error`.
- `YAML_MCP_LANG` — `en` (default) или `ru`.
- `YAML_MCP_SHUTDOWN_TIMEOUT` — таймаут graceful shutdown.

## 📄 Примеры

- `configs/github_secrets_postgres_k8s.yaml`
  (содержит два инструмента: github_create_env_secret_k8s и k8s_create_postgres_db)
- `configs/github_review.yaml`
  (инструменты для детерминированной работы с review/комментариями PR)

## 🧷 Заметки по безопасности

`yaml-mcp-server` — это **универсальный MCP‑gateway**, который изолирует опасные действия от модели и даёт выполнять их
только через явный approval. Пример с GitHub‑secret — лишь демонстрация подхода: модель не знает токенов и значений,
но может инициировать создание через утверждённый поток.

Пока **нет встроенного разграничения прав доступа**. Поэтому сервис должен работать либо локально,
либо в кластере с жёстким сетевым ограничением доступа к `yaml-mcp-server`.
