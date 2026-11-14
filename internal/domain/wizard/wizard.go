/*
Package wizard orchestrates the finder enrollment wizard. D3 classifies the
photos and extracts the structured attributes used for ownership verification;
DeepSeek sees the photo and fills the editable card (brand, model, colour,
condition) together with the specific private details. Mapbox supplies the zone
and walking distances.

The wizard never persists the analyzed bytes: it is a UX layer, while the
existing item enrollment remains the only write and the policy authority.
*/
package wizard

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// AnalyzeInput is one wizard analysis request. Slot/Note are advisory and
// never contain private data beyond the finder's own note.
type AnalyzeInput struct {
	Photos []item.ImageInput
	Slot   string
	Note   string
}

// PhotoSlot is one additional photo the finder should take. The first photo is
// already uploaded and is not repeated here.
type PhotoSlot struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Instruction string `json:"instruction"`
	Required    bool   `json:"required"`
}

// Prefill is the editable card-1 prefill.
type Prefill struct {
	Category  string `json:"category"`
	Brand     string `json:"brand"`
	Model     string `json:"model"`
	Color     string `json:"color"`
	Condition string `json:"condition"`
}

// PlannerInput is the data DeepSeek receives: the already-classified public
// attributes plus the first photos. Photos are sent as multimodal parts and
// are excluded from the JSON text payload.
type PlannerInput struct {
	Category   string            `json:"category"`
	Color      string            `json:"color"`
	Condition  string            `json:"condition"`
	CaseColor  string            `json:"caseColor"`
	HasCrack   bool              `json:"hasCrack"`
	HasSticker bool              `json:"hasSticker"`
	HasCase    bool              `json:"hasCase"`
	IsDocument bool              `json:"isDocument"`
	PhotoCount int               `json:"photoCount"`
	Photos     []item.ImageInput `json:"-"`
}

// Plan is the DeepSeek planner output. The service normalizes and truncates it
// before it reaches the finder.
type Plan struct {
	PhotoPlan      []PhotoSlot `json:"photoPlan"`
	Prefill        Prefill     `json:"prefill"`
	PrivatePrefill string      `json:"privatePrefill"`
	Warnings       []string    `json:"warnings"`
}

// Analysis is the wizard response.
type Analysis struct {
	ObjectType        string
	Confidence        float64
	PhotoPlan         []PhotoSlot
	Prefill           Prefill
	PrivatePrefill    string
	PrivateAttributes map[string]any
	Warnings          []string
	Source            string
	IsDocument        bool
	Sensitive         bool
}

// DropPoint is one active partner with its walking distance from the report.
type DropPoint struct {
	ID              string
	Name            string
	Address         string
	Hours           string
	DistanceMeters  float64
	DurationSeconds float64
	WalkingMinutes  int
	Approximate     bool
	Source          string
}

// Analyzer is the D3 classification port.
type Analyzer interface {
	Analyze(ctx context.Context, photos []item.ImageInput, form item.CatalogForm) (item.CatalogResult, error)
}

// Planner is the DeepSeek vision/text port used for the editable prefill.
type Planner interface {
	Plan(ctx context.Context, input PlannerInput) (Plan, error)
}

// Router is the walking-distance port.
type Router interface {
	Walking(ctx context.Context, origin mapbox.Point, destinations []mapbox.Point) ([]mapbox.Leg, error)
}

// Geocoder is the reverse-geocoding port.
type Geocoder interface {
	Reverse(ctx context.Context, at mapbox.Point) (mapbox.Place, error)
}

// Deps are the wizard ports. Items is required for analysis; the other ports
// have documented fallbacks.
type Deps struct {
	Items    Analyzer
	Planner  Planner
	Router   Router
	Geocoder Geocoder
}

// Service is the finder wizard surface.
type Service interface {
	Analyze(ctx context.Context, input AnalyzeInput) (Analysis, error)
	Nearest(ctx context.Context, at *mapbox.Point, limit int) ([]DropPoint, error)
	Reverse(ctx context.Context, at mapbox.Point) (mapbox.Place, error)
}

// New returns a wizard service. The database is used by Nearest to read the
// active intermediaries.
func New(db repository.Database, deps Deps) Service {
	return &service{db: db, deps: deps}
}

type service struct {
	db   repository.Database
	deps Deps
}
