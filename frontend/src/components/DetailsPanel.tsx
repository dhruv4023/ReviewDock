import React, { useEffect, useState } from 'react';
import { useAppStore } from '../stores/appStore';
import { ExternalLink, CheckCircle2, XCircle, AlertCircle, RefreshCw, X, Copy, Check, Eye } from 'lucide-react';

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

  // States for copy feedback
  const [copiedHead, setCopiedHead] = useState(false);
  const [copiedBase, setCopiedBase] = useState(false);
  const [copiedDiff, setCopiedDiff] = useState(false);
  const [copiedModalDiff, setCopiedModalDiff] = useState(false);

  // States for Diff Modal
  const [isDiffModalOpen, setIsDiffModalOpen] = useState(false);
  const [diffText, setDiffText] = useState('');
  const [isLoadingDiff, setIsLoadingDiff] = useState(false);

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
      // Reset diff states
      setIsDiffModalOpen(false);
      setDiffText('');
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

  const copyToClipboard = (text: string, setCopiedState: (v: boolean) => void) => {
    navigator.clipboard.writeText(text);
    setCopiedState(true);
    setTimeout(() => setCopiedState(false), 1500);
  };

  const handleViewDiff = async () => {
    if (!selectedPR) return;
    setIsDiffModalOpen(true);
    setIsLoadingDiff(true);
    try {
      const baseRef = selectedPR.base_label || selectedPR.base_branch;
      const headRef = selectedPR.head_branch;
      const diff = await window.go.main.App.GetPRDiff(selectedPR.repo_id, baseRef, headRef);
      setDiffText(diff);
    } catch (err) {
      setDiffText(`Failed to load diff: ${err}`);
    } finally {
      setIsLoadingDiff(false);
    }
  };

  const handleCopyDiff = async () => {
    if (!selectedPR) return;
    try {
      const baseRef = selectedPR.base_label || selectedPR.base_branch;
      const headRef = selectedPR.head_branch;
      const diff = await window.go.main.App.GetPRDiff(selectedPR.repo_id, baseRef, headRef);
      copyToClipboard(diff || "No changes / Empty diff", setCopiedDiff);
    } catch (err) {
      alert(`Failed to fetch and copy diff: ${err}`);
    }
  };

  const formatDiffLine = (line: string, index: number) => {
    let className = 'py-0.5 px-2 font-mono whitespace-pre-wrap text-[11px] ';
    if (line.startsWith('+')) {
      className += 'bg-green-950/30 text-green-400 border-l-2 border-green-600';
    } else if (line.startsWith('-')) {
      className += 'bg-red-950/30 text-red-400 border-l-2 border-red-600';
    } else if (line.startsWith('@@')) {
      className += 'bg-blue-950/20 text-blue-400 font-semibold';
    } else if (line.startsWith('diff --git') || line.startsWith('index ') || line.startsWith('--- ') || line.startsWith('+++ ')) {
      className += 'text-zinc-500 font-semibold';
    } else {
      className += 'text-zinc-300';
    }
    return (
      <div key={index} className={className}>
        {line}
      </div>
    );
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
              <button
                onClick={() => window.runtime.BrowserOpenURL(selectedPR.html_url)}
                className="flex items-center gap-1 text-blue-500 hover:text-blue-400 font-medium transition cursor-pointer"
              >
                GitHub <ExternalLink size={11} />
              </button>
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
              <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-2.5 space-y-2 text-[11px]">
                <div className="flex items-center justify-between gap-2 group/branch">
                  <span className="text-gray-500 shrink-0">Head:</span>
                  <div className="flex items-center gap-1.5 truncate">
                    <span 
                      className="font-mono text-zinc-300 truncate" 
                      title={selectedPR.head_label || selectedPR.head_branch}
                    >
                      {selectedPR.head_label || selectedPR.head_branch}
                    </span>
                    <button
                      onClick={() => copyToClipboard(selectedPR.head_branch, setCopiedHead)}
                      className="p-1 text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800 rounded transition shrink-0"
                      title="Copy Head Branch Name"
                    >
                      {copiedHead ? <Check size={11} className="text-green-500" /> : <Copy size={11} />}
                    </button>
                  </div>
                </div>

                <div className="flex items-center justify-between gap-2 group/branch">
                  <span className="text-gray-500 shrink-0">Base:</span>
                  <div className="flex items-center gap-1.5 truncate">
                    <span 
                      className="font-mono text-zinc-300 truncate" 
                      title={selectedPR.base_label || selectedPR.base_branch}
                    >
                      {selectedPR.base_label || selectedPR.base_branch}
                    </span>
                    <button
                      onClick={() => copyToClipboard(selectedPR.base_branch, setCopiedBase)}
                      className="p-1 text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800 rounded transition shrink-0"
                      title="Copy Base Branch Name"
                    >
                      {copiedBase ? <Check size={11} className="text-green-500" /> : <Copy size={11} />}
                    </button>
                  </div>
                </div>

                <div className="flex justify-between gap-2 pt-1 border-t border-zinc-800/60">
                  <span className="text-gray-500 shrink-0">Status:</span>
                  <span className={`font-semibold capitalize ${selectedPR.state === 'open' ? 'text-green-500' : 'text-zinc-500'}`}>
                    {selectedPR.state}{selectedPR.is_draft && ' (Draft)'}
                  </span>
                </div>
              </div>
            </div>

            {/* Compare & Diff Actions */}
            <div>
              <h4 className="font-semibold text-gray-400 mb-1.5">Git Diff Tools</h4>
              <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-2.5 flex gap-2">
                <button
                  onClick={handleViewDiff}
                  className="flex-1 py-1.5 px-2 bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-zinc-300 rounded text-center transition flex items-center justify-center gap-1 font-medium hover:text-white"
                >
                  <Eye size={12} /> See Diff
                </button>
                <button
                  onClick={handleCopyDiff}
                  className="py-1.5 px-2.5 bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-zinc-300 rounded transition flex items-center justify-center gap-1 font-medium hover:text-white"
                  title="Copy complete PR git diff"
                >
                  {copiedDiff ? <Check size={12} className="text-green-500" /> : <Copy size={12} />}
                  Copy Diff
                </button>
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

      {/* GORGEOUS DIFF OVERLAY MODAL */}
      {isDiffModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm select-none">
          <div className="bg-[#161b22] border border-zinc-800 rounded-xl w-full max-w-5xl h-[85vh] flex flex-col overflow-hidden shadow-2xl relative">
            
            {/* Modal Header */}
            <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-800 bg-[#0d1117]/40">
              <div>
                <h3 className="text-sm font-bold text-gray-100 flex items-center gap-2">
                  <span>Changes for PR #{selectedPR?.number}</span>
                  <span className="px-1.5 py-0.5 rounded bg-zinc-800 text-[10px] font-mono text-zinc-400">
                    {selectedPR?.base_branch} ➔ {selectedPR?.head_branch}
                  </span>
                </h3>
                <p className="text-[10px] text-zinc-500 mt-0.5">
                  Compared local base branch to PR branch ({selectedPR?.repo_name})
                </p>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => copyToClipboard(diffText, setCopiedModalDiff)}
                  disabled={isLoadingDiff || !diffText}
                  className="px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-300 disabled:opacity-50 hover:text-white rounded border border-zinc-700 text-xs transition flex items-center gap-1.5"
                >
                  {copiedModalDiff ? <Check size={13} className="text-green-500" /> : <Copy size={13} />}
                  <span>Copy Whole Diff</span>
                </button>
                <button
                  onClick={() => setIsDiffModalOpen(false)}
                  className="p-1.5 hover:bg-zinc-800 text-zinc-500 hover:text-white rounded transition"
                  title="Close Diff"
                >
                  <X size={16} />
                </button>
              </div>
            </div>

            {/* Modal Body */}
            <div className="flex-1 overflow-auto bg-[#0d1117] p-4 select-text">
              {isLoadingDiff ? (
                <div className="h-full w-full flex flex-col items-center justify-center gap-3 text-zinc-500">
                  <RefreshCw size={24} className="animate-spin text-blue-500" />
                  <span className="text-xs">Computing git diff comparison...</span>
                </div>
              ) : !diffText ? (
                <div className="h-full w-full flex flex-col items-center justify-center gap-2 text-zinc-500">
                  <CheckCircle2 size={32} className="text-green-500/80" />
                  <span className="text-xs font-semibold text-zinc-400">No differences detected</span>
                  <span className="text-[10px] text-zinc-600">The branches are currently identical.</span>
                </div>
              ) : (
                <div className="font-mono text-xs leading-relaxed">
                  {diffText.split('\n').map((line, idx) => formatDiffLine(line, idx))}
                </div>
              )}
            </div>

          </div>
        </div>
      )}
    </div>
  );
};
