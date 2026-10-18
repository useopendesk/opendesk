// Applies the saved theme before first paint so a dark panel never flashes
// white. Same key and rules as src/lib/theme.ts: follows the system unless a
// theme was chosen.
(function () {
  var t = "system";
  try {
    t = localStorage.getItem("od-admin-theme") || "system";
  } catch (e) {}
  if (t !== "dark" && t !== "light") t = "system";
  var dark = t === "dark" || (t === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  var root = document.documentElement;
  root.classList.toggle("dark", dark);
  root.style.colorScheme = dark ? "dark" : "light";
  root.dataset.themePref = t;
})();
