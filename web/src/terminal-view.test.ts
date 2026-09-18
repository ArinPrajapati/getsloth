import { createTerminalView, type TerminalLike } from './terminal-view';
import { MAX_INPUT_BYTES } from './protocol';

class FakeTerminal implements TerminalLike {
  openedIn: HTMLElement | null = null;
  readonly writes: Uint8Array[] = [];
  private dataHandler: ((data: string) => void) | null = null;

  open(element: HTMLElement): void {
    this.openedIn = element;
  }

  write(data: Uint8Array): void {
    this.writes.push(data);
  }

  onData(handler: (data: string) => void): void {
    this.dataHandler = handler;
  }

  type(data: string): void {
    this.dataHandler?.(data);
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

  it('does not forward keystrokes when not the active writer', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onInput = vi.fn();

    createTerminalView(element, () => terminal, { onInput });
    terminal.type('y');

    expect(onInput).not.toHaveBeenCalled();
  });

  it('forwards keystrokes as encoded bytes once set active', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onInput = vi.fn();

    const view = createTerminalView(element, () => terminal, { onInput });
    view.setActive(true);
    terminal.type('y');

    expect(onInput).toHaveBeenCalledWith(new TextEncoder().encode('y'));
  });

  it('stops forwarding keystrokes once control is taken back', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onInput = vi.fn();

    const view = createTerminalView(element, () => terminal, { onInput });
    view.setActive(true);
    view.setActive(false);
    terminal.type('y');

    expect(onInput).not.toHaveBeenCalled();
  });

  it('truncates a pasted chunk to the protocol byte limit instead of getting the connection closed', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onInput = vi.fn();

    const view = createTerminalView(element, () => terminal, { onInput });
    view.setActive(true);
    terminal.type('a'.repeat(MAX_INPUT_BYTES + 500));

    const sent = onInput.mock.calls[0]?.[0] as Uint8Array;
    expect(sent.length).toBeLessThanOrEqual(MAX_INPUT_BYTES);
  });
});
