# Data Catalogue Template

> **Owner:** Tech Lead / Data Owner  
> **Required in every project.**

Copy this file into your project as `docs/data-catalogue.md` and keep it updated as the project evolves.

---

## 1. Overview

This document lists all data entities, their sources, ownership, sensitivity, and lifecycle rules.

### 1.1 Project
- Project name:
- Last updated:
- Owner:

### 1.2 Data Principles
- Data is treated as an asset.
- Sensitive data is classified and protected.
- Data retention follows legal and organizational policies.

---

## 2. Data Entities

### 2.1 Entity: `<EntityName>`

| Field | Description |
|---|---|
| **Name** | |
| **Description** | |
| **Source** | e.g., user input, external API, generated |
| **Storage** | e.g., PostgreSQL table `users` |
| **Owner** | Team or role |
| **Sensitivity** | Public / Internal / Confidential / Restricted |
| **PII** | Yes / No |
| **Retention** | e.g., 7 years after account deletion |
| **Access Control** | Who can read / write |
| **Related Entities** | |

#### Attributes

| Attribute | Type | Nullable | Default | Description |
|---|---|---|---|---|
| id | UUID | No | auto | Primary key |
| created_at | timestamp | No | now | Record creation time |

---

## 3. Data Flow

Describe how data moves through the system.

```text
[Source] → [Service/API] → [Database] → [Consumer]
```

Attach a data-flow diagram to `docs/dataflow-diagrams/`.

---

## 4. Data Classification

| Classification | Description | Examples | Handling |
|---|---|---|---|
| Public | Safe to disclose | Public landing page content | Standard controls |
| Internal | Business use only | Analytics aggregates | Limited access |
| Confidential | Sensitive business or user data | Email, phone number | Encryption, access logs |
| Restricted | Highly sensitive | Passwords, tokens, government IDs | Encryption, strict need-to-know |

---

## 5. Compliance & Retention

| Regulation / Policy | Requirement | Affected Data |
|---|---|---|
| GDPR / PDP | Right to erasure | User PII |
| Organization policy | Logs retained 90 days | Application logs |

---

## 6. Data Quality

- Validation rules:
- Cleansing procedures:
- Monitoring:

---

## 7. Backups & Recovery

| Data Store | Backup Frequency | Retention | Recovery Procedure |
|---|---|---|---|
| PostgreSQL | Daily | 30 days | Document in runbook |
