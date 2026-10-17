// Applies the saved theme before first paint (kept external for the CSP).
try {
  var t = JSON.parse(localStorage.getItem('wh:prefs') || '{}').theme;
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t;
} catch (e) {}
