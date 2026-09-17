export interface TerminalLike {
  open(element: HTMLElement): void;
  write(data: Uint8Array): void;
}

export interface TerminalView {
  write(bytes: Uint8Array): void;
}

export function createTerminalView(element: HTMLElement, createTerminal: () => TerminalLike): TerminalView {
  const terminal = createTerminal();
  terminal.open(element);

  return {
    write(bytes: Uint8Array): void {
      terminal.write(bytes);
    }
  };
}
