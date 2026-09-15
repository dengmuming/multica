package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/loophttp"
	"github.com/multica-ai/multica/server/internal/loopservice"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type manifoldAgentRouteBundle struct { Loop *loophttp.Handler; Read *loophttp.ReadHandler; WorkGraph *loophttp.WorkGraphHandler; Templates *loophttp.TemplateHandler; Artifacts *loophttp.ArtifactHandler }

func buildManifoldAgentRoutes(pool *pgxpool.Pool, queries *db.Queries, h *handler.Handler) *manifoldAgentRouteBundle {
	if pool == nil || queries == nil || h == nil || h.IssueService == nil { return nil }
	templates := loopservice.NewRawLoopTemplateRepository(pool); bindings := loopservice.NewMulticaBindingScopeValidator(queries); preflight := loopservice.NewInstantiateService(templates, bindings, templates)
	dispatcher := loopservice.NewMulticaInitialIssueDispatcher(h.IssueService); graphStore := loopservice.NewMulticaIssueGraphStore(h.IssueService); graphGateway := loopservice.NewDurableIssueGraphGateway(graphStore, dispatcher); writer := loopservice.NewMulticaIssueWriter(graphGateway); instantiator := loopservice.NewInstantiator(preflight, writer)
	loopState := loopservice.NewRawLoopStateRepository(pool); projection := loopservice.NewMulticaPolicyProjectionReader(loopservice.DBPolicyIssueReader{Queries: queries}, loopState, loopState)
	approvalRequester := loopservice.NewMulticaApprovalRequester(); mutationStore := loopservice.NewMulticaPolicyMutationStore(h.IssueService, approvalRequester); mutationSink := loopservice.NewDurablePolicyMutationSink(mutationStore, dispatcher); policy := loopservice.NewPolicyApplicationService(projection, mutationSink); registerManifoldAgentPolicyEvents(h.Bus, queries, policy)
	evaluations := loopservice.NewRawEvaluationRepository(pool, projection); evidence := loopservice.NewRawEvaluationEvidenceAuthorizer(pool); evaluationApp := loopservice.NewEvaluationApplicationService(evaluations, evidence, evaluations, policy)
	approvals := loopservice.NewMulticaApprovalRepository(h.IssueService); approvalApp := loopservice.NewApprovalApplicationService(approvals, approvals, policy)
	templateAdminRepo := loopservice.NewRawTemplateAdminRepository(h.IssueService); templateAdmin := loopservice.NewTemplateAdminService(templateAdminRepo)
	artifacts := loopservice.NewRawArtifactRepository(pool); artifactApp := loopservice.NewArtifactApplicationService(artifacts)
	rawLoopReads := loopservice.NewRawLoopReadRepository(pool); loopReads := loopservice.NewLoopReadService(rawLoopReads); workGraphs := loopservice.NewMulticaWorkGraphProjectionReader(rawLoopReads, loopState, artifacts)
	return &manifoldAgentRouteBundle{Loop: loophttp.New(instantiator, evaluationApp, approvalApp), Read: loophttp.NewReadHandler(loopReads), WorkGraph: loophttp.NewWorkGraphHandler(workGraphs), Templates: loophttp.NewTemplateHandler(templateAdmin), Artifacts: loophttp.NewArtifactHandler(artifactApp)}
}

func registerManifoldAgentRoutes(r chi.Router, bundle *manifoldAgentRouteBundle) { if r == nil || bundle == nil { return }; if bundle.Loop != nil { bundle.Loop.Register(r) }; if bundle.Read != nil { bundle.Read.Register(r) }; if bundle.WorkGraph != nil { bundle.WorkGraph.Register(r) }; if bundle.Templates != nil { bundle.Templates.Register(r) }; if bundle.Artifacts != nil { bundle.Artifacts.Register(r) } }
