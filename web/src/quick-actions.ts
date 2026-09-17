export type QuickAction =
  | { action: 'yes' }
  | { action: 'no' }
  | { action: 'continue' }
  | { action: 'text'; text: string };

export interface QuickActionsOptions {
  onInput(bytes: Uint8Array): void;
}

const textEncoder = new TextEncoder();

// docs/protocol.md Limits: input.data_base64, decoded, must be <= 4096 bytes
// — the relay closes the whole connection (BAD_REQUEST, close code 4002) on
// violation, not just the one message, so this has to be enforced client-side.
const MAX_INPUT_BYTES = 4096;

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

  const carriageReturn = textEncoder.encode('\r');
  const textBytes = truncateToByteLimit(action.text, MAX_INPUT_BYTES - carriageReturn.length);
  const bytes = new Uint8Array(textBytes.length + carriageReturn.length);
  bytes.set(textBytes);
  bytes.set(carriageReturn, textBytes.length);
  return bytes;
}

function truncateToByteLimit(text: string, maxBytes: number): Uint8Array {
  const bytes = textEncoder.encode(text);

  if (bytes.length <= maxBytes) {
    return bytes;
  }

  // Back off byte-by-byte until the prefix is valid UTF-8 again, so the cut
  // never lands inside a multi-byte character.
  let end = maxBytes;

  while (end > 0) {
    try {
      new TextDecoder('utf-8', { fatal: true }).decode(bytes.slice(0, end));
      return bytes.slice(0, end);
    } catch {
      end -= 1;
    }
  }

  return new Uint8Array(0);
}

export interface QuickActionsHandle {
  setActive(active: boolean): void;
}

export function createQuickActions(root: HTMLElement, options: QuickActionsOptions): QuickActionsHandle {
  const panel = document.createElement('section');
  panel.className = 'quick-actions-card';
  panel.setAttribute('aria-label', 'Quick actions');

  const title = document.createElement('h2');
  title.textContent = 'Quick actions';

  const buttons = document.createElement('div');
  buttons.className = 'quick-action-buttons';

  // docs/protocol.md: input from a connection that isn't the current active
  // writer is silently dropped by the relay (not forwarded to the host).
  // Buttons stay disabled until this viewer actually holds control, so the
  // UI never implies a tap did something it didn't — see F4's same rule for
  // the take-control button.
  let isActive = false;

  const yes = actionButton('Yes', 'yes', () => {
    if (isActive) {
      options.onInput(quickActionBytes({ action: 'yes' }));
    }
  });
  const no = actionButton('No', 'no', () => {
    if (isActive) {
      options.onInput(quickActionBytes({ action: 'no' }));
    }
  });
  const continueButton = actionButton('Continue', 'continue', () => {
    if (isActive) {
      options.onInput(quickActionBytes({ action: 'continue' }));
    }
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
  // UX guard, not the enforcement boundary — quickActionBytes truncates to
  // the protocol's byte limit regardless of what gets past this.
  input.maxLength = MAX_INPUT_BYTES - 1;

  const submit = document.createElement('button');
  submit.type = 'submit';
  submit.textContent = 'Send';

  form.append(label, input, submit);
  form.addEventListener('submit', (event) => {
    event.preventDefault();

    if (!isActive) {
      return;
    }

    const text = input.value.trim();

    if (!text) {
      return;
    }

    options.onInput(quickActionBytes({ action: 'text', text }));
    input.value = '';
  });

  panel.append(title, buttons, form);
  root.append(panel);

  const controls = [yes, no, continueButton, input, submit];

  function setActive(active: boolean): void {
    isActive = active;

    for (const control of controls) {
      control.disabled = !active;
    }
  }

  setActive(false);

  return { setActive };
}

function actionButton(label: string, action: string, onClick: () => void): HTMLButtonElement {
  const button = document.createElement('button');
  button.type = 'button';
  button.dataset.action = action;
  button.textContent = label;
  button.addEventListener('click', onClick);
  return button;
}
