'use client';

import React, { useState } from 'react';
import { ArbitrageOpportunity, ConsolidatedBook } from '@/types/market';
import { ArrowRight, Calculator, Flame, TrendingUp } from 'lucide-react';

interface ArbitrageRadarProps {
  opportunities: ArbitrageOpportunity[];
  book: ConsolidatedBook | null;
  onClearHistory: () => void;
}

function renderExchangeTag(name: string) {
  const n = name.toLowerCase();
  if (n.includes('binance')) {
    return <span className="text-[#F0B90B] font-bold">Binance</span>;
  }
  if (n.includes('coinbase')) {
    return <span className="text-[#3B82F6] font-bold">Coinbase</span>;
  }
  if (n.includes('kraken')) {
    return <span className="text-[#A855F7] font-bold">Kraken</span>;
  }
  return <span className="text-gray-300 font-bold">{name}</span>;
}

const formatMoney = (val: number) =>
  val.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export default function ArbitrageRadar({
  opportunities,
  book,
  onClearHistory,
}: ArbitrageRadarProps) {
  const [simCapital, setSimCapital] = useState<number>(10000);
  const latestOpp = opportunities[0] || null;

  return (
    <div className="bg-[#0B0E14] border border-gray-800/80 rounded-xl overflow-hidden flex flex-col font-mono shadow-xl">
      {/* Header */}
      <div className="px-4 py-3 border-b border-gray-800/80 flex items-center justify-between bg-[#0D111A]">
        <div className="flex items-center gap-2">
          <Flame className="w-4 h-4 text-amber-400 animate-pulse" />
          <h2 className="text-xs font-bold uppercase tracking-wider text-white">
            Arbitrage Radar & Scanner
          </h2>
          <span className="text-[10px] bg-amber-950/70 text-amber-400 border border-amber-800/60 px-1.5 py-0.5 rounded font-bold">
            {opportunities.length} Detected
          </span>
        </div>

        {opportunities.length > 0 && (
          <button
            type="button"
            suppressHydrationWarning
            onClick={onClearHistory}
            className="text-[10px] text-gray-400 hover:text-gray-200 bg-gray-900/80 hover:bg-gray-800 px-2 py-1 rounded border border-gray-800 transition-colors"
          >
            Clear History
          </button>
        )}
      </div>

      {/* Live Profit Simulator */}
      <div className="p-4 bg-gradient-to-br from-[#0F1420] to-[#0A0D14] border-b border-gray-800/80">
        <div className="flex items-center justify-between gap-4 mb-2">
          <div className="flex items-center gap-1.5 text-xs text-gray-300 font-semibold">
            <Calculator className="w-3.5 h-3.5 text-emerald-400" />
            <span>Instant Execution Simulator</span>
          </div>

          <div className="flex items-center gap-1.5 text-xs">
            <span className="text-[11px] text-gray-500">Trade Size ($):</span>
            <input
              type="number"
              suppressHydrationWarning
              value={simCapital}
              onChange={(e) => setSimCapital(Math.max(0, Number(e.target.value)))}
              className="w-24 bg-gray-900 border border-gray-700 rounded px-2 py-0.5 text-right text-xs text-white focus:outline-none focus:border-emerald-500 font-mono"
            />
          </div>
        </div>

        {latestOpp ? (
          <div className="flex flex-wrap items-center justify-between gap-2 p-2.5 rounded-lg bg-emerald-950/30 border border-emerald-800/40 text-xs">
            <div>
              <span className="text-[10px] text-gray-400 uppercase">Route: </span>
              <span className="font-semibold text-white">
                Buy {renderExchangeTag(latestOpp.buy_exchange)} ➔ Sell{' '}
                {renderExchangeTag(latestOpp.sell_exchange)}
              </span>
            </div>

            <div className="flex items-center gap-4">
              <div>
                <span className="text-[10px] text-gray-400 uppercase">Margin: </span>
                <span className="text-emerald-400 font-bold">
                  +{latestOpp.profit_pct.toFixed(3)}%
                </span>
              </div>
              <div className="h-4 w-px bg-emerald-800/50" />
              <div>
                <span className="text-[10px] text-gray-400 uppercase">Est. Gross Profit: </span>
                <span className="text-emerald-300 font-bold text-sm">
                  +${formatMoney(simCapital * (latestOpp.profit_pct / 100))}
                </span>
              </div>
            </div>
          </div>
        ) : (
          <div className="p-2.5 rounded-lg bg-gray-900/40 border border-gray-800 text-xs text-gray-500 text-center">
            Monitoring cross-exchange books for spread margin opportunities...
          </div>
        )}
      </div>

      {/* Opportunities Stream List */}
      <div className="flex flex-col overflow-y-auto max-h-[380px] divide-y divide-gray-800/50 scrollbar-thin scrollbar-thumb-gray-800">
        {opportunities.length === 0 ? (
          <div className="p-8 text-center text-xs text-gray-500 font-mono flex flex-col items-center justify-center gap-2">
            <TrendingUp className="w-6 h-6 text-gray-600 animate-pulse" />
            <span>Scanning for cross-exchange price divergences...</span>
            <span className="text-[10px] text-gray-600">
              Triggers when Bid(Exchange A) &gt; Ask(Exchange B)
            </span>
          </div>
        ) : (
          opportunities.map((opp, idx) => {
            const timeStr = new Date(opp.timestamp).toLocaleTimeString();
            return (
              <div
                key={`arb-${idx}-${opp.timestamp}`}
                className="p-3 hover:bg-gray-800/30 transition-colors flex flex-col gap-2 group"
              >
                <div className="flex items-center justify-between text-xs">
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-white bg-gray-900 px-1.5 py-0.5 rounded text-[11px] border border-gray-800">
                      {opp.symbol}
                    </span>
                    <div className="flex items-center gap-1 text-[11px]">
                      {renderExchangeTag(opp.buy_exchange)}
                      <ArrowRight className="w-3 h-3 text-gray-500" />
                      {renderExchangeTag(opp.sell_exchange)}
                    </div>
                  </div>

                  <div className="flex items-center gap-1.5">
                    <span className="text-[10px] text-gray-400">{timeStr}</span>
                    <span className="px-2 py-0.5 rounded text-xs font-bold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 shadow-sm">
                      +{opp.profit_pct.toFixed(3)}%
                    </span>
                  </div>
                </div>

                <div className="grid grid-cols-3 text-[11px] text-gray-400 bg-gray-900/40 p-2 rounded border border-gray-800/40">
                  <div>
                    <span className="text-[10px] text-gray-500 block">BUY ASK</span>
                    <span className="text-gray-200 font-semibold">${formatMoney(opp.buy_price)}</span>
                  </div>
                  <div>
                    <span className="text-[10px] text-gray-500 block">SELL BID</span>
                    <span className="text-gray-200 font-semibold">${formatMoney(opp.sell_price)}</span>
                  </div>
                  <div className="text-right">
                    <span className="text-[10px] text-gray-500 block">SPREAD MARGIN</span>
                    <span className="text-emerald-400 font-semibold">+${formatMoney(opp.spread_margin)}</span>
                  </div>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}




