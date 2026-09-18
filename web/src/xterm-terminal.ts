import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { TerminalLike, TerminalSize } from './terminal-view';

export function createXtermTerminal(): TerminalLike {
  const terminal = new Terminal({
    cursorBlink: true,
    convertEol: true,
    fontFamily: 'JetBrains Mono, SFMono-Regular, Consolas, monospace',
    fontSize: 14,
    theme: {
      background: '#070808',
      foreground: '#d7e2d1',
      cursor: '#f4efe7',
      selectionBackground: '#3b5f66'
    }
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  let root: HTMLElement | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let onResize: ((size: TerminalSize) => void) | null = null;
  let lastSize: TerminalSize | null = null;

  function fitAndReport(notify: boolean): TerminalSize | null {
    if (!root) {
      return null;
    }

    fitAddon.fit();
    const size = { cols: terminal.cols, rows: terminal.rows };

    if (!lastSize || lastSize.cols !== size.cols || lastSize.rows !== size.rows) {
      lastSize = size;
      if (notify) {
        onResize?.(size);
      }
    }

    return size;
  }

  return {
    open(element): void {
      root = element;
      terminal.open(element);
      fitAndReport(true);

      resizeObserver?.disconnect();
      resizeObserver = new ResizeObserver(() => {
        fitAndReport(true);
      });
      resizeObserver.observe(element);
    },
    write(data): void {
      terminal.write(data);
    },
    onData(handler): void {
      terminal.onData(handler);
    },
    focus(): void {
      terminal.focus();
    },
    fit(): TerminalSize | null {
      return fitAndReport(false);
    },
    onResize(handler): void {
      onResize = handler;
    }
  };
}
