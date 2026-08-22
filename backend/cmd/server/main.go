package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nexus-feed/backend/pkg/conflation"
	"nexus-feed/backend/pkg/engine"
	"nexus-feed/backend/pkg/ingestion"
	"nexus-feed/backend/pkg/stream"
)

func main() {
	log.Println("==========================================================")
	log.Println("  ⚡ NEXUS-FEED: High-Frequency Cross-Exchange Market Engine")
	log.Println("==========================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize WebSocket Hub
	wsHub := stream.NewHub()
	go wsHub.Run(ctx)
	log.Println("[Stream] WebSocket Hub initialized and running.")

	// 2. Initialize Conflation Engine (50ms / 20 FPS flush interval)
	conflator := conflation.NewConflator(50*time.Millisecond, wsHub.HandleConflatedBatch)
	go conflator.Start(ctx)
	log.Println("[Conflation] Conflation Engine active (50ms / 20 FPS flush interval).")

	// 3. Initialize Engine Dispatcher with 64K queue
	dispatcher := engine.NewDispatcher(65536, conflator.Push)
	go dispatcher.Start(ctx)
	log.Println("[Engine] Dispatcher and Consolidated SkipList Books active.")

	// 4. Start live exchange feeds
	symbols := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT"}
	binanceAdapter := ingestion.NewBinanceAdapter(symbols, dispatcher)
	coinbaseAdapter := ingestion.NewCoinbaseAdapter(symbols, dispatcher)

	go binanceAdapter.Start(ctx)
	go coinbaseAdapter.Start(ctx)
	log.Printf("[Ingestion] Live feeds active for Binance & Coinbase. Symbols: %v\n", symbols)

	// 5. Configure HTTP & WebSocket routes
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsHub.ServeWS(w, r)
	})

	// Loader.io domain verification
	loaderVerification := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("loaderio-07d7114154811d522ccb87b78f736542"))
	}
	mux.HandleFunc("/loaderio-07d7114154811d522ccb87b78f736542.txt", loaderVerification)
	mux.HandleFunc("/loaderio-07d7114154811d522ccb87b78f736542/", loaderVerification)
	mux.HandleFunc("/loaderio-07d7114154811d522ccb87b78f736542", loaderVerification)

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"system": "nexus-feed",
		})
	})

	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		engineMetrics := dispatcher.GetMetrics()
		inputs, outputs, arbs, ratio := conflator.GetMetrics()

		metricsPayload := map[string]interface{}{
			"engine": engineMetrics,
			"conflation": map[string]interface{}{
				"raw_inputs_received": inputs,
				"conflated_outputs":   outputs,
				"arbitrage_events":    arbs,
				"compression_ratio":   ratio,
			},
			"stream": map[string]interface{}{
				"active_ws_clients": wsHub.GetActiveClients(),
			},
			"active_symbols": dispatcher.GetActiveSymbols(),
			"timestamp":      time.Now().UnixNano() / 1000,
		}

		json.NewEncoder(w).Encode(metricsPayload)
	})

	mux.HandleFunc("/api/book", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			symbol = "BTC-USDT"
		}

		book, exists := dispatcher.GetBook(symbol)
		if !exists {
			http.Error(w, fmt.Sprintf("Symbol %s not found", symbol), http.StatusNotFound)
			return
		}

		snapshot := book.GetSnapshot(50)
		json.NewEncoder(w).Encode(snapshot)
	})

	// Global CORS handler
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		mux.ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	server := &http.Server{
		Addr:    port,
		Handler: corsHandler,
	}

	go func() {
		log.Printf("[Server] HTTP & WebSocket Server listening on http://localhost%s\n", port)
		log.Printf("         - WebSocket Endpoint: ws://localhost%s/ws\n", port)
		log.Printf("         - Telemetry Metrics:  http://localhost%s/api/metrics\n", port)
		log.Printf("         - Health Check:       http://localhost%s/api/health\n", port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Server] HTTP server listen error: %v\n", err)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	log.Println("\n[Server] Shutdown signal received. Gracefully terminating all components...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Server] Server forced to shutdown: %v\n", err)
	}

	log.Println("[Server] Nexus-Feed shutdown complete. Bye!")
}