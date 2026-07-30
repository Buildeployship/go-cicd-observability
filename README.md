[![CI/CD](https://github.com/Buildeployship/go-cicd-observability/actions/workflows/ci.yml/badge.svg)](https://github.com/Buildeployship/go-cicd-observability/actions)
[![GitLab CI/CD](https://img.shields.io/badge/GitLab%20CI%2FCD-7%20stages-FC6D26?logo=gitlab&logoColor=white)](.gitlab-ci.yml)
[![Terraform](https://img.shields.io/badge/Terraform-1.14+-7B42BC?logo=terraform&logoColor=white)](https://www.terraform.io/)
[![Docker](https://img.shields.io/badge/Docker-multi--stage-2496ED?logo=docker&logoColor=white)](Dockerfile)
[![AWS](https://img.shields.io/badge/AWS-ECS%20Fargate-FF9900?logo=amazonwebservices&logoColor=white)](terraform/)

# go-cicd-observability

**Description:** A Go webhook relay shipped from source code through a seven-stage GitLab CI/CD pipeline: observability, orchestration, AWS deployment, secrets management.

**Why Go:** It compiles to a single binary, which makes the Docker image smaller and the multi-stage build cleaner. It also signals fluency in the DevOps ecosystem — Docker, Kubernetes, Terraform, and Prometheus are all written in Go.

**Target:** maximum mid-level DevOps coverage in one coherent project rather than tools scattered across unrelated exercises.

**Full ownership from problem to production**: Identify, design, build, deploy, monitor, optimize. No handoffs.

**Pillars:** Automation (Build) · Delivery (Ship) · Operations (Run).

**Tech Stack**
- Language / app: Go, slog, prometheus/client_golang
- Containers: Docker, Docker Compose
- CI/CD: GitLab CI/CD, GitHub Actions
- Security scanning: Trivy, golangci-lint, govulncheck
- Observability: OpenTelemetry, OTel Collector, Loki-Grafana-Tempo-Mimir (LGTM) stack, Alloy, Alertmanager
- Orchestration: Nomad, Consul, Consul Connect (service mesh) mTLS, Envoy
- IaC: Terraform, CloudFormation
- AWS: VPC, ALB/ELB, ECS/Fargate, ECR, S3, IAM, CloudWatch, EC2, KMS
- Secrets: HashiCorp Vault (AppRole, KMS auto-unseal), SOPS, age, AWS Secrets Manager
- Networking: Tailscale
- Scripting/config: Bash, YAML, HCL, Git

---

## Phases

| Phase | Focus                  | Key tools                                                                           |
| ----- | ---------------------- | ----------------------------------------------------------------------------------- |
| 1     | Go webhook relay       | Go, Docker, Docker Compose, slog, prometheus/client_golang                          |
| 2     | GitLab CI/CD pipeline  | GitLab CI/CD, GitHub Actions, Trivy, golangci-lint, govulncheck dependency scanning |
| 3     | Observability          | OpenTelemetry, OTel Collector, Tempo, Mimir, Loki, Alloy, Grafana, Alertmanager     |
| 4     | Consul + Nomad         | Consul, Nomad, Consul Connect (service mesh), mTLS                                  |
| 5     | Terraform + AWS        | Terraform, CloudFormation, VPC, subnets, ALB, ECS, S3, IAM, ECR, CloudWatch         |
| 6     | Secrets management     | HashiCorp Vault, SOPS, AWS Secrets Manager                                          |
| 7     | AWS ECS deployment     | ECS (Fargate/EC2), ECR, ALB routing, CloudWatch                                     |
| 8     | Kubernetes deployment  | K8s manifests, Helm, rolling update, blue-green/canary docs                         |
| 9     | Lambda cleanup         | Lambda (Go/Python), CloudWatch Events, IAM, ECR cleanup                             |
| 10    | README & documentation | Architecture diagram, deployment paths, secrets flow, observability setup           |

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

## Phase 1 — Go webhook relay

An HTTP server in Go that accepts webhook events, emits structured logs, and exposes Prometheus metrics. Multi-stage Docker image (distroless, ~5MB), run locally with Docker Compose.

- HTTP server on port 8080
- `POST /webhook` — accepts JSON, structured logs via `slog`, returns event ID
- `GET /health` — returns `{"status":"healthy"}` for load-balancer health checks
- `GET /metrics` — Prometheus counters and histograms (request count, error count, latency, payload size)
- Multi-stage Dockerfile (golang-alpine → distroless, ~5MB)
- Compose for local dev

```
go-cicd-observability/
├── cmd/relay/main.go
├── internal/handler/webhook.go
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── .gitignore
└── README.md
```

**Run it**

​```bash
docker compose up --build
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{"event":"test","data":"hello"}'
​```

**Proven:** Docker port mapping, container-to-host networking, multi-stage build producing a minimal distroless image.

---

## Phase 2 — GitLab CI/CD pipeline

A seven-stage pipeline runs on every push: **lint → test → secrets → build → scan → push → mirror**. A parallel GitHub Actions workflow runs lint and test so the public mirror carries its own green checks.

| Stage   | What it does                                                                                |
|---------|---------------------------------------------------------------------------------------------|
| lint    | `golangci-lint run` across the module                                                       |
| test    | `go vet`, `govulncheck`, `go test -v ./...`                                                 |
| secrets | Retrieves a secret from Vault via AppRole and decrypts a SOPS file (Phase 6)                 |
| build   | Multi-stage Docker build, tagged with `$CI_COMMIT_SHORT_SHA` and `latest`                   |
| scan    | `trivy image` on the built image (HIGH/CRITICAL gated)                                      |
| push    | Pushes to the self-hosted GitLab Container Registry                                         |
| mirror  | Pushes the commit to the public GitHub mirror (`Buildeployship/go-cicd-observability`)      |

Protected `main` branch, masked CI/CD variables for registry and mirror credentials. Real vulnerabilities have been caught in-flight and gated the release until patched — `govulncheck` flagged an OTel advisory in the test stage, `trivy` flagged transitive CVEs in `golang.org/x/net` (fixed v0.55.0) in the scan stage. The pipeline functions as intended.

**Proven:** Docker-in-Docker builds, registry authentication, cross-platform mirroring, supply-chain scanning inside CI.

---

## Phase 3 — Observability

The Go app is instrumented with the OpenTelemetry SDK. Traces, metrics, and logs flow through a single OTel Collector and fan out to the LGTM stack running in the homelab.

​```
Go app (OTel SDK)
  │
  ├─ traces  → OTel Collector → Tempo
  ├─ metrics → OTel Collector → Mimir  (via remote_write)
  └─ logs    → OTel Collector → Loki

Alloy scrapes /metrics on the relay for a secondary Prometheus-style path.
Grafana reads all four data sources for dashboards and Explore.
Alertmanager receives rules pushed to the Mimir ruler.
​```

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

**Proven:** Alloy scrape-target config (Prometheus format), service-to-service container networking, OTLP endpoint format (`host:port`, no scheme), Mimir `X-Scope-OrgID: anonymous` tenant header for Grafana.

**Known quirk:** Tempo `v2.10.x` has ring/memberlist issues in this topology — stack pinned to `v2.3.1`.

---

## Phase 4 — Nomad + Consul

The relay is deployed to a Nomad cluster as a Consul-registered service, with an Envoy sidecar enabling Consul Connect mTLS. A second service (`echo`) was added so the mesh has something to encrypt between.

**Nomad job** (`nomad/relay.nomad.hcl`)

- Pulls `localhost:5050/buildeployship/go-cicd-observability:latest` from the GitLab registry
- Injects `OTEL_COLLECTOR_ENDPOINT` via template using `attr.unique.network.ip-address` so telemetry reaches the collector on the host LAN IP
- Registers with Consul under the `relay` service name
- HTTP health check on `/health` every 10s
- `connect { sidecar_service {} }` attaches the Envoy proxy

**Consul / Nomad integration fix**

Nomad refused to place the job with `Constraint "${attr.consul.grpc} > 0": 1 nodes excluded by filter`. Root cause: `consul.hcl` had `ports { grpc = "0.0.0.0" }` — a string where an integer was expected. Fix was to split the concerns:

​```hcl
ports {
  grpc = 8502
}
addresses {
  grpc = "0.0.0.0"
}
​```

Nomad's fingerprint flipped from `consul.grpc = -1` to `8502`, the constraint passed, and both services landed with healthy Connect sidecars.

**Known issue (registry pull):** the GitLab registry advertises its token auth realm as `http://gitlab:80` (`registry['token_realm']`), unreachable from Nomad's bridge network — image pulls fail on the JWT auth step. The relay job spec is complete and credential-free; fixing the realm is tracked as follow-up.

**Proven:** Consul DNS, Consul Connect sidecar proxy, mTLS between services, service mesh routing, Nomad template variables.

---

## Phase 5 — Terraform + AWS

Eleven Terraform files define 27 AWS resources. Applied to a live AWS account: relay image pushed to ECR, ECS Fargate ran it, verified against the live ALB, served traffic, then destroyed cleanly.

- Terraform: VPC, subnets, security groups, ALB, ECS cluster, S3, IAM, ECR
- One CloudFormation template (S3 + IAM policy) to show both IaC tools.
- CloudWatch alarms
- Deploy → verify against live ALB → destroy → Meter stopped

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

**Proven:** VPC/CIDR/subnet design, route tables, security groups, NAT-free public subnets, ALB target-group health checks, SSL/TLS termination, ECS task/service lifecycle, IAM least-privilege execution role, S3 state with versioning + encryption, local Docker image → public AWS endpoint.

---

## Phase 6 — Secrets management

Three secret stores, each scoped to its job:

- **HashiCorp Vault** — stores app secrets; runtime secrets pulled by GitLab CI at deploy time. systemd service, Tailscale-only listener, KV v2 at `secret/`. Auto-unseal via AWS KMS (scoped `vault-unseal` IAM user: Encrypt/Decrypt/DescribeKey on one key; Shamir keys migrated to recovery).
- **AppRole for CI** — `gitlab-read` policy, read-only on `secret/data/*`, 403 on write verified. `role_id`/`secret_id` as masked GitLab variables.
- **SOPS + age** — `secrets.enc.yaml` committed encrypted (values ciphertext, keys visible). Private key never in repo; CI decrypts via masked `SOPS_AGE_KEY`.
- **AWS Secrets Manager** — production secrets: created + retrieval verified via CLI. For the ECS deploy path (Phase 7), where task defs reference SM ARNs directly.
- **Nomad relay job** — hardcoded registry token removed; creds now in a root-only host file (`/etc/nomad-docker/docker-auth.json`, 600). Job spec credential-free and committed.

`secrets` stage runs two jobs, both green: `vault-secret-test` (AppRole retrieval) + `sops-decrypt-test` (SOPS decrypt). Two CVEs caught by the gate in-flight (OTel → v1.43.0, golang.org/x/net → v0.55.0).

**Proven:** Vault init/unseal + KMS auto-unseal, AppRole machine auth, least-privilege IAM, SOPS encrypt-commit-decrypt, CI retrieval from Vault + SOPS, credential-free Nomad job.

---
## Phase 7 — AWS deployment

- `deploy` stage in the pipeline: pushes to ECR, deploys to ECS service
- ALB routes to the ECS service
- Secrets from Secrets Manager via the task execution role
- CloudWatch for AWS-side monitoring

**Proven:** ALB → ECS target group routing, container port mappings, ECS service networking.

---
## Phase 8 — Kubernetes deployment

- K8s manifests (Deployment, Service, Ingress)
- Helm chart
- Rolling update strategy
- README documents blue-green/canary concepts and how Argo Rollouts enables them.

**Proven:** K8s Service, Ingress, NetworkPolicy, cluster DNS.

---
## Phase 9 — Lambda cleanup function

- ECR image-cleanup Lambda, deployed via Terraform (Lambda + CloudWatch Events schedule + IAM role)
- Deletes images older than N days or beyond the last N tags

---

## Phase 10 — README & documentation

- Architecture diagram: code → CI/CD → Docker → ECR → ECS/K8s/Nomad → LGTM
- Document each deployment path, secrets flow, and the observability setup
