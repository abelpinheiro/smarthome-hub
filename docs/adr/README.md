# Architecture Decision Records

An ADR captures a single architectural decision, the context that forced it, and
the consequences — including the ones we accepted knowingly.

Format: [Michael Nygard's](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
— see [0000-template.md](0000-template.md).

ADRs are immutable once accepted. A decision that changes is not edited; it is
**superseded** by a new ADR that references it. The history of how the system's
thinking evolved is as valuable as its current state.

## Index

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-modular-monolith-in-go.md) | Event-driven modular monolith in Go | ✅ accepted |
| [0002](0002-mqtt-device-protocol.md) | MQTT as the device-facing protocol | 🚧 to write |
| [0003](0003-postgres-timescaledb.md) | PostgreSQL + TimescaleDB as the single store | 🚧 to write |
| [0004](0004-hybrid-hub-first-topology.md) | Hybrid hub-first topology, cloud as a future module | 🚧 to write |
| [0005](0005-simulators-as-device-strategy.md) | Simulators as the development device strategy | 🚧 to write |
| [0006](0006-control-authority-hierarchy.md) | Control Authority Hierarchy and auto/manual arbitration | 🚧 to write |
| [0007](0007-open-source-posture.md) | Open-source posture: public code, private configuration | 🚧 to write |

## Writing one

1. Copy `0000-template.md` to `NNNN-short-kebab-title.md`.
2. Fill **Context** before **Decision**. If you cannot articulate the forces,
   you do not yet understand the decision well enough to record it.
3. Be specific in **Consequences → Negative**. "Adds some complexity" says
   nothing; "one deployable means a memory leak in telemetry takes down device
   control too" says everything.
4. Fill **Revisit when** with a measurable signal, not a feeling.
5. Add the row to the index above.
