# Worker Runbook

## Purpose

The worker is an MVP follow-up loop. It probes Redis and object storage, writes a lightweight cycle marker to Redis, and can optionally emit a JSON status report to object storage.

## Environment

- `SESHAT_REDIS_ADDR`
- `SESHAT_REDIS_PASSWORD`
- `SESHAT_REDIS_DB`
- `SESHAT_MINIO_ENDPOINT`
- `SESHAT_MINIO_BUCKET`
- `SESHAT_WORKER_INTERVAL`
- `SESHAT_WORKER_ONCE`
- `SESHAT_OBJECTSTORE_WRITE_REPORT`

## Typical Local Run

```bash
SESHAT_REDIS_ADDR=localhost:6379 \
SESHAT_MINIO_ENDPOINT=localhost:9000 \
SESHAT_MINIO_BUCKET=seshat \
SESHAT_WORKER_ONCE=true \
go run ./cmd/worker
```

## Notes

- The Redis wrapper speaks RESP directly and is intended for lightweight health checks and simple key writes.
- The object storage wrapper is an HTTP/S3-style MVP client that expects a reachable endpoint and bucket path.
- If you need authenticated uploads, wire a signed proxy or extend the wrapper in a later storage task.
