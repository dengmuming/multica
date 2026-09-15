package loopservice

import "context"

// WorkGraph is the product-facing projection of a Manifold Agent Native Loop.
// It intentionally hides Multica persistence/runtime details from API consumers.
type WorkGraph struct {
	ID                string                 `json:"id"`
	WorkspaceID       string                 `json:"workspace_id"`
	ProjectID         string                 `json:"project_id"`
	Intent            WorkGraphIntent        `json:"intent"`
	Status            string                 `json:"status"`
	CurrentStage      string                 `json:"current_stage,omitempty"`
	Nodes             []WorkGraphNode        `json:"nodes"`
	Edges             []WorkGraphEdge        `json:"edges"`
	Gates             []WorkGraphGate        `json:"gates"`
	ProvenanceSummary WorkGraphProvenance    `json:"provenance_summary"`
}

type WorkGraphIntent struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

type WorkGraphNode struct {
	Key            string               `json:"key"`
	Kind           string               `json:"kind"`
	Role           string               `json:"role,omitempty"`
	Responsibility string               `json:"responsibility,omitempty"`
	Stage          string               `json:"stage"`
	State          string               `json:"state"`
	Assignee       *WorkGraphAssignee   `json:"assignee,omitempty"`
	CurrentTask    *WorkGraphTask       `json:"current_task,omitempty"`
	Evaluation     *WorkGraphEvaluation `json:"evaluation,omitempty"`
	ArtifactCount  int                  `json:"artifact_count"`
}

type WorkGraphAssignee struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type WorkGraphTask struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Attempt int    `json:"attempt,omitempty"`
}

type WorkGraphEvaluation struct {
	Verdict      string   `json:"verdict"`
	Score        *float64 `json:"score,omitempty"`
	FindingCount int      `json:"finding_count"`
}

type WorkGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type WorkGraphGate struct {
	NodeKey string `json:"node_key"`
	Type    string `json:"type"`
	State   string `json:"state"`
}

type WorkGraphProvenance struct {
	ArtifactCount   int `json:"artifact_count"`
	EvaluationCount int `json:"evaluation_count"`
}

// WorkGraphReader isolates the Manifold product contract from the concrete
// workforce/runtime implementation. Multica is the first projection backend.
type WorkGraphReader interface {
	GetWorkGraph(ctx context.Context, workspaceID, parentIssueID string) (*WorkGraph, error)
}
