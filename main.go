package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	port            := flag.String("exporter_port", ":9586", "Address to listen on")
	ifacesFlag      := flag.String("interfaces", "", "Comma-separated list of WireGuard interfaces to monitor (empty = all)")
	onlineThreshold := flag.Duration("online_threshold", defaultOnlineThreshold, "Max age of last handshake for peer to be considered online")
	flag.Parse()

	var ifaces []string
	if *ifacesFlag != "" {
		for _, s := range strings.Split(*ifacesFlag, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				ifaces = append(ifaces, s)
			}
		}
	}

	collector, err := newCollector(ifaces, *onlineThreshold)
	if err != nil {
		log.Fatalf("failed to create collector: %v", err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><a href="/metrics">Metrics</a></body></html>`))
	})

	log.Printf("Listening on %s, monitoring interfaces: %v", *port, ifaces)
	log.Fatal(http.ListenAndServe(*port, nil))
}
