package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/loopevaluation"
)

// RawEvaluationEvidenceAuthorizer proves every evidence/artifact reference is
// registered on the same loop instance. P0 accepts a reference when it matches
// one of loop_artifact.id, loop_artifact.ref_id, or loop_artifact.ref_uri.
// Arbitrary URLs are therefore not trusted merely because they are well formed;
// they must first be registered as loop_artifact provenance.
type RawEvaluationEvidenceAuthorizer struct {
	db LoopStateDB
}

func NewRawEvaluationEvidenceAuthorizer(db LoopStateDB) *RawEvaluationEvidenceAuthorizer {
	return &RawEvaluationEvidenceAuthorizer{db: db}
}

func (a *RawEvaluationEvidenceAuthorizer) AuthorizeEvaluationEvidence(
	ctx context.Context,
	workspaceID, parentIssueID string,
	submission loopevaluation.Submission,
) error {
	if a == nil || a.db == nil {
		return errors.New("evaluation evidence database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return err
	}
	pid, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil {
		return err
	}

	refs := collectEvaluationEvidenceRefs(submission)
	for _, ref := range refs {
		var exists bool
		err := a.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1
				FROM loop_artifact
				WHERE workspace_id = $1
				  AND parent_issue_id = $2
				  AND (
					id::text = $3
					OR ref_id::text = $3
					OR ref_uri = $3
				  )
			)`, wid, pid, ref).Scan(&exists)
		if err != nil {
			return fmt.Errorf("authorize evidence %q: %w", ref, err)
		}
		if !exists {
			return fmt.Errorf("evidence reference %q is not registered on this loop", ref)
		}
	}
	return nil
}

func collectEvaluationEvidenceRefs(submission loopevaluation.Submission) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(submission.Evidence))
	add := func(raw string) {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	for _, ref := range submission.Evidence {
		add(ref)
	}
	for _, finding := range submission.Findings {
		for _, ref := range finding.ArtifactRefs {
			add(ref)
		}
	}
	return out
}
