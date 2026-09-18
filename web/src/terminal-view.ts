export interface TerminalLike {
  open(element: HTMLElement): void;
  write(data: Uint8Array): void;
  onData(handler: (data: string) => void): void;
}

export interface TerminalView {
  write(bytes: Uint8Array): void;
  setActive(active: boolean): void;
}

export interface TerminalViewOptions {
  onInput?(bytes: Uint8Array): void;
}

const textEncoder = new TextEncoder();

// Typed keystrokes are only ever forwarded while this viewer is the active
// writer - matches F4's requirement that a non-active-writer's local
// keystrokes never get sent, so the UI doesn't imply a keypress did
// something the relay would silently drop (docs/protocol.md).
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

    options.onInput?.(textEncoder.encode(data));
  });

  return {
    write(bytes: Uint8Array): void {
      terminal.write(bytes);
    },
    setActive(active: boolean): void {
      isActive = active;
    }
  };
}
