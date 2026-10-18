import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

const dialogGlobals = [
  { name: "confirm", message: "Use a dialog." },
  { name: "alert", message: "Use a toast." },
  { name: "prompt", message: "Use a dialog." },
];
const storageGlobals = [
  { name: "localStorage", message: "Only src/lib/theme.ts keeps anything in storage." },
  { name: "sessionStorage", message: "Keep tokens in memory." },
];

export default tseslint.config(
  { ignores: ["dist", "node_modules", "public/config.js", "public/theme.js"] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ["**/*.{ts,tsx}"],
    languageOptions: { ecmaVersion: 2022, globals: globals.browser },
    plugins: { "react-hooks": reactHooks, "react-refresh": reactRefresh },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
      "@typescript-eslint/no-explicit-any": "error",
      // The access token lives in memory only (contracts/admin.md); the theme is the one thing kept in storage.
      "no-restricted-globals": ["error", ...dialogGlobals, ...storageGlobals],
      "no-restricted-properties": [
        "error",
        { object: "window", property: "localStorage", message: "Only src/lib/theme.ts keeps anything in storage." },
        { object: "window", property: "sessionStorage", message: "Keep tokens in memory." },
      ],
    },
  },
  {
    files: ["src/lib/theme.ts", "src/app/RouteStates.tsx"],
    rules: { "no-restricted-globals": ["error", ...dialogGlobals] },
  },
  {
    files: ["*.config.{js,ts}"],
    languageOptions: { globals: globals.node },
  },
);
