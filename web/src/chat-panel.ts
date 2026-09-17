import type { ChatBroadcastMsg } from './protocol';

export interface ChatPanel {
  addMessage(message: Omit<ChatBroadcastMsg, 'v' | 'type'>): void;
}

export interface ChatPanelOptions {
  onSend(text: string): void;
}

export function createChatPanel(root: HTMLElement, options: ChatPanelOptions): ChatPanel {
  const panel = document.createElement('section');
  panel.className = 'chat-card';
  panel.setAttribute('aria-label', 'Session chat');

  const title = document.createElement('h2');
  title.textContent = 'Chat';

  const messages = document.createElement('ol');
  messages.className = 'chat-messages';
  messages.setAttribute('aria-label', 'Chat messages');

  const form = document.createElement('form');
  form.className = 'chat-form';

  const label = document.createElement('label');
  label.htmlFor = 'chat-message';
  label.textContent = 'Message';

  const input = document.createElement('input');
  input.id = 'chat-message';
  input.name = 'chat-message';
  input.autocomplete = 'off';

  const submit = document.createElement('button');
  submit.type = 'submit';
  submit.textContent = 'Send';

  form.append(label, input, submit);
  panel.append(title, messages, form);
  root.append(panel);

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    const text = input.value.trim();

    if (!text) {
      return;
    }

    options.onSend(text);
    input.value = '';
  });

  return {
    addMessage(message): void {
      const item = document.createElement('li');
      const sender = document.createElement('strong');
      const body = document.createElement('span');

      sender.textContent = message.sender_display_name ?? (message.sender_role === 'host' ? 'Host' : 'Viewer');
      body.textContent = message.text;
      item.append(sender, document.createTextNode(': '), body);
      messages.append(item);
    }
  };
}
