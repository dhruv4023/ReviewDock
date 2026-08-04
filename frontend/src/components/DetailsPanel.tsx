import React, { useEffect, useState, useCallback } from 'react';
import { useAppStore, PRStats } from '../stores/appStore';
import {
  ExternalLink, CheckCircle2, XCircle, AlertCircle, RefreshCw, X, Copy, Check, Eye,
  Bot, Loader2, BarChart3, Info, User, Tag, GitCommit, FileCode,
  Plus, Minus, Clock, MessageSquare, Shield, Zap,
} from 'lucide-react';

// ─── Types ────────────────────────────────────────────────────────────────────
type PanelTab = 'details' | 'statistics';

// ─── Reviewer state helpers ────────────────────────────────────────────────────
const REVIEWER_STATE_CONFIG: Record<string, { label: string; className: string }> = {
  APPROVED:           { label: 'Approved',          className: 'bg-green-950/60 text-green-400 border-green-800/60' },
  CHANGES_REQUESTED:  { label: 'Changes Requested', className: 'bg-red-950/60 text-red-400 border-red-800/60' },
  COMMENTED:          { label: 'Commented',          className: 'bg-amber-950/60 text-amber-400 border-amber-800/60' },
  DISMISSED:          { label: 'Dismissed',          className: 'bg-zinc-800/60 text-zinc-400 border-zinc-700/60' },
};

function ReviewerStateBadge({ state }: { state: string }) {
  const cfg = REVIEWER_STATE_CONFIG[state] ?? { label: state, className: 'bg-zinc-800/60 text-zinc-400 border-zinc-700/60' };
  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded border text-[9px] font-semibold uppercase tracking-wide ${cfg.className}`}>
      {cfg.label}
    </span>
  );
}

// ─── Small utility components ─────────────────────────────────────────────────
function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h4 className="font-semibold text-gray-400 mb-1.5 text-[11px] uppercase tracking-wider">{children}</h4>;
}

function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-2 py-1.5 border-b border-zinc-800/50 last:border-0">
      <span className="text-zinc-500 shrink-0 text-[11px]">{label}</span>
      <span className="text-gray-300 text-right text-[11px] break-words min-w-0">{children}</span>
    </div>
  );
}

function StatCard({ icon, label, value, sub, accent }: {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  sub?: string;
  accent?: string;
}) {
  return (
    <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-3 flex flex-col gap-1">
      <div className={`flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider ${accent ?? 'text-zinc-500'}`}>
        {icon}
        {label}
      </div>
      <div className="text-xl font-bold text-gray-100 leading-none">{value}</div>
      {sub && <div className="text-[10px] text-zinc-500">{sub}</div>}
    </div>
  );
}

// ─── Skeleton loader for Statistics tab ───────────────────────────────────────
function StatsSkeleton() {
  return (
    <div className="space-y-4 animate-pulse">
      <div className="grid grid-cols-2 gap-2">
        {[...Array(4)].map((_, i) => (
          <div key={i} className="bg-zinc-800/40 border border-zinc-800 rounded-lg h-16" />
        ))}
      </div>
      <div className="bg-zinc-800/40 border border-zinc-800 rounded-lg h-24" />
      <div className="bg-zinc-800/40 border border-zinc-800 rounded-lg h-20" />
    </div>
  );
}

// ─── Statistics tab content ───────────────────────────────────────────────────
function StatisticsTab({ stats, loading, onRefresh }: {
  stats: PRStats | null;
  loading: boolean;
  onRefresh: () => void;
}) {
  if (loading && !stats) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-center gap-2 py-4 text-zinc-500 text-xs">
          <Loader2 size={14} className="animate-spin text-blue-400" />
          <span>Computing statistics…</span>
        </div>
        <StatsSkeleton />
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="flex flex-col items-center justify-center py-10 gap-3 text-zinc-500">
        <BarChart3 size={28} className="text-zinc-600" />
        <p className="text-xs text-center">No statistics loaded yet.</p>
      </div>
    );
  }

  const totalReviewers = Object.keys(stats.reviewer_states).length;
  const ageDays = stats.pr_age_days;
  const ageDisplay = ageDays < 1
    ? `${Math.round(ageDays * 24)}h`
    : `${Math.round(ageDays)}d`;

  // Last updated relative time
  const lastUpdated = stats.last_updated ? new Date(stats.last_updated) : null;
  const relativeTime = lastUpdated
    ? (() => {
        const diff = Date.now() - lastUpdated.getTime();
        if (diff < 60_000) return 'just now';
        if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
        if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
        return lastUpdated.toLocaleDateString();
      })()
    : '—';

  return (
    <div className="space-y-4">
      {/* Refresh header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-[10px] text-zinc-500">
          {loading ? (
            <Loader2 size={11} className="animate-spin text-blue-400" />
          ) : (
            <Clock size={11} />
          )}
          <span>
            {stats.from_cache ? 'Cached' : 'Fresh'} · {relativeTime}
          </span>
        </div>
        <button
          onClick={onRefresh}
          disabled={loading}
          className="flex items-center gap-1 px-2 py-1 rounded text-[10px] bg-zinc-800 hover:bg-zinc-700 text-zinc-400 hover:text-gray-200 border border-zinc-700 transition disabled:opacity-50"
          title="Force refresh statistics"
        >
          <RefreshCw size={10} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {/* Overview: comment metrics */}
      <div>
        <SectionTitle>Comments</SectionTitle>
        <div className="grid grid-cols-2 gap-2">
          <StatCard
            icon={<MessageSquare size={11} />}
            label="Total"
            value={stats.total_comments}
            sub="all sources"
            accent="text-blue-400"
          />
          <StatCard
            icon={<MessageSquare size={11} />}
            label="PR Comments"
            value={stats.pr_conversation_comments}
            sub="conversation"
            accent="text-sky-400"
          />
          <StatCard
            icon={<MessageSquare size={11} />}
            label="Review Bodies"
            value={stats.review_comments}
            sub="from reviews"
            accent="text-indigo-400"
          />
          <StatCard
            icon={<MessageSquare size={11} />}
            label="Author Replies"
            value={stats.author_replies}
            sub="by author"
            accent="text-amber-400"
          />
        </div>
      </div>

      {/* Code stats */}
      <div>
        <SectionTitle>Code Changes</SectionTitle>
        <div className="grid grid-cols-2 gap-2">
          <StatCard
            icon={<GitCommit size={11} />}
            label="Commits"
            value={stats.commits}
            accent="text-violet-400"
          />
          <StatCard
            icon={<FileCode size={11} />}
            label="Files"
            value={stats.changed_files}
            sub="changed"
            accent="text-cyan-400"
          />
          <StatCard
            icon={<Plus size={11} />}
            label="Additions"
            value={`+${stats.additions}`}
            accent="text-green-400"
          />
          <StatCard
            icon={<Minus size={11} />}
            label="Deletions"
            value={`-${stats.deletions}`}
            accent="text-red-400"
          />
        </div>
        <div className="mt-2 bg-[#0d1117] border border-zinc-800 rounded-lg p-3 flex items-center justify-between">
          <div className="flex items-center gap-1.5 text-[10px] text-zinc-500">
            <Clock size={11} />
            <span className="uppercase tracking-wider font-semibold">PR Age</span>
          </div>
          <span className="text-gray-200 font-bold text-sm">{ageDisplay}</span>
        </div>
      </div>

      {/* Reviewer breakdown */}
      {totalReviewers > 0 && (
        <div>
          <SectionTitle>Reviewers ({totalReviewers})</SectionTitle>
          <div className="bg-[#0d1117] border border-zinc-800 rounded-lg overflow-hidden">
            {Object.entries(stats.reviewer_states).map(([login, state], idx) => {
              const commentCount = stats.review_comments_by_reviewer?.[login] ?? 0;
              return (
                <div
                  key={login}
                  className={`flex items-center justify-between px-3 py-2 text-[11px] ${
                    idx > 0 ? 'border-t border-zinc-800/60' : ''
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <div className="w-5 h-5 rounded-full bg-zinc-700 flex items-center justify-center text-[9px] font-bold text-zinc-300 shrink-0">
                      {login[0]?.toUpperCase()}
                    </div>
                    <span className="text-gray-300 font-medium">{login}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    {commentCount > 0 && (
                      <span className="text-zinc-500 text-[10px]">{commentCount} comment{commentCount !== 1 ? 's' : ''}</span>
                    )}
                    <ReviewerStateBadge state={state} />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Merge info */}
      <div>
        <SectionTitle>Merge Info</SectionTitle>
        <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-3 flex items-center gap-3">
          {stats.directly_merged ? (
            <>
              <Zap size={14} className="text-amber-400 shrink-0" />
              <div>
                <p className="text-amber-300 font-semibold text-[11px]">Directly Merged</p>
                <p className="text-zinc-500 text-[10px]">Merged without an approving review</p>
              </div>
            </>
          ) : (
            <>
              <Shield size={14} className="text-green-400 shrink-0" />
              <div>
                <p className="text-green-300 font-semibold text-[11px]">Review-Gated</p>
                <p className="text-zinc-500 text-[10px]">Had an approving review before merge</p>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

// ─── Main DetailsPanel ────────────────────────────────────────────────────────
interface DetailsPanelProps {
  width: number;
  onClose: () => void;
  onResizeStart: (e: React.MouseEvent) => void;
}

export const DetailsPanel: React.FC<DetailsPanelProps> = ({ width, onClose, onResizeStart }) => {
  const {
    selectedPR,
    rebaseSinglePR,
    isRebasing,
    reviewTemplate,
    prStats,
    isPRStatsLoading,
    fetchPRStats,
    clearPRStats,
  } = useAppStore();

  const [activeTab, setActiveTab] = useState<PanelTab>('details');
  const [ciStatus, setCiStatus] = useState<string>('loading');
  const [isRefreshingCi, setIsRefreshingCi] = useState(false);

  // Copy feedback states
  const [copiedHead, setCopiedHead] = useState(false);
  const [copiedBase, setCopiedBase] = useState(false);
  const [copiedDiff, setCopiedDiff] = useState(false);
  const [copiedModalDiff, setCopiedModalDiff] = useState(false);
  const [copiedReviewPrompt, setCopiedReviewPrompt] = useState(false);
  const [isGeneratingPrompt, setIsGeneratingPrompt] = useState(false);

  // Diff modal states
  const [isDiffModalOpen, setIsDiffModalOpen] = useState(false);
  const [diffText, setDiffText] = useState('');
  const [isLoadingDiff, setIsLoadingDiff] = useState(false);

  // Reset to Details tab when selected PR changes
  useEffect(() => {
    if (selectedPR) {
      setActiveTab('details');
      setCiStatus('loading');
      fetchCI();
      setIsDiffModalOpen(false);
      setDiffText('');
    }
  }, [selectedPR?.id]);

  // Fetch stats when Statistics tab becomes active
  useEffect(() => {
    if (activeTab === 'statistics' && selectedPR) {
      fetchPRStats(selectedPR);
    }
  }, [activeTab, selectedPR?.id]);

  const fetchCI = async () => {
    if (!selectedPR) return;
    setIsRefreshingCi(true);
    try {
      const status = await window.go.main.App.GetPRCIStatus(selectedPR.repo_id, selectedPR.head_branch);
      setCiStatus(status);
    } catch {
      setCiStatus('unknown');
    } finally {
      setIsRefreshingCi(false);
    }
  };

  const handleAction = async (amend: boolean, push: boolean) => {
    if (!selectedPR) return;
    await rebaseSinglePR(selectedPR.id, amend, push);
  };

  const copyToClipboard = useCallback((text: string, setCopiedState: (v: boolean) => void) => {
    navigator.clipboard.writeText(text);
    setCopiedState(true);
    setTimeout(() => setCopiedState(false), 1500);
  }, []);

  const handleViewDiff = async () => {
    if (!selectedPR) return;
    setIsDiffModalOpen(true);
    setIsLoadingDiff(true);
    try {
      const baseRef = selectedPR.base_label || selectedPR.base_branch;
      const diff = await window.go.main.App.GetPRDiff(selectedPR.repo_id, baseRef, selectedPR.head_branch);
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
      const diff = await window.go.main.App.GetPRDiff(selectedPR.repo_id, baseRef, selectedPR.head_branch);
      copyToClipboard(diff || 'No changes / Empty diff', setCopiedDiff);
    } catch (err) {
      alert(`Failed to fetch and copy diff: ${err}`);
    }
  };

  const DEFAULT_TEMPLATE = `You are an expert code reviewer. Please review the following pull request carefully.\n\n## Instructions\n- Identify bugs, security issues, and logic errors.\n- Suggest improvements to code clarity and maintainability.\n- Check for missing tests or documentation.\n- Be concise and prioritize critical issues.\n- Note any positive aspects of the implementation.\n\n## PR Title\n{{PR_TITLE}}\n\n## PR Description\n{{PR_DESCRIPTION}}\n\n## Code Changes\n{{PR_DIFF}}`;

  const handleCopyReviewPrompt = async () => {
    if (!selectedPR || isGeneratingPrompt) return;
    setIsGeneratingPrompt(true);
    try {
      const baseRef = selectedPR.base_label || selectedPR.base_branch;
      const diff = await window.go.main.App.GetPRDiff(selectedPR.repo_id, baseRef, selectedPR.head_branch);
      const tmpl = reviewTemplate || DEFAULT_TEMPLATE;
      const prompt = tmpl
        .replace(/\{\{PR_TITLE\}\}/g, selectedPR.title || '')
        .replace(/\{\{PR_DESCRIPTION\}\}/g, selectedPR.description || 'No description provided.')
        .replace(/\{\{PR_DIFF\}\}/g, diff || 'No diff available.');
      copyToClipboard(prompt, setCopiedReviewPrompt);
    } catch (err) {
      alert(`Failed to generate review prompt: ${err}`);
    } finally {
      setIsGeneratingPrompt(false);
    }
  };

  const formatDiffLine = (line: string, index: number) => {
    let className = 'py-0.5 px-2 font-mono whitespace-pre-wrap text-[11px] ';
    if (line.startsWith('+')) className += 'bg-green-950/30 text-green-400 border-l-2 border-green-600';
    else if (line.startsWith('-')) className += 'bg-red-950/30 text-red-400 border-l-2 border-red-600';
    else if (line.startsWith('@@')) className += 'bg-blue-950/20 text-blue-400 font-semibold';
    else if (line.startsWith('diff --git') || line.startsWith('index ') || line.startsWith('--- ') || line.startsWith('+++ '))
      className += 'text-zinc-500 font-semibold';
    else className += 'text-zinc-300';
    return <div key={index} className={className}>{line}</div>;
  };

  // ─── Helper: format dates ──────────────────────────────────────────────────
  const formatDate = (iso: string | undefined) => {
    if (!iso) return '—';
    try { return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }); }
    catch { return '—'; }
  };

  // ─── Stat cache last-updated for Details tab ───────────────────────────────
  const cacheRefreshedText = (() => {
    if (!prStats?.last_updated) return null;
    const d = new Date(prStats.last_updated);
    const diff = Date.now() - d.getTime();
    if (diff < 60_000) return 'just now';
    if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
    return d.toLocaleString();
  })();

  // ─── Tab bar ───────────────────────────────────────────────────────────────
  const TAB_CONFIG: { id: PanelTab; label: string; icon: React.ReactNode }[] = [
    { id: 'details',    label: 'Details',    icon: <Info size={12} /> },
    { id: 'statistics', label: 'Statistics', icon: <BarChart3 size={12} /> },
  ];

  return (
    <div
      className="bg-[#161b22] border-l border-zinc-800 flex flex-col h-full overflow-hidden text-xs relative"
      style={{ width }}
    >
      {/* Drag-resize handle */}
      <div
        onMouseDown={onResizeStart}
        className="absolute left-0 top-0 h-full w-1 cursor-col-resize hover:bg-blue-600/40 transition z-10 group"
        title="Drag to resize"
      >
        <div className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-12 rounded-full bg-zinc-700 group-hover:bg-blue-500 transition opacity-0 group-hover:opacity-100" />
      </div>

      {/* ── Header ── */}
      <div className="pl-3 pr-3 py-3 border-b border-zinc-800 flex flex-col gap-1 bg-[#0d1117]/10 shrink-0">
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
            <span className="text-[10px] text-zinc-500 truncate">{selectedPR.repo_name}</span>
          </>
        )}
      </div>

      {/* ── No PR selected ── */}
      {!selectedPR ? (
        <div className="flex-1 flex items-center justify-center p-4 text-center select-none">
          <p className="text-xs text-gray-600">Click a pull request row to view details</p>
        </div>
      ) : (
        <>
          {/* ── Tab bar ── */}
          <div className="flex border-b border-zinc-800 bg-[#0d1117]/20 shrink-0">
            {TAB_CONFIG.map(tab => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex-1 flex items-center justify-center gap-1.5 py-2.5 text-[11px] font-semibold transition border-b-2 ${
                  activeTab === tab.id
                    ? 'border-blue-500 text-blue-400 bg-blue-950/10'
                    : 'border-transparent text-zinc-500 hover:text-gray-300 hover:bg-zinc-800/30'
                }`}
              >
                {tab.icon}
                {tab.label}
                {tab.id === 'statistics' && isPRStatsLoading && (
                  <Loader2 size={10} className="animate-spin ml-0.5 text-blue-400" />
                )}
              </button>
            ))}
          </div>

          {/* ── Tab content ── */}
          <div className="flex-1 overflow-y-auto">
            {/* ════════════════ DETAILS TAB ════════════════ */}
            {activeTab === 'details' && (
              <div className="p-3 space-y-4">
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
                      <><RefreshCw size={12} className="animate-spin text-amber-500" /><span className="text-amber-400">Running…</span></>
                    ) : ciStatus === 'loading' ? (
                      <><RefreshCw size={12} className="animate-spin text-zinc-500" /><span className="text-zinc-500">Querying…</span></>
                    ) : (
                      <><AlertCircle size={14} className="text-zinc-500" /><span className="text-zinc-400">No status checks</span></>
                    )}
                  </div>
                </div>

                {/* PR Metadata */}
                <div>
                  <SectionTitle>PR Metadata</SectionTitle>
                  <div className="bg-[#0d1117] border border-zinc-800 rounded-lg px-3 py-1">
                    <MetaRow label="Author">
                      <span className="flex items-center gap-1">
                        <User size={10} className="text-zinc-500" />
                        {selectedPR.author || '—'}
                      </span>
                    </MetaRow>
                    <MetaRow label="Status">
                      <span className={`font-semibold capitalize ${
                        selectedPR.state === 'open'   ? 'text-green-400' :
                        selectedPR.state === 'draft'  ? 'text-amber-400' :
                        selectedPR.state === 'merged' ? 'text-purple-400' :
                        'text-zinc-400'
                      }`}>
                        {selectedPR.state}{selectedPR.is_draft && selectedPR.state !== 'draft' ? ' (Draft)' : ''}
                      </span>
                    </MetaRow>
                    <MetaRow label="Labels">
                      {selectedPR.labels?.length ? (
                        <div className="flex flex-wrap gap-1 justify-end">
                          {selectedPR.labels.map(l => (
                            <span key={l} className="flex items-center gap-0.5 px-1.5 py-0.5 bg-zinc-800 border border-zinc-700 rounded text-[10px] text-zinc-300">
                              <Tag size={9} />
                              {l}
                            </span>
                          ))}
                        </div>
                      ) : <span className="text-zinc-600 italic">None</span>}
                    </MetaRow>
                    <MetaRow label="Reviewers">
                      {selectedPR.requested_reviewers?.length ? (
                        <div className="flex flex-wrap gap-1 justify-end">
                          {selectedPR.requested_reviewers.map(r => (
                            <span key={r} className="px-1.5 py-0.5 bg-zinc-800 border border-zinc-700 rounded text-[10px] text-zinc-300">
                              {r}
                            </span>
                          ))}
                        </div>
                      ) : <span className="text-zinc-600 italic">None requested</span>}
                    </MetaRow>
                    <MetaRow label="Created">{formatDate(selectedPR.created_at)}</MetaRow>
                    <MetaRow label="Updated">{formatDate(selectedPR.updated_at)}</MetaRow>
                    {cacheRefreshedText && (
                      <MetaRow label="Stats Cache">
                        <span className="text-zinc-500 italic">Refreshed {cacheRefreshedText}</span>
                      </MetaRow>
                    )}
                  </div>
                </div>

                {/* Branch Configuration */}
                <div>
                  <SectionTitle>Branch Configuration</SectionTitle>
                  <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-2.5 space-y-2 text-[11px]">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-gray-500 shrink-0">Head:</span>
                      <div className="flex items-center gap-1.5 truncate">
                        <span className="font-mono text-zinc-300 truncate" title={selectedPR.head_label || selectedPR.head_branch}>
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
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-gray-500 shrink-0">Base:</span>
                      <div className="flex items-center gap-1.5 truncate">
                        <span className="font-mono text-zinc-300 truncate" title={selectedPR.base_label || selectedPR.base_branch}>
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
                      <span className="text-gray-500 shrink-0">Merge Status:</span>
                      <span className={`font-semibold ${
                        selectedPR.mergeable_status === 'mergeable'  ? 'text-green-500' :
                        selectedPR.mergeable_status === 'conflicting' ? 'text-red-400' :
                        'text-zinc-500'
                      }`}>
                        {selectedPR.mergeable_status === 'mergeable'  ? 'Mergeable' :
                         selectedPR.mergeable_status === 'conflicting' ? 'Conflicting' : 'Unknown'}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Git Diff Tools */}
                <div>
                  <SectionTitle>Git Diff Tools</SectionTitle>
                  <div className="bg-[#0d1117] border border-zinc-800 rounded-lg p-2.5 flex flex-col gap-2">
                    <div className="flex gap-2">
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
                    <button
                      onClick={handleCopyReviewPrompt}
                      disabled={isGeneratingPrompt}
                      className={`w-full py-1.5 px-2.5 border rounded transition flex items-center justify-center gap-1.5 font-medium text-[11px] ${
                        copiedReviewPrompt
                          ? 'bg-green-900/30 border-green-700 text-green-400'
                          : isGeneratingPrompt
                          ? 'bg-zinc-800/50 border-zinc-700 text-zinc-500 cursor-wait'
                          : 'bg-indigo-950/40 hover:bg-indigo-900/40 border-indigo-800/60 text-indigo-300 hover:text-indigo-200 hover:border-indigo-600'
                      }`}
                      title="Generate and copy an AI code review prompt"
                    >
                      {copiedReviewPrompt ? (
                        <><Check size={12} className="text-green-500" /> Prompt Copied!</>
                      ) : isGeneratingPrompt ? (
                        <><RefreshCw size={12} className="animate-spin" /> Generating…</>
                      ) : (
                        <><Bot size={12} /> Copy AI Review Prompt</>
                      )}
                    </button>
                  </div>
                </div>

                {/* Description */}
                <div>
                  <SectionTitle>Description</SectionTitle>
                  <div className="bg-[#0d1117]/30 border border-zinc-800 rounded-lg p-2.5 max-h-40 overflow-y-auto text-gray-400 leading-normal text-[11px] break-words whitespace-pre-wrap">
                    {selectedPR.description || <span className="text-gray-600 italic">No description provided</span>}
                  </div>
                </div>
              </div>
            )}

            {/* ════════════════ STATISTICS TAB ════════════════ */}
            {activeTab === 'statistics' && (
              <div className="p-3">
                <StatisticsTab
                  stats={prStats}
                  loading={isPRStatsLoading}
                  onRefresh={() => {
                    clearPRStats();
                    if (selectedPR) fetchPRStats(selectedPR);
                  }}
                />
              </div>
            )}
          </div>

          {/* ── Footer actions (Details tab only) ── */}
          {activeTab === 'details' && (
            <div className="p-3 border-t border-zinc-800 bg-[#0d1117]/20 flex flex-col gap-2 shrink-0">
              <button
                onClick={() => handleAction(true, false)}
                disabled={isRebasing}
                className="w-full py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded font-medium text-xs transition flex items-center justify-center gap-1.5"
              >
                {isRebasing ? <Loader2 size={12} className="animate-spin" /> : null}
                Rebase Branch
              </button>
              <button
                onClick={() => handleAction(true, true)}
                disabled={isRebasing}
                className="w-full py-1.5 bg-[#238636] hover:bg-[#2ea043] disabled:opacity-50 text-white rounded font-medium text-xs transition flex items-center justify-center gap-1.5"
              >
                {isRebasing ? <Loader2 size={12} className="animate-spin" /> : null}
                Rebase + Force Push
              </button>
            </div>
          )}
        </>
      )}

      {/* ── Diff modal ── */}
      {isDiffModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm select-none">
          <div className="bg-[#161b22] border border-zinc-800 rounded-xl w-full max-w-5xl h-[85vh] flex flex-col overflow-hidden shadow-2xl relative">
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
                >
                  <X size={16} />
                </button>
              </div>
            </div>
            <div className="flex-1 overflow-auto bg-[#0d1117] p-4 select-text">
              {isLoadingDiff ? (
                <div className="h-full w-full flex flex-col items-center justify-center gap-3 text-zinc-500">
                  <RefreshCw size={24} className="animate-spin text-blue-500" />
                  <span className="text-xs">Computing git diff comparison…</span>
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
