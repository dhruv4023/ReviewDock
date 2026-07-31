import React, { useState, useEffect } from 'react';
import { X, Loader2, AlertTriangle } from 'lucide-react';

export interface AbortableJob {
  id: string;
  prTitle: string;
  repoName: string;
  status: 'queued' | 'running';
}

interface AbortJobsDialogProps {
  jobs: AbortableJob[];
  onConfirm: (ids: string[]) => void;
  onClose: () => void;
}

export const AbortJobsDialog: React.FC<AbortJobsDialogProps> = ({ jobs, onConfirm, onClose }) => {
  // Pre-select all jobs
  const [selected, setSelected] = useState<Set<string>>(() => new Set(jobs.map(j => j.id)));

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  // Keep selection in sync if a job finishes/disappears while the dialog is open
  useEffect(() => {
    const liveIds = new Set(jobs.map(j => j.id));
    setSelected(prev => new Set([...prev].filter(id => liveIds.has(id))));
  }, [jobs]);

  const toggle = (id: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const toggleAll = () => {
    if (selected.size === jobs.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(jobs.map(j => j.id)));
    }
  };

  const handleConfirm = () => {
    onConfirm(Array.from(selected));
  };

  const allSelected = selected.size === jobs.length;
  const noneSelected = selected.size === 0;

  return (
    /* Backdrop */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="relative w-full max-w-md bg-[#161b22] border border-zinc-700/80 rounded-xl shadow-2xl overflow-hidden">

        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-800">
          <div className="flex items-center gap-2.5">
            <div className="p-1.5 rounded-md bg-red-950/40 border border-red-800/60 text-red-400">
              <AlertTriangle size={14} />
            </div>
            <div>
              <h2 className="text-sm font-bold text-gray-100 leading-tight">Abort Jobs</h2>
              <p className="text-[10px] text-zinc-500 mt-0.5">Select which jobs to cancel</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded hover:bg-zinc-800 text-zinc-500 hover:text-gray-300 transition cursor-pointer"
          >
            <X size={14} />
          </button>
        </div>

        {/* Select-all row */}
        <div className="px-5 py-2.5 border-b border-zinc-800/60 bg-[#0d1117]/40">
          <label className="flex items-center gap-2.5 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={allSelected}
              ref={el => { if (el) el.indeterminate = !allSelected && !noneSelected; }}
              onChange={toggleAll}
              className="accent-red-500 rounded cursor-pointer"
            />
            <span className="text-xs text-zinc-400 font-medium">
              {allSelected ? 'Deselect all' : 'Select all'} ({jobs.length} job{jobs.length !== 1 ? 's' : ''})
            </span>
          </label>
        </div>

        {/* Job list */}
        <div className="max-h-64 overflow-y-auto divide-y divide-zinc-800/60">
          {jobs.map(job => {
            const isChecked = selected.has(job.id);
            return (
              <label
                key={job.id}
                className={`flex items-center gap-3 px-5 py-3 cursor-pointer select-none transition-colors ${
                  isChecked ? 'bg-red-950/10 hover:bg-red-950/20' : 'hover:bg-zinc-800/30'
                }`}
              >
                <input
                  type="checkbox"
                  checked={isChecked}
                  onChange={() => toggle(job.id)}
                  className="accent-red-500 rounded cursor-pointer shrink-0"
                />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 truncate">
                    {/* Status badge */}
                    {job.status === 'running' ? (
                      <span className="inline-flex items-center gap-1 bg-blue-950/40 text-blue-400 px-1.5 py-0.5 rounded border border-blue-900/60 text-[9px] font-semibold animate-pulse shrink-0">
                        <Loader2 size={9} className="animate-spin" /> Running
                      </span>
                    ) : (
                      <span className="inline-flex items-center bg-zinc-800 text-zinc-400 px-1.5 py-0.5 rounded border border-zinc-700 text-[9px] font-semibold animate-pulse shrink-0">
                        Queued
                      </span>
                    )}
                    <span className="text-xs text-gray-200 truncate font-medium">{job.prTitle}</span>
                  </div>
                  <span className="text-[10px] text-zinc-500 mt-0.5 block truncate">{job.repoName}</span>
                </div>
              </label>
            );
          })}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2.5 px-5 py-4 border-t border-zinc-800 bg-[#0d1117]/30">
          <button
            onClick={onClose}
            className="px-3.5 py-1.5 rounded text-xs font-medium text-zinc-400 hover:text-gray-200 hover:bg-zinc-800 transition cursor-pointer"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={noneSelected}
            className="flex items-center gap-1.5 px-4 py-1.5 bg-red-700 hover:bg-red-600 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded text-xs font-semibold transition cursor-pointer"
          >
            <X size={12} />
            Abort {selected.size} Job{selected.size !== 1 ? 's' : ''}
          </button>
        </div>
      </div>
    </div>
  );
};
