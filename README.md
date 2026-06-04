# wireguard-exporter

![Build Status](https://img.shields.io/github/actions/workflow/status/ccmpbll/wireguard-exporter/release.yml) ![Latest Release](https://img.shields.io/github/v/release/ccmpbll/wireguard-exporter) ![Go Version](https://img.shields.io/github/go-mod/go-version/ccmpbll/wireguard-exporter) ![Docker Image Size](https://img.shields.io/docker/image-size/ccmpbll/wireguard-exporter/latest) ![Docker Pulls](https://img.shields.io/docker/pulls/ccmpbll/wireguard-exporter) ![License](https://img.shields.io/badge/License-MIT-blue.svg)

A Prometheus exporter for WireGuard. Uses [`wgctrl`](https://pkg.go.dev/golang.zx2c4.com/wireguard/wgctrl) to read peer state via netlink directly from the WireGuard kernel module — no `wg show` subprocess, no parsing.

## Metrics

### Per-peer

| Metric | Type | Description |
|--------|------|-------------|
| `wireguard_peer_received_bytes_total` | Counter | Total bytes received from peer |
| `wireguard_peer_sent_bytes_total` | Counter | Total bytes sent to peer |
| `wireguard_peer_last_handshake_seconds` | Gauge | Seconds since last handshake |
| `wireguard_peer_online` | Gauge | 1 if peer handshaked within threshold, 0 otherwise |

All per-peer metrics are labeled with `interface`, `public_key`, and `endpoint`.

Bandwidth rates can be derived using `rate(wireguard_peer_received_bytes_total[5m])` and `rate(wireguard_peer_sent_bytes_total[5m])`.

### Per-interface

| Metric | Type | Description |
|--------|------|-------------|
| `wireguard_interface_received_bytes_total` | Counter | Total bytes received on interface |
| `wireguard_interface_sent_bytes_total` | Counter | Total bytes sent on interface |
| `wireguard_interface_received_packets_total` | Counter | Total packets received on interface |
| `wireguard_interface_sent_packets_total` | Counter | Total packets sent on interface |
| `wireguard_interface_receive_errors_total` | Counter | Total receive errors on interface |
| `wireguard_interface_transmit_errors_total` | Counter | Total transmit errors on interface |
| `wireguard_interface_receive_drops_total` | Counter | Total dropped inbound packets |
| `wireguard_interface_transmit_drops_total` | Counter | Total dropped outbound packets |

### Summary

| Metric | Type | Description |
|--------|------|-------------|
| `wireguard_active_peers` | Gauge | Peers online (within threshold) |
| `wireguard_total_peers` | Gauge | Total configured peers |

## Usage

```
wireguard-exporter [flags]

Flags:
  --exporter_port      Address to listen on (default: :9586)
  --interfaces         Comma-separated list of WireGuard interfaces to monitor (default: all)
  --online_threshold   Max age of last handshake to consider peer online (default: 5m)
```

Metrics are always served at `/metrics`.

The exporter needs `CAP_NET_ADMIN` to read WireGuard state.

## Running with Docker

```bash
docker run -d \
  --name wireguard-exporter \
  --cap-add NET_ADMIN \
  --network host \
  ccmpbll/wireguard-exporter \
  --interfaces=wg0
```

## Running as a systemd service

A sample unit file is included at [`wireguard-exporter.service`](wireguard-exporter.service). It uses `DynamicUser` (systemd creates a temporary unprivileged user at runtime — no need to create a system user manually) and grants only `CAP_NET_ADMIN`.

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

## Dependencies

| Module | Purpose |
|--------|---------|
| [`golang.zx2c4.com/wireguard/wgctrl`](https://pkg.go.dev/golang.zx2c4.com/wireguard/wgctrl) | Reads WireGuard peer state via netlink |
| [`github.com/prometheus/client_golang`](https://pkg.go.dev/github.com/prometheus/client_golang) | Prometheus metrics exposition |

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: wireguard
    static_configs:
      - targets: ['<host>:9586']
```

