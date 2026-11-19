import React, { useCallback, useEffect, useState } from 'react';
import { Play, RotateCcw, Trash2, Plus, Save, RefreshCw } from 'lucide-react';
import { KBRepository, KBSettings, KBSyncRun, kbApi, useKnowledgeStore } from '../../stores/knowledgeStore';
import { Badge, Btn, Empty, Field, Panel, inputCls, relTime, shortDate } from './ui';

// "21:05" (input[type=time]) → "09:05 PM" (format shared with the rebase schedule).
function to12h(hhmm: string): string {
  const [h, m] = hhmm.split(':').map(Number);
  const period = h >= 12 ? 'PM' : 'AM';
  const h12 = h % 12 === 0 ? 12 : h % 12;
  return `${String(h12).padStart(2, '0')}:${String(m).padStart(2, '0')} ${period}`;
}

export function useKBSettings() {
  const { version, setError } = useKnowledgeStore();
  const [settings, setSettings] = useState<KBSettings | null>(null);
  const [saved, setSaved] = useState(false);
  useEffect(() => { kbApi()?.KBSettings().then(setSettings).catch(e => setError(String(e))); }, [version, setError]);
  const save = async (s: KBSettings) => {
    try {
      await kbApi()!.KBSaveSettings(s);
      setSaved(true);
      setTimeout(() => setSaved(false), 1500);
      setSettings(await kbApi()!.KBSettings());
    } catch (e) {
      setError(String(e));
    }
  };
  return { settings, setSettings, save, saved };
}

export const SyncTab: React.FC = () => {
  const { running, runSync, retryFailed, version, bump, setError, events } = useKnowledgeStore();
  const [repos, setRepos] = useState<KBRepository[]>([]);
  const [runs, setRuns] = useState<KBSyncRun[]>([]);
  const [newRepo, setNewRepo] = useState('');
  const [newAuthor, setNewAuthor] = useState('@me');
  const [authors, setAuthors] = useState<Record<number, string>>({});
  const [newTime, setNewTime] = useState('09:00');
  const { settings, setSettings, save, saved } = useKBSettings();

  const load = useCallback(async () => {
    const api = kbApi();
    if (!api) return;
    try {
      const [r, h] = await Promise.all([api.KBRepositories(), api.KBRuns(30)]);
      setRepos(r ?? []);
      setRuns(h ?? []);
      setAuthors(Object.fromEntries((r ?? []).map(x => [x.id, x.author_filter])));
    } catch (e) {
      setError(String(e));
    }
  }, [setError]);
  useEffect(() => { load(); }, [load, version]);
  // Refresh repo status live while a run is progressing.
  useEffect(() => { if (running) load(); }, [events.length]);

  const call = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      bump();
    } catch (e) {
      setError(String(e));
    }
  };

  const failed = repos.some(r => r.enabled && ['error', 'rate_limited', 'interrupted'].includes(r.sync_status));

  return (
    <div className="p-4 space-y-4 overflow-y-auto h-full">
      <Panel
        title="Repositories"
        actions={
          <>
            <Btn variant="primary" busy={running} onClick={() => runSync()}><Play size={11} /> Sync all now</Btn>
            <Btn disabled={!failed || running} onClick={retryFailed}><RotateCcw size={11} /> Retry failed</Btn>
          </>
        }
      >
        <table className="w-full text-[11px]">
          <thead className="text-[10px] uppercase tracking-wider text-zinc-500">
            <tr className="border-b border-zinc-800">
              <th className="py-1.5 text-left">Repository</th>
              <th className="py-1.5 text-left" title="GitHub search author qualifier: whose PRs are collected">PR author</th>
              <th className="py-1.5 text-left">Status</th>
              <th className="py-1.5 text-right">PRs</th>
              <th className="py-1.5 text-right">Comments</th>
              <th className="py-1.5 text-left pl-3" title="Next incremental sync fetches PRs updated since this time">Checkpoint</th>
              <th className="py-1.5 text-left">Last success</th>
              <th className="py-1.5" />
            </tr>
          </thead>
          <tbody>
            {repos.map(r => (
              <tr key={r.id} className="border-b border-zinc-800/60 align-top">
                <td className="py-1.5 pr-2">
                  <label className="flex items-center gap-1.5">
                    <input type="checkbox" checked={r.enabled} onChange={e => call(() => kbApi()!.KBUpdateRepository(r.id, e.target.checked, r.author_filter))} />
                    <span className={r.enabled ? 'text-gray-200' : 'text-zinc-500'}>{r.full_name}</span>
                  </label>
                  {r.last_error && <div className="text-[10px] text-red-400 mt-0.5 max-w-[420px] break-words select-text">{r.last_error}</div>}
                </td>
                <td className="py-1.5 pr-2">
                  <input
                    value={authors[r.id] ?? ''}
                    onChange={e => setAuthors(a => ({ ...a, [r.id]: e.target.value }))}
                    onBlur={() => authors[r.id] !== r.author_filter && call(() => kbApi()!.KBUpdateRepository(r.id, r.enabled, authors[r.id]))}
                    className={`${inputCls} w-24`}
                  />
                </td>
                <td className="py-1.5 pr-2">
                  <Badge value={r.sync_status} />
                  {!r.full_import_done && r.sync_status !== 'never' && <div className="text-[10px] text-zinc-500 mt-0.5">history import in progress</div>}
                </td>
                <td className="py-1.5 text-right tabular-nums">{r.pr_count}</td>
                <td className="py-1.5 text-right tabular-nums">{r.comment_count}</td>
                <td className="py-1.5 pl-3 text-zinc-400">{shortDate(r.checkpoint_updated_at)}</td>
                <td className="py-1.5 text-zinc-400">{relTime(r.last_success_at)}</td>
                <td className="py-1.5 text-right whitespace-nowrap">
                  <Btn variant="ghost" disabled={running || !r.enabled} title="Sync this repository" onClick={() => runSync([r.id])}><RefreshCw size={11} /></Btn>
                  <Btn variant="ghost" title="Re-import full history on next sync (keeps data)" onClick={() => call(() => kbApi()!.KBResetRepository(r.id))}><RotateCcw size={11} /></Btn>
                  <Btn variant="ghost" title="Remove repository and its collected comments" onClick={() => {
                    if (confirm(`Remove ${r.full_name} and all its collected comments from the knowledge database?`)) call(() => kbApi()!.KBRemoveRepository(r.id));
                  }}><Trash2 size={11} /></Btn>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {repos.length === 0 && <Empty>Repositories added in the sidebar appear here automatically.</Empty>}
        <form
          onSubmit={e => { e.preventDefault(); call(async () => { await kbApi()!.KBAddRepository(newRepo, newAuthor); setNewRepo(''); }); }}
          className="flex items-center gap-1.5 mt-3"
        >
          <input value={newRepo} onChange={e => setNewRepo(e.target.value)} placeholder="owner/name (no local clone needed)" className={`${inputCls} w-64`} />
          <input value={newAuthor} onChange={e => setNewAuthor(e.target.value)} placeholder="@me" className={`${inputCls} w-24`} title="PR author" />
          <Btn type="submit" disabled={!newRepo.includes('/')}><Plus size={11} /> Add</Btn>
        </form>
      </Panel>

      {settings && (
        <Panel title="Schedule & extraction" actions={<Btn variant="primary" onClick={() => save(settings)}><Save size={11} /> {saved ? 'Saved' : 'Save'}</Btn>}>
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            <Field label="Scheduled sync" hint="Runs at these times only while review-dock is open, like the scheduled rebase. Nothing runs when the app is closed.">
              <label className="flex items-center gap-1.5 text-[11px] text-gray-200">
                <input type="checkbox" checked={settings.schedule_enabled} onChange={e => setSettings({ ...settings, schedule_enabled: e.target.checked })} /> enabled
              </label>
            </Field>
            <Field label="Times" hint="Sync → extract → publish skill at each time.">
              <div className="flex flex-wrap items-center gap-1">
                {(settings.schedule_times ?? []).map(t => (
                  <span key={t} className="flex items-center gap-1 px-1.5 py-0.5 bg-zinc-800 border border-zinc-700 rounded text-[11px] text-zinc-200">
                    {t}
                    <button type="button" title="Remove" className="text-zinc-500 hover:text-red-400"
                      onClick={() => setSettings({ ...settings, schedule_times: settings.schedule_times.filter(x => x !== t) })}>✕</button>
                  </span>
                ))}
                <input type="time" value={newTime} onChange={e => setNewTime(e.target.value)} className={`${inputCls} w-24`} />
                <Btn disabled={!newTime} onClick={() => {
                  const t = to12h(newTime);
                  if (!settings.schedule_times.includes(t)) setSettings({ ...settings, schedule_times: [...settings.schedule_times, t] });
                }}><Plus size={11} /> Add</Btn>
              </div>
            </Field>
            <Field label="Ignored authors" hint="Bots and merge bots (comma separated). [bot] accounts are always ignored.">
              <input value={settings.ignored_authors.join(', ')} onChange={e => setSettings({ ...settings, ignored_authors: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })} className={inputCls} />
            </Field>
            <Field label="Auto-approve" hint="0 = never. Otherwise taxonomy rules seen in at least N PRs are approved automatically.">
              <input type="number" min={0} value={settings.auto_approve_min_prs} onChange={e => setSettings({ ...settings, auto_approve_min_prs: Number(e.target.value) })} className={inputCls} />
            </Field>
          </div>
        </Panel>
      )}

      <Panel title="Synchronization history">
        {runs.length === 0 ? <Empty>No runs yet.</Empty> : (
          <table className="w-full text-[11px]">
            <thead className="text-[10px] uppercase tracking-wider text-zinc-500">
              <tr className="border-b border-zinc-800">
                <th className="py-1.5 text-left">#</th>
                <th className="py-1.5 text-left">Started</th>
                <th className="py-1.5 text-left">Trigger</th>
                <th className="py-1.5 text-left">Status</th>
                <th className="py-1.5 text-right">Repos</th>
                <th className="py-1.5 text-right">PRs</th>
                <th className="py-1.5 text-right">Changed</th>
                <th className="py-1.5 text-right">Deleted</th>
                <th className="py-1.5 text-right">Rules</th>
              </tr>
            </thead>
            <tbody>
              {runs.map(run => (
                <React.Fragment key={run.id}>
                  <tr className="border-b border-zinc-800/40">
                    <td className="py-1.5 text-zinc-500">{run.id}</td>
                    <td className="py-1.5" title={run.started_at}>{relTime(run.started_at)}</td>
                    <td className="py-1.5 text-zinc-400">{run.trigger}</td>
                    <td className="py-1.5"><Badge value={run.status} /></td>
                    <td className="py-1.5 text-right tabular-nums">{run.repos_total}{run.repos_failed ? <span className="text-red-400"> ({run.repos_failed}✗)</span> : null}</td>
                    <td className="py-1.5 text-right tabular-nums">{run.prs_synced}</td>
                    <td className="py-1.5 text-right tabular-nums">{run.comments_upserted}</td>
                    <td className="py-1.5 text-right tabular-nums">{run.comments_deleted}</td>
                    <td className="py-1.5 text-right tabular-nums">{run.rules_total}</td>
                  </tr>
                  {(run.error || (run.repos ?? []).some(x => x.error)) && (
                    <tr className="border-b border-zinc-800/60">
                      <td />
                      <td colSpan={8} className="pb-1.5 text-[10px] text-red-400 select-text break-words">
                        {(run.repos ?? []).filter(x => x.error).map(x => <div key={x.repo_id}>{x.full_name}: {x.error}</div>)}
                        {!(run.repos ?? []).some(x => x.error) && run.error}
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </div>
  );
};
