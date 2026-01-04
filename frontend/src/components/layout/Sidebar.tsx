import { Link, useLocation } from 'react-router-dom';
import { Settings, FolderKanban } from 'lucide-react';
import { Button } from "@/components/ui/button";

export const Sidebar = () => {
    const location = useLocation();

    const menuItems = [
        { path: '/', icon: <FolderKanban size={20} />, label: 'Projects' },
    ];

    return (
        <aside className="w-64 bg-slate-950 text-slate-200 flex flex-col h-screen border-r border-slate-800">
            <div className="p-6 border-b border-slate-800">
                <h1 className="text-xl font-bold bg-gradient-to-r from-blue-400 to-purple-500 text-transparent bg-clip-text">
                    TaskMaster
                </h1>
            </div>

            <nav className="flex-1 p-4 space-y-2">
                {menuItems.map((item) => (
                    <Link
                        key={item.path}
                        to={item.path}
                        className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${location.pathname === item.path
                            ? 'bg-blue-600 text-white'
                            : 'hover:bg-slate-800 text-slate-400'
                            }`}
                    >
                        {item.icon}
                        <span>{item.label}</span>
                    </Link>
                ))}
            </nav>

            <div className="p-4 border-t border-slate-800">
                <Button variant="outline" className="w-full gap-2 text-slate-900">
                    <Settings size={16} /> Settings
                </Button>
            </div>
        </aside>
    );
};
