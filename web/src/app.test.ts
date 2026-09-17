import { renderAppShell } from './app';

describe('renderAppShell', () => {
  it('renders an accessible placeholder for the session viewer', () => {
    const root = document.createElement('div');

    renderAppShell(root);

    expect(root.querySelector('main')).not.toBeNull();
    expect(root.querySelector('h1')?.textContent).toBe('getsloth');
    expect(root.textContent).toContain('Live agent session viewer');
    expect(root.querySelector('[aria-label="Session status"]')?.textContent).toContain('Waiting for session');
  });
});
