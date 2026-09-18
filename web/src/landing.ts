import './styles.css';

export function mountLandingPage(root: HTMLElement): void {
  root.innerHTML = '';

  const page = document.createElement('main');
  page.className = 'landing';

  const hero = document.createElement('section');
  hero.className = 'landing-hero';
  hero.innerHTML = `
    <p class="landing-eyebrow">Live agent session viewer</p>
    <h1>Sloth</h1>
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
  shots.innerHTML = `
    <figure class="landing-shot landing-shot-desktop">
      <div class="device-frame device-frame-mac">
        <div class="device-frame-mac-screen">
          <span class="device-frame-mac-camera"></span>
          <img src="/screenshots/desktop-busy.png" alt="Sloth viewer showing a live agent session on desktop" loading="lazy" />
        </div>
        <div class="device-frame-mac-base"></div>
      </div>
    </figure>
    <figure class="landing-shot landing-shot-mobile">
      <div class="device-frame device-frame-phone">
        <span class="device-frame-phone-button device-frame-phone-mute"></span>
        <span class="device-frame-phone-button device-frame-phone-volume-up"></span>
        <span class="device-frame-phone-button device-frame-phone-volume-down"></span>
        <span class="device-frame-phone-button device-frame-phone-power"></span>
        <div class="device-frame-phone-screen">
          <div class="device-frame-phone-island"></div>
          <img src="/screenshots/mobile-busy.png" alt="Sloth viewer showing a live agent session on a phone" loading="lazy" />
        </div>
      </div>
    </figure>
  `;

  const how = document.createElement('section');
  how.className = 'landing-how';
  how.innerHTML = `
    <h2>How it works</h2>
    <ol>
      <li>Run <code>sloth claude</code> (or any agent, or a plain shell) in front of whatever you want to control remotely.</li>
      <li>It prints a share URL and a separate password.</li>
      <li>Open the URL on your phone, enter the password, and you're in &mdash; watching or taking control instantly.</li>
    </ol>
    <p class="landing-note">
      The password is checked by your own machine, not by the relay server &mdash;
      the relay never sees it. Works the same way for handing control back and
      forth between a team, which is a secondary use case, not the headline one.
    </p>
  `;

  const footer = document.createElement('footer');
  footer.className = 'landing-footer';
  footer.innerHTML = `
    <a href="https://github.com/arinprajapati/getsloth">GitHub</a>
    <span>&middot;</span>
    <span>AGPL-3.0</span>
  `;

  page.append(hero, shots, how, footer);
  root.append(page);
}
