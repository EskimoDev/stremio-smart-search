// Runs in <head> before the page paints, so a saved light or dark choice never flashes.
// "system" follows the operating system through the stylesheet's prefers-color-scheme rules.
(() => {
  const key = 'sss-theme';
  const modes = ['system', 'light', 'dark'];
  const labels = { system: 'Auto', light: 'Light', dark: 'Dark' };
  const colours = { light: '#ffffff', dark: '#0a0a0a' };
  const root = document.documentElement;
  const read = () => { try { const v = localStorage.getItem(key); return modes.includes(v) ? v : 'system'; } catch { return 'system'; } };
  const apply = (mode) => {
    if (mode === 'system') root.removeAttribute('data-theme'); else root.setAttribute('data-theme', mode);
    document.querySelectorAll('meta[name="theme-color"]').forEach((meta) => {
      meta.content = mode === 'system' ? colours[meta.dataset.scheme] : colours[mode];
    });
    const button = document.getElementById('theme-toggle');
    if (button) {
      const next = modes[(modes.indexOf(mode) + 1) % modes.length];
      button.dataset.mode = mode;
      button.querySelector('.theme-label').textContent = labels[mode];
      button.setAttribute('aria-label', `Theme: ${labels[mode] === 'Auto' ? 'match system' : labels[mode]}. Switch to ${next === 'system' ? 'match system' : next}.`);
      button.title = mode === 'system' ? 'Theme follows your system setting' : `${labels[mode]} theme`;
    }
  };
  apply(read());
  document.addEventListener('DOMContentLoaded', () => {
    const button = document.getElementById('theme-toggle');
    apply(read());
    button.addEventListener('click', () => {
      const mode = modes[(modes.indexOf(read()) + 1) % modes.length];
      try { if (mode === 'system') localStorage.removeItem(key); else localStorage.setItem(key, mode); } catch {}
      apply(mode);
    });
  });
  // Keep other open settings tabs in step.
  window.addEventListener('storage', (event) => { if (event.key === key) apply(read()); });
})();
