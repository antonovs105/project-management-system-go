import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import api from '@/lib/axios';
import { CreateProjectDialog } from '@/components/project/CreateProjectDialog';
import { Button } from '@/components/ui/button';
import { Card, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Plus, ArrowRight, Folder } from 'lucide-react';

interface Project {
    ID: number;
    Name: string;
    Description: string;
    CreatedAt: string;
}

export default function ProjectsPage() {
    const { data: projects, isLoading, error } = useQuery<Project[]>({
        queryKey: ['projects'],
        queryFn: async () => {
            const res = await api.get('/api/projects');
            return res.data;
        },
    });

    if (isLoading) return <div className="p-8">Loading projects...</div>;
    if (error) return <div className="p-8 text-red-500">Error loading projects</div>;

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <div>
                    <h2 className="text-3xl font-bold tracking-tight">Projects</h2>
                    <p className="text-muted-foreground">Manage your projects here.</p>
                </div>
                <CreateProjectDialog>
                    <Button className="gap-2">
                        <Plus size={16} /> New Project
                    </Button>
                </CreateProjectDialog>
            </div>

            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
                {projects?.map((project) => (
                    <Card key={project.ID} className="felx flex-col justify-between hover:shadow-md transition-shadow">
                        <CardHeader>
                            <div className="flex items-start justify-between">
                                <div className="bg-blue-100 p-2 rounded-lg text-blue-600 mb-2 w-fit">
                                    <Folder size={20} />
                                </div>
                            </div>
                            <CardTitle>{project.Name}</CardTitle>
                            <CardDescription className="line-clamp-2">
                                {project.Description || 'No description provided.'}
                            </CardDescription>
                        </CardHeader>
                        <CardFooter>
                            <Button asChild variant="outline" className="w-full justify-between group">
                                <Link to={`/projects/${project.ID}`}>
                                    View Board
                                    <ArrowRight size={16} className="text-slate-400 group-hover:text-slate-900 transition-colors" />
                                </Link>
                            </Button>
                        </CardFooter>
                    </Card>
                ))}

                {/* Empty State */}
                {projects?.length === 0 && (
                    <div className="col-span-full py-12 flex flex-col items-center justify-center text-center border-2 border-dashed rounded-lg bg-slate-50/50">
                        <div className="bg-slate-100 p-4 rounded-full mb-4">
                            <Folder size={32} className="text-slate-400" />
                        </div>
                        <h3 className="text-lg font-semibold">No projects yet</h3>
                        <p className="text-muted-foreground mb-4 max-w-sm">Create your first project to start managing tasks.</p>
                        <CreateProjectDialog>
                            <Button>Create Project</Button>
                        </CreateProjectDialog>
                    </div>
                )}
            </div>
        </div>
    );
}
