'use client';

import React from 'react';
import { ConsolidatedBook } from '@/types/market';
import { BarChart3 } from 'lucide-react';

interface DepthChartProps {
  book: ConsolidatedBook | null;
}

export default function DepthChart({ book }: DepthChartProps) {
  // Guard: If book has no bids or asks, don't render anything
  if (!book || (!book.bids.length && !book.asks.length)) {
    return null;
  }

  // Calculate total volume on Bids side vs Asks side
  const totalBidVolume = book.bids.reduce((acc, lvl) => acc + lvl.quantity, 0);
  const totalAskVolume = book.asks.reduce((acc, lvl) => acc + lvl.quantity, 0);
  const totalVolume = totalBidVolume + totalAskVolume;

  // Calculate percentage ratio (defaults to 50/50 if volume is zero)
  const bidRatio = totalVolume > 0 ? (totalBidVolume / totalVolume) * 100 : 50;
  const askRatio = totalVolume > 0 ? (totalAskVolume / totalVolume) * 100 : 50;

  // Format volume numbers with commas and 2 decimals
  const formatVol = (v: number) =>
    v.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  return (
    <div className="bg-[#0B0E14] border border-gray-800/80 rounded-xl p-4 font-mono shadow-xl">
      
      {/* 1. HEADER: Title & Total Depth */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <BarChart3 className="w-4 h-4 text-cyan-400" />
          <h3 className="text-xs font-bold uppercase tracking-wider text-white">
            Order Book Liquidity Balance
          </h3>
        </div>
        <div className="text-[11px] text-gray-400">
          Total Depth:{' '}
          <span className="text-white font-semibold">{formatVol(totalVolume)}</span>
        </div>
      </div>

      {/* 2. LIQUIDITY STATS & DUAL GRADIENT BAR */}
      <div className="space-y-1.5">
        {/* Text Labels: Bids % (Green) vs Asks % (Red) */}
        <div className="flex items-center justify-between text-xs font-semibold">
          <span className="text-emerald-400 flex items-center gap-1.5">
            Bids: {bidRatio.toFixed(1)}% ({formatVol(totalBidVolume)})
          </span>
          <span className="text-rose-400 flex items-center gap-1.5">
            ({formatVol(totalAskVolume)}) {askRatio.toFixed(1)}% :Asks
          </span>
        </div>

        {/* Dynamic Dual-Color Liquidity Bar Track */}
        <div className="h-3 w-full bg-gray-900 rounded-full overflow-hidden flex border border-gray-800">
          {/* Green Bid Volume Fill */}
          <div
            className="h-full bg-gradient-to-r from-emerald-600 to-emerald-400 transition-all duration-300 shadow-sm shadow-emerald-500/20"
            style={{ width: `${bidRatio}%` }}
          />
          {/* Red Ask Volume Fill */}
          <div
            className="h-full bg-gradient-to-l from-rose-600 to-rose-400 transition-all duration-300 shadow-sm shadow-rose-500/20"
            style={{ width: `${askRatio}%` }}
          />
        </div>
      </div>

    </div>
  );
}
