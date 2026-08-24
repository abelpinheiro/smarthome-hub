# ADR-0007: Open-source posture: public code, private configuration

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** Abel Pinheiro

## Context

We intend to build and maintain the smart home hub platform as an open-source project to encourage community contribution, maintain transparent architecture, and build a reusable portfolio codebase. However, running a smart home platform inherently touches highly sensitive operational data—including local network topology, Wi-Fi credentials, internal IP addresses, physical device IDs, geographic telemetry, and authentication keys.

Exposing operational secrets or real home telemetry in public version control introduces critical security risks (e.g., unauthorized access to local MQTT brokers or cloud proxies) and severe privacy leaks (e.g., exposing personal presence patterns or home location data).

We need a strict, automated, and unambiguous data categorization strategy that allows us to publish 100% of the project's source code, documentation, and schemas without ever risking the exposure of personal deployment data or instance credentials.

The decisive force, however, is not contribution or portfolio value — both are real but neither would justify the discipline this costs. It is **Kerckhoffs's principle**: a system must remain secure even when everything about its design is known to the adversary. Publishing the code is not merely compatible with that requirement; it is the mechanism that *enforces* it. In a private repository, "nobody knows our topic layout" or "nobody knows how arbitration works" can silently become load-bearing without anyone deciding that it should. In a public one they cannot, because everybody knows. The repository being public is therefore a forcing function against security by obscurity, and that — not community contribution — is why it is public by design.

## Options considered

### Option A — Fully Private Repository

Keeping the entire codebase, configuration, documentation, and deployment scripts in a private version control repository. Eliminates the risk of accidental secret or privacy exposure to the public internet. However, it completely prevents open-source community contributions, restricts code sharing, and defeats the goal of maintaining a public engineering portfolio.

### Option B — Open-Source Codebase with Post-Hoc Secret Scrubbing

Developing in a single public or private repository with hardcoded/local settings and attempting to scrub secrets or sensitive data right before public commits/releases.

Trade-off: Minimal friction during initial setup. However, it relies entirely on human memory and inevitably leads to accidental secret leaks in Git history (e.g., committing a real .env or internal IP log), which requires complex Git history rewrites or credential revocations to remediate.

### Option C — Strict Open-Source Posture with Tri-Tier Configuration Boundaries

Developing the codebase in public from day one using strict environment-driven configuration patterns, template files, and automated repository boundary controls (.gitignore, secret scanners) enforcing three explicit data governance tiers.

Trade-off: Guarantees absolute code visibility while preventing credential and telemetry exposure by design. The trade-off is the operational discipline required to maintain template files (.env.example), configuration loaders, and strict git-hygiene rules across all development workflows.

## Decision

We will adopt a Public Code, Private Configuration strategy (Option C). All application logic, infrastructure code, and documentation will reside in public repositories under three strict data governance rules:

Data Governance Categories
1. Public (Committed to Version Control):
   - All application source code (Go backend, frontend, scripts).
   - Architectural Decision Records (ADRs) and design documentation.
   - MQTT topic schemas, database migration files, and protobuf definitions
   - Broker Access Control List rules (`deployments/mosquitto/acl`), configuration templates (`.env.example`), and Docker Compose manifests using dummy/placeholder values. Note that the ACL file itself is committed rather than templated: its content is policy, not secret — which topics a device may write is exactly the kind of design detail Kerckhoffs says must survive being public. What must never be committed is the password file the ACL refers to.
2. Per-Instance & Generated (Local Only, Strictly .gitignored):
    - Active environment config files (.env).
    - Instance-specific identifiers (e.g., HUB_HOME_ID), generated UUIDs, and system hostnames.
    - Password files (passwd), TLS certificates, private keys, and local Certificate Authority (CA) material.
    - Local database files (/data/ directories, SQLite/TimescaleDB local mounts).
3. Forbidden — Never Versioned or Published (Not Even as Placeholders):
    - Real home telemetry dumps, historical sensor logs, or personal activity patterns.
    - Real hardware identifiers (e.g., physical ESP32 MAC addresses, serial numbers, real device_id values bound to a real home).
    - Real network layout details: home Wi-Fi SSIDs, WPA keys, residential public/private IP addresses, or router DNS records.

## Consequences

### Positive

- Code, architecture, and schemas can be shared, reviewed, and contributed to publicly without code sanitization delays before release.
- Enforces 12-Factor App practices (configuring via environment variables) across all microservices and edge microcontrollers from day one.
- Ensures that cloning the public repository provides a functional development environment without exposing any operational access to actual deployment instances.
- **There is no default secret in the repository for anyone to forget to change.** `make secrets` generates credentials locally on first setup, so the class of vulnerability that ships with a committed `admin/admin` and waits to be discovered cannot exist here. This is the strongest security property of the decision, and it is a control rather than a policy: it does not depend on anyone remembering it. The initialization step this requires is listed below as a cost, and it is the same fact seen from the other side — the friction *is* the control.

### Negative — accepted knowingly

- Any new environment variable or configuration flag requires updating both the runtime configuration parser and its public template (`.env.example`), and any new ACL policy requires updating `deployments/mosquitto/acl`. The template and the parser drift apart silently: nothing fails when `.env.example` is missing a variable, it just leaves the next person with a broken first run.
- Fresh hub deployments require a explicit initialization step (generating instance secrets, certificates, and runtime .env files) rather than using pre-baked default credentials.
- Requires active repository guardrails (e.g., pre-commit hooks like gitleaks or trufflehog) to prevent accidental commits of real .env or credential files.
- **Git history is permanent, so a leak is an incident and not a mistake to undo.** A committed secret is not repaired by deleting the file in a later commit: it remains in history, in every clone, and in any fork. The only correct response is rotation of the leaked credential, followed by history rewriting as cleanup rather than as remedy. This is why two *independent* barriers exist instead of one: the `gitleaks` pre-commit hook is client-side and can be bypassed with `--no-verify` or by a fresh clone that never ran `make hooks`, so GitHub Push Protection backs it server-side where it cannot be skipped.
- **The adversary reads our threat model.** Public ADRs hand an attacker the arbitration policy, the authority hierarchy, the topic structure, and the list of surfaces we already know are weak — including the debts we have accepted and not yet paid, such as the plaintext broker listener in ADR-0002. We are trading reconnaissance advantage for auditability, deliberately, on the grounds that an advantage which depends on the reader's ignorance was never a control.
- **Publishing a security policy creates an obligation.** `SECURITY.md` commits to acknowledging reports within seven days. On a single-maintainer project that is a real duty with no one to delegate it to, and failing it quietly is worse than never having promised it.

## Revisit when

- **A second person gains commit access.** Secret hygiene stops being one developer's personal discipline and becomes a process that has to work across people who did not write these rules.
- **The project starts accepting external contributions.** The client-side `gitleaks` hook does not run on a contributor's machine, so scanning has to move into CI, where it cannot be skipped, before the first pull request is merged.
- **Firmware source enters the repository.** Wi-Fi credentials embedded in firmware samples are a classic leak, and the tri-tier categorization above was written for a Go backend rather than for code that ships onto a device.
- The deployment scales to use a dedicated secret manager (e.g., HashiCorp Vault, SOPS, or Bitwarden Secrets Manager) where configuration templates are managed via automated secrets-injection pipelines rather than local .env files.

Note what is deliberately *not* a revisit trigger: an actual secret leak. That is an incident to be handled by rotation under the process described above, not evidence that this decision was wrong. A posture is not falsified by the failure it was designed to anticipate.