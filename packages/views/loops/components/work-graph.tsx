import type { WorkGraph as WorkGraphModel, WorkGraphNode } from "@multica/core/loops";
import { ArrowRight, GitBranch, RotateCcw } from "lucide-react";

function tone(state: string) {
  const value = state.toLowerCase();
  if (["done", "completed", "pass", "approved", "released"].includes(value)) return "border-emerald-500/30 bg-emerald-500/5";
  if (["failed", "fail", "rejected", "blocked"].includes(value)) return "border-destructive/30 bg-destructive/5";
  if (["running", "in_progress", "awaiting_approval", "pending"].includes(value)) return "border-primary/30 bg-primary/5";
  return "border-border bg-card";
}

function NodeCard({ node }: { node: WorkGraphNode }) {
  return <div className={`min-w-48 rounded-xl border p-3 ${tone(node.state)}`}>
    <div className="flex items-center justify-between gap-3"><span className="text-sm font-medium">{node.key}</span><span className="rounded-full bg-muted px-2 py-0.5 text-[10px] uppercase text-muted-foreground">{node.kind}</span></div>
    <div className="mt-1 text-xs text-muted-foreground">{node.role || "system"} · {node.state}</div>
    {node.current_task ? <div className="mt-2 truncate text-[11px] text-muted-foreground">Task: {node.current_task.status} · attempt {node.current_task.attempt ?? 1}</div> : null}
    {node.evaluation ? <div className="mt-2 text-[11px]">Evaluation: <strong>{node.evaluation.verdict}</strong>{node.evaluation.finding_count ? ` · ${node.evaluation.finding_count} findings` : ""}</div> : null}
    {node.artifact_count ? <div className="mt-1 text-[11px] text-muted-foreground">{node.artifact_count} artifacts</div> : null}
  </div>;
}

export function WorkGraph({ graph }: { graph: WorkGraphModel }) {
  const stages = Array.from(new Set(graph.nodes.map((node) => node.stage))).sort((a, b) => Number(a) - Number(b));
  const recovery = graph.edges.filter((edge) => edge.kind === "recovery");
  return <section className="rounded-xl border bg-background p-5" aria-label="Work Graph">
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div><div className="flex items-center gap-2 text-sm font-semibold"><GitBranch className="size-4" />Work Graph</div><p className="mt-1 text-xs text-muted-foreground">Intent → parallel work → gates → delivery. Graph semantics come from the server projection.</p></div>
      <div className="flex gap-2 text-xs"><span className="rounded-full bg-muted px-2.5 py-1">{graph.status}</span><span className="rounded-full bg-muted px-2.5 py-1">{graph.provenance_summary.artifact_count} artifacts</span><span className="rounded-full bg-muted px-2.5 py-1">{graph.provenance_summary.evaluation_count} evaluations</span></div>
    </div>
    <div className="mt-5 overflow-x-auto pb-2"><div className="flex min-w-max items-stretch gap-3">
      {stages.map((stage, index) => <div key={stage} className="flex items-center gap-3"><div className="rounded-xl border border-dashed p-3"><div className="mb-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Stage {stage}{graph.current_stage === stage ? " · current" : ""}</div><div className="flex gap-2">{graph.nodes.filter((node) => node.stage === stage).map((node) => <NodeCard key={node.key} node={node} />)}</div></div>{index < stages.length - 1 ? <ArrowRight className="size-4 shrink-0 text-muted-foreground" /> : null}</div>)}
    </div></div>
    {recovery.length ? <div className="mt-4 border-t pt-3"><div className="mb-2 flex items-center gap-1.5 text-xs font-medium"><RotateCcw className="size-3.5" />Recovery paths</div><div className="flex flex-wrap gap-2">{recovery.map((edge) => <span key={`${edge.from}-${edge.to}`} className="rounded-md bg-muted px-2 py-1 text-[11px]">{edge.from} ↺ {edge.to}</span>)}</div></div> : null}
  </section>;
}
