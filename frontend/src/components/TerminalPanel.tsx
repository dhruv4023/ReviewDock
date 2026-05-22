import React, { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { Trash2, Terminal as TermIcon } from 'lucide-react';
import '@xterm/xterm/css/xterm.css';

export const TerminalPanel: React.FC = () => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermInstance = useRef<Terminal | null>(null);
  const fitAddonInstance = useRef<FitAddon | null>(null);

  useEffect(() => {
    if (!terminalRef.current) return;

    // Instantiate Xterm
    const term = new Terminal({
      theme: {
        background: '#0b0e14',
        foreground: '#f8f8f2',
        cursor: '#f8f8f0',
        black: '#000000',
        red: '#ff5555',
        green: '#50fa7b',
        yellow: '#f1fa8c',
        blue: '#bd93f9',
        magenta: '#ff79c6',
        cyan: '#8be9fd',
        white: '#bfbfbf',
      },
      cursorBlink: false,
      fontSize: 11,
      fontFamily: 'Menlo, Monaco, Consolas, "Courier New", monospace',
      convertEol: true,
      rows: 10,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    term.open(terminalRef.current);
    
    // Delay fit to ensure DOM has reflowed and dimensions are computed
    setTimeout(() => {
      try {
        fitAddon.fit();
      } catch (err) {
        console.warn('Initial xterm fit delayed/failed:', err);
      }
    }, 50);

    // Welcome Message
    term.write('\u001b[36m[SYSTEM] Pull Request Rebase Manager live terminal session initialized.\u001b[0m\r\n');

    xtermInstance.current = term;
    fitAddonInstance.current = fitAddon;

    // Subscribe to Wails log stream events
    const logListener = (msg: string) => {
      term.write(msg);
    };

    if (window.runtime) {
      window.runtime.EventsOn('terminal:log', logListener);
    }

    // Resize event listener
    const handleResize = () => {
      try {
        fitAddon.fit();
      } catch (err) {
        // Suppress dimensions errors if DOM container is hidden/zero-sized
      }
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (window.runtime) {
        window.runtime.EventsOff('terminal:log');
      }
      term.dispose();
    };
  }, []);

  const handleClear = () => {
    if (xtermInstance.current) {
      xtermInstance.current.clear();
      xtermInstance.current.write('\u001b[36m[SYSTEM] Terminal logs cleared.\u001b[0m\r\n');
    }
  };

  return (
    <div className="h-60 bg-[#0b0e14] border-t border-zinc-800 flex flex-col overflow-hidden">
      {/* Terminal Title Bar */}
      <div className="h-8 px-4 border-b border-zinc-800 bg-[#161b22] flex items-center justify-between select-none">
        <div className="flex items-center gap-2 text-zinc-400 text-xs font-semibold uppercase tracking-wider">
          <TermIcon size={13} />
          <span>Execution Output Logs</span>
        </div>
        <button
          onClick={handleClear}
          className="text-zinc-500 hover:text-zinc-300 transition p-1 hover:bg-zinc-800 rounded action-btn"
          title="Clear Terminal Output"
        >
          <Trash2 size={13} />
        </button>
      </div>

      {/* Xterm viewport container */}
      <div ref={terminalRef} className="flex-1 p-2 overflow-hidden bg-[#0b0e14]" />
    </div>
  );
};
export default TerminalPanel;
