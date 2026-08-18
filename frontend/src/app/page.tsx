'use client';

import React, { useState, useEffect } from 'react';
import { useMarketStream } from '@/hooks/useMarketStream';
import Header from '@/components/Header';
import TelemetryBar from '@/components/TelemeterBar';
import OrderBook from '@/components/OrderBook';
import ArbitrageRadar from '@/components/ArbitrageRadar';
import DepthChart from '@/components/DepthChart';
import { Activity, ShieldCheck, Zap } from 'lucide-react';

export default function Home() {
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  // 1. WEBSOCKET HOOK: Connects to Go backend and provides live reactive state
  const {
    book,
    arbitrageOpportunities,
    connectionStatus,
    activeSymbol,
    setActiveSymbol,
    clearArbitrageHistory,
    lastTickTime,
  } = useMarketStream('BTC-USDT');

  // Format price helper
  const formatPrice = (p: number | undefined) => {
    if (!p) return '---.--';
    return p.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  };

  return (
    <div className="min-h-screen bg-[#080B10] text-gray-100 flex flex-col font-sans selection:bg-emerald-500 selection:text-black">
      
      {/* 1. STICKY TERMINAL HEADER */}
      <Header
        activeSymbol={activeSymbol}
        onSelectSymbol={setActiveSymbol}
        connectionStatus={connectionStatus}
        book={book}
      />

      {/* 2. LIVE HFT ENGINE TELEMETRY BAR (TanStack Query) */}
      <TelemetryBar />

      {/* 3. MAIN TERMINAL WORKSPACE (Bento Grid) */}
      <main className="flex-1 max-w-[1920px] w-full mx-auto p-4 flex flex-col gap-4">
        
        {/* TOP SUMMARY TICKER STRIP */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 font-mono text-xs">
          
          {/* Card 1: Active Symbol */}
          <div className="bg-[#0B0E14] border border-gray-800/80 rounded-lg p-3 flex items-center justify-between">
            <div>
              <span className="text-[10px] text-gray-400 block uppercase">Trading Pair</span>
              <span className="text-sm font-bold text-white tracking-wide">{activeSymbol}</span>
            </div>
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-ping" />
          </div>

          {/* Card 2: 24h Mid Price */}
          <div className="bg-[#0B0E14] border border-gray-800/80 rounded-lg p-3">
            <span className="text-[10px] text-gray-400 block uppercase">Global Mid Price</span>
            <span className="text-sm font-bold text-emerald-400">
              ${formatPrice(book?.mid_price)}
            </span>
          </div>

          {/* Card 3: Spread Margin */}
          <div className="bg-[#0B0E14] border border-gray-800/80 rounded-lg p-3">
            <span className="text-[10px] text-gray-400 block uppercase">NBBO Spread</span>
            <span className="text-sm font-bold text-gray-200">
              ${formatPrice(book?.spread)}{' '}
              <span className="text-xs text-gray-400 font-normal">
                ({book?.spread_pct ? book.spread_pct.toFixed(4) : '0.0000'}%)
              </span>
            </span>
          </div>

          {/* Card 4: Conflation Frame Rate */}
          <div className="bg-[#0B0E14] border border-gray-800/80 rounded-lg p-3 flex items-center justify-between">
            <div>
              <span className="text-[10px] text-gray-400 block uppercase">Engine Flush Rate</span>
              <span className="text-sm font-bold text-purple-400">50ms / 20 FPS</span>
            </div>
            <Activity className="w-4 h-4 text-purple-400" />
          </div>

        </div>

        {/* 4. MAIN BENTO GRID */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-4 flex-1 items-start">
          
          {/* LEFT COLUMN (7 Cols): Liquidity Depth & Consolidated L2 Order Book */}
          <div className="lg:col-span-7 flex flex-col gap-4">
            <DepthChart book={book} />
            <OrderBook book={book} />
          </div>

          {/* RIGHT COLUMN (5 Cols): Cross-Exchange Arbitrage Radar & Exchange Matrix */}
          <div className="lg:col-span-5 flex flex-col gap-4">
            <ArbitrageRadar
              opportunities={arbitrageOpportunities}
              book={book}
              onClearHistory={clearArbitrageHistory}
            />

            {/* CROSS-EXCHANGE SPREAD MATRIX CARD */}
            <div className="bg-[#0B0E14] border border-gray-800/80 rounded-xl p-4 font-mono shadow-xl text-xs">
              <div className="flex items-center gap-2 mb-3">
                <ShieldCheck className="w-4 h-4 text-emerald-400" />
                <h3 className="font-bold uppercase tracking-wider text-white">
                  Multi-Exchange Feed Matrix
                </h3>
              </div>

              <div className="grid grid-cols-2 gap-3 text-center text-[11px]">
                {/* Binance Card */}
                <div className="bg-gray-900/60 border border-gray-800 rounded-lg p-2.5 flex flex-col gap-1">
                  <span className="text-[#F0B90B] font-bold text-xs">Binance L2 Stream</span>
                  <span className="text-[10px] text-gray-400">Status: LIVE</span>
                  <span className="text-emerald-400 font-bold">100% Health</span>
                </div>

                {/* Coinbase Card */}
                <div className="bg-gray-900/60 border border-gray-800 rounded-lg p-2.5 flex flex-col gap-1">
                  <span className="text-[#3B82F6] font-bold text-xs">Coinbase L2 Stream</span>
                  <span className="text-[10px] text-gray-400">Status: LIVE</span>
                  <span className="text-emerald-400 font-bold">100% Health</span>
                </div>
              </div>

              <p className="mt-3 text-[10px] text-gray-500 leading-relaxed">
                Aggregating real-time Depth of Market (DOM) feeds via Go SkipList engine. Updates are coalesced and conflated into 50ms frames for optimal UI rendering performance.
              </p>
            </div>

          </div>

        </div>

      </main>

      {/* 5. LOW-PROFILE TERMINAL FOOTER */}
      <footer className="border-t border-gray-800/80 bg-[#06080D] px-4 py-2 text-center font-mono text-[11px] text-gray-500 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Zap className="w-3.5 h-3.5 text-emerald-400" />
          <span className="text-gray-400">Nexus-Feed HFT Market Engine</span>
          <span className="text-gray-600">•</span>
          <span>SkipList L2 Aggregator Active</span>
        </div>
        <div className="text-gray-500">
          Last Heartbeat:{' '}
          <span className="text-gray-400" suppressHydrationWarning>
            {mounted ? new Date(lastTickTime).toLocaleTimeString() : '--:--:--'}
          </span>
        </div>
      </footer>

    </div>
  );
}
