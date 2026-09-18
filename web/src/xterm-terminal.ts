import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { TerminalLike, TerminalPresentationMode, TerminalSize } from './terminal-view';

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
  let grid: HTMLElement | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let resizeTimer: ReturnType<typeof setTimeout> | null = null;
  let onResize: ((size: TerminalSize) => void) | null = null;
  let lastSize: TerminalSize | null = null;
  let autoFit = false;
  let presentationMode: TerminalPresentationMode = 'fit';

  function resetGridForAutoFit(): void {
    if (!root || !grid) {
      return;
    }

    root.dataset.presentation = 'active';
    grid.style.width = '100%';
    grid.style.height = '100%';
    grid.style.transform = '';
  }

  function fitAndReport(notify: boolean): TerminalSize | null {
    if (!root) {
      return null;
    }

    resetGridForAutoFit();
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

  function proposedSize(): TerminalSize | null {
    if (!root || !terminal.element) {
      return null;
    }

    const screen = terminal.element.querySelector<HTMLElement>('.xterm-screen');
    const rect = screen?.getBoundingClientRect();

    if (!rect || rect.width <= 0 || rect.height <= 0 || terminal.cols <= 0 || terminal.rows <= 0) {
      return fitAddon.proposeDimensions() ?? null;
    }

    const scale = grid ? readScale(grid.style.transform) : 1;
    const cellWidth = rect.width / scale / terminal.cols;
    const cellHeight = rect.height / scale / terminal.rows;

    return {
      cols: Math.max(2, Math.floor(root.clientWidth / cellWidth)),
      rows: Math.max(1, Math.floor(root.clientHeight / cellHeight))
    };
  }

  function updatePresentation(): void {
    if (!root || !grid || autoFit || !terminal.element) {
      return;
    }

    grid.style.transform = '';
    const screen = terminal.element.querySelector<HTMLElement>('.xterm-screen');
    const rect = screen?.getBoundingClientRect();

    if (!rect || rect.width <= 0 || rect.height <= 0) {
      return;
    }

    grid.style.width = `${String(rect.width)}px`;
    grid.style.height = `${String(rect.height)}px`;
    root.dataset.presentation = presentationMode;

    if (presentationMode === 'fit') {
      const scale = Math.min(1, root.clientWidth / rect.width, root.clientHeight / rect.height);
      grid.style.transform = `scale(${String(scale)})`;
    }
  }

  return {
    open(element): void {
      root = element;
      grid = document.createElement('div');
      grid.className = 'terminal-grid';
      element.replaceChildren(grid);
      terminal.open(grid);
      fitAndReport(false);
      autoFit = false;
      updatePresentation();

      resizeObserver?.disconnect();
      resizeObserver = new ResizeObserver(() => {
        if (autoFit) {
          if (resizeTimer !== null) {
            clearTimeout(resizeTimer);
          }
          resizeTimer = setTimeout(() => {
            resizeTimer = null;
            fitAndReport(true);
          }, 75);
        } else {
          updatePresentation();
        }
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
    proposeSize(): TerminalSize | null {
      return proposedSize();
    },
    resize(cols, rows): void {
      if (terminal.cols !== cols || terminal.rows !== rows) {
        terminal.resize(cols, rows);
      }
      lastSize = { cols, rows };
      requestAnimationFrame(updatePresentation);
    },
    setAutoFit(active): void {
      autoFit = active;
      if (active) {
        fitAndReport(false);
      } else {
        if (resizeTimer !== null) {
          clearTimeout(resizeTimer);
          resizeTimer = null;
        }
        updatePresentation();
      }
    },
    setPresentationMode(mode): void {
      presentationMode = mode;
      updatePresentation();
    },
    onResize(handler): void {
      onResize = handler;
    }
  };
}

function readScale(transform: string): number {
  const match = /^scale\(([^)]+)\)$/.exec(transform);
  const value = match ? Number(match[1]) : 1;
  return Number.isFinite(value) && value > 0 ? value : 1;
}
