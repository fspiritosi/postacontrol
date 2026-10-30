import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist", "bindings", "node_modules"] },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: { globals: globals.browser },
    plugins: { "react-hooks": reactHooks },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // `shadcn add` suele generar `import { cn } from "cn"` (paquete ajeno).
      "no-restricted-imports": [
        "error",
        { paths: [{ name: "cn", message: "Usá cn() de @/lib/utils" }] },
      ],
    },
  },
  {
    // Componentes generados por shadcn: exportan variantes junto al componente.
    files: ["src/components/ui/**"],
    rules: { "@typescript-eslint/no-unused-vars": "off" },
  },
);
