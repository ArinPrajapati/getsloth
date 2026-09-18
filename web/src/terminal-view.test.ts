import { createTerminalView, type TerminalLike } from './terminal-view';
import { MAX_INPUT_BYTES } from './protocol';

class FakeTerminal implements TerminalLike {
  openedIn: HTMLElement | null = null;
  readonly writes: Uint8Array[] = [];
  size = { cols: 120, rows: 36 };
  private dataHandler: ((data: string) => void) | null = null;
  private resizeHandler: ((size: { cols: number; rows: number }) => void) | null = null;

  open(element: HTMLElement): void {
    this.openedIn = element;
  }

  write(data: Uint8Array): void {
    this.writes.push(data);
  }

  onData(handler: (data: string) => void): void {
    this.dataHandler = handler;
  }

  fit(): { cols: number; rows: number } {
    return this.size;
  }

  onResize(handler: (size: { cols: number; rows: number }) => void): void {
    this.resizeHandler = handler;
  }

  type(data: string): void {
    this.dataHandler?.(data);
  }

  resize(cols: number, rows: number): void {
    this.size = { cols, rows };
    this.resizeHandler?.(this.size);
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

  it('does not emit terminal size while not the active writer', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onResize = vi.fn();

    createTerminalView(element, () => terminal, { onResize });
    terminal.resize(140, 40);

    expect(onResize).not.toHaveBeenCalled();
  });

  it('emits the current terminal size when control becomes active', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onResize = vi.fn();

    const view = createTerminalView(element, () => terminal, { onResize });
    view.setActive(true);

    expect(onResize).toHaveBeenCalledWith({ cols: 120, rows: 36 });
  });

  it('emits later terminal resizes while active', () => {
    const element = document.createElement('div');
    const terminal = new FakeTerminal();
    const onResize = vi.fn();

    const view = createTerminalView(element, () => terminal, { onResize });
    view.setActive(true);
    terminal.resize(180, 50);

    expect(onResize).toHaveBeenLastCalledWith({ cols: 180, rows: 50 });
  });
});
