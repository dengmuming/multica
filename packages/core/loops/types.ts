export type LoopState =
  | "running"
  | "waiting_approval"
  | "blocked"
  | "completed"
  | "failed"
  | "cancelled"
  | string;

export interface LoopSummary {
  parent_issue_id: string;
  project_id?: string;
  title: string;
  description?: string;
  issue_status: string;
  state: LoopState;
  template_key: string;
  template_version: number;
  policy_version: string;
  instance_key: string;
  created_at: string;
  updated_at: string;
}

export interface LoopFinding {
  code?: string;
  severity?: string;
  owner_role?: string;
  owner_node_key?: string;
  summary: string;
  artifact_refs?: string[];
}

export interface LoopNode extends Record<string, unknown> {
  issue_id: string;
  node_key: string;
  node_type: string;
  role?: string;
  title: string;
  description?: string;
  status: string;
  stage: number;
  required: boolean;
  retry_count: number;
  max_retries: number;
  assignee_type?: string;
  assignee_id?: string;
  latest_task_id?: string;
  latest_task_status?: string;
  evaluation_id?: string;
  evaluation_verdict?: string;
  evaluation_target?: string;
  evaluation_findings?: LoopFinding[];
  evaluation_evidence?: string[];
  approval_id?: string;
  approval_state?: string;
  approval_requested_from_id?: string;
  approval_rationale?: string;
}

export interface LoopDetail extends LoopSummary {
  nodes: LoopNode[];
}

export interface LoopArtifact {
  id: string;
  artifact_type: string;
  relation: string;
  title: string;
  ref_kind: string;
  ref_id?: string;
  ref_uri?: string;
  node_issue_id?: string;
  task_id?: string;
  created_at: string;
  metadata?: Record<string, unknown>;
}

export interface LoopRoleBinding {
  assignee_type: "agent" | "squad" | "member";
  assignee_id: string;
}

export interface InstantiateLoopRequest {
  project_id: string;
  template_key: string;
  version: number;
  instance_key: string;
  title: string;
  description?: string;
  bindings: Record<string, LoopRoleBinding>;
}

export interface InstantiateLoopResponse {
  parent_issue_id: string;
  node_issue_ids: Record<string, string>;
  template_key: string;
  version: number;
}

export interface SubmitLoopEvaluationRequest {
  evaluation_kind: string;
  verdict: "pass" | "fail" | "warn" | "inconclusive" | string;
  score?: number;
  findings?: LoopFinding[];
  evidence?: string[];
}

export interface SubmitLoopEvaluationResponse {
  evaluation_id: string;
  created: boolean;
  normalized: Record<string, unknown>;
  policy_applied?: boolean;
  policy_reason?: string;
  warning?: string;
}

export interface DecideLoopApprovalRequest {
  decision: "approved" | "rejected";
  rationale?: string;
}

export interface DecideLoopApprovalResponse {
  approval_id: string;
  state: string;
  changed: boolean;
  policy_applied?: boolean;
  policy_reason?: string;
  warning?: string;
}

export interface RegisterLoopArtifactRequest {
  node_issue_id?: string;
  task_id?: string;
  artifact_type: string;
  relation: string;
  title?: string;
  ref_kind: string;
  ref_id?: string;
  ref_uri?: string;
  metadata?: Record<string, unknown>;
}

export interface ListLoopsParams {
  projectId?: string;
  limit?: number;
}

export interface ListLoopsResponse {
  loops: LoopSummary[];
}

export interface ListLoopArtifactsResponse {
  artifacts: LoopArtifact[];
}
