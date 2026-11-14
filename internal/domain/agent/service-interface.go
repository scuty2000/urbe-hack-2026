package agent

import (
	"context"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/trace"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// LLM is the chat completion port. deepseek.Client implements it; tests use a
// scripted fake so no network call is ever made.
type LLM interface {
	Complete(ctx context.Context, messages []deepseek.Message, tools []deepseek.Tool) (deepseek.Completion, error)
}

// ItemReader loads the private item aggregate for the server-side tools. Its
// result never leaves the process verbatim: the tools sanitize it first.
type ItemReader interface {
	ItemForAgent(ctx context.Context, itemID string) (itemdomain.Item, error)
}

// PickupLocator reads the public pickup point (name, address, hours) of an
// intermediary. repository.Database implements it.
type PickupLocator interface {
	GetIntermediaryByID(ctx context.Context, id string) (record.IntermediaryRecord, error)
}

// Deps are the agent dependencies. Every field is optional: Search, Items and
// Claim default to services built on db, Engine defaults to the deterministic
// local scorer, and an empty DeepSeekKey selects the mock agent.
type Deps struct {
	Search search.Service
	Items  ItemReader
	Claim  claim.Service
	Engine claim.Engine
	// Places locates the custodian of a claim under review, so the searcher
	// can verify ownership in person. Defaults to db.
	Places PickupLocator

	// Client overrides the DeepSeek client (tests). When nil and DeepSeekKey is
	// set, a deepseek.Client is built from the values below.
	Client LLM

	DeepSeekKey     string
	DeepSeekURL     string
	DeepSeekModel   string
	DeepSeekTimeout time.Duration

	// MaxToolRounds defaults to 5 and is capped at 5.
	MaxToolRounds int

	// Offline enables the deterministic keyword agent instead of DeepSeek.
	// Tests only: production never sets it, so without DeepSeek the chat
	// returns errs.ErrUnavailable instead of answering with local logic.
	Offline bool

	// Trace receives the model outputs, tool calls, ownership decisions and
	// replies of every turn as JSONL. Nil disables tracing.
	Trace *trace.Recorder
}

// Service is the orchestrator API exposed to the HTTP layer.
type Service interface {
	// Chat runs one conversation turn for the authenticated searcher and
	// returns a role-safe reply. It falls back to the mock agent when the
	// provider is not configured or unavailable.
	Chat(ctx context.Context, searcherID string, messages []Message) (Reply, error)
}

// New returns an agent service backed by db. Passing no Deps uses the SQLite
// search fallback, the local verification scorer and the mock agent.
func New(db repository.Database, options ...Deps) Service {
	deps := Deps{}
	if len(options) > 0 {
		deps = options[0]
	}
	if deps.Search == nil {
		deps.Search = search.New(db, search.Deps{})
	}
	if deps.Items == nil {
		deps.Items = dbItemReader{db: db}
	}
	if deps.Claim == nil {
		deps.Claim = claim.New(db)
	}
	if deps.Engine == nil {
		deps.Engine = claim.LocalEngine()
	}
	if deps.Places == nil {
		deps.Places = dbPickupLocator{db: db}
	}
	if deps.Client == nil && strings.TrimSpace(deps.DeepSeekKey) != "" {
		url := strings.TrimSpace(deps.DeepSeekURL)
		if url == "" {
			url = "https://api.deepseek.com"
		}
		model := strings.TrimSpace(deps.DeepSeekModel)
		if model == "" {
			model = "deepseek-v4.1-flash"
		}
		timeout := deps.DeepSeekTimeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		deps.Client = deepseek.New(url, deps.DeepSeekKey, model, timeout).WithTrace(deps.Trace, "agent")
	}
	rounds := deps.MaxToolRounds
	if rounds <= 0 || rounds > maxToolRounds {
		rounds = defaultMaxToolRounds
	}

	return &service{
		deps:   deps,
		rounds: rounds,
	}
}

// dbItemReader is the default ItemReader. The repository is the only trusted
// path to the private aggregate.
type dbItemReader struct {
	db repository.Database
}

func (r dbItemReader) ItemForAgent(ctx context.Context, itemID string) (itemdomain.Item, error) {
	row, err := r.db.GetItemByID(ctx, itemID)
	if err != nil {
		return itemdomain.Item{}, err
	}
	return itemdomain.FromRecord(row)
}

// dbPickupLocator is the default PickupLocator. Intermediaries are public
// civic points: only their name, address and hours reach the searcher.
type dbPickupLocator struct {
	db repository.Database
}

func (l dbPickupLocator) GetIntermediaryByID(ctx context.Context, id string) (record.IntermediaryRecord, error) {
	return l.db.GetIntermediaryByID(ctx, id)
}
