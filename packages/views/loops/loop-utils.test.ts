import { describe, expect, it } from "vitest";
import {
  currentLoopStage,
  groupLoopNodesByStage,
  nodeIsSatisfied,
  requiredLoopProgress,
  workflowRetryTotal,
} from "./loop-utils";
import type { LoopDetailView, LoopNodeView } from "./types";

function node(overrides: Partial<LoopNodeView>): LoopNodeView {
  return {
    issue_id: "issue",
    node_key: "node",
    node_type: "agent",
    title: "Node",
    status: "backlog",
    stage: 10,
    required: true,
    retry_count: 0,
    max_retries: 2,
    ...overrides,
  };
}

function loop(nodes: LoopNodeView[]): LoopDetailView {
  return {
    parent_issue_id: "parent",
    title: "Feature",
    issue_status: "backlog",
    state: "running",
    template_key: "feature-development",
    template_version: 1,
    policy_version: "policy-v1",
    instance_key: "feature-1",
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
    nodes,
  };
}

describe("loop stage projection", () => {
  it("keeps same-stage nodes parallel and orders stages deterministically", () => {
    const stages = groupLoopNodesByStage([
      node({ node_key: "review", stage: 40 }),
      node({ node_key: "frontend", stage: 30, status: "todo" }),
      node({ node_key: "backend", stage: 30, status: "in_progress" }),
    ]);

    expect(stages.map((stage) => stage.stage)).toEqual([30, 40]);
    expect(stages[0]?.nodes.map((item) => item.node_key)).toEqual(["backend", "frontend"]);
    expect(stages[0]?.active).toBe(true);
    expect(stages[0]?.satisfied).toBe(false);
    expect(stages[0]?.state).toBe("running");
  });

  it("uses authoritative evaluation and approval state for satisfaction", () => {
    expect(
      nodeIsSatisfied(node({ node_type: "evaluation", status: "done", evaluation_verdict: "fail" })),
    ).toBe(false);
    expect(
      nodeIsSatisfied(node({ node_type: "evaluation", status: "done", evaluation_verdict: "warn" })),
    ).toBe(true);
    expect(
      nodeIsSatisfied(node({ node_type: "approval", status: "done", approval_state: "rejected" })),
    ).toBe(false);
    expect(
      nodeIsSatisfied(node({ node_type: "approval", status: "todo", approval_state: "approved" })),
    ).toBe(true);
  });

  it("surfaces evaluation fail and pending approval as explicit stage states", () => {
    const failed = groupLoopNodesByStage([
      node({
        node_key: "test",
        node_type: "evaluation",
        status: "done",
        stage: 50,
        evaluation_verdict: "fail",
      }),
    ]);
    const approval = groupLoopNodesByStage([
      node({
        node_key: "approval",
        node_type: "approval",
        status: "todo",
        stage: 60,
        approval_state: "pending",
      }),
    ]);

    expect(failed[0]?.state).toBe("failed");
    expect(failed[0]?.hasEvaluationFailure).toBe(true);
    expect(approval[0]?.state).toBe("waiting_approval");
    expect(approval[0]?.hasPendingApproval).toBe(true);
  });

  it("returns the first unsatisfied stage and sums workflow retries", () => {
    const value = loop([
      node({ node_key: "product", stage: 10, status: "done", retry_count: 0 }),
      node({ node_key: "backend", stage: 30, status: "todo", retry_count: 2 }),
      node({ node_key: "test", stage: 50, node_type: "evaluation", status: "backlog", retry_count: 1 }),
    ]);

    expect(currentLoopStage(value)).toBe(30);
    expect(workflowRetryTotal(value)).toBe(3);
  });

  it("counts progress using only required nodes and gate semantics", () => {
    const value = loop([
      node({ node_key: "product", stage: 10, status: "done" }),
      node({ node_key: "embedded", stage: 30, status: "backlog", required: false }),
      node({
        node_key: "review",
        stage: 40,
        node_type: "evaluation",
        status: "done",
        evaluation_verdict: "warn",
      }),
      node({
        node_key: "approval",
        stage: 60,
        node_type: "approval",
        status: "todo",
        approval_state: "pending",
      }),
    ]);

    expect(requiredLoopProgress(value)).toEqual({ completed: 2, total: 3 });
  });
});
