import React from 'react';
import { SystemVitalsCard } from '../Cards/SystemVitalsCard.tsx';
import { WorkerCard } from '../Cards/WorkerCard.tsx';
import type { SystemVitals, WorkerProfile } from '../../types/telemetry.ts';
import { RefreshCw, Users, Activity } from 'lucide-react';

interface OverviewTabProps {
  vitals: SystemVitals | null;
  workers: WorkerProfile[];
  loadingVitals: boolean;
  loadingWorkers: boolean;
  onRefresh: () => void;
  onSelectWorker: (worker: WorkerProfile) => void;
  onLaunchPrompt: (worker: WorkerProfile) => void;
  onActivate?: (worker: WorkerProfile) => void;
}

export const OverviewTab: React.FC<OverviewTabProps> = ({
  vitals,
  workers,
  loadingVitals,
  loadingWorkers,
  onRefresh,
  onSelectWorker,
  onLaunchPrompt,
  onActivate
}) => {
  const activeCount = workers.filter((w) => w.status === 'running').length;
  const idleCount = workers.filter((w) => w.status === 'idle').length;
  const unauthCount = workers.filter((w) => w.status === 'unauthenticated').length;

  const sortedWorkers = [...workers].sort((a, b) => {
    if (a.authenticated && !b.authenticated) return -1;
    if (!a.authenticated && b.authenticated) return 1;
    return a.id.localeCompare(b.id);
  });

  return (
    <div className="space-y-3">
      <div className="card p-3.5 space-y-2.5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Activity className="w-4 h-4 text-indigo-600" />
            <h2 className="text-xs font-bold text-slate-800 uppercase tracking-wider">Status Kluster</h2>
          </div>
          <button
            onClick={onRefresh}
            className="p-1.5 rounded-[4px] bg-slate-100 hover:bg-slate-200 text-slate-700 transition-colors"
            title="Segarkan"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loadingVitals || loadingWorkers ? 'animate-spin' : ''}`} />
          </button>
        </div>

        <div className="grid grid-cols-3 gap-2 text-center pt-2 border-t border-slate-100">
          <div className="p-2 rounded-[4px] bg-slate-50 border border-slate-200/70">
            <div className="text-base font-bold text-emerald-700 font-mono">{activeCount}</div>
            <div className="text-[10px] text-slate-500 font-medium uppercase">Aktif</div>
          </div>
          <div className="p-2 rounded-[4px] bg-slate-50 border border-slate-200/70">
            <div className="text-base font-bold text-amber-700 font-mono">{idleCount}</div>
            <div className="text-[10px] text-slate-500 font-medium uppercase">Idle</div>
          </div>
          <div className="p-2 rounded-[4px] bg-slate-50 border border-slate-200/70">
            <div className="text-base font-bold text-slate-500 font-mono">{unauthCount}</div>
            <div className="text-[10px] text-slate-400 font-medium uppercase">Off</div>
          </div>
        </div>
      </div>

      <SystemVitalsCard vitals={vitals} loading={loadingVitals} />

      <div className="space-y-2">
        <div className="flex items-center justify-between px-1">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500 flex items-center gap-1.5">
            <Users className="w-3.5 h-3.5 text-slate-700" />
            <span>Worker</span>
          </h3>
          <span className="text-[11px] font-mono text-slate-500">{workers.length}</span>
        </div>

        <div className="space-y-2">
          {sortedWorkers.map((worker) => (
            <WorkerCard
              key={worker.id}
              worker={worker}
              onSelect={onSelectWorker}
              onLaunchPrompt={onLaunchPrompt}
              onActivate={onActivate}
            />
          ))}
        </div>
      </div>
    </div>
  );
};
