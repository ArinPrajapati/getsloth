import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { TerminalLike } from './terminal-view';

export function createXtermTerminal(): TerminalLike {
  return new Terminal({
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
}
