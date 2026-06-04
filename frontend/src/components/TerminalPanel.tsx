import React, { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { Trash2, Terminal as TermIcon, ChevronDown, ChevronUp } from 'lucide-react';
import '@xterm/xterm/css/xterm.css';

interface TerminalPanelProps {
  height: number;
  onResizeStart: (e: React.MouseEvent) => void;
}

export const TerminalPanel: React.FC<TerminalPanelProps> = ({ height, onResizeStart }) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermInstance = useRef<Terminal | null>(null);
  const fitAddonInstance = useRef<FitAddon | null>(null);
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    if (!terminalRef.current) return;

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

    setTimeout(() => {
      try { fitAddon.fit(); } catch (_) {}
    }, 50);

    term.write('\u001b[36m[SYSTEM] Pull Request Rebase Manager live terminal session initialized.\u001b[0m\r\n');
    xtermInstance.current = term;
    fitAddonInstance.current = fitAddon;

    const logListener = (msg: string) => term.write(msg);
    if (window.runtime) window.runtime.EventsOn('terminal:log', logListener);

    const handleResize = () => {
      try { fitAddon.fit(); } catch (_) {}
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (window.runtime) window.runtime.EventsOff('terminal:log');
      term.dispose();
    };
  }, []);

  // Refit xterm whenever height changes or panel is expanded
  useEffect(() => {
    if (!collapsed) {
      setTimeout(() => {
        try { fitAddonInstance.current?.fit(); } catch (_) {}
      }, 30);
    }
  }, [height, collapsed]);

  const handleClear = () => {
    if (xtermInstance.current) {
      xtermInstance.current.clear();
      xtermInstance.current.write('\u001b[36m[SYSTEM] Terminal logs cleared.\u001b[0m\r\n');
    }
  };

  return (
    <div
      className="bg-[#0b0e14] border-t border-zinc-800 flex flex-col overflow-hidden relative"
      style={{ height: collapsed ? 32 : height }}
    >
      {/* Drag-resize handle on top edge — only when expanded */}
      {!collapsed && (
        <div
          onMouseDown={onResizeStart}
          className="absolute top-0 left-0 w-full h-1 cursor-row-resize hover:bg-blue-600/40 transition z-10 group"
          title="Drag to resize terminal"
        >
          <div className="absolute top-0 left-1/2 -translate-x-1/2 h-1 w-12 rounded-full bg-zinc-700 group-hover:bg-blue-500 transition opacity-0 group-hover:opacity-100" />
        </div>
      )}

      {/* Title bar */}
      <div className="h-8 shrink-0 px-4 border-b border-zinc-800 bg-[#161b22] flex items-center justify-between select-none">
        <div className="flex items-center gap-2 text-zinc-400 text-xs font-semibold uppercase tracking-wider">
          <TermIcon size={13} />
          <span>Execution Output Logs</span>
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={handleClear}
            className="text-zinc-500 hover:text-zinc-300 transition p-1 hover:bg-zinc-800 rounded"
            title="Clear Terminal Output"
          >
            <Trash2 size={13} />
          </button>
          <button
            onClick={() => setCollapsed(c => !c)}
            className="text-zinc-500 hover:text-zinc-300 transition p-1 hover:bg-zinc-800 rounded"
            title={collapsed ? 'Expand terminal' : 'Collapse terminal'}
          >
            {collapsed ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
          </button>
        </div>
      </div>

      {/* Xterm viewport */}
      {!collapsed && (
        <div ref={terminalRef} className="flex-1 p-2 overflow-hidden bg-[#0b0e14]" />
      )}
    </div>
  );
};
export default TerminalPanel;
