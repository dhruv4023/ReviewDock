import React, { useCallback, useEffect, useState } from 'react';
import { Check, X, Archive, RotateCcw, GitMerge, ExternalLink, Search } from 'lucide-react';
import { KBRule, KBRuleDetail, KBRuleEdit, KBRuleFilter, kbApi, useKnowledgeStore } from '../../stores/knowledgeStore';
import { Badge, Btn, DiffHunk, Empty, Field, Select, inputCls, openURL, shortDate } from './ui';

const STATUSES = ['pending', 'approved', 'rejected', 'superseded', 'merged'];
const SEVERITIES = ['high', 'medium', 'low', 'context-dependent'];

export const RulesTab: React.FC<{ focusRuleId: number | null; onFocusHandled: () => void }> = ({ focusRuleId, onFocusHandled }) => {
  const { categories, version, bump, setError } = useKnowledgeStore();
  const [filter, setFilter] = useState<KBRuleFilter>({ status: '', category: '', query: '', min_occurrences: 0, limit: 1000 });
  const [query, setQuery] = useState('');
  const [rules, setRules] = useState<KBRule[]>([]);
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [openId, setOpenId] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const api = kbApi();
    if (!api) return;
    try {
      setRules((await api.KBRules(filter)) ?? []);
    } catch (e) {
      setError(String(e));
    }
  }, [filter, setError]);

  useEffect(() => { load(); }, [load, version]);
  useEffect(() => {
    if (focusRuleId) {
      setFilter(f => ({ ...f, status: '', category: '', query: '' }));
      setOpenId(focusRuleId);
      onFocusHandled();
    }
  }, [focusRuleId, onFocusHandled]);

  const toggle = (id: number) => setChecked(s => {
    const n = new Set(s);
    n.has(id) ? n.delete(id) : n.add(id);
    return n;
  });

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      setChecked(new Set());
      bump();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  const setStatus = (status: string) => act(() => kbApi()!.KBSetRuleStatus([...checked], status, ''));
  const selectedRules = rules.filter(r => checked.has(r.id));
  const [mergeTarget, setMergeTarget] = useState<string>('');
  useEffect(() => { setMergeTarget(selectedRules[0] ? String(selectedRules[0].id) : ''); }, [checked.size]);

  const catTitle = (id: string) => categories.find(c => c.id === id)?.title ?? id;

  return (
    <div className="flex h-full min-h-0">
      <div className="flex-1 flex flex-col min-w-0 border-r border-zinc-800">
        <form onSubmit={e => { e.preventDefault(); setFilter(f => ({ ...f, query })); }} className="flex flex-wrap items-center gap-1.5 p-2 border-b border-zinc-800 bg-[#161b22]/60">
          <div className="relative">
            <Search size={11} className="absolute left-2 top-1/2 -translate-y-1/2 text-zinc-500" />
            <input value={query} onChange={e => setQuery(e.target.value)} placeholder="Search rules…" className={`${inputCls} pl-6 w-48`} />
          </div>
          <Select value={filter.status} onChange={v => setFilter(f => ({ ...f, status: v }))}>
            <option value="">All statuses</option>
            {STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
          </Select>
          <Select value={filter.category} onChange={v => setFilter(f => ({ ...f, category: v }))}>
            <option value="">All areas</option>
            {categories.map(c => <option key={c.id} value={c.id}>{c.title}</option>)}
          </Select>
          <Select value={String(filter.min_occurrences)} onChange={v => setFilter(f => ({ ...f, min_occurrences: Number(v) }))}>
            <option value="0">Any frequency</option>
            <option value="2">Seen ≥ 2×</option>
            <option value="3">Seen ≥ 3×</option>
            <option value="5">Seen ≥ 5×</option>
          </Select>
          <span className="ml-auto text-[10px] text-zinc-500">{rules.length} rules</span>
        </form>

        {checked.size > 0 && (
          <div className="flex flex-wrap items-center gap-1.5 px-2 py-1.5 border-b border-zinc-800 bg-blue-950/10">
            <span className="text-[11px] text-zinc-400 mr-1">{checked.size} selected</span>
            <Btn busy={busy} onClick={() => setStatus('approved')}><Check size={11} /> Approve</Btn>
            <Btn busy={busy} onClick={() => setStatus('rejected')}><X size={11} /> Reject</Btn>
            <Btn busy={busy} onClick={() => setStatus('superseded')}><Archive size={11} /> Superseded</Btn>
            <Btn busy={busy} onClick={() => setStatus('pending')}><RotateCcw size={11} /> Reopen</Btn>
            {checked.size > 1 && (
              <span className="flex items-center gap-1 ml-2">
                <span className="text-[11px] text-zinc-500">merge into</span>
                <Select value={mergeTarget} onChange={setMergeTarget}>
                  {selectedRules.map(r => <option key={r.id} value={r.id}>{r.code}</option>)}
                </Select>
                <Btn busy={busy} onClick={() => act(() => kbApi()!.KBMergeRules(Number(mergeTarget), [...checked].filter(id => id !== Number(mergeTarget))))}>
                  <GitMerge size={11} /> Merge
                </Btn>
              </span>
            )}
            <Btn variant="ghost" onClick={() => setChecked(new Set())}>Clear</Btn>
          </div>
        )}

        <div className="flex-1 overflow-y-auto">
          {rules.length === 0 ? (
            <Empty>No rules. Rules are extracted automatically after each sync.</Empty>
          ) : (
            <table className="w-full text-[11px]">
              <thead className="sticky top-0 bg-[#161b22] text-[10px] uppercase tracking-wider text-zinc-500">
                <tr className="border-b border-zinc-800">
                  <th className="w-7 py-1.5 pl-2 text-left">
                    <input type="checkbox" checked={checked.size === rules.length} onChange={e => setChecked(e.target.checked ? new Set(rules.map(r => r.id)) : new Set())} />
                  </th>
                  <th className="py-1.5 text-left">ID</th>
                  <th className="py-1.5 text-left">Rule</th>
                  <th className="py-1.5 text-right" title="Occurrences">×</th>
                  <th className="py-1.5 text-right">PRs</th>
                  <th className="py-1.5 text-right" title="Confidence">Conf</th>
                  <th className="py-1.5 pr-2 text-right">Status</th>
                </tr>
              </thead>
              <tbody>
                {rules.map(r => (
                  <tr key={r.id} onClick={() => setOpenId(r.id)} className={`border-b border-zinc-800/60 cursor-pointer hover:bg-zinc-800/30 ${openId === r.id ? 'bg-blue-950/20' : ''}`}>
                    <td className="pl-2 py-1.5" onClick={e => e.stopPropagation()}>
                      <input type="checkbox" checked={checked.has(r.id)} onChange={() => toggle(r.id)} />
                    </td>
                    <td className="py-1.5 pr-2 font-mono text-zinc-500 whitespace-nowrap">{r.code}</td>
                    <td className="py-1.5 pr-2">
                      <div className="text-gray-200 leading-snug">{r.title}</div>
                      <div className="text-[10px] text-zinc-500">
                        {catTitle(r.category)}{r.subcategory ? ` · ${r.subcategory}` : ''} · {r.origin}{r.scope === 'general' ? ' · general' : ' · context-specific'}{r.edited ? ' · edited' : ''}
                      </div>
                    </td>
                    <td className="py-1.5 text-right tabular-nums">{r.occurrences}</td>
                    <td className="py-1.5 text-right tabular-nums">{r.distinct_prs}</td>
                    <td className="py-1.5 text-right tabular-nums">{r.confidence.toFixed(2)}</td>
                    <td className="py-1.5 pr-2 text-right"><Badge value={r.status} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="w-[44%] max-w-[640px] min-w-[380px] overflow-y-auto">
        {openId ? <RuleDetail id={openId} onChanged={bump} /> : <Empty>Select a rule to review its evidence.</Empty>}
      </div>
    </div>
  );
};

const RuleDetail: React.FC<{ id: number; onChanged: () => void }> = ({ id, onChanged }) => {
  const { categories, version, setError } = useKnowledgeStore();
  const [d, setD] = useState<KBRuleDetail | null>(null);
  const [edit, setEdit] = useState<KBRuleEdit | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    kbApi()?.KBRule(id).then(r => { setD(r); setEdit(null); }).catch(e => setError(String(e)));
  }, [id, version, setError]);

  if (!d) return <Empty>Loading…</Empty>;
  const r = d.rule;

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="p-3 space-y-3">
      <div className="flex items-center gap-2">
        <span className="font-mono text-[12px] text-zinc-400">{r.code}</span>
        <Badge value={r.status} />
        <Badge value={r.severity} />
        <span className="text-[10px] text-zinc-500 ml-auto font-mono" title="Stable extraction key">{r.rule_key}</span>
      </div>

      {edit ? (
        <div className="space-y-2">
          <Field label="Rule"><textarea rows={3} value={edit.title} onChange={e => setEdit({ ...edit, title: e.target.value })} className={`${inputCls} resize-y`} /></Field>
          <Field label="How to check"><textarea rows={3} value={edit.guidance} onChange={e => setEdit({ ...edit, guidance: e.target.value })} className={`${inputCls} resize-y`} /></Field>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Area">
              <Select value={edit.category} onChange={v => setEdit({ ...edit, category: v })}>
                {categories.map(c => <option key={c.id} value={c.id}>{c.title}</option>)}
              </Select>
            </Field>
            <Field label="Topic"><input value={edit.subcategory} onChange={e => setEdit({ ...edit, subcategory: e.target.value })} className={inputCls} /></Field>
            <Field label="Severity">
              <Select value={edit.severity} onChange={v => setEdit({ ...edit, severity: v })}>
                {SEVERITIES.map(s => <option key={s} value={s}>{s}</option>)}
              </Select>
            </Field>
          </div>
          <Field label="Note (not published)"><input value={edit.note} onChange={e => setEdit({ ...edit, note: e.target.value })} className={inputCls} /></Field>
          <div className="flex justify-end gap-1.5">
            <Btn variant="ghost" onClick={() => setEdit(null)}>Cancel</Btn>
            <Btn variant="primary" busy={busy} onClick={() => run(() => kbApi()!.KBEditRule(r.id, edit))}>Save</Btn>
          </div>
          <p className="text-[10px] text-zinc-600">Edited rules keep your text. Later extractions update only the statistics and evidence.</p>
        </div>
      ) : (
        <div>
          <div className="text-[13px] text-gray-100 leading-snug select-text">{r.title}</div>
          {r.guidance && <div className="text-[11px] text-zinc-400 mt-1.5 leading-relaxed select-text"><b className="text-zinc-300">How to check:</b> {r.guidance}</div>}
          {r.note && <div className="text-[11px] text-zinc-500 mt-1.5 italic select-text">{r.note}</div>}
        </div>
      )}

      <div className="grid grid-cols-4 gap-2 text-center">
        {[['Occurrences', r.occurrences], ['PRs', r.distinct_prs], ['Reviewers', r.distinct_reviewers], ['Confidence', r.confidence.toFixed(2)]].map(([k, v]) => (
          <div key={k as string} className="rounded border border-zinc-800 bg-[#0d1117] py-1.5">
            <div className="text-[14px] font-bold text-gray-200">{v}</div>
            <div className="text-[9px] uppercase tracking-wider text-zinc-500">{k}</div>
          </div>
        ))}
      </div>
      <div className="text-[10px] text-zinc-500">
        {Math.round(r.requirement_ratio * 100)}% explicit requirements · {Math.round(r.addressed_ratio * 100)}% led to a code change · seen {shortDate(r.first_seen_at)} → {shortDate(r.last_seen_at)}
      </div>

      {!edit && (
        <div className="flex flex-wrap gap-1.5">
          {r.status !== 'approved' && <Btn variant="primary" busy={busy} onClick={() => run(() => kbApi()!.KBSetRuleStatus([r.id], 'approved', ''))}><Check size={11} /> Approve</Btn>}
          {r.status !== 'rejected' && <Btn busy={busy} onClick={() => run(() => kbApi()!.KBSetRuleStatus([r.id], 'rejected', ''))}><X size={11} /> Reject</Btn>}
          {r.status !== 'superseded' && <Btn busy={busy} onClick={() => run(() => kbApi()!.KBSetRuleStatus([r.id], 'superseded', ''))}><Archive size={11} /> Superseded</Btn>}
          {r.status !== 'pending' && <Btn busy={busy} onClick={() => run(() => kbApi()!.KBSetRuleStatus([r.id], 'pending', ''))}><RotateCcw size={11} /> Reopen</Btn>}
          <Btn variant="ghost" onClick={() => setEdit({ title: r.title, guidance: r.guidance, category: r.category, subcategory: r.subcategory, severity: r.severity, note: r.note })}>Edit</Btn>
        </div>
      )}

      {(d.merged ?? []).length > 0 && (
        <div className="text-[11px] text-zinc-400">Merged here: {(d.merged ?? []).map(m => m.code).join(', ')}</div>
      )}

      <div className="border-t border-zinc-800 pt-2">
        <div className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 mb-2">
          Supporting reviewer comments ({(d.evidence ?? []).filter(e => e.link_source !== 'excluded').length})
        </div>
        <div className="space-y-2">
          {(d.evidence ?? []).map(e => (
            <div key={e.id} className={`rounded border px-2.5 py-2 ${e.link_source === 'excluded' ? 'border-zinc-800 opacity-50' : 'border-zinc-800 bg-[#0d1117]'}`}>
              <div className="flex items-center gap-1.5 text-[10px] text-zinc-500 mb-1">
                <span className="font-semibold text-gray-300">{e.author}</span>
                <span>{e.repo}#{e.pr_number}</span>
                <span>{shortDate(e.created_at)}</span>
                {e.kind && <Badge value={e.kind} />}
                {e.code_changed === 'yes' && <Badge value="code changed" />}
                {e.link_source !== 'auto' && <Badge value={e.link_source ?? ''} />}
                <span className="ml-auto flex gap-1">
                  <button className="hover:text-blue-400" title="Open on GitHub" onClick={() => openURL(e.html_url || e.pr_url)}><ExternalLink size={11} /></button>
                  {e.link_source === 'excluded' ? (
                    <button className="hover:text-green-400" onClick={() => run(() => kbApi()!.KBSetEvidence(r.id, e.id, 'remove'))}>restore</button>
                  ) : (
                    <button className="hover:text-red-400" title="This comment does not support the rule" onClick={() => run(() => kbApi()!.KBSetEvidence(r.id, e.id, 'excluded'))}>exclude</button>
                  )}
                </span>
              </div>
              {e.path && <div className="text-[10px] font-mono text-zinc-500 mb-1">{e.path}{e.line || e.original_line ? `:${e.line || e.original_line}` : ''}</div>}
              <div className="text-[11px] text-gray-200 whitespace-pre-wrap break-words select-text leading-relaxed">{e.body}</div>
              {e.diff_hunk && (
                <details className="mt-1">
                  <summary className="text-[10px] text-zinc-500 cursor-pointer">code context</summary>
                  <DiffHunk text={e.diff_hunk} />
                </details>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
