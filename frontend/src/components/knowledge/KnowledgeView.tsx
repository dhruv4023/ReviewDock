import React, { useCallback, useEffect, useState } from 'react';
import { Loader2, Play, X } from 'lucide-react';
import { useKnowledgeStore } from '../../stores/knowledgeStore';
import { OverviewTab } from './OverviewTab';
import { CommentsTab } from './CommentsTab';
import { RulesTab } from './RulesTab';
import { SyncTab } from './SyncTab';
import { SkillTab } from './SkillTab';
import { Btn } from './ui';

type Tab = 'overview' | 'comments' | 'rules' | 'sync' | 'skill';
const TABS: { id: Tab; label: string }[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'comments', label: 'Comments' },
  { id: 'rules', label: 'Rules' },
  { id: 'sync', label: 'Sync' },
  { id: 'skill', label: 'Skill' },
];

const LS_TAB = 'knowledge:tab';

export const KnowledgeView: React.FC = () => {
  const { init, running, events, error, setError, runSync } = useKnowledgeStore();
  const [tab, setTab] = useState<Tab>(() => {
    try { return (localStorage.getItem(LS_TAB) as Tab) || 'overview'; } catch { return 'overview'; }
  });
  const [focusRule, setFocusRule] = useState<number | null>(null);

  useEffect(() => { init(); }, [init]);
  useEffect(() => { try { localStorage.setItem(LS_TAB, tab); } catch { /* ignore */ } }, [tab]);

  const openRule = useCallback((id: number) => { setFocusRule(id); setTab('rules'); }, []);
  const clearFocus = useCallback(() => setFocusRule(null), []);
  const last = events[events.length - 1];

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex items-center border-b border-zinc-800 bg-[#0d1117] shrink-0 pr-2">
        {TABS.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-3 py-1.5 text-[11px] font-semibold border-b-2 transition ${tab === t.id ? 'border-emerald-500 text-emerald-400' : 'border-transparent text-zinc-500 hover:text-gray-300'}`}
          >
            {t.label}
          </button>
        ))}
        <div className="ml-auto flex items-center gap-2 min-w-0">
          {running && last && (
            <span className="flex items-center gap-1.5 text-[10px] text-blue-300 truncate max-w-[420px]" title={last.message}>
              <Loader2 size={11} className="animate-spin shrink-0" /> {last.message}
            </span>
          )}
          <Btn variant="primary" busy={running} onClick={() => runSync()}><Play size={11} /> Sync</Btn>
        </div>
      </div>

      {error && (
        <div className="flex items-start gap-2 px-3 py-1.5 text-[11px] text-red-300 bg-red-950/30 border-b border-red-900/50 select-text">
          <span className="flex-1 break-words">{error}</span>
          <button onClick={() => setError(null)} className="text-red-400 hover:text-red-200"><X size={12} /></button>
        </div>
      )}

      <div className="flex-1 min-h-0">
        {tab === 'overview' && <OverviewTab onOpenRule={openRule} />}
        {tab === 'comments' && <CommentsTab onOpenRule={openRule} />}
        {tab === 'rules' && <RulesTab focusRuleId={focusRule} onFocusHandled={clearFocus} />}
        {tab === 'sync' && <SyncTab />}
        {tab === 'skill' && <SkillTab />}
      </div>
    </div>
  );
};
