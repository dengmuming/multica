import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { getLoop, listLoopArtifacts, listLoops } from "./api";

export const loopKeys = {
  all: (workspaceId: string) => ["loops", workspaceId] as const,
  list: (workspaceId: string, projectId?: string) =>
    [...loopKeys.all(workspaceId), "list", projectId ?? "all"] as const,
  detail: (workspaceId: string, parentIssueId: string) =>
    [...loopKeys.all(workspaceId), "detail", parentIssueId] as const,
  artifacts: (workspaceId: string, parentIssueId: string) =>
    [...loopKeys.detail(workspaceId, parentIssueId), "artifacts"] as const,
};

export function loopListOptions(workspaceId: string, projectId?: string) {
  return queryOptions({
    queryKey: loopKeys.list(workspaceId, projectId),
    queryFn: () => listLoops(api, { projectId }),
    select: (data) => data.loops,
  });
}

export function loopDetailOptions(workspaceId: string, parentIssueId: string) {
  return queryOptions({
    queryKey: loopKeys.detail(workspaceId, parentIssueId),
    queryFn: () => getLoop(api, parentIssueId),
    enabled: Boolean(parentIssueId),
  });
}

export function loopArtifactsOptions(workspaceId: string, parentIssueId: string) {
  return queryOptions({
    queryKey: loopKeys.artifacts(workspaceId, parentIssueId),
    queryFn: () => listLoopArtifacts(api, parentIssueId),
    select: (data) => data.artifacts,
    enabled: Boolean(parentIssueId),
  });
}
