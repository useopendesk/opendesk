import "./styles/app.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import { configureClient, noteActivity } from "./lib/api/client";
import { config } from "./lib/config";
import { initTheme } from "./lib/theme";
import { router } from "./router";

configureClient({ baseUrl: config.apiUrl });
initTheme();

// The session is kept alive while the operator works; an idle panel lets it lapse.
noteActivity();
for (const ev of ["pointerdown", "keydown"] as const) window.addEventListener(ev, () => noteActivity(), { passive: true, capture: true });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
