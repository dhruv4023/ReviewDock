import React, { useCallback, useEffect, useState } from 'react';
import { ExternalLink, Search, Plus } from 'lucide-react';
import {
  EMPTY_COMMENT_FILTER, KBComment, KBCommentFilter, KBRepository, kbApi, useKnowledgeStore,
} from '../../stores/knowledgeStore';
import { Badge, Btn, DiffHunk, Empty, Field, Select, inputCls, openURL, shortDate } from './ui';

const KINDS = ['requirement', 'suggestion', 'question', 'superseded', 'unclear', 'noise', 'self', 'bot'];

export const CommentsTab: React.FC<{ onOpenRule: (id: number) => void }> = ({ onOpenRule }) => {
  const { categories, reviewers, version, bump, setError } = useKnowledgeStore();
  const [filter, setFilter] = useState<KBCommentFilter>(EMPTY_COMMENT_FILTER);
  const [query, setQuery] = useState('');
  const [items, setItems] = useState<KBComment[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [repos, setRepos] = useState<KBRepository[]>([]);
  const [selected, setSelected] = useState<KBComment | null>(null);

  const load = useCallback(async (f: KBCommentFilter, append = false) => {
    const api = kbApi();
    if (!api) return;
    setLoading(true);
    try {
      const page = await api.KBSearchComments(f);
      setItems(prev => (append ? [...prev, ...(page.items ?? [])] : page.items ?? []));
      setTotal(page.total);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [setError]);

  useEffect(() => { load(filter); }, [filter, version, load]);
  useEffect(() => { kbApi()?.KBRepositories().then(r => setRepos(r ?? [])).catch(() => {}); }, [version]);

  const update = (patch: Partial<KBCommentFilter>) => setFilter(f => ({ ...f, ...patch, offset: 0 }));

  return (
    <div className="flex h-full min-h-0">
      <div className="flex-1 flex flex-col min-w-0 border-r border-zinc-800">
        <form
          onSubmit={e => { e.preventDefault(); update({ query }); }}
          className="flex flex-wrap items-center gap-1.5 p-2 border-b border-zinc-800 bg-[#161b22]/60"
        >
          <div className="relative">
            <Search size={11} className="absolute left-2 top-1/2 -translate-y-1/2 text-zinc-500" />
            <input value={query} onChange={e => setQuery(e.target.value)} placeholder="Search text, path, PR title…" className={`${inputCls} pl-6 w-56`} />
          </div>
          <Select value={filter.repo} onChange={v => update({ repo: v })}>
            <option value="">All repositories</option>
            {repos.map(r => <option key={r.id} value={r.full_name}>{r.full_name}</option>)}
          </Select>
          <Select value={filter.reviewer} onChange={v => update({ reviewer: v })}>
            <option value="">All reviewers</option>
            {reviewers.map(r => <option key={r} value={r}>{r}</option>)}
          </Select>
          <Select value={filter.category} onChange={v => update({ category: v })}>
            <option value="">All areas</option>
            {categories.map(c => <option key={c.id} value={c.id}>{c.title}</option>)}
          </Select>
          <Select value={filter.kind} onChange={v => update({ kind: v })}>
            <option value="">All kinds</option>
            {KINDS.map(k => <option key={k} value={k}>{k}</option>)}
          </Select>
          <input type="date" value={filter.since} onChange={e => update({ since: e.target.value })} className={inputCls} title="From" />
          <input type="date" value={filter.until} onChange={e => update({ until: e.target.value })} className={inputCls} title="Until" />
          <span className="ml-auto text-[10px] text-zinc-500 tabular-nums">{total} comments</span>
        </form>

        <div className="flex-1 overflow-y-auto">
          {items.length === 0 && !loading ? (
            <Empty>No comments match. Sync repositories from the Sync tab first.</Empty>
          ) : (
            items.map(c => (
              <button
                key={c.id}
                onClick={() => setSelected(c)}
                className={`w-full text-left px-3 py-2 border-b border-zinc-800/70 hover:bg-zinc-800/30 transition ${selected?.id === c.id ? 'bg-blue-950/20' : ''}`}
              >
                <div className="flex items-center gap-1.5 text-[10px] text-zinc-500 mb-1">
                  <span className="font-semibold text-gray-300">{c.author}</span>
                  <span>on {c.repo}#{c.pr_number}</span>
                  {c.path && <span className="font-mono truncate max-w-[260px]">{c.path}{c.line || c.original_line ? `:${c.line || c.original_line}` : ''}</span>}
                  <span className="ml-auto whitespace-nowrap">{shortDate(c.created_at)}</span>
                </div>
                <div className="text-[11px] text-gray-200 line-clamp-2 leading-snug">{c.body || <i className="text-zinc-500">(no text — {c.review_state || c.source})</i>}</div>
                <div className="flex items-center gap-1 mt-1">
                  {c.kind && <Badge value={c.kind} />}
                  {c.category && c.kind !== 'self' && c.kind !== 'bot' && <Badge value={c.category} />}
                  {c.code_changed === 'yes' && <Badge value="code changed" title="The commented lines changed afterwards" />}
                  {c.is_resolved && <Badge value="resolved" />}
                  {(c.rule_ids ?? []).length > 0 && <span className="text-[10px] text-blue-400">{(c.rule_ids ?? []).length} rule(s)</span>}
                </div>
              </button>
            ))
          )}
          {items.length < total && (
            <div className="p-2 text-center">
              <Btn busy={loading} onClick={() => { const f = { ...filter, offset: items.length }; load(f, true); }}>Load more</Btn>
            </div>
          )}
        </div>
      </div>

      <div className="w-[44%] max-w-[620px] min-w-[360px] overflow-y-auto">
        {selected ? (
          <CommentDetail comment={selected} onOpenRule={onOpenRule} onChanged={bump} />
        ) : (
          <Empty>Select a comment to inspect its thread and code context.</Empty>
        )}
      </div>
    </div>
  );
};

const CommentDetail: React.FC<{ comment: KBComment; onOpenRule: (id: number) => void; onChanged: () => void }> = ({ comment: c, onOpenRule, onChanged }) => {
  const { categories, setError } = useKnowledgeStore();
  const [thread, setThread] = useState<KBComment[]>([]);
  const [revisions, setRevisions] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState('');
  const [category, setCategory] = useState('general');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const api = kbApi();
    if (!api) return;
    setCreating(false);
    setTitle((c.body.split(/(?<=[.!?])\s|\n/)[0] || '').replace(/[`*>]/g, '').trim().slice(0, 200));
    setCategory(c.category || 'general');
    api.KBCommentThread(c.id).then(t => setThread(t ?? [])).catch(() => setThread([]));
    if (c.revisions > 0) api.KBCommentRevisions(c.id).then(r => setRevisions(r ?? [])).catch(() => {});
    else setRevisions([]);
  }, [c.id]);

  const createRule = async () => {
    const api = kbApi();
    if (!api) return;
    setBusy(true);
    try {
      const id = await api.KBCreateRuleFromComment(c.id, { title, guidance: '', category, subcategory: '', severity: 'context-dependent', note: `created from comment ${c.html_url}` });
      setCreating(false);
      onChanged();
      onOpenRule(id);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="p-3 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="text-[12px] font-semibold text-gray-200">{c.pr_title}</div>
          <div className="text-[10px] text-zinc-500">{c.repo}#{c.pr_number} · by {c.pr_author} · {c.source} comment {c.github_id}</div>
        </div>
        <Btn onClick={() => openURL(c.html_url || c.pr_url)}><ExternalLink size={11} /> GitHub</Btn>
      </div>

      <div className="flex flex-wrap gap-1">
        {c.kind && <Badge value={c.kind} />}
        {c.category && <Badge value={c.category} />}
        {c.subcategory && <Badge value={c.subcategory} />}
        {c.review_state && <Badge value={c.review_state.toLowerCase()} />}
        {c.is_outdated && <Badge value="outdated" />}
        {c.deleted_at && <Badge value="deleted" />}
        {c.signals && c.signals.split(',').map(s => <Badge key={s} value={s} title="matched signal" />)}
      </div>

      {c.path && <div className="text-[10px] font-mono text-zinc-400">{c.path}{c.line || c.original_line ? `:${c.line || c.original_line}` : ''}</div>}
      <DiffHunk text={c.diff_hunk} />

      <div className="space-y-2">
        {(thread.length ? thread : [c]).map(t => (
          <div key={t.id} className={`rounded border px-2.5 py-2 ${t.id === c.id ? 'border-blue-800/60 bg-blue-950/10' : 'border-zinc-800 bg-[#0d1117]'}`}>
            <div className="flex items-center gap-1.5 text-[10px] text-zinc-500 mb-1">
              <span className="font-semibold text-gray-300">{t.author}</span>
              {t.author === t.pr_author && <span className="text-zinc-600">(PR author)</span>}
              <span className="ml-auto">{shortDate(t.created_at)}</span>
              {t.kind && <Badge value={t.kind} />}
            </div>
            <div className="text-[11px] text-gray-200 whitespace-pre-wrap break-words select-text leading-relaxed">{t.body || <i className="text-zinc-500">(no text)</i>}</div>
          </div>
        ))}
      </div>

      {revisions.length > 0 && (
        <details className="text-[11px]">
          <summary className="cursor-pointer text-zinc-400">Edited — {revisions.length} earlier version(s) preserved</summary>
          {revisions.map((r, i) => (
            <div key={i} className="mt-1 rounded border border-zinc-800 bg-[#0d1117] px-2 py-1.5 text-zinc-400 whitespace-pre-wrap select-text">{r}</div>
          ))}
        </details>
      )}

      <div className="border-t border-zinc-800 pt-3">
        <div className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 mb-1.5">Linked rules</div>
        <div className="flex flex-wrap gap-1.5 items-center">
          {(c.rule_ids ?? []).map(id => (
            <button key={id} onClick={() => onOpenRule(id)} className="font-mono text-[11px] text-blue-400 hover:underline">R-{String(id).padStart(4, '0')}</button>
          ))}
          {(c.rule_ids ?? []).length === 0 && <span className="text-[11px] text-zinc-500">none</span>}
          {!creating && <Btn variant="ghost" onClick={() => setCreating(true)}><Plus size={11} /> Create rule from this comment</Btn>}
        </div>
        {creating && (
          <div className="mt-2 space-y-2 p-2 rounded border border-zinc-800 bg-[#0d1117]">
            <Field label="Rule">
              <textarea value={title} onChange={e => setTitle(e.target.value)} rows={2} className={`${inputCls} resize-none`} />
            </Field>
            <Field label="Area">
              <Select value={category} onChange={setCategory}>
                {categories.map(cat => <option key={cat.id} value={cat.id}>{cat.title}</option>)}
              </Select>
            </Field>
            <div className="flex gap-1.5 justify-end">
              <Btn variant="ghost" onClick={() => setCreating(false)}>Cancel</Btn>
              <Btn variant="primary" busy={busy} disabled={!title.trim()} onClick={createRule}>Create (pending)</Btn>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
