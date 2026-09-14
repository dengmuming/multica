import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import {
  decideLoopApproval,
  instantiateLoop,
  registerLoopArtifact,
  submitLoopEvaluation,
} from "./api";
import { loopKeys } from "./queries";
import type {
  DecideLoopApprovalRequest,
  InstantiateLoopRequest,
  RegisterLoopArtifactRequest,
  SubmitLoopEvaluationRequest,
} from "./types";

export function useInstantiateLoop() {
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceId();
  return useMutation({
    mutationFn: (request: InstantiateLoopRequest) => instantiateLoop(api, request),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: loopKeys.all(workspaceId) });
      queryClient.invalidateQueries({ queryKey: loopKeys.detail(workspaceId, result.parent_issue_id) });
    },
  });
}

export function useSubmitLoopEvaluation() {
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceId();
  return useMutation({
    mutationFn: ({
      taskId,
      parentIssueId,
      request,
    }: {
      taskId: string;
      parentIssueId: string;
      request: SubmitLoopEvaluationRequest;
    }) => submitLoopEvaluation(api, taskId, request),
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: loopKeys.detail(workspaceId, variables.parentIssueId) });
      queryClient.invalidateQueries({ queryKey: loopKeys.list(workspaceId) });
    },
  });
}

export function useDecideLoopApproval() {
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceId();
  return useMutation({
    mutationFn: ({
      approvalId,
      parentIssueId,
      request,
    }: {
      approvalId: string;
      parentIssueId: string;
      request: DecideLoopApprovalRequest;
    }) => decideLoopApproval(api, approvalId, request),
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: loopKeys.detail(workspaceId, variables.parentIssueId) });
      queryClient.invalidateQueries({ queryKey: loopKeys.list(workspaceId) });
    },
  });
}

export function useRegisterLoopArtifact() {
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceId();
  return useMutation({
    mutationFn: ({
      parentIssueId,
      request,
    }: {
      parentIssueId: string;
      request: RegisterLoopArtifactRequest;
    }) => registerLoopArtifact(api, parentIssueId, request),
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: loopKeys.artifacts(workspaceId, variables.parentIssueId) });
      queryClient.invalidateQueries({ queryKey: loopKeys.detail(workspaceId, variables.parentIssueId) });
    },
  });
}
