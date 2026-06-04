import React, { useState, useMemo } from 'react';
import { useAppStore } from '../stores/appStore';
import { RefreshCw, Play, Search, Filter, AlertTriangle, CheckCircle, HelpCircle, Wifi } from 'lucide-react';
import { RemoteSetupModal } from './RemoteSetupModal';

import { PullRequest } from '../stores/appStore';

interface PRTableProps {
  onRowClick: (pr: PullRequest) => void;
}

export const PRTable: React.FC<PRTableProps> = ({ onRowClick }) => {
  const { 
    prs, 
    fetchPRs, 
    isLoadingPRs, 
    selectedPRIds, 
    toggleSelectPR, 
    selectAllPRs, 
    deselectAllPRs,
    selectedPR,
    rebaseSelected,
    settings,
    setSettings,
    pendingRemoteSetup,
    _processNextRemoteSetup,
  } = useAppStore();

  const [stateFilter, setStateFilter] = useState<'open' | 'draft' | 'closed' | 'all'>('all');
  const [excludeDrafts, setExcludeDrafts] = useState(false);
  const [repoFilter, setRepoFilter] = useState<string>('all');
  const [search, setSearch] = useState('');
  const [noTrackingOnly, setNoTrackingOnly] = useState(false);

  // Auto-derived repo list for filter selector
  const repositoriesList = useMemo(() => {
    const names = new Set((prs || []).map(p => p.repo_name));
    return Array.from(names);
  }, [prs]);

  // Apply filters
  const filteredPRs = useMemo(() => {
    return (prs || []).filter(pr => {
      // 1. Search text (matches branch names or title)
      const matchesSearch = 
        pr.title.toLowerCase().includes(search.toLowerCase()) || 
        (pr.head_label || pr.head_branch).toLowerCase().includes(search.toLowerCase()) ||
        (pr.base_label || pr.base_branch).toLowerCase().includes(search.toLowerCase());
      
      // 2. Repo filter
      const matchesRepo = repoFilter === 'all' || pr.repo_name === repoFilter;

      // 3. Draft filter
      if (excludeDrafts && pr.is_draft) {
        return false;
      }

      // 4. State filter
      let matchesState = true;
      if (stateFilter === 'open') {
        matchesState = pr.state === 'open' && !pr.is_draft;
      } else if (stateFilter === 'draft') {
        matchesState = pr.is_draft;
      } else if (stateFilter === 'closed') {
        matchesState = pr.state === 'closed';
      }

      // 5. No-tracking filter
      if (noTrackingOnly && pr.head_label?.includes('/')) {
        return false;
      }

      return matchesSearch && matchesRepo && matchesState;
    });
  }, [prs, search, repoFilter, stateFilter, excludeDrafts, noTrackingOnly]);

  const allSelected = useMemo(() => {
    if (filteredPRs.length === 0) return false;
    return filteredPRs.every(pr => selectedPRIds.includes(pr.id));
  }, [filteredPRs, selectedPRIds]);

  const handleSelectAllToggle = () => {
    if (allSelected) {
      deselectAllPRs();
    } else {
      selectAllPRs(filteredPRs);
    }
  };

  const handleBulkRebase = async (amend: boolean, push: boolean) => {
    if (!settings) return;
    
    // Save temporary rebase configurations
    const updatedSettings = {
      ...settings,
      amend_commit_timestamp: amend,
      force_push_after_rebase: push
    };
    await setSettings(updatedSettings);

    await rebaseSelected();
    deselectAllPRs();
  };

  // Opens the remote-setup modal for all untracked PRs in the same repository.
  // submitAfterSetup=false: only sets tracking, does NOT trigger a rebase/push job.
  const handleSetRemote = async (e: React.MouseEvent, pr: import('../stores/appStore').PullRequest) => {
    e.stopPropagation();
    // Collect all PRs in the same repo that also lack a remote, starting with the clicked one.
    const sameRepoUntracked = prs
      .filter(p => p.repo_id === pr.repo_id && !p.head_label?.includes('/'))
      .sort((a, b) => (a.id === pr.id ? -1 : b.id === pr.id ? 1 : 0)); // clicked PR first
    await _processNextRemoteSetup(sameRepoUntracked.map(p => ({
      id: p.id,
      repo_id: p.repo_id,
      head_label: p.head_label || p.head_branch,
      base_label: p.base_label || p.base_branch,
    })), false); // false = tracking only, no rebase after
  };

  return (
    <>
    <div className="flex-1 flex flex-col h-full bg-[#0d1117] overflow-hidden">
      {/* Top action bar */}
      <div className="p-3 border-b border-zinc-800 bg-[#161b22] flex flex-wrap gap-3 items-center justify-between">
        <div className="flex gap-2 items-center">
          <button
            onClick={() => handleBulkRebase(true, false)}
            disabled={selectedPRIds.length === 0}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded text-xs font-semibold action-btn"
          >
            <Play size={13} /> Rebase Selected
          </button>
          <button
            onClick={() => handleBulkRebase(true, true)}
            disabled={selectedPRIds.length === 0}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-[#238636] hover:bg-[#2ea043] disabled:opacity-50 text-white rounded text-xs font-semibold action-btn"
          >
            <Play size={13} /> Rebase + Force Push
          </button>
          <button
            onClick={() => handleBulkRebase(false, true)}
            disabled={selectedPRIds.length === 0}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 disabled:opacity-50 text-zinc-300 rounded text-xs font-semibold action-btn"
          >
            Force Push Only
          </button>
          {selectedPRIds.length > 0 && (
            <span className="text-xs text-gray-400 font-medium ml-2">
              {selectedPRIds.length} Selected
            </span>
          )}
        </div>

        <button
          onClick={fetchPRs}
          className="flex items-center gap-1.5 px-2.5 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-gray-300 rounded text-xs font-medium action-btn"
        >
          <RefreshCw size={13} className={isLoadingPRs ? 'animate-spin' : ''} /> Refresh PRs
        </button>
      </div>

      {/* Filter controls */}
      <div className="p-3 border-b border-zinc-800 bg-[#161b22]/50 flex flex-wrap gap-3 items-center">
        <div className="flex items-center gap-2">
          <Search size={14} className="text-gray-500" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search branches or titles..."
            className="bg-[#0d1117] border border-zinc-800 rounded px-2.5 py-1 text-xs text-gray-200 focus:outline-none focus:border-zinc-700 w-48"
          />
        </div>

        <div className="flex items-center gap-2">
          <Filter size={14} className="text-gray-500" />
          <select
            value={repoFilter}
            onChange={(e) => setRepoFilter(e.target.value)}
            className="bg-[#0d1117] border border-zinc-800 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none focus:border-zinc-750"
          >
            <option value="all">All Repositories</option>
            {repositoriesList.map(repo => (
              <option key={repo} value={repo}>{repo}</option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-1.5 border border-zinc-800 rounded p-0.5 bg-[#0d1117]">
          {(['open', 'draft', 'closed', 'all'] as const).map(state => (
            <button
              key={state}
              onClick={() => setStateFilter(state)}
              className={`px-2 py-0.5 rounded text-[11px] font-medium capitalize transition ${
                stateFilter === state 
                  ? 'bg-zinc-800 text-white' 
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              {state}
            </button>
          ))}
        </div>

        <label className="flex items-center gap-2 text-xs text-gray-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={excludeDrafts}
            onChange={(e) => setExcludeDrafts(e.target.checked)}
            className="accent-blue-600 rounded"
          />
          Exclude Draft PRs
        </label>

        <label className="flex items-center gap-2 text-xs cursor-pointer select-none">
          <input
            type="checkbox"
            checked={noTrackingOnly}
            onChange={(e) => setNoTrackingOnly(e.target.checked)}
            className="accent-amber-500 rounded"
          />
          <span className={noTrackingOnly ? 'text-amber-400 font-medium' : 'text-gray-400'}>
            No Tracking Only
          </span>
        </label>
      </div>

      {/* Main Table view */}
      <div className="flex-1 overflow-auto">
        {isLoadingPRs ? (
          <div className="h-full flex flex-col items-center justify-center gap-2 text-gray-500">
            <RefreshCw size={24} className="animate-spin" />
            <span className="text-xs">Fetching pull request data from GitHub...</span>
          </div>
        ) : filteredPRs.length === 0 ? (
          <div className="h-full flex items-center justify-center text-xs text-gray-500">
            No matching pull requests found
          </div>
        ) : (
          <table className="w-full text-left text-xs border-collapse">
            <thead>
              <tr className="border-b border-zinc-850 text-gray-400 bg-[#161b22]/30 sticky top-0 z-10 font-semibold select-none">
                <th className="py-2.5 px-3 w-8">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={handleSelectAllToggle}
                    className="accent-blue-600 rounded"
                  />
                </th>
                <th className="py-2.5 px-2 w-16">PR</th>
                <th className="py-2.5 px-2">Title</th>
                <th className="py-2.5 px-2 w-32">Repository</th>
                <th className="py-2.5 px-2 w-28">Base Branch</th>
                <th className="py-2.5 px-2 w-28">Head Branch</th>
                <th className="py-2.5 px-2 w-20">Merge Status</th>
                <th className="py-2.5 px-2 w-16 text-center">Remote Sync</th>
                <th className="py-2.5 px-2 w-24 text-center">Local Sync</th>
                <th className="py-2.5 px-3 w-28 text-right">Updated</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-850">
              {filteredPRs.map((pr) => {
                const isSelected = selectedPRIds.includes(pr.id);
                const isCurrent = selectedPR?.id === pr.id;
                
                return (
                  <tr
                    key={pr.id}
                    onClick={() => onRowClick(pr)}
                    className={`hover:bg-zinc-800/40 transition cursor-pointer select-none ${
                      isCurrent ? 'bg-zinc-800/50 border-l-2 border-blue-500' : ''
                    } ${isSelected ? 'bg-blue-900/10' : ''}`}
                  >
                    <td className="py-2.5 px-3 w-8" onClick={(e) => e.stopPropagation()}>
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => toggleSelectPR(pr.id)}
                        className="accent-blue-600 rounded"
                      />
                    </td>
                    <td className="py-2.5 px-2 text-gray-500">#{pr.number}</td>
                    <td className="py-2.5 px-2 font-medium text-gray-200 truncate max-w-xs" title={pr.title}>
                      {pr.title}
                    </td>
                    <td className="py-2.5 px-2 text-zinc-400 truncate max-w-[120px]" title={pr.repo_name}>
                      {pr.repo_name}
                    </td>
                    <td className="py-2.5 px-2 text-gray-400 truncate max-w-[100px]" title={pr.base_label || pr.base_branch}>
                      <span className="bg-zinc-800/80 px-1.5 py-0.5 rounded border border-zinc-700 text-[10px]">
                        {pr.base_label || pr.base_branch}
                      </span>
                    </td>
                    <td className="py-2.5 px-2 text-gray-450 truncate max-w-[100px]" title={pr.head_label || pr.head_branch}>
                      <span className="inline-flex items-center gap-1">
                        {!pr.head_label?.includes('/') && (
                          <button
                            title="No remote tracking — click to set"
                            onClick={(e) => handleSetRemote(e, pr)}
                            className="text-amber-400 hover:text-amber-300 hover:scale-110 transition-transform cursor-pointer"
                          >
                            <Wifi size={11} />
                          </button>
                        )}
                        <span className="bg-zinc-800/80 px-1.5 py-0.5 rounded border border-zinc-700 text-[10px] text-zinc-300">
                          {pr.head_label || pr.head_branch}
                        </span>
                      </span>
                    </td>
                    <td className="py-2.5 px-2">
                      {pr.mergeable_status === 'mergeable' ? (
                        <span className="flex items-center gap-1 text-green-500 font-medium text-[10px]">
                          <CheckCircle size={12} /> Clean
                        </span>
                      ) : pr.mergeable_status === 'conflicting' ? (
                        <span className="flex items-center gap-1 text-red-400 font-medium text-[10px]">
                          <AlertTriangle size={12} /> Conflict
                        </span>
                      ) : (
                        <span className="flex items-center gap-1 text-zinc-500 font-medium text-[10px]">
                          <HelpCircle size={12} /> Unknown
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 px-2 text-center">
                      <span className={`inline-flex items-center gap-1 font-mono text-[10px] ${
                        (pr.ahead_count > 0 || pr.behind_count > 0)
                          ? 'text-amber-400'
                          : 'text-zinc-600'
                      }`}>
                        <span title="Local commits not on remote">↑{pr.ahead_count}</span>
                        <span title="Remote commits not in local">↓{pr.behind_count}</span>
                      </span>
                    </td>
                    <td className="py-2.5 px-2 text-center">
                      <span className={`inline-flex items-center gap-1 font-mono text-[10px] ${
                        (pr.local_ahead_count > 0 || pr.local_behind_count > 0)
                          ? 'text-amber-400'
                          : 'text-zinc-600'
                      }`}>
                        <span title="Local commits not on remote">↑{pr.local_ahead_count}</span>
                        <span title="Remote commits not in local">↓{pr.local_behind_count}</span>
                      </span>
                    </td>
                    <td className="py-2.5 px-3 text-right text-gray-500 text-[10px]">
                      {new Date(pr.updated_at).toLocaleDateString()}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>

    {/* Remote setup modal — shown when a selected PR's head branch lacks tracking */}
    {pendingRemoteSetup && <RemoteSetupModal setup={pendingRemoteSetup} />}
  </>);
};
