import React, { useState, useEffect } from 'react';
import { fetchWorkerTranscripts } from '../../services/api.ts';
import type { WorkerProfile, TranscriptItem } from '../../types/telemetry.ts';
import { Clock, Loader2, RefreshCw } from 'lucide-react';

interface TranscriptsTabProps {
  workers: WorkerProfile[];
}

export const TranscriptsTab: React.FC<TranscriptsTabProps> = ({ workers }) => {
  const activeWorkers = workers.filter((w) => w.authenticated);
  const [selectedWorkerId, setSelectedWorkerId] = useState<string>(
    activeWorkers[0]?.id || 'agy01'
  );
  const [transcripts, setTranscripts] = useState<TranscriptItem[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (activeWorkers.length > 0 && !activeWorkers.some((w) => w.id === selectedWorkerId)) {
      setSelectedWorkerId(activeWorkers[0].id);
    }
  }, [activeWorkers, selectedWorkerId]);

  const loadTranscripts = () => {
    if (!selectedWorkerId) return;
    setLoading(true);
    fetchWorkerTranscripts(selectedWorkerId)
      .then((items) => setTranscripts(items))
      .catch(() => setTranscripts([]))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    loadTranscripts();
  }, [selectedWorkerId]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-bold text-slate-900">Log Sesi</h2>
        <button
          onClick={loadTranscripts}
          className="p-1.5 rounded-[4px] bg-white hover:bg-slate-100 text-slate-700 border border-slate-200 transition-colors shadow-xs"
          title="Segarkan"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      <div className="flex gap-1.5 overflow-x-auto pb-0.5 scrollbar-none">
        {workers.map((w) => {
          const isSelected = selectedWorkerId === w.id;
          return (
            <button
              key={w.id}
              onClick={() => setSelectedWorkerId(w.id)}
              className={`px-2.5 py-1 rounded-[4px] text-xs font-mono font-medium whitespace-nowrap transition-colors uppercase border shadow-xs ${
                isSelected
                  ? 'bg-slate-900 text-white border-slate-900 font-bold'
                  : 'bg-white border border-slate-200 text-slate-600 hover:bg-slate-50'
              }`}
            >
              {w.id}
            </button>
          );
        })}
      </div>

      <div className="space-y-2">
        {loading ? (
          <div className="flex items-center justify-center py-10 text-slate-500 gap-1.5 text-xs">
            <Loader2 className="w-3.5 h-3.5 animate-spin text-slate-700" />
            Memuat...
          </div>
        ) : transcripts.length === 0 ? (
          <div className="card text-center py-8 text-slate-400 text-xs">
            Kosong
          </div>
        ) : (
          transcripts.map((t) => (
            <div
              key={t.conversation_id}
              className="card p-3 space-y-2"
            >
              <div className="flex items-center justify-between text-[10px] text-slate-500 pb-1.5 border-b border-slate-100 font-mono">
                <span className="text-slate-900 font-semibold">
                  {t.conversation_id.slice(0, 8)}
                </span>
                <span className="flex items-center gap-1 text-slate-400">
                  <Clock className="w-2.5 h-2.5" />
                  {new Date(t.last_updated_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                </span>
              </div>

              <div>
                <div className="text-[10px] uppercase font-bold text-slate-400 mb-0.5">
                  Prompt
                </div>
                <div className="text-xs text-slate-900 font-medium line-clamp-2">
                  {t.first_prompt}
                </div>
              </div>

              {t.last_response && (
                <div>
                  <div className="text-[10px] uppercase font-bold text-slate-400 mb-0.5">
                    Respon
                  </div>
                  <div className="text-xs text-slate-700 line-clamp-3 bg-slate-50 p-2.5 rounded-[4px] border border-slate-200/80 font-sans leading-relaxed">
                    {t.last_response}
                  </div>
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
};
