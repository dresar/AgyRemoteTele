import React from 'react';
import { Cpu, HardDrive, MemoryStick, Clock } from 'lucide-react';
import type { SystemVitals } from '../../types/telemetry.ts';

interface SystemVitalsCardProps {
  vitals: SystemVitals | null;
  loading: boolean;
}

export const SystemVitalsCard: React.FC<SystemVitalsCardProps> = ({ vitals, loading }) => {
  const formatUptime = (seconds: number) => {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
  };

  const formatBytes = (bytes: number) => {
    const gb = bytes / (1024 * 1024 * 1024);
    return `${gb.toFixed(1)} GB`;
  };

  if (loading && !vitals) {
    return (
      <div className="card p-3.5 animate-pulse space-y-2">
        <div className="h-3 bg-slate-100 rounded-[4px] w-1/4"></div>
        <div className="grid grid-cols-3 gap-2">
          <div className="h-16 bg-slate-100 rounded-[4px]"></div>
          <div className="h-16 bg-slate-100 rounded-[4px]"></div>
          <div className="h-16 bg-slate-100 rounded-[4px]"></div>
        </div>
      </div>
    );
  }

  if (!vitals) {
    return (
      <div className="card p-3 text-center py-4">
        <p className="text-xs text-slate-400">Tidak ada data</p>
      </div>
    );
  }

  return (
    <div className="card p-3.5 space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">
          Vitals
        </h3>
        <div className="flex items-center gap-1 text-[11px] font-mono text-slate-700 bg-slate-100 px-2 py-0.5 rounded-[4px] border border-slate-200">
          <Clock className="w-2.5 h-2.5 text-slate-500" />
          <span>{formatUptime(vitals.uptime)}</span>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2">
        <div className="p-2.5 rounded-[4px] bg-slate-50 border border-slate-200/80">
          <div className="flex items-center gap-1 text-slate-600 mb-1">
            <Cpu className="w-3 h-3 text-indigo-600" />
            <span className="text-[10px] font-semibold uppercase">CPU</span>
          </div>
          <div className="text-sm font-bold text-slate-900 font-mono">
            {vitals.cpu.load1.toFixed(1)}
          </div>
          <div className="text-[10px] text-slate-500 mt-0.5 font-mono">
            {vitals.cpu.cores} vCPU
          </div>
        </div>

        <div className="p-2.5 rounded-[4px] bg-slate-50 border border-slate-200/80">
          <div className="flex items-center gap-1 text-slate-600 mb-1">
            <MemoryStick className="w-3 h-3 text-indigo-600" />
            <span className="text-[10px] font-semibold uppercase">RAM</span>
          </div>
          <div className="text-sm font-bold text-slate-900 font-mono">
            {vitals.memory.percent.toFixed(0)}%
          </div>
          <div className="text-[10px] text-slate-500 mt-0.5 font-mono">
            {formatBytes(vitals.memory.used)} / {formatBytes(vitals.memory.total)}
          </div>
        </div>

        <div className="p-2.5 rounded-[4px] bg-slate-50 border border-slate-200/80">
          <div className="flex items-center gap-1 text-slate-600 mb-1">
            <HardDrive className="w-3 h-3 text-indigo-600" />
            <span className="text-[10px] font-semibold uppercase">Disk</span>
          </div>
          <div className="text-sm font-bold text-slate-900 font-mono">
            {vitals.disk.percent.toFixed(0)}%
          </div>
          <div className="text-[10px] text-slate-500 mt-0.5 font-mono">
            {formatBytes(vitals.disk.free)} sisa
          </div>
        </div>
      </div>
    </div>
  );
};
