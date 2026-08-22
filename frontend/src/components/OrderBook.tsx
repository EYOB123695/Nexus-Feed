'use client';

import React, { useState } from 'react';
import { ConsolidatedBook } from '@/types/market';
import { Layers } from 'lucide-react';

interface OrderBookProps {
  book: ConsolidatedBook | null;
}

function getExchangeBadge(exchange: string) {
  const ex = exchange.toLowerCase();
  if (ex.includes('binance')) {
    return (
      <span className="text-[10px] font-mono font-bold px-1.5 py-0.5 rounded bg-[#F0B90B]/15 text-[#F0B90B] border border-[#F0B90B]/30">
        BIN
      </span>
    );
  }
  if (ex.includes('coinbase')) {
    return (
      <span className="text-[10px] font-mono font-bold px-1.5 py-0.5 rounded bg-[#0052FF]/15 text-[#3B82F6] border border-[#0052FF]/30">
        CB
      </span>
    );
  }
  if (ex.includes('kraken')) {
    return (
      <span className="text-[10px] font-mono font-bold px-1.5 py-0.5 rounded bg-[#5741D9]/15 text-[#A855F7] border border-[#5741D9]/30">
        KRK
      </span>
    );
  }
  return (
    <span className="text-[10px] font-mono font-bold px-1.5 py-0.5 rounded bg-gray-800 text-gray-400">
      {exchange.slice(0, 3).toUpperCase()}
    </span>
  );
}

export default function OrderBook({ book }: OrderBookProps) {
  const [depthLimit, setDepthLimit] = useState<number>(20);

  const formatPrice = (p: number) =>
    p.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  const formatQty = (q: number) =>
    q.toLocaleString('en-US', { minimumFractionDigits: 4, maximumFractionDigits: 4 });

  if (!book || (!book.bids.length && !book.asks.length)) {
    return (
      <div className="bg-[#0B0E14] border border-gray-800/80 rounded-xl p-6 flex flex-col items-center justify-center min-h-[500px] text-gray-500 font-mono text-xs shadow-xl">
        <div className="w-8 h-8 rounded-full border-2 border-emerald-500/30 border-t-emerald-400 animate-spin mb-3" />
        <span>Aggregating cross-exchange order books...</span>
      </div>
    );
  }

  // Asks are ascending from best ask upwards; reverse so best ask is closest to spread banner
  const visibleAsks = book.asks.slice(0, depthLimit).reverse();
  const visibleBids = book.bids.slice(0, depthLimit);

  return (
    <div className="bg-[#0B0E14] border border-gray-800/80 rounded-xl overflow-hidden flex flex-col font-mono shadow-xl">
      {/* Header & Depth Selector */}
      <div className="px-4 py-3 border-b border-gray-800/80 flex items-center justify-between bg-[#0D111A]">
        <div className="flex items-center gap-2">
          <Layers className="w-4 h-4 text-emerald-400" />
          <h2 className="text-xs font-bold uppercase tracking-wider text-white">
            Consolidated Order Book (L2)
          </h2>
          <span className="text-[10px] bg-gray-800 text-gray-400 px-1.5 py-0.5 rounded font-mono font-semibold">
            {book.symbol}
          </span>
        </div>

        <div className="flex items-center gap-1 bg-gray-900/80 p-0.5 rounded border border-gray-800">
          {[15, 20, 30, 50].map((limit) => (
            <button
              key={limit}
              type="button"
              suppressHydrationWarning
              onClick={() => setDepthLimit(limit)}
              className={`px-2 py-0.5 text-[10px] rounded font-semibold transition-all ${
                depthLimit === limit
                  ? 'bg-emerald-500 text-black shadow font-bold'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              {limit}
            </button>
          ))}
        </div>
      </div>

      {/* Column Headers */}
      <div className="grid grid-cols-4 px-4 py-2 text-[10px] font-semibold text-gray-500 uppercase tracking-wider border-b border-gray-800/50 bg-[#090C12]">
        <div>Exchange</div>
        <div className="text-right">Price (USD)</div>
        <div className="text-right">Size</div>
        <div className="text-right">Total Depth</div>
      </div>

      {/* Asks (Sell Orders) */}
      <div className="flex flex-col-reverse justify-end overflow-hidden max-h-[280px] overflow-y-auto scrollbar-thin scrollbar-thumb-gray-800">
        {visibleAsks.map((lvl, idx) => (
          <div
            key={`ask-${lvl.exchange}-${lvl.price}-${idx}`}
            className="grid grid-cols-4 px-4 py-1 text-xs relative group hover:bg-rose-500/10 transition-colors"
          >
            <div
              className="absolute inset-y-0 right-0 bg-rose-500/15 pointer-events-none transition-all duration-75"
              style={{ width: `${lvl.depthPct ?? 0}%` }}
            />
            <div className="relative z-10 flex items-center">
              {getExchangeBadge(lvl.exchange)}
            </div>
            <div className="relative z-10 text-right font-bold text-rose-400">
              {formatPrice(lvl.price)}
            </div>
            <div className="relative z-10 text-right text-gray-200">
              {formatQty(lvl.quantity)}
            </div>
            <div className="relative z-10 text-right text-gray-400 font-normal">
              {formatQty(lvl.total ?? lvl.quantity)}
            </div>
          </div>
        ))}
      </div>

      {/* Spread & Mid-Price Banner */}
      <div className="px-4 py-2.5 bg-gradient-to-r from-gray-900 via-[#121722] to-gray-900 border-y border-gray-800/80 flex items-center justify-between text-xs font-semibold">
        <div className="flex items-center gap-2">
          <span className="text-base font-bold text-white tracking-tight">
            ${formatPrice(book.mid_price)}
          </span>
          <span className="text-[10px] text-gray-400 uppercase">Mid Price</span>
        </div>

        <div className="flex items-center gap-3 text-[11px]">
          <span className="text-gray-400">Spread:</span>
          <span className="text-emerald-400 font-bold">
            ${formatPrice(book.spread)}
          </span>
          <span className="text-gray-500 text-[10px]">
            ({book.spread_pct.toFixed(4)}%)
          </span>
        </div>
      </div>

      {/* Bids (Buy Orders) */}
      <div className="flex flex-col overflow-hidden max-h-[280px] overflow-y-auto scrollbar-thin scrollbar-thumb-gray-800">
        {visibleBids.map((lvl, idx) => (
          <div
            key={`bid-${lvl.exchange}-${lvl.price}-${idx}`}
            className="grid grid-cols-4 px-4 py-1 text-xs relative group hover:bg-emerald-500/10 transition-colors"
          >
            <div
              className="absolute inset-y-0 right-0 bg-emerald-500/15 pointer-events-none transition-all duration-75"
              style={{ width: `${lvl.depthPct ?? 0}%` }}
            />
            <div className="relative z-10 flex items-center">
              {getExchangeBadge(lvl.exchange)}
            </div>
            <div className="relative z-10 text-right font-bold text-emerald-400">
              {formatPrice(lvl.price)}
            </div>
            <div className="relative z-10 text-right text-gray-200">
              {formatQty(lvl.quantity)}
            </div>
            <div className="relative z-10 text-right text-gray-400 font-normal">
              {formatQty(lvl.total ?? lvl.quantity)}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}







