import { renderAppShell } from './app';

describe('renderAppShell', () => {
  it('renders a terminal-first viewer shell with a status bar', () => {
    const root = document.createElement('div');

    renderAppShell(root);

    expect(root.querySelector('main')).not.toBeNull();
    expect(root.querySelector('h1')).toBeNull();
    expect(root.textContent).not.toContain('getsloth');
    expect(root.querySelector('[aria-label="Terminal output"]')).not.toBeNull();
    expect(root.querySelector('[aria-label="Session status bar"]')?.textContent).toContain('Waiting for session');
    expect(root.querySelector('[aria-label="Focus terminal input"]')).not.toBeNull();
  });
});
