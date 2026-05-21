import { useQuery } from "@tanstack/react-query";
import { FolderKanban, LayoutDashboard, LogOut, Menu, Shield, X } from "lucide-react";
import { useState } from "react";
import { Link, NavLink, Outlet } from "react-router-dom";
import { api } from "../../lib/api";
import { initials } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import { useAuthStore } from "../../store/auth";
import { IconButton } from "../ui";

function navClass(isActive: boolean): string {
  return [
    "focus-ring flex items-center gap-3 rounded-md px-3 py-2 text-sm",
    isActive ? "bg-cyan-700 text-white" : "text-slate-600 hover:bg-slate-100 hover:text-slate-950",
  ].join(" ");
}

export function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);

  const { data: projects = [] } = useQuery({
    queryKey: queryKeys.projects,
    queryFn: api.listProjects,
  });

  const sidebar = (
    <aside className="flex h-full w-72 flex-col border-r border-slate-200 bg-white">
      <div className="flex h-16 items-center justify-between border-b border-slate-200 px-5">
        <Link to="/projects" className="focus-ring rounded text-base font-semibold text-slate-950">
          Project Mesh
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
        </div>

        <div>
          <div className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">Open Projects</div>
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

      <div className="border-t border-slate-200 p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-md bg-slate-900 text-sm font-semibold text-white">
            {initials(user?.email || user?.userId)}
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium text-slate-900">{user?.email || "Signed in"}</div>
            <div className="flex items-center gap-1 text-xs text-slate-500">
              <Shield size={12} />
              {user?.role || "worker"}
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
    <div className="min-h-screen bg-slate-50 text-slate-950">
      <div className="hidden md:fixed md:inset-y-0 md:left-0 md:block">{sidebar}</div>

      {sidebarOpen ? (
        <div className="fixed inset-0 z-40 md:hidden">
          <button
            type="button"
            aria-label="Close navigation overlay"
            className="absolute inset-0 bg-slate-950/40"
            onClick={() => setSidebarOpen(false)}
          />
          <div className="absolute inset-y-0 left-0">{sidebar}</div>
        </div>
      ) : null}

      <div className="md:pl-72">
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-slate-200 bg-white/90 px-4 backdrop-blur md:px-6">
          <div className="flex items-center gap-3">
            <IconButton label="Open navigation" className="md:hidden" onClick={() => setSidebarOpen(true)}>
              <Menu size={18} />
            </IconButton>
            <span className="text-sm font-medium text-slate-600">Workspace</span>
          </div>
          <div className="text-xs text-slate-500">{user?.userId.slice(0, 8)}</div>
        </header>

        <main className="px-4 py-5 md:px-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
