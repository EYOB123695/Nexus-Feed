package main

import (
	// context manages cancellation signals across all background workers
	"context"
	// json encodes API responses into standard JSON format
	"encoding/json"
	// fmt formats console banners and addresses
	"fmt"
	// log handles server logs and startup output
	"log"
	// http sets up the REST and WebSocket multiplexer server
	"net/http"
	// os enables reading operating system interrupt signals
	"os"
	// signal listens for Ctrl+C and SIGTERM termination signals
	"os/signal"
	// syscall defines standard OS signal constants
	"syscall"
	// time manages timeouts and timestamps
	"time"

	// conflation coalesces high-speed updates into 50ms batches
	"nexus-feed/backend/pkg/conflation"
	// engine provides the multi-exchange SkipList order book and dispatcher
	"nexus-feed/backend/pkg/engine"
	// ingestion provides live adapters for Binance, Coinbase, and Kraken
	"nexus-feed/backend/pkg/ingestion"
	// stream manages WebSocket connections and client fan-out
	"nexus-feed/backend/pkg/stream"
)

func main() {
	// Print startup banner
	log.Println("==========================================================")
	log.Println("  ⚡ NEXUS-FEED: High-Frequency Cross-Exchange Market Engine")
	log.Println("==========================================================")

	// Create root cancellation context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: Initialize the WebSocket Streaming Hub
	wsHub := stream.NewHub()
	// Run WebSocket Hub coordinator in a dedicated goroutine
	go wsHub.Run(ctx)
	log.Println("[Stream] WebSocket Hub initialized and running.")

	// Step 2: Initialize Conflation Engine (50ms flush rate)
	// Wire conflator flush output directly to wsHub.HandleConflatedBatch
	conflator := conflation.NewConflator(50*time.Millisecond, wsHub.HandleConflatedBatch)
	// Run Conflator ticker loop in a dedicated goroutine
	go conflator.Start(ctx)
	log.Println("[Conflation] Conflation Engine active (50ms / 20 FPS flush interval).")

	// Step 3: Initialize Engine Dispatcher with 64K lock-free queue
	// Wire dispatcher output directly to conflator.Push
	dispatcher := engine.NewDispatcher(65536, conflator.Push)
	// Run Dispatcher event loop in a dedicated goroutine
	go dispatcher.Start(ctx)
	log.Println("[Engine] Dispatcher and Consolidated SkipList Books active.")

	// Step 4: Define tracked market symbols
	symbols := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT"}

	// Step 5: Initialize Live Ingestion Adapters for Binance and Coinbase
	binanceAdapter := ingestion.NewBinanceAdapter(symbols, dispatcher)
	coinbaseAdapter := ingestion.NewCoinbaseAdapter(symbols, dispatcher)

	// Start live exchange feeds concurrently
	go binanceAdapter.Start(ctx)
	go coinbaseAdapter.Start(ctx)
	log.Printf("[Ingestion] Live feeds active for Binance & Coinbase. Symbols: %v\n", symbols)

	// Step 6: Set up HTTP router & endpoints
	mux := http.NewServeMux()

	// WebSocket streaming endpoint (ws://localhost:8080/ws)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsHub.ServeWS(w, r)
	})

	// Health check endpoint (http://localhost:8080/api/health)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"system": "nexus-feed",
		})
	})

	// System Telemetry & Performance Metrics endpoint (http://localhost:8080/api/metrics)
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Query engine metrics
		engineMetrics := dispatcher.GetMetrics()

		// Query conflation metrics
		inputs, outputs, arbs, ratio := conflator.GetMetrics()

		// Package comprehensive telemetry payload
		metricsPayload := map[string]interface{}{
			"engine": engineMetrics,
			"conflation": map[string]interface{}{
				"raw_inputs_received":   inputs,
				"conflated_outputs":     outputs,
				"arbitrage_events":      arbs,
				"compression_ratio":     ratio,
			},
			"stream": map[string]interface{}{
				"active_ws_clients": wsHub.GetActiveClients(),
			},
			"active_symbols": dispatcher.GetActiveSymbols(),
			"timestamp":      time.Now().UnixNano() / 1000,
		}

		json.NewEncoder(w).Encode(metricsPayload)
	})

	// Instant Order Book Snapshot endpoint (e.g. http://localhost:8080/api/book?symbol=BTC-USDT)
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

		// Retrieve consolidated snapshot
		snapshot := book.GetSnapshot(50)
		json.NewEncoder(w).Encode(snapshot)
	})

	// Configure global CORS handler (supporting preflight OPTIONS and all origins)
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

	// Configure HTTP server
	port := ":8080"
	server := &http.Server{
		Addr:    port,
		Handler: corsHandler,
	}

	// Step 7: Launch HTTP server in background goroutine
	go func() {
		log.Printf("[Server] HTTP & WebSocket Server listening on http://localhost%s\n", port)
		log.Printf("         - WebSocket Endpoint: ws://localhost%s/ws\n", port)
		log.Printf("         - Telemetry Metrics:  http://localhost%s/api/metrics\n", port)
		log.Printf("         - Health Check:       http://localhost%s/api/health\n", port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Server] HTTP server listen error: %v\n", err)
		}
	}()

	// Step 8: Handle graceful shutdown on OS interrupt signal (Ctrl+C / SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Block until interrupt signal is received
	<-quit
	log.Println("\n[Server] Shutdown signal received. Gracefully terminating all components...")

	// Trigger context cancellation to stop background ingestion & event loops
	cancel()

	// Shutdown HTTP server with a 5-second deadline
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Server] Server forced to shutdown: %v\n", err)
	}

	log.Println("[Server] Nexus-Feed shutdown complete. Bye!")
}