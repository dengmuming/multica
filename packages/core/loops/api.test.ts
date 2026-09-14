import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "../api/client";
import {
  decideLoopApproval,
  getLoop,
  instantiateLoop,
  listLoopArtifacts,
  listLoops,
  registerLoopArtifact,
  submitLoopEvaluation,
} from "./api";

function clientWithFetch() {
  const fetch = vi.fn().mockResolvedValue({});
  return {
    client: { fetch } as unknown as ApiClient,
    fetch,
  };
}

describe("loop ApiClient transport", () => {
  it("lists loops with encoded filters", async () => {
    const { client, fetch } = clientWithFetch();
    await listLoops(client, { projectId: "project/1", limit: 25 });
    expect(fetch).toHaveBeenCalledWith("/api/loops?project_id=project%2F1&limit=25");
  });

  it("loads loop detail and artifacts with encoded parent id", async () => {
    const { client, fetch } = clientWithFetch();
    await getLoop(client, "parent/1");
    await listLoopArtifacts(client, "parent/1");
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/loops/parent%2F1");
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/loops/parent%2F1/artifacts");
  });

  it("creates a loop through the shared JSON transport", async () => {
    const { client, fetch } = clientWithFetch();
    const request = {
      project_id: "project-1",
      template_key: "feature-development",
      version: 1,
      instance_key: "feature-42",
      title: "Feature 42",
      bindings: {
        backend: { assignee_type: "agent" as const, assignee_id: "agent-1" },
      },
    };
    await instantiateLoop(client, request);
    expect(fetch).toHaveBeenCalledWith("/api/loops", {
      method: "POST",
      body: JSON.stringify(request),
    });
  });

  it("submits evaluation to the authenticated task endpoint", async () => {
    const { client, fetch } = clientWithFetch();
    const request = { evaluation_kind: "test", verdict: "fail", evidence: ["artifact-1"] };
    await submitLoopEvaluation(client, "task/1", request);
    expect(fetch).toHaveBeenCalledWith("/api/tasks/task%2F1/loop-evaluation", {
      method: "POST",
      body: JSON.stringify(request),
    });
  });

  it("decides approval without a caller supplied actor", async () => {
    const { client, fetch } = clientWithFetch();
    const request = { decision: "approved" as const, rationale: "verified" };
    await decideLoopApproval(client, "approval/1", request);
    expect(fetch).toHaveBeenCalledWith("/api/loop-approvals/approval%2F1/decision", {
      method: "POST",
      body: JSON.stringify(request),
    });
  });

  it("registers provenance through the loop artifact endpoint", async () => {
    const { client, fetch } = clientWithFetch();
    const request = {
      artifact_type: "pull_request",
      relation: "output",
      ref_kind: "url",
      ref_uri: "https://example.test/pr/1",
    };
    await registerLoopArtifact(client, "parent-1", request);
    expect(fetch).toHaveBeenCalledWith("/api/loops/parent-1/artifacts", {
      method: "POST",
      body: JSON.stringify(request),
    });
  });
});
