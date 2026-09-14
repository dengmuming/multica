import type { LoopDetailView, LoopNodeView } from "./types";

export type LoopStageState =
  | "backlog"
  | "ready"
  | "running"
  | "waiting_approval"
  | "failed"
  | "done";

export interface LoopStageView {
  stage: number;
  nodes: LoopNodeView[];
  active: boolean;
  satisfied: boolean;
  state: LoopStageState;
  retryCount: number;
  maxRetries: number;
  hasEvaluationFailure: boolean;
  hasPendingApproval: boolean;
}

const TERMINAL_NODE_STATUSES = new Set(["done", "cancelled"]);
const ACTIVE_NODE_STATUSES = new Set(["todo", "in_progress", "in_review"]);

function stageState(nodes: LoopNodeView[]): LoopStageState {
  if (
    nodes.some(
      (node) =>
        node.evaluation_verdict === "fail" || node.approval_state === "rejected",
    )
  ) {
    return "failed";
  }
  if (
    nodes.some(
      (node) => node.node_type === "approval" && node.approval_state === "pending",
    )
  ) {
    return "waiting_approval";
  }
  if (nodes.every((node) => nodeIsSatisfied(node))) return "done";
  if (
    nodes.some(
      (node) => node.status === "in_progress" || node.status === "in_review",
    )
  ) {
    return "running";
  }
  if (nodes.some((node) => ACTIVE_NODE_STATUSES.has(node.status))) return "ready";
  return "backlog";
}

export function groupLoopNodesByStage(nodes: LoopNodeView[]): LoopStageView[] {
  const grouped = new Map<number, LoopNodeView[]>();
  for (const node of nodes) {
    const bucket = grouped.get(node.stage);
    if (bucket) bucket.push(node);
    else grouped.set(node.stage, [node]);
  }

  return [...grouped.entries()]
    .sort(([a], [b]) => a - b)
    .map(([stage, stageNodes]) => {
      const stableNodes = [...stageNodes].sort((a, b) =>
        a.node_key.localeCompare(b.node_key),
      );
      const satisfied = stableNodes.every((node) => nodeIsSatisfied(node));
      return {
        stage,
        nodes: stableNodes,
        active: stableNodes.some((node) => ACTIVE_NODE_STATUSES.has(node.status)),
        satisfied,
        state: stageState(stableNodes),
        retryCount: stableNodes.reduce(
          (sum, node) => sum + Math.max(0, node.retry_count),
          0,
        ),
        maxRetries: stableNodes.reduce(
          (sum, node) => sum + Math.max(0, node.max_retries),
          0,
        ),
        hasEvaluationFailure: stableNodes.some(
          (node) => node.evaluation_verdict === "fail",
        ),
        hasPendingApproval: stableNodes.some(
          (node) =>
            node.node_type === "approval" && node.approval_state === "pending",
        ),
      };
    });
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

export function requiredLoopProgress(loop: LoopDetailView): {
  completed: number;
  total: number;
} {
  const required = loop.nodes.filter((node) => node.required);
  return {
    completed: required.filter((node) => nodeIsSatisfied(node)).length,
    total: required.length,
  };
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
