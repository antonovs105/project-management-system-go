import ForceGraph2D from "react-force-graph-2d";
import { ArrowUpRight, Network, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { StatusBadge } from "../../components/StatusBadge";
import { Button, EmptyState, IconButton, Panel } from "../../components/ui";
import { compactId, relativeDate } from "../../lib/format";
import type { GraphData, GraphLink, GraphNode, ID, Ticket, TicketPriority, TicketType } from "../../types";

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

function endpointId(endpoint: GraphLink["source"] | GraphLink["target"]): ID {
  return typeof endpoint === "string" ? endpoint : endpoint.id;
}

export function ProjectGraph({
  data,
  tickets,
  onOpenTicket,
}: {
  data: GraphData;
  tickets: Ticket[];
  onOpenTicket: (ticketId: ID) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 1, height: 560 });
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

  const selectedTicket = selectedNode ? tickets.find((ticket) => ticket.id === selectedNode.id) : null;
  const nodeById = useMemo(() => new Map(data.nodes.map((node) => [node.id, node])), [data.nodes]);
  const selectedLinks = useMemo(() => {
    if (!selectedNode) {
      return [];
    }
    return data.links.filter((link) => endpointId(link.source) === selectedNode.id || endpointId(link.target) === selectedNode.id);
  }, [data.links, selectedNode]);

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
    const selected = selectedNode?.id === node.id;
    const radius = selected ? 7 : 5;
    const label = node.label || node.id;
    const fontSize = Math.max(9, 12 / globalScale);

    if (selected) {
      ctx.beginPath();
      ctx.arc(node.x || 0, node.y || 0, radius + 4, 0, 2 * Math.PI);
      ctx.strokeStyle = "#09090b";
      ctx.lineWidth = 1.5 / globalScale;
      ctx.stroke();
    }

    ctx.beginPath();
    ctx.arc(node.x || 0, node.y || 0, radius, 0, 2 * Math.PI);
    ctx.fillStyle = nodeColor(node.type, node.priority);
    ctx.fill();

    ctx.font = `${fontSize}px Inter, sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    ctx.fillStyle = "#334155";
    ctx.fillText(label, node.x || 0, (node.y || 0) + radius + 4);
  }, [selectedNode]);

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
        {data.truncated ? (
          <span className="rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-amber-700">
            Showing first {data.limit} nodes
          </span>
        ) : null}
        <span className="ml-auto rounded-full border border-zinc-200 px-2 py-0.5 text-zinc-500">
          {data.nodes.length} nodes / {data.links.length} links
        </span>
      </div>
      <div className="grid xl:grid-cols-[1fr_340px]">
        <div ref={containerRef} className="h-[calc(100vh-240px)] min-h-[420px] w-full bg-white">
          <ForceGraph2D
            width={size.width}
            height={size.height}
            graphData={data}
            nodeCanvasObject={paintNode}
            nodeLabel="label"
            onNodeClick={(node) => setSelectedNode(node as GraphNode)}
            linkDirectionalArrowLength={4}
            linkDirectionalArrowRelPos={1}
            linkCurvature={0.18}
            linkColor={() => "#94a3b8"}
            backgroundColor="#ffffff"
          />
        </div>

        <aside className="border-t border-zinc-200 bg-zinc-50 p-4 xl:border-l xl:border-t-0">
          {selectedNode ? (
            <div className="grid gap-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap gap-1.5">
                    <StatusBadge value={selectedNode.type} kind="type" />
                    <StatusBadge value={selectedNode.priority} kind="priority" />
                    <StatusBadge value={selectedNode.status} kind="status" />
                  </div>
                  <h3 className="mt-3 line-clamp-2 text-base font-semibold text-zinc-950">{selectedNode.label}</h3>
                  <p className="mt-1 text-xs text-zinc-500">{compactId(selectedNode.id)}</p>
                </div>
                <IconButton label="Close node details" onClick={() => setSelectedNode(null)}>
                  <X size={16} />
                </IconButton>
              </div>

              {selectedTicket ? (
                <div className="rounded-2xl border border-zinc-200 bg-white p-3">
                  {selectedTicket.description ? (
                    <p className="line-clamp-4 text-sm text-zinc-600">{selectedTicket.description}</p>
                  ) : (
                    <p className="text-sm text-zinc-400">No description</p>
                  )}
                  <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-zinc-500">
                    <div>
                      <div className="text-zinc-400">Reporter</div>
                      <div className="mt-1 truncate text-zinc-950">{compactId(selectedTicket.reporter_id)}</div>
                    </div>
                    <div>
                      <div className="text-zinc-400">Assignee</div>
                      <div className="mt-1 truncate text-zinc-950">
                        {selectedTicket.assignee_id ? compactId(selectedTicket.assignee_id) : "unassigned"}
                      </div>
                    </div>
                    <div>
                      <div className="text-zinc-400">Resolved</div>
                      <div className="mt-1 text-zinc-950">{selectedTicket.is_resolved ? "yes" : "no"}</div>
                    </div>
                    <div>
                      <div className="text-zinc-400">Updated</div>
                      <div className="mt-1 text-zinc-950">{relativeDate(selectedTicket.updated_at)}</div>
                    </div>
                  </div>
                  <Button className="mt-3 w-full" onClick={() => onOpenTicket(selectedTicket.id)}>
                    <ArrowUpRight size={16} />
                    Open ticket
                  </Button>
                </div>
              ) : null}

              <div className="rounded-2xl border border-zinc-200 bg-white p-3">
                <h4 className="text-sm font-semibold text-zinc-950">Links</h4>
                <div className="mt-3 grid gap-2">
                  {selectedLinks.length === 0 ? <div className="text-sm text-zinc-400">No links</div> : null}
                  {selectedLinks.map((link, index) => {
                    const source = endpointId(link.source);
                    const target = endpointId(link.target);
                    const otherID = source === selectedNode.id ? target : source;
                    const otherNode = nodeById.get(otherID);
                    return (
                      <button
                        key={`${source}-${target}-${index}`}
                        type="button"
                        className="focus-ring rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 text-left text-sm transition hover:bg-white"
                        onClick={() => otherNode && setSelectedNode(otherNode)}
                      >
                        <div className="font-medium text-zinc-950">{otherNode?.label || compactId(otherID)}</div>
                        <div className="mt-1 text-xs text-zinc-500">{link.type}</div>
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          ) : (
            <div className="rounded-2xl border border-dashed border-zinc-300 bg-white p-4 text-sm text-zinc-400">No node selected</div>
          )}
        </aside>
      </div>
    </Panel>
  );
}
