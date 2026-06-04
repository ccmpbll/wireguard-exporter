package main

import (
	"log"
	"net"
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

	rxBytes       *prometheus.Desc
	txBytes       *prometheus.Desc
	lastHandshake *prometheus.Desc
	peerOnline    *prometheus.Desc
	activePeers   *prometheus.Desc
	totalPeers    *prometheus.Desc
}

func newCollector(iface string, onlineThreshold time.Duration) (*collector, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	labels := []string{"interface", "public_key", "endpoint"}

	return &collector{
		client:          client,
		iface:           iface,
		onlineThreshold: onlineThreshold,

		rxBytes: prometheus.NewDesc(
			"wireguard_peer_received_bytes_total",
			"Total bytes received from peer.",
			labels, nil,
		),
		txBytes: prometheus.NewDesc(
			"wireguard_peer_sent_bytes_total",
			"Total bytes sent to peer.",
			labels, nil,
		),
		lastHandshake: prometheus.NewDesc(
			"wireguard_peer_last_handshake_seconds",
			"Unix timestamp of last handshake with peer.",
			labels, nil,
		),
		peerOnline: prometheus.NewDesc(
			"wireguard_peer_online",
			"1 if peer has handshaked within the online threshold, 0 otherwise.",
			labels, nil,
		),
		activePeers: prometheus.NewDesc(
			"wireguard_active_peers",
			"Number of peers online (handshaked within threshold).",
			[]string{"interface"}, nil,
		),
		totalPeers: prometheus.NewDesc(
			"wireguard_total_peers",
			"Total number of configured peers.",
			[]string{"interface"}, nil,
		),
	}, nil
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.rxBytes
	ch <- c.txBytes
	ch <- c.lastHandshake
	ch <- c.peerOnline
	ch <- c.activePeers
	ch <- c.totalPeers
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
	}
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
