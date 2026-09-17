import type { ControlChangedMsg, PresenceConnection } from './protocol';

export interface ControlPanel {
  setLocalConnectionId(connectionId: string | null): void;
  updateControl(control: Pick<ControlChangedMsg, 'active_writer_id' | 'active_writer_role'>): void;
  updatePresence(connections: PresenceConnection[]): void;
}

export interface ControlPanelOptions {
  localConnectionId: string | null;
  onTakeControl(): void;
}

export function createControlPanel(root: HTMLElement, options: ControlPanelOptions): ControlPanel {
  const panel = document.createElement('section');
  panel.className = 'control-card';
  panel.setAttribute('aria-label', 'Session control');

  const status = document.createElement('p');
  status.className = 'status-label';
  status.setAttribute('aria-label', 'Control status');
  status.textContent = 'Host is driving';

  const button = document.createElement('button');
  button.type = 'button';
  button.textContent = 'Take control';
  button.addEventListener('click', () => {
    options.onTakeControl();
  });

  const participants = document.createElement('ul');
  participants.setAttribute('aria-label', 'Participants');

  panel.append(status, button, participants);
  root.append(panel);

  let localConnectionId = options.localConnectionId;

  return {
    setLocalConnectionId(connectionId): void {
      localConnectionId = connectionId;
    },
    updateControl(control): void {
      const isLocal = control.active_writer_id === localConnectionId;
      status.textContent = isLocal ? 'You are driving' : `${roleLabel(control.active_writer_role)} is driving`;
      button.disabled = isLocal;
    },
    updatePresence(connections): void {
      participants.replaceChildren(
        ...connections.map((connection) => {
          const item = document.createElement('li');
          const name = connection.id === localConnectionId ? 'You' : connection.display_name ?? roleLabel(connection.role);
          item.textContent = `${name}${connection.is_active_writer ? ' — driving' : ''}`;
          return item;
        })
      );
    }
  };
}

function roleLabel(role: 'host' | 'viewer'): string {
  return role === 'host' ? 'Host' : 'Viewer';
}
