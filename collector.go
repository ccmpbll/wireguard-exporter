package main

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const defaultOnlineThreshold = 5 * time.Minute

type collector struct {
	mu              sync.Mutex
	client          *wgctrl.Client
	ifaces          []string
	onlineThreshold time.Duration

	// Per-peer metrics
	rxBytes       *prometheus.Desc
	txBytes       *prometheus.Desc
	handshakeAge  *prometheus.Desc
	peerOnline    *prometheus.Desc
	activePeers   *prometheus.Desc
	totalPeers    *prometheus.Desc

	// Per-interface metrics
	ifaceRxBytes   *prometheus.Desc
	ifaceTxBytes   *prometheus.Desc
	ifaceRxPackets *prometheus.Desc
	ifaceTxPackets *prometheus.Desc
	ifaceRxErrors  *prometheus.Desc
	ifaceTxErrors  *prometheus.Desc
	ifaceRxDropped *prometheus.Desc
	ifaceTxDropped *prometheus.Desc
}

func newCollector(ifaces []string, onlineThreshold time.Duration) (*collector, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	peerLabels := []string{"interface", "public_key", "endpoint"}
	ifaceLabels := []string{"interface"}

	return &collector{
		client:          client,
		ifaces:          ifaces,
		onlineThreshold: onlineThreshold,

		rxBytes: prometheus.NewDesc(
			"wireguard_peer_received_bytes_total",
			"Total bytes received from peer.",
			peerLabels, nil,
		),
		txBytes: prometheus.NewDesc(
			"wireguard_peer_sent_bytes_total",
			"Total bytes sent to peer.",
			peerLabels, nil,
		),
		handshakeAge: prometheus.NewDesc(
			"wireguard_peer_last_handshake_seconds",
			"Seconds since last handshake with peer.",
			peerLabels, nil,
		),
		peerOnline: prometheus.NewDesc(
			"wireguard_peer_online",
			"1 if peer has handshaked within the online threshold, 0 otherwise.",
			peerLabels, nil,
		),
		activePeers: prometheus.NewDesc(
			"wireguard_active_peers",
			"Number of peers online (handshaked within threshold).",
			ifaceLabels, nil,
		),
		totalPeers: prometheus.NewDesc(
			"wireguard_total_peers",
			"Total number of configured peers.",
			ifaceLabels, nil,
		),

		ifaceRxBytes: prometheus.NewDesc(
			"wireguard_interface_received_bytes_total",
			"Total bytes received on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceTxBytes: prometheus.NewDesc(
			"wireguard_interface_sent_bytes_total",
			"Total bytes sent on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceRxPackets: prometheus.NewDesc(
			"wireguard_interface_received_packets_total",
			"Total packets received on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceTxPackets: prometheus.NewDesc(
			"wireguard_interface_sent_packets_total",
			"Total packets sent on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceRxErrors: prometheus.NewDesc(
			"wireguard_interface_receive_errors_total",
			"Total receive errors on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceTxErrors: prometheus.NewDesc(
			"wireguard_interface_transmit_errors_total",
			"Total transmit errors on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceRxDropped: prometheus.NewDesc(
			"wireguard_interface_receive_drops_total",
			"Total dropped inbound packets on the WireGuard interface.",
			ifaceLabels, nil,
		),
		ifaceTxDropped: prometheus.NewDesc(
			"wireguard_interface_transmit_drops_total",
			"Total dropped outbound packets on the WireGuard interface.",
			ifaceLabels, nil,
		),
	}, nil
}

func (c *collector) Close() {
	c.client.Close()
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.rxBytes
	ch <- c.txBytes
	ch <- c.handshakeAge
	ch <- c.peerOnline
	ch <- c.activePeers
	ch <- c.totalPeers
	ch <- c.ifaceRxBytes
	ch <- c.ifaceTxBytes
	ch <- c.ifaceRxPackets
	ch <- c.ifaceTxPackets
	ch <- c.ifaceRxErrors
	ch <- c.ifaceTxErrors
	ch <- c.ifaceRxDropped
	ch <- c.ifaceTxDropped
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	devices, err := c.devices()
	if err != nil {
		slog.Error("error reading WireGuard devices", "err", err)
		return
	}

	ifaceStats, err := readIfaceStats()
	if err != nil {
		slog.Error("error reading interface stats", "err", err)
	}

	for _, dev := range devices {
		active := 0
		total := len(dev.Peers)

		for _, peer := range dev.Peers {
			endpoint := endpointStr(peer.Endpoint)
			pubKey := peer.PublicKey.String()
			lbls := []string{dev.Name, pubKey, endpoint}

			ch <- prometheus.MustNewConstMetric(c.rxBytes, prometheus.CounterValue, float64(peer.ReceiveBytes), lbls...)
			ch <- prometheus.MustNewConstMetric(c.txBytes, prometheus.CounterValue, float64(peer.TransmitBytes), lbls...)

			if !peer.LastHandshakeTime.IsZero() {
				age := now.Sub(peer.LastHandshakeTime).Seconds()
				ch <- prometheus.MustNewConstMetric(c.handshakeAge, prometheus.GaugeValue, age, lbls...)
			}

			online := 0.0
			if !peer.LastHandshakeTime.IsZero() && now.Sub(peer.LastHandshakeTime) <= c.onlineThreshold {
				online = 1.0
				active++
			}
			ch <- prometheus.MustNewConstMetric(c.peerOnline, prometheus.GaugeValue, online, lbls...)
		}

		ch <- prometheus.MustNewConstMetric(c.activePeers, prometheus.GaugeValue, float64(active), dev.Name)
		ch <- prometheus.MustNewConstMetric(c.totalPeers, prometheus.GaugeValue, float64(total), dev.Name)

		if stats, ok := ifaceStats[dev.Name]; ok {
			ch <- prometheus.MustNewConstMetric(c.ifaceRxBytes, prometheus.CounterValue, float64(stats.RxBytes), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceRxPackets, prometheus.CounterValue, float64(stats.RxPackets), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceRxErrors, prometheus.CounterValue, float64(stats.RxErrors), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceRxDropped, prometheus.CounterValue, float64(stats.RxDropped), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxBytes, prometheus.CounterValue, float64(stats.TxBytes), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxPackets, prometheus.CounterValue, float64(stats.TxPackets), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxErrors, prometheus.CounterValue, float64(stats.TxErrors), dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxDropped, prometheus.CounterValue, float64(stats.TxDropped), dev.Name)
		}
	}
}

// readIfaceStats reads per-interface counters from /proc/net/dev via procfs.
func readIfaceStats() (procfs.NetDev, error) {
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return nil, err
	}
	return fs.NetDev()
}

func (c *collector) devices() ([]*wgtypes.Device, error) {
	if len(c.ifaces) == 0 {
		return c.client.Devices()
	}
	var devices []*wgtypes.Device
	for _, iface := range c.ifaces {
		dev, err := c.client.Device(iface)
		if err != nil {
			return nil, err
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func endpointStr(ep *net.UDPAddr) string {
	if ep == nil {
		return ""
	}
	return ep.String()
}
