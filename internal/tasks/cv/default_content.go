// Package cv provides the CV task for managing resume content.
package cv

const defaultCVMarkdown = `# Zhonghua (Laisky) Cai
Ottawa, ON, Canada · Open to remote (Canada/US)
job@laisky.com · LinkedIn: https://www.linkedin.com/in/zhonghua-cai-14237926/
GitHub: https://github.com/Laisky · Blog: https://blog.laisky.com/pages/0/

## Summary
Senior Software Engineer (Backend/Infrastructure) with 10+ years building and running Linux services and internal platforms (PaaS/SaaS/Infra). I focus on **user value** (external users and internal developers) and build systems that are **reliable, maintainable, and fast**. Strong in distributed systems, production ownership, and cross-team delivery.
Top performance rating at every company. Remote-ready with US/EU collaboration experience; strong written communication (spoken English improving).

## Skills
- **Languages:** Go, Python, JavaScript/TypeScript
- **Backend:** API design, distributed systems, concurrency/async, performance tuning, data pipelines
- **Infra/platform:** Kubernetes, Docker, CI/CD, automation (e.g., Ansible), tracing/observability
- **Cloud:** AWS; IDC operations
- **Security:** PKI, KMS, zero-trust patterns; confidential computing (TEE, SGX, SEV-SNP, TDX, TPM)

## Experience

### Career Break (Relocation) + Independent Projects / Contract Work
Ottawa, Canada (Remote) · Oct 2024 – Feb 2026
- Relocated to Canada for immigration and family reasons; not available for full-time work during the move.
- Kept shipping: ran and maintained self-hosted services (blog, **one-api** gateway, RAG/chat APIs, MCP server, chat web app) with Postgres/Mongo/Redis/MinIO plus backups and background jobs.
- Took over maintainership of **one-api** (28k+ stars): forked after the original author stepped back, shipped ongoing fixes/features, handled issues/PRs, and supported real production users; also contributed to two remote international contract projects (details on request).

### R&D Engineer, Infrastructure Security (Backend/Platform)
Shanghai BaseBit Technologies · Apr 2022 – Oct 2024
- Built a general-purpose SGX SDK in Go to make enclave-based workloads usable for service teams.
- Designed a confidential Kubernetes setup with practical deployment/upgrade paths.
- Implemented PKI and multi-party key workflows for confidential VMs to reduce operational and insider risk.
**Performance:** top rating.

### Server Expert (Cloud Security) / Team Lead
Qihoo 360 (Government & Enterprise Security) · Feb 2019 – Apr 2022
- Led backend/platform delivery for multi-tenant cloud security products (CWPP, CSMP).
- Improved stability and maintainability through clearer service boundaries, safer releases, and better troubleshooting tooling.
- Coordinated delivery across PM/QA/infra teams.
**Performance:** top rating.

### Architect, Cloud Platform (PaaS)
Shanghai PATEO (Connected Vehicles / IoV) · Jan 2018 – Feb 2019
- Owned platform reliability work across 300+ services and thousands of containers on Kubernetes.
- Built **go-fluentd** (Go) logging middleware sustaining ~1 Gbps ingest for large-scale telemetry.
- Standardized CI/CD and runbooks used across teams.
**Performance:** top rating.

### Senior Software Engineer (Remote, US HQ)
Movoto (US Real Estate) · Dec 2016 – Dec 2017
- Improved Python data pipelines for millions of listings and 300M+ addresses via profiling-driven refactors and async processing.
- Built and open-sourced **kipp** (Tornado/asyncio-compatible) to simplify maintainable async services.
**Performance:** top rating.

### Earlier Roles
SAIC Motor E-Commerce (DevOps Lead) · 2015 – 2016 · Kubernetes adoption, build platform (~1,000 builds/day) · **Top rating**
Shanghai Qisense (Python Engineer) · 2014 – 2015 · AWS automation + data/ML pipelines · **Top rating**
Meteorological Bureau (Research Engineer) · 2012 – 2014 · forecast verification systems · **Top rating**

## Open Source
- Maintainer/fork owner: **one-api** (28k+ stars) — https://github.com/Laisky/one-api
- Selected OSS: go-fluentd, kipp — see GitHub: https://github.com/Laisky

## Education
B.S., Atmospheric Science (Math/Physics concentration) — Lanzhou University, 2012

## Languages
Mandarin (native) · English (professional; strong written communication, improving spoken)

`
