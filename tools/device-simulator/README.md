# Device Simulator

Lands in **SH-003**.

Simulates IoT devices against the real broker: publishes telemetry on an
interval, responds to commands with ACKs, and — most importantly — reproduces
the failure modes that real hardware will not produce on demand:

- network drops mid-command
- lost ACKs
- clock skew between device and hub
- a device that stops reporting without disconnecting cleanly

Until then, `make smoke` uses `mosquitto_pub` to validate the ingestion path.
