# OpenEverest Audit Plugin

A generic plugin (spec [`003-generic-plugins`](https://github.com/openeverest/specs/blob/main/specs/003-generic-plugins.md)) that subscribes to the OpenEverest event stream, persists every captured event, and exposes a UI to browse, filter and search the audit log.

Built for compliance and operational forensics: who did what, when, to which resource.

---

## Status — MVP (Phase 1)

| Area | Phase 1 (this release) | Phase 2 | Phase 3 | Phase 4 |
| --- | --- | --- | --- | --- |
| Storage | Embedded SQLite (PVC) | External PostgreSQL | Retention / vacuum policy | — |
| UI | Filter, search, JSON drawer | Saved views | MUI + theming | — |
| Operability | `/healthz`, single replica | — | Prom metrics, structured logs | — |
| Delivery | — | — | — | Webhook / Slack / S3 export |

The MVP is **deployable end-to-end**: a Helm chart installs the Plugin CR, the backend daemon consumes SSE events from the host and stores them in SQLite, and the React extension renders the audit log at `/audit` in the OpenEverest UI.

---

## Architecture

```
┌──────────────────┐    SSE /v1/events       ┌─────────────────────────────┐
│ OpenEverest host │ ──────────────────────▶ │ audit pod                   │
│ /v1/events       │                         │ ┌─────────────────────────┐ │
└──────────────────┘                         │ │ consumer (daemon)       │ │
        ▲                                    │ │  - cursor in meta table │ │
        │  /api/* (proxied, JWT injected)    │ └─────────┬───────────────┘ │
        │                                    │           ▼                 │
┌──────────────────┐                         │ ┌─────────────────────────┐ │
│ React extension  │ ──────────────────────▶ │ │ SQLite (PVC: /data)     │ │
│  /audit          │                         │ └─────────┬───────────────┘ │
└──────────────────┘                         │           ▲                 │
                                             │ ┌─────────┴───────────────┐ │
                                             │ │ REST: /api/events …     │ │
                                             │ └─────────────────────────┘ │
                                             └─────────────────────────────┘
```

Three plugin modes are enabled per spec 003 §10.4:
- **`daemon`** — long-running event consumer, single replica
- **`eventConsumer`** — subscribes to the configured event types
- **`requestHandler`** — exposes `/api/*` to the frontend bundle, proxied with the caller's JWT in `X-Everest-User`

---

## Quick start

### Install from a release

```bash
helm install audit oci://ghcr.io/openeverest/charts/audit \
  --version 0.1.0 \
  --namespace everest-system
```

### Install from source

```bash
# 1. Build the React bundle (writes backend/dist/main.js)
npm ci
npm run build

# 2. Build & push the container
docker build -t ghcr.io/<you>/plugin-audit:dev .
docker push ghcr.io/<you>/plugin-audit:dev

# 3. Install the chart
helm install audit charts/audit \
  --namespace everest-system \
  --set image.repository=ghcr.io/<you>/plugin-audit \
  --set image.tag=dev
```

The Plugin CR is created automatically; the OpenEverest controller will register the route and sidebar item at `/audit`.

---

## Configuration

Key Helm values (see [`charts/audit/values.yaml`](charts/audit/values.yaml) for the full list):

| Value | Default | Description |
| --- | --- | --- |
| `plugin.eventTypes` | curated audit list | Event types to subscribe to; empty list = all |
| `plugin.namespaces` | `[]` | Optional namespace filter |
| `storage.persistence.enabled` | `true` | Provision a PVC for SQLite |
| `storage.persistence.size` | `5Gi` | PVC size |
| `storage.sqlite.path` | `/data/audit.db` | Database file path inside the pod |
| `everestAPIURL` | `""` | Override Everest API URL; auto-discovered when empty |
| `resources` | small | CPU/memory requests & limits |

Environment variables (also configurable directly on the Deployment):

| Var | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP listen port |
| `AUDIT_DB_PATH` | `/data/audit.db` | SQLite path |
| `AUDIT_EVENT_TYPES` | (chart-driven) | CSV of event types |
| `AUDIT_NAMESPACES` | (chart-driven) | CSV of namespaces |
| `EVEREST_API_URL` | auto | Override Everest API URL |
| `EVEREST_TOKEN_PATH` | `/var/run/secrets/everest/token` | Mounted plugin service token |

---

## REST API (host-proxied)

All paths below are reached as `/api/...` from the host (the host strips that prefix when forwarding):

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/events` | List events. Query: `types`, `namespaces`, `actors`, `search`, `since`, `until`, `limit` (≤500), `beforeID` (cursor). |
| GET | `/api/events/{id}` | Single event with full envelope. |
| GET | `/api/events/types` | Distinct event types seen. |
| GET | `/api/events/namespaces` | Distinct namespaces seen. |
| GET | `/api/stats` | Aggregate counts by type / namespace / actor + last seen. |
| GET | `/healthz` | Liveness/readiness. |

---

## Local development

```bash
# Frontend (Vite lib mode → backend/dist/main.js)
npm ci
npm run dev          # rebuilds on change

# Backend
cd backend
touch dist/main.js dist/icon.png   # placeholders for go:embed
go run .
```

Point the running server at a real Everest API with `EVEREST_API_URL` and provide a token file via `EVEREST_TOKEN_PATH`.

---

## Roadmap

- **Phase 2** — pluggable storage with PostgreSQL backend, retention policy, schema migration tool.
- **Phase 3** — MUI-themed UI, structured logs, Prometheus metrics, saved filter views, CSV/NDJSON export.
- **Phase 4** — outbound delivery (webhook, Slack, S3) and alerting rules.

---

## License

[Apache-2.0](LICENSE)
