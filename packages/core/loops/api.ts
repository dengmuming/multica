import type { ApiClient } from "../api/client";
import type {
  DecideLoopApprovalRequest,
  DecideLoopApprovalResponse,
  InstantiateLoopRequest,
  InstantiateLoopResponse,
  ListLoopArtifactsResponse,
  ListLoopsParams,
  ListLoopsResponse,
  LoopArtifact,
  LoopDetail,
  RegisterLoopArtifactRequest,
  SubmitLoopEvaluationRequest,
  SubmitLoopEvaluationResponse,
} from "./types";
import type { WorkGraph } from "./work-graph";

type ApiClientTransport = { fetch<T>(path: string, init?: RequestInit): Promise<T> };
function transport(client: ApiClient): ApiClientTransport { return client as unknown as ApiClientTransport; }

export async function listLoops(client: ApiClient, params: ListLoopsParams = {}): Promise<ListLoopsResponse> {
  const search = new URLSearchParams();
  if (params.projectId) search.set("project_id", params.projectId);
  if (params.limit) search.set("limit", String(params.limit));
  const suffix = search.toString() ? `?${search.toString()}` : "";
  return transport(client).fetch(`/api/loops${suffix}`);
}

export async function getLoop(client: ApiClient, parentIssueId: string): Promise<LoopDetail> {
  return transport(client).fetch(`/api/loops/${encodeURIComponent(parentIssueId)}`);
}

export async function getWorkGraph(client: ApiClient, parentIssueId: string): Promise<WorkGraph> {
  return transport(client).fetch(`/api/loops/${encodeURIComponent(parentIssueId)}/work-graph`);
}

export async function instantiateLoop(client: ApiClient, request: InstantiateLoopRequest): Promise<InstantiateLoopResponse> {
  return transport(client).fetch("/api/loops", { method: "POST", body: JSON.stringify(request) });
}

export async function submitLoopEvaluation(client: ApiClient, taskId: string, request: SubmitLoopEvaluationRequest): Promise<SubmitLoopEvaluationResponse> {
  return transport(client).fetch(`/api/tasks/${encodeURIComponent(taskId)}/loop-evaluation`, { method: "POST", body: JSON.stringify(request) });
}

export async function decideLoopApproval(client: ApiClient, approvalId: string, request: DecideLoopApprovalRequest): Promise<DecideLoopApprovalResponse> {
  return transport(client).fetch(`/api/loop-approvals/${encodeURIComponent(approvalId)}/decision`, { method: "POST", body: JSON.stringify(request) });
}

export async function listLoopArtifacts(client: ApiClient, parentIssueId: string): Promise<ListLoopArtifactsResponse> {
  return transport(client).fetch(`/api/loops/${encodeURIComponent(parentIssueId)}/artifacts`);
}

export async function registerLoopArtifact(client: ApiClient, parentIssueId: string, request: RegisterLoopArtifactRequest): Promise<LoopArtifact> {
  return transport(client).fetch(`/api/loops/${encodeURIComponent(parentIssueId)}/artifacts`, { method: "POST", body: JSON.stringify(request) });
}
