# ADR-0004: Hybrid hub-first topology, cloud as a future module

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** Abel Pinheiro

## Context

Smart home platforms generally follow one of two structural architectures:

- **Cloud-First Topology:** Devices talk directly (or via a thin bridge) to a
  cloud provider (e.g., Tuya, SmartThings, Google Home). All automation logic,
  rule evaluation, and state persistence execute off-site.
- **Local-First Topology:** Devices communicate exclusively with an on-premises
  local controller (e.g., our Raspberry Pi hub). All state, automation logic,
  and processing remain strictly within the local area network (LAN).

A pure cloud-first model creates hard external dependencies: internet outages
break basic local controls (e.g., turning on lights), introduce variable network
latency for real-time automations, and raise privacy concerns regarding sensor
data stream exposure.

Conversely, a strictly local-only system limits key user expectations such as
secure out-of-home remote control, multi-hub synchronization across locations,
cloud backup/restore, and integrations with third-party voice assistants (Alexa,
Google Assistant). We need an architectural topology that guarantees absolute
local autonomy while leaving a clean path for cloud-based remote capabilities
later.

One force deserves to be named on its own, because it constrains every later
decision about retention, export and API exposure: **home telemetry is presence
data**. An irrigation history is a precise record of when the house was empty.
Whatever we do with that data, doing it locally by default is not a preference —
it is the conservative choice.

## Options considered

### Option A - Cloud-First Architecture

The Raspberry Pi hub acts primarily as an edge gateway/forwarder. Local device
events are piped to a cloud backend, where automations, history storage, and
rule evaluation take place.

Trade-off: Offloads computational heavy lifting and complex data analytics off
the local Pi, making remote access trivially easy out of the box. However, it
violates local autonomy: if the internet connection drops, local automation
breaks, latency increases for every sensor-to-actuator loop, and recurring cloud
infrastructure costs are introduced immediately.

### Option B - Strictly Local-Isolated Architecture

The hub operates entirely isolated within the local network. No external
endpoints, cloud services, or remote access mechanisms are designed or
permitted.

Trade-off: Guarantees 100% data privacy, zero recurring cloud costs, and
rock-solid local reliability unaffected by external internet connectivity.
However, it completely blocks remote management when the user leaves home and
makes integration with mainstream ecosystem integrations impossible without
complex self-hosted VPN setups by the user.

### Option C - Hybrid Hub-First Topology (Cloud as an Optional Sync Module)

The local hub is the single source of truth and primary execution engine for all
devices, state management, and rule evaluation. The system is designed to run
100% functionally offline. A cloud tier is architected as an optional,
decoupled, pluggable module that sits downstream — subscribing to hub state
changes for remote proxying, backup, and external integrations without touching
the core execution path.

Trade-off: Requires upfront architectural discipline to keep local hub logic
completely decoupled from cloud transport interfaces. Adds slight complexity in
designing eventual consistency and state synchronization when the cloud module
is eventually attached, but ensures zero runtime dependency on internet access
for local operation.

## Decision

We will adopt a Hybrid Hub-First Topology where the local hub operates fully
autonomously as the master coordinator, treating any future cloud infrastructure
purely as an opt-in, non-critical peripheral module.

Concretely, "downstream module" is not an aspiration — it is the shape the
codebase already enforces. A cloud tier will be a `modular.Module` like any
other (ADR-0001). Three invariants define what that module may and may not be,
stated now because they are cheap as text and expensive as a rewrite.

**1. The cloud is never on the sensor→actuator path.** Local automation must
complete with the internet unplugged. Remote *control*, however, necessarily
carries user intent inward, and a pure subscriber cannot do that — so the
invariant is directional, not a blanket ban: the cloud may act as an
authenticated transport on the **user→actuator** path, and never on the
**sensor→actuator** one.

**2. The cloud is never a `CommandSource`.** A command arriving through a relay
is authenticated at the hub as the end user and stamped level 3 (ADR-0006). If
the hub cannot authenticate the end user on its own, the command is discarded.
The relay carries the message; it never vouches for it. Without this,
compromising one relay means owning every house connected to it — precisely the
centralized failure this product exists to avoid.

**3. The cloud receives a derived projection, never the raw telemetry stream.**
Encryption does not protect data from whoever holds the key, and in this model
that party is us. The only mitigation with teeth against occupancy inference is
reducing resolution and scope at the source. The cost is explicit: coarse data
means no second-resolution remote charts, no remote alerting on a moisture drop,
no cross-device analytics in the cloud. We would rather decide that deliberately
than discover it as a leak.

A related question this ADR deliberately leaves open, while naming both answers
so the choice cannot be made by accident: either the cloud **can** read the
projection (conventional encryption in transit and at rest, with us as trusted
operator), or it **cannot** (end-to-end encryption, hub-held keys, the app
decrypting). These are different architectures, not different feature flags —
E2EE rules out server-side search, alerting and web access, and creates a key
recovery problem. Whichever is chosen, choosing it after the module exists means
rewriting it.

## Consequences

### Positive

- Local sensor-to-actuator automations stay entirely on the LAN, so their
  latency is **predictable**: it excludes internet round-trip time and its
  variance entirely. The dominant remaining cost is the Wi-Fi hop and broker
  delivery — tens of milliseconds in practice, but bounded by hardware we
  control rather than by an ISP.
- User sensor telemetry and activity logs remain on the local storage engine
  (PostgreSQL/TimescaleDB) unless explicitly opted into cloud synchronization by
  the user.
- The core product operates without requiring active cloud servers during
  initial development and local deployment phases.
- Presence data does not leave the house by default. Nothing has to be trusted,
  because nothing is transmitted.

### Negative — accepted knowingly

- The Raspberry Pi must handle all state evaluation, rule processing, database
  writes, and local API management autonomously within its hardware resource
  limits.
- Out-of-home control is not available out of the box in the initial phase
  without manual network configurations (e.g., VPNs/Tailscale) until the cloud
  relay/proxy module is formally implemented.
- **We become the operator.** Local-first means there is no third party
  responsible for uptime, backups or recovery. SD card corruption is the classic
  failure mode of a Raspberry Pi running 24/7; unbounded telemetry filling the
  disk is a matter of time; power loss is routine. Retention policy and a
  restore path are therefore not optional features of SH-003 — they are the cost
  of this decision, due when telemetry starts being persisted.
- **The privacy guarantee is deferred, not solved.** Keeping data local by
  default is easy while there is no cloud module. The day one exists, it exports
  the occupancy pattern of the house, and it inherits the whole burden: explicit
  opt-in, per-user scope, a defined retention on the remote side, and the key
  custody question named in the Decision above. This ADR does not grant that
  module permission to exist — it only reserves a constrained place for it.
- **Remote control is a security decision we have deferred, not avoided.** The
  three invariants bound what a cloud module may do, but they do not solve
  end-user authentication at the hub, which is what invariant 2 depends on. That
  work has to exist before any relay does, and it is not in the current backlog.

## Revisit when

- The application scope expands to require seamless multi-location coordination
  (e.g., managing primary home + vacation home) where a centralized cloud
  coordinator becomes mandatory for cross-site logic.
- Integrating with ecosystem providers (like Apple HomeKit, Google Home, or
  Amazon Alexa) imposes mandatory cloud-to-cloud OAuth/Webhook architectures
  that cannot be proxied locally or through lightweight peer-to-peer tunnels.
- A cloud module is actually proposed. At that point this ADR stops being a
  topology decision and becomes a data-exposure decision, and it must be
  reopened with a security review rather than amended in passing.
