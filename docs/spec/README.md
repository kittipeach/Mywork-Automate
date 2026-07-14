# MyWork Automate — Specification Package

> **Project:** MyWork Automate — Node-based Workflow Automation Engine (ส่วนหนึ่งของ MyWork HCM Platform)
> **Version:** 1.0 (Ready for Use — phase-aligned, พร้อมวางลง repo และเริ่มงานกับ Claude Code)
> **Date:** 14 July 2026
> **Owner:** SDM / MyWork Platform Team

## เอกสารในชุดนี้

| ไฟล์ | เนื้อหา | ใช้โดย |
|---|---|---|
| `01-market-research.md` | ระบบ Automation ทั่วโลก + Feature Matrix เทียบกับ Requirement | PO, SA, Management |
| `02-functional-requirements.md` | Functional Requirements ละเอียด (FR-xxx) | SA, BA, Dev, QA |
| `03-non-functional-requirements.md` | NFR: Performance, Security, Availability, Compliance | SA, DevOps, Security |
| `04-architecture.md` | System Design: Next.js + Go + PostgreSQL บน AKS | SA, Dev Lead, DevOps |
| `05-node-catalog.md` | สเปกของ Node ทุกตัว (Trigger, Query, File Gen, Delivery, Logic, AI) | Dev, QA |
| `06-data-model-api.md` | Database Schema + REST API Contracts | Dev Backend, Frontend |
| `07-security-testing.md` | RBAC, AuthN/AuthZ, Pen Test, Perf Test, SonarQube, Trivy | Security, QA, DevOps |
| `08-backlog-epics-stories-tasks.md` | Epic → Story → Task พร้อม Acceptance Criteria + estimate สำหรับ break down เข้า Jira | ทุกทีม |
| `09-phase-plan.md` | แผนแบ่งเฟส: MVP (Phase 1) → Phase 2 → 3 → 4 พร้อม scope, exit criteria, sprint map | PO, Management, ทุกทีม |
| `10-claude-code-playbook.md` | Claude Code prompts: multi-agent parallel, CLAUDE.md, subagents, coverage gates ~100%, test scripts (Makefile, k6, Playwright, CI) | Dev Lead, ทุก Dev |

## สรุปขอบเขตระบบ (One-paragraph Scope)

MyWork Automate คือเครื่องมือสร้าง Workflow Automation แบบ node-based (แนวเดียวกับ Power Automate / n8n) ภายใน MyWork Admin Portal ผู้ใช้ลาก-วาง Node บน canvas เพื่อประกอบ Flow เช่น **Trigger (recurring/event) → Query Azure PostgreSQL → Generate File (Excel/PDF/TXT จาก Template ที่ออกแบบเองได้) → ส่งออกทาง MFT / Email / Download** พร้อม Logic Condition, AI Node ช่วยผู้ใช้, Versioning ทุก Flow, สถานะ Draft / Published / Paused / Stopped, RBAC ระดับข้อมูล sensitive, Login ด้วย Microsoft Entra ID (และ Local User สำหรับ Dev) — Deploy บน Azure AKS, เก็บ Secret ใน Key Vault, เก็บไฟล์บน Blob Storage (Local storage ระหว่าง Dev)

## Tech Stack Summary

| Layer | Technology |
|---|---|
| Frontend | Next.js (App Router), React Flow (@xyflow/react), Tailwind, `@mywork/ui` |
| Backend | Golang (Gin), Worker (Go), Temporal.io (execution engine) |
| Database | Azure Database for PostgreSQL Flexible Server |
| File Storage | Local volume (Dev) → Azure Blob Storage (SIT/UAT/Prod) ผ่าน storage abstraction |
| Secrets | Azure Key Vault (CSI Secret Store Driver + Go SDK) |
| Identity | Microsoft Entra ID (OIDC) + Local User (Dev only, feature-flagged) |
| Deploy | Azure AKS + Helm, Kong API Gateway (existing MyWork infra) |
| Observability | Azure Application Insights (OpenTelemetry), ELK (existing) |
| Quality Gates | k6 (perf test), Pen test (OWASP), SonarQube (SAST), Trivy (CVE/image scan) |
