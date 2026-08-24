# Security Policy

This project controls physical devices in people's homes — valves, pumps,
lighting. A defect here is not a wrong pixel; it can flood a room or leave a
house dark. Security is treated accordingly.

## Reporting a vulnerability

Please report security issues privately through
[GitHub Security Advisories](https://github.com/abelpinheiro/smarthome-hub/security/advisories/new),
not as a public issue.

Expect an acknowledgement within 7 days. Please allow a reasonable window for a
fix before public disclosure. Reports are welcome regardless of severity, and
credit is given unless you prefer otherwise.

## Threat model summary

The design assumes the following adversaries:

| Adversary | Assumption | Mitigation |
|---|---|---|
| **Compromised device** | One device *will* be compromised — stale firmware, key extracted from flash, unit physically stolen from a garden. | Per-device credentials and per-device broker ACLs. A device may publish only under its own topic path and subscribe only to its own commands. |
| **Network attacker on the LAN** | Guest Wi-Fi, an IoT device with a known CVE, a neighbour. | Mandatory broker authentication (no anonymous access, ever). Database bound to loopback only. **Known gaps, tracked and not yet closed:** the broker listener is still plaintext, so credentials travel in clear in the CONNECT packet (accepted debt with a reversal trigger in ADR-0002, ticket SH-007), and the hub's HTTP API on `:8080` has no authentication yet. Neither is safe to rely on today. |
| **Internet attacker** | Automated scanning of residential IPs. | **Nothing is reachable from the internet**: no port is forwarded at the router, and the hub never opens a socket outside the LAN. It does listen *on the LAN* — HTTP on `:8080`, broker on `:1883` — so those surfaces belong to the row above, not to this one. Any future cloud connectivity is outbound-only, initiated by the hub. Port-forwarding an MQTT broker is explicitly out of scope and will not be supported. |
| **Repository reader** | The code is public; assume adversaries read it. | Security depends only on secrets, never on the design being hidden (Kerckhoffs's principle). No credential, key or instance identifier lives in this repository. |
| **Buggy automation** | Our own code will have bugs. | The failsafe layer sits in firmware/hardware and is sovereign **at runtime**: no command from the hub, the user or a future cloud module can suspend it while the device is running. It is not immune to a malicious firmware update — see ADR-0006, which scopes this claim precisely and states what would be required to strengthen it. |

## What never enters this repository

- Credentials, tokens, private keys, certificates
- Real device IDs, home IDs, MAC addresses
- Wi-Fi SSIDs or passwords (including inside firmware samples)
- Residential IPs, DDNS hostnames, geolocation
- **Real telemetry data** — irrigation and lighting histories are presence data.
  They reveal when a house was empty and predict when it will be again. Demos
  and screenshots use the device simulator, never a live installation.

Two independent barriers enforce this: a `gitleaks` pre-commit hook
(client-side, install with `make hooks`) and GitHub Push Protection
(server-side, cannot be bypassed with `--no-verify`).

## Deploying this yourself

If you run this in your own home:

1. Run `make secrets` — never reuse the example values from `.env.example`.
2. Keep the broker on your LAN. Do not port-forward it. For remote access, use
   a VPN (WireGuard/Tailscale) until the cloud module ships.
3. Give every device its own credential. Shared credentials mean one
   compromised device exposes the whole house.
4. Keep `.env` and `deployments/mosquitto/passwd` out of any backup that leaves
   your network unencrypted.

## Supported versions

The project is pre-1.0. Security fixes are applied to `main` only.
