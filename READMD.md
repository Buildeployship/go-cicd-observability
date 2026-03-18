# go-cicd-observability

A webhook relay service demonstrating full DevOps CI/CD pipeline with observability.

## Phase 1: Local Development

### Run locally
```bash
go run ./cmd/relay
```

### Run with Docker
```bash
docker compose up --build
```

### Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook` | POST | Accepts JSON payload, logs event, returns event ID |
| `/health` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |

### Test
```bash
curl -X POST http://localhost:8080/webhook \
    - H "Content-Type: application/json" \
    - d '{"event"" "test", "data": "hello"}'
```
