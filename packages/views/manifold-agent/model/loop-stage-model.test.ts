import { describe, expect, it } from "vitest";
import type { ManifoldLoopDetail, ManifoldLoopNode } from "@multica/core/types/manifold-agent";
import { currentLoopStage, loopProgress, projectLoopStages } from "./loop-stage-model";

function node(overrides: Partial<ManifoldLoopNode> & Pick<ManifoldLoopNode, "node_key" | "stage">): ManifoldLoopNode {
  return {
    issue_id: `issue-${overrides.node_key}`,
    node_type: "agent",
    title: overrides.node_key,
    status: "backlog",
    required: true,
    retry_count: 0,
    max_retries: 1,
    ...overrides,
  };
}

function loop(nodes: ManifoldLoopNode[]): ManifoldLoopDetail {
  return {
    parent_issue_id: "parent-1",
    title: "Feature delivery",
    issue_status: "in_progress",
    state: "running",
    template_key: "feature-development",
    template_version: 1,
    policy_version: "policy-v1",
    instance_key: "instance-1",
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
    nodes,
  };
}

describe("projectLoopStages", () => {
  it("keeps same-stage development nodes parallel and deterministically ordered", () => {
    const stages = projectLoopStages(loop([
      node({ node_key: "frontend", stage: 30, status: "todo" }),
      node({ node_key: "product", stage: 10, status: "done" }),
      node({ node_key: "backend", stage: 30, status: "in_progress" }),
    ]));

    expect(stages.map((stage) => stage.stage)).toEqual([10, 30]);
    expect(stages[1]?.nodes.map((entry) => entry.node_key)).toEqual(["backend", "frontend"]);
    expect(stages[1]?.status).toBe("running");
  });

  it("surfaces evaluation failure above ordinary issue status", () => {
    const stages = projectLoopStages(loop([
      node({
        node_key: "test",
        node_type: "evaluation",
        stage: 50,
        status: "done",
        evaluation_verdict: "fail",
        evaluation_target: "backend",
      }),
    ]));

    expect(stages[0]?.status).toBe("failed");
    expect(stages[0]?.hasEvaluationFailure).toBe(true);
  });

  it("surfaces a pending approval as its own stage state", () => {
    const stages = projectLoopStages(loop([
      node({
        node_key: "approval",
        node_type: "approval",
        stage: 60,
        status: "todo",
        approval_state: "pending",
      }),
    ]));

    expect(stages[0]?.status).toBe("waiting_approval");
    expect(stages[0]?.hasPendingApproval).toBe(true);
  });

  it("aggregates workflow retry counters across parallel nodes", () => {
    const stages = projectLoopStages(loop([
      node({ node_key: "backend", stage: 30, retry_count: 2, max_retries: 3 }),
      node({ node_key: "frontend", stage: 30, retry_count: 1, max_retries: 2 }),
    ]));

    expect(stages[0]?.retryCount).toBe(3);
    expect(stages[0]?.maxRetries).toBe(5);
  });
});

describe("currentLoopStage", () => {
  it("returns the first non-complete barrier", () => {
    const detail = loop([
      node({ node_key: "product", stage: 10, status: "done" }),
      node({ node_key: "architecture", stage: 20, status: "done" }),
      node({ node_key: "backend", stage: 30, status: "in_progress" }),
      node({ node_key: "review", stage: 40, status: "backlog" }),
    ]);

    expect(currentLoopStage(detail)?.stage).toBe(30);
  });
});

describe("loopProgress", () => {
  it("counts only required nodes", () => {
    const progress = loopProgress(loop([
      node({ node_key: "product", stage: 10, status: "done" }),
      node({ node_key: "backend", stage: 30, status: "in_progress" }),
      node({ node_key: "embedded", stage: 30, status: "backlog", required: false }),
    ]));

    expect(progress).toEqual({ completed: 1, total: 2 });
  });
});
