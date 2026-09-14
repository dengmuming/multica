package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/loophttp"
	"github.com/multica-ai/multica/server/internal/loopservice"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// buildManifoldAgentLoopHandler wires the P0 Manifold Agent Native Loop onto
// Multica's existing Issue/Task/Runtime control plane. Keeping assembly in a
// small file prevents router.go from becoming the composition root for every
// loop implementation detail and makes the temporary raw-PGX adapters easy to
// replace with generated SQLC repositories later.
func buildManifoldAgentLoopHandler(pool *pgxpool.Pool, queries *db.Queries, h *handler.Handler) *loophttp.Handler {
	if pool == nil || queries == nil || h == nil || h.IssueService == nil {
		return nil
	}

	templates := loopservice.NewRawLoopTemplateRepository(pool)
	bindings := loopservice.NewMulticaBindingScopeValidator(queries)
	preflight := loopservice.NewInstantiateService(templates, bindings, templates)

	dispatcher := loopservice.NewMulticaInitialIssueDispatcher(h.IssueService)
	graphStore := loopservice.NewMulticaIssueGraphStore(h.IssueService)
	graphGateway := loopservice.NewDurableIssueGraphGateway(graphStore, dispatcher)
	writer := loopservice.NewMulticaIssueWriter(graphGateway)
	instantiator := loopservice.NewInstantiator(preflight, writer)

	loopState := loopservice.NewRawLoopStateRepository(pool)
	projection := loopservice.NewMulticaPolicyProjectionReader(
		loopservice.DBPolicyIssueReader{Queries: queries},
		loopState,
		loopState,
	)
	approvalRequester := loopservice.NewMulticaApprovalRequester()
	mutationStore := loopservice.NewMulticaPolicyMutationStore(h.IssueService, approvalRequester)
	mutationSink := loopservice.NewDurablePolicyMutationSink(mutationStore, dispatcher)
	policy := loopservice.NewPolicyApplicationService(projection, mutationSink)

	evaluations := loopservice.NewRawEvaluationRepository(pool, projection)
	evidence := loopservice.NewRawEvaluationEvidenceAuthorizer(pool)
	evaluationApp := loopservice.NewEvaluationApplicationService(evaluations, evidence, evaluations, policy)

	approvals := loopservice.NewMulticaApprovalRepository(h.IssueService)
	approvalApp := loopservice.NewApprovalApplicationService(approvals, approvals, policy)

	return loophttp.New(instantiator, evaluationApp, approvalApp)
}

// registerManifoldAgentLoopRoutes must be called from the authenticated,
// RequireWorkspaceMember-protected router group. Auth task tokens stamp both
// X-User-ID and X-Workspace-ID, so evaluator Agents use the same workspace
// membership boundary as humans while ApprovalApplicationService still rejects
// machine actors.
func registerManifoldAgentLoopRoutes(r chi.Router, loopHandler *loophttp.Handler) {
	if r == nil || loopHandler == nil {
		return
	}
	loopHandler.Register(r)
}
