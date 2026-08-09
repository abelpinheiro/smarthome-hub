# ADR-0001: Event-driven modular monolith in Go

- **Status:** Accepted
- **Date:** 2026-08-09
- **Deciders:** Abel Pinheiro

## Context

This system will be built and maintained by a single developer whose stated
goal is to deepen software architecture skills, not to operate infrastructure.
Whatever we choose has to spend that developer's time on domain design.

The product requires modularity by definition: irrigation, lighting and future
modules must evolve independently. That requirement is not negotiable.

The deployment target is a Raspberry Pi in a house. Memory and CPU are hard
constraints, and the process must survive power cuts and flaky Wi-Fi.

The domain is inherently asynchronous: devices publish when they have something
to report, not when we ask. Scaling requirements are unknown. This may serve
one house or a hundred, and we will not know for months.

## Options considered

### Option A — Microservices

Each capability is a separately deployed application with its own database,
communicating over the network. The system exists only as the composition of
those services.

The cost is not the difficulty of starting — it is continuous. Eventual
consistency, sagas and compensating transactions, distributed tracing, network
partition handling and contract versioning between services are paid on every
feature, forever. A single developer pays that toll alone, and it comes out of
the time budget that was supposed to go to domain design.

### Option B — Classic layered monolith

A single application divided into three technical layers: controller, service
and repository. It is the fastest structure to start writing code in.

The problem is that the layers are technical, not functional: irrigation logic
ends up spread across all three, tangled with lighting logic. Modularity — a
non-negotiable product requirement here — becomes impractical, and the domain
being asynchronous means we would be fighting the request/response model from
day one.

### Option C — Event-driven modular monolith

A single deployable application divided by business capability rather than by
technical layer. Each module owns its domain and communicates with the others
only through an event bus, never by direct import.

It gives up the independent scalability of Option A, so it inherits the
scaling and fault-isolation limits of any monolith.

## Decision

We will build an event-driven modular monolith in Go (Option C).

It lets us deliver an initial working version that is scalable enough for what
we need today, while keeping the components untangled. If we later decide to
move to microservices, the boundaries already exist and modules can be
extracted without rewriting domain logic.

Modularity is enforced by the compiler, not by convention: each module keeps
its domain under a nested `internal/` package, which Go forbids other modules
from importing. A boundary violation is a build error, not a code review note.

**Why event-driven specifically.** It is doing two jobs at once. First, the
domain demands it: devices publish when they have something to report, so a
request/response core would force us to poll. Second, and more importantly, it
is the mechanism that keeps modules from importing each other. Publishing to a
bus instead of calling a neighbour is what makes future extraction viable —
without it, "modular monolith" is just a monolith with tidy folders.

**Why Go.** The Raspberry Pi target makes runtime footprint a hard constraint.
Go produces a single static binary of roughly 15 MB with no runtime to install,
which .NET and the JVM do not match at that memory budget. Its concurrency
model also fits an MQTT workload of many long-lived connections naturally.

## Consequences

### Positive

- Boundaries exist from day one, so extracting a module later is viable without
  rewriting domain logic.
- A working product arrives fast, and the developer's time goes to domain
  design rather than distributed systems plumbing.
- One deployable means one thing to build, ship, back up and debug — which
  matters when a house depends on it running unattended.
- The compiler enforces module isolation, so the architecture cannot silently
  erode.

### Negative — accepted knowingly

- **No fault isolation between modules.** A panic or a memory leak in telemetry
  ingestion takes down command dispatch with it. The valve stops responding
  because of a bug in a humidity chart.
- **Coupled scaling.** If ingestion needs more CPU, we scale the whole process,
  including the parts that are idle.
- **Coupled deployment.** Changing an irrigation rule requires redeploying the
  command dispatcher, which is the safety-critical component.
- **Technology lock-in.** Every module must be written in Go. A future module
  that would be better served by another ecosystem does not fit.
- **Synchronous event delivery.** The current `bus.InProcess` implementation
  delivers events in sequence, so a slow handler blocks the publisher. There is
  also no durability: an event is lost if the process dies between publish and
  handling. Acceptable for telemetry, deliberately not acceptable for commands,
  which get their own durable path.

## Revisit when

- A single hub instance manages more than 200 devices, or ingestion exceeds
  1000 messages per second.
- Any single module's failure profile or scaling profile diverges materially
  from the rest — that module is the first extraction candidate.
- More than one person works on the codebase at the same time, making
  deployment coupling a coordination cost rather than a personal one.
