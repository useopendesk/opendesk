---
title: Scaling
description: Split opendesk into API, worker and gateway roles, and use an external NATS cluster.
---

A single machine running the self-host bundle serves a sizeable team. When you outgrow it, opendesk scales horizontally without changing code: the hosted service is the same binary, split by role.

## Roles

The opendesk binary runs in one of three roles, set with `OPENDESK_ROLE`:

| Role | Runs | Scale by |
| --- | --- | --- |
| `all` | Everything in one process. The default and the self-host setup. | one instance |
| `api` | The HTTP API (and the web app with `SERVE_WEB=true`). Stateless. | requests |
| `worker` | Background jobs (email sync and sending, automations, SLA, webhooks) and the event relay. | job volume |

Run several `api` instances behind a load balancer, and one or more `worker` instances. Jobs are coordinated through Postgres, so workers do not conflict.

The **realtime gateway** is a separate service and scales independently. It holds no database connection. To share Presence across several gateway nodes, set `CLUSTER_DNS` to a DNS name that resolves to every node.

## External NATS

With one process, NATS runs embedded. As soon as you run more than one opendesk instance, every instance and every gateway must share **one** NATS:

```sh
# opendesk (api and worker)
NATS_URL=nats://nats-1:4222,nats://nats-2:4222,nats://nats-3:4222
NATS_TOKEN=...

# realtime gateway
NATS_URL=nats://nats-1:4222
NATS_TOKEN=...
```

Setting `NATS_URL` turns the embedded server off. A three-node NATS cluster with JetStream, or a managed NATS service, works.

## Database

- Point `DATABASE_URL` at a pooler and `DATABASE_URL_DIRECT` at the primary. See [Postgres](/self-hosting/postgres/).
- Workers and the event relay use the direct URL.

## Storage

Multiple `api` instances need shared object storage: use `STORAGE_DRIVER=s3`, not `local`. See [Storage](/self-hosting/storage/).

## Load balancing

- Route `/socket` to the gateway and everything else to `api` instances, or give the gateway its own hostname and set `GATEWAY_URL`.
- Health checks: `GET /healthz` (liveness) and `GET /readyz` (database and NATS) on opendesk, `GET /health` on the gateway.
- Allow long-lived WebSocket connections through the load balancer.
- Rate limits are counted per process, so the effective limit grows with the number of `api` instances.
