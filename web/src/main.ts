import { renderAppShell } from './app';

const root = document.querySelector<HTMLElement>('#app');

if (!root) {
  throw new Error('Missing #app root element');
}

renderAppShell(root);
