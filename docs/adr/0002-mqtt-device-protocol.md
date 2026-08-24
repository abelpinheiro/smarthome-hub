# ADR-0002: MQTT as the device-facing protocol

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** Abel Pinheiro

## Context

We are building a local smart home hub deployed on resource-constrained hardware (Raspberry Pi) that acts as a central coordinator for various IoT end-devices (sensors, actuators, and smart appliances).

The edge devices range from mains-powered microcontrollers to low-power, battery-operated sensors communicating over local wireless networks. The system requires a messaging protocol that features extremely low overhead, minimal CPU/memory footprint on end-nodes, efficient power consumption, and support for asynchronous bi-directional communication (publish/subscribe pattern) with reliable QoS levels over potentially unstable network connections.

## Options considered

### Option A — MQTT

MQTT stands for Message Queuing Telemetry Transport. It is a lightweight protocol very commonly used in IOT due to its low overhead and native support support in low-cost microcontrollers. The consequences is that it requires managing a local MQTT broker on the Raspberry Pi and payload structure must be defined separately.

### Option B — HTTP/ REST API

Standard request-response over TCP. It is easy to debug, and simple to implement on web-facing platforms. However, there's high HTTP header overhead per payload, and lacks efficient push for real time sensor updates.

### Option C - WebSockets

It is a full-duplex persistent TCP connections. It has a low overhead once connected and allows real-time bi-directional streaming. However, there's high connection setup/maintenance overhead on low-power battery devices and lacks native pub/sub routing.

## Decision

We will use MQTT. 

## Consequences

### Positive

- Native support in low-cost microcontrollers and established local brokers (e.g., Mosquitto running on the Raspberry Pi).
- Binary header (as small as 2 bytes) minimizes network usage and power draw on microcontrollers.

### Negative — accepted knowingly

- Requires managing a local MQTT broker on the Raspberry Pi.
- Unencrypted MQTT (port 1883) requires securing the local network or managing TLS certificates/MQTTS on constrained devices, which adds hardware overhead.

  **Accepted debt, with an explicit reversal trigger.** The broker currently
  runs a plaintext listener on 1883 bound to all interfaces. This means MQTT
  credentials travel in clear text inside the CONNECT packet, so anyone with
  passive presence on the home Wi-Fi — a guest network, a cheap bulb with a
  known CVE, a neighbour who has the PSK — can capture them and then publish
  commands, forge telemetry and forge ACKs for any device. "The LAN is trusted"
  is a weak premise precisely here: under WPA2-PSK every device that knows the
  passphrase can decrypt the others' traffic given the 4-way handshake, which a
  deauth forces.

  We accept this **only while no physical device exists on the network**, which
  is true today: the sole credential is the hub's own, and the only publisher is
  a simulator on loopback. The reversal trigger is therefore not a date and not
  a metric — it is an event: **before the first physical device joins the LAN**,
  the broker must expose a TLS listener on 8883 backed by a local CA, and 1883
  must stop being published. Freshness and replay controls (ADR-0006 §4) are
  worth nothing while the channel itself is sniffable, so this comes first.

  The known costs of paying it, so the future reader is not surprised:
  certificate lifecycle on devices without an RTC (a wrong clock at boot breaks
  `notBefore`/`notAfter` validation, forcing either SNTP-before-TLS or long-lived
  certificates with relaxed time validation), custody of the CA key becoming our
  responsibility, and TLS handshakes costing battery on sensors. The step beyond,
  when strong device identity is wanted, is mTLS with `require_certificate true`
  and `use_identity_as_username true`, which makes channel provenance
  cryptographic instead of a shareable password.
- MQTT only transports raw bytes; payload formats (e.g., JSON, Protocol Buffers) and topic hierarchies must be defined.

## Revisit when

- **Before the first physical device joins the LAN** — the plaintext listener
  must be replaced by TLS on 8883 with a local CA. See the accepted debt above;
  this is the highest-priority security item in the backlog.
- The number of connected local nodes or message throughput exceeds the resource limits or connection pool capacity of a single Raspberry Pi-hosted Mosquitto broker.
- Extremely battery-sensitive remote sensors are added that cannot sustain persistent TCP connections required by MQTT and demand UDP-based protocols.