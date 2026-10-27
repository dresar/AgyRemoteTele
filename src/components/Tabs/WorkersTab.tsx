import React, { useState, useMemo } from 'react';
import { WorkerCard } from '../Cards/WorkerCard.tsx';
import type { WorkerProfile } from '../../types/telemetry.ts';
import { Search, RefreshCw } from 'lucide-react';

interface WorkersTabProps {
  workers: WorkerProfile[];
  loading: boolean;
  onRefresh: () => void;
  onSelectWorker: (worker: WorkerProfile) => void;
  onLaunchPrompt: (worker: WorkerProfile) => void;
  onActivate?: (worker: WorkerProfile) => void;
}

export const WorkersTab: React.FC<WorkersTabProps> = ({
  workers,
  loading,
  onRefresh,
  onSelectWorker,
  onLaunchPrompt,
  onActivate
}) => {
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'running' | 'idle' | 'unauthenticated'>('all');

  const filteredWorkers = useMemo(() => {
    return workers
      .filter((w) => {
        const matchSearch =
          w.id.toLowerCase().includes(search.toLowerCase()) ||
          w.name.toLowerCase().includes(search.toLowerCase()) ||
          w.email.toLowerCase().includes(search.toLowerCase()) ||
          (w.hostname && w.hostname.toLowerCase().includes(search.toLowerCase()));

        const matchStatus = statusFilter === 'all' || w.status === statusFilter;
        return matchSearch && matchStatus;
      })
      .sort((a, b) => {
        if (a.authenticated && !b.authenticated) return -1;
        if (!a.authenticated && b.authenticated) return 1;
        return a.id.localeCompare(b.id);
      });
  }, [workers, search, statusFilter]);

  const filterLabels: Record<string, string> = {
    all: 'Semua',
    running: 'Aktif',
    idle: 'Idle',
    unauthenticated: 'Off'
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-bold text-slate-900">Workers</h2>
        <button
          onClick={onRefresh}
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
            placeholder="Cari worker..."
            className="w-full pl-8 pr-3 py-1.5 rounded-[4px] bg-white border border-slate-200 text-xs text-slate-900 placeholder:text-slate-400 focus:outline-none focus:border-slate-400 transition-colors shadow-xs"
          />
        </div>

        <div className="flex gap-1.5 overflow-x-auto pb-0.5 scrollbar-none">
          {(['all', 'running', 'idle', 'unauthenticated'] as const).map((filter) => {
            const label = filterLabels[filter];
            const count = filter === 'all' ? workers.length : workers.filter((w) => w.status === filter).length;
            const active = statusFilter === filter;

            return (
              <button
                key={filter}
                onClick={() => setStatusFilter(filter)}
                className={`px-2.5 py-1 rounded-[4px] text-xs font-medium whitespace-nowrap transition-colors flex items-center gap-1.5 border shadow-xs ${
                  active
                    ? 'bg-slate-900 text-white border-slate-900 font-semibold'
                    : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-50'
                }`}
              >
                <span>{label}</span>
                <span className={`text-[10px] px-1 rounded-[4px] ${active ? 'bg-slate-800 text-slate-200' : 'bg-slate-100 text-slate-600'}`}>
                  {count}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="space-y-2">
        {loading && workers.length === 0 ? (
          <div className="space-y-2">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-16 bg-white border border-slate-200 rounded-[4px] animate-pulse" />
            ))}
          </div>
        ) : filteredWorkers.length === 0 ? (
          <div className="text-center py-8 rounded-[4px] bg-white border border-slate-200 text-slate-400 text-xs shadow-xs">
            Kosong
          </div>
        ) : (
          filteredWorkers.map((worker) => (
            <WorkerCard
              key={worker.id}
              worker={worker}
              onSelect={onSelectWorker}
              onLaunchPrompt={onLaunchPrompt}
              onActivate={onActivate}
            />
          ))
        )}
      </div>
    </div>
  );
};
