import React from 'react';
import { LayoutDashboard, Users, MessageSquare, Terminal, Settings } from 'lucide-react';
import type { TabType } from '../../types/telemetry.ts';

interface TabBarProps {
  activeTab: TabType;
  onTabChange: (tab: TabType) => void;
}

interface TabItem {
  id: TabType;
  name: 'Overview' | 'Workers' | 'Transcripts' | 'Console' | 'Settings';
  label: string;
  Icon: React.ComponentType<{ className?: string }>;
}

const TABS: TabItem[] = [
  { id: 'overview', name: 'Overview', label: 'Overview', Icon: LayoutDashboard },
  { id: 'workers', name: 'Workers', label: 'Workers', Icon: Users },
  { id: 'transcripts', name: 'Transcripts', label: 'Logs', Icon: MessageSquare },
  { id: 'console', name: 'Console', label: 'Console', Icon: Terminal },
  { id: 'settings', name: 'Settings', label: 'Settings', Icon: Settings }
];

export const TabBar: React.FC<TabBarProps> = ({ activeTab, onTabChange }) => {
  return (
    <nav className="fixed bottom-0 inset-x-0 max-w-md mx-auto w-full z-40 bg-white/95 backdrop-blur-md border-t border-slate-200 pb-[env(safe-area-inset-bottom,12px)]">
      <div className="flex items-center justify-around h-14 px-2">
        {TABS.map(({ id, label, Icon }) => {
          const isActive = activeTab === id;
          return (
            <button
              key={id}
              onClick={() => onTabChange(id)}
              className={`flex flex-col items-center justify-center flex-1 py-1 rounded-[4px] transition-colors duration-150 ${
                isActive ? 'text-slate-900 font-semibold bg-slate-100/70' : 'text-slate-500 hover:text-slate-800'
              }`}
            >
              <Icon className={`w-4 h-4 mb-0.5 ${isActive ? 'stroke-[2.2]' : 'stroke-[1.6]'}`} />
              <span className="text-[10px] tracking-tight">{label}</span>
            </button>
          );
        })}
      </div>
    </nav>
  );
};
