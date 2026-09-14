export type LoopState =
  | "running"
  | "waiting_approval"
  | "blocked"
  | "completed"
  | "failed"
  | "cancelled"
  | string;

export interface LoopSummaryView {
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

export interface LoopFindingView {
  code?: string;
  severity?: string;
  owner_role?: string;
  owner_node_key?: string;
  summary: string;
  artifact_refs?: string[];
}

export interface LoopNodeView {
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
  evaluation_findings?: LoopFindingView[];
  evaluation_evidence?: string[];
  approval_id?: string;
  approval_state?: string;
  approval_requested_from_id?: string;
  approval_rationale?: string;
}

export interface LoopDetailView extends LoopSummaryView {
  nodes: LoopNodeView[];
}

export interface LoopArtifactView {
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
}

export interface LoopViewCopy {
  stage: string;
  state: string;
  role: string;
  assignee: string;
  task: string;
  retries: string;
  evaluation: string;
  findings: string;
  approval: string;
  evidence: string;
  artifacts: string;
  noFindings: string;
  noEvidence: string;
  noArtifacts: string;
  required: string;
  optional: string;
}

export interface LoopListCopy {
  state: string;
  template: string;
  updated: string;
  empty: string;
}
