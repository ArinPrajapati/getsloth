import { createQuickActions, quickActionBytes } from './quick-actions';

describe('quickActionBytes', () => {
  it('maps mobile quick actions to PTY bytes', () => {
    expect([...quickActionBytes({ action: 'yes' })]).toEqual([121, 13]);
    expect([...quickActionBytes({ action: 'no' })]).toEqual([110, 13]);
    expect([...quickActionBytes({ action: 'continue' })]).toEqual([13]);
    expect([...quickActionBytes({ action: 'text', text: 'ship it' })]).toEqual([115, 104, 105, 112, 32, 105, 116, 13]);
  });
});

describe('createQuickActions', () => {
  it('sends button and text quick actions as bytes', () => {
    const root = document.createElement('div');
    const sent: number[][] = [];

    createQuickActions(root, { onInput: (bytes) => sent.push([...bytes]) });

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
});
