import React, { useEffect } from 'react';
import { useAppStore } from './stores/appStore';
import { Sidebar } from './components/Sidebar';
import { PRTable } from './components/PRTable';
import { DetailsPanel } from './components/DetailsPanel';
import { TerminalPanel } from './components/TerminalPanel';
import { Github, LogOut, AlertTriangle, ExternalLink, Copy } from 'lucide-react';

export const App: React.FC = () => {
  const {
    session,
    init,
    login,
    logout,
    oauthPrompt,
    oauthError
  } = useAppStore();

  useEffect(() => {
    init();
  }, []);

  const copyVerificationUrl = async () => {
    try {
      await navigator.clipboard.writeText(oauthPrompt?.verificationUri || "");
      console.log("Verification URL copied");
    } catch (err) {
      console.error("Failed to copy URL", err);
    }
  };

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

          {oauthPrompt ? (
            <div className="w-full space-y-4">
              <div className="bg-[#0b0e14] border border-zinc-800 rounded p-4 text-center">
                <span className="text-[10px] uppercase text-zinc-500 tracking-wider font-semibold block mb-1">
                  Device Authorization Code
                </span>
                <span className="text-2xl font-mono tracking-widest font-bold text-blue-400 select-all">
                  {oauthPrompt.userCode}
                </span>
              </div>
              <p className="text-xs text-gray-400 text-center leading-normal">
                Open the link below and paste the code above to authorize this desktop app.
              </p>
              <a
                href={oauthPrompt.verificationUri}
                target="_blank"
                rel="noopener noreferrer"
                className="w-full py-2 bg-blue-600 hover:bg-blue-500 text-white rounded font-semibold text-xs transition flex items-center justify-center gap-1.5 action-btn"
              >
                Open Verification Link <ExternalLink size={13} />
              </a>
              <button
                type="button"
                onClick={copyVerificationUrl}
                className="px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded font-semibold text-xs transition flex items-center justify-center gap-1.5"
              >
                <Copy size={13} />
                Copy URL
              </button>
              <div className="text-[10px] text-gray-500 text-center italic animate-pulse mt-2">
                Waiting for authorization from GitHub...
              </div>
            </div>
          ) : (
            <div className="w-full space-y-3">
              {oauthError && (
                <div className="p-2.5 bg-red-950/20 border border-red-850 rounded flex items-start gap-2 text-red-400 text-[11px] leading-snug">
                  <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                  <span>{oauthError}</span>
                </div>
              )}
              <button
                onClick={login}
                className="w-full py-2.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg font-semibold text-xs transition flex items-center justify-center gap-2 action-btn shadow-lg shadow-blue-900/20"
              >
                <Github size={16} /> Sign in with GitHub OAuth
              </button>
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
