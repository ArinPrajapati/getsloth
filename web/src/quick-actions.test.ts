import { createQuickActions, quickActionBytes } from './quick-actions';

describe('quickActionBytes', () => {
  it('maps mobile quick actions to PTY bytes', () => {
    expect([...quickActionBytes({ action: 'yes' })]).toEqual([121, 13]);
    expect([...quickActionBytes({ action: 'no' })]).toEqual([110, 13]);
    expect([...quickActionBytes({ action: 'continue' })]).toEqual([13]);
    expect([...quickActionBytes({ action: 'text', text: 'ship it' })]).toEqual([115, 104, 105, 112, 32, 105, 116, 13]);
  });

  it('truncates free text so the encoded input never exceeds the relay\'s 4096-byte limit', () => {
    // docs/protocol.md Limits: input.data_base64, decoded, must be <= 4096 bytes,
    // or the relay closes the whole connection with BAD_REQUEST (4002).
    const bytes = quickActionBytes({ action: 'text', text: 'a'.repeat(5000) });

    expect(bytes.length).toBeLessThanOrEqual(4096);
    expect(bytes.at(-1)).toBe(13);
  });

  it('does not split a multi-byte UTF-8 character when truncating', () => {
    const bytes = quickActionBytes({ action: 'text', text: '💚'.repeat(2000) });

    expect(bytes.length).toBeLessThanOrEqual(4096);
    expect(() => new TextDecoder('utf-8', { fatal: true }).decode(bytes.slice(0, -1))).not.toThrow();
  });
});

describe('createQuickActions', () => {
  it('sends button and text quick actions as bytes once active', () => {
    const root = document.createElement('div');
    const sent: number[][] = [];

    const quickActions = createQuickActions(root, { onInput: (bytes) => sent.push([...bytes]) });
    quickActions.setActive(true);

    root.querySelector<HTMLButtonElement>('[data-action="yes"]')?.click();
    root.querySelector<HTMLButtonElement>('[data-action="no"]')?.click();
    root.querySelector<HTMLButtonElement>('[data-action="continue"]')?.click();

    const input = root.querySelector<HTMLInputElement>('#quick-action-text');
    expect(input).not.toBeNull();

    if (!input) {
      throw new Error('Expected quick action text input to render');
    }

    input.value = 'retry tests';
    root.querySelector('form')?.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));

    expect(sent).toEqual([
      [121, 13],
      [110, 13],
      [13],
      [114, 101, 116, 114, 121, 32, 116, 101, 115, 116, 115, 13]
    ]);
    expect(input.value).toBe('');
  });

  it('starts inactive and disables every control, since input is silently dropped when not the active writer', () => {
    const root = document.createElement('div');

    createQuickActions(root, { onInput: vi.fn() });

    expect(root.querySelector<HTMLButtonElement>('[data-action="yes"]')?.disabled).toBe(true);
    expect(root.querySelector<HTMLButtonElement>('[data-action="no"]')?.disabled).toBe(true);
    expect(root.querySelector<HTMLButtonElement>('[data-action="continue"]')?.disabled).toBe(true);
    expect(root.querySelector<HTMLInputElement>('#quick-action-text')?.disabled).toBe(true);
    expect(root.querySelector('form button[type="submit"]')?.hasAttribute('disabled')).toBe(true);
  });

  it('does not call onInput while inactive, even if a disabled control is force-clicked', () => {
    const root = document.createElement('div');
    const onInput = vi.fn();

    createQuickActions(root, { onInput });
    root.querySelector<HTMLButtonElement>('[data-action="yes"]')?.click();

    expect(onInput).not.toHaveBeenCalled();
  });

  it('re-disables every control when control is lost again', () => {
    const root = document.createElement('div');
    const onInput = vi.fn();

    const quickActions = createQuickActions(root, { onInput });
    quickActions.setActive(true);
    quickActions.setActive(false);

    root.querySelector<HTMLButtonElement>('[data-action="yes"]')?.click();

    expect(root.querySelector<HTMLButtonElement>('[data-action="yes"]')?.disabled).toBe(true);
    expect(onInput).not.toHaveBeenCalled();
  });
});
