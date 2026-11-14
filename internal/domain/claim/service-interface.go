package claim

import (
	"context"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// Engine scores a verification state. It is an interface so the claim service
// can run against a fake in tests and against D3 (with local fallback) in
// production.
type Engine interface {
	Score(ctx context.Context, state verification.State, questions verification.Questions) (verification.D3Result, error)
}

// Deps are the optional claim-service dependencies. A nil Engine uses the
// deterministic local scorer.
type Deps struct {
	Engine Engine
	// Now is injectable for deterministic tests; production uses time.Now.
	Now func() time.Time
}

// Service defines the claim operations exposed to the HTTP layer and to the
// agent tools of later tasks.
type Service interface {
	// CreateClaim verifies the searcher's answers, applies the policy and
	// atomically writes the claim, the verification session, the claim_created
	// item event and the audit entry. It never issues a pickup pass.
	CreateClaim(ctx context.Context, input CreateClaimInput) (Claim, error)

	// GetClaim returns a claim to its owner or to the custodian / institution.
	// The owner receives only status, confidence band and return channel; the
	// custodian and the institution receive the verification report.
	GetClaim(ctx context.Context, claimID string, actor Actor) (Claim, error)

	// ReviewClaim records a custodian or institution decision on an
	// under-review claim and appends the approved/rejected event and audit.
	ReviewClaim(ctx context.Context, claimID, decision, actorID string) (Claim, error)
}

// New returns a claim Service backed by db. Passing no Deps uses the local
// scorer and the system clock.
func New(db repository.Database, options ...Deps) Service {
	deps := Deps{}
	if len(options) > 0 {
		deps = options[0]
	}
	if deps.Engine == nil {
		deps.Engine = LocalEngine()
	}
	return &service{db: db, deps: deps}
}

// LocalEngine returns an Engine that always uses the deterministic local
// scorer. It is the safe default when D3 is not configured.
func LocalEngine() Engine {
	return localEngine{}
}

type localEngine struct{}

func (localEngine) Score(_ context.Context, state verification.State, questions verification.Questions) (verification.D3Result, error) {
	return verification.LocalScore(state, questions), nil
}

// D3Engine returns an Engine backed by the D3 client. The verification engine
// itself falls back to the local scorer when D3 is unavailable, so this engine
// never fails because of a provider outage. A nil client disables D3.
func D3Engine(client itemdomain.Cataloger) Engine {
	if client == nil {
		return LocalEngine()
	}
	return d3Engine{client: client}
}

type d3Engine struct {
	client itemdomain.Cataloger
}

func (e d3Engine) Score(ctx context.Context, state verification.State, questions verification.Questions) (verification.D3Result, error) {
	return verification.D3Score(ctx, e.client, state, questions, nil)
}
