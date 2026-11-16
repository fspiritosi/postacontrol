import type { Settings } from "../../bindings/postacontrol/core/model/models";

export type ThemePref = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

const CACHE_KEY = "pc:theme";

function asPref(v: unknown): ThemePref {
  return v === "light" || v === "dark" ? v : "system";
}

// Configuración vieja, vacía o con un valor desconocido: sigue al sistema.
export function themePref(s: Settings | undefined): ThemePref {
  return asPref(s?.theme);
}

export function resolveTheme(pref: ThemePref, systemDark: boolean): ResolvedTheme {
  if (pref === "system") return systemDark ? "dark" : "light";
  return pref;
}

// Copia en el navegador del último tema elegido: se usa en el primer render,
// antes de que lleguen los ajustes, para que la ventana no parpadee.
export function loadCachedTheme(): ThemePref {
  try {
    return asPref(localStorage.getItem(CACHE_KEY));
  } catch {
    return "system";
  }
}

export function saveCachedTheme(p: ThemePref): void {
  try {
    localStorage.setItem(CACHE_KEY, p);
  } catch {
    // ignorar: la copia es solo para arrancar sin parpadeo
  }
}
