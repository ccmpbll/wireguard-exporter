package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const defaultOnlineThreshold = 5 * time.Minute

type collector struct {
	mu              sync.Mutex
	client          *wgctrl.Client
	iface           string
	onlineThreshold time.Duration

	// Per-peer metrics
	rxBytes        *prometheus.Desc
	txBytes        *prometheus.Desc
	lastHandshake  *prometheus.Desc
	handshakeAge   *prometheus.Desc
	peerOnline     *prometheus.Desc
	activePeers    *prometheus.Desc
	totalPeers     *prometheus.Desc

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

func newCollector(iface string, onlineThreshold time.Duration) (*collector, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	peerLabels := []string{"interface", "public_key", "endpoint"}
	ifaceLabels := []string{"interface"}

	return &collector{
		client:          client,
		iface:           iface,
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
		lastHandshake: prometheus.NewDesc(
			"wireguard_peer_last_handshake_seconds",
			"Unix timestamp of last handshake with peer.",
			peerLabels, nil,
		),
		handshakeAge: prometheus.NewDesc(
			"wireguard_peer_last_handshake_age_seconds",
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

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.rxBytes
	ch <- c.txBytes
	ch <- c.lastHandshake
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
		log.Printf("error reading WireGuard devices: %v", err)
		return
	}

	ifaceStats, err := readIfaceStats()
	if err != nil {
		log.Printf("error reading interface stats: %v", err)
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

			var handshakeTS float64
			if !peer.LastHandshakeTime.IsZero() {
				handshakeTS = float64(peer.LastHandshakeTime.Unix())
				age := now.Sub(peer.LastHandshakeTime).Seconds()
				ch <- prometheus.MustNewConstMetric(c.handshakeAge, prometheus.GaugeValue, age, lbls...)
			}
			ch <- prometheus.MustNewConstMetric(c.lastHandshake, prometheus.GaugeValue, handshakeTS, lbls...)

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
			ch <- prometheus.MustNewConstMetric(c.ifaceRxBytes, prometheus.CounterValue, stats[0], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceRxPackets, prometheus.CounterValue, stats[1], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceRxErrors, prometheus.CounterValue, stats[2], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceRxDropped, prometheus.CounterValue, stats[3], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxBytes, prometheus.CounterValue, stats[4], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxPackets, prometheus.CounterValue, stats[5], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxErrors, prometheus.CounterValue, stats[6], dev.Name)
			ch <- prometheus.MustNewConstMetric(c.ifaceTxDropped, prometheus.CounterValue, stats[7], dev.Name)
		}
	}
}

// readIfaceStats parses /proc/net/dev and returns a map of interface name to
// [rxBytes, rxPackets, rxErrors, rxDropped, txBytes, txPackets, txErrors, txDropped].
func readIfaceStats() (map[string][8]float64, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stats := make(map[string][8]float64)
	scanner := bufio.NewScanner(f)

	// Skip two header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colonIdx])
		fields := strings.Fields(line[colonIdx+1:])
		if len(fields) < 16 {
			continue
		}

		var vals [8]float64
		indices := []int{0, 1, 2, 3, 8, 9, 10, 11} // rx: bytes packets errs drop; tx: bytes packets errs drop
		for i, idx := range indices {
			v, err := strconv.ParseFloat(fields[idx], 64)
			if err != nil {
				continue
			}
			vals[i] = v
		}
		stats[name] = vals
	}

	return stats, scanner.Err()
}

func (c *collector) devices() ([]*wgtypes.Device, error) {
	if c.iface != "" {
		dev, err := c.client.Device(c.iface)
		if err != nil {
			return nil, err
		}
		return []*wgtypes.Device{dev}, nil
	}
	return c.client.Devices()
}

func endpointStr(ep *net.UDPAddr) string {
	if ep == nil {
		return ""
	}
	return ep.String()
}
