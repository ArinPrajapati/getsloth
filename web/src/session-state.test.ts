import { createSessionState } from './session-state';

describe('createSessionState', () => {
  it('shows a kicked state distinct from session_ended, replacing prior content', () => {
    const root = document.createElement('div');
    const stale = document.createElement('p');
    stale.textContent = 'stale terminal content';
    root.append(stale);

    const sessionState = createSessionState(root);
    sessionState.showKicked();

    expect(root.contains(stale)).toBe(false);
    const panel = root.querySelector('[aria-label="Session status"]');
    expect(panel?.textContent).toContain('Host ended your access');
    expect(panel?.textContent).not.toContain('Session is over');
  });

  it('shows a session_ended state distinct from kicked, replacing prior content', () => {
    const root = document.createElement('div');
    const stale = document.createElement('p');
    stale.textContent = 'stale terminal content';
    root.append(stale);

    const sessionState = createSessionState(root);
    sessionState.showSessionEnded('process_exited');

    expect(root.contains(stale)).toBe(false);
    const panel = root.querySelector('[aria-label="Session status"]');
    expect(panel?.textContent).toContain('Session is over');
    expect(panel?.textContent).not.toContain('Host ended your access');
  });

  it('describes each session_ended reason distinctly', () => {
    const root = document.createElement('div');
    const sessionState = createSessionState(root);

    sessionState.showSessionEnded('host_disconnected');
    expect(root.querySelector('[aria-label="Session status"]')?.textContent).toContain('host disconnected');

    sessionState.showSessionEnded('host_ended');
    expect(root.querySelector('[aria-label="Session status"]')?.textContent).toContain('host ended the session');
  });

  it('renders reason text as textContent, never innerHTML', () => {
    const root = document.createElement('div');
    const sessionState = createSessionState(root);

    sessionState.showKicked();

    const panel = root.querySelector('[aria-label="Session status"]');
    expect(panel?.innerHTML).not.toContain('<script>');
    expect(panel?.querySelector('script')).toBeNull();
  });
});
