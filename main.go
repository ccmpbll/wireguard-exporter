package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	listenAddr := flag.String("web.listen-address", ":9586", "Address to listen on")
	metricsPath := flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	iface := flag.String("wg.interface", "", "WireGuard interface to monitor (empty = all)")
	onlineThreshold := flag.Duration("wg.online-threshold", defaultOnlineThreshold, "Max age of last handshake for peer to be considered online")
	flag.Parse()

	collector, err := newCollector(*iface, *onlineThreshold)
	if err != nil {
		log.Fatalf("failed to create collector: %v", err)
	}

	prometheus.MustRegister(collector)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><a href="` + *metricsPath + `">Metrics</a></body></html>`))
	})

	log.Printf("Listening on %s, scraping interface %q", *listenAddr, *iface)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
