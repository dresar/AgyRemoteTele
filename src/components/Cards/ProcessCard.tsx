import React from 'react';
import { Terminal, Trash2 } from 'lucide-react';
import type { ProcessItem } from '../../types/telemetry.ts';

interface ProcessCardProps {
  process: ProcessItem;
  onSelect: (process: ProcessItem) => void;
  onKill: (process: ProcessItem) => void;
}

export const ProcessCard: React.FC<ProcessCardProps> = ({ process, onSelect, onKill }) => {
  const isHighCpu = process.cpu > 5.0;

  return (
    <div
      onClick={() => onSelect(process)}
      className={`card p-3 transition-all cursor-pointer ${isHighCpu ? 'card--accent' : ''}`}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="p-1.5 rounded-[4px] bg-slate-100 text-slate-700 shrink-0 border border-slate-200/60">
            <Terminal className="w-3.5 h-3.5 text-indigo-600" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="font-mono text-xs font-semibold text-slate-900">
                PID {process.pid}
              </span>
              <span className="text-[10px] px-1.5 py-0.5 rounded-[4px] bg-slate-100 text-slate-600 font-mono border border-slate-200/60">
                {process.user}
              </span>
            </div>
            <p className="text-[11px] text-slate-500 font-mono truncate mt-0.5">
              {process.command}
            </p>
          </div>
        </div>

        <div className="shrink-0 flex items-center gap-2">
          <div className="text-right">
            <div className="text-xs font-semibold font-mono text-slate-800">
              {process.cpu.toFixed(1)}% <span className="text-[9px] text-slate-400 font-normal">CPU</span>
            </div>
            <div className="text-xs font-semibold font-mono text-slate-700">
              {process.mem.toFixed(1)}% <span className="text-[9px] text-slate-400 font-normal">MEM</span>
            </div>
          </div>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onKill(process);
            }}
            className="p-1.5 rounded-[4px] text-slate-400 hover:text-rose-600 hover:bg-rose-50 transition-colors"
            title="Hentikan"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
};
