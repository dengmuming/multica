import type { ManifoldLoopDetail, ManifoldLoopNode } from "@multica/core/types/manifold-agent";

export type ManifoldStageStatus =
  | "backlog"
  | "ready"
  | "running"
  | "waiting_approval"
  | "failed"
  | "done";

export interface ManifoldStageProjection {
  stage: number;
  status: ManifoldStageStatus;
  nodes: ManifoldLoopNode[];
  retryCount: number;
  maxRetries: number;
  hasEvaluationFailure: boolean;
  hasPendingApproval: boolean;
}

const ACTIVE_NODE_STATUSES = new Set(["todo", "in_progress", "in_review"]);
const TERMINAL_NODE_STATUSES = new Set(["done", "cancelled"]);

function nodeFailed(node: ManifoldLoopNode): boolean {
  return node.evaluation_verdict === "fail" || node.approval_state === "rejected";
}

function nodeWaitingApproval(node: ManifoldLoopNode): boolean {
  return node.node_type === "approval" && node.approval_state === "pending";
}

function stageStatus(nodes: ManifoldLoopNode[]): ManifoldStageStatus {
  if (nodes.some(nodeFailed)) return "failed";
  if (nodes.some(nodeWaitingApproval)) return "waiting_approval";
  if (nodes.every((node) => TERMINAL_NODE_STATUSES.has(node.status))) return "done";
  if (nodes.some((node) => node.status === "in_progress" || node.status === "in_review")) {
    return "running";
  }
  if (nodes.some((node) => ACTIVE_NODE_STATUSES.has(node.status))) return "ready";
  return "backlog";
}

export function projectLoopStages(loop: ManifoldLoopDetail): ManifoldStageProjection[] {
  const grouped = new Map<number, ManifoldLoopNode[]>();
  for (const node of loop.nodes) {
    const nodes = grouped.get(node.stage) ?? [];
    nodes.push(node);
    grouped.set(node.stage, nodes);
  }

  return [...grouped.entries()]
    .sort(([left], [right]) => left - right)
    .map(([stage, nodes]) => {
      const stableNodes = [...nodes].sort((left, right) => left.node_key.localeCompare(right.node_key));
      return {
        stage,
        status: stageStatus(stableNodes),
        nodes: stableNodes,
        retryCount: stableNodes.reduce((total, node) => total + node.retry_count, 0),
        maxRetries: stableNodes.reduce((total, node) => total + node.max_retries, 0),
        hasEvaluationFailure: stableNodes.some((node) => node.evaluation_verdict === "fail"),
        hasPendingApproval: stableNodes.some(nodeWaitingApproval),
      };
    });
}

export function currentLoopStage(loop: ManifoldLoopDetail): ManifoldStageProjection | undefined {
  const stages = projectLoopStages(loop);
  return stages.find((stage) => stage.status !== "done") ?? stages.at(-1);
}

export function loopProgress(loop: ManifoldLoopDetail): { completed: number; total: number } {
  const required = loop.nodes.filter((node) => node.required);
  return {
    completed: required.filter((node) => node.status === "done").length,
    total: required.length,
  };
}
