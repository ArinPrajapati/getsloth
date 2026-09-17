export type QuickAction =
  | { action: 'yes' }
  | { action: 'no' }
  | { action: 'continue' }
  | { action: 'text'; text: string };

export interface QuickActionsOptions {
  onInput(bytes: Uint8Array): void;
}

const textEncoder = new TextEncoder();

export function quickActionBytes(action: QuickAction): Uint8Array {
  if (action.action === 'yes') {
    return textEncoder.encode('y\r');
  }

  if (action.action === 'no') {
    return textEncoder.encode('n\r');
  }

  if (action.action === 'continue') {
    return textEncoder.encode('\r');
  }

  return textEncoder.encode(`${action.text}\r`);
}

export function createQuickActions(root: HTMLElement, options: QuickActionsOptions): void {
  const panel = document.createElement('section');
  panel.className = 'quick-actions-card';
  panel.setAttribute('aria-label', 'Quick actions');

  const title = document.createElement('h2');
  title.textContent = 'Quick actions';

  const buttons = document.createElement('div');
  buttons.className = 'quick-action-buttons';

  const yes = actionButton('Yes', 'yes', () => {
    options.onInput(quickActionBytes({ action: 'yes' }));
  });
  const no = actionButton('No', 'no', () => {
    options.onInput(quickActionBytes({ action: 'no' }));
  });
  const continueButton = actionButton('Continue', 'continue', () => {
    options.onInput(quickActionBytes({ action: 'continue' }));
  });
  buttons.append(yes, no, continueButton);

  const form = document.createElement('form');
  form.className = 'quick-action-form';

  const label = document.createElement('label');
  label.htmlFor = 'quick-action-text';
  label.textContent = 'Short reply';

  const input = document.createElement('input');
  input.id = 'quick-action-text';
  input.name = 'quick-action-text';
  input.autocomplete = 'off';

  const submit = document.createElement('button');
  submit.type = 'submit';
  submit.textContent = 'Send';

  form.append(label, input, submit);
  form.addEventListener('submit', (event) => {
    event.preventDefault();
    const text = input.value.trim();

    if (!text) {
      return;
    }

    options.onInput(quickActionBytes({ action: 'text', text }));
    input.value = '';
  });

  panel.append(title, buttons, form);
  root.append(panel);
}

function actionButton(label: string, action: string, onClick: () => void): HTMLButtonElement {
  const button = document.createElement('button');
  button.type = 'button';
  button.dataset.action = action;
  button.textContent = label;
  button.addEventListener('click', onClick);
  return button;
}
