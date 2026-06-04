package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	defer collector.Close()

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><a href="/metrics">Metrics</a></body></html>`))
	})

	srv := &http.Server{
		Addr:    *port,
		Handler: mux,
	}

	go func() {
		log.Printf("Listening on %s, monitoring interfaces: %v", *port, ifaces)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
}
