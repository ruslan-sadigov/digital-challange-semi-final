// Dark / light theme toggle (the initial theme is set inline in <head> to avoid a flash).
function currentTheme() { return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light'; }

function setTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  try { localStorage.setItem('theme', theme); } catch (_) {}
  syncThemeButton();
  window.dispatchEvent(new CustomEvent('themechange', { detail: theme }));
}

function syncThemeButton() {
  const b = document.getElementById('theme-toggle');
  if (!b) return;
  const next = currentTheme() === 'dark' ? 'light' : 'dark';
  b.dataset.i18nTitle = 'theme.' + next;
  b.title = typeof t === 'function' ? t('theme.' + next) : next;
  b.setAttribute('aria-label', b.title);
}

// Map colors follow the theme tokens.
function cssVar(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }

document.addEventListener('click', e => {
  if (e.target.closest('#theme-toggle')) setTheme(currentTheme() === 'dark' ? 'light' : 'dark');
});
window.addEventListener('langchange', syncThemeButton);
syncThemeButton();
