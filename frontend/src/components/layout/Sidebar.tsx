import { Link, useLocation } from 'react-router-dom';
import { Settings, FolderKanban, Folder } from 'lucide-react';
import { Button } from "@/components/ui/button";
import { useQuery } from '@tanstack/react-query';
import api from '@/lib/axios';

interface Project {
    ID: number;
    Name: string;
}

export const Sidebar = () => {
    const location = useLocation();

    const { data: projects } = useQuery<Project[]>({
        queryKey: ['projects'],
        queryFn: async () => {
            const res = await api.get('/api/projects');
            return res.data;
        },
    });

    return (
        <aside className="w-64 bg-slate-950 text-slate-200 flex flex-col h-screen border-r border-slate-800">
            <div className="p-6 border-b border-slate-800">
                <h1 className="text-xl font-bold bg-gradient-to-r from-blue-400 to-purple-500 text-transparent bg-clip-text">
                    TaskMaster
                </h1>
            </div>

            <nav className="flex-1 p-4 space-y-2 overflow-y-auto">
                <Link
                    to="/"
                    className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${location.pathname === '/'
                        ? 'bg-blue-600 text-white'
                        : 'hover:bg-slate-800 text-slate-400'
                        }`}
                >
                    <FolderKanban size={20} />
                    <span>Projects</span>
                </Link>

                <div className="space-y-1 pl-4">
                    {projects?.map((project) => (
                        <Link
                            key={project.ID}
                            to={`/projects/${project.ID}`}
                            className={`flex items-center gap-3 px-4 py-2 text-sm rounded-lg transition-colors ${location.pathname === `/projects/${project.ID}`
                                ? 'bg-slate-800 text-blue-400'
                                : 'hover:bg-slate-900 text-slate-500 hover:text-slate-300'
                                }`}
                        >
                            <Folder size={16} />
                            <span className="truncate">{project.Name}</span>
                        </Link>
                    ))}
                </div>
            </nav>

            <div className="p-4 border-t border-slate-800">
                <Button variant="outline" className="w-full gap-2 text-slate-900">
                    <Settings size={16} /> Settings
                </Button>
            </div>
        </aside>
    );
};
