import './styles.css';
import './landing.css';

export function mountLandingPage(root: HTMLElement): void {
  root.innerHTML = '';
  document.title = 'Sloth | Control an AI coding agent from your phone';
  setMetaDescription('Control a live AI coding agent session from your phone with Sloth. No SSH keys or app install required.');

  const page = document.createElement('main');
  page.className = 'landing';

  const hero = document.createElement('section');
  hero.className = 'landing-hero';
  hero.setAttribute('aria-labelledby', 'landing-title');
  hero.innerHTML = `
    <p class="landing-eyebrow">Live agent session viewer</p>
    <h1 id="landing-title">Sloth</h1>
    <p class="landing-tagline">Control an AI coding agent session from your phone.</p>
    <p class="landing-lede">
      Start an AI coding agent on your laptop, walk away, and keep
      watching or steering it from your phone &mdash; no SSH keys, no
      app install, nothing to configure on the viewing device. Works
      with any agent, or any terminal command at all.
    </p>
    <div class="landing-cta">
      <a class="landing-button primary" href="https://github.com/arinprajapati/getsloth">Get the code</a>
      <code class="landing-install">curl -fsSL https://getsloth.dev/install.sh | sh</code>
    </div>
  `;

  const shots = document.createElement('section');
  shots.className = 'landing-shots';
  shots.setAttribute('aria-label', 'Sloth on desktop and mobile');
  shots.innerHTML = `
    <figure class="landing-shot landing-shot-desktop">
      <div class="device-frame device-frame-mac">
        <div class="device-frame-mac-screen">
          <span class="device-frame-mac-camera" aria-hidden="true"></span>
          <img src="/screenshots/desktop-busy.png" alt="Sloth viewer showing a live agent session on desktop" width="1440" height="900" loading="lazy" decoding="async" />
        </div>
        <div class="device-frame-mac-base"></div>
      </div>
    </figure>
    <figure class="landing-shot landing-shot-mobile">
      <div class="device-frame device-frame-phone">
        <span class="device-frame-phone-button device-frame-phone-mute" aria-hidden="true"></span>
        <span class="device-frame-phone-button device-frame-phone-volume-up" aria-hidden="true"></span>
        <span class="device-frame-phone-button device-frame-phone-volume-down" aria-hidden="true"></span>
        <span class="device-frame-phone-button device-frame-phone-power" aria-hidden="true"></span>
        <div class="device-frame-phone-screen">
          <div class="device-frame-phone-island" aria-hidden="true"></div>
          <img src="/screenshots/mobile-busy.png" alt="Sloth viewer showing a live agent session on a phone" width="402" height="874" loading="lazy" decoding="async" />
        </div>
      </div>
    </figure>
  `;

  const how = document.createElement('section');
  how.className = 'landing-how';
  how.setAttribute('aria-labelledby', 'landing-how-title');
  how.innerHTML = `
    <h2 id="landing-how-title">How it works</h2>
    <ol>
      <li>Run <code>sloth claude</code> (or any agent, or a plain shell) in front of whatever you want to control remotely.</li>
      <li>It prints a share URL, a password &mdash; and a QR code right there in your terminal.</li>
      <li>Scan the QR code with your phone and you're straight in, no typing &mdash; watching or taking control instantly.</li>
    </ol>
    <p class="landing-note">
      The password is checked by your own machine, not by the relay server &mdash;
      the relay never sees it. Scanning the QR skips typing it in only because
      you're already looking at it on the same terminal; if you send the link
      itself to someone else instead, they'll still need the password. Works
      the same way for handing control back and forth between a team, which is
      a secondary use case, not the headline one.
    </p>
  `;

  const footer = document.createElement('footer');
  footer.className = 'landing-footer';
  footer.setAttribute('aria-label', 'Site footer');
  footer.innerHTML = `
    <a href="https://github.com/arinprajapati/getsloth">GitHub</a>
    <span>&middot;</span>
    <span>AGPL-3.0</span>
  `;

  page.append(hero, shots, how, footer);
  root.append(page);
}

function setMetaDescription(content: string): void {
  let description = document.querySelector<HTMLMetaElement>('meta[name="description"]');
  if (!description) {
    description = document.createElement('meta');
    description.name = 'description';
    document.head.append(description);
  }
  description.content = content;
}
