# smarthome-hub

A modular, self-hosted smart home automation hub. Built to run on hardware you
own, in a house you control, with no mandatory cloud account.

> **Status:** early development (Sprint 0). The skeleton, the boundaries and the
> security posture are in place; business modules are being built.

## Why another home automation project

Most home automation stacks make you choose between "everything runs in someone
else's cloud" and "everything breaks when the internet does". This one is built
around a different premise:

**Devices must keep working when the hub is down, and the hub must keep working
when the internet is down.** The cloud is an enhancement, never a dependency.

## Design principles

| Principle | What it means here |
|---|---|
| **Automatic first** | Every module automates by default. Manual control is a deliberate, time-boxed exception. |
| **Safety is not overridable** | A hardware/firmware failsafe outranks the user, the app and the cloud. Always. |
| **Graceful degradation** | Hub offline? Devices keep their local rules. Internet offline? The house still works. |
| **Boundaries enforced by the compiler** | Modules cannot import each other. Go's `internal/` makes it a build error, not a code-review note. |
| **Security by design** | Per-device credentials, per-device ACLs, no anonymous broker access — from the first commit. |

## Architecture

```
┌── PHYSICAL ──┐   ┌── EDGE ──┐   ┌────────── HUB (single binary) ──────────┐   ┌─ CLIENTS ─┐

 ESP32 / relay                    ┌──────────────────────┐
 soil sensor    ──MQTT/TLS──▶  MQTT │  Ingestion            │
 lamp / dimmer    (per-device  Broker└──────────┬───────────┘
 light sensor      credentials)      ╔══════════▼══════════╗
                                     ║     EVENT BUS       ║  (in-process today,
                                     ╚══╤═══════╤═══════╤══╝   NATS-ready)
                                        │       │       │
                                     Device  Telemetry  Command
                                     Registry  & Metrics Dispatcher
                                     +Shadow      │      (arbitration,
                                        │         ▼       ACK, audit)
                                        │   TimescaleDB
                                        │
                                     ┌──▼───────────────────┐
                                     │ MODULES (pluggable)  │ ──REST/WS──▶ Web
                                     │  • Irrigation        │              Mobile
                                     │  • Lighting          │
                                     └──────────────────────┘
```

### Control Authority Hierarchy

The core idea that makes manual and automatic control coexist safely:

```
Level 3  USER (web/mobile)      → expresses intent, time-boxed
Level 2  HUB (rule engine)      → cross-device automation, scheduling
Level 1  DEVICE (firmware)      → local autonomy, works with the hub down
Level 0  FAILSAFE (hw/firmware) → watchdog. Overrides everyone. Always.
```

Higher levels express **intent**; lower levels guarantee **safety**. A user
command can suspend automation, but nothing suspends the failsafe.

Manual control comes in two distinct kinds, never conflated:

- **Pulse** — a one-shot action (`water for 30s now`). Automation stays active.
- **Mode change** — suspends automation for an explicit TTL, then reverts to
  automatic on its own.

## Quick start

Requirements: Docker, Go 1.24+, `make`.

```bash
make setup   # generates .env with fresh secrets + installs git hooks
make up      # starts MQTT broker and TimescaleDB, waits until healthy
make run     # starts the hub
```

Verify it works:

```bash
make smoke   # publishes test telemetry and queries /health
```

## Repository layout

```
cmd/hub/              composition root — the only place that knows every part
internal/
  platform/           technical capabilities, zero business logic
  core/               core bounded contexts (device, telemetry, command)
  contracts/          shared vocabulary between contexts
  modules/            pluggable business modules
    irrigation/
      module.go       the module's only public entry point
      internal/       sealed by the Go compiler — no other module can import
  api/                HTTP transport (inbound adapter)
deployments/          docker-compose, broker config and ACLs
docs/adr/             architecture decision records
```

**Dependency rule:** `platform` knows nobody. `core` knows `platform` and
`contracts`. `modules` know `platform`, `contracts` and core *interfaces* —
never another module. All inter-module communication goes through the event bus.

## Security

This repository is public by design. Publishing the architecture does not weaken
the system — if it did, the architecture would already be broken
(Kerckhoffs's principle).

What is **never** committed: credentials, certificates, device IDs, Wi-Fi
details, or real telemetry. That last one matters more than it looks: an
irrigation history is presence data, and reveals exactly when a house was empty.

See [SECURITY.md](SECURITY.md) for the full posture and how to report a
vulnerability.

## Roadmap

| Ticket | Scope | Status |
|---|---|---|
| SH-001 | Foundation: ADRs, modular skeleton, local environment | 🚧 in progress |
| SH-002 | Device Registry + Shadow (desired/reported/mode) | ⏳ |
| SH-003 | Telemetry ingestion pipeline | ⏳ |
| SH-004 | Command Dispatcher with ACK and arbitration | ⏳ |
| SH-005 | Failsafe / watchdog layer | ⏳ |
| SH-006 | Irrigation module (first vertical slice) | ⏳ |

## License

[Apache License 2.0](LICENSE)
