# go-cicd-observability

[![CI/CD](https://github.com/Buildeployship/go-cicd-observability/actions/workflows/ci.yml/badge.svg)](https://github.com/Buildeployship/go-cicd-observability/actions)
[![GitLab CI/CD](https://img.shields.io/badge/GitLab%20CI%2FCD-6%20stages-FC6D26?logo=gitlab&logoColor=white)](.gitlab-ci.yml)
[![Terraform](https://img.shields.io/badge/Terraform-1.14+-7B42BC?logo=terraform&logoColor=white)](https://www.terraform.io/)
[![Docker](https://img.shields.io/badge/Docker-multi--stage-2496ED?logo=docker&logoColor=white)](Dockerfile)
[![AWS](https://img.shields.io/badge/AWS-ECS%20Fargate-FF9900?logo=amazonwebservices&logoColor=white)](terraform/)

A Go webhook relay service taken from source code through a full CI/CD pipeline, local orchestration with service mesh, a complete observability stack, and a live AWS deployment — all defined as code and torn down cleanly.

**Full ownership from problem to production.** Identify, design, build, deploy, monitor, optimize. No handoffs.

---

## Phases

| Phase | Focus                                           |
|-------|-------------------------------------------------|
| 1     | Go webhook relay (local)                        |
| 2     | GitLab CI/CD pipeline                           |
| 3     | Observability (LGTM stack)                      |
| 4     | Nomad + Consul service mesh                     |
| 5     | Terraform + AWS                                 |
| 6     | Vault + SOPS + AWS Secrets Manager              |
| 7     | GitLab CI/CD deploy stage to ECS                |
| 8     | K8s manifests + Helm chart (alternative to ECS) |
| 9     | Lambda + CloudWatch Events for ECR cleanup      |
| 10    | Architecture diagrams + deployment path docs    |

---

## Architecture

```
code  →  GitLab CI/CD  →  Docker  →  GitLab Container Registry  →  Nomad + Consul (homelab)
                                  └→  ECR  →  ECS Fargate behind ALB (AWS)

Go app (OTel SDK)  →  OTel Collector  →  Tempo (traces) / Mimir (metrics) / Loki (logs)  →  Grafana
                                                                                         →  Alertmanager
```

The same container image ships to two deploy targets: a Nomad cluster with Consul Connect service mesh on the homelab, and ECS Fargate behind an ALB on AWS. Observability signals from both paths flow into a single LGTM stack.

---

## Tech stack

**Automation:** GitLab CI/CD, GitHub Actions, Terraform, Bash, Go, YAML, HCL, Git

**Delivery:** Docker, Docker Compose, Nomad, Consul, Consul Connect (service mesh), AWS (IAM, EC2, ECS Fargate, ECR, S3, ALB/ELB, CloudWatch)

**Operations:** OpenTelemetry, OTel Collector, Tempo, Mimir, Loki, Grafana, Alertmanager, Alloy, CloudWatch

---

## Phase 1 — Go webhook relay (local)

A small HTTP server written in Go that accepts webhook events, emits structured logs, and exposes Prometheus metrics. Packaged as a multi-stage Docker image (distroless runtime, ~5MB) and run locally with Docker Compose.

**Endpoints**

| Method | Path       | Purpose                                                           |
|-------:|------------|-------------------------------------------------------------------|
| POST   | `/webhook` | Accepts JSON payloads, logs via `slog`, returns event ID          |
| GET    | `/health`  | Returns `{"status":"healthy"}` for load balancer health checks    |
| GET    | `/metrics` | Prometheus-format counters and histograms (requests, errors, latency, payload size) |

**Run it**

```bash
docker compose up --build
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{"event":"test","data":"hello"}'
```

**Proven:** Docker port mapping, container-to-host networking, multi-stage build producing a minimal distroless image.

---

## Phase 2 — GitLab CI/CD pipeline

A six-stage pipeline runs on every push: **lint → test → build → scan → push → mirror**. A parallel GitHub Actions workflow runs lint and test so the public mirror carries its own green checks.

| Stage  | What it does                                                                                |
|--------|---------------------------------------------------------------------------------------------|
| lint   | `golangci-lint run` across the module                                                       |
| test   | `go vet`, `govulncheck`, `go test -v ./...`                                                 |
| build  | Multi-stage Docker build, tagged with `$CI_COMMIT_SHORT_SHA` and `latest`                   |
| scan   | `trivy image` on the built image (HIGH/CRITICAL gated)                                      |
| push   | Pushes to the self-hosted GitLab Container Registry                                         |
| mirror | Pushes the commit to the public GitHub mirror (`Buildeployship/go-cicd-observability`)      |

Protected `main` branch, masked CI/CD variables for registry and mirror credentials. A real HIGH-severity finding in `go.opentelemetry.io/otel/sdk` (CVE-2026-39883) was caught by the scan stage and resolved by bumping OTel to `v1.43.0` — the pipeline did its job.

**Proven:** Docker-in-Docker builds, registry authentication, cross-platform mirroring, supply-chain scanning inside CI.

---

## Phase 3 — Observability (local LGTM stack)

The Go app is instrumented with the OpenTelemetry SDK. Traces, metrics, and logs flow through a single OTel Collector and fan out to the LGTM stack running in the homelab.

```
Go app (OTel SDK)
  │
  ├─ traces  → OTel Collector → Tempo
  ├─ metrics → OTel Collector → Mimir  (via remote_write)
  └─ logs    → OTel Collector → Loki

Alloy scrapes /metrics on the relay for a secondary Prometheus-style path.
Grafana reads all four data sources for dashboards and Explore.
Alertmanager receives rules pushed to the Mimir ruler.
```

**What was built**

- OpenTelemetry SDK initialization in `cmd/relay/main.go` with graceful shutdown
- `internal/telemetry/otel.go` exposing a single `InitTelemetry()` entry point
- OTLP HTTP exporter pointed at the homelab collector
- Grafana dashboard panels for request rate, error rate, latency, and payload size
- Alertmanager rules pushed to the Mimir ruler: `RelayHighErrorRate`, `RelayDown`, `RelayHighLatency`, plus node-level `InstanceDown`, `HighCPUUsage`, `HighMemoryUsage`, `DiskSpaceLow`
- Loki retention at 720h (30 days) with the compactor enabled
- Docker `json-file` log rotation

**RCA exercise**

A deliberate burst of `405 Method Not Allowed` errors was generated against `/webhook` (GET instead of POST), then traced end-to-end in Grafana Explore:

1. **Loki** — `{container=~".*relay.*"} |= "error"` returned the five error entries
2. **Tempo** — the corresponding spans, filtered by `service.name=webhook-relay` and HTTP status
3. **Mimir** — the error-rate spike visible on the dashboard over the same window

Logs correlated to traces correlated to metrics. Three pillars, one incident, one story.

**Proven:** Prometheus scrape target config, service-to-service container networking, OTLP endpoint format (`host:port`, no scheme), Mimir `X-Scope-OrgID: anonymous` tenant header for Grafana.

**Known quirk:** Tempo `v2.10.x` has ring/memberlist issues in this topology — stack pinned to `v2.3.1`.

---

## Phase 4 — Nomad + Consul with service mesh

The relay is deployed to a Nomad cluster as a Consul-registered service, with an Envoy sidecar enabling Consul Connect mTLS. A second service (`echo`) was added so the mesh has something to encrypt between.

**Nomad job** (`nomad/relay.nomad.hcl`)

- Pulls `localhost:5050/buildeployship/go-cicd-observability:latest` from the GitLab Container Registry
- Injects `OTEL_COLLECTOR_ENDPOINT` via template using `attr.unique.network.ip-address` so telemetry reaches the collector on the host LAN IP
- Registers with Consul under the `relay` service name
- HTTP health check on `/health` every 10s
- `connect { sidecar_service {} }` block attaches the Envoy proxy

**Consul / Nomad integration fix**

Nomad refused to place the job with `Constraint "${attr.consul.grpc} > 0": 1 nodes excluded by filter`. Root cause: `consul.hcl` had `ports { grpc = "0.0.0.0" }` — a string where an integer was expected. Fix was to split the concerns:

```hcl
ports {
  grpc = 8502
}
addresses {
  grpc = "0.0.0.0"
}
```

Nomad's fingerprint flipped from `consul.grpc = -1` to `8502`, the constraint passed, and both services landed with healthy Connect sidecars.

**Proven:** Consul DNS, Consul Connect sidecar proxy, mTLS between services, service mesh routing, Nomad template variables, private registry auth from Nomad.

**Note on Nomad env vars:** `nomad alloc exec` opens a shell that does *not* inherit task-level env vars from `template { env = true }` blocks. Runtime values must be verified with `nomad alloc logs -stderr`, not by shelling in and running `echo`.

---

## Phase 5 — Terraform + AWS (full deploy / verify / destroy cycle)

Eleven Terraform files define 27 AWS resources. The stack was applied to a live AWS account, the relay image was pushed to ECR, ECS Fargate pulled and ran it, the ALB served real traffic, and everything was destroyed cleanly.

**File layout**

```
terraform/
├── main.tf               provider + backend
├── variables.tf          region, project_name, environment
├── outputs.tf            ALB DNS, ECR URL, cluster name, state bucket
├── vpc.tf                VPC, public subnets (x2 AZ), IGW, route tables
├── security_groups.tf    ALB SG (80 in), ECS SG (app port in from ALB only)
├── alb.tf                ALB, target group (health checks), listener
├── ecr.tf                repo + lifecycle policy
├── ecs.tf                cluster, task definition, service, log group
├── iam.tf                execution role + policy attachment
├── s3.tf                 tfstate bucket, versioning, encryption, public-access-block
└── cloudwatch.tf         CPU, memory, and healthy-host alarms
```

**Mental model**

```
Providers  →  Variables / Outputs  →  Resources  →  Data sources

Resources map to infrastructure layers:
  Network        vpc.tf
  Access         security_groups.tf, iam.tf
  Load balance   alb.tf
  Compute        ecs.tf
  Storage        s3.tf, ecr.tf
  Observability  cloudwatch.tf
```

**Deploy cycle**

```bash
terraform init
terraform validate
terraform plan     # Plan: 27 to add, 0 to change, 0 to destroy
terraform apply    # 27 added

aws ecr get-login-password --region us-west-2 \
  | docker login --username AWS --password-stdin <acct>.dkr.ecr.us-west-2.amazonaws.com
docker tag  localhost:5050/buildeployship/go-cicd-observability:latest \
            <acct>.dkr.ecr.us-west-2.amazonaws.com/go-cicd-observability/relay:latest
docker push <acct>.dkr.ecr.us-west-2.amazonaws.com/go-cicd-observability/relay:latest
```

**Verification against the live ALB**

```
GET  /health   → {"status":"healthy"}
POST /webhook  → {"status":"received","message":"webhook processed successfully"}
```

Screenshots captured for ECS cluster, ECR repo, ALB, CloudWatch alarms, VPC, and the S3 state bucket.

```bash
terraform destroy   # Destroy complete! Resources: 27 destroyed.
```

Total cost: a few dollars in credits. Meter stopped.

**Proven:** VPC/CIDR/subnet design, NAT-free public subnet layout, ALB target group health checks, ECS task/service lifecycle, IAM least-privilege execution role, S3 state storage with versioning and encryption, end-to-end path from local Docker image to public AWS endpoint.

---

## Roadmapped

- **Phase 6** — HashiCorp Vault for app secrets, SOPS for encrypted config files in-repo, AWS Secrets Manager for production secrets (replacing the currently-gitignored registry token in `nomad/relay.nomad.hcl`)
- **Phase 7** — ECS deployment wired directly into the GitLab pipeline's `deploy` stage
- **Phase 8** — Kubernetes path: manifests, Helm chart, documented rolling / blue-green / canary strategies (Argo Rollouts concepts)
- **Phase 9** — Lambda (Go or Python) + CloudWatch Events to clean up old ECR images, deployed via Terraform
- **Phase 10** — Architecture diagram, deployment-path docs, secrets-flow docs, observability-setup docs

---

## Repo pointers

- GitLab (primary): `buildeployship/go-cicd-observability`
- GitHub (mirror): `Buildeployship/go-cicd-observability`
- Homelab stack integrates with: [self-hosted-cicd-observability-stack](https://github.com/Buildeployship/self-hosted-cicd-observability-stack)
