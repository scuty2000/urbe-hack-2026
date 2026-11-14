package wizard

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
)

const (
	defaultNearestLimit = 5
	maxNearestLimit     = 10
	walkingMetersPerSec = 1.3
)

// Nearest returns the active drop-off points sorted by walking duration.
// Mapbox supplies real walking legs; a provider failure falls back to the
// documented haversine estimate, never to an error.
func (s *service) Nearest(ctx context.Context, at mapbox.Point, limit int) ([]DropPoint, error) {
	if limit <= 0 {
		limit = defaultNearestLimit
	}
	if limit > maxNearestLimit {
		limit = maxNearestLimit
	}

	rows, err := s.db.ListIntermediaries(ctx)
	if err != nil {
		return nil, fmt.Errorf("nearest drop points: %w", err)
	}
	active := make([]DropPoint, 0, len(rows))
	destinations := make([]mapbox.Point, 0, len(rows))
	for _, row := range rows {
		if row.Status != "active" || (row.GeoLat == 0 && row.GeoLng == 0) {
			continue
		}
		active = append(active, DropPoint{
			ID:      row.ID,
			Name:    row.Name,
			Address: row.Address,
			Hours:   row.Hours,
		})
		destinations = append(destinations, mapbox.Point{Lat: row.GeoLat, Lng: row.GeoLng})
	}
	if len(active) == 0 {
		return []DropPoint{}, nil
	}

	if s.deps.Router != nil {
		legs, walkErr := s.deps.Router.Walking(ctx, at, destinations)
		if walkErr == nil && len(legs) == len(active) {
			for i := range active {
				active[i].DistanceMeters = legs[i].DistanceMeters
				active[i].DurationSeconds = legs[i].DurationSeconds
				active[i].WalkingMinutes = walkingMinutes(legs[i].DurationSeconds)
				active[i].Source = "mapbox"
			}
			return sortAndCap(active, limit), nil
		}
	}

	for i, destination := range destinations {
		distance := haversineMeters(at, destination)
		duration := distance / walkingMetersPerSec
		active[i].DistanceMeters = distance
		active[i].DurationSeconds = duration
		active[i].WalkingMinutes = walkingMinutes(duration)
		active[i].Approximate = true
		active[i].Source = "haversine"
	}
	return sortAndCap(active, limit), nil
}

// Reverse resolves the zone. A provider failure leaves the zone empty and
// marks the source as unavailable; the finder can type it.
func (s *service) Reverse(ctx context.Context, at mapbox.Point) (mapbox.Place, error) {
	if s.deps.Geocoder == nil {
		return mapbox.Place{Source: "unavailable"}, nil
	}
	place, err := s.deps.Geocoder.Reverse(ctx, at)
	if err != nil {
		return mapbox.Place{Source: "unavailable"}, nil
	}
	if place.Source == "" {
		place.Source = "mapbox"
	}
	return place, nil
}

func sortAndCap(points []DropPoint, limit int) []DropPoint {
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].DurationSeconds != points[j].DurationSeconds {
			return points[i].DurationSeconds < points[j].DurationSeconds
		}
		if points[i].DistanceMeters != points[j].DistanceMeters {
			return points[i].DistanceMeters < points[j].DistanceMeters
		}
		return points[i].Name < points[j].Name
	})
	if len(points) > limit {
		points = points[:limit]
	}
	return points
}

func walkingMinutes(seconds float64) int {
	if seconds <= 0 {
		return 0
	}
	return int(math.Ceil(seconds / 60))
}

func haversineMeters(a, b mapbox.Point) float64 {
	const earthRadius = 6371000.0
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(h))
}
