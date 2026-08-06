import React, { useEffect, useMemo } from 'react';
import { useAppStore, ReviewerSummary } from '../stores/appStore';
import {
  RefreshCw, Database, GitPullRequest, GitMerge, MessageSquare,
  GitCommit, Plus, Minus, Clock, CheckCircle, AlertCircle,
  Loader2, BarChart2,
} from 'lucide-react';

// ─── Palette ─────────────────────────────────────────────────────────────────
const COLORS = {
  open:    '#22c55e',
  draft:   '#f59e0b',
  closed:  '#6b7280',
  blue:    '#3b82f6',
  indigo:  '#6366f1',
  violet:  '#8b5cf6',
  cyan:    '#06b6d4',
  rose:    '#f43f5e',
  green:   '#22c55e',
  amber:   '#f59e0b',
};

const REVIEWER_PALETTE = [
  '#3b82f6','#8b5cf6','#06b6d4','#f59e0b','#22c55e',
  '#f43f5e','#ec4899','#14b8a6','#a78bfa','#fb923c',
];

// ─── Helper: relative time ────────────────────────────────────────────────────
function relTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return 'just now';
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

// ─── Summary Card ─────────────────────────────────────────────────────────────
function SumCard({
  icon, label, value, sub, color = 'text-gray-200',
}: {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  sub?: string;
  color?: string;
}) {
  return (
    <div className="bg-[#161b22] border border-zinc-800 rounded-xl p-4 flex flex-col gap-2 hover:border-zinc-700 transition">
      <div className="flex items-center gap-2 text-zinc-500 text-[11px] font-semibold uppercase tracking-wider">
        {icon}
        {label}
      </div>
      <div className={`text-2xl font-black leading-none ${color}`}>{value}</div>
      {sub && <div className="text-[10px] text-zinc-600">{sub}</div>}
    </div>
  );
}

// ─── Section wrapper ──────────────────────────────────────────────────────────
function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-[#161b22] border border-zinc-800 rounded-xl p-4">
      <h3 className="text-[11px] font-bold uppercase tracking-wider text-zinc-500 mb-4">{title}</h3>
      {children}
    </div>
  );
}

// ─── Donut Chart (pure SVG) ───────────────────────────────────────────────────
function DonutChart({
  segments, size = 160,
}: {
  segments: { value: number; color: string; label: string }[];
  size?: number;
}) {
  const total = segments.reduce((s, x) => s + x.value, 0);
  const cx = size / 2;
  const cy = size / 2;
  const outerR = size * 0.42;
  const innerR = size * 0.26;
  const gap = 0.015; // radians gap between segments

  let cumPct = 0;
  const arcs = segments
    .filter(s => s.value > 0)
    .map(seg => {
      const pct = seg.value / total;
      const startAngle = cumPct * 2 * Math.PI - Math.PI / 2 + gap;
      cumPct += pct;
      const endAngle   = cumPct * 2 * Math.PI - Math.PI / 2 - gap;

      const x1  = cx + outerR * Math.cos(startAngle);
      const y1  = cy + outerR * Math.sin(startAngle);
      const x2  = cx + outerR * Math.cos(endAngle);
      const y2  = cy + outerR * Math.sin(endAngle);
      const xi1 = cx + innerR * Math.cos(endAngle);
      const yi1 = cy + innerR * Math.sin(endAngle);
      const xi2 = cx + innerR * Math.cos(startAngle);
      const yi2 = cy + innerR * Math.sin(startAngle);
      const largeArc = pct > 0.5 ? 1 : 0;

      const d = [
        `M ${x1.toFixed(2)} ${y1.toFixed(2)}`,
        `A ${outerR} ${outerR} 0 ${largeArc} 1 ${x2.toFixed(2)} ${y2.toFixed(2)}`,
        `L ${xi1.toFixed(2)} ${yi1.toFixed(2)}`,
        `A ${innerR} ${innerR} 0 ${largeArc} 0 ${xi2.toFixed(2)} ${yi2.toFixed(2)}`,
        'Z',
      ].join(' ');

      return { d, color: seg.color, label: seg.label, value: seg.value, pct };
    });

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      {arcs.map((arc, i) => (
        <path key={i} d={arc.d} fill={arc.color} className="transition-all duration-500" />
      ))}
      <text
        x={cx} y={cy - 7}
        textAnchor="middle" fill="#f3f4f6"
        fontSize={size * 0.13} fontWeight="700"
      >
        {total}
      </text>
      <text
        x={cx} y={cy + 10}
        textAnchor="middle" fill="#52525b"
        fontSize={size * 0.075}
      >
        Total PRs
      </text>
    </svg>
  );
}

// ─── Horizontal Bar Chart ─────────────────────────────────────────────────────
function HBarChart({
  data,
}: {
  data: { label: string; value: number; color?: string }[];
}) {
  const max = Math.max(...data.map(d => d.value), 1);
  return (
    <div className="space-y-2.5">
      {data.map((d, i) => (
        <div key={i} className="flex items-center gap-2.5">
          <span className="text-[11px] text-zinc-400 w-28 truncate text-right shrink-0" title={d.label}>
            {d.label}
          </span>
          <div className="flex-1 bg-zinc-800 rounded-full h-2 overflow-hidden">
            <div
              className="h-2 rounded-full transition-all duration-700 ease-out"
              style={{
                width: `${Math.round((d.value / max) * 100)}%`,
                backgroundColor: d.color ?? COLORS.blue,
              }}
            />
          </div>
          <span className="text-[11px] font-semibold text-zinc-300 w-6 text-right shrink-0">
            {d.value}
          </span>
        </div>
      ))}
    </div>
  );
}

// ─── Area / Line Chart (SVG, PRs over time) ────────────────────────────────────
function AreaChart({ data }: { data: { label: string; value: number }[] }) {
  const W = 800;
  const H = 140;
  const PL = 32, PR = 10, PT = 12, PB = 28;
  const iW = W - PL - PR;
  const iH = H - PT - PB;

  const maxV = Math.max(...data.map(d => d.value), 1);
  const n = data.length;

  if (n === 0) return <div className="text-zinc-600 text-xs text-center py-6">No data</div>;

  const pts = data.map((d, i) => ({
    x: PL + (n === 1 ? iW / 2 : (i / (n - 1)) * iW),
    y: PT + iH - (d.value / maxV) * iH,
    ...d,
  }));

  const lineStr  = pts.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
  const areaPath =
    `M ${pts[0].x.toFixed(1)},${(PT + iH).toFixed(1)} ` +
    pts.map(p => `L ${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ') +
    ` L ${pts[n - 1].x.toFixed(1)},${(PT + iH).toFixed(1)} Z`;

  // Y-axis labels at 0, 50%, 100%
  const yLabels = [0, 0.5, 1].map(f => ({
    y: PT + iH * (1 - f),
    label: Math.round(maxV * f),
  }));

  // Show every Nth x-label to avoid crowding
  const step = Math.max(1, Math.ceil(n / 8));

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full" preserveAspectRatio="none">
      <defs>
        <linearGradient id="dashGrad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%"   stopColor={COLORS.blue} stopOpacity="0.35" />
          <stop offset="100%" stopColor={COLORS.blue} stopOpacity="0" />
        </linearGradient>
      </defs>

      {/* Grid lines */}
      {yLabels.map((yl, i) => (
        <g key={i}>
          <line x1={PL} y1={yl.y} x2={W - PR} y2={yl.y} stroke="#27272a" strokeWidth="1" />
          <text x={PL - 4} y={yl.y + 3.5} textAnchor="end" fill="#52525b" fontSize="9">{yl.label}</text>
        </g>
      ))}

      {/* Area fill */}
      <path d={areaPath} fill="url(#dashGrad)" />

      {/* Line */}
      <polyline points={lineStr} fill="none" stroke={COLORS.blue} strokeWidth="2" strokeLinejoin="round" />

      {/* Dots */}
      {pts.map((p, i) => (
        <circle key={i} cx={p.x} cy={p.y} r="3" fill={COLORS.blue} />
      ))}

      {/* X-axis labels */}
      {pts.map((p, i) =>
        i % step === 0 ? (
          <text key={i} x={p.x} y={H - 6} textAnchor="middle" fill="#52525b" fontSize="9">
            {p.label}
          </text>
        ) : null
      )}
    </svg>
  );
}

// ─── Reviewer table ───────────────────────────────────────────────────────────
function ReviewerTable({
  data,
}: {
  data: { login: string; summary: ReviewerSummary; color: string }[];
}) {
  if (data.length === 0) {
    return <p className="text-zinc-600 text-xs italic">No reviewer data yet.</p>;
  }
  return (
    <div className="space-y-2">
      {data.map((d, i) => {
        const total = d.summary.approved + d.summary.changes_requested + d.summary.commented;
        const approvedPct = total > 0 ? (d.summary.approved / total) * 100 : 0;
        return (
          <div key={i} className="flex items-center gap-2.5">
            {/* Avatar placeholder */}
            <div
              className="w-6 h-6 rounded-full flex items-center justify-center text-[10px] font-bold text-white shrink-0"
              style={{ backgroundColor: d.color }}
            >
              {d.login[0]?.toUpperCase()}
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center justify-between mb-0.5">
                <span className="text-[11px] font-medium text-gray-300 truncate">{d.login}</span>
                <div className="flex items-center gap-1.5 shrink-0 ml-2">
                  {d.summary.approved > 0 && (
                    <span className="text-[9px] px-1.5 py-0.5 bg-green-950/60 text-green-400 border border-green-800/60 rounded font-semibold">
                      ✓ {d.summary.approved}
                    </span>
                  )}
                  {d.summary.changes_requested > 0 && (
                    <span className="text-[9px] px-1.5 py-0.5 bg-red-950/60 text-red-400 border border-red-800/60 rounded font-semibold">
                      ✗ {d.summary.changes_requested}
                    </span>
                  )}
                  {d.summary.total_comments > 0 && (
                    <span className="text-[9px] text-zinc-500">{d.summary.total_comments} cmts</span>
                  )}
                </div>
              </div>
              {/* Approval bar */}
              <div className="h-1 bg-zinc-800 rounded-full overflow-hidden">
                <div
                  className="h-1 rounded-full bg-green-500 transition-all duration-700"
                  style={{ width: `${approvedPct}%` }}
                />
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ─── Empty / Loading states ───────────────────────────────────────────────────
function EmptyState({ loading }: { loading: boolean }) {
  return (
    <div className="flex-1 flex flex-col items-center justify-center gap-4 text-zinc-500 select-none py-20">
      {loading ? (
        <>
          <Loader2 size={32} className="animate-spin text-blue-400" />
          <p className="text-sm font-medium text-zinc-400">Computing statistics for all PRs…</p>
          <p className="text-xs text-zinc-600">This may take a moment on the first run.</p>
        </>
      ) : (
        <>
          <BarChart2 size={32} className="text-zinc-700" />
          <p className="text-sm font-medium">No dashboard data yet</p>
          <p className="text-xs">Click Refresh to compute statistics.</p>
        </>
      )}
    </div>
  );
}

// ─── Main Dashboard ───────────────────────────────────────────────────────────
export const Dashboard: React.FC = () => {
  const { dashboardStats: stats, isDashboardLoading, fetchDashboardStats } = useAppStore();

  // Auto-fetch on first mount
  useEffect(() => {
    if (!stats) fetchDashboardStats();
  }, []);

  // ── Derived chart data ─────────────────────────────────────────────────────
  const donutSegments = useMemo(() => [
    { value: stats?.open_prs   ?? 0, color: COLORS.open,   label: 'Open' },
    { value: stats?.draft_prs  ?? 0, color: COLORS.draft,  label: 'Draft' },
    { value: stats?.closed_prs ?? 0, color: COLORS.closed, label: 'Closed' },
  ], [stats]);

  const repoData = useMemo(() => {
    if (!stats) return [];
    return Object.entries(stats.prs_by_repo)
      .sort(([, a], [, b]) => b - a)
      .slice(0, 10)
      .map(([label, value], i) => ({ label, value, color: REVIEWER_PALETTE[i % REVIEWER_PALETTE.length] }));
  }, [stats]);

  const timeData = useMemo(() => {
    if (!stats) return [];
    return Object.entries(stats.prs_by_month)
      .sort(([a], [b]) => a.localeCompare(b))
      .slice(-18)
      .map(([key, value]) => {
        const [year, month] = key.split('-');
        const label = new Date(Number(year), Number(month) - 1).toLocaleDateString(undefined, { month: 'short', year: '2-digit' });
        return { label, value };
      });
  }, [stats]);

  const reviewerData = useMemo(() => {
    if (!stats) return [];
    return Object.entries(stats.reviewer_activity)
      .sort(([, a], [, b]) => (b.approved + b.changes_requested + b.commented) - (a.approved + a.changes_requested + a.commented))
      .slice(0, 10)
      .map(([login, summary], i) => ({
        login,
        summary,
        color: REVIEWER_PALETTE[i % REVIEWER_PALETTE.length],
      }));
  }, [stats]);

  const avgAgeDisplay = useMemo(() => {
    if (!stats) return '—';
    const d = stats.avg_pr_age_days;
    if (d < 1)  return `${Math.round(d * 24)}h`;
    if (d < 30) return `${Math.round(d)}d`;
    return `${(d / 30).toFixed(1)}mo`;
  }, [stats]);

  const isRefreshing = isDashboardLoading && !!stats;

  // ── Render ─────────────────────────────────────────────────────────────────
  return (
    <div className="flex-1 flex flex-col bg-[#0d1117] overflow-hidden">
      {/* ── Dashboard toolbar ── */}
      <div className="px-4 py-2.5 border-b border-zinc-800 bg-[#161b22] flex items-center justify-between shrink-0">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 text-[11px] text-zinc-500">
            <Database size={12} />
            <span className="font-semibold text-zinc-400">Overall Statistics</span>
          </div>
          {stats && (
            <div className="flex items-center gap-3 text-[10px] text-zinc-600">
              <span>Last refreshed: {relTime(stats.last_updated)}</span>
              <span>·</span>
              <span className="text-green-600">{stats.cache_hits} cached</span>
              {stats.stale_recomputed > 0 && (
                <>
                  <span>·</span>
                  <span className="text-amber-600">{stats.stale_recomputed} recomputed</span>
                </>
              )}
            </div>
          )}
          {isRefreshing && (
            <div className="flex items-center gap-1.5 text-[10px] text-blue-400 animate-pulse">
              <Loader2 size={11} className="animate-spin" />
              <span>Refreshing…</span>
            </div>
          )}
        </div>
        <button
          onClick={fetchDashboardStats}
          disabled={isDashboardLoading}
          className="flex items-center gap-1.5 px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-zinc-300 hover:text-white rounded text-xs font-medium transition disabled:opacity-50"
        >
          <RefreshCw size={12} className={isDashboardLoading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {/* ── Content ── */}
      {(!stats && isDashboardLoading) || (!stats && !isDashboardLoading) ? (
        <EmptyState loading={isDashboardLoading} />
      ) : stats ? (
        <div className="flex-1 overflow-y-auto p-4 space-y-4">

          {/* ── Summary cards ── */}
          <div className="grid grid-cols-5 gap-3">
            <SumCard icon={<GitPullRequest size={12} />} label="Total PRs"     value={stats.total_prs}            color="text-gray-100" />
            <SumCard icon={<CheckCircle   size={12} />} label="Open"           value={stats.open_prs}             color="text-green-400" />
            <SumCard icon={<AlertCircle   size={12} />} label="Draft"          value={stats.draft_prs}            color="text-amber-400" />
            <SumCard icon={<GitMerge      size={12} />} label="Closed"         value={stats.closed_prs}           color="text-zinc-400" />
            <SumCard icon={<Clock         size={12} />} label="Avg PR Age"     value={avgAgeDisplay}              color="text-violet-400" />
          </div>

          {/* ── Row 2: Stats cards ── */}
          <div className="grid grid-cols-4 gap-3">
            <SumCard icon={<MessageSquare size={12} />} label="Total Comments" value={stats.total_comments.toLocaleString()} color="text-sky-400" />
            <SumCard icon={<GitCommit     size={12} />} label="Total Commits"  value={stats.total_commits.toLocaleString()}  color="text-indigo-400" />
            <SumCard icon={<Plus          size={12} />} label="Lines Added"    value={`+${stats.total_additions.toLocaleString()}`} color="text-green-400" />
            <SumCard icon={<Minus         size={12} />} label="Lines Removed"  value={`-${stats.total_deletions.toLocaleString()}`} color="text-red-400" />
          </div>

          {/* ── Row 3: Donut + Repos ── */}
          <div className="grid grid-cols-5 gap-4">
            {/* PR States donut */}
            <Section title="PR States">
              <div className="flex items-center gap-6">
                <DonutChart segments={donutSegments} size={160} />
                <div className="space-y-3">
                  {donutSegments.map(seg => (
                    <div key={seg.label} className="flex items-center gap-2">
                      <div className="w-3 h-3 rounded-sm shrink-0" style={{ backgroundColor: seg.color }} />
                      <span className="text-[11px] text-zinc-400">{seg.label}</span>
                      <span className="text-[11px] font-bold text-gray-200 ml-auto pl-4">{seg.value}</span>
                    </div>
                  ))}
                </div>
              </div>
            </Section>

            {/* PRs by repo */}
            <div className="col-span-2">
              <Section title="PRs by Repository">
                {repoData.length === 0
                  ? <p className="text-zinc-600 text-xs italic">No data</p>
                  : <HBarChart data={repoData} />
                }
              </Section>
            </div>

            {/* Code impact */}
            <div className="col-span-2">
              <Section title="Code Impact">
                <div className="space-y-4">
                  <div>
                    <div className="flex justify-between text-[10px] text-zinc-500 mb-1">
                      <span>Additions</span>
                      <span className="text-green-400 font-semibold">+{stats.total_additions.toLocaleString()}</span>
                    </div>
                    <div className="h-2 bg-zinc-800 rounded-full overflow-hidden">
                      <div
                        className="h-2 bg-green-500 rounded-full transition-all duration-700"
                        style={{
                          width: `${stats.total_additions + stats.total_deletions > 0
                            ? (stats.total_additions / (stats.total_additions + stats.total_deletions)) * 100
                            : 50}%`,
                        }}
                      />
                    </div>
                  </div>
                  <div>
                    <div className="flex justify-between text-[10px] text-zinc-500 mb-1">
                      <span>Deletions</span>
                      <span className="text-red-400 font-semibold">-{stats.total_deletions.toLocaleString()}</span>
                    </div>
                    <div className="h-2 bg-zinc-800 rounded-full overflow-hidden">
                      <div
                        className="h-2 bg-red-500 rounded-full transition-all duration-700"
                        style={{
                          width: `${stats.total_additions + stats.total_deletions > 0
                            ? (stats.total_deletions / (stats.total_additions + stats.total_deletions)) * 100
                            : 50}%`,
                        }}
                      />
                    </div>
                  </div>
                  <div className="pt-2 border-t border-zinc-800/60 grid grid-cols-2 gap-3 text-center">
                    <div>
                      <div className="text-lg font-black text-violet-400">{stats.total_commits.toLocaleString()}</div>
                      <div className="text-[10px] text-zinc-600">Commits</div>
                    </div>
                    <div>
                      <div className="text-lg font-black text-cyan-400">{stats.total_comments.toLocaleString()}</div>
                      <div className="text-[10px] text-zinc-600">Comments</div>
                    </div>
                  </div>
                </div>
              </Section>
            </div>
          </div>

          {/* ── Row 4: PRs over time ── */}
          <Section title={`PRs Over Time (${timeData.length} months)`}>
            {timeData.length === 0 ? (
              <p className="text-zinc-600 text-xs italic">No timeline data available.</p>
            ) : (
              <AreaChart data={timeData} />
            )}
          </Section>

          {/* ── Row 5: Reviewers ── */}
          <Section title={`Top Reviewers (${reviewerData.length})`}>
            {reviewerData.length === 0 ? (
              <p className="text-zinc-600 text-xs italic">No reviewer data yet.</p>
            ) : (
              <div className="grid grid-cols-2 gap-6">
                <div>
                  <ReviewerTable data={reviewerData.slice(0, 5)} />
                </div>
                {reviewerData.length > 5 && (
                  <div>
                    <ReviewerTable data={reviewerData.slice(5)} />
                  </div>
                )}
              </div>
            )}
          </Section>

        </div>
      ) : null}
    </div>
  );
};
