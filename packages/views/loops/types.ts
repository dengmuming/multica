import type {
  LoopArtifact,
  LoopDetail,
  LoopFinding,
  LoopNode,
  LoopState as CoreLoopState,
  LoopSummary,
} from "@multica/core/loops";

// Transport/domain DTOs are owned by @multica/core/loops. Views keep aliases
// so the existing public surface remains source-compatible while avoiding a
// second copy of the Manifold Agent API contract.
export type LoopState = CoreLoopState;
export type LoopSummaryView = LoopSummary;
export type LoopFindingView = LoopFinding;
export type LoopNodeView = LoopNode;
export type LoopDetailView = LoopDetail;
export type LoopArtifactView = LoopArtifact;

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
