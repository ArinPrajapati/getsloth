export interface AuthSubmission {
  password: string;
  displayName?: string;
}

export interface AuthGate {
  showError(message: string): void;
  remove(): void;
}

export interface AuthGateOptions {
  onSubmit(submission: AuthSubmission): void;
  /**
   * Pre-fills and immediately submits the password, used when it arrived
   * via the QR code's `p=` fragment param instead of manual entry - see
   * docs/ideas/getsloth.md's "QR bypasses the password prompt" decision.
   * Manual entry stays the fallback: a wrong or stale password here still
   * surfaces through the normal showError path, form intact.
   */
  initialPassword?: string;
  initialDisplayName?: string;
}

export function createAuthGate(root: HTMLElement, options: AuthGateOptions): AuthGate {
  const panel = document.createElement('section');
  panel.className = 'auth-card';
  panel.setAttribute('aria-label', 'Session password');

  const title = document.createElement('h2');
  title.textContent = 'Enter session password';

  const description = document.createElement('p');
  description.textContent = 'The password is checked by the host process. The relay only forwards an encrypted attempt.';

  const alert = document.createElement('p');
  alert.className = 'auth-error';
  alert.setAttribute('role', 'alert');
  alert.hidden = true;

  const form = document.createElement('form');
  form.noValidate = true;

  const nameLabel = document.createElement('label');
  nameLabel.htmlFor = 'display-name';
  nameLabel.textContent = 'Display name';

  const nameInput = document.createElement('input');
  nameInput.id = 'display-name';
  nameInput.name = 'display-name';
  nameInput.autocomplete = 'name';
  nameInput.placeholder = 'Phone';

  const passwordLabel = document.createElement('label');
  passwordLabel.htmlFor = 'session-password';
  passwordLabel.textContent = 'Password';

  const passwordInput = document.createElement('input');
  passwordInput.id = 'session-password';
  passwordInput.name = 'session-password';
  passwordInput.type = 'password';
  passwordInput.autocomplete = 'current-password';
  passwordInput.required = true;

  const submit = document.createElement('button');
  submit.type = 'submit';
  submit.textContent = 'Join session';

  form.append(nameLabel, nameInput, passwordLabel, passwordInput, submit);
  panel.append(title, description, alert, form);
  root.prepend(panel);

  if (options.initialDisplayName) {
    nameInput.value = options.initialDisplayName;
  }
  if (options.initialPassword) {
    passwordInput.value = options.initialPassword;
  }

  function trySubmit(): void {
    const password = passwordInput.value;
    const displayName = nameInput.value.trim();

    if (!password) {
      alert.textContent = 'Enter the session password';
      alert.hidden = false;
      return;
    }

    options.onSubmit({ password, ...(displayName ? { displayName } : {}) });
  }

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    trySubmit();
  });

  if (options.initialPassword) {
    trySubmit();
  }

  return {
    showError(message: string): void {
      alert.textContent = message;
      alert.hidden = false;
    },
    remove(): void {
      panel.remove();
    }
  };
}
