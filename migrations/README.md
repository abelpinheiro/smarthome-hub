# Migrations

Goose migrations land in **SH-002**, together with the device registry schema.

The TimescaleDB extension itself is created by the `timescale/timescaledb`
image at first boot; the hub's `/health` endpoint verifies it is present, so a
misconfigured database fails loudly at startup instead of silently at the first
hypertable.
