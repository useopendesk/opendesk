// Runs before the page paints: the reader's theme, and their language when the address
// names none (a choice made here before, else the browser's). Crawlers keep the page's
// own language, so every address keeps one meaning.
(function () {
  var d = document.documentElement;
  try {
    var t = localStorage.getItem("od-theme");
    if (t === "dark" || ((!t || t === "system") && matchMedia("(prefers-color-scheme: dark)").matches)) d.classList.add("dark");
  } catch (e) {}
  try {
    if (/bot|crawl|spider|slurp|preview|lighthouse/i.test(navigator.userAgent) || navigator.webdriver) return;
    var u = new URL(location.href);
    if (u.searchParams.has("lang") || u.searchParams.has("q")) return;
    var langs = (d.getAttribute("data-langs") || "").split(" ");
    var match = function (tag) {
      if (!tag) return "";
      var x = tag.toLowerCase().replace("_", "-");
      for (var i = 0; i < langs.length; i++) if (langs[i].toLowerCase() === x) return langs[i];
      var base = x.split("-")[0];
      if (base === "zh") return /hant|-tw$|-hk$/.test(x) ? "" : langs.indexOf("zh-CN") >= 0 ? "zh-CN" : "";
      return langs.indexOf(base) >= 0 ? base : "";
    };
    var want = "";
    try {
      want = match(localStorage.getItem("od-help-lang"));
    } catch (e) {}
    if (!want) {
      var nav = navigator.languages || [navigator.language];
      for (var j = 0; j < nav.length && !want; j++) want = match(nav[j]);
    }
    if (want && want !== d.lang) {
      u.searchParams.set("lang", want);
      location.replace(u.href);
    }
  } catch (e) {}
})();
