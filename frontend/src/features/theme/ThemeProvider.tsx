import { useLayoutEffect, type ReactNode } from "react";

// Sigue prefers-color-scheme y alterna la clase `dark` en <html>. Sin interruptor
// manual en la Fase 1. El Toaster usa theme="system" de sonner, que lee la misma
// media query, así que los dos coinciden sin un segundo sistema de temas.
export function ThemeProvider({ children }: { children: ReactNode }) {
  useLayoutEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.classList.toggle("dark", mq.matches);
      document.documentElement.style.colorScheme = mq.matches ? "dark" : "light";
    };
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);
  return <>{children}</>;
}
