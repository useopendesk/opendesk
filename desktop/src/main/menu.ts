// The application menu: the standard edit, view and window menus, plus Switch
// server, Check for updates and Get beta updates.

import { app, Menu, shell } from "electron";
import { isDev } from "./env";
import type { UpdateChannel } from "./store";
import type { UpdateState } from "./updater";

export type MenuActions = {
  switchServer: () => void;
  checkForUpdates: () => void;
  restartToUpdate: () => void;
  updateState: () => UpdateState;
  updateChannel: () => UpdateChannel;
  setUpdateChannel: (c: UpdateChannel) => void;
  reload: () => void;
};

function updateItem(a: MenuActions): Electron.MenuItemConstructorOptions {
  const s = a.updateState();
  switch (s.status) {
    case "ready":
      return { label: `Restart to update to ${s.version}`, click: a.restartToUpdate };
    case "available":
    case "downloading":
      return { label: "Downloading update…", enabled: false };
    case "checking":
      return { label: "Checking for updates…", enabled: false };
    default:
      return { label: "Check for updates…", click: a.checkForUpdates };
  }
}

export function buildMenu(a: MenuActions) {
  const mac = process.platform === "darwin";
  const updates: Electron.MenuItemConstructorOptions[] = [
    updateItem(a),
    {
      label: "Get beta updates",
      type: "checkbox",
      checked: a.updateChannel() === "beta",
      click: (item) => a.setUpdateChannel(item.checked ? "beta" : "stable"),
    },
  ];
  const switchServer: Electron.MenuItemConstructorOptions = { label: "Switch server…", accelerator: "CmdOrCtrl+Shift+O", click: a.switchServer };

  const template: Electron.MenuItemConstructorOptions[] = [
    ...(mac
      ? [
          {
            label: app.name,
            submenu: [
              { role: "about" },
              ...updates,
              { type: "separator" },
              switchServer,
              { type: "separator" },
              { role: "services" },
              { type: "separator" },
              { role: "hide" },
              { role: "hideOthers" },
              { role: "unhide" },
              { type: "separator" },
              { role: "quit" },
            ],
          } satisfies Electron.MenuItemConstructorOptions,
        ]
      : []),
    {
      label: "File",
      submenu: mac ? [{ role: "close" }] : [switchServer, { type: "separator" }, { role: "quit" }],
    },
    { role: "editMenu" },
    {
      label: "View",
      submenu: [
        { label: "Reload", accelerator: "CmdOrCtrl+R", click: a.reload },
        ...(isDev ? ([{ role: "forceReload" }, { role: "toggleDevTools" }] as Electron.MenuItemConstructorOptions[]) : []),
        { type: "separator" },
        { role: "resetZoom" },
        { role: "zoomIn" },
        { role: "zoomOut" },
        { type: "separator" },
        { role: "togglefullscreen" },
      ],
    },
    { role: "windowMenu" },
    {
      role: "help",
      submenu: [
        { label: "Documentation", click: () => void shell.openExternal("https://docs.opendesk.sh") },
        { label: "Report an issue", click: () => void shell.openExternal("https://github.com/useopendesk/opendesk/issues") },
        ...(mac ? [] : ([{ type: "separator" }, ...updates, { role: "about" }] as Electron.MenuItemConstructorOptions[])),
      ],
    },
  ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}
