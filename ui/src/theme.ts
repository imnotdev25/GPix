// Light / dark / system theme. "system" follows the OS; the other two set
// data-theme on <html>, which styles.css keys off. index.html applies the saved
// choice before the first paint so there's no flash.

export type ThemePref = "system" | "light" | "dark";

const KEY = "gpix-theme";

export function getThemePref(): ThemePref {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    /* storage unavailable */
  }
  return "system";
}

export function setThemePref(p: ThemePref) {
  try {
    if (p === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, p);
  } catch {
    /* storage unavailable: still apply for this page view */
  }
  const el = document.documentElement;
  if (p === "system") el.removeAttribute("data-theme");
  else el.setAttribute("data-theme", p);
}
