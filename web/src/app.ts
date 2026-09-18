import './styles.css';

export function renderAppShell(root: HTMLElement): void {
  root.innerHTML = '';

  const main = document.createElement('main');
  main.className = 'viewer-shell';

  const terminal = document.createElement('section');
  terminal.className = 'terminal-stage';
  terminal.setAttribute('aria-label', 'Terminal output');

  const terminalSurface = document.createElement('div');
  terminalSurface.className = 'terminal-surface';
  terminalSurface.dataset.terminal = 'true';

  terminal.append(terminalSurface);

  const statusBar = document.createElement('nav');
  statusBar.className = 'session-status-bar';
  statusBar.setAttribute('aria-label', 'Session status bar');

  const sessionGroup = document.createElement('div');
  sessionGroup.className = 'status-bar-group status-bar-session';

  const statusText = document.createElement('span');
  statusText.className = 'status-bar-item';
  statusText.setAttribute('aria-label', 'Connection status');
  statusText.textContent = 'Waiting for session';

  const controlText = document.createElement('span');
  controlText.className = 'status-bar-item';
  controlText.setAttribute('aria-label', 'Active writer status');
  controlText.textContent = 'host driving';

  sessionGroup.append(statusText, controlText);

  const focusText = document.createElement('span');
  focusText.className = 'status-bar-focus';
  focusText.setAttribute('aria-label', 'Session focus');
  focusText.textContent = 'terminal';

  const actionGroup = document.createElement('div');
  actionGroup.className = 'status-bar-group status-bar-actions';

  const chatButton = statusButton('Chat', 'chat');
  chatButton.setAttribute('aria-label', 'Open chat');

  const typeButton = statusButton('Type', 'type');
  typeButton.className = 'status-bar-button primary';
  typeButton.setAttribute('aria-label', 'Focus terminal input');
  typeButton.dataset.remoteOnly = 'true';

  const controlButton = statusButton('Control', 'control');
  controlButton.setAttribute('aria-label', 'Open session control');
  controlButton.dataset.remoteOnly = 'true';

  const settingsButton = statusButton('Settings', 'settings');
  settingsButton.setAttribute('aria-label', 'Open terminal settings');

  actionGroup.append(chatButton, typeButton, controlButton, settingsButton);
  statusBar.append(sessionGroup, focusText, actionGroup);

  const terminalHelper = createMobileTerminalHelper();

  const overlayLayer = document.createElement('div');
  overlayLayer.className = 'viewer-overlays';
  overlayLayer.setAttribute('aria-label', 'Viewer overlays');

  const chatPanel = overlayPanel('chat', 'Chat overlay');
  const controlPanel = overlayPanel('control', 'Control overlay');
  const settingsPanel = overlayPanel('settings', 'Terminal settings');

  settingsPanel.append(createSettingsPanel());
  overlayLayer.append(chatPanel, controlPanel, settingsPanel);

  main.append(terminal, overlayLayer, terminalHelper, statusBar);
  root.append(main);
}

function createMobileTerminalHelper(): HTMLElement {
  const row = document.createElement('nav');
  row.className = 'mobile-terminal-helper';
  row.setAttribute('aria-label', 'Terminal helper keys');
  row.hidden = true;

  for (const { label, key, ariaLabel } of [
    { label: 'Esc', key: 'escape', ariaLabel: 'Escape' },
    { label: 'Ctrl', key: 'ctrl', ariaLabel: 'Ctrl' },
    { label: 'Tab', key: 'tab', ariaLabel: 'Tab' },
    { label: '←', key: 'arrow-left', ariaLabel: 'Left' },
    { label: '↑', key: 'arrow-up', ariaLabel: 'Up' },
    { label: '↓', key: 'arrow-down', ariaLabel: 'Down' },
    { label: '→', key: 'arrow-right', ariaLabel: 'Right' },
    { label: 'Paste', key: 'paste', ariaLabel: 'Paste' }
  ]) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'mobile-terminal-helper-button';
    button.dataset.terminalKey = key;
    button.textContent = label;
    button.setAttribute('aria-label', `Terminal helper: ${ariaLabel}`);
    row.append(button);
  }

  return row;
}

function statusButton(label: string, panel: string): HTMLButtonElement {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'status-bar-button';
  button.dataset.panel = panel;
  button.textContent = label;
  return button;
}

function overlayPanel(panel: string, label: string): HTMLElement {
  const element = document.createElement('section');
  element.className = 'viewer-overlay';
  element.dataset.panel = panel;
  element.setAttribute('aria-label', label);
  element.hidden = true;
  return element;
}

function createSettingsPanel(): HTMLElement {
  const form = document.createElement('form');
  form.className = 'settings-card';

  const title = document.createElement('h2');
  title.textContent = 'Terminal settings';

  const fontLabel = document.createElement('label');
  fontLabel.htmlFor = 'terminal-font-size';
  fontLabel.textContent = 'Font size';

  const fontSelect = document.createElement('select');
  fontSelect.id = 'terminal-font-size';
  fontSelect.name = 'terminal-font-size';
  fontSelect.disabled = true;

  for (const size of ['13', '14', '16', '18']) {
    const option = document.createElement('option');
    option.value = size;
    option.textContent = `${size}px`;
    option.selected = size === '14';
    fontSelect.append(option);
  }

  const themeLabel = document.createElement('p');
  themeLabel.className = 'settings-note';
  themeLabel.textContent = 'Fit keeps the whole terminal visible. Actual size preserves readable text and allows panning.';

  const viewLabel = document.createElement('label');
  viewLabel.htmlFor = 'terminal-view-mode';
  viewLabel.textContent = 'Terminal view';

  const viewSelect = document.createElement('select');
  viewSelect.id = 'terminal-view-mode';
  viewSelect.name = 'terminal-view-mode';

  for (const [value, label] of [['fit', 'Fit to screen'], ['actual', 'Actual size']] as const) {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = label;
    viewSelect.append(option);
  }

  form.append(title, viewLabel, viewSelect, themeLabel, fontLabel, fontSelect);
  return form;
}
