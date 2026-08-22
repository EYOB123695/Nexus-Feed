'use client';

import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { fetchTelemetry } from '@/lib/api';
import { Activity, Cpu, Gauge, Layers, Server } from 'lucide-react';

export default function TelemetryBar() {
  const { data: telemetry, isLoading, isError } = useQuery({
    queryKey: ['system-telemetry'],
    queryFn: fetchTelemetry,
    refetchInterval: 1000,
  });

  if (isLoading && !telemetry) {
    return (
      <div className="bg-[#0B0E14] border-b border-gray-800/80 px-4 py-1.5 text-xs font-mono text-gray-400 animate-pulse flex items-center gap-2">
        <Server className="w-3.5 h-3.5 text-gray-500" />
        <span>Synchronizing HFT Engine Telemetry...</span>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="bg-rose-950/30 border-b border-rose-900/50 px-4 py-1.5 text-xs font-mono text-rose-400 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Server className="w-3.5 h-3.5 text-rose-500" />
          <span>Telemetry Stream Offline — Backend not detected on http://localhost:8080</span>
        </div>
      </div>
    );
  }

  const engine = telemetry?.engine;
  const conflation = telemetry?.conflation;
  const latency = engine?.average_latency_micros ?? 0;

  const latencyColor =
    latency < 50
      ? 'text-emerald-400'
      : latency < 200
      ? 'text-yellow-400'
      : 'text-rose-400';

  const bufferSat = engine?.buffer_saturation_pct ?? 0;

  return (
    <div className="bg-[#080B10] border-b border-gray-800/80 px-4 py-1.5 font-mono text-xs text-gray-300">
      <div className="max-w-[1920px] mx-auto flex flex-wrap items-center justify-between gap-4">
        {/* Left: Metrics */}
        <div className="flex flex-wrap items-center gap-6">
          {/* Throughput */}
          <div className="flex items-center gap-2">
            <Activity className="w-3.5 h-3.5 text-cyan-400" />
            <span className="text-gray-400">Throughput:</span>
            <span className="text-white font-semibold">
              {engine?.messages_per_sec ? engine.messages_per_sec.toLocaleString() : 0}{' '}
              <span className="text-[10px] text-gray-400 font-normal">msgs/s</span>
            </span>
          </div>

          <div className="h-3.5 w-px bg-gray-800 hidden sm:block" />

          {/* Engine Latency */}
          <div className="flex items-center gap-2">
            <Gauge className={`w-3.5 h-3.5 ${latencyColor}`} />
            <span className="text-gray-400">Engine Latency:</span>
            <span className={`font-semibold ${latencyColor}`}>
              {latency.toFixed(2)} <span className="text-[10px] text-gray-400 font-normal">µs</span>
            </span>
          </div>

          <div className="h-3.5 w-px bg-gray-800 hidden sm:block" />

          {/* Conflation Compression */}
          <div className="flex items-center gap-2">
            <Layers className="w-3.5 h-3.5 text-purple-400" />
            <span className="text-gray-400">Conflation:</span>
            <span className="text-white font-semibold">
              {conflation?.compression_ratio ? conflation.compression_ratio.toFixed(1) : 0}%{' '}
              <span className="text-[10px] text-gray-400 font-normal">saved</span>
            </span>
          </div>

          <div className="h-3.5 w-px bg-gray-800 hidden sm:block" />

          {/* Buffer Saturation */}
          <div className="flex items-center gap-2">
            <Cpu className="w-3.5 h-3.5 text-amber-400" />
            <span className="text-gray-400">64K Buffer:</span>
            <div className="flex items-center gap-1.5">
              <div className="w-16 h-1.5 bg-gray-800 rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all duration-300 ${
                    bufferSat < 50
                      ? 'bg-emerald-400'
                      : bufferSat < 80
                      ? 'bg-yellow-400'
                      : 'bg-rose-500'
                  }`}
                  style={{ width: `${Math.min(100, bufferSat)}%` }}
                />
              </div>
              <span className="text-white font-semibold text-[11px]">{bufferSat.toFixed(1)}%</span>
            </div>
          </div>
        </div>

        {/* Right: Total Ticks */}
        <div className="flex items-center gap-4 text-gray-400 text-[11px]">
          <div className="flex items-center gap-1.5">
            <Activity className="w-3.5 h-3.5 text-emerald-400" />
            <span>Total Ticks:</span>
            <span className="text-white font-bold font-mono">
              {engine?.ticks_processed ? engine.ticks_processed.toLocaleString() : 0}
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}







