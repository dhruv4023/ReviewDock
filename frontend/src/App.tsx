import React, { useEffect, useState, useRef, useCallback } from 'react';
import { useAppStore } from './stores/appStore';
import { Sidebar } from './components/Sidebar';
import { PRTable } from './components/PRTable';
import { DetailsPanel } from './components/DetailsPanel';
import { TerminalPanel } from './components/TerminalPanel';
import { Github, LogOut, AlertTriangle, RefreshCw, PanelRight, Copy, ExternalLink, Check } from 'lucide-react';

const DETAILS_MIN = 260;
const DETAILS_MAX = 600;
const DETAILS_DEFAULT = 320;

const TERMINAL_MIN = 80;
const TERMINAL_MAX = 600;
const TERMINAL_DEFAULT = 220;

// ── Layout persistence helpers ──────────────────────────────────────────────
const LS_KEYS = {
  sidebarCollapsed: 'layout:sidebarCollapsed',
  detailsWidth:     'layout:detailsWidth',
  terminalHeight:   'layout:terminalHeight',
} as const;

function lsGet<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

function lsSet(key: string, value: unknown) {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch {}
}

export const App: React.FC = () => {
  const { session, init, login, logout, oauthError, setSelectedPR, isCheckingSession, deviceCode, deviceUrl } = useAppStore();
  const [isLoggingIn, setIsLoggingIn] = useState(false);
  const [copiedCode, setCopiedCode] = useState(false);

  // Panel layout state — initialised from localStorage so they survive restarts
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() =>
    lsGet(LS_KEYS.sidebarCollapsed, false)
  );
  const [detailsOpen, setDetailsOpen] = useState(false); // always start closed
  const [detailsWidth, setDetailsWidth] = useState<number>(() =>
    lsGet(LS_KEYS.detailsWidth, DETAILS_DEFAULT)
  );
  const [terminalHeight, setTerminalHeight] = useState<number>(() =>
    lsGet(LS_KEYS.terminalHeight, TERMINAL_DEFAULT)
  );

  // Persist whenever values change
  useEffect(() => { lsSet(LS_KEYS.sidebarCollapsed, sidebarCollapsed); }, [sidebarCollapsed]);
  useEffect(() => { lsSet(LS_KEYS.detailsWidth,     detailsWidth);     }, [detailsWidth]);
  useEffect(() => { lsSet(LS_KEYS.terminalHeight,   terminalHeight);   }, [terminalHeight]);

  // Resize drag tracking refs
  const detailsDragRef = useRef<{ startX: number; startW: number } | null>(null);
  const terminalDragRef = useRef<{ startY: number; startH: number } | null>(null);

  const handleLogin = async () => {
    setIsLoggingIn(true);
    await login();
  };

  const handleCopyCode = (code: string) => {
    if (window.runtime && window.runtime.ClipboardSetText) {
      window.runtime.ClipboardSetText(code);
    } else {
      navigator.clipboard.writeText(code);
    }
    setCopiedCode(true);
    setTimeout(() => setCopiedCode(false), 1500);
  };

  useEffect(() => {
    if (oauthError) setIsLoggingIn(false);
  }, [oauthError]);

  useEffect(() => { init(); }, []);



  // ── Details panel resize ────────────────────────────────────────────────────
  const onDetailsResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    detailsDragRef.current = { startX: e.clientX, startW: detailsWidth };

    const onMove = (ev: MouseEvent) => {
      if (!detailsDragRef.current) return;
      const delta = detailsDragRef.current.startX - ev.clientX; // dragging left edge leftward grows panel
      const newW = Math.min(DETAILS_MAX, Math.max(DETAILS_MIN, detailsDragRef.current.startW + delta));
      setDetailsWidth(newW);
    };
    const onUp = () => {
      detailsDragRef.current = null;
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }, [detailsWidth]);

  // ── Terminal panel resize ───────────────────────────────────────────────────
  const onTerminalResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    terminalDragRef.current = { startY: e.clientY, startH: terminalHeight };

    const onMove = (ev: MouseEvent) => {
      if (!terminalDragRef.current) return;
      const delta = terminalDragRef.current.startY - ev.clientY; // dragging up grows panel
      const newH = Math.min(TERMINAL_MAX, Math.max(TERMINAL_MIN, terminalDragRef.current.startH + delta));
      setTerminalHeight(newH);
    };
    const onUp = () => {
      terminalDragRef.current = null;
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }, [terminalHeight]);

  // Opening details panel (called when a PR row is clicked in PRTable)
  const handleRowClick = useCallback((pr: any) => {
    setSelectedPR(pr);
    setDetailsOpen(true);
  }, []);

  const handleDetailsClose = useCallback(() => {
    setDetailsOpen(false);
    setSelectedPR(null);
  }, []);

  // ── Login screen ────────────────────────────────────────────────────────────
  if (isCheckingSession) {
    return (
      <div className="h-screen w-screen flex flex-col justify-center items-center bg-[#0d1117] text-zinc-400 select-none">
        <RefreshCw size={28} className="animate-spin text-blue-500 mb-2.5" />
        <span className="text-xs">Checking GitHub session...</span>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="h-screen w-screen flex flex-col justify-center items-center bg-[#0d1117] text-gray-150 relative overflow-hidden select-none font-sans">
        <div className="absolute top-[-20%] left-[-10%] w-[500px] h-[500px] rounded-full bg-blue-500/10 blur-[120px] pointer-events-none" />
        <div className="absolute bottom-[-10%] right-[-10%] w-[600px] h-[600px] rounded-full bg-indigo-500/10 blur-[130px] pointer-events-none" />

        <div className="bg-[#161b22]/80 border border-zinc-800 rounded-xl shadow-2xl p-8 max-w-md w-full backdrop-blur-md relative z-10 flex flex-col items-center">
          <div className="p-3 bg-zinc-800/40 rounded-full border border-zinc-700/50 text-blue-500 mb-4 animate-pulse">
            <Github size={40} />
          </div>
          <h1 className="text-xl font-bold text-gray-100 tracking-tight mb-1">PR Rebase Manager</h1>
          <p className="text-xs text-gray-500 text-center mb-6">
            Maintain pull requests across multiple repositories concurrently.
          </p>

          {isLoggingIn ? (
            deviceCode ? (
              <div className="w-full space-y-5">
                <div className="p-4 bg-zinc-900/40 border border-zinc-800 rounded-lg flex flex-col items-center gap-2.5">
                  <span className="text-[10px] uppercase font-semibold tracking-wider text-zinc-500 select-none">
                    One-Time Activation Code
                  </span>
                  <div className="flex items-center gap-3">
                    <span className="text-2xl font-mono font-bold tracking-widest text-white select-all">
                      {deviceCode}
                    </span>
                    <button
                      onClick={() => handleCopyCode(deviceCode)}
                      className="p-1.5 hover:bg-zinc-800 rounded text-zinc-400 hover:text-zinc-200 transition cursor-pointer flex items-center justify-center"
                      title="Copy Code"
                    >
                      {copiedCode ? <Check size={14} className="text-green-500" /> : <Copy size={14} />}
                    </button>
                  </div>
                </div>

                <div className="space-y-2.5 text-center">
                  <p className="text-xs text-zinc-400 leading-normal">
                    Copy the code above, click the link below, and paste the code to authorize this application:
                  </p>
                  <a
                    href={deviceUrl || "https://github.com/login/device"}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => {
                      if (window.runtime && window.runtime.BrowserOpenURL) {
                        e.preventDefault();
                        window.runtime.BrowserOpenURL(deviceUrl || "https://github.com/login/device");
                      }
                    }}
                    className="inline-flex items-center gap-1.5 text-xs text-blue-400 hover:text-blue-300 font-semibold underline underline-offset-4 cursor-pointer"
                  >
                    {deviceUrl || "github.com/login/device"} <ExternalLink size={12} />
                  </a>
                </div>

                <div className="pt-2 flex items-center justify-center gap-2 text-[10px] text-zinc-500 select-none">
                  <RefreshCw size={11} className="animate-spin text-blue-500" />
                  <span>Waiting for GitHub CLI activation...</span>
                </div>
              </div>
            ) : (
              <div className="w-full flex flex-col items-center gap-3 py-4">
                <RefreshCw size={20} className="animate-spin text-blue-400" />
                <p className="text-xs text-zinc-400 text-center">
                  Requesting one-time device code...
                </p>
              </div>
            )
          ) : (
            <div className="w-full space-y-4">
              {oauthError && (
                <div className="p-2.5 bg-red-950/20 border border-red-800 rounded flex items-start gap-2 text-red-400 text-[11px] leading-snug">
                  <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                  <span>{oauthError}</span>
                </div>
              )}
              
              <button
                onClick={handleLogin}
                className="w-full py-2.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg font-semibold text-xs transition flex items-center justify-center gap-2 shadow-lg shadow-blue-900/20 cursor-pointer"
              >
                <Github size={16} /> Sign in with GitHub CLI
              </button>
              
              <p className="text-[10px] text-zinc-500 text-center leading-normal">
                Uses standard <code className="text-zinc-400">gh auth login</code> device activation. No browser is opened automatically.
              </p>
            </div>
          )}
        </div>
      </div>
    );
  }

  // ── Main workspace ──────────────────────────────────────────────────────────
  return (
    <div className="h-screen w-screen flex flex-col bg-[#0d1117] text-gray-200 overflow-hidden font-sans">
      {/* Title bar */}
      <header className="h-12 border-b border-zinc-800 bg-[#161b22] px-4 flex items-center justify-between select-none shrink-0">
        <div className="flex items-center gap-2.5">
          <div className="p-1 bg-zinc-800 rounded text-blue-500">
            <Github size={16} />
          </div>
          <span className="font-bold text-xs tracking-wide">PR Rebase Manager</span>
        </div>

        <div className="flex items-center gap-3">
          {/* Toggle details panel from toolbar */}
          <button
            onClick={() => setDetailsOpen(o => !o)}
            className={`p-1.5 rounded transition text-xs ${detailsOpen ? 'bg-zinc-700 text-gray-200' : 'hover:bg-zinc-800 text-zinc-500 hover:text-gray-300'}`}
            title={detailsOpen ? 'Hide details panel' : 'Show details panel'}
          >
            <PanelRight size={15} />
          </button>

          <div className="h-4 w-px bg-zinc-800" />

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

      {/* Main workspace: sidebar | table | details */}
      <div className="flex-1 flex overflow-hidden min-h-0">
        <Sidebar
          collapsed={sidebarCollapsed}
          onToggleCollapse={() => setSidebarCollapsed(c => !c)}
        />

        {/* Centre: PR table fills remaining space */}
        <div className="flex-1 flex flex-col min-w-0">
          <PRTable onRowClick={handleRowClick} />
        </div>

        {/* Right: Details panel — hidden when closed */}
        {detailsOpen && (
          <DetailsPanel
            width={detailsWidth}
            onClose={handleDetailsClose}
            onResizeStart={onDetailsResizeStart}
          />
        )}
      </div>

      {/* Bottom terminal */}
      <TerminalPanel
        height={terminalHeight}
        onResizeStart={onTerminalResizeStart}
      />
    </div>
  );
};
export default App;
