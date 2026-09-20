import { mountLandingPage } from './landing';

describe('mountLandingPage', () => {
  it('renders a single-page marketing view with install and GitHub links, no pricing', () => {
    const root = document.createElement('div');

    mountLandingPage(root);

    expect(root.querySelector('h1')?.textContent).toBe('Sloth');
    expect(root.textContent).toContain('Control an AI coding agent session from your phone.');
    expect(root.querySelector('.landing-install')?.textContent).toContain('install.sh');
    expect(root.querySelectorAll('a[href="https://github.com/arinprajapati/getsloth"]').length).toBeGreaterThan(0);
    expect(root.querySelectorAll('img').length).toBe(2);
    expect(root.textContent.toLowerCase()).not.toContain('pricing');
    expect(root.textContent.toLowerCase()).not.toContain('testimonial');
  });

  it('explains the QR code as the no-typing path, and the plain link as still requiring the password', () => {
    const root = document.createElement('div');

    mountLandingPage(root);

    const howItWorks = root.querySelector('.landing-how')?.textContent ?? '';
    expect(howItWorks.toLowerCase()).toContain('qr code');
    expect(howItWorks.toLowerCase()).toContain('scan');
    expect(howItWorks).toContain('password');
  });

  it('clears any previously rendered content on remount', () => {
    const root = document.createElement('div');
    root.innerHTML = '<p>stale</p>';

    mountLandingPage(root);

    expect(root.textContent).not.toContain('stale');
  });
});
