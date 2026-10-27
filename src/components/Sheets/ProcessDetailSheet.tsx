import React, { useState } from 'react';
import { BottomSheet } from './BottomSheet.tsx';
import { killProcess } from '../../services/api.ts';
import type { ProcessItem } from '../../types/telemetry.ts';
import { Trash2, AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react';

interface ProcessDetailSheetProps {
  process: ProcessItem | null;
  isOpen: boolean;
  onClose: () => void;
  onTerminated: (pid: number) => void;
}

export const ProcessDetailSheet: React.FC<ProcessDetailSheetProps> = ({
  process: proc,
  isOpen,
  onClose,
  onTerminated
}) => {
  const [confirming, setConfirming] = useState(false);
  const [killing, setKilling] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<string | null>(null);

  if (!proc) return null;

  const handleKill = async () => {
    setKilling(true);
    setError(null);
    setResult(null);

    try {
      const res = await killProcess(proc.pid);
      if (res.success) {
        setResult(`PID ${proc.pid} dihentikan`);
        setTimeout(() => {
          onTerminated(proc.pid);
          onClose();
          setConfirming(false);
          setResult(null);
        }, 1000);
      } else {
        setError('Gagal');
      }
    } catch (err: any) {
      setError(err.message || 'Gagal');
    } finally {
      setKilling(false);
    }
  };

  return (
    <BottomSheet isOpen={isOpen} onClose={onClose} title={`PID ${proc.pid}`}>
      <div className="space-y-3">
        <div className="p-3 rounded-[4px] bg-slate-50 border border-slate-200/80 space-y-1.5 text-xs">
          <div className="flex justify-between">
            <span className="text-slate-500">PID</span>
            <span className="font-mono text-slate-900 font-semibold">{proc.pid}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">PPID</span>
            <span className="font-mono text-slate-700">{proc.ppid}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">User</span>
            <span className="font-mono text-slate-700">{proc.user}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">CPU</span>
            <span className="font-mono text-slate-900 font-semibold">{proc.cpu.toFixed(1)}%</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">MEM</span>
            <span className="font-mono text-slate-900 font-semibold">{proc.mem.toFixed(1)}%</span>
          </div>
        </div>

        <div className="space-y-1">
          <span className="text-[10px] font-semibold text-slate-500 uppercase">
            Perintah
          </span>
          <div className="p-2.5 rounded-[4px] bg-slate-950 border border-slate-800 font-mono text-[11px] text-slate-200 break-all leading-relaxed">
            {proc.command}
          </div>
        </div>

        {error && (
          <div className="p-2.5 rounded-[4px] bg-rose-50 border border-rose-200 text-xs text-rose-700">
            {error}
          </div>
        )}

        {result && (
          <div className="p-2.5 rounded-[4px] bg-emerald-50 border border-emerald-200 text-xs text-emerald-800 flex items-center gap-1.5">
            <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" />
            {result}
          </div>
        )}

        {!confirming ? (
          <button
            onClick={() => setConfirming(true)}
            className="w-full py-2 px-3 rounded-[4px] bg-rose-50 hover:bg-rose-100 text-rose-700 border border-rose-200 font-semibold text-xs flex items-center justify-center gap-1.5 transition-colors shadow-xs"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Hentikan
          </button>
        ) : (
          <div className="flex gap-2">
            <button
              onClick={() => setConfirming(false)}
              disabled={killing}
              className="flex-1 py-2 px-3 rounded-[4px] bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs font-semibold border border-slate-200 transition-colors shadow-xs"
            >
              Batal
            </button>
            <button
              onClick={handleKill}
              disabled={killing}
              className="flex-1 py-2 px-3 rounded-[4px] bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold flex items-center justify-center gap-1.5 transition-colors shadow-xs"
            >
              {killing ? (
                <>
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  Memproses...
                </>
              ) : (
                <>
                  <AlertTriangle className="w-3.5 h-3.5" />
                  Konfirmasi
                </>
              )}
            </button>
          </div>
        )}
      </div>
    </BottomSheet>
  );
};
