package main

import (
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// mountManifoldAgentRoutes appends the Manifold Agent P0 API to the already
// constructed server router. It deliberately reuses the exact Auth and
// workspace-membership middleware used by the main authenticated route group.
//
// Keeping this seam outside router.go lets the feature branch integrate the
// vertical slice without making the large legacy router the composition root
// for Loop internals. Once the product boundary stabilizes, this helper can be
// folded into NewRouterWithOptions without changing any HTTP contract.
func mountManifoldAgentRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	queries *db.Queries,
	h *handler.Handler,
	rdb *redis.Client,
) {
	if r == nil || pool == nil || queries == nil || h == nil {
		return
	}

	bundle := buildManifoldAgentRoutes(pool, queries, h)
	if bundle == nil {
		return
	}

	cloudPAT := auth.NewCloudPATVerifier(auth.CloudPATVerifierConfig{
		FleetBaseURL: strings.TrimSpace(os.Getenv("MULTICA_CLOUD_URL")),
		Redis:        rdb,
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(queries, h.PATCache, cloudPAT))
		r.Use(middleware.RequireWorkspaceMember(queries))
		registerManifoldAgentRoutes(r, bundle)
	})
}
