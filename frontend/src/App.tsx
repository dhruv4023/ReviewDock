import React, { useEffect, useState } from 'react';
import { useAppStore } from './stores/appStore';
import { Sidebar } from './components/Sidebar';
import { PRTable } from './components/PRTable';
import { DetailsPanel } from './components/DetailsPanel';
import { TerminalPanel } from './components/TerminalPanel';
import { Github, LogOut, AlertTriangle, RefreshCw } from 'lucide-react';

export const App: React.FC = () => {
  const {
    session,
    init,
    login,
    logout,
    oauthError
  } = useAppStore();

  const [isLoggingIn, setIsLoggingIn] = useState(false);

  const handleLogin = async () => {
    setIsLoggingIn(true);
    await login();
    // isLoggingIn stays true until oauth:success or oauth:error clears it
  };

  useEffect(() => {
    if (oauthError) setIsLoggingIn(false);
  }, [oauthError]);

  useEffect(() => {
    init();
  }, []);


  // Show a premium dark login portal if user is not authenticated
  if (!session) {
    return (
      <div className="h-screen w-screen flex flex-col justify-center items-center bg-[#0d1117] text-gray-150 relative overflow-hidden select-none font-sans">
        {/* Abstract glowing backdrops */}
        <div className="absolute top-[-20%] left-[-10%] w-[500px] h-[500px] rounded-full bg-blue-500/10 blur-[120px] pointer-events-none" />
        <div className="absolute bottom-[-10%] right-[-10%] w-[600px] h-[600px] rounded-full bg-indigo-500/10 blur-[130px] pointer-events-none" />

        <div className="bg-[#161b22]/80 border border-zinc-805 rounded-xl shadow-2xl p-8 max-w-md w-full backdrop-blur-md relative z-10 flex flex-col items-center">
          <div className="p-3 bg-zinc-800/40 rounded-full border border-zinc-700/50 text-blue-500 mb-4 animate-pulse">
            <Github size={40} />
          </div>

          <h1 className="text-xl font-bold text-gray-100 tracking-tight mb-1">PR Rebase Manager</h1>
          <p className="text-xs text-gray-500 text-center mb-6">
            Maintain pull requests across multiple repositories concurrently.
          </p>

          {isLoggingIn ? (
            <div className="w-full flex flex-col items-center gap-3 py-2">
              <RefreshCw size={20} className="animate-spin text-blue-400" />
              <p className="text-xs text-gray-400 text-center">
                A browser window has opened. Complete authorization on GitHub, then return here.
              </p>
            </div>
          ) : (
            <div className="w-full space-y-3">
              {oauthError && (
                <div className="p-2.5 bg-red-950/20 border border-red-800 rounded flex items-start gap-2 text-red-400 text-[11px] leading-snug">
                  <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                  <span>{oauthError}</span>
                </div>
              )}
              <button
                onClick={handleLogin}
                className="w-full py-2.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg font-semibold text-xs transition flex items-center justify-center gap-2 action-btn shadow-lg shadow-blue-900/20"
              >
                <Github size={16} /> Sign in with GitHub
              </button>
              <p className="text-[10px] text-gray-600 text-center">
                Opens browser via <code className="text-gray-500">gh auth login</code>
              </p>
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="h-screen w-screen flex flex-col bg-[#0d1117] text-gray-200 overflow-hidden font-sans">
      {/* Title bar / Top Bar */}
      <header className="h-12 border-b border-zinc-800 bg-[#161b22] px-4 flex items-center justify-between select-none">
        <div className="flex items-center gap-2.5">
          <div className="p-1 bg-zinc-800 rounded text-blue-500">
            <Github size={16} />
          </div>
          <span className="font-bold text-xs tracking-wide">PR Rebase Manager</span>
        </div>

        {/* User profile / actions */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <img
              src={session.user.avatar_url}
              alt={session.user.login}
              className="w-6 h-6 rounded-full border border-zinc-700"
            />
            <span className="text-xs font-semibold text-gray-300">{session.user.login}</span>
          </div>

          <div className="h-4 w-px bg-zinc-800" />

          <button
            onClick={logout}
            className="p-1.5 hover:bg-zinc-800 rounded text-gray-500 hover:text-red-400 transition"
            title="Log Out"
          >
            <LogOut size={14} />
          </button>
        </div>
      </header>

      {/* Main Workspace Area (Tri-pane layout) */}
      <div className="flex-1 flex overflow-hidden">
        {/* Pane 1: Repositories list */}
        <Sidebar />

        {/* Pane 2: Central pull request data list */}
        <div className="flex-1 flex flex-col min-w-0">
          <PRTable />
        </div>

        {/* Pane 3: Right selected PR detail panel */}
        <DetailsPanel />
      </div>

      {/* Pane 4: Bottom log console terminal pane */}
      <TerminalPanel />
    </div>
  );
};
export default App;
