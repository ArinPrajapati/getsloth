import { mountLandingPage } from './landing';

describe('mountLandingPage', () => {
  it('renders a single-page marketing view with install and GitHub links, no pricing', () => {
    const root = document.createElement('div');

    mountLandingPage(root);

    expect(root.querySelector('h1')?.textContent).toBe('Sloth');
    expect(root.textContent).toContain('Control an AI coding agent session from your phone.');
    expect(root.querySelector('.landing-install')?.textContent).toContain('go install');
    expect(root.querySelectorAll('a[href="https://github.com/arinprajapati/getsloth"]').length).toBeGreaterThan(0);
    expect(root.querySelectorAll('img').length).toBe(2);
    expect(root.textContent.toLowerCase()).not.toContain('pricing');
    expect(root.textContent.toLowerCase()).not.toContain('testimonial');
  });

  it('clears any previously rendered content on remount', () => {
    const root = document.createElement('div');
    root.innerHTML = '<p>stale</p>';

    mountLandingPage(root);

    expect(root.textContent).not.toContain('stale');
  });
});
