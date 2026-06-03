#!/bin/sh
set -e
systemctl stop wireguard-exporter || true
systemctl disable wireguard-exporter || true
