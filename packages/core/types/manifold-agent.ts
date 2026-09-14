export type ManifoldLoopState =
  | "running"
  | "waiting_approval"
  | "blocked"
  | "completed"
  | "cancelled"
  | "failed"
  | string;

export type ManifoldLoopNodeType = "agent" | "evaluation" | "approval" | string;

export type ManifoldEvaluationVerdict =
  | "pass"
  | "fail"
  | "warn"
  | "inconclusive"
  | string;

export type ManifoldApprovalState =
  | "pending"
  | "approved"
  | "rejected"
  | "cancelled"
  | "expired"
  | string;

export interface ManifoldEvaluationFinding {
  code: string;
  severity: "info" | "low" | "medium" | "high" | "critical" | string;
  owner_role?: string;
  owner_node_key?: string;
  summary: string;
  artifact_refs?: string[];
}

export interface ManifoldLoopSummary {
  parent_issue_id: string;
  project_id?: string;
  title: string;
  description?: string;
  issue_status: string;
  state: ManifoldLoopState;
  template_key: string;
  template_version: number;
  policy_version: string;
  instance_key: string;
  created_at: string;
  updated_at: string;
}

export interface ManifoldLoopNode {
  issue_id: string;
  node_key: string;
  node_type: ManifoldLoopNodeType;
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
  evaluation_verdict?: ManifoldEvaluationVerdict;
  evaluation_target?: string;
  evaluation_findings?: ManifoldEvaluationFinding[];
  evaluation_evidence?: string[];
  approval_id?: string;
  approval_state?: ManifoldApprovalState;
  approval_requested_from_id?: string;
  approval_rationale?: string;
}

export interface ManifoldLoopDetail extends ManifoldLoopSummary {
  nodes: ManifoldLoopNode[];
}

export interface ManifoldLoopListResponse {
  loops: ManifoldLoopSummary[];
}
