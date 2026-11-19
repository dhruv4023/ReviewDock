import React, { useEffect, useState } from 'react';
import { Hammer, Eye, ShieldCheck, ShieldAlert, Save, Copy, CheckCircle2 } from 'lucide-react';
import { KBPreview, KBSkillInfo, kbApi, useKnowledgeStore } from '../../stores/knowledgeStore';
import { useKBSettings } from './SyncTab';
import { Badge, Btn, Empty, Field, Panel, copyText, inputCls, relTime } from './ui';

function DiffView({ text }: { text: string }) {
  if (!text) return <Empty>No differences: the published skill matches the database.</Empty>;
  return (
    <pre className="bg-[#0b0e14] border border-zinc-800 rounded text-[10.5px] font-mono overflow-auto max-h-[420px] py-1 select-text">
      {text.split('\n').map((l, i) => (
        <div key={i} className={`px-2 whitespace-pre ${l.startsWith('+++') || l.startsWith('---') ? 'text-zinc-300 font-bold' : l.startsWith('+') ? 'text-green-400 bg-green-950/20' : l.startsWith('-') ? 'text-red-400 bg-red-950/20' : l.startsWith('@@') ? 'text-blue-400' : 'text-zinc-500'}`}>{l || ' '}</div>
      ))}
    </pre>
  );
}

function changesSummary(json: string): string {
  try {
    const c = JSON.parse(json) as { added: string[]; removed: string[]; changed: string[] };
    const parts = [];
    if (c.added.length) parts.push(`+${c.added.length}`);
    if (c.removed.length) parts.push(`−${c.removed.length}`);
    if (c.changed.length) parts.push(`~${c.changed.length}`);
    return parts.join(' ') || 'no rule changes';
  } catch {
    return '';
  }
}

export const SkillTab: React.FC = () => {
  const { version, bump, setError, refreshOverview } = useKnowledgeStore();
  const [info, setInfo] = useState<KBSkillInfo | null>(null);
  const [file, setFile] = useState('SKILL.md');
  const [preview, setPreview] = useState<KBPreview | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const { settings, setSettings, save, saved } = useKBSettings();

  useEffect(() => { kbApi()?.KBSkillInfo().then(setInfo).catch(e => setError(String(e))); }, [version, setError]);

  const rebuild = async (publish: boolean) => {
    setBusy(publish ? 'build' : 'validate');
    setMessage(null);
    try {
      const r = await kbApi()!.KBRebuildSkill(publish);
      const b = r.build;
      setMessage(publish
        ? `${b.status}: version ${b.content_hash.slice(0, 12)} · ${b.rule_count} rules · ${b.network_calls} network / ${b.llm_calls} LLM calls`
        : `Valid: would produce version ${b.content_hash.slice(0, 12)} with ${b.rule_count} rules`);
      bump();
      refreshOverview();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const doPreview = async () => {
    setBusy('preview');
    try {
      setPreview(await kbApi()!.KBPreviewSkill());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  if (!info) return <Empty>Loading…</Empty>;
  const zt = info.zero_token;
  const zeroOK = zt.forbidden_linked.length === 0 && zt.last_build_network_calls === 0 && zt.last_build_llm_calls === 0;
  const files = info.file_order ?? [];

  return (
    <div className="p-4 space-y-4 overflow-y-auto h-full">
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
        <Panel
          title="Published skill"
          className="xl:col-span-2"
          actions={
            <>
              <Btn busy={busy === 'preview'} onClick={doPreview}><Eye size={11} /> Preview changes</Btn>
              <Btn busy={busy === 'validate'} onClick={() => rebuild(false)}><CheckCircle2 size={11} /> Validate</Btn>
              <Btn variant="primary" busy={busy === 'build'} onClick={() => rebuild(true)}><Hammer size={11} /> Rebuild & publish</Btn>
            </>
          }
        >
          <div className="space-y-1.5 text-[11px]">
            <div className="flex items-center gap-2">
              <span className="text-zinc-500 w-24">Version</span>
              <span className="font-mono text-gray-200">{info.version || 'not published yet'}</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-zinc-500 w-24">Current</span>
              <span className="font-mono text-zinc-300 select-text truncate">{info.current_link}</span>
              <button onClick={() => copyText(info.current_link)} className="text-zinc-500 hover:text-gray-200" title="Copy path"><Copy size={11} /></button>
            </div>
            {(info.targets ?? []).map(t => (
              <div key={t} className="flex items-center gap-2">
                <span className="text-zinc-500 w-24">Agent link</span>
                <span className="font-mono text-zinc-300 select-text truncate">{t}</span>
                <button onClick={() => copyText(t)} className="text-zinc-500 hover:text-gray-200" title="Copy path"><Copy size={11} /></button>
              </div>
            ))}
            {message && <div className="text-[11px] text-green-400 pt-1 select-text">{message}</div>}
          </div>
        </Panel>

        <Panel title={<span className="flex items-center gap-1.5">{zeroOK ? <ShieldCheck size={12} className="text-green-400" /> : <ShieldAlert size={12} className="text-red-400" />} Zero-LLM verification</span>}>
          <ul className="text-[11px] space-y-1">
            <li className={zt.forbidden_linked.length === 0 ? 'text-green-400' : 'text-red-400'}>
              {zt.forbidden_linked.length === 0 ? '✓' : '✗'} No LLM/embedding SDK linked ({zt.linked_modules} modules inspected)
            </li>
            <li className={zt.last_build_network_calls === 0 ? 'text-green-400' : 'text-red-400'}>
              {zt.last_build_network_calls === 0 ? '✓' : '✗'} Last build: {zt.last_build_network_calls} network calls (HTTP guard active)
            </li>
            <li className={zt.last_build_llm_calls === 0 ? 'text-green-400' : 'text-red-400'}>
              {zt.last_build_llm_calls === 0 ? '✓' : '✗'} Last build: {zt.last_build_llm_calls} LLM calls
            </li>
          </ul>
          <p className="text-[10px] text-zinc-500 mt-2 leading-snug">{zt.statement}</p>
          {zt.forbidden_linked.length > 0 && <p className="text-[10px] text-red-400 mt-1">{zt.forbidden_linked.join(', ')}</p>}
        </Panel>
      </div>

      {preview && (
        <Panel
          title={`Preview: ${preview.current_version || 'none'} → ${preview.version} (${preview.rule_count} rules)`}
          actions={<Btn variant="ghost" onClick={() => setPreview(null)}>Close</Btn>}
        >
          {(preview.errors ?? []).length > 0 && (
            <div className="text-[11px] text-red-400 mb-2">{(preview.errors ?? []).map(e => <div key={e}>✗ {e}</div>)}</div>
          )}
          <DiffView text={preview.diff} />
        </Panel>
      )}

      <Panel title="Skill files">
        {files.length === 0 ? <Empty>Nothing published yet. Click "Rebuild & publish".</Empty> : (
          <div className="flex gap-3 min-h-[300px]">
            <div className="w-56 shrink-0 space-y-0.5">
              {files.map(f => (
                <button key={f} onClick={() => setFile(f)} className={`block w-full text-left px-2 py-1 rounded font-mono text-[11px] truncate ${file === f ? 'bg-blue-950/30 text-blue-300' : 'text-zinc-400 hover:bg-zinc-800/50'}`}>{f}</button>
              ))}
            </div>
            <pre className="flex-1 min-w-0 bg-[#0b0e14] border border-zinc-800 rounded p-3 text-[11px] font-mono text-zinc-300 whitespace-pre-wrap break-words overflow-auto max-h-[560px] select-text">
              {info.files[file] ?? ''}
            </pre>
          </div>
        )}
      </Panel>

      {settings && (
        <Panel title="Generation & publishing" actions={<Btn variant="primary" onClick={() => save(settings)}><Save size={11} /> {saved ? 'Saved' : 'Save'}</Btn>}>
          <div className="grid grid-cols-2 lg:grid-cols-3 gap-3">
            <Field label="Skill name" hint="Lowercase letters, digits and dashes.">
              <input value={settings.skill_name} onChange={e => setSettings({ ...settings, skill_name: e.target.value })} className={inputCls} />
            </Field>
            <Field label="Publish to (skill directories)" hint="Comma separated. A symlink <dir>/<skill name> to the current version is maintained; existing real folders are never overwritten.">
              <input value={settings.publish_targets.join(', ')} onChange={e => setSettings({ ...settings, publish_targets: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })} className={inputCls} />
            </Field>
            <Field label="Versions kept" hint="Older versions are pruned; the current one is always kept.">
              <input type="number" min={2} value={settings.keep_versions} onChange={e => setSettings({ ...settings, keep_versions: Number(e.target.value) })} className={inputCls} />
            </Field>
            <Field label="Minimum occurrences" hint="Rules with fewer supporting comments are left out of the skill.">
              <input type="number" min={1} value={settings.min_occurrences} onChange={e => setSettings({ ...settings, min_occurrences: Number(e.target.value) })} className={inputCls} />
            </Field>
            <Field label="Publish after each sync">
              <label className="flex items-center gap-1.5 text-[11px] text-gray-200">
                <input type="checkbox" checked={settings.auto_publish} onChange={e => setSettings({ ...settings, auto_publish: e.target.checked })} /> automatically
              </label>
            </Field>
            <Field label="Unapproved rules" hint="Off by default: only approved rules are published.">
              <label className="flex items-center gap-1.5 text-[11px] text-gray-200">
                <input type="checkbox" checked={settings.include_pending} onChange={e => setSettings({ ...settings, include_pending: e.target.checked })} /> include, marked as unapproved
              </label>
            </Field>
          </div>
        </Panel>
      )}

      <Panel title="Generation history">
        {(info.builds ?? []).length === 0 ? <Empty>No builds yet.</Empty> : (
          <table className="w-full text-[11px]">
            <thead className="text-[10px] uppercase tracking-wider text-zinc-500">
              <tr className="border-b border-zinc-800">
                <th className="py-1.5 text-left">When</th>
                <th className="py-1.5 text-left">Status</th>
                <th className="py-1.5 text-left">Version</th>
                <th className="py-1.5 text-right">Rules</th>
                <th className="py-1.5 text-left pl-3">Rule changes</th>
                <th className="py-1.5 text-right">Network</th>
                <th className="py-1.5 text-right">LLM</th>
              </tr>
            </thead>
            <tbody>
              {(info.builds ?? []).map(b => (
                <tr key={b.id} className="border-b border-zinc-800/60 align-top">
                  <td className="py-1.5" title={b.created_at}>{relTime(b.created_at)}</td>
                  <td className="py-1.5"><Badge value={b.status} /></td>
                  <td className="py-1.5 font-mono text-zinc-400">{b.content_hash ? b.content_hash.slice(0, 12) : '—'}</td>
                  <td className="py-1.5 text-right tabular-nums">{b.rule_count}</td>
                  <td className="py-1.5 pl-3 text-zinc-400">
                    {b.error ? <span className="text-red-400 select-text">{b.error}</span> : changesSummary(b.changes)}
                  </td>
                  <td className="py-1.5 text-right tabular-nums">{b.network_calls}</td>
                  <td className="py-1.5 text-right tabular-nums">{b.llm_calls}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </div>
  );
};
