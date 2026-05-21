import ForceGraph2D from "react-force-graph-2d";
import { Network } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { GraphData, GraphNode, TicketPriority, TicketType } from "../../types";
import { EmptyState, Panel } from "../../components/ui";

function nodeColor(type: TicketType, priority: TicketPriority): string {
  if (priority === "urgent") {
    return "#dc2626";
  }
  switch (type) {
    case "epic":
      return "#7c3aed";
    case "subtask":
      return "#64748b";
    default:
      return "#0e7490";
  }
}

export function ProjectGraph({ data }: { data: GraphData }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 1, height: 560 });

  useEffect(() => {
    const element = containerRef.current;
    if (!element) {
      return;
    }

    const measure = () => {
      setSize({
        width: Math.max(1, element.clientWidth),
        height: Math.max(420, element.clientHeight),
      });
    };
    measure();

    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const paintNode = useCallback((node: GraphNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
    const radius = 5;
    const label = node.label || node.id;
    const fontSize = Math.max(9, 12 / globalScale);

    ctx.beginPath();
    ctx.arc(node.x || 0, node.y || 0, radius, 0, 2 * Math.PI);
    ctx.fillStyle = nodeColor(node.type, node.priority);
    ctx.fill();

    ctx.font = `${fontSize}px Inter, sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    ctx.fillStyle = "#334155";
    ctx.fillText(label, node.x || 0, (node.y || 0) + radius + 4);
  }, []);

  if (data.nodes.length === 0) {
    return <EmptyState icon={<Network size={36} />} title="No graph data" body="Create related tickets to build the graph." />;
  }

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-wrap items-center gap-4 border-b border-slate-200 px-4 py-3 text-xs text-slate-600">
        <span className="inline-flex items-center gap-1">
          <span className="h-2.5 w-2.5 rounded-full bg-violet-600" />
          Epic
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="h-2.5 w-2.5 rounded-full bg-cyan-700" />
          Task
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="h-2.5 w-2.5 rounded-full bg-slate-500" />
          Subtask
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="h-2.5 w-2.5 rounded-full bg-red-600" />
          Urgent
        </span>
      </div>
      <div ref={containerRef} className="h-[calc(100vh-240px)] min-h-[420px] w-full bg-white">
        <ForceGraph2D
          width={size.width}
          height={size.height}
          graphData={data}
          nodeCanvasObject={paintNode}
          nodeLabel="label"
          linkDirectionalArrowLength={4}
          linkDirectionalArrowRelPos={1}
          linkCurvature={0.18}
          linkColor={() => "#94a3b8"}
          backgroundColor="#ffffff"
        />
      </div>
    </Panel>
  );
}
