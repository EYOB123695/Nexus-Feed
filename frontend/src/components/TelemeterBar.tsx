'use client';
// Directive: Marks this as a Client Component so it can use React hooks and TanStack Query in the browser.

import React from 'react';
//  React import for JSX syntax.

import { useQuery } from '@tanstack/react-query';
//  Imports the primary TanStack Query hook used to fetch, cache, and auto-poll backend REST endpoints.

import { fetchTelemetry } from '@/lib/api';
//  Imports our typed API helper function from api.ts that makes the HTTP GET /api/metrics call.

import { Activity, Cpu, Gauge, Layers, Server, Users } from 'lucide-react';


export default function TelemetryBar() {

    //TANSTACK QUERY HOOK: Manages polling, caching, loading, and error states automatically

    const { data: telemetry, isLoading, isError } = useQuery({
        queryKey: ['system-telemetry'],
        queryFn: fetchTelemetry,
        refetchInterval: 1000,
    });

    // 1. LOADING STATE: Shown while waiting for the first telemetry response
    if (isLoading && !telemetry) {
        return (
            <div className="bg-[#0B0E14] border-b border-gray-800/80 px-4 py-1.5 text-xs font-mono text-gray-400 animate-pulse flex items-center gap-2">
                <Server className="w-3.5 h-3.5 text-gray-500" />
                <span>Synchronizing HFT Engine Telemetry...</span>
            </div>
        );
    }

    // 2. ERROR STATE: Shown if backend server is not running on localhost:8080
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
    // Extract child structs safely with optional chaining (?.)
    const engine = telemetry?.engine;
    const conflation = telemetry?.conflation;
    const stream = telemetry?.stream;

    // Nullish Coalescing (??): If undefined, fallback to 0 safely
    const latency = engine?.average_latency_micros ?? 0;

    // Dynamic Color Coding based on HFT speed:
    // - Under 50µs = Ultra-fast (Emerald Green)
    // - Under 200µs = Acceptable (Yellow)
    // - Over 200µs = Elevated Latency (Rose Red)
    const latencyColor =
        latency < 50
            ? 'text-emerald-400'
            : latency < 200
                ? 'text-yellow-400'
                : 'text-rose-400';

    // 64K lock-free buffer saturation percentage (0% to 100%)
    const bufferSat = engine?.buffer_saturation_pct ?? 0;


    return (
        <div className="bg-[#080B10] border-b border-gray-800/80 px-4 py-1.5 font-mono text-xs text-gray-300">
            <div className="max-w-[1920px] mx-auto flex flex-wrap items-center justify-between gap-4">

                {/* LEFT: TELEMETRY METRICS ROW */}
                <div className="flex flex-wrap items-center gap-6">

                    {/* Metric 1: Ingestion Throughput (msgs/sec) */}
                    <div className="flex items-center gap-2">
                        <Activity className="w-3.5 h-3.5 text-cyan-400" />
                        <span className="text-gray-400">Throughput:</span>
                        <span className="text-white font-semibold">
                            {engine?.messages_per_sec ? engine.messages_per_sec.toLocaleString() : 0}{' '}
                            <span className="text-[10px] text-gray-400 font-normal">msgs/s</span>
                        </span>
                    </div>

                    <div className="h-3.5 w-px bg-gray-800 hidden sm:block" />

                    {/* Metric 2: SkipList Engine Latency in Microseconds (µs) */}
                    <div className="flex items-center gap-2">
                        <Gauge className={`w-3.5 h-3.5 ${latencyColor}`} />
                        <span className="text-gray-400">Engine Latency:</span>
                        <span className={`font-semibold ${latencyColor}`}>
                            {latency.toFixed(2)} <span className="text-[10px] text-gray-400 font-normal">µs</span>
                        </span>
                    </div>

                    <div className="h-3.5 w-px bg-gray-800 hidden sm:block" />

                    {/* Metric 3: Conflation Compression Ratio */}
                    <div className="flex items-center gap-2">
                        <Layers className="w-3.5 h-3.5 text-purple-400" />
                        <span className="text-gray-400">Conflation:</span>
                        <span className="text-white font-semibold">
                            {conflation?.compression_ratio ? conflation.compression_ratio.toFixed(1) : 0}%{' '}
                            <span className="text-[10px] text-gray-400 font-normal">saved</span>
                        </span>
                    </div>

                    <div className="h-3.5 w-px bg-gray-800 hidden sm:block" />

                    {/* Metric 4: 64K Lock-Free Buffer Saturation with Visual Gauge Bar */}
                    <div className="flex items-center gap-2">
                        <Cpu className="w-3.5 h-3.5 text-amber-400" />
                        <span className="text-gray-400">64K Buffer:</span>
                        <div className="flex items-center gap-1.5">
                            {/* Background track (16 = 64px width, 1.5 = 6px height) */}
                            <div className="w-16 h-1.5 bg-gray-800 rounded-full overflow-hidden">
                                {/* Dynamic Fill bar */}
                                <div
                                    className={`h-full rounded-full transition-all duration-300 ${bufferSat < 50
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

                {/* RIGHT: ACTIVE BROWSER WEBSOCKET CLIENTS */}
                <div className="flex items-center gap-4 text-gray-400 text-[11px]">
                    <div className="flex items-center gap-1.5">
                        <Users className="w-3.5 h-3.5 text-emerald-400" />
                        <span>Active WS Clients:</span>
                        <span className="text-emerald-400 font-bold font-mono">
                            {stream?.active_ws_clients ?? 1}
                        </span>
                    </div>
                </div>

            </div>
        </div>
    );
}







