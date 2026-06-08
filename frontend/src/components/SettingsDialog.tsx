import React, { useState, useEffect, useCallback } from 'react';
import { useAppStore } from '../stores/appStore';
import { X, Save, RotateCcw, Settings, Bot, SlidersHorizontal, Check, AlertTriangle } from 'lucide-react';

const DEFAULT_TEMPLATE = `You are an expert code reviewer. Please review the following pull request carefully.

## Instructions
- Identify bugs, security issues, and logic errors.
- Suggest improvements to code clarity and maintainability.
- Check for missing tests or documentation.
- Be concise and prioritize critical issues.
- Note any positive aspects of the implementation.

## PR Title
{{PR_TITLE}}

## PR Description
{{PR_DESCRIPTION}}

## Code Changes
{{PR_DIFF}}`;

interface SettingsDialogProps {
  onClose: () => void;
}

type Tab = 'general' | 'template';

export const SettingsDialog: React.FC<SettingsDialogProps> = ({ onClose }) => {
  const { settings, setSettings, reviewTemplate, saveReviewTemplate } = useAppStore();

  const [activeTab, setActiveTab] = useState<Tab>('template');

  // General settings state
  const [concurrency, setConcurrency] = useState<number>(settings?.concurrency_limit ?? 3);
  const [amendTimestamp, setAmendTimestamp] = useState<boolean>(settings?.amend_commit_timestamp ?? true);
  const [forcePush, setForcePush] = useState<boolean>(settings?.force_push_after_rebase ?? false);

  // Template state
  const [template, setTemplate] = useState<string>('');
  const [templateSaved, setTemplateSaved] = useState(false);
  const [templateError, setTemplateError] = useState<string | null>(null);

  // General settings saved state
  const [generalSaved, setGeneralSaved] = useState(false);

  // Initialise template from store when dialog opens
  useEffect(() => {
    setTemplate(reviewTemplate || DEFAULT_TEMPLATE);
  }, [reviewTemplate]);

  // Reset form if settings change from outside
  useEffect(() => {
    if (settings) {
      setConcurrency(settings.concurrency_limit);
      setAmendTimestamp(settings.amend_commit_timestamp);
      setForcePush(settings.force_push_after_rebase);
    }
  }, [settings]);

  const handleSaveGeneral = useCallback(async () => {
    if (!settings) return;
    const updated = {
      ...settings,
      concurrency_limit: concurrency,
      amend_commit_timestamp: amendTimestamp,
      force_push_after_rebase: forcePush,
    };
    await setSettings(updated);
    setGeneralSaved(true);
    setTimeout(() => setGeneralSaved(false), 2000);
  }, [settings, concurrency, amendTimestamp, forcePush, setSettings]);

  const handleSaveTemplate = useCallback(async () => {
    setTemplateError(null);
    try {
      await saveReviewTemplate(template);
      setTemplateSaved(true);
      setTimeout(() => setTemplateSaved(false), 2000);
    } catch {
      setTemplateError('Failed to save template. Please try again.');
    }
  }, [template, saveReviewTemplate]);

  const handleResetTemplate = useCallback(() => {
    setTemplate(DEFAULT_TEMPLATE);
  }, []);

  // Close on backdrop click
  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) onClose();
  };

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
      onClick={handleBackdropClick}
    >
      <div className="bg-[#161b22] border border-zinc-800 rounded-xl w-full max-w-2xl shadow-2xl flex flex-col overflow-hidden"
           style={{ maxHeight: 'calc(100vh - 48px)' }}>

        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-800 bg-[#0d1117]/40 shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="p-1.5 bg-zinc-800 rounded text-blue-400">
              <Settings size={15} />
            </div>
            <h2 className="text-sm font-bold text-gray-100 tracking-tight">Settings</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-zinc-800 text-zinc-500 hover:text-white rounded transition"
            title="Close"
          >
            <X size={15} />
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-zinc-800 shrink-0">
          <button
            onClick={() => setActiveTab('template')}
            className={`flex items-center gap-1.5 px-5 py-2.5 text-xs font-medium transition border-b-2 -mb-px ${
              activeTab === 'template'
                ? 'border-blue-500 text-blue-400'
                : 'border-transparent text-zinc-500 hover:text-zinc-300'
            }`}
          >
            <Bot size={13} />
            AI Review Template
          </button>
          <button
            onClick={() => setActiveTab('general')}
            className={`flex items-center gap-1.5 px-5 py-2.5 text-xs font-medium transition border-b-2 -mb-px ${
              activeTab === 'general'
                ? 'border-blue-500 text-blue-400'
                : 'border-transparent text-zinc-500 hover:text-zinc-300'
            }`}
          >
            <SlidersHorizontal size={13} />
            General
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto">

          {/* ── AI Review Template Tab ── */}
          {activeTab === 'template' && (
            <div className="p-5 space-y-4">
              <div>
                <p className="text-xs text-zinc-400 leading-relaxed mb-1">
                  Customise the prompt template used when you click <strong className="text-zinc-200">"Copy AI Review Prompt"</strong> on a PR.
                  Use the placeholders below — they will be replaced automatically.
                </p>
                <div className="flex flex-wrap gap-2 mt-2">
                  {['{{PR_TITLE}}', '{{PR_DESCRIPTION}}', '{{PR_DIFF}}'].map(ph => (
                    <span
                      key={ph}
                      className="px-2 py-0.5 bg-zinc-800 border border-zinc-700 rounded font-mono text-[11px] text-blue-300 select-all"
                    >
                      {ph}
                    </span>
                  ))}
                </div>
              </div>

              <div className="relative">
                <textarea
                  id="review-template-textarea"
                  value={template}
                  onChange={e => setTemplate(e.target.value)}
                  className="w-full h-80 bg-[#0d1117] border border-zinc-700 focus:border-blue-500 rounded-lg p-3 text-xs font-mono text-zinc-200 resize-none outline-none transition leading-relaxed placeholder:text-zinc-600"
                  placeholder={DEFAULT_TEMPLATE}
                  spellCheck={false}
                />
              </div>

              {templateError && (
                <div className="flex items-start gap-2 p-2.5 bg-red-950/20 border border-red-800 rounded text-red-400 text-xs">
                  <AlertTriangle size={13} className="shrink-0 mt-0.5" />
                  <span>{templateError}</span>
                </div>
              )}

              <div className="flex items-center justify-between gap-3 pt-1">
                <button
                  onClick={handleResetTemplate}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-zinc-400 hover:text-zinc-200 rounded text-xs transition"
                >
                  <RotateCcw size={12} />
                  Reset to Default
                </button>
                <button
                  onClick={handleSaveTemplate}
                  className={`flex items-center gap-1.5 px-4 py-1.5 rounded text-xs font-semibold transition ${
                    templateSaved
                      ? 'bg-green-700 text-white'
                      : 'bg-blue-600 hover:bg-blue-500 text-white'
                  }`}
                >
                  {templateSaved ? (
                    <><Check size={13} /> Saved!</>
                  ) : (
                    <><Save size={13} /> Save Template</>
                  )}
                </button>
              </div>
            </div>
          )}

          {/* ── General Settings Tab ── */}
          {activeTab === 'general' && (
            <div className="p-5 space-y-5">

              {/* Concurrency */}
              <div className="space-y-1.5">
                <label htmlFor="concurrency-input" className="text-xs font-medium text-zinc-300">
                  Concurrency Limit
                </label>
                <p className="text-[11px] text-zinc-500">Maximum number of rebase jobs running in parallel.</p>
                <input
                  id="concurrency-input"
                  type="number"
                  min={1}
                  max={20}
                  value={concurrency}
                  onChange={e => setConcurrency(Number(e.target.value))}
                  className="w-24 bg-[#0d1117] border border-zinc-700 focus:border-blue-500 rounded px-2.5 py-1.5 text-xs text-zinc-200 outline-none transition"
                />
              </div>

              <div className="w-full h-px bg-zinc-800" />

              {/* Toggles */}
              {/* <div className="space-y-3">
                <ToggleRow
                  id="amend-timestamp-toggle"
                  label="Amend Commit Timestamp"
                  description="Update the commit timestamp when rebasing, keeping history clean."
                  checked={amendTimestamp}
                  onChange={setAmendTimestamp}
                />
                <ToggleRow
                  id="force-push-toggle"
                  label="Force Push After Rebase"
                  description="Automatically force-push the branch after a successful rebase."
                  checked={forcePush}
                  onChange={setForcePush}
                />
              </div> */}

              <div className="flex justify-end pt-1">
                <button
                  onClick={handleSaveGeneral}
                  className={`flex items-center gap-1.5 px-4 py-1.5 rounded text-xs font-semibold transition ${
                    generalSaved
                      ? 'bg-green-700 text-white'
                      : 'bg-blue-600 hover:bg-blue-500 text-white'
                  }`}
                >
                  {generalSaved ? (
                    <><Check size={13} /> Saved!</>
                  ) : (
                    <><Save size={13} /> Save Settings</>
                  )}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

// ── Helper sub-component ─────────────────────────────────────────────────────

// interface ToggleRowProps {
//   id: string;
//   label: string;
//   description: string;
//   checked: boolean;
//   onChange: (v: boolean) => void;
// }

// const ToggleRow: React.FC<ToggleRowProps> = ({ id, label, description, checked, onChange }) => (
//   <div className="flex items-start justify-between gap-4">
//     <div>
//       <label htmlFor={id} className="text-xs font-medium text-zinc-300 cursor-pointer">{label}</label>
//       <p className="text-[11px] text-zinc-500 mt-0.5">{description}</p>
//     </div>
//     <button
//       id={id}
//       role="switch"
//       aria-checked={checked}
//       onClick={() => onChange(!checked)}
//       className={`relative shrink-0 w-9 h-5 rounded-full transition-colors duration-200 focus:outline-none mt-0.5 ${
//         checked ? 'bg-blue-600' : 'bg-zinc-700'
//       }`}
//     >
//       <span
//         className={`absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow transition-transform duration-200 ${
//           checked ? 'translate-x-4' : 'translate-x-0'
//         }`}
//       />
//     </button>
//   </div>
// );
