import React from 'react';
import { Loader2 } from 'lucide-react';

export function relTime(iso?: string): string {
  if (!iso) return '—';
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const diff = Date.now() - t;
  if (diff < 60_000) return 'just now';
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  if (diff < 30 * 86_400_000) return `${Math.round(diff / 86_400_000)}d ago`;
  return new Date(iso).toLocaleDateString();
}

export function shortDate(iso?: string): string {
  return iso ? iso.slice(0, 10) : '—';
}

export function openURL(url: string) {
  if (!url) return;
  if (window.runtime?.BrowserOpenURL) window.runtime.BrowserOpenURL(url);
  else window.open(url, '_blank', 'noopener');
}

export function copyText(text: string) {
  if (window.runtime?.ClipboardSetText) window.runtime.ClipboardSetText(text);
  else navigator.clipboard?.writeText(text);
}

const STATUS_STYLES: Record<string, string> = {
  ok: 'bg-green-950/40 text-green-400 border-green-800/50',
  approved: 'bg-green-950/40 text-green-400 border-green-800/50',
  published: 'bg-green-950/40 text-green-400 border-green-800/50',
  requirement: 'bg-blue-950/40 text-blue-300 border-blue-800/50',
  pending: 'bg-amber-950/30 text-amber-400 border-amber-800/50',
  running: 'bg-blue-950/40 text-blue-300 border-blue-800/50',
  suggestion: 'bg-indigo-950/40 text-indigo-300 border-indigo-800/50',
  partial: 'bg-amber-950/30 text-amber-400 border-amber-800/50',
  rate_limited: 'bg-amber-950/30 text-amber-400 border-amber-800/50',
  unchanged: 'bg-zinc-800/60 text-zinc-400 border-zinc-700/60',
  'dry-run': 'bg-zinc-800/60 text-zinc-400 border-zinc-700/60',
  error: 'bg-red-950/40 text-red-400 border-red-800/50',
  failed: 'bg-red-950/40 text-red-400 border-red-800/50',
  rejected: 'bg-red-950/30 text-red-400 border-red-900/50',
  interrupted: 'bg-orange-950/30 text-orange-400 border-orange-800/50',
  superseded: 'bg-zinc-800/60 text-zinc-500 border-zinc-700/60 line-through',
  merged: 'bg-violet-950/30 text-violet-300 border-violet-800/50',
  high: 'bg-red-950/30 text-red-400 border-red-900/50',
  medium: 'bg-amber-950/30 text-amber-400 border-amber-800/50',
  low: 'bg-zinc-800/60 text-zinc-400 border-zinc-700/60',
};

export function Badge({ value, title }: { value: string; title?: string }) {
  const cls = STATUS_STYLES[value] ?? 'bg-zinc-800/60 text-zinc-400 border-zinc-700/60';
  return (
    <span title={title} className={`inline-flex items-center px-1.5 py-px rounded border text-[10px] font-semibold whitespace-nowrap ${cls}`}>
      {value.replace('_', ' ')}
    </span>
  );
}

type BtnVariant = 'primary' | 'default' | 'danger' | 'ghost';
const BTN: Record<BtnVariant, string> = {
  primary: 'bg-blue-600 hover:bg-blue-500 text-white border-blue-500',
  default: 'bg-zinc-800 hover:bg-zinc-700 text-gray-200 border-zinc-700',
  danger: 'bg-red-950/40 hover:bg-red-900/50 text-red-300 border-red-800/60',
  ghost: 'bg-transparent hover:bg-zinc-800 text-zinc-400 hover:text-gray-200 border-transparent',
};

export function Btn({ children, onClick, variant = 'default', disabled, busy, title, type = 'button' }: {
  children: React.ReactNode; onClick?: () => void; variant?: BtnVariant; disabled?: boolean; busy?: boolean;
  title?: string; type?: 'button' | 'submit';
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled || busy}
      title={title}
      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded border text-[11px] font-semibold transition disabled:opacity-40 disabled:cursor-not-allowed ${BTN[variant]}`}
    >
      {busy && <Loader2 size={11} className="animate-spin" />}
      {children}
    </button>
  );
}

export const inputCls =
  'bg-[#0d1117] border border-zinc-700 focus:border-blue-500 rounded px-2 py-1 text-[11px] text-gray-200 outline-none transition placeholder:text-zinc-600';

export function Select({ value, onChange, children, className = '' }: {
  value: string; onChange: (v: string) => void; children: React.ReactNode; className?: string;
}) {
  return (
    <select value={value} onChange={e => onChange(e.target.value)} className={`${inputCls} ${className}`}>
      {children}
    </select>
  );
}

export function Panel({ title, actions, children, className = '' }: {
  title?: React.ReactNode; actions?: React.ReactNode; children: React.ReactNode; className?: string;
}) {
  return (
    <div className={`bg-[#161b22] border border-zinc-800 rounded-lg ${className}`}>
      {(title || actions) && (
        <div className="flex items-center justify-between gap-2 px-3 py-2 border-b border-zinc-800">
          <h3 className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">{title}</h3>
          <div className="flex items-center gap-1.5">{actions}</div>
        </div>
      )}
      <div className="p-3">{children}</div>
    </div>
  );
}

export function Stat({ label, value, sub }: { label: string; value: React.ReactNode; sub?: React.ReactNode }) {
  return (
    <div className="bg-[#161b22] border border-zinc-800 rounded-lg px-3 py-2.5">
      <div className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500">{label}</div>
      <div className="text-xl font-black text-gray-100 leading-tight mt-0.5">{value}</div>
      {sub && <div className="text-[10px] text-zinc-500 mt-0.5">{sub}</div>}
    </div>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <div className="text-[11px] text-zinc-500 py-6 text-center">{children}</div>;
}

export function Field({ label, hint, children, className = '' }: { label: string; hint?: string; children: React.ReactNode; className?: string }) {
  return (
    <label className={`flex flex-col gap-1 ${className}`}>
      <span className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500">{label}</span>
      {children}
      {hint && <span className="text-[10px] text-zinc-600 leading-snug">{hint}</span>}
    </label>
  );
}

/** Colourises a GitHub diff hunk. */
export function DiffHunk({ text }: { text: string }) {
  if (!text) return null;
  return (
    <pre className="bg-[#0b0e14] border border-zinc-800 rounded text-[10.5px] font-mono overflow-x-auto py-1 select-text">
      {text.split('\n').map((l, i) => (
        <div
          key={i}
          className={`px-2 whitespace-pre ${l.startsWith('+') ? 'text-green-400 bg-green-950/20' : l.startsWith('-') ? 'text-red-400 bg-red-950/20' : l.startsWith('@@') ? 'text-blue-400' : 'text-zinc-400'}`}
        >
          {l || ' '}
        </div>
      ))}
    </pre>
  );
}
