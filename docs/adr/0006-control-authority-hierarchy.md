# ADR-0006: Control Authority Hierarchy and auto/manual arbitration

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** Abel Pinheiro

## Context

In a smart home system with automated rule engines, user interfaces, schedules,
and physical inputs, multiple control sources frequently send conflicting
commands to the same physical actuator (e.g., an automated schedule attempts to
turn on irrigation at 06:00, but a user manually disabled it at 05:59).

Three properties of this domain make the problem different from ordinary state
management:

- **A physical actuator has no rollback.** In a CRUD system a wrong state is
  corrected with an UPDATE. A valve opened by mistake has already flooded the
  floor. The cost of the error is asymmetric and irreversible.
- **The hub is not a reliable arbiter of its own safety.** ADR-0001 accepted,
  knowingly, that this is a single process without fault isolation between
  modules: a panic in telemetry ingestion takes command dispatch down with it.
  Anything that must survive a hub failure cannot live in the hub.
- **The device is a control source, not only a target.** The product premise is
  that devices keep working when the hub is down. Firmware that acts on its own
  — a dry-run pump cutoff, a local moisture threshold — is issuing commands, and
  the arbitration policy has to account for it.

To build a reliable control loop, the hub architecture must resolve three core
operational questions:

1. **Arbitration:** Who wins when two distinct sources command the same
   actuator?
2. **Manual Override Termination:** How and when does a manual user override
   revert back to automated control?
3. **Failsafe Location:** Where does the safety watchdog live to protect
   physical hardware (e.g., preventing a water valve from staying open
   indefinitely if the software crashes)?

Without clear boundaries, actuators suffer from race conditions, silent command
overwrites, unrecoverable hardware lockups, or safety risks if the central
controller crashes mid-operation.

## Options considered

### Part 1: Arbitration Strategy

#### Option A — Last Write Wins (LWW)

The database/actuator state simply accepts the last payload received regardless
of sender. Zero structural complexity initially, but a 06:00 scheduled rule
silently overwrites a user's 05:59 "do not turn on" intent. Eliminates any
concept of authority hierarchy and makes tracing command intent virtually
impossible.

#### Option B — Fixed Source Priority (User > Rule Engine > System Default)

Every command carries an explicit origin/provenance field. Higher authority
sources strictly override lower ones. Clear operational hierarchy (a user always
beats a schedule). However, it requires embedding provenance fields in every API
payload, message schema, and database write across the system from day one.

#### Option C - Fixed Priority + Time-to-Live (TTL) Lease

Higher authority sources take temporary control by holding a time-bound lease.
Once the TTL expires, control yields back to automated rules. Prevents permanent
manual locks. However, state expiration depends on an active scheduler daemon
that must handle edge cases like hub reboots, system time jumps, or Pi clock
drifts during power outages.

#### Option D - Explicit Ownership Locking

A source acquires an exclusive lock on an actuator until it explicitly releases
it. Avoids race conditions completely, but if a caller crashes before releasing
its lock, the actuator remains permanently frozen unless a secondary
renewal/lease mechanism is added (reinventing Option C with extra moving parts).

#### Option E - Layered Authority: safety as constraint, intent as priority

A single linear priority ladder forces one ordering onto two different
questions, and the ordering it produces is incoherent: firmware must be able to
cut a dry-running pump against the user's wish, yet the user must be able to
suspend a schedule. Ranking "firmware" and "user" on one axis cannot express
both.

This option splits the hierarchy into two mechanisms over the same four levels:

- **Levels 0–1 (failsafe, device firmware) are constraints.** They do not
  compete for the actuator; they bound the envelope of what any source may ask
  for. A veto, not a bid.
- **Levels 2–3 (hub automation, user) are intent.** They compete with each
  other by priority, and there the user outranks the schedule and the rule
  engine.

Trade-off: two concepts to learn instead of one, and two code paths in the
dispatcher — a constraint check and a priority comparison — rather than a single
integer comparison. In exchange, each of the two rules stays trivially simple
and independently testable, and neither has to be bent to accommodate the other.

### Part 2: Manual Override Termination

Note that this part has **two independent axes**, and an option that answers
only one of them does not settle the question:

- *Nature of the action* — is this a one-shot execution or a state transition?
- *Termination* — how does the resulting state end?

#### Option A — Infinite Boolean Flag (is_manual_override = true)

A toggle flag that stays active until a human manually flips it back. Answers
termination with "never, unless asked". Leads to the "vacation bug" — a user
toggles an override, goes on vacation, forgets about it, and automated schedules
(like plant watering) remain disabled indefinitely.

#### Option B — Mandatory TTL on All Manual Actions

Every manual action requires an explicit expiration duration (e.g., override for
2 hours). Solves the vacation bug, but introduces state ambiguity if the hub is
powered off when the TTL expires (e.g., whether to revert immediately on boot or
wait for state confirmation), and it makes every manual action a timed one —
including the ones a user legitimately wants to last, such as disabling a zone
with a broken valve until the part arrives.

#### Option C — Explicit Action Nature (Pulse vs. Mode Change)

Distinguishing command intent at the API/domain level into two distinct
operations: a transient Pulse (a single targeted execution with a mandatory
timeout, e.g., "water for 10 minutes") versus a persistent Mode Change (an
explicit system state transition, e.g., "switch irrigation controller to
Manual/Off").

Trade-off: Reflects physical reality cleanly in the domain layer, at the cost of
higher API design complexity (two distinct verbs) and a UI that must teach the
difference to end users. But it answers only the *nature* axis: a Mode Change
still has to say how it ends, and if the answer is "never", this option has
silently become Option A.

#### Option D — Pulse vs. Mode Change, with TTL as the default termination

Option C for the nature axis, Option B for the termination axis, plus one
deliberate escape hatch:

- A **Pulse** carries a mandatory maximum duration and does not change the
  control mode. Automation stays active and resumes on its own.
- A **timed Mode Change** suspends automation until an explicit expiry and
  returns to automatic by itself. This is the default for manual control.
- A **persistent Mode Change** exists for the maintenance case, is never implied
  by a normal manual action, and must be chosen explicitly by the user.

Trade-off: three concepts in the domain instead of one flag, and the persistent
mode reopens the vacation bug for exactly the users who ask for it. In exchange,
the dangerous default is the safe one, and forgetting is no longer failure mode
number one.

### Part 3: Failsafe / Watchdog Location

#### Option A — Hub-Level Watchdog (Software in Go)

The Raspberry Pi central software monitors actuator timers and issues shutoff
commands when thresholds are exceeded.

Trade-off: Easy to write, test, and update in application code. However, as
established in ADR-0001, a process panic, unhandled runtime exception, or OS
freeze on the Pi kills the watchdog simultaneously with the application, leaving
physical actuators stuck in their last active state. A watchdog that shares its
fate with the thing it supervises is not a watchdog.

#### Option B — Firmware/Hardware-Level Watchdog (Edge Device)

The edge microcontroller firmware (e.g., ESP32) enforces a hard, autonomous
hardware cutoff timer (e.g., relay automatically opens after 15 minutes of
continuous activation unless a keep-alive pulse is received).

Trade-off: Completely decoupled from central hub crashes or network
disconnects. The trade-off is that firmware rules must remain simple, absolute,
and static (e.g., max duration limits), as changing them requires OTA firmware
reflashing.

## Decision

We will adopt a multi-layered authority model structured as follows.

### 1. Arbitration — Option E, layered authority

Four levels, and every command payload and state transition must explicitly
record its origin provenance:

| Level | Source | Mechanism |
|---|---|---|
| 0 | `failsafe` — hardware/firmware watchdog | Constraint. Sovereign at runtime against the hub, the user and the cloud |
| 1 | `device_local` — firmware autonomy | Constraint. Bounds what the hub and the user may request |
| 2 | `schedule`, `rule_engine` — hub automation | Intent. Loses to level 3 |
| 3 | `user_web`, `user_mobile` — user | Intent. Wins among intent sources |

The two rules, stated plainly:

- **A constraint is never outranked by an intent.** The user may suspend the
  schedule; the user may not suspend the firmware's dry-run cutoff, and nobody
  suspends the failsafe.
- **Among intent sources, the higher level wins.** A user command overrides a
  schedule or a rule, and does so explicitly rather than by arriving later.

A third rule constrains the constraints themselves, and without it the safety
axis becomes an escalation path:

- **A constraint may only restrict, never energize.** Constraints are monotonic
  in the safe direction: deny, cut, cap a duration. A "constraint" asking for a
  valve to open is not a constraint — it is intent wearing the wrong badge. With
  this rule, a compromised device claiming constraint authority achieves, at
  worst, denial of service over its own actuator, which it could already achieve
  by simply disobeying. Without it, the level-0/1 axis is a way for the cheapest
  device in the house to acquire the tap.

#### Where provenance is established

Provenance decides authority, so provenance itself must not be a claim made by
the party being ranked. It is established by the channel, never taken from the
payload:

| Channel | Legitimate sources | How it is established |
|---|---|---|
| Client REST/WS | `user_web`, `user_mobile` | **Authenticated session.** The transport stamps the source; the payload never declares it. Web vs. mobile is cosmetic — both are level 3, and the *level* comes from the session |
| In-process component | `schedule`, `rule_engine` | **Construction.** The scheduler receives an issuer already bound to `SourceSchedule`. There is no API in which a caller names its own source |
| Device MQTT | `device_local` | **Topic and ACL.** Which device is speaking is trustworthy, because the device-id topic segment is bound to the MQTT credential. That it speaks as a *constraint* remains its claim, and is bounded by the may-only-restrict rule above |
| None | `failsafe` | **Never accepted on the wire.** See below |

**`failsafe` is not a wire-representable source.** By Part 3 the failsafe lives
in firmware and acts on the relay directly; it never needs to enter the hub as a
command. A device may *report* that its failsafe tripped, as telemetry, which
the hub records and displays and which confers authority over nothing. This
removes level 0 from the attack surface entirely, at the cost of one sentence.

#### Settled details

- **Unknown or absent provenance is rejected and audited, never arbitrated.**
  Provenance is a precondition of dispatch, not a field with a fallback. This is
  explicitly *not* "assign it the weakest authority": there is no authority
  weaker than intent in this model — level 3 is the *strongest* intent level, so
  defaulting an unparseable source to it would mean that omitting a field
  promotes a message above every hub automation. Parsing happens at the
  boundary, so by the time arbitration runs, an unknown source no longer exists.
- **Ties inside a level resolve by last write wins**, where "last" means the
  order in which the hub received the command — never a timestamp carried in the
  payload, which an attacker sets to the year 3000 and wins forever. `schedule`
  and `rule_engine` share level 2, and we accept LWW between them: both are
  hub-owned and cooperative, so the failure mode is a suboptimal watering, not an
  unsafe one. This is the one place where Option A survives, and it survives
  bounded.
- **Level 2 is reserved for in-process hub components.** Anything outside the
  process — a webhook, a voice assistant, the future cloud module of ADR-0004 —
  gets its own source constant and its own level. Admitting an external party to
  level 2 would let it defeat every hub automation with a single write
  primitive, with no race to win and no timing to get right.

### 2. Override Termination — Option D

Pulses carry a mandatory maximum duration and leave the control mode untouched.
Manual control is expressed as a timed Mode Change with a mandatory expiry that
returns to automatic on its own. A persistent Mode Change exists for maintenance
and requires an explicit, separate user action; it is never the result of an
ordinary manual command.

### 3. Failsafe Location — Option B

We will place all critical hardware safety watchdogs directly in the device
firmware/hardware. The hub remains responsible for intelligent business logic,
but edge microcontrollers independently enforce hard, unmanaged hardware
timeouts.

The scope of "sovereign" is stated precisely, because the honest version is
weaker than the tempting one: the failsafe is sovereign **at runtime**. No
command, from the hub, the user or a future cloud module, can suspend it while
the device is running. It is *not* immune to a malicious firmware update — the
same OTA path that lets us change the limits lets an attacker who owns the hub
remove them. Therefore:

- Failsafe limits are compile-time constants. They are never configurable over
  the wire: no MQTT topic, no command parameter, ever.
- The hub has no authority to install firmware that the device accepts without
  verifying a signature the hub does not hold.

Making the strong claim true requires device-side secure boot with signed OTA
and an anti-rollback counter, plus — for anything touching water — a timeout
that is not in reprogrammable software at all (a normally-closed valve with
spring return, so loss of power equals closed). Both are out of scope here and
belong to SH-005; what this ADR fixes is that the claim in this document matches
what the system actually guarantees today.

### 4. Command freshness

Freshness belongs to this decision rather than to the implementation ticket,
because half of the fix lives in firmware and firmware is expensive to change
later. An unauthenticated or stale command defeats arbitration without ever
contradicting it — it simply arrives wearing a valid badge.

- **Every command carries a `command_id` and an absolute validity window.** The
  device discards a command that is expired or already seen.
- **An ACK echoes the `command_id` of the command it answers.** The hub accepts
  an ACK only for a dispatch that is currently pending, exactly once. Without
  correlation, a device replays an old "valve closed" ACK and the hub stops
  retrying and stops alarming while the valve is open.
- **A command is never published with the retain flag.** A retained command is a
  replay generator living inside the broker: it re-executes on every reconnect,
  forever, and survives a broker restart. This is enforced by the command
  publishing API rather than by convention.

Note that the network supplies replay for free, with no attacker involved: QoS 1
plus persistent sessions plus an unstable Wi-Fi means a device can reconnect
after twenty minutes and execute a twenty-minute-old "water for 30s".

The device clock is as untrustworthy as the hub's — an ESP32 has no RTC either —
so a monotonic counter persisted in NVS is the mechanism we expect to use rather
than wall-clock comparison alone. Its resynchronization path (reflashed device,
hub restored from backup) is itself an attack surface and must require something
an attacker cannot do remotely: physical presence in a provisioning mode.

## Consequences

### Positive

- **Deterministic Command Precedence:** System components can never silently
  overwrite higher-authority decisions; user actions consistently take
  precedence over automated schedules without log forensics.
- **Hardware Protection Against Hub Failures:** If the Raspberry Pi crashes,
  loses power, or suffers a network partition while an actuator is active (e.g.,
  a water pump), the edge firmware autonomously cuts power locally, preventing
  physical damage or resource waste.
- **Clear Domain Semantics:** Splitting actions into transient Pulses and
  persistent Mode Changes prevents accidental permanent locks caused by simple
  manual interactions.
- **The two rules are independently testable.** "Does a constraint hold?" and
  "which intent wins?" are separate pure functions. Neither needs the other to
  be exercised, which is what keeps the arbitration policy from degenerating
  into nested conditionals as sources are added.
- **Arbitration cannot be bypassed, and the compiler enforces it.** Business
  modules do not receive the MQTT client: `modular.Deps` deliberately omits it,
  so publishing a command is the exclusive responsibility of `core/command`. A
  module that tries to talk to the broker directly — and thereby skip
  arbitration, provenance and auditing — does not compile. This is the part of
  the decision that does not rely on anyone remembering it.
- **The firmware failsafe bounds the damage of every replay class.** Whatever
  slips past freshness checks — a queued command delivered late, a captured
  message re-emitted, a bug in our own dispatcher — can only actuate up to the
  device's absolute hardware limit. This is the strongest argument for Part 3
  and it is worth stating explicitly: the safety guarantee does not depend on
  the correctness of the arbitration code that sits above it.
- **The command path fails closed.** A message with missing, malformed or
  unrecognized provenance is rejected and audited rather than degraded to some
  default authority. Omitting a field can no longer produce a privileged
  message.

### Negative — accepted knowingly

- **Schema Rigidity:** All internal event messages, MQTT payloads, database
  records, and API schemas must carry provenance fields (`source`, authority
  level) from the outset, increasing initial development overhead. Provenance
  cannot be retrofitted cheaply once history exists.
- **UI Complexity:** Front-end interfaces must present distinct user controls for
  single-run actions versus system mode overrides to avoid user confusion.
- **Dual State Maintenance:** Safety limits must be configured twice — flexible
  business rules on the hub and conservative hard limits flashed onto the
  microcontroller firmware. The two can drift apart, and the firmware is the one
  that is expensive to correct.
- **Timed modes depend on a clock we do not fully trust.** The Raspberry Pi has
  no RTC. After a power cut it can boot with a wrong time until NTP converges,
  so an override expiry may be evaluated against a bogus `now`. We accept this
  and constrain the blast radius: expiry is evaluated as "resume automatic",
  never as "actuate", so a wrong clock can only end a manual mode early — it can
  never open a valve. An override whose expiry passed while the hub was off is
  treated as expired on boot.
- **Two mechanisms instead of one integer comparison.** A single numeric ladder
  would be shorter to write and easier to explain in one sentence. We are paying
  extra code and extra vocabulary to avoid an ordering that cannot express the
  domain.
- **LWW survives inside a level.** Between `schedule` and `rule_engine` there is
  no principled winner today. If those two ever start disagreeing meaningfully,
  this is the first thing that breaks.
- **Provenance is bound to the channel, which constrains the transport.** A
  source cannot be asserted by a payload, so every channel must be able to
  identify its caller: authenticated sessions on the client API, and per-device
  credentials with topic ACLs on MQTT. Until that transport security exists, the
  arbitration model documented here is stronger on paper than in deployment. See
  ADR-0002 for the accepted state of the broker and its reversal trigger.
- **Freshness costs a counter, and the counter costs a recovery path.** A
  monotonic sequence in device NVS wears flash and, more importantly, needs a
  resynchronization procedure for reflashed devices and restored hubs. That
  procedure is an attack surface: it must require physical presence, which means
  device recovery is a walk to the garden rather than a remote operation.

## Revisit when

- **Multi-User Permission Hierarchy Required:** The system scales to support
  multi-tenant or role-based user access control (e.g., Admin vs. Guest
  overrides) where fixed source priority is insufficient to resolve user-vs-user
  command conflicts.
- **Level-2 conflicts become real:** `schedule` and `rule_engine` begin issuing
  contradictory commands often enough that last-write-wins between them produces
  user-visible wrong behaviour.
- **A legitimate case appears for automation to override a user**, such as a
  freeze warning or a detected leak, where a level-2 source must act against a
  standing level-3 intent. That case does not fit the current model and requires
  either a new constraint level or an explicit exception path.
- **Edge Microcontroller Memory/Processing Limits:** Connecting lightweight,
  resource-constrained legacy microcontrollers that lack programmable firmware or
  local storage capability to execute local watchdog timers.
- **An external party needs to act as hub automation** — a voice assistant, a
  webhook, the cloud module of ADR-0004. Level 2 is reserved for in-process
  components, so admitting one requires deciding its own level rather than
  reusing an existing one.
- **Signed OTA and secure boot become available on the device fleet.** At that
  point the weaker runtime-scoped sovereignty claim in Part 3 can be
  strengthened, and this ADR should be superseded rather than edited.
- **Addition of Complex Multi-Actuator Interlocks:** Automations require
  real-time, cross-device safety interlocks (e.g., Actuator A cannot run while
  Sensor B detects a condition AND Actuator C is active) that exceed what static
  edge firmware timeouts can evaluate independently.
