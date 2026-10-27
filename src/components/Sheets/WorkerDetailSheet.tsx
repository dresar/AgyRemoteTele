import React, { useState, useEffect } from 'react';
import { BottomSheet } from './BottomSheet.tsx';
import { fetchWorkerTranscripts, startWorkerLogin, submitWorkerAuthCode, logoutWorker } from '../../services/api.ts';
import { streamWorkerPrompt } from '../../services/sse.ts';
import type { WorkerProfile, TranscriptItem } from '../../types/telemetry.ts';
import { Send, Bot, Mail, Globe, Sparkles, Terminal, CheckCircle2, AlertCircle, Loader2, Power, LogIn, LogOut, ExternalLink, Copy, Check } from 'lucide-react';

interface WorkerDetailSheetProps {
  worker: WorkerProfile | null;
  isOpen: boolean;
  onClose: () => void;
  onPromptClick?: () => void;
  onActivate?: (worker: WorkerProfile) => void;
  onWorkerUpdated?: (worker: WorkerProfile) => void;
  initialPromptMode?: boolean;
}

export const WorkerDetailSheet: React.FC<WorkerDetailSheetProps> = ({
  worker,
  isOpen,
  onClose,
  onPromptClick,
  onActivate,
  onWorkerUpdated,
  initialPromptMode = false
}) => {
  const [currentWorker, setCurrentWorker] = useState<WorkerProfile | null>(worker);
  const [activeSubTab, setActiveSubTab] = useState<'info' | 'transcripts' | 'prompt'>('info');
  const [transcripts, setTranscripts] = useState<TranscriptItem[]>([]);
  const [loadingTranscripts, setLoadingTranscripts] = useState(false);

  const [promptText, setPromptText] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamingText, setStreamingText] = useState('');
  const [toolBadges, setToolBadges] = useState<Array<{ name: string; state: string }>>([]);
  const [streamResult, setStreamResult] = useState<string | null>(null);
  const [streamError, setStreamError] = useState<string | null>(null);

  const [isLoggingIn, setIsLoggingIn] = useState(false);
  const [authUrl, setAuthUrl] = useState<string | null>(null);
  const [authCode, setAuthCode] = useState('');
  const [isSubmittingCode, setIsSubmittingCode] = useState(false);
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);
  const [authSuccess, setAuthSuccess] = useState<string | null>(null);
  const [copiedUrl, setCopiedUrl] = useState(false);

  useEffect(() => {
    setCurrentWorker(worker);
    setAuthUrl(null);
    setAuthCode('');
    setAuthError(null);
    setAuthSuccess(null);
  }, [worker]);

  useEffect(() => {
    if (initialPromptMode) {
      setActiveSubTab('prompt');
    } else {
      setActiveSubTab('info');
    }
  }, [initialPromptMode, worker?.id]);

  useEffect(() => {
    if (isOpen && currentWorker && activeSubTab === 'transcripts') {
      setLoadingTranscripts(true);
      fetchWorkerTranscripts(currentWorker.id)
        .then((items) => setTranscripts(items))
        .catch(() => setTranscripts([]))
        .finally(() => setLoadingTranscripts(false));
    }
  }, [isOpen, currentWorker?.id, activeSubTab]);

  if (!currentWorker) return null;

  const handleStartLogin = async () => {
    setIsLoggingIn(true);
    setAuthError(null);
    setAuthSuccess(null);
    setAuthUrl(null);
    try {
      const res = await startWorkerLogin(currentWorker.id);
      if (res.success && res.authUrl) {
        setAuthUrl(res.authUrl);
      } else {
        setAuthError('Gagal mendapatkan tautan login');
      }
    } catch (err: any) {
      setAuthError(err.message || 'Gagal memulai login');
    } finally {
      setIsLoggingIn(false);
    }
  };

  const handleSubmitCode = async () => {
    if (!authCode.trim()) return;
    setIsSubmittingCode(true);
    setAuthError(null);
    try {
      const res = await submitWorkerAuthCode(currentWorker.id, authCode.trim());
      if (res.success) {
        setAuthSuccess(res.message || 'Login berhasil');
        setAuthUrl(null);
        setAuthCode('');
        if (res.worker) {
          setCurrentWorker(res.worker);
          onWorkerUpdated?.(res.worker);
        }
      } else {
        setAuthError('Otorisasi gagal');
      }
    } catch (err: any) {
      setAuthError(err.message || 'Kode otorisasi tidak valid');
    } finally {
      setIsSubmittingCode(false);
    }
  };

  const handleLogout = async () => {
    if (!window.confirm(`Yakin ingin logout ${currentWorker.id.toUpperCase()}?`)) return;
    setIsLoggingOut(true);
    setAuthError(null);
    try {
      const res = await logoutWorker(currentWorker.id);
      if (res.success) {
        setAuthSuccess(res.message || 'Berhasil keluar');
        if (res.worker) {
          setCurrentWorker(res.worker);
          onWorkerUpdated?.(res.worker);
        }
      }
    } catch (err: any) {
      setAuthError(err.message || 'Gagal logout');
    } finally {
      setIsLoggingOut(false);
    }
  };

  const handleCopyUrl = () => {
    if (!authUrl) return;
    navigator.clipboard.writeText(authUrl);
    setCopiedUrl(true);
    setTimeout(() => setCopiedUrl(false), 2000);
  };

  const handleSendPrompt = () => {
    if (!promptText.trim() || isStreaming) return;

    setIsStreaming(true);
    setStreamingText('');
    setToolBadges([]);
    setStreamResult(null);
    setStreamError(null);

    const cancel = streamWorkerPrompt(
      currentWorker.id,
      promptText.trim(),
      {
        onDelta: (text) => {
          setStreamingText((prev) => prev + text);
        },
        onTool: (tool) => {
          setToolBadges((prev) => {
            const existingIndex = prev.findIndex((t) => t.name === tool.name);
            if (existingIndex >= 0) {
              const updated = [...prev];
              updated[existingIndex] = tool;
              return updated;
            }
            return [...prev, tool];
          });
        },
        onResult: (result) => {
          setStreamResult(result.summary || 'Selesai');
          setIsStreaming(false);
        },
        onError: (err) => {
          setStreamError(err);
          setIsStreaming(false);
        },
        onClose: () => {
          setIsStreaming(false);
        }
      }
    );

    return cancel;
  };

  return (
    <BottomSheet isOpen={isOpen} onClose={onClose} title={currentWorker.id.toUpperCase()}>
      <div className="space-y-3">
        <div className="flex border-b border-slate-200">
          <button
            onClick={() => setActiveSubTab('info')}
            className={`flex-1 py-2 text-xs font-medium border-b-2 transition-colors ${
              activeSubTab === 'info'
                ? 'border-slate-900 text-slate-900 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-800'
            }`}
          >
            Profil
          </button>
          <button
            onClick={() => setActiveSubTab('transcripts')}
            className={`flex-1 py-2 text-xs font-medium border-b-2 transition-colors ${
              activeSubTab === 'transcripts'
                ? 'border-slate-900 text-slate-900 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-800'
            }`}
          >
            Log
          </button>
          <button
            onClick={() => setActiveSubTab('prompt')}
            className={`flex-1 py-2 text-xs font-medium border-b-2 transition-colors ${
              activeSubTab === 'prompt'
                ? 'border-slate-900 text-slate-900 font-bold'
                : 'border-transparent text-slate-500 hover:text-slate-800'
            }`}
          >
            Prompt
          </button>
        </div>

        {activeSubTab === 'info' && (
          <div className="space-y-3">
            <div className="p-3 rounded-[4px] bg-slate-50 border border-slate-200/80 space-y-2 text-xs">
              <div className="flex items-center justify-between">
                <span className="text-slate-500 flex items-center gap-1.5">
                  <Bot className="w-3.5 h-3.5 text-slate-700" /> ID
                </span>
                <span className="font-mono font-semibold text-slate-900 uppercase">
                  {currentWorker.id}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-500 flex items-center gap-1.5">
                  <Globe className="w-3.5 h-3.5 text-slate-700" /> Host
                </span>
                <span className="font-mono text-slate-700">
                  {currentWorker.hostname || currentWorker.name}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-500 flex items-center gap-1.5">
                  <Mail className="w-3.5 h-3.5 text-slate-700" /> Akun
                </span>
                <span className="font-mono text-slate-700 truncate max-w-[200px]">
                  {currentWorker.email}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-500">Status</span>
                <span
                  className={`px-1.5 py-0.5 rounded-[4px] text-[10px] font-medium border ${
                    currentWorker.authenticated
                      ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
                      : 'bg-slate-100 text-slate-500 border-slate-200'
                  }`}
                >
                  {currentWorker.authenticated ? 'Aktif' : 'Off'}
                </span>
              </div>
            </div>

            {authSuccess && (
              <div className="p-2.5 rounded-[4px] bg-emerald-50 border border-emerald-200 text-xs text-emerald-800 flex items-center gap-1.5">
                <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600 shrink-0" />
                <span>{authSuccess}</span>
              </div>
            )}

            {authError && (
              <div className="p-2.5 rounded-[4px] bg-rose-50 border border-rose-200 text-xs text-rose-800 flex items-center gap-1.5">
                <AlertCircle className="w-3.5 h-3.5 text-rose-600 shrink-0" />
                <span>{authError}</span>
              </div>
            )}

            {authUrl && (
              <div className="p-3 rounded-[4px] bg-white border border-slate-200 space-y-2.5 shadow-xs">
                <div className="flex items-center justify-between text-xs font-semibold text-slate-900">
                  <span className="flex items-center gap-1.5">
                    <LogIn className="w-3.5 h-3.5 text-indigo-600" />
                    Otorisasi Google
                  </span>
                </div>
                <p className="text-[11px] text-slate-600 leading-relaxed">
                  Buka tautan login di bawah, setujui akses akun Google, lalu salin kode yang muncul ke kolom berikut:
                </p>
                <div className="flex gap-2">
                  <a
                    href={authUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex-1 py-1.5 px-2.5 rounded-[4px] bg-indigo-600 hover:bg-indigo-700 text-white font-medium text-xs flex items-center justify-center gap-1 transition-colors"
                  >
                    <ExternalLink className="w-3 h-3" />
                    Buka Google
                  </a>
                  <button
                    onClick={handleCopyUrl}
                    className="py-1.5 px-2.5 rounded-[4px] bg-slate-100 hover:bg-slate-200 text-slate-700 font-medium text-xs flex items-center justify-center gap-1 border border-slate-300 transition-colors"
                  >
                    {copiedUrl ? <Check className="w-3 h-3 text-emerald-600" /> : <Copy className="w-3 h-3" />}
                    {copiedUrl ? 'Tersalin' : 'Salin Link'}
                  </button>
                </div>
                <div className="space-y-1.5 pt-1">
                  <input
                    type="text"
                    value={authCode}
                    onChange={(e) => setAuthCode(e.target.value)}
                    placeholder="Tempel kode otorisasi di sini..."
                    disabled={isSubmittingCode}
                    className="w-full p-2 rounded-[4px] bg-slate-50 border border-slate-200 text-xs text-slate-900 focus:outline-none focus:border-slate-400 font-mono"
                  />
                  <div className="flex gap-2">
                    <button
                      onClick={handleSubmitCode}
                      disabled={isSubmittingCode || !authCode.trim()}
                      className="flex-1 py-1.5 px-3 rounded-[4px] bg-slate-900 hover:bg-slate-800 disabled:bg-slate-200 text-white font-semibold text-xs flex items-center justify-center gap-1.5 transition-colors"
                    >
                      {isSubmittingCode ? (
                        <>
                          <Loader2 className="w-3 h-3 animate-spin" />
                          Memverifikasi...
                        </>
                      ) : (
                        'Verifikasi'
                      )}
                    </button>
                    <button
                      onClick={() => setAuthUrl(null)}
                      className="py-1.5 px-2.5 rounded-[4px] text-xs text-slate-500 hover:text-slate-800 transition-colors"
                    >
                      Batal
                    </button>
                  </div>
                </div>
              </div>
            )}

            {currentWorker.authenticated ? (
              <div className="space-y-2">
                <button
                  onClick={() => {
                    if (onPromptClick) {
                      onPromptClick();
                    } else {
                      setActiveSubTab('prompt');
                    }
                  }}
                  className="w-full py-2 px-3 rounded-[4px] bg-slate-900 hover:bg-slate-800 text-white font-semibold text-xs flex items-center justify-center gap-1.5 transition-colors shadow-xs"
                >
                  <Sparkles className="w-3.5 h-3.5" />
                  Prompt
                </button>
                <button
                  onClick={handleLogout}
                  disabled={isLoggingOut}
                  className="w-full py-1.5 px-3 rounded-[4px] bg-rose-50 hover:bg-rose-100 text-rose-700 font-medium text-xs flex items-center justify-center gap-1.5 border border-rose-200 transition-colors"
                >
                  {isLoggingOut ? (
                    <>
                      <Loader2 className="w-3 h-3 animate-spin" />
                      Memproses...
                    </>
                  ) : (
                    <>
                      <LogOut className="w-3 h-3" />
                      Logout
                    </>
                  )}
                </button>
              </div>
            ) : (
              <div className="space-y-2">
                {!authUrl && (
                  <button
                    onClick={handleStartLogin}
                    disabled={isLoggingIn}
                    className="w-full py-2 px-3 rounded-[4px] bg-indigo-600 hover:bg-indigo-700 text-white font-semibold text-xs flex items-center justify-center gap-1.5 transition-colors shadow-xs"
                  >
                    {isLoggingIn ? (
                      <>
                        <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        Menyiapkan...
                      </>
                    ) : (
                      <>
                        <LogIn className="w-3.5 h-3.5" />
                        Login Google
                      </>
                    )}
                  </button>
                )}
                <button
                  onClick={() => {
                    onActivate?.(currentWorker);
                  }}
                  className="w-full py-1.5 px-3 rounded-[4px] bg-slate-50 hover:bg-slate-100 text-slate-700 font-medium text-xs flex items-center justify-center gap-1.5 border border-slate-200 transition-colors"
                >
                  <Power className="w-3 h-3" />
                  Aktifkan
                </button>
              </div>
            )}
          </div>
        )}

        {activeSubTab === 'transcripts' && (
          <div className="space-y-2">
            {loadingTranscripts ? (
              <div className="flex items-center justify-center py-6 text-slate-500 gap-1.5 text-xs">
                <Loader2 className="w-3.5 h-3.5 animate-spin text-slate-700" />
                Memuat...
              </div>
            ) : transcripts.length === 0 ? (
              <div className="text-center py-6 text-slate-400 text-xs">
                Kosong
              </div>
            ) : (
              transcripts.map((t) => (
                <div
                  key={t.conversation_id}
                  className="p-3 rounded-[4px] bg-white border border-slate-200 shadow-xs space-y-1"
                >
                  <div className="flex items-center justify-between text-[10px] text-slate-500 font-mono">
                    <span className="truncate max-w-[180px] font-semibold text-slate-900">
                      {t.conversation_id.slice(0, 8)}
                    </span>
                    <span>{new Date(t.last_updated_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                  </div>
                  <div className="text-xs font-medium text-slate-800 line-clamp-2">
                    {t.first_prompt}
                  </div>
                  {t.last_response && (
                    <div className="text-[11px] text-slate-600 line-clamp-2 bg-slate-50 p-2 rounded-[4px] border border-slate-200/80">
                      {t.last_response}
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        )}

        {activeSubTab === 'prompt' && (
          <div className="space-y-2.5">
            <div className="space-y-1.5">
              <textarea
                value={promptText}
                onChange={(e) => setPromptText(e.target.value)}
                placeholder="Ketik prompt..."
                rows={3}
                disabled={isStreaming}
                className="w-full p-2.5 rounded-[4px] bg-white border border-slate-200 text-slate-900 text-xs focus:outline-none focus:border-slate-400 transition-colors disabled:opacity-50 resize-none font-sans shadow-xs"
              />
              <button
                onClick={handleSendPrompt}
                disabled={isStreaming || !promptText.trim()}
                className="w-full py-2 px-3 rounded-[4px] bg-slate-900 hover:bg-slate-800 disabled:bg-slate-100 disabled:text-slate-400 text-white font-semibold text-xs flex items-center justify-center gap-1.5 transition-colors shadow-xs"
              >
                {isStreaming ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    Memproses...
                  </>
                ) : (
                  <>
                    <Send className="w-3.5 h-3.5" />
                    Kirim
                  </>
                )}
              </button>
            </div>

            {toolBadges.length > 0 && (
              <div className="space-y-1">
                <div className="flex flex-wrap gap-1">
                  {toolBadges.map((tool, idx) => (
                    <span
                      key={idx}
                      className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-[4px] text-[10px] font-mono bg-slate-100 text-slate-700 border border-slate-200"
                    >
                      <Terminal className="w-2.5 h-2.5" />
                      {tool.name}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {(streamingText || isStreaming) && (
              <div className="p-2.5 rounded-[4px] bg-slate-950 border border-slate-800 space-y-1">
                <div className="text-[10px] text-slate-400 font-mono uppercase">
                  {isStreaming ? 'Streaming...' : 'Output'}
                </div>
                <div className="text-xs font-mono text-slate-100 whitespace-pre-wrap max-h-48 overflow-y-auto leading-relaxed">
                  {streamingText || 'Menunggu...'}
                </div>
              </div>
            )}

            {streamResult && (
              <div className="p-2.5 rounded-[4px] bg-emerald-50 border border-emerald-200 space-y-1">
                <div className="flex items-center gap-1 text-xs font-semibold text-emerald-800">
                  <CheckCircle2 className="w-3.5 h-3.5" />
                  Hasil
                </div>
                <p className="text-xs text-slate-800 leading-relaxed font-sans">
                  {streamResult}
                </p>
              </div>
            )}

            {streamError && (
              <div className="p-2.5 rounded-[4px] bg-rose-50 border border-rose-200 space-y-1">
                <div className="flex items-center gap-1 text-xs font-semibold text-rose-700">
                  <AlertCircle className="w-3.5 h-3.5" />
                  Gagal
                </div>
                <p className="text-xs text-rose-800 leading-relaxed font-mono">
                  {streamError}
                </p>
              </div>
            )}
          </div>
        )}
      </div>
    </BottomSheet>
  );
};
