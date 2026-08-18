'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import {
  ArbitrageOpportunity,
  ConsolidatedBook,
  ConnectionStatus,
  MarketSymbol,
  PriceLevel,
  WSMessage,
} from '@/types/market';

// WebSocket URL: uses environment variable if available, otherwise dynamically matches hostname:8080/ws
const WS_BASE_URL =
  process.env.NEXT_PUBLIC_WS_URL ||
  (typeof window !== 'undefined' && window.location.hostname !== 'localhost'
    ? `ws://${window.location.hostname}:8080/ws`
    : 'ws://localhost:8080/ws');

/**
 * HELPER FUNCTION: enrichPriceLevels
 * Computes cumulative volume totals and depth percentage for visual order book bars.
 *
 * @param levels - Array of raw price levels from the Go SkipList
 * @returns Enriched array with `total` (running sum) and `depthPct` (0 to 100%)
 */
function enrichPriceLevels(levels: PriceLevel[] | undefined): PriceLevel[] {
  // If the array is empty or undefined, return an empty array safely
  if (!levels || levels.length === 0) return [];

  // Track the running cumulative volume (sum of quantities from top of book downward)
  let runningTotal = 0;

  // First pass: Calculate running cumulative volume for each price level
  const enriched: PriceLevel[] = levels.map((lvl) => {
    runningTotal += lvl.quantity;
    return {
      ...lvl,             // Copy original price, quantity, exchange, updated_at
      total: runningTotal // Add cumulative total up to this level
    };
  });

  // Protect against division by zero: ensure maxTotal is at least 1
  const maxTotal = runningTotal > 0 ? runningTotal : 1;

  // Second pass: Calculate relative percentage (0% to 100%) for CSS depth bar width
  return enriched.map((lvl) => ({
    ...lvl,
    depthPct: Math.min(100, (lvl.total! / maxTotal) * 100),
  }));
}

/**
 * MAIN HOOK: useMarketStream
 * Connects to the WebSocket, subscribes to market data, and maintains real-time state.
 */
export function useMarketStream(initialSymbol: MarketSymbol = 'BTC-USDT') {
  // 1. activeSymbol: Holds the currently selected coin pair (defaults to 'BTC-USDT')
  const [activeSymbol, setActiveSymbolState] = useState<MarketSymbol>(initialSymbol);

  // 2. connectionStatus: Tracks WebSocket state ('CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR')
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('CONNECTING');

  // 3. book: Stores the consolidated order book with enriched bids and asks
  const [book, setBook] = useState<ConsolidatedBook | null>(null);

  // 4. arbitrageOpportunities: An array of the latest 50 detected cross-exchange arbitrage opportunities
  const [arbitrageOpportunities, setArbitrageOpportunities] = useState<ArbitrageOpportunity[]>([]);

  // 5. lastTickTime: Holds the microsecond/millisecond timestamp of the last received frame
  const [lastTickTime, setLastTickTime] = useState<number>(Date.now());

  // REFS: Keep references across renders without triggering unnecessary re-renders
  const wsRef = useRef<WebSocket | null>(null);                  // Stores the active browser WebSocket instance
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null); // Stores the timer ID for auto-reconnection
  const activeSymbolRef = useRef<MarketSymbol>(initialSymbol);     // Keeps the current symbol available in WS callbacks without stale closures

  // Synchronize activeSymbolRef with the current activeSymbol state on every render
  activeSymbolRef.current = activeSymbol;

  /**
   * ACTION: setActiveSymbol
   * Switches the active coin and immediately sends a subscription request to the Go backend.
   */
  const setActiveSymbol = useCallback((symbol: MarketSymbol) => {
    // Update local state to re-render the UI with the new symbol
    setActiveSymbolState(symbol);

    // If the WebSocket is connected and open, send the JSON subscription command to Go
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          action: 'subscribe',
          symbol: symbol,
        })
      );
    }
  }, []);

  /**
   * ACTION: clearArbitrageHistory
   * Resets the arbitrage opportunity history table.
   */
  const clearArbitrageHistory = useCallback(() => {
    setArbitrageOpportunities([]);
  }, []);

  /**
   * LIFECYCLE: useEffect
   * Connects to the backend WebSocket on mount, parses 50ms batch streams, and cleans up on unmount.
   */
  useEffect(() => {
    // Flag to prevent state updates if the user leaves the page while connecting
    let isComponentMounted = true;

    function connect() {
      if (!isComponentMounted) return;

      // Set status to CONNECTING for the header badge
      setConnectionStatus('CONNECTING');

      // Open new WebSocket connection to ws://localhost:8080/ws
      const ws = new WebSocket(WS_BASE_URL);
      wsRef.current = ws;

      // 1. ON OPEN: Connected successfully
      ws.onopen = () => {
        if (!isComponentMounted) {
          ws.close();
          return;
        }
        setConnectionStatus('CONNECTED');

        // Immediately subscribe to the active symbol
        ws.send(
          JSON.stringify({
            action: 'subscribe',
            symbol: activeSymbolRef.current,
          })
        );
      };

      // 2. ON MESSAGE: Receiving live 50ms batches
      ws.onmessage = (event) => {
        if (!isComponentMounted) return;

        try {
          // In Go hub.go, multiple messages may be separated by '\n' (coalescing)
          const rawLines = event.data.toString().split('\n');

          for (const line of rawLines) {
            if (!line.trim()) continue;

            // Parse the WSMessage envelope: { type: string, data: any, timestamp: number }
            const msg: WSMessage = JSON.parse(line);

            // Handle Consolidated Order Book Updates
            if (msg.type === 'book_update') {
              const bookData = msg.data as ConsolidatedBook;

              // Only update if it belongs to the current symbol or if subscribed to 'ALL'
              if (activeSymbolRef.current === 'ALL' || bookData.symbol === activeSymbolRef.current) {
                setBook({
                  ...bookData,
                  bids: enrichPriceLevels(bookData.bids),
                  asks: enrichPriceLevels(bookData.asks),
                });
                setLastTickTime(Date.now());
              }
            }
            // Handle Cross-Exchange Arbitrage Opportunities
            else if (msg.type === 'arbitrage') {
              const arbData = msg.data as ArbitrageOpportunity;

              // Prepend to array and limit to 50 items (Ring Buffer)
              setArbitrageOpportunities((prev) => [arbData, ...prev].slice(0, 50));
            }
          }
        } catch (err) {
          console.error('[useMarketStream] Failed to parse WebSocket frame:', err);
        }
      };

      // 3. ON ERROR
      ws.onerror = () => {
        // Browser WebSocket triggers onerror with an empty Event object when connection is pending/refused
        setConnectionStatus('ERROR');
      };

      // 4. ON CLOSE: Connection lost -> Auto-reconnect
      ws.onclose = () => {
        if (!isComponentMounted) return;
        setConnectionStatus('DISCONNECTED');

        // Wait 2.5 seconds, then attempt auto-reconnect
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, 2500);
      };
    }

    // Start connection
    connect();

    // CLEANUP ON UNMOUNT
    return () => {
      isComponentMounted = false;
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, []);

  // Return reactive states and action handlers to whichever component calls this hook
  return {
    book,
    arbitrageOpportunities,
    connectionStatus,
    activeSymbol,
    setActiveSymbol,
    clearArbitrageHistory,
    lastTickTime,
  };
}
