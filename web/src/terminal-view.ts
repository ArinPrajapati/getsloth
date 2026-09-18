import { MAX_INPUT_BYTES, truncateToByteLimit } from './protocol';

export interface TerminalLike {
  open(element: HTMLElement): void;
  write(data: Uint8Array): void;
  onData(handler: (data: string) => void): void;
  focus?(): void;
}

export interface TerminalView {
  focus(): void;
  write(bytes: Uint8Array): void;
  setActive(active: boolean): void;
}

export interface TerminalViewOptions {
  onInput?(bytes: Uint8Array): void;
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
  terminal.open(element);

  let isActive = false;

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
    }
  };
}
