import React, { useEffect, useState } from 'react';
import { useAppStore } from '../stores/appStore';
import { ExternalLink, CheckCircle2, XCircle, AlertCircle, RefreshCw } from 'lucide-react';

export const DetailsPanel: React.FC = () => {
  const { selectedPR, settings, setSettings, rebaseSelected, toggleSelectPR, selectedPRIds } = useAppStore();
  const [ciStatus, setCiStatus] = useState<string>('loading');
  const [isRefreshingCi, setIsRefreshingCi] = useState(false);

  const fetchCI = async () => {
    if (!selectedPR) return;
    setIsRefreshingCi(true);
    try {
      const status = await window.go.main.App.GetPRCIStatus(selectedPR.repo_id, selectedPR.head_branch);
      setCiStatus(status);
    } catch (err) {
      setCiStatus('unknown');
    } finally {
      setIsRefreshingCi(false);
    }
  };

  useEffect(() => {
    if (selectedPR) {
      setCiStatus('loading');
      fetchCI();
    }
  }, [selectedPR]);

  if (!selectedPR) {
    return (
      <div className="w-80 bg-[#161b22] border-l border-zinc-800 flex items-center justify-center p-4 text-center select-none">
        <p className="text-xs text-gray-500">Select a Pull Request to view details</p>
      </div>
    );
  }

  const handleAction = async (amend: boolean, push: boolean) => {
    if (!settings) return;
    // Set temporary rebase configurations
    await setSettings({
      ...settings,
      amend_commit_timestamp: amend,
      force_push_after_rebase: push
    });
    
    // Select only this PR to run rebase on
    const currentSelected = [...selectedPRIds];
    toggleSelectPR(selectedPR.id);
    
    await rebaseSelected();
    
    // Restore selection
    if (!currentSelected.includes(selectedPR.id)) {
      toggleSelectPR(selectedPR.id);
    }
  };

  return (
    <div className="w-80 bg-[#161b22] border-l border-zinc-800 flex flex-col h-full overflow-hidden text-xs">
      {/* Header */}
      <div className="p-4 border-b border-zinc-800 flex flex-col gap-1 bg-[#0d1117]/10">
        <div className="flex items-start justify-between gap-2">
          <span className="font-semibold text-gray-400">PR #{selectedPR.number}</span>
          <a
            href={selectedPR.html_url}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-blue-500 hover:text-blue-400 font-medium transition"
          >
            Open GitHub <ExternalLink size={12} />
          </a>
        </div>
        <h3 className="font-bold text-gray-200 text-sm leading-tight mt-1" title={selectedPR.title}>
          {selectedPR.title}
        </h3>
        <span className="text-[10px] text-zinc-500 truncate mt-1">Repo: {selectedPR.repo_name}</span>
      </div>

      {/* Body content scrollable */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Status checks card */}
        <div className="bg-[#0d1117] border border-zinc-850 rounded-lg p-3">
          <div className="flex items-center justify-between mb-2">
            <span className="font-semibold text-gray-400">CI Status</span>
            <button
              onClick={fetchCI}
              className="text-zinc-500 hover:text-zinc-300 transition"
              title="Refresh Checks"
            >
              <RefreshCw size={11} className={isRefreshingCi ? 'animate-spin' : ''} />
            </button>
          </div>

          <div className="flex items-center gap-2 mt-1">
            {ciStatus === 'success' ? (
              <>
                <CheckCircle2 size={16} className="text-green-500" />
                <span className="text-green-400 font-medium">All checks passed successfully</span>
              </>
            ) : ciStatus === 'failure' ? (
              <>
                <XCircle size={16} className="text-red-500" />
                <span className="text-red-400 font-medium">Some status checks failed</span>
              </>
            ) : ciStatus === 'running' ? (
              <>
                <RefreshCw size={14} className="animate-spin text-amber-500" />
                <span className="text-amber-400 font-medium">CI checks running...</span>
              </>
            ) : ciStatus === 'loading' ? (
              <>
                <RefreshCw size={14} className="animate-spin text-zinc-500" />
                <span className="text-zinc-500">Querying check-runs...</span>
              </>
            ) : (
              <>
                <AlertCircle size={16} className="text-zinc-500" />
                <span className="text-zinc-400">No status checks reported</span>
              </>
            )}
          </div>
        </div>

        {/* Branch references info */}
        <div>
          <h4 className="font-semibold text-gray-400 mb-1.5">Branch Configuration</h4>
          <div className="bg-[#0d1117] border border-zinc-850 rounded-lg p-2.5 space-y-1 text-[11px]">
            <div className="flex justify-between">
              <span className="text-gray-500">Head:</span>
              <span className="font-mono text-zinc-300 truncate max-w-[160px]" title={selectedPR.head_branch}>
                {selectedPR.head_branch}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Base:</span>
              <span className="font-mono text-zinc-300 truncate max-w-[160px]" title={selectedPR.base_branch}>
                {selectedPR.base_branch}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Status:</span>
              <span className={`font-semibold capitalize ${
                selectedPR.state === 'open' ? 'text-green-500' : 'text-zinc-500'
              }`}>
                {selectedPR.state} {selectedPR.is_draft && '(Draft)'}
              </span>
            </div>
          </div>
        </div>

        {/* PR description */}
        <div>
          <h4 className="font-semibold text-gray-400 mb-1">Description</h4>
          <div className="bg-[#0d1117]/30 border border-zinc-850 rounded-lg p-2.5 max-h-40 overflow-y-auto text-gray-400 leading-normal text-[11px] font-sans break-words whitespace-pre-wrap">
            {selectedPR.description || <span className="text-gray-600 italic">No description provided</span>}
          </div>
        </div>
      </div>

      {/* Action footer */}
      <div className="p-4 border-t border-zinc-800 bg-[#0d1117]/20 flex flex-col gap-2">
        <button
          onClick={() => handleAction(true, false)}
          className="w-full py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded font-medium action-btn text-xs"
        >
          Rebase Branch
        </button>
        <button
          onClick={() => handleAction(true, true)}
          className="w-full py-1.5 bg-[#238636] hover:bg-[#2ea043] text-white rounded font-medium action-btn text-xs"
        >
          Rebase + Force Push
        </button>
      </div>
    </div>
  );
};
