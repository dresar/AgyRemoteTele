import React, { useState, useEffect, useMemo } from 'react';
import { ProcessCard } from '../Cards/ProcessCard.tsx';
import { ProcessDetailSheet } from '../Sheets/ProcessDetailSheet.tsx';
import { fetchProcesses } from '../../services/api.ts';
import type { ProcessItem } from '../../types/telemetry.ts';
import { Search, RefreshCw } from 'lucide-react';

export const ConsoleTab: React.FC = () => {
  const [processes, setProcesses] = useState<ProcessItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [sortBy, setSortBy] = useState<'cpu' | 'mem' | 'pid'>('cpu');
  const [selectedProcess, setSelectedProcess] = useState<ProcessItem | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);

  const loadProcesses = () => {
    setLoading(true);
    fetchProcesses()
      .then((items) => setProcesses(items))
      .catch(() => setProcesses([]))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    loadProcesses();
    const interval = setInterval(loadProcesses, 5000);
    return () => clearInterval(interval);
  }, []);

  const filteredProcesses = useMemo(() => {
    return processes
      .filter((p) => {
        const query = search.toLowerCase();
        return (
          p.command.toLowerCase().includes(query) ||
          p.user.toLowerCase().includes(query) ||
          p.pid.toString().includes(query)
        );
      })
      .sort((a, b) => {
        if (sortBy === 'cpu') return b.cpu - a.cpu;
        if (sortBy === 'mem') return b.mem - a.mem;
        return b.pid - a.pid;
      });
  }, [processes, search, sortBy]);

  const handleSelect = (proc: ProcessItem) => {
    setSelectedProcess(proc);
    setIsDetailOpen(true);
  };

  const handleKillClick = (proc: ProcessItem) => {
    setSelectedProcess(proc);
    setIsDetailOpen(true);
  };

  const handleTerminated = (pid: number) => {
    setProcesses((prev) => prev.filter((p) => p.pid !== pid));
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-bold text-slate-900">Proses</h2>
        <button
          onClick={loadProcesses}
          className="p-1.5 rounded-[4px] bg-white hover:bg-slate-100 text-slate-700 border border-slate-200 transition-colors shadow-xs"
          title="Segarkan"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      <div className="space-y-2">
        <div className="relative">
          <Search className="w-3.5 h-3.5 absolute left-3 top-2.5 text-slate-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Cari proses..."
            className="w-full pl-8 pr-3 py-1.5 rounded-[4px] bg-white border border-slate-200 text-xs text-slate-900 placeholder:text-slate-400 focus:outline-none focus:border-slate-400 transition-colors shadow-xs"
          />
        </div>

        <div className="flex items-center justify-between px-1">
          <div className="flex gap-1.5">
            {(['cpu', 'mem', 'pid'] as const).map((mode) => (
              <button
                key={mode}
                onClick={() => setSortBy(mode)}
                className={`px-2 py-0.5 rounded-[4px] text-[11px] font-medium uppercase font-mono transition-colors border shadow-xs ${
                  sortBy === mode
                    ? 'bg-slate-900 text-white border-slate-900 font-bold'
                    : 'bg-white border border-slate-200 text-slate-600 hover:bg-slate-50'
                }`}
              >
                {mode}
              </button>
            ))}
          </div>
          <span className="text-[11px] text-slate-500 font-mono">
            {filteredProcesses.length}
          </span>
        </div>
      </div>

      <div className="space-y-1.5">
        {loading && processes.length === 0 ? (
          <div className="space-y-2">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-14 bg-white border border-slate-200 rounded-[4px] animate-pulse" />
            ))}
          </div>
        ) : filteredProcesses.length === 0 ? (
          <div className="text-center py-8 rounded-[4px] bg-white border border-slate-200 text-slate-400 text-xs shadow-xs">
            Kosong
          </div>
        ) : (
          filteredProcesses.map((proc) => (
            <ProcessCard
              key={proc.pid}
              process={proc}
              onSelect={handleSelect}
              onKill={handleKillClick}
            />
          ))
        )}
      </div>

      <ProcessDetailSheet
        process={selectedProcess}
        isOpen={isDetailOpen}
        onClose={() => setIsDetailOpen(false)}
        onTerminated={handleTerminated}
      />
    </div>
  );
};
