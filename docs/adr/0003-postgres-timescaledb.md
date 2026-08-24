# ADR-0003: PostgreSQL + TimescaleDB as the single store

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** Abel Pinheiro
- 
## Context

The hub needs to define how and where it will store its data. There will be relational data such as user data and for each of the modules, but there will also need to have time series data to store raw sensor data. These are two different types of databases, how we will store them ? 

## Options considered

### Option A — PostgreSQL + TimescaleDB

Postgres is a popular open source SD. TimescaleDB is an open-source time-series database packaged as a relational PostgreSQL extension. It uses automatic time-based partitioning, native compression, and custom SQL functions to handle massive volumes of timestamped data like IoT telemetry. The positive here is that it allows us to store all the data in a single system. 

### Option B — Dual-Store Architecture (PostgreSQL + InfluxDB / Prometheus)

Using PostgreSQL for relational entities/configurations and a specialized time-series database (like InfluxDB) dedicated strictly to sensor telemetry.

Trade-off: Provides best-in-class time-series ingestion and compression out of the box, but significantly increases RAM usage and CPU overhead on the Raspberry Pi. It forces developer burden to handle dual-write consistency, run two distinct database daemons, maintain separate backup/restore strategies, and perform cross-database queries in application code.

### Option C - AWS

The whole ecosystem will be aws-centered. THat way, we can use all of the AWS functions and strong points for all of the data. the downside is that will cost more. AWS already contains components that allows the user to handle IoT data like RDS, Amazon TimeStream and Aurora. the downside is that we will need more places to code, more infrastructure to handle. It will cost more than option A as well.

## Decision

We will use PostgreSQL + TimescaleDB. 

## Consequences

### Positive

- A single database process to install, run, back up, and monitor on the Raspberry Pi, drastically reducing background RAM and CPU consumption compared to running multiple database engines.
- Telemetry data (hypertables) can be joined directly with device metadata and automation configuration tables in standard SQL queries without application-level data stitching.

### Negative — accepted knowingly

- Heavy time-series write batches or intensive analytic aggregation queries can consume CPU/IOPS that temporarily impact relational query latency.
- While TimescaleDB supports background compression, its raw uncompressed insert path still incurs standard PostgreSQL write-ahead log (WAL) overhead compared to ultra-lean, specialized time-series engines like InfluxDB TSM or VictoriaMetrics.
- Requires compiling or pulling PostgreSQL image builds bundled with timescaledb, slightly complicating custom base OS images for the Raspberry Pi.

## Revisit when

- The combined memory/CPU usage of PostgreSQL + TimescaleDB under heavy telemetry ingestion starves core hub automation services on a low-RAM Raspberry Pi board.
- Ingestion rate exceeds 2,000 writes/second or the connected device count scale causes hypertables to outgrow local storage bounds despite retention/compression policies.
- The project pivots from a local edge hub model to a multi-node or cloud-connected architecture where splitting microservices into independent, polyglot databases becomes architecturally desirable.
