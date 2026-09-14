import type { LoopDetailView, LoopNodeView } from "./types";

export interface LoopStageView {
  stage: number;
  nodes: LoopNodeView[];
  active: boolean;
  satisfied: boolean;
}

const TERMINAL_NODE_STATUSES = new Set(["done", "cancelled"]);
const ACTIVE_NODE_STATUSES = new Set(["todo", "in_progress", "in_review"]);

export function groupLoopNodesByStage(nodes: LoopNodeView[]): LoopStageView[] {
  const grouped = new Map<number, LoopNodeView[]>();
  for (const node of nodes) {
    const bucket = grouped.get(node.stage);
    if (bucket) bucket.push(node);
    else grouped.set(node.stage, [node]);
  }

  return [...grouped.entries()]
    .sort(([a], [b]) => a - b)
    .map(([stage, stageNodes]) => ({
      stage,
      nodes: [...stageNodes].sort((a, b) => a.node_key.localeCompare(b.node_key)),
      active: stageNodes.some((node) => ACTIVE_NODE_STATUSES.has(node.status)),
      satisfied: stageNodes.every((node) => nodeIsSatisfied(node)),
    }));
}

export function nodeIsSatisfied(node: LoopNodeView): boolean {
  if (node.node_type === "approval") return node.approval_state === "approved";
  if (node.node_type === "evaluation") {
    return (
      node.status === "done" &&
      (node.evaluation_verdict === "pass" || node.evaluation_verdict === "warn")
    );
  }
  return TERMINAL_NODE_STATUSES.has(node.status);
}

export function currentLoopStage(loop: LoopDetailView): number | null {
  const stages = groupLoopNodesByStage(loop.nodes);
  return stages.find((stage) => !stage.satisfied)?.stage ?? null;
}

export function workflowRetryTotal(loop: LoopDetailView): number {
  return loop.nodes.reduce((sum, node) => sum + Math.max(0, node.retry_count), 0);
}

export function findingsForLoop(loop: LoopDetailView) {
  return loop.nodes.flatMap((node) =>
    (node.evaluation_findings ?? []).map((finding) => ({
      nodeKey: node.node_key,
      verdict: node.evaluation_verdict,
      finding,
    })),
  );
}
