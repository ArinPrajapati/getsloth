import { MAX_INPUT_BYTES, truncateToByteLimit } from './protocol';

export interface TerminalLike {
  open(element: HTMLElement): void;
  write(data: Uint8Array): void;
  onData(handler: (data: string) => void): void;
  focus?(): void;
  fit?(): TerminalSize | null;
  proposeSize?(): TerminalSize | null;
  resize?(cols: number, rows: number): void;
  setAutoFit?(active: boolean): void;
  setPresentationMode?(mode: TerminalPresentationMode): void;
  onResize?(handler: (size: TerminalSize) => void): void;
}

export interface TerminalSize {
  cols: number;
  rows: number;
}

export interface TerminalView {
  focus(): void;
  write(bytes: Uint8Array): void;
  setActive(active: boolean): void;
  setCanonicalSize(size: TerminalSize): void;
  desiredSize(): TerminalSize;
  setPresentationMode(mode: TerminalPresentationMode): void;
}

export type TerminalPresentationMode = 'fit' | 'actual';

export interface TerminalViewOptions {
  onInput?(bytes: Uint8Array): void;
  onResize?(size: TerminalSize): void;
}

// Typed keystrokes are only ever forwarded while this viewer is the active
// writer - matches F4's requirement that a non-active-writer's local
// keystrokes never get sent, so the UI doesn't imply a keypress did
// something the relay would silently drop (docs/protocol.md). Every chunk
// is also truncated to the protocol's input byte limit before sending - a
// paste can hand xterm's onData a single large chunk, and exceeding the
// limit closes the whole connection (see MAX_INPUT_BYTES).
export function createTerminalView(
  element: HTMLElement,
  createTerminal: () => TerminalLike,
  options: TerminalViewOptions = {}
): TerminalView {
  const terminal = createTerminal();

  let isActive = false;
  let lastSize: TerminalSize | null = null;
  let applyingCanonicalSize = false;

  function rememberSize(size: TerminalSize | null): void {
    if (!size) {
      return;
    }
    lastSize = size;

    if (isActive) {
      options.onResize?.(size);
    }
  }

  terminal.onResize?.((size) => {
    if (applyingCanonicalSize) {
      lastSize = size;
      return;
    }
    rememberSize(size);
  });
  terminal.open(element);
  rememberSize(terminal.proposeSize?.() ?? terminal.fit?.() ?? null);
  terminal.setAutoFit?.(false);

  terminal.onData((data) => {
    if (!isActive) {
      return;
    }

    options.onInput?.(truncateToByteLimit(data, MAX_INPUT_BYTES));
  });

  return {
    focus(): void {
      terminal.focus?.();
    },
    write(bytes: Uint8Array): void {
      terminal.write(bytes);
    },
    setActive(active: boolean): void {
      isActive = active;
      terminal.setAutoFit?.(active);

      if (active) {
        const fitSize = terminal.fit?.() ?? terminal.proposeSize?.() ?? null;
        rememberSize(fitSize ?? lastSize);
      }
    },
    setCanonicalSize(size: TerminalSize): void {
      lastSize = size;
      applyingCanonicalSize = true;
      terminal.resize?.(size.cols, size.rows);
      applyingCanonicalSize = false;
    },
    desiredSize(): TerminalSize {
      return terminal.proposeSize?.() ?? terminal.fit?.() ?? lastSize ?? { cols: 80, rows: 24 };
    },
    setPresentationMode(mode: TerminalPresentationMode): void {
      terminal.setPresentationMode?.(mode);
    }
  };
}
