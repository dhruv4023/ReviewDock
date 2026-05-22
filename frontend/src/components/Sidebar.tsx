import React, { useState } from 'react';
import { useAppStore } from '../stores/appStore';
import { FolderPlus, Trash2, GitBranch, RefreshCw, AlertCircle } from 'lucide-react';

export const Sidebar: React.FC = () => {
  const { repos, addRepo, removeRepo, fetchRepos, isLoadingRepos } = useAppStore();
  const [newPath, setNewPath] = useState('');
  const [isAdding, setIsAdding] = useState(false);
  const [search, setSearch] = useState('');

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPath.trim()) return;
    try {
      await addRepo(newPath.trim());
      setNewPath('');
      setIsAdding(false);
    } catch (err) {
      // alert inside store handles errors
    }
  };

  const filteredRepos = repos.filter(r => 
    r.name.toLowerCase().includes(search.toLowerCase()) || 
    r.owner.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <aside className="w-64 bg-[#161b22] border-r border-zinc-800 flex flex-col h-full">
      <div className="p-4 border-b border-zinc-800 flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wider text-gray-400 uppercase flex items-center gap-2">
          <GitBranch size={16} /> Repositories
        </h2>
        <button
          onClick={() => setIsAdding(!isAdding)}
          className="p-1 hover:bg-zinc-800 text-blue-500 rounded transition action-btn"
          title="Add Repository"
        >
          <FolderPlus size={18} />
        </button>
      </div>

      {isAdding && (
        <form onSubmit={handleAdd} className="p-4 border-b border-zinc-800 bg-[#0d1117]/50">
          <label className="block text-xs text-gray-400 mb-1">Local Git Repository Path</label>
          <input
            type="text"
            value={newPath}
            onChange={(e) => setNewPath(e.target.value)}
            placeholder="/home/user/src/repo-name"
            className="w-full bg-[#0d1117] border border-zinc-800 rounded px-2 py-1 text-xs text-gray-100 focus:outline-none focus:border-blue-500 mb-2"
          />
          <div className="flex gap-2 justify-end">
            <button
              type="button"
              onClick={() => setIsAdding(false)}
              className="px-2 py-1 bg-zinc-800 text-xs rounded hover:bg-zinc-700 action-btn"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-2 py-1 bg-blue-600 text-xs rounded hover:bg-blue-500 text-white font-medium action-btn"
            >
              Add
            </button>
          </div>
        </form>
      )}

      <div className="px-4 py-2 border-b border-zinc-800">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Filter repositories..."
          className="w-full bg-[#0d1117] border border-zinc-800 rounded px-2.5 py-1 text-xs text-gray-300 focus:outline-none focus:border-zinc-700"
        />
      </div>

      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {filteredRepos.length === 0 ? (
          <p className="text-xs text-gray-500 text-center mt-4">No repositories tracked</p>
        ) : (
          filteredRepos.map((repo) => (
            <div
              key={repo.id}
              className="group flex flex-col p-2 rounded hover:bg-zinc-800/60 transition relative text-xs"
            >
              <div className="flex items-center justify-between font-medium text-gray-200">
                <span className="truncate" title={`${repo.owner}/${repo.name}`}>
                  {repo.owner}/{repo.name}
                </span>
                <div className="flex items-center gap-1.5">
                  {repo.sync_status === 'synced' ? (
                    <span className="w-1.5 h-1.5 rounded-full bg-green-500" title="Synced" />
                  ) : repo.sync_status === 'error' ? (
                    <span title="Sync Error"><AlertCircle size={12} className="text-red-500" /></span>
                  ) : (
                    <RefreshCw size={10} className="animate-spin text-zinc-500" />
                  )}
                  <button
                    onClick={() => removeRepo(repo.id)}
                    className="opacity-0 group-hover:opacity-100 text-gray-500 hover:text-red-400 p-0.5 rounded transition"
                    title="Remove Repository"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              </div>
              <span className="text-[10px] text-gray-500 truncate mt-0.5" title={repo.local_path}>
                {repo.local_path}
              </span>
            </div>
          ))
        )}
      </div>

      <div className="p-3 border-t border-zinc-800 bg-[#0d1117]/30 flex justify-between items-center">
        <span className="text-[10px] text-gray-500">
          {isLoadingRepos ? 'Updating...' : `${repos.length} Repos loaded`}
        </span>
        <button
          onClick={fetchRepos}
          className="p-1 hover:bg-zinc-800 rounded text-gray-400 hover:text-gray-200 transition"
          title="Refresh Repositories"
        >
          <RefreshCw size={12} className={isLoadingRepos ? 'animate-spin' : ''} />
        </button>
      </div>
    </aside>
  );
};
