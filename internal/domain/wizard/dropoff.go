package wizard

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
)

// Nearest and Reverse are implemented by Task 4. Keeping the service surface
// complete in Task 3 lets the handler and tests compile without the drop-off
// provider.
func (s *service) Nearest(context.Context, mapbox.Point, int) ([]DropPoint, error) {
	return []DropPoint{}, nil
}

func (s *service) Reverse(context.Context, mapbox.Point) (mapbox.Place, error) {
	return mapbox.Place{Source: "unavailable"}, nil
}
