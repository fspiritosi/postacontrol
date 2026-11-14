import { createContext, useContext, useEffect, useLayoutEffect, useState, type ReactNode } from "react";
import { useSettings } from "@/lib/queries";
import {
  loadCachedTheme,
  resolveTheme,
  saveCachedTheme,
  themePref,
  type ResolvedTheme,
} from "@/lib/theme";

const ResolvedThemeContext = createContext<ResolvedTheme>("light");

// El tema que se ve (claro u oscuro), ya resuelto desde la preferencia.
export function useResolvedTheme(): ResolvedTheme {
  return useContext(ResolvedThemeContext);
}

const darkQuery = () => window.matchMedia("(prefers-color-scheme: dark)");

// Aplica el tema elegido en la configuración (claro, oscuro o el del sistema)
// alternando la clase `dark` en <html>. Hasta que llegan los ajustes usa la
// copia guardada en el navegador, para no arrancar con el tema equivocado.
// Con "sistema" sigue escuchando prefers-color-scheme. Va adentro del
// QueryClientProvider: lee los ajustes con useSettings.
export function ThemeProvider({ children }: { children: ReactNode }) {
  const settings = useSettings();
  const pref = settings.data ? themePref(settings.data) : loadCachedTheme();
  const [systemDark, setSystemDark] = useState(() => darkQuery().matches);

  useEffect(() => {
    if (pref !== "system") return;
    const mq = darkQuery();
    const onChange = () => setSystemDark(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [pref]);

  useEffect(() => {
    if (settings.data) saveCachedTheme(themePref(settings.data));
  }, [settings.data]);

  const resolved = resolveTheme(pref, systemDark);
  useLayoutEffect(() => {
    document.documentElement.classList.toggle("dark", resolved === "dark");
    document.documentElement.style.colorScheme = resolved;
  }, [resolved]);

  return <ResolvedThemeContext.Provider value={resolved}>{children}</ResolvedThemeContext.Provider>;
}
