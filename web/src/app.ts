import './styles.css';

export function renderAppShell(root: HTMLElement): void {
  root.innerHTML = '';

  const main = document.createElement('main');
  main.className = 'app-shell';

  const header = document.createElement('header');
  header.className = 'hero';

  const eyebrow = document.createElement('p');
  eyebrow.className = 'eyebrow';
  eyebrow.textContent = 'Live agent session viewer';

  const title = document.createElement('h1');
  title.textContent = 'getsloth';

  const summary = document.createElement('p');
  summary.className = 'summary';
  summary.textContent = 'Watch and control a shared terminal session from this browser. WebSocket streaming lands in the next slice.';

  header.append(eyebrow, title, summary);

  const status = document.createElement('section');
  status.className = 'status-card';
  status.setAttribute('aria-label', 'Session status');

  const statusLabel = document.createElement('p');
  statusLabel.className = 'status-label';
  statusLabel.textContent = 'Waiting for session';

  const statusText = document.createElement('p');
  statusText.setAttribute('aria-label', 'Connection status');
  statusText.textContent = 'Open a getsloth share link to connect to a live PTY stream.';

  status.append(statusLabel, statusText);

  const terminal = document.createElement('section');
  terminal.className = 'terminal-card';
  terminal.setAttribute('aria-label', 'Terminal output');

  const terminalSurface = document.createElement('div');
  terminalSurface.className = 'terminal-surface';
  terminalSurface.dataset.terminal = 'true';

  terminal.append(terminalSurface);
  main.append(header, status, terminal);
  root.append(main);
}
