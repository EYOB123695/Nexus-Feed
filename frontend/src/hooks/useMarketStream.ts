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

const WS_BASE_URL =
  process.env.NEXT_PUBLIC_WS_URL ||
  (typeof window !== 'undefined' && window.location.hostname !== 'localhost'
    ? `ws://${window.location.hostname}:8080/ws`
    : 'ws://localhost:8080/ws');

/**
 * Computes cumulative volume totals and depth percentage for visual order book bars.
 */
function enrichPriceLevels(levels: PriceLevel[] | undefined): PriceLevel[] {
  if (!levels || levels.length === 0) return [];

  let runningTotal = 0;
  const enriched: PriceLevel[] = levels.map((lvl) => {
    runningTotal += lvl.quantity;
    return {
      ...lvl,
      total: runningTotal,
    };
  });

  const maxTotal = runningTotal > 0 ? runningTotal : 1;
  return enriched.map((lvl) => ({
    ...lvl,
    depthPct: Math.min(100, (lvl.total! / maxTotal) * 100),
  }));
}

/**
 * Connects to the backend WebSocket, subscribes to market data, and maintains real-time state.
 */
export function useMarketStream(initialSymbol: MarketSymbol = 'BTC-USDT') {
  const [activeSymbol, setActiveSymbolState] = useState<MarketSymbol>(initialSymbol);
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('CONNECTING');
  const [book, setBook] = useState<ConsolidatedBook | null>(null);
  const [arbitrageOpportunities, setArbitrageOpportunities] = useState<ArbitrageOpportunity[]>([]);
  const [lastTickTime, setLastTickTime] = useState<number>(Date.now());

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const activeSymbolRef = useRef<MarketSymbol>(initialSymbol);

  activeSymbolRef.current = activeSymbol;

  const setActiveSymbol = useCallback((symbol: MarketSymbol) => {
    setActiveSymbolState(symbol);
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          action: 'subscribe',
          symbol: symbol,
        })
      );
    }
  }, []);

  const clearArbitrageHistory = useCallback(() => {
    setArbitrageOpportunities([]);
  }, []);

  useEffect(() => {
    let isComponentMounted = true;

    function connect() {
      if (!isComponentMounted) return;
      setConnectionStatus('CONNECTING');

      const ws = new WebSocket(WS_BASE_URL);
      wsRef.current = ws;

      ws.onopen = () => {
        if (!isComponentMounted) {
          ws.close();
          return;
        }
        setConnectionStatus('CONNECTED');
        ws.send(
          JSON.stringify({
            action: 'subscribe',
            symbol: activeSymbolRef.current,
          })
        );
      };

      ws.onmessage = (event) => {
        if (!isComponentMounted) return;

        try {
          const rawLines = event.data.toString().split('\n');

          for (const line of rawLines) {
            if (!line.trim()) continue;
            const msg: WSMessage = JSON.parse(line);

            if (msg.type === 'book_update') {
              const bookData = msg.data as ConsolidatedBook;
              if (activeSymbolRef.current === 'ALL' || bookData.symbol === activeSymbolRef.current) {
                setBook({
                  ...bookData,
                  bids: enrichPriceLevels(bookData.bids),
                  asks: enrichPriceLevels(bookData.asks),
                });
                setLastTickTime(Date.now());
              }
            } else if (msg.type === 'arbitrage') {
              const arbData = msg.data as ArbitrageOpportunity;
              setArbitrageOpportunities((prev) => [arbData, ...prev].slice(0, 50));
            }
          }
        } catch (err) {
          console.error('[useMarketStream] Failed to parse WebSocket frame:', err);
        }
      };

      ws.onerror = () => {
        setConnectionStatus('ERROR');
      };

      ws.onclose = () => {
        if (!isComponentMounted) return;
        setConnectionStatus('DISCONNECTED');

        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, 2500);
      };
    }

    connect();

    return () => {
      isComponentMounted = false;
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, []);

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
