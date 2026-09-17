import { createControlPanel } from './control-panel';

describe('createControlPanel', () => {
  it('requests control only when the button is clicked', () => {
    const root = document.createElement('div');
    const requests: number[] = [];

    const panel = createControlPanel(root, {
      localConnectionId: 'viewer-1',
      onTakeControl: () => requests.push(1)
    });

    expect(requests).toEqual([]);
    root.querySelector('button')?.click();

    expect(requests).toEqual([1]);
    expect(panel).toBeDefined();
  });

  it('renders active writer and presence updates from relay state', () => {
    const root = document.createElement('div');
    const panel = createControlPanel(root, {
      localConnectionId: 'viewer-1',
      onTakeControl: () => undefined
    });

    panel.updatePresence([
      { id: 'host-1', role: 'host', is_active_writer: false },
      { id: 'viewer-1', role: 'viewer', is_active_writer: true },
      { id: 'viewer-2', role: 'viewer', display_name: 'Phone', is_active_writer: false }
    ]);
    panel.updateControl({ active_writer_id: 'viewer-1', active_writer_role: 'viewer' });

    expect(root.querySelector('[aria-label="Control status"]')?.textContent).toContain('You are driving');
    expect(root.querySelector('[aria-label="Participants"]')?.textContent).toContain('Host');
    expect(root.querySelector('[aria-label="Participants"]')?.textContent).toContain('Phone');
    expect(root.querySelector('button')?.disabled).toBe(true);

    panel.updateControl({ active_writer_id: 'host-1', active_writer_role: 'host' });

    expect(root.querySelector('[aria-label="Control status"]')?.textContent).toContain('Host is driving');
    expect(root.querySelector('button')?.disabled).toBe(false);
  });
});
