# ADR-0005: Simulators as the development device strategy

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** Abel Pinheiro

## Context

Developing and testing the smart home hub relies on continuous interactions with hardware edge modules (e.g., the autonomous irrigation module, environmental sensor nodes, and relay controllers). However, physical hardware is not always available, fully assembled, or accessible during early software development. Physical devices also present testing challenges: simulating hardware fault states (e.g., sensor drift, network packet loss, unexpected power cuts) on real microcontrollers like an ESP32 is cumbersome and hard to automate in CI/CD pipelines.

We need a strategy to develop, test, and validate the end-to-end functionality of the hub (ingestion, rule engine execution, state synchronization, and UI updates) without blocking software progress on physical hardware readiness or relying on manual hardware setups.

Hardware availability, however, is the weaker of the two forces, because it expires: the day the ESP32 arrives by post, that argument is gone. The durable one is fault injection. Real hardware does not break on demand — you cannot ask an ESP32 to drop its ACK precisely between dispatch and confirmation, to boot with its clock at 1970, or to stop reporting without closing its TCP connection. A simulator is therefore a permanent test instrument, not a temporary crutch, and this ADR is written on that basis.

## Options considered

### Option A — Direct Hardware-Only Development

Developing exclusively against physical microcontrollers (ESP32) connected via Wi-Fi/Ethernet or serial breadboard setups.

Trade-off: Guarantees 100% fidelity to real-world hardware behavior, timing, and network quirks. However, it completely blocks backend and frontend development whenever hardware is unbuilt or offline, makes automated unit/integration testing in cloud environments impossible, and risks physical wear or damage during stress testing.

### Option B — Mocking at the Application Code Level

Inserting software mocks directly inside the hub's Go application layer (e.g., mocking the database repository or internal event bus interfaces).

Trade-off: Very fast to execute in unit tests and requires no network overhead. However, it completely bypasses the real device-facing communication layer (MQTT broker, protocol encoding/decoding, network serialization, and payload schemas). It tests application logic in isolation but fails to evaluate system correctness as a whole.

### Option C - Standalone Network-Level Device Simulators (Faking the Microcontroller)

Building lightweight executable simulators (e.g., in Go, Python, or Dockerized scripts) that mimic edge hardware by connecting to the MQTT broker and publishing/subscribing to exact device topics using the real protocol payload contracts.

Trade-off: Validates the entire hub pipeline end-to-end—from the MQTT network boundary through parsing, TimescaleDB persistence, rule execution, and UI state updates—exactly as if a real ESP32 were communicating. The trade-off is the extra initial effort required to write and maintain simulator scripts alongside the main codebase.

## Decision

We will use Standalone Network-Level Device Simulators as our primary development and integration testing strategy.

The choice of option is the easy half. The decision that gives it teeth is the invariant:

> **A simulator is indistinguishable from real hardware from the hub's point of view.** Same topic hierarchy, its own per-device MQTT credential, subject to the same broker ACL as any physical device, and the same payload schema.

The hub must contain no code path that knows a simulator is a simulator. The moment an `if isSimulator` branch exists, the strategy is dead: we would be exercising a path that does not exist in production and calling the result an integration test. A pull request introducing such a branch violates this ADR, and that is the specific thing this document exists to prevent.

Two consequences follow directly, and both are testable rather than aspirational:

- **The simulator is provisioned as a real device (SH-002).** It gets an entry in the broker password file and a rule in the ACL, exactly like an ESP32 would. It does not borrow the hub's credential.
- **`make smoke` gains a negative test.** Today it publishes with `$MQTT_HUB_USERNAME` — the hub's own credential — which proves only that the hub talks to itself. Once the simulator has its own identity, the smoke test must also assert that `dev-sim-001` publishing to `dev-sim-002`'s topic **fails**. That is what turns the per-device ACL from a documented intention into a verified property.

### Relationship to ADR-0002

This decision is what currently defers ADR-0002's TLS debt. Plaintext MQTT on 1883 is accepted only while the sole publisher is a simulator on loopback; the reversal trigger there is the arrival of the first physical device on the LAN. In other words, ADR-0005 is what holds SH-007 back. Replacing simulators with real hardware fires that trigger, and the two documents must be read together.

## Consequences

### Positive

- Backend, frontend, and rule engine development can proceed at full speed regardless of physical hardware availability or assembly status.
- Tests validate the complete runtime path.
- Simulators are deterministic and scriptable, so the failure modes that matter — a lost ACK, a mid-command disconnect, a device that goes silent without disconnecting — become repeatable test cases instead of anecdotes. This is a capability real hardware does not offer at any price.
- Because the simulator is provisioned as an ordinary device, exercising it also exercises the credential and ACL path. Testing the happy path and testing the isolation boundary become the same activity.

### Negative — accepted knowingly

- Simulators must be kept strictly in sync with the hardware firmware's MQTT protocol spec; schema changes require updating both firmware and simulator code.
- Simulators run on host machine CPUs and may not capture exact hardware timing constraints, low-level ESP32 Wi-Fi reconnect jitter, or subtle hardware-level edge cases (e.g., analog-to-digital converter noise).
- **A simulator encodes our own assumptions about the hardware, so it can only produce the failures we already thought of.** This is the expensive negative, and it is worse than a fidelity gap: it manufactures confidence. Green against the simulator is not "it works" — it is "none of the failures I imagined occurred". Real firmware reboots mid-publish, writes partially, and has bugs we did not model.
- There is no CI in this repository today, so the deterministic integration tests this strategy enables are run manually. The value described above is available, not yet automated.
- Requires writing simulator code and CLI flags/configurations for spinning them up alongside local dev environments (e.g., Docker Compose).

## Revisit when

- **The simulator and the real firmware diverge** — a defect passes against the simulator and fails on hardware, or the reverse. That is the measurable signal that the protocol contract has drifted and that one of the two implementations is lying. It is the trigger that matters most, because it is the one that arrives silently.
- Physical modules are mass-assembled, flashed, and connected to a dedicated automated physical testing bench. Note this changes the simulator's *role* — hardware becomes the authority for final release verification — but does not retire it: fault injection remains something only a simulator can do on demand.
- The hub ever needs to distinguish a simulator from a real device for any reason whatsoever. That requirement contradicts the invariant above, and it means either the requirement or this ADR is wrong.
