import React, { useEffect, useState } from 'react';
import { useAppStore } from '../stores/appStore';
import { ExternalLink, CheckCircle2, XCircle, AlertCircle, RefreshCw, X } from 'lucide-react';

interface DetailsPanelProps {
  /** Current panel width in px. Controlled by App.tsx drag logic. */
  width: number;
  onClose: () => void;
  /** Called with the delta px as user drags the resize handle. */
  onResizeStart: (e: React.MouseEvent) => void;
}

export const DetailsPanel: React.FC<DetailsPanelProps> = ({ width, onClose, onResizeStart }) => {
  const { selectedPR, settings, setSettings, rebaseSelected, toggleSelectPR, selectedPRIds } = useAppStore();
  const [ciStatus, setCiStatus] = useState<string>('loading');
  const [isRefreshingCi, setIsRefreshingCi] = useState(false);

  const fetchCI = async () => {
    if (!selectedPR) return;
    setIsRefreshingCi(true);
    try {
      const status = await window.go.main.App.GetPRCIStatus(selectedPR.repo_id, selectedPR.head_branch);
      setCiStatus(status);
    } catch (_) {
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

  const handleAction = async (amend: boolean, push: boolean) => {
    if (!settings || !selectedPR) return;
    await setSettings({ ...settings, amend_commit_timestamp: amend, force_push_after_rebase: push });
    const currentSelected = [...selectedPRIds];
    toggleSelectPR(selectedPR.id);
    await rebaseSelected();
    if (!currentSelected.includes(selectedPR.id)) toggleSelectPR(selectedPR.id);
  };

  return (
    <div
      className="bg-[#161b22] border-l border-zinc-800 flex flex-col h-full overflow-hidden text-xs relative"
      style={{ width }}
    >
      {/* Drag-resize handle on left edge */}
      <div
        onMouseDown={onResizeStart}
        className="absolute left-0 top-0 h-full w-1 cursor-col-resize hover:bg-blue-600/40 transition z-10 group"
        title="Drag to resize"
      >
        <div className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-12 rounded-full bg-zinc-700 group-hover:bg-blue-500 transition opacity-0 group-hover:opacity-100" />
      </div>

      {/* Header */}
      <div className="pl-3 pr-3 py-3 border-b border-zinc-800 flex flex-col gap-1 bg-[#0d1117]/10">
        <div className="flex items-center justify-between gap-2">
          {selectedPR ? (
            <span className="font-semibold text-gray-400">PR #{selectedPR.number}</span>
          ) : (
            <span className="font-semibold text-gray-500 text-[11px]">No PR selected</span>
          )}
          <div className="flex items-center gap-2">
            {selectedPR && (
              <a
                href={selectedPR.html_url}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-blue-500 hover:text-blue-400 font-medium transition"
              >
                GitHub <ExternalLink size={11} />
              </a>
            )}
            <button
              onClick={onClose}
              className="p-0.5 hover:bg-zinc-800 rounded text-zinc-500 hover:text-gray-200 transition"
              title="Close panel"
            >
              <X size={13} />
            </button>
          </div>
        </div>
        {selectedPR && (
          <>
            <h3 className="font-bold text-gray-200 text-sm leading-tight mt-0.5" title={selectedPR.title}>
              {selectedPR.title}
            </h3>
            <span className="text-[10px] text-zinc-500 truncate">Repo: {selectedPR.repo_name}</span>
          </>
        )}
      </div>

      {!selectedPR ? (
        <div className="flex-1 flex items-center justify-center p-4 text-center select-none">
          <p className="text-xs text-gray-600">Click a pull request row to view details</p>
        </div>
      ) : (
        <>
          {/* Body */}
          <div className="flex-1 overflow-y-auto p-3 space-y-4">
            {/* CI Status */}
            <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-3">
              <div className="flex items-center justify-between mb-2">
                <span className="font-semibold text-gray-400">CI Status</span>
                <button onClick={fetchCI} className="text-zinc-500 hover:text-zinc-300 transition" title="Refresh">
                  <RefreshCw size={11} className={isRefreshingCi ? 'animate-spin' : ''} />
                </button>
              </div>
              <div className="flex items-center gap-2 mt-1">
                {ciStatus === 'success' ? (
                  <><CheckCircle2 size={14} className="text-green-500" /><span className="text-green-400">All checks passed</span></>
                ) : ciStatus === 'failure' ? (
                  <><XCircle size={14} className="text-red-500" /><span className="text-red-400">Checks failed</span></>
                ) : ciStatus === 'running' ? (
                  <><RefreshCw size={12} className="animate-spin text-amber-500" /><span className="text-amber-400">Running...</span></>
                ) : ciStatus === 'loading' ? (
                  <><RefreshCw size={12} className="animate-spin text-zinc-500" /><span className="text-zinc-500">Querying...</span></>
                ) : (
                  <><AlertCircle size={14} className="text-zinc-500" /><span className="text-zinc-400">No status checks</span></>
                )}
              </div>
            </div>

            {/* Branch info */}
            <div>
              <h4 className="font-semibold text-gray-400 mb-1.5">Branch Configuration</h4>
              <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-2.5 space-y-1 text-[11px]">
                <div className="flex justify-between gap-2">
                  <span className="text-gray-500 shrink-0">Head:</span>
                  <span className="font-mono text-zinc-300 truncate" title={selectedPR.head_label || selectedPR.head_branch}>
                    {selectedPR.head_label || selectedPR.head_branch}
                  </span>
                </div>
                <div className="flex justify-between gap-2">
                  <span className="text-gray-500 shrink-0">Base:</span>
                  <span className="font-mono text-zinc-300 truncate" title={selectedPR.base_label || selectedPR.base_branch}>
                    {selectedPR.base_label || selectedPR.base_branch}
                  </span>
                </div>
                <div className="flex justify-between gap-2">
                  <span className="text-gray-500 shrink-0">Status:</span>
                  <span className={`font-semibold capitalize ${selectedPR.state === 'open' ? 'text-green-500' : 'text-zinc-500'}`}>
                    {selectedPR.state}{selectedPR.is_draft && ' (Draft)'}
                  </span>
                </div>
              </div>
            </div>

            {/* Description */}
            <div>
              <h4 className="font-semibold text-gray-400 mb-1">Description</h4>
              <div className="bg-[#0d1117]/30 border border-zinc-800 rounded-lg p-2.5 max-h-40 overflow-y-auto text-gray-400 leading-normal text-[11px] break-words whitespace-pre-wrap">
                {selectedPR.description || <span className="text-gray-600 italic">No description provided</span>}
              </div>
            </div>
          </div>

          {/* Footer actions */}
          <div className="p-3 border-t border-zinc-800 bg-[#0d1117]/20 flex flex-col gap-2">
            <button
              onClick={() => handleAction(true, false)}
              className="w-full py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded font-medium text-xs transition"
            >
              Rebase Branch
            </button>
            <button
              onClick={() => handleAction(true, true)}
              className="w-full py-1.5 bg-[#238636] hover:bg-[#2ea043] text-white rounded font-medium text-xs transition"
            >
              Rebase + Force Push
            </button>
          </div>
        </>
      )}
    </div>
  );
};
