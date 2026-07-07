// usage-gauge client: theme follows the system by default, with an optional
// manual override (the toggle remembers the choice and then stops following).
(function () {
  "use strict";

  var root = document.documentElement;
  var MQ = "(prefers-color-scheme: dark)";
  var mq = window.matchMedia ? window.matchMedia(MQ) : null;

  function storedTheme() {
    try { return localStorage.getItem("theme"); } catch (e) { return null; }
  }

  function isSystemDark() {
    return !!(mq && mq.matches);
  }

  // Explicit user choice wins; otherwise follow the system.
  function apply() {
    var t = storedTheme();
    var dark = t === "dark" || (t !== "light" && isSystemDark());
    root.classList.toggle("dark", dark);
  }

  // Manual toggle: remember the choice so it stops tracking the system.
  window.toggleTheme = function () {
    var dark = !root.classList.contains("dark");
    root.classList.toggle("dark", dark);
    try { localStorage.setItem("theme", dark ? "dark" : "light"); } catch (e) {}
  };

  // Re-apply when the system theme changes, unless the user chose explicitly.
  if (mq) {
    var onChange = function () {
      var t = storedTheme();
      if (t !== "dark" && t !== "light") {
        root.classList.toggle("dark", mq.matches);
      }
    };
    if (mq.addEventListener) {
      mq.addEventListener("change", onChange);
    } else if (mq.addListener) { // older Safari
      mq.addListener(onChange);
    }
  }

  // Re-sync on load (the inline head script already prevents a flash).
  apply();
})();
