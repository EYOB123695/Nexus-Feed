'use client';

import React from 'react';
import { ConnectionStatus, ConsolidatedBook, MarketSymbol } from '@/types/market';
import { Radio, Zap } from 'lucide-react';

interface HeaderProps {
  activeSymbol: MarketSymbol;
  onSelectSymbol: (symbol: MarketSymbol) => void;
  connectionStatus: ConnectionStatus;
  book: ConsolidatedBook | null;
}

const SYMBOLS: MarketSymbol[] = ['BTC-USDT', 'ETH-USDT', 'SOL-USDT', 'ALL'];

export default function Header({
  activeSymbol,
  onSelectSymbol,
  connectionStatus,
  book,
}: HeaderProps) {
  // Helper to format currency numbers to 2 decimal places with commas (e.g. $96,450.20)
  const formatPrice = (p: number | undefined) => {
    if (p === undefined || p === 0) return '---.--';
    return p.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  };

  return (
    <header className="border-b border-gray-800 bg-[#0B0E14]/95 backdrop-blur-md sticky top-0 z-50 px-4 py-2.5">
      <div className="max-w-[1920px] mx-auto flex flex-wrap items-center justify-between gap-4">
        
        {/* 1. LEFT SECTION: BRANDING & SYMBOL SELECTOR */}
        <div className="flex items-center gap-5">
          {/* Logo */}
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-gradient-to-tr from-emerald-500 to-cyan-500 flex items-center justify-center shadow-lg shadow-emerald-500/20">
              <Zap className="w-5 h-5 text-black fill-black" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-base tracking-wider text-white">
                  NEXUS<span className="text-emerald-400">FEED</span>
                </span>
                <span className="text-[10px] font-mono uppercase bg-emerald-950/80 text-emerald-400 border border-emerald-800/60 px-1.5 py-0.5 rounded font-semibold">
                  HFT Engine
                </span>
              </div>
              <p className="text-[10px] text-gray-400">Cross-Exchange Market Arbitrage</p>
            </div>
          </div>

          <div className="h-6 w-px bg-gray-800 hidden sm:block" />

          {/* Symbol Switcher Tabs */}
          <div className="flex items-center bg-gray-900/90 p-1 rounded-lg border border-gray-800/80">
            {SYMBOLS.map((sym) => {
              const isActive = activeSymbol === sym;
              return (
                <button
                  key={sym}
                  type="button"
                  suppressHydrationWarning
                  onClick={() => onSelectSymbol(sym)}
                  className={`px-3 py-1 rounded-md text-xs font-mono font-semibold transition-all duration-150 ${
                    isActive
                      ? 'bg-emerald-500 text-black shadow-md shadow-emerald-500/20 font-bold'
                      : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800/50'
                  }`}
                >
                  {sym}
                </button>
              );
            })}
          </div>
        </div>

        {/* 2. MIDDLE SECTION: REAL-TIME GLOBAL PRICE & SPREAD TICKER */}
        {book && (
          <div className="hidden lg:flex items-center gap-6 font-mono text-xs">
            {/* Global Mid Price */}
            <div>
              <div className="text-[10px] text-gray-400 uppercase tracking-wider">Global Mid Price</div>
              <div className="text-base font-bold text-white tracking-tight">
                ${formatPrice(book.mid_price)}
              </div>
            </div>

            <div className="h-7 w-px bg-gray-800" />

            {/* NBBO Best Bid */}
            <div>
              <div className="text-[10px] text-emerald-400 uppercase tracking-wider">Best Bid (NBBO)</div>
              <div className="text-sm font-semibold text-emerald-400">
                ${formatPrice(book.best_bid)}
              </div>
            </div>

            {/* NBBO Best Ask */}
            <div>
              <div className="text-[10px] text-rose-400 uppercase tracking-wider">Best Ask (NBBO)</div>
              <div className="text-sm font-semibold text-rose-400">
                ${formatPrice(book.best_ask)}
              </div>
            </div>

            <div className="h-7 w-px bg-gray-800" />

            {/* Global Spread */}
            <div>
              <div className="text-[10px] text-gray-400 uppercase tracking-wider">Spread</div>
              <div className="text-xs font-semibold text-gray-300">
                ${formatPrice(book.spread)} <span className="text-gray-500 font-normal">({book.spread_pct.toFixed(4)}%)</span>
              </div>
            </div>
          </div>
        )}

        {/* 3. RIGHT SECTION: EXCHANGE STATUS BADGES & WEBSOCKET STATUS */}
        <div className="flex items-center gap-4">
          
          {/* Exchange Feed Status Pills */}
          <div className="hidden md:flex items-center gap-1.5 text-[11px] font-mono">
            <span className="flex items-center gap-1 px-2 py-0.5 rounded bg-[#F0B90B]/10 text-[#F0B90B] border border-[#F0B90B]/30">
              <span className="w-1.5 h-1.5 rounded-full bg-[#F0B90B] animate-pulse" />
              Binance (Live)
            </span>
            <span className="flex items-center gap-1 px-2 py-0.5 rounded bg-[#0052FF]/10 text-[#3B82F6] border border-[#0052FF]/30">
              <span className="w-1.5 h-1.5 rounded-full bg-[#0052FF] animate-pulse" />
              Coinbase (Live)
            </span>
          </div>

          <div className="h-5 w-px bg-gray-800 hidden md:block" />

          {/* WebSocket Connection Status Pill */}
          <div
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-mono font-medium border ${
              connectionStatus === 'CONNECTED'
                ? 'bg-emerald-950/60 text-emerald-400 border-emerald-700/50'
                : connectionStatus === 'CONNECTING'
                ? 'bg-yellow-950/60 text-yellow-400 border-yellow-700/50'
                : 'bg-rose-950/60 text-rose-400 border-rose-700/50'
            }`}
          >
            <Radio
              className={`w-3.5 h-3.5 ${
                connectionStatus === 'CONNECTED' ? 'animate-pulse text-emerald-400' : ''
              }`}
            />
            <span>{connectionStatus}</span>
          </div>

        </div>

      </div>
    </header>
  );
}
