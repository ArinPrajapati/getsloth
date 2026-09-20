import { createAuthGate } from './auth-gate';

describe('createAuthGate', () => {
  it('submits the entered password and display name', () => {
    const root = document.createElement('div');
    const submissions: Array<{ password: string; displayName?: string }> = [];

    createAuthGate(root, {
      onSubmit: (submission) => {
        submissions.push(submission);
      }
    });

    const name = root.querySelector<HTMLInputElement>('#display-name');
    const password = root.querySelector<HTMLInputElement>('#session-password');
    expect(name).not.toBeNull();
    expect(password).not.toBeNull();

    if (!name || !password) {
      throw new Error('Expected auth inputs to render');
    }

    name.value = 'Phone';
    password.value = 'secret';
    root.querySelector('form')?.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));

    expect(submissions).toEqual([{ displayName: 'Phone', password: 'secret' }]);
  });

  it('shows an auth failure without removing the form', () => {
    const root = document.createElement('div');
    const gate = createAuthGate(root, { onSubmit: () => undefined });

    gate.showError('Wrong password');

    expect(root.querySelector('[role="alert"]')?.textContent).toBe('Wrong password');
    expect(root.querySelector('form')).not.toBeNull();
  });

  it('auto-submits an initialPassword without requiring form interaction', () => {
    const root = document.createElement('div');
    const submissions: Array<{ password: string; displayName?: string }> = [];

    createAuthGate(root, {
      initialPassword: 'from-qr',
      initialDisplayName: 'Phone',
      onSubmit: (submission) => {
        submissions.push(submission);
      }
    });

    expect(submissions).toEqual([{ displayName: 'Phone', password: 'from-qr' }]);
  });

  it('still requires manual submission when no initialPassword is given', () => {
    const root = document.createElement('div');
    const submissions: unknown[] = [];

    createAuthGate(root, {
      onSubmit: (submission) => {
        submissions.push(submission);
      }
    });

    expect(submissions).toEqual([]);
  });
});
