import React from 'react';
import { useKnowledgeStore } from '../../stores/knowledgeStore';
import { Badge, Empty, Panel, Stat, relTime } from './ui';

function Bars({ items, labels }: { items: { key: string; count: number }[]; labels?: Record<string, string> }) {
  const max = Math.max(1, ...items.map(i => i.count));
  return (
    <div className="space-y-1.5">
      {items.map(i => (
        <div key={i.key} className="flex items-center gap-2 text-[11px]">
          <span className="w-40 shrink-0 truncate text-zinc-400" title={labels?.[i.key] ?? i.key}>{labels?.[i.key] ?? i.key}</span>
          <div className="flex-1 h-2 bg-zinc-800/60 rounded">
            <div className="h-2 rounded bg-blue-500/70" style={{ width: `${(i.count / max) * 100}%` }} />
          </div>
          <span className="w-10 text-right tabular-nums text-zinc-300">{i.count}</span>
        </div>
      ))}
    </div>
  );
}

export const OverviewTab: React.FC<{ onOpenRule: (id: number) => void }> = ({ onOpenRule }) => {
  const { overview: o, categories } = useKnowledgeStore();
  if (!o) return <Empty>Loading…</Empty>;

  const byStatus = Object.fromEntries((o.rules_by_status ?? []).map(c => [c.key, c.count]));
  const catLabels = Object.fromEntries(categories.map(c => [c.id, c.title]));
  const run = o.last_run;
  const build = o.last_build;

  return (
    <div className="p-4 space-y-4 overflow-y-auto h-full">
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
        <Stat label="Pull requests" value={o.prs} sub={`${o.repositories} repositories`} />
        <Stat label="Reviewer comments" value={o.reviewer_comments} sub={`${o.comments} incl. your replies & bots`} />
        <Stat label="Extracted rules" value={o.rules} sub={`${byStatus.approved ?? 0} approved · ${byStatus.pending ?? 0} pending`} />
        <Stat
          label="Last sync"
          value={run ? <Badge value={run.status} /> : '—'}
          sub={run ? `${relTime(run.started_at)} · ${run.trigger}` : 'never'}
        />
        <Stat
          label="Skill"
          value={<span className="font-mono text-base">{build?.content_hash ? build.content_hash.slice(0, 12) : '—'}</span>}
          sub={build ? `${build.status} ${relTime(build.created_at)} · ${build.rule_count} rules` : 'not built yet'}
        />
      </div>

      {run?.error && (
        <div className="text-[11px] text-red-300 bg-red-950/30 border border-red-900/60 rounded px-3 py-2 select-text">
          Last sync error: {run.error}
        </div>
      )}

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <Panel title="Most frequently repeated feedback">
          {(o.top_rules ?? []).length === 0 ? (
            <Empty>No rules extracted yet. Run a sync to collect review comments.</Empty>
          ) : (
            <table className="w-full text-[11px]">
              <tbody>
                {(o.top_rules ?? []).map(r => (
                  <tr key={r.id} onClick={() => onOpenRule(r.id)} className="border-b border-zinc-800/60 last:border-0 hover:bg-zinc-800/40 cursor-pointer">
                    <td className="py-1.5 pr-2 font-mono text-zinc-500 whitespace-nowrap">{r.code}</td>
                    <td className="py-1.5 pr-2 text-gray-200">{r.title}</td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-zinc-300 whitespace-nowrap">×{r.occurrences}</td>
                    <td className="py-1.5 text-right"><Badge value={r.status} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Panel>

        <Panel title="Actionable feedback by area">
          {(o.category_counts ?? []).length === 0 ? <Empty>No data yet.</Empty> : <Bars items={o.category_counts ?? []} labels={catLabels} />}
        </Panel>

        <Panel title="Feedback kinds">
          {(o.kind_counts ?? []).length === 0 ? <Empty>No data yet.</Empty> : <Bars items={o.kind_counts ?? []} />}
          <p className="text-[10px] text-zinc-600 mt-3 leading-snug">
            Only <b>requirement</b> and <b>suggestion</b> comments become rule evidence. Questions, superseded or withdrawn feedback, noise (LGTM, bot
            commands), and your own replies are kept as context only.
          </p>
        </Panel>

        <Panel title="Most active reviewers">
          {(o.top_reviewers ?? []).length === 0 ? <Empty>No data yet.</Empty> : <Bars items={o.top_reviewers ?? []} />}
        </Panel>
      </div>

      <div className="text-[10px] text-zinc-600 select-text">
        Database: {o.db_path} · schema v{o.schema_version}
      </div>
    </div>
  );
};
