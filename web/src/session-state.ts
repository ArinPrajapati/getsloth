import type { SessionEndedMsg } from './protocol';

export interface SessionState {
  showKicked(): void;
  showSessionEnded(reason: SessionEndedMsg['reason']): void;
}

export function createSessionState(root: HTMLElement): SessionState {
  function show(title: string, body: string): void {
    root.innerHTML = '';

    const panel = document.createElement('section');
    panel.className = 'session-status-card';
    panel.setAttribute('aria-label', 'Session status');
    panel.setAttribute('role', 'status');

    const heading = document.createElement('h1');
    heading.textContent = title;

    const message = document.createElement('p');
    message.textContent = body;

    panel.append(heading, message);
    root.append(panel);
  }

  return {
    showKicked(): void {
      show('Host ended your access', 'The host used the kill switch to disconnect this session. Ask for a new link to rejoin.');
    },
    showSessionEnded(reason: SessionEndedMsg['reason']): void {
      show('Session is over', sessionEndedBody(reason));
    }
  };
}

function sessionEndedBody(reason: SessionEndedMsg['reason']): string {
  if (reason === 'process_exited') {
    return 'The wrapped command finished running.';
  }

  if (reason === 'host_ended') {
    return 'The host ended the session.';
  }

  return 'The host disconnected and the session was closed.';
}
