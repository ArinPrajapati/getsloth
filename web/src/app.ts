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

  const controlButton = statusButton('Control', 'control');
  controlButton.setAttribute('aria-label', 'Open session control');

  const settingsButton = statusButton('Settings', 'settings');
  settingsButton.setAttribute('aria-label', 'Open terminal settings');

  actionGroup.append(chatButton, typeButton, controlButton, settingsButton);
  statusBar.append(sessionGroup, focusText, actionGroup);

  const overlayLayer = document.createElement('div');
  overlayLayer.className = 'viewer-overlays';
  overlayLayer.setAttribute('aria-label', 'Viewer overlays');

  const chatPanel = overlayPanel('chat', 'Chat overlay');
  const controlPanel = overlayPanel('control', 'Control overlay');
  const settingsPanel = overlayPanel('settings', 'Terminal settings');

  settingsPanel.append(createSettingsPanel());
  overlayLayer.append(chatPanel, controlPanel, settingsPanel);

  main.append(terminal, overlayLayer, statusBar);
  root.append(main);
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
  themeLabel.textContent = 'Viewer-local theme controls land in the implementation pass.';

  form.append(title, fontLabel, fontSelect, themeLabel);
  return form;
}
