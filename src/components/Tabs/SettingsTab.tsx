import React, { useState } from 'react';
import { getAuthToken, setAuthToken } from '../../services/api.ts';
import { Key, Globe, Check } from 'lucide-react';

export const SettingsTab: React.FC = () => {
  const [tokenInput, setTokenInput] = useState(getAuthToken());
  const [saved, setSaved] = useState(false);

  const handleSaveToken = () => {
    setAuthToken(tokenInput.trim());
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="space-y-3">
      <h2 className="text-sm font-bold text-slate-900">Setelan</h2>

      <div className="card p-3.5 space-y-2.5">
        <div className="flex items-center gap-2">
          <Key className="w-4 h-4 text-indigo-600" />
          <h3 className="text-xs font-semibold text-slate-900">Token API</h3>
        </div>

        <div className="space-y-2">
          <input
            type="password"
            value={tokenInput}
            onChange={(e) => setTokenInput(e.target.value)}
            placeholder="Token..."
            className="w-full p-2 rounded-[4px] bg-white border border-slate-200 text-xs text-slate-900 font-mono focus:outline-none focus:border-slate-400 transition-colors shadow-xs"
          />
          <button
            onClick={handleSaveToken}
            className="w-full py-1.5 px-3 rounded-[4px] bg-slate-100 hover:bg-slate-200 text-slate-800 font-semibold text-xs flex items-center justify-center gap-1 border border-slate-200 transition-colors shadow-xs"
          >
            {saved ? (
              <>
                <Check className="w-3 h-3 text-emerald-600" />
                Tersimpan
              </>
            ) : (
              'Simpan'
            )}
          </button>
        </div>
      </div>

      <div className="card p-3.5 space-y-2">
        <div className="flex items-center gap-2">
          <Globe className="w-4 h-4 text-indigo-600" />
          <h3 className="text-xs font-semibold text-slate-900">Endpoint</h3>
        </div>

        <div className="space-y-1 text-xs font-mono">
          <div className="flex justify-between p-2 rounded-[4px] bg-slate-50 border border-slate-200/80">
            <span className="text-slate-500">Host</span>
            <span className="text-slate-900 font-semibold">agyremote.agentai.my.id</span>
          </div>
          <div className="flex justify-between p-2 rounded-[4px] bg-slate-50 border border-slate-200/80">
            <span className="text-slate-500">Port</span>
            <span className="text-slate-900 font-semibold">8787</span>
          </div>
        </div>
      </div>
    </div>
  );
};
