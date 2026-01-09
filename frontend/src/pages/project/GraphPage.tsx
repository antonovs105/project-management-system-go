import { useRef, useState, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import ForceGraph2D from 'react-force-graph-2d';
import api from '@/lib/axios';

interface GraphNode {
    id: number;
    label: string;
    type: string;
    status: string;
    priority: string;
    group: string;
    x?: number;
    y?: number;
}

interface GraphLink {
    source: number | GraphNode;
    target: number | GraphNode;
    type: string;
}

interface GraphData {
    nodes: GraphNode[];
    links: GraphLink[];
}

export default function GraphPage() {
    const { projectId } = useParams();
    const containerRef = useRef<HTMLDivElement>(null);
    const [dimensions, setDimensions] = useState({ width: 1, height: 1 });

    const { data: graphData, isLoading } = useQuery<GraphData>({
        queryKey: ['graph', projectId],
        queryFn: async () => {
            const res = await api.get(`/api/projects/${projectId}/graph`);
            return res.data;
        },
        enabled: !!projectId,
    });

    useEffect(() => {
        if (!containerRef.current) return;

        // Initial measure
        setDimensions({
            width: containerRef.current.offsetWidth,
            height: containerRef.current.offsetHeight
        });

        const resizeObserver = new ResizeObserver((entries) => {
            for (const entry of entries) {
                // Use getBoundingClientRect or contentRect. 
                // offsetWidth/Height is often safer for "visual size" including padding/borders which we want for the canvas container
                if (entry.target instanceof HTMLElement) {
                    setDimensions({
                        width: entry.target.offsetWidth,
                        height: entry.target.offsetHeight,
                    });
                }
            }
        });

        resizeObserver.observe(containerRef.current);

        return () => resizeObserver.disconnect();
    }, []);

    const getNodeColor = (type: string) => {
        switch (type) {
            case 'epic': return '#9333ea'; // Purple
            case 'task': return '#2563eb'; // Blue
            case 'subtask': return '#64748b'; // Slate
            default: return '#9ca3af';
        }
    };

    const paintNode = useCallback((node: any, ctx: CanvasRenderingContext2D, globalScale: number) => {
        const label = node.label;
        const fontSize = 12 / globalScale;
        const radius = 5;

        // Node circle
        ctx.beginPath();
        ctx.arc(node.x, node.y, radius, 0, 2 * Math.PI, false);
        ctx.fillStyle = getNodeColor(node.type);
        ctx.fill();

        // Text
        ctx.font = `${fontSize}px Sans-Serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = 'rgba(0, 0, 0, 0.8)';
        ctx.fillText(label, node.x, node.y + radius + fontSize);
    }, []);

    if (isLoading) return <div className="p-8">Loading graph...</div>;

    return (
        <div className="h-full flex flex-col bg-slate-50">
            <div className="h-12 border-b flex items-center px-4 bg-white">
                <h2 className="font-semibold text-slate-700">Project Graph</h2>
                <div className="ml-4 flex gap-4 text-xs">
                    <div className="flex items-center gap-1"><div className="w-3 h-3 rounded-full bg-purple-600"></div>Epic</div>
                    <div className="flex items-center gap-1"><div className="w-3 h-3 rounded-full bg-blue-600"></div>Task</div>
                    <div className="flex items-center gap-1"><div className="w-3 h-3 rounded-full bg-slate-500"></div>Subtask</div>
                </div>
            </div>
            <div className="flex-1 w-full h-full overflow-hidden relative" ref={containerRef}>
                {graphData && (
                    <ForceGraph2D
                        width={dimensions.width}
                        height={dimensions.height}
                        graphData={graphData}
                        nodeLabel="label"
                        nodeCanvasObject={paintNode}
                        linkDirectionalArrowLength={3.5}
                        linkDirectionalArrowRelPos={1}
                        linkCurvature={0.25}
                        backgroundColor="#f8fafc"
                        linkColor={() => '#94a3b8'}
                    />
                )}
            </div>
        </div>
    );
}
