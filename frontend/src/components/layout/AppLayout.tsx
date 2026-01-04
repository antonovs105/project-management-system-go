import { Outlet } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { useAuthStore } from '@/store/auth';
import { Button } from '@/components/ui/button';
import { LogOut } from 'lucide-react';

export const AppLayout = () => {
    const user = useAuthStore((state) => state.user);
    const logout = useAuthStore((state) => state.logout);

    return (
        <div className="flex h-screen bg-background overflow-hidden">
            <Sidebar />

            <main className="flex-1 flex flex-col h-screen overflow-hidden">
                <header className="h-16 border-b flex items-center px-6 bg-white justify-between">
                    <h2 className="font-semibold text-lg text-slate-700">Project Management System</h2>
                    <div className="flex items-center gap-4">
                        <div className="flex items-center gap-2">
                            <span className="text-sm text-slate-500">{user?.email || 'User'}</span>
                            <div className="w-8 h-8 bg-blue-500 rounded-full flex items-center justify-center text-white text-xs">
                                {user?.username?.[0]?.toUpperCase() || 'U'}
                            </div>
                        </div>
                        <Button variant="ghost" size="icon" onClick={logout} title="Logout">
                            <LogOut size={18} />
                        </Button>
                    </div>
                </header>

                <div className="flex-1 p-6 overflow-auto bg-slate-50">
                    <Outlet />
                </div>
            </main>
        </div>
    );
};
