# Service Catalogue Template

> **Owner:** Tech Lead  
> **Required in every project.**

Copy this file into your project as `docs/service-catalogue.md` and keep it updated as services are added or changed.

---

## 1. Overview

This document lists every service, its responsibilities, dependencies, ownership, and operational details.

### 1.1 Project
- Project name:
- Last updated:
- Owner:

### 1.2 Architecture Notes
- Monolith / microservices / serverless?
- Primary cloud / hosting platform:
- Link to architecture diagram in `docs/dataflow-diagrams/`.

---

## 2. Services

### 2.1 Service: `<ServiceName>`

| Field | Description |
|---|---|
| **Name** | |
| **Purpose** | One-sentence responsibility |
| **Owner** | Team or role |
| **Repository** | Git URL |
| **Tech Stack** | Language, framework, runtime |
| **Type** | API / Worker / Web / Mobile / Library |
| **Environment** | Staging / Production URLs |

#### Responsibilities

- 
- 

#### API Surface

| Endpoint / Topic | Method | Description | Consumers |
|---|---|---|---|
| `/api/v1/users` | GET | List users | Web app |

#### Dependencies

| Dependency | Type | Required? | Notes |
|---|---|---|---|
| PostgreSQL | Database | Yes | |
| Redis | Cache | No | Rate limiting |
| External SMS provider | Third-party API | Yes | OTP delivery |

#### Deployment

- CI/CD pipeline:
- Deployment strategy (rolling, blue-green, canary):
- Health check endpoint:

#### Monitoring & Alerting

- Metrics dashboard:
- Key alerts:
- Error tracking:

#### Secrets & Configuration

- Required environment variables:
- Secrets manager location:

---

## 3. Service Interaction Map

List which services talk to which, and how.

| Source Service | Target Service | Protocol | Purpose |
|---|---|---|---|
| Web App | API Gateway | HTTPS | User requests |
| API Gateway | User Service | gRPC / HTTP | User data |

---

## 4. External Integrations

| Integration | Purpose | Owner | Contact |
|---|---|---|---|
| Payment gateway | Process transactions | | |

---

## 5. On-Call & Escalation

| Severity | Response Time | Escalation Path |
|---|---|---|
| P1 | 15 minutes | Tech Lead → Engineering Manager |
| P2 | 1 hour | On-call engineer → Tech Lead |
| P3 | 1 business day | Create ticket |
