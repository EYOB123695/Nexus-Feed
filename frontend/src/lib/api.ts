import { ConsolidatedBook, HealthStatus, SystemTelemetry } from '@/types/market';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

/**
 * Fetch engine performance, conflation metrics, and active client counts.
 * GET /api/metrics
 */
export async function fetchTelemetry(): Promise<SystemTelemetry> {
  const res = await fetch(`${API_BASE_URL}/api/metrics`, {
    headers: { 'Content-Type': 'application/json' },
    cache: 'no-store', // Always fetch fresh metrics
  });

  if (!res.ok) {
    throw new Error(`Failed to fetch system telemetry: ${res.statusText}`);
  }

  return res.json();
}

/**
 * Fetch instant consolidated order book snapshot for a specific symbol.
 * GET /api/book?symbol=BTC-USDT
 */
export async function fetchOrderBookSnapshot(symbol: string = 'BTC-USDT'): Promise<ConsolidatedBook> {
  const res = await fetch(`${API_BASE_URL}/api/book?symbol=${encodeURIComponent(symbol)}`, {
    headers: { 'Content-Type': 'application/json' },
    cache: 'no-store',
  });

  if (!res.ok) {
    throw new Error(`Failed to fetch book snapshot for ${symbol}: ${res.statusText}`);
  }

  return res.json();
}

/**
 * Health check endpoint.
 * GET /api/health
 */
export async function fetchHealth(): Promise<HealthStatus> {
  const res = await fetch(`${API_BASE_URL}/api/health`, {
    headers: { 'Content-Type': 'application/json' },
  });

  if (!res.ok) {
    throw new Error(`Backend health check failed: ${res.statusText}`);
  }

  return res.json();
}
