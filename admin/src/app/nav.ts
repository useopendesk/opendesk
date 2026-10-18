import { Activity, Building2, LayoutDashboard, ListChecks, ScrollText, ShieldCheck, Users, type LucideIcon } from "lucide-react";

export type NavItem = { to: "/" | "/workspaces" | "/users" | "/jobs" | "/health" | "/audit" | "/admins"; label: string; icon: LucideIcon; keys: string };

export const NAV: NavItem[] = [
  { to: "/", label: "Overview", icon: LayoutDashboard, keys: "g o" },
  { to: "/workspaces", label: "Workspaces", icon: Building2, keys: "g w" },
  { to: "/users", label: "Users", icon: Users, keys: "g u" },
  { to: "/jobs", label: "Jobs", icon: ListChecks, keys: "g j" },
  { to: "/health", label: "Health", icon: Activity, keys: "g h" },
  { to: "/audit", label: "Audit log", icon: ScrollText, keys: "g l" },
  { to: "/admins", label: "Admins", icon: ShieldCheck, keys: "g a" },
];

export const SHORTCUTS: { keys: string; label: string }[] = [
  { keys: "/", label: "Search this page" },
  ...NAV.map((n) => ({ keys: n.keys, label: `Go to ${n.label.toLowerCase()}` })),
  { keys: "?", label: "Keyboard shortcuts" },
];
