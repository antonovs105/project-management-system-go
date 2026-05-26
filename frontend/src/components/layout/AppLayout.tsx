import { useQuery } from "@tanstack/react-query";
import {
  CircleDot,
  FolderKanban,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  RadioTower,
  ScrollText,
  Shield,
  Users,
  X,
} from "lucide-react";
import { useState } from "react";
import { Link, NavLink, Outlet } from "react-router-dom";
import { api } from "../../lib/api";
import { initials } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import { useAuthStore } from "../../store/auth";
import { IconButton } from "../ui";

function navClass(isActive: boolean): string {
  return [
    "focus-ring flex items-center gap-3 rounded-2xl px-3 py-2 text-sm transition",
    isActive ? "bg-zinc-950 text-white shadow-sm" : "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-950",
  ].join(" ");
}

export function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);
  const canUseAdmin = user?.instanceRole === "owner" || user?.instanceRole === "admin";

  const { data: projects = [] } = useQuery({
    queryKey: queryKeys.projects,
    queryFn: api.listProjects,
  });

  const sidebar = (
    <aside className="flex h-full w-72 flex-col border-r border-zinc-200 bg-white/95 backdrop-blur">
      <div className="flex h-16 items-center justify-between border-b border-zinc-200 px-5">
        <Link to="/projects" className="focus-ring flex items-center gap-2 rounded-2xl text-base font-semibold text-zinc-950">
          <span className="flex h-8 w-8 items-center justify-center rounded-2xl bg-zinc-950 text-white">
            <CircleDot size={17} />
          </span>
          TaskFlow
        </Link>
        <IconButton label="Close navigation" className="md:hidden" onClick={() => setSidebarOpen(false)}>
          <X size={18} />
        </IconButton>
      </div>

      <nav className="flex-1 space-y-6 overflow-auto px-3 py-4">
        <div className="space-y-1">
          <NavLink to="/projects" className={({ isActive }) => navClass(isActive)} onClick={() => setSidebarOpen(false)}>
            <LayoutDashboard size={18} />
            Projects
          </NavLink>
          <NavLink to="/federation" className={({ isActive }) => navClass(isActive)} onClick={() => setSidebarOpen(false)}>
            <RadioTower size={18} />
            Federation
          </NavLink>
          <NavLink to="/account" className={({ isActive }) => navClass(isActive)} onClick={() => setSidebarOpen(false)}>
            <KeyRound size={18} />
            Account
          </NavLink>
        </div>

        {canUseAdmin ? (
          <div>
            <div className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-zinc-400">Administration</div>
            <div className="space-y-1">
              <NavLink to="/admin/users" className={({ isActive }) => navClass(isActive)} onClick={() => setSidebarOpen(false)}>
                <Users size={18} />
                Users
              </NavLink>
              <NavLink
                to="/admin/federation"
                className={({ isActive }) => navClass(isActive)}
                onClick={() => setSidebarOpen(false)}
              >
                <RadioTower size={18} />
                Federation
              </NavLink>
              <NavLink to="/admin/audit" className={({ isActive }) => navClass(isActive)} onClick={() => setSidebarOpen(false)}>
                <ScrollText size={18} />
                Audit
              </NavLink>
            </div>
          </div>
        ) : null}

        <div>
          <div className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-zinc-400">Open Projects</div>
          <div className="space-y-1">
            {projects.map((project) => (
              <NavLink
                key={project.id}
                to={`/projects/${project.id}`}
                className={({ isActive }) => navClass(isActive)}
                onClick={() => setSidebarOpen(false)}
              >
                <FolderKanban size={18} />
                <span className="truncate">{project.name}</span>
              </NavLink>
            ))}
          </div>
        </div>
      </nav>

      <div className="border-t border-zinc-200 p-4">
        <div className="flex items-center gap-3 rounded-3xl border border-zinc-200 bg-zinc-50 p-2">
          <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-zinc-950 text-sm font-semibold text-white">
            {initials(user?.email || user?.userId)}
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium text-zinc-950">{user?.email || "Signed in"}</div>
            <div className="flex items-center gap-1 text-xs text-zinc-500">
              <Shield size={12} />
              {user?.instanceRole || "user"}
            </div>
          </div>
          <IconButton label="Log out" onClick={logout}>
            <LogOut size={18} />
          </IconButton>
        </div>
      </div>
    </aside>
  );

  return (
    <div className="min-h-screen bg-zinc-100 text-zinc-950">
      <div className="hidden md:fixed md:inset-y-0 md:left-0 md:block">{sidebar}</div>

      {sidebarOpen ? (
        <div className="fixed inset-0 z-40 md:hidden">
          <button
            type="button"
            aria-label="Close navigation overlay"
            className="absolute inset-0 bg-zinc-950/40 backdrop-blur-sm"
            onClick={() => setSidebarOpen(false)}
          />
          <div className="absolute inset-y-0 left-0">{sidebar}</div>
        </div>
      ) : null}

      <div className="md:pl-72">
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-zinc-200 bg-white/80 px-4 backdrop-blur md:px-6">
          <div className="flex items-center gap-3">
            <IconButton label="Open navigation" className="md:hidden" onClick={() => setSidebarOpen(true)}>
              <Menu size={18} />
            </IconButton>
            <span className="text-sm font-medium text-zinc-500">Workspace</span>
          </div>
          <div className="rounded-full border border-zinc-200 bg-white px-2.5 py-1 text-xs text-zinc-500 shadow-sm">
            {user?.userId.slice(0, 8)}
          </div>
        </header>

        <main className="px-4 py-5 md:px-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
