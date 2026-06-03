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

const defaultOnlineThreshold = 3 * time.Minute

type peerState struct {
	rxBytes  int64
	txBytes  int64
	lastSeen time.Time
}

type collector struct {
	mu              sync.Mutex
	client          *wgctrl.Client
	iface           string
	onlineThreshold time.Duration
	prevState       map[wgtypes.Key]peerState

	// Gauges / counters
	rxBytes        *prometheus.Desc
	txBytes        *prometheus.Desc
	lastHandshake  *prometheus.Desc
	peerOnline     *prometheus.Desc
	rxBytesRate    *prometheus.Desc
	txBytesRate    *prometheus.Desc
	activePeers    *prometheus.Desc
	totalPeers     *prometheus.Desc
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
		prevState:       make(map[wgtypes.Key]peerState),

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
		rxBytesRate: prometheus.NewDesc(
			"wireguard_peer_receive_bytes_rate",
			"Bytes received from peer per second since last scrape.",
			labels, nil,
		),
		txBytesRate: prometheus.NewDesc(
			"wireguard_peer_send_bytes_rate",
			"Bytes sent to peer per second since last scrape.",
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
	ch <- c.rxBytesRate
	ch <- c.txBytesRate
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

	newState := make(map[wgtypes.Key]peerState)

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

			// Rate computation
			cur := peerState{rxBytes: peer.ReceiveBytes, txBytes: peer.TransmitBytes, lastSeen: now}
			newState[peer.PublicKey] = cur

			if prev, ok := c.prevState[peer.PublicKey]; ok {
				dt := now.Sub(prev.lastSeen).Seconds()
				if dt > 0 {
					rxRate := float64(cur.rxBytes-prev.rxBytes) / dt
					txRate := float64(cur.txBytes-prev.txBytes) / dt
					if rxRate < 0 {
						rxRate = 0
					}
					if txRate < 0 {
						txRate = 0
					}
					ch <- prometheus.MustNewConstMetric(c.rxBytesRate, prometheus.GaugeValue, rxRate, lbls...)
					ch <- prometheus.MustNewConstMetric(c.txBytesRate, prometheus.GaugeValue, txRate, lbls...)
				}
			}
		}

		ch <- prometheus.MustNewConstMetric(c.activePeers, prometheus.GaugeValue, float64(active), dev.Name)
		ch <- prometheus.MustNewConstMetric(c.totalPeers, prometheus.GaugeValue, float64(total), dev.Name)
	}

	c.prevState = newState
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
