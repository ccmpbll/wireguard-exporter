# wireguard-exporter

A Prometheus exporter for WireGuard. Reads peer state directly from the kernel via netlink — no `wg show` subprocess.

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `wireguard_peer_received_bytes_total` | Counter | Total bytes received from peer |
| `wireguard_peer_sent_bytes_total` | Counter | Total bytes sent to peer |
| `wireguard_peer_last_handshake_seconds` | Gauge | Unix timestamp of last handshake |
| `wireguard_peer_online` | Gauge | 1 if peer handshaked within threshold, 0 otherwise |
| `wireguard_peer_receive_bytes_rate` | Gauge | Bytes/sec received since last scrape |
| `wireguard_peer_send_bytes_rate` | Gauge | Bytes/sec sent since last scrape |
| `wireguard_active_peers` | Gauge | Peers online (within threshold) |
| `wireguard_total_peers` | Gauge | Total configured peers |

All per-peer metrics are labeled with `interface`, `public_key`, and `endpoint`.

## Usage

```
wireguard-exporter [flags]

Flags:
  --web.listen-address   Address to listen on (default: :9586)
  --web.telemetry-path   Path to expose metrics (default: /metrics)
  --wg.interface         WireGuard interface to monitor (default: all interfaces)
  --wg.online-threshold  Max age of last handshake to consider peer online (default: 3m)
```

The exporter needs `CAP_NET_ADMIN` to read WireGuard state.

## Running with Docker

```bash
docker run -d \
  --name wireguard-exporter \
  --cap-add NET_ADMIN \
  --network host \
  ccmpbll/wireguard-exporter \
  --wg.interface=wg0
```

## Running as a systemd service

A sample unit file is included at [`wireguard-exporter.service`](wireguard-exporter.service). It uses `DynamicUser` and grants only `CAP_NET_ADMIN`.

```bash
cp wireguard-exporter.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now wireguard-exporter
```

## Building from source

Requires Go 1.22+.

```bash
go mod tidy
make build          # produces wireguard-exporter (linux/amd64)
make deploy         # build + scp to your host + systemctl restart
```

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: wireguard
    static_configs:
      - targets: ['<host>:9586']
```

## Releases

Tagged releases are built automatically via GitHub Actions. Each release publishes:
- A pre-built `wireguard-exporter-linux-amd64` binary attached to the GitHub release
- A Docker image pushed to [Docker Hub](https://hub.docker.com/r/ccmpbll/wireguard-exporter)
