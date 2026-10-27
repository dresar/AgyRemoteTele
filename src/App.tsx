import React, { useState, useEffect } from 'react';
import { TabBar } from './components/Navigation/TabBar.tsx';
import { OverviewTab } from './components/Tabs/OverviewTab.tsx';
import { WorkersTab } from './components/Tabs/WorkersTab.tsx';
import { TranscriptsTab } from './components/Tabs/TranscriptsTab.tsx';
import { ConsoleTab } from './components/Tabs/ConsoleTab.tsx';
import { SettingsTab } from './components/Tabs/SettingsTab.tsx';
import { PromptPage } from './components/Pages/PromptPage.tsx';
import { WorkerDetailSheet } from './components/Sheets/WorkerDetailSheet.tsx';
import { fetchVitals, fetchWorkers, activateWorker } from './services/api.ts';
import type { TabType, WorkerProfile, SystemVitals } from './types/telemetry.ts';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [vitals, setVitals] = useState<SystemVitals | null>(null);
  const [workers, setWorkers] = useState<WorkerProfile[]>([]);
  const [loadingVitals, setLoadingVitals] = useState(false);
  const [loadingWorkers, setLoadingWorkers] = useState(false);

  const [selectedWorker, setSelectedWorker] = useState<WorkerProfile | null>(null);
  const [isWorkerSheetOpen, setIsWorkerSheetOpen] = useState(false);
  const [promptingWorker, setPromptingWorker] = useState<WorkerProfile | null>(null);

  const loadData = async () => {
    setLoadingVitals(true);
    setLoadingWorkers(true);

    try {
      const [vData, wData] = await Promise.all([
        fetchVitals().catch(() => null),
        fetchWorkers().catch(() => [])
      ]);
      if (vData) setVitals(vData);
      if (wData) setWorkers(wData);
    } finally {
      setLoadingVitals(false);
      setLoadingWorkers(false);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 8000);
    return () => clearInterval(interval);
  }, []);

  const handleSelectWorker = (worker: WorkerProfile) => {
    setSelectedWorker(worker);
    setIsWorkerSheetOpen(true);
  };

  const handleLaunchPrompt = (worker: WorkerProfile) => {
    setIsWorkerSheetOpen(false);
    setPromptingWorker(worker);
  };

  const handleActivateWorker = async (worker: WorkerProfile) => {
    try {
      await activateWorker(worker.id);
      loadData();
    } catch {}
  };

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900 flex flex-col items-center overflow-x-hidden">
      <div
        className={`w-full max-w-md min-w-[360px] mx-auto flex flex-col relative px-4 pt-3 ${
          promptingWorker ? 'h-[100dvh] pb-16 overflow-hidden' : 'min-h-screen pb-24 overflow-x-hidden'
        }`}
      >
        <header className="flex items-center justify-between py-2 mb-2 shrink-0 border-b border-slate-200">
          <div className="flex items-center gap-2">
            <img
              src="/logo.png"
              alt="ORCAGY"
              className="w-7 h-7 rounded-[4px] border border-slate-200 object-cover shadow-xs"
            />
            <div className="flex items-center gap-1.5">
              <span className="font-bold text-sm tracking-tight text-slate-900 font-mono">
                AgyRemote
              </span>
              <span className="inline-block w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
            </div>
          </div>
          <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] bg-slate-100 text-slate-700 text-[10px] font-mono font-semibold border border-slate-200">
            <span>2 vCPU</span>
            <span className="text-slate-300">•</span>
            <span>4 GB</span>
          </div>
        </header>

        <main
          className={`w-full ${
            promptingWorker
              ? 'flex-1 flex flex-col min-h-0 overflow-hidden'
              : 'flex-1 transition-opacity duration-150 motion-reduce:transition-none'
          }`}
        >
          {promptingWorker ? (
            <PromptPage
              worker={promptingWorker}
              onBack={() => setPromptingWorker(null)}
              onActivate={handleActivateWorker}
            />
          ) : (
            <>
              {activeTab === 'overview' && (
                <OverviewTab
                  vitals={vitals}
                  workers={workers}
                  loadingVitals={loadingVitals}
                  loadingWorkers={loadingWorkers}
                  onRefresh={loadData}
                  onSelectWorker={handleSelectWorker}
                  onLaunchPrompt={handleLaunchPrompt}
                  onActivate={handleActivateWorker}
                />
              )}

              {activeTab === 'workers' && (
                <WorkersTab
                  workers={workers}
                  loading={loadingWorkers}
                  onRefresh={loadData}
                  onSelectWorker={handleSelectWorker}
                  onLaunchPrompt={handleLaunchPrompt}
                  onActivate={handleActivateWorker}
                />
              )}

              {activeTab === 'transcripts' && (
                <TranscriptsTab workers={workers} />
              )}

              {activeTab === 'console' && (
                <ConsoleTab />
              )}

              {activeTab === 'settings' && (
                <SettingsTab />
              )}
            </>
          )}
        </main>

        <TabBar
          activeTab={activeTab}
          onTabChange={(tab) => {
            setPromptingWorker(null);
            setActiveTab(tab);
          }}
        />

        <WorkerDetailSheet
          worker={selectedWorker}
          isOpen={isWorkerSheetOpen}
          onClose={() => setIsWorkerSheetOpen(false)}
          onPromptClick={() => {
            if (selectedWorker) {
              handleLaunchPrompt(selectedWorker);
            }
          }}
          onActivate={handleActivateWorker}
          onWorkerUpdated={(updated) => {
            setSelectedWorker(updated);
            loadData();
          }}
        />
      </div>
    </div>
  );
};

export default App;
