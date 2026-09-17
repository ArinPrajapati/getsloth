import { createChatPanel } from './chat-panel';

describe('createChatPanel', () => {
  it('sends typed chat text without writing to the terminal', () => {
    const root = document.createElement('div');
    const sent: string[] = [];

    createChatPanel(root, { onSend: (text) => sent.push(text) });
    const input = root.querySelector<HTMLInputElement>('#chat-message');
    expect(input).not.toBeNull();

    if (!input) {
      throw new Error('Expected chat input to render');
    }

    input.value = 'check auth middleware';
    root.querySelector('form')?.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));

    expect(sent).toEqual(['check auth middleware']);
    expect(input.value).toBe('');
  });

  it('renders received messages as text content', () => {
    const root = document.createElement('div');
    const panel = createChatPanel(root, { onSend: () => undefined });

    panel.addMessage({ sender_id: 'viewer-1', sender_role: 'viewer', sender_display_name: 'Phone', text: '<script>alert(1)</script>' });

    expect(root.querySelector('[aria-label="Chat messages"]')?.textContent).toContain('Phone');
    expect(root.querySelector('[aria-label="Chat messages"]')?.textContent).toContain('<script>alert(1)</script>');
    expect(root.querySelector('script')).toBeNull();
  });
});
