#!/bin/sh
set -e
systemctl daemon-reload
systemctl enable wireguard-exporter
systemctl start wireguard-exporter
