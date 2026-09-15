export type WorkGraphStatus =
  | 'planning'
  | 'running'
  | 'blocked'
  | 'awaiting_approval'
  | 'released'
  | 'observing'
  | 'completed'
  | 'failed'
  | 'cancelled'

export type WorkGraphNodeKind =
  | 'agent'
  | 'evaluation'
  | 'approval'
  | 'release'
  | 'observe'

export interface WorkGraph {
  id: string
  workspace_id: string
  project_id: string
  intent: WorkGraphIntent
  status: WorkGraphStatus
  current_stage?: string
  nodes: WorkGraphNode[]
  edges: WorkGraphEdge[]
  gates: WorkGraphGate[]
  provenance_summary: WorkGraphProvenanceSummary
}

export interface WorkGraphIntent {
  title: string
  description?: string
  source?: string
}

export interface WorkGraphNode {
  key: string
  kind: WorkGraphNodeKind
  role?: string
  responsibility?: string
  stage: string
  state: string
  assignee?: WorkGraphAssignee
  current_task?: WorkGraphTask
  evaluation?: WorkGraphEvaluation
  artifact_count: number
}

export interface WorkGraphAssignee {
  type: 'agent' | 'squad' | 'human'
  id: string
  name?: string
}

export interface WorkGraphTask {
  id: string
  status: string
  attempt?: number
}

export interface WorkGraphEvaluation {
  verdict: string
  score?: number
  finding_count: number
}

export interface WorkGraphEdge {
  from: string
  to: string
  kind: 'dependency' | 'recovery' | 'gate'
}

export interface WorkGraphGate {
  node_key: string
  type: 'human_approval' | 'evaluation'
  state: string
}

export interface WorkGraphProvenanceSummary {
  artifact_count: number
  evaluation_count: number
}
