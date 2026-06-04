import React, { useState } from 'react';
import { GitBranch, AlertTriangle, Check, SkipForward } from 'lucide-react';
import { useAppStore, PendingRemoteSetup } from '../stores/appStore';

interface RemoteSetupModalProps {
  setup: PendingRemoteSetup;
}

export const RemoteSetupModal: React.FC<RemoteSetupModalProps> = ({ setup }) => {
  const { confirmRemoteSetup, skipRemoteSetup } = useAppStore();
  const [selectedRemote, setSelectedRemote] = useState<string>(setup.remotes[0] ?? '');
  const [isConfirming, setIsConfirming] = useState(false);

  const handleConfirm = async () => {
    if (!selectedRemote) return;
    setIsConfirming(true);
    await confirmRemoteSetup(selectedRemote);
    setIsConfirming(false);
  };

  const handleSkip = async () => {
    await skipRemoteSetup();
  };

  const { pr } = setup;

  return (
    // Backdrop
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      {/* Modal card */}
      <div className="relative w-[420px] bg-[#161b22] border border-zinc-700 rounded-xl shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center gap-3 px-5 py-4 border-b border-zinc-800 bg-amber-950/20">
          <AlertTriangle size={18} className="text-amber-400 shrink-0" />
          <div className="flex-1 min-w-0">
            <h2 className="text-sm font-semibold text-amber-300">No Remote Tracking Configured</h2>
            <p className="text-[11px] text-zinc-400 mt-0.5 truncate">
              PR #{pr.number} — {pr.title}
            </p>
          </div>
        </div>

        {/* Body */}
        <div className="px-5 py-4 space-y-4">
          {/* Branch info */}
          <div className="flex items-center gap-2 p-3 bg-zinc-900/60 rounded-lg border border-zinc-800">
            <GitBranch size={14} className="text-zinc-400 shrink-0" />
            <div className="min-w-0">
              <p className="text-[11px] text-zinc-400">Head branch</p>
              <p className="text-xs font-mono text-zinc-200 truncate">{pr.head_branch}</p>
            </div>
          </div>

          <p className="text-xs text-zinc-400 leading-relaxed">
            This branch has no upstream remote tracking configured. Select a remote below
            to set tracking and proceed with the rebase{setup.pendingRequests.length > 1 ? ' (this is one of several pending jobs)' : ''}.
          </p>

          {/* Remote options */}
          {setup.remotes.length === 0 ? (
            <div className="p-3 bg-red-950/30 border border-red-900/50 rounded-lg text-xs text-red-400">
              No remotes found in this repository. Please add a remote with{' '}
              <code className="font-mono bg-red-950/40 px-1 rounded">git remote add &lt;name&gt; &lt;url&gt;</code>{' '}
              and try again.
            </div>
          ) : (
            <div className="space-y-1.5">
              <p className="text-[11px] font-medium text-zinc-400 uppercase tracking-wide">
                Available remotes
              </p>
              {setup.remotes.map(remote => (
                <label
                  key={remote}
                  className={`flex items-center gap-3 px-3 py-2.5 rounded-lg border cursor-pointer transition-all ${
                    selectedRemote === remote
                      ? 'border-blue-600/70 bg-blue-950/30 text-blue-300'
                      : 'border-zinc-800 hover:border-zinc-700 text-zinc-300 hover:bg-zinc-800/40'
                  }`}
                >
                  <input
                    type="radio"
                    name="remote"
                    value={remote}
                    checked={selectedRemote === remote}
                    onChange={() => setSelectedRemote(remote)}
                    className="accent-blue-500"
                  />
                  <span className="font-mono text-xs">{remote}</span>
                  {selectedRemote === remote && (
                    <span className="ml-auto text-[10px] text-blue-400 font-medium">
                      {remote}/{pr.head_branch}
                    </span>
                  )}
                </label>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between gap-2 px-5 py-3 border-t border-zinc-800 bg-zinc-900/30">
          <button
            onClick={handleSkip}
            disabled={isConfirming}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded transition disabled:opacity-50"
          >
            <SkipForward size={13} />
            Skip this PR
          </button>

          <button
            onClick={handleConfirm}
            disabled={!selectedRemote || setup.remotes.length === 0 || isConfirming}
            className="flex items-center gap-1.5 px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded text-xs font-semibold transition"
          >
            {isConfirming ? (
              <>
                <span className="animate-spin inline-block w-3 h-3 border border-white/30 border-t-white rounded-full" />
                Setting tracking...
              </>
            ) : (
              <>
                <Check size={13} />
                Set Tracking &amp; Continue
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
