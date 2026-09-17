import { createTerminalView, type TerminalLike } from './terminal-view';

class FakeTerminal implements TerminalLike {
  openedIn: HTMLElement | null = null;
  readonly writes: Uint8Array[] = [];

  open(element: HTMLElement): void {
    this.openedIn = element;
  }

  write(data: Uint8Array): void {
    this.writes.push(data);
  }
}

describe('createTerminalView', () => {
  it('opens a terminal in the provided element and writes output bytes', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();

    const view = createTerminalView(element, () => terminal);
    view.write(new Uint8Array([72, 105]));

    expect(terminal.openedIn).toBe(element);
    expect(terminal.writes).toEqual([new Uint8Array([72, 105])]);
  });
});
