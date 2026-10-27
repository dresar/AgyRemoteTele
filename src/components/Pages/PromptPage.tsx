import React, { useState, useEffect, useRef } from 'react';
import type { WorkerProfile } from '../../types/telemetry.ts';
import { streamWorkerPrompt } from '../../services/sse.ts';
import { fetchWorkerChanges, type FileChangeItem } from '../../services/api.ts';
import {
  ArrowLeft,
  Send,
  Square,
  Bot,
  FileCode,
  FilePlus,
  FileX,
  FileText,
  Terminal,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Layers,
  Sparkles,
  Power
} from 'lucide-react';

interface PromptPageProps {
  worker: WorkerProfile;
  onBack: () => void;
  onActivate?: (worker: WorkerProfile) => void;
}

interface ExecutionStep {
  name: string;
  state: string;
  path?: string;
  action?: string;
  detail?: string;
}

interface ChatMessage {
  id: string;
  sender: 'user' | 'agent';
  content: string;
  timestamp: string;
  steps?: ExecutionStep[];
}

export const PromptPage: React.FC<PromptPageProps> = ({ worker, onBack }) => {
  const [activeTab, setActiveTab] = useState<'chat' | 'files'>('chat');
  const [promptText, setPromptText] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [currentSteps, setCurrentSteps] = useState<ExecutionStep[]>([]);
  const [streamingDelta, setStreamingDelta] = useState('');
  const [fileChanges, setFileChanges] = useState<FileChangeItem[]>([]);
  const [fileFilter, setFileFilter] = useState<'all' | 'modified' | 'created' | 'deleted'>('all');
  const [loadingChanges, setLoadingChanges] = useState(false);

  const abortRef = useRef<(() => void) | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const loadChanges = () => {
    setLoadingChanges(true);
    fetchWorkerChanges(worker.id)
      .then((data) => {
        if (data && data.changes) {
          setFileChanges(data.changes);
        }
      })
      .catch(() => {})
      .finally(() => setLoadingChanges(false));
  };

  useEffect(() => {
    loadChanges();
  }, [worker.id]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamingDelta, currentSteps]);

  const handleSend = () => {
    const text = promptText.trim();
    if (!text || isStreaming) return;

    const userMsg: ChatMessage = {
      id: `u_${Date.now()}`,
      sender: 'user',
      content: text,
      timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    };

    setMessages((prev) => [...prev, userMsg]);
    setPromptText('');
    setIsStreaming(true);
    setCurrentSteps([]);
    setStreamingDelta('');

    const cancel = streamWorkerPrompt(
      worker.id,
      text,
      {
        onDelta: (chunk) => {
          setStreamingDelta((prev) => prev + chunk);
        },
        onTool: (tool) => {
          setCurrentSteps((prev) => {
            const copy = [...prev];
            const existingIdx = copy.findIndex((s) => s.name === tool.name && s.path === tool.path);
            if (existingIdx >= 0) {
              copy[existingIdx] = tool;
              return copy;
            }
            return [...copy, tool];
          });
        },
        onFileChange: (file) => {
          setFileChanges((prev) => {
            const exists = prev.some((f) => f.path === file.path);
            if (exists) return prev;
            return [
              {
                path: file.path,
                name: file.name,
                action: file.action,
                timestamp: new Date().toISOString()
              },
              ...prev
            ];
          });
        },
        onResult: (result) => {
          const agentMsg: ChatMessage = {
            id: `a_${Date.now()}`,
            sender: 'agent',
            content: streamingDelta || result.summary || 'Selesai',
            timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
            steps: currentSteps
          };
          setMessages((prev) => [...prev, agentMsg]);
          setIsStreaming(false);
          setStreamingDelta('');
          setCurrentSteps([]);
          loadChanges();
        },
        onError: (err) => {
          const errMsg: ChatMessage = {
            id: `err_${Date.now()}`,
            sender: 'agent',
            content: `Error: ${err}`,
            timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
          };
          setMessages((prev) => [...prev, errMsg]);
          setIsStreaming(false);
          setStreamingDelta('');
          setCurrentSteps([]);
        },
        onClose: () => {
          setIsStreaming(false);
        }
      }
    );

    abortRef.current = cancel;
  };

  const handleStop = () => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
    }
    setIsStreaming(false);
  };

  const handleShortcut = (cmd: string) => {
    setPromptText((prev) => (prev ? `${prev} ${cmd} ` : `${cmd} `));
  };

  const filteredFiles = fileChanges.filter((f) => {
    if (fileFilter === 'all') return true;
    return f.action === fileFilter;
  });

  const modifiedCount = fileChanges.filter((f) => f.action === 'modified').length;
  const createdCount = fileChanges.filter((f) => f.action === 'created').length;
  const deletedCount = fileChanges.filter((f) => f.action === 'deleted').length;

  return (
    <div className="w-full flex flex-col flex-1 h-full min-h-0 overflow-hidden">
      <div className="shrink-0 bg-slate-50 pb-2 pt-1 border-b border-slate-200 mb-2 space-y-2">
        <div className="flex items-center justify-between">
          <button
            onClick={onBack}
            className="flex items-center gap-1.5 px-2.5 py-1 rounded-[4px] bg-white border border-slate-200 text-xs font-semibold text-slate-800 hover:bg-slate-100 transition-colors shadow-xs"
          >
            <ArrowLeft className="w-3.5 h-3.5" />
            <span>Kembali</span>
          </button>

          <div className="flex items-center gap-2">
            <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] bg-white border border-slate-200 text-xs font-mono font-bold text-slate-900 shadow-xs">
              <Bot className="w-3.5 h-3.5 text-indigo-600" />
              <span>{worker.id.toUpperCase()}</span>
            </div>
            <span
              className={`px-2 py-0.5 rounded-[4px] text-[10px] font-semibold border ${
                worker.authenticated
                  ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
                  : 'bg-slate-100 text-slate-500 border-slate-200'
              }`}
            >
              {worker.authenticated ? 'Aktif' : 'Off'}
            </span>
          </div>
        </div>

        {!worker.authenticated && (
          <div className="p-2 rounded-[4px] bg-amber-50 border border-amber-200 flex items-center justify-between gap-2 shadow-xs">
            <span className="text-[11px] text-amber-800 font-medium">Antigravity non-aktif</span>
            <button
              onClick={() => onActivate?.(worker)}
              className="px-2 py-0.5 rounded-[4px] bg-indigo-600 hover:bg-indigo-700 text-white text-[11px] font-semibold flex items-center gap-1 shadow-xs transition-colors"
            >
              <Power className="w-2.5 h-2.5" />
              <span>Aktifkan</span>
            </button>
          </div>
        )}

        <div className="flex gap-1.5">
          <button
            onClick={() => setActiveTab('chat')}
            className={`flex-1 py-1.5 px-3 rounded-[4px] text-xs font-semibold flex items-center justify-center gap-1.5 transition-colors border shadow-xs ${
              activeTab === 'chat'
                ? 'bg-slate-900 text-white border-slate-900'
                : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-100'
            }`}
          >
            <Sparkles className="w-3.5 h-3.5" />
            <span>Prompt</span>
          </button>
          <button
            onClick={() => setActiveTab('files')}
            className={`flex-1 py-1.5 px-3 rounded-[4px] text-xs font-semibold flex items-center justify-center gap-1.5 transition-colors border shadow-xs ${
              activeTab === 'files'
                ? 'bg-slate-900 text-white border-slate-900'
                : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-100'
            }`}
          >
            <Layers className="w-3.5 h-3.5" />
            <span>File Berubah</span>
            {fileChanges.length > 0 && (
              <span
                className={`text-[10px] px-1.5 py-0.2 rounded-[4px] ${
                  activeTab === 'files'
                    ? 'bg-slate-800 text-slate-200'
                    : 'bg-indigo-50 text-indigo-700 border border-indigo-200'
                }`}
              >
                {fileChanges.length}
              </span>
            )}
          </button>
        </div>
      </div>

      {activeTab === 'chat' ? (
        <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
          <div className="flex-1 overflow-y-auto space-y-2.5 px-0.5 pb-2 min-h-0">
            {messages.length === 0 && !isStreaming && (
              <div className="card p-4 text-center space-y-2 my-auto">
                <div className="w-8 h-8 rounded-[4px] bg-indigo-50 text-indigo-600 mx-auto flex items-center justify-center border border-indigo-100">
                  <Bot className="w-4 h-4" />
                </div>
                <h3 className="text-xs font-bold text-slate-900 font-mono">
                  {worker.id.toUpperCase()} SIAP
                </h3>
                <p className="text-[11px] text-slate-500 max-w-xs mx-auto">
                  Kirim perintah untuk mengeksekusi tugas. Perubahan file dan alur kerja akan terpantau ringkas secara real-time.
                </p>
              </div>
            )}

            {messages.map((m) => (
              <div
                key={m.id}
                className={`flex flex-col ${m.sender === 'user' ? 'items-end' : 'items-start'}`}
              >
                <div className="flex items-center gap-1.5 mb-1 px-1 text-[10px] text-slate-400 font-mono">
                  <span>{m.sender === 'user' ? 'Operator' : worker.id.toUpperCase()}</span>
                  <span>•</span>
                  <span>{m.timestamp}</span>
                </div>

                <div
                  className={`max-w-[90%] p-3 rounded-[4px] text-xs leading-relaxed ${
                    m.sender === 'user'
                      ? 'bg-slate-900 text-white shadow-xs'
                      : 'card text-slate-900'
                  }`}
                >
                  <div className="whitespace-pre-wrap">{m.content}</div>

                  {m.steps && m.steps.length > 0 && (
                    <div className="mt-2.5 pt-2 border-t border-slate-100 space-y-1">
                      <div className="text-[10px] font-semibold text-slate-500 uppercase">
                        Eksekusi ({m.steps.length})
                      </div>
                      <div className="flex flex-wrap gap-1">
                        {m.steps.map((st, idx) => (
                          <span
                            key={idx}
                            className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-[4px] text-[10px] font-mono bg-slate-50 border border-slate-200 text-slate-700"
                          >
                            <Terminal className="w-2.5 h-2.5 text-slate-500" />
                            <span>{st.name}</span>
                            {st.path && (
                              <span className="text-slate-500 max-w-[120px] truncate">
                                {st.path.split('/').pop()}
                              </span>
                            )}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            ))}

            {isStreaming && (
              <div className="flex flex-col items-start space-y-2">
                <div className="flex items-center gap-1.5 px-1 text-[10px] text-slate-400 font-mono">
                  <span>{worker.id.toUpperCase()}</span>
                  <span>•</span>
                  <span className="flex items-center gap-1 text-indigo-600 font-semibold">
                    <Loader2 className="w-3 h-3 animate-spin" />
                    Memproses...
                  </span>
                </div>

                {currentSteps.length > 0 && (
                  <div className="card card--accent p-2.5 w-full space-y-1.5">
                    <div className="text-[10px] font-bold uppercase tracking-wider text-slate-500 flex items-center justify-between">
                      <span>Langkah Real-Time</span>
                      <span className="font-mono text-slate-400">{currentSteps.length} aktif</span>
                    </div>
                    <div className="space-y-1 max-h-32 overflow-y-auto pr-1">
                      {currentSteps.map((step, idx) => (
                        <div
                          key={idx}
                          className="flex items-center justify-between text-[11px] font-mono py-1 px-2 rounded-[4px] bg-slate-50 border border-slate-200/80"
                        >
                          <div className="flex items-center gap-1.5 min-w-0">
                            {step.action === 'modified' ? (
                              <FileCode className="w-3 h-3 text-amber-600 shrink-0" />
                            ) : step.action === 'created' ? (
                              <FilePlus className="w-3 h-3 text-emerald-600 shrink-0" />
                            ) : step.action === 'deleted' ? (
                              <FileX className="w-3 h-3 text-rose-600 shrink-0" />
                            ) : (
                              <Terminal className="w-3 h-3 text-indigo-600 shrink-0" />
                            )}
                            <span className="font-semibold text-slate-900">{step.name}</span>
                            {step.path && (
                              <span className="text-slate-500 truncate max-w-[160px]">
                                {step.path.split('/').pop()}
                              </span>
                            )}
                          </div>
                          <span className="text-[10px] px-1 rounded-[4px] bg-white border border-slate-200 text-slate-600 shrink-0">
                            {step.state}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {streamingDelta && (
                  <div className="card p-3 max-w-[90%] text-xs leading-relaxed text-slate-900">
                    <div className="whitespace-pre-wrap">{streamingDelta}</div>
                  </div>
                )}
              </div>
            )}

            <div ref={messagesEndRef} />
          </div>

          <div className="shrink-0 space-y-2 pt-2 border-t border-slate-200 bg-slate-50 mt-auto">
            <div className="flex gap-1 overflow-x-auto pb-0.5 scrollbar-none">
              {['/plan', '/learn', '/boost', '/teamwork', '/kill'].map((cmd) => (
                <button
                  key={cmd}
                  onClick={() => handleShortcut(cmd)}
                  className="px-2 py-0.5 rounded-[4px] bg-white border border-slate-200 hover:bg-slate-100 text-slate-700 text-[10px] font-mono font-medium whitespace-nowrap shadow-xs transition-colors"
                >
                  {cmd}
                </button>
              ))}
            </div>

            <div className="flex gap-2 items-end">
              <textarea
                value={promptText}
                onChange={(e) => setPromptText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    handleSend();
                  }
                }}
                placeholder="Ketik perintah..."
                rows={2}
                disabled={isStreaming}
                className="flex-1 p-2.5 rounded-[4px] bg-white border border-slate-200 text-xs text-slate-900 placeholder:text-slate-400 focus:outline-none focus:border-slate-400 disabled:opacity-50 resize-none font-sans shadow-xs"
              />

              {isStreaming ? (
                <button
                  onClick={handleStop}
                  className="h-10 px-3 rounded-[4px] bg-rose-600 hover:bg-rose-500 text-white font-semibold text-xs flex items-center justify-center gap-1 transition-colors shadow-xs"
                  title="Hentikan"
                >
                  <Square className="w-3.5 h-3.5 fill-current" />
                  <span>Stop</span>
                </button>
              ) : (
                <button
                  onClick={handleSend}
                  disabled={!promptText.trim()}
                  className="h-10 px-4 rounded-[4px] bg-slate-900 hover:bg-slate-800 disabled:bg-slate-200 disabled:text-slate-400 text-white font-semibold text-xs flex items-center justify-center gap-1 transition-colors shadow-xs"
                >
                  <Send className="w-3.5 h-3.5" />
                  <span>Kirim</span>
                </button>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div className="space-y-3 flex-1">
          <div className="card p-3 space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="font-semibold text-slate-900">Audit Berkas</span>
              <span className="font-mono text-slate-500">{fileChanges.length} total</span>
            </div>

            <div className="grid grid-cols-3 gap-2 text-center pt-1 border-t border-slate-100">
              <div className="p-1.5 rounded-[4px] bg-amber-50 border border-amber-200/70">
                <div className="text-sm font-bold text-amber-800 font-mono">{modifiedCount}</div>
                <div className="text-[10px] text-amber-700 font-medium">Diedit</div>
              </div>
              <div className="p-1.5 rounded-[4px] bg-emerald-50 border border-emerald-200/70">
                <div className="text-sm font-bold text-emerald-800 font-mono">{createdCount}</div>
                <div className="text-[10px] text-emerald-700 font-medium">Dibuat</div>
              </div>
              <div className="p-1.5 rounded-[4px] bg-rose-50 border border-rose-200/70">
                <div className="text-sm font-bold text-rose-800 font-mono">{deletedCount}</div>
                <div className="text-[10px] text-rose-700 font-medium">Dihapus</div>
              </div>
            </div>
          </div>

          <div className="flex gap-1.5 overflow-x-auto pb-0.5 scrollbar-none">
            {[
              { id: 'all', label: 'Semua', count: fileChanges.length },
              { id: 'modified', label: 'Diedit', count: modifiedCount },
              { id: 'created', label: 'Dibuat', count: createdCount },
              { id: 'deleted', label: 'Dihapus', count: deletedCount }
            ].map((f) => (
              <button
                key={f.id}
                onClick={() => setFileFilter(f.id as any)}
                className={`px-2.5 py-1 rounded-[4px] text-xs font-medium whitespace-nowrap transition-colors flex items-center gap-1.5 border shadow-xs ${
                  fileFilter === f.id
                    ? 'bg-slate-900 text-white border-slate-900 font-semibold'
                    : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-50'
                }`}
              >
                <span>{f.label}</span>
                <span
                  className={`text-[10px] px-1 rounded-[4px] ${
                    fileFilter === f.id ? 'bg-slate-800 text-slate-200' : 'bg-slate-100 text-slate-600'
                  }`}
                >
                  {f.count}
                </span>
              </button>
            ))}
          </div>

          <div className="space-y-2">
            {loadingChanges && fileChanges.length === 0 ? (
              <div className="flex items-center justify-center py-10 text-slate-400 gap-1.5 text-xs">
                <Loader2 className="w-3.5 h-3.5 animate-spin text-slate-600" />
                Memuat berkas...
              </div>
            ) : filteredFiles.length === 0 ? (
              <div className="card text-center py-8 text-slate-400 text-xs">
                Tidak ada perubahan
              </div>
            ) : (
              filteredFiles.map((file, idx) => (
                <div
                  key={idx}
                  className="card p-3 flex items-center justify-between gap-2.5"
                >
                  <div className="flex items-center gap-2.5 min-w-0">
                    <div
                      className={`p-1.5 rounded-[4px] shrink-0 border ${
                        file.action === 'modified'
                          ? 'bg-amber-50 text-amber-700 border-amber-200'
                          : file.action === 'created'
                          ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
                          : 'bg-rose-50 text-rose-700 border-rose-200'
                      }`}
                    >
                      {file.action === 'modified' ? (
                        <FileCode className="w-3.5 h-3.5" />
                      ) : file.action === 'created' ? (
                        <FilePlus className="w-3.5 h-3.5" />
                      ) : (
                        <FileX className="w-3.5 h-3.5" />
                      )}
                    </div>
                    <div className="min-w-0">
                      <div className="text-xs font-semibold text-slate-900 truncate font-mono">
                        {file.name}
                      </div>
                      <div className="text-[10px] text-slate-400 truncate font-mono">
                        {file.path}
                      </div>
                    </div>
                  </div>

                  <span
                    className={`px-1.5 py-0.5 rounded-[4px] text-[10px] font-semibold border shrink-0 uppercase font-mono ${
                      file.action === 'modified'
                        ? 'bg-amber-50 text-amber-800 border-amber-200'
                        : file.action === 'created'
                        ? 'bg-emerald-50 text-emerald-800 border-emerald-200'
                        : 'bg-rose-50 text-rose-800 border-rose-200'
                    }`}
                  >
                    {file.action === 'modified'
                      ? 'Diedit'
                      : file.action === 'created'
                      ? 'Dibuat'
                      : 'Dihapus'}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
};
