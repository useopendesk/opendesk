// Preload for the server picker page.

import { contextBridge, ipcRenderer } from "electron";

const info = ipcRenderer.sendSync("picker:info") as { platform: string; cloudOrigin: string; current: string | null; canCancel: boolean } | null;

contextBridge.exposeInMainWorld(
  "picker",
  Object.freeze({
    info: info ?? { platform: "linux", cloudOrigin: "https://app.opendesk.sh", current: null, canCancel: false },
    choose: (address: string) => ipcRenderer.invoke("picker:choose", String(address ?? "").slice(0, 2048)),
    cancel: () => ipcRenderer.send("picker:cancel"),
  }),
);
