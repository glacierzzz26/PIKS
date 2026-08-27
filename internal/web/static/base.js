/* PIKS Web — 主题切换(深浅色自适应,localStorage 记忆) */
(function () {
  const KEY = 'piks-theme';
  const btn = document.getElementById('themeToggle');
  if (!btn) return;

  const icons = { dark: '🌙', light: '☀️' };
  function apply(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    btn.textContent = icons[theme] || '🌙';
  }
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === 'light' || saved === 'dark') apply(saved);
  } catch (_) { /* 隐私模式等:跟随系统 */ }

  btn.addEventListener('click', () => {
    const cur = document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
    const next = cur === 'dark' ? 'light' : 'dark';
    apply(next);
    try { localStorage.setItem(KEY, next); } catch (_) {}
  });
})();
