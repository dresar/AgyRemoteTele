import React from 'react';
import { Bot, ChevronRight, CheckCircle2, Clock, AlertCircle, LogIn } from 'lucide-react';
import type { WorkerProfile } from '../../types/telemetry.ts';

interface WorkerCardProps {
  worker: WorkerProfile;
  onSelect: (worker: WorkerProfile) => void;
  onLaunchPrompt: (worker: WorkerProfile) => void;
  onActivate?: (worker: WorkerProfile) => void;
}

export const WorkerCard: React.FC<WorkerCardProps> = ({ worker, onSelect, onLaunchPrompt }) => {
  const isRunning = worker.status === 'running';

  const getStatusBadge = () => {
    switch (worker.status) {
      case 'running':
        return (
          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-[4px] text-[10px] font-medium bg-emerald-50 text-emerald-700 border border-emerald-200">
            <CheckCircle2 className="w-2.5 h-2.5" />
            Aktif
          </span>
        );
      case 'idle':
        return (
          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-[4px] text-[10px] font-medium bg-amber-50 text-amber-700 border border-amber-200">
            <Clock className="w-2.5 h-2.5" />
            Idle
          </span>
        );
      case 'unauthenticated':
      default:
        return (
          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-[4px] text-[10px] font-medium bg-slate-100 text-slate-500 border border-slate-200">
            <AlertCircle className="w-2.5 h-2.5" />
            Off
          </span>
        );
    }
  };

  return (
    <div
      onClick={() => onSelect(worker)}
      className={`card p-3 transition-all cursor-pointer ${isRunning ? 'card--accent' : ''}`}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="p-1.5 rounded-[4px] bg-slate-100 text-slate-800 shrink-0 border border-slate-200/60">
            <Bot className="w-3.5 h-3.5 text-indigo-600" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="font-semibold text-xs text-slate-900 uppercase tracking-wide font-mono">
                {worker.id}
              </span>
              <span className="text-[11px] text-slate-500 truncate font-mono">
                {worker.hostname || worker.name}
              </span>
            </div>
            <p className="text-[11px] text-slate-500 truncate">
              {worker.email}
            </p>
          </div>
        </div>
        <div className="shrink-0 flex items-center gap-1.5">
          {getStatusBadge()}
          <ChevronRight className="w-3.5 h-3.5 text-slate-400" />
        </div>
      </div>

      {worker.authenticated ? (
        <div className="mt-2.5 pt-2 border-t border-slate-100 flex items-center justify-between">
          <span className="text-[11px] text-slate-500 font-mono">
            {worker.activeSession ? 'Sesi aktif' : 'Siap'}
          </span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onLaunchPrompt(worker);
            }}
            className="px-2.5 py-0.5 rounded-[4px] text-[11px] font-medium bg-slate-900 hover:bg-slate-800 text-white transition-colors"
          >
            Prompt
          </button>
        </div>
      ) : (
        <div className="mt-2.5 pt-2 border-t border-slate-100 flex items-center justify-between">
          <span className="text-[11px] text-slate-400 font-mono">
            Non-aktif
          </span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onSelect(worker);
            }}
            className="px-2.5 py-0.5 rounded-[4px] text-[11px] font-medium bg-indigo-50 hover:bg-indigo-100 text-indigo-700 border border-indigo-200 transition-colors flex items-center gap-1"
          >
            <LogIn className="w-2.5 h-2.5" />
            <span>Login Google</span>
          </button>
        </div>
      )}
    </div>
  );
};
