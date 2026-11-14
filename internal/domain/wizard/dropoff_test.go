package wizard_test

import (
	"context"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

type fakeRouter struct {
	legs         []mapbox.Leg
	byPoint      map[mapbox.Point]mapbox.Leg
	err          error
	calls        int
	destinations []mapbox.Point
}

func (f *fakeRouter) Walking(_ context.Context, _ mapbox.Point, destinations []mapbox.Point) ([]mapbox.Leg, error) {
	f.calls++
	f.destinations = destinations
	if f.byPoint != nil {
		legs := make([]mapbox.Leg, 0, len(destinations))
		for _, destination := range destinations {
			leg, ok := f.byPoint[destination]
			if !ok {
				leg = mapbox.Leg{DistanceMeters: 800, DurationSeconds: 600}
			}
			legs = append(legs, leg)
		}
		return legs, f.err
	}
	return f.legs, f.err
}

type fakeGeocoder struct {
	place mapbox.Place
	err   error
}

func (f *fakeGeocoder) Reverse(context.Context, mapbox.Point) (mapbox.Place, error) {
	return f.place, f.err
}

func TestNearestUsesWalkingLegsAndSorts(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	router := &fakeRouter{byPoint: map[mapbox.Point]mapbox.Leg{
		{Lat: 41.9010, Lng: 12.5015}: {DistanceMeters: 160, DurationSeconds: 120},
		{Lat: 41.9080, Lng: 12.4620}: {DistanceMeters: 240, DurationSeconds: 180},
	}}
	svc := wizard.New(db, wizard.Deps{Router: router})

	points, err := svc.Nearest(context.Background(), &mapbox.Point{Lat: 41.9, Lng: 12.5}, 2)
	if err != nil {
		t.Fatalf("Nearest() error = %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
	if points[0].ID != "tabaccheria-termini" || points[0].WalkingMinutes != 2 {
		t.Fatalf("first = %+v", points[0])
	}
	if points[1].ID != "punto-prati" || points[1].WalkingMinutes != 3 {
		t.Fatalf("second = %+v", points[1])
	}
	if points[0].Approximate || points[0].Source != "mapbox" {
		t.Fatalf("first source = %+v", points[0])
	}
	if router.calls != 1 || len(router.destinations) != 7 {
		t.Fatalf("router calls=%d destinations=%d", router.calls, len(router.destinations))
	}
}

func TestNearestWithoutLocationSortsByName(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := wizard.New(db, wizard.Deps{})

	points, err := svc.Nearest(context.Background(), nil, 5)
	if err != nil {
		t.Fatalf("Nearest() error = %v", err)
	}
	if len(points) != 5 {
		t.Fatalf("points = %d, want 5", len(points))
	}
	for i, point := range points {
		if point.Source != "none" || !point.Approximate {
			t.Fatalf("point = %+v, want the no-location mode", point)
		}
		if point.DistanceMeters != 0 || point.WalkingMinutes != 0 {
			t.Fatalf("point = %+v, want no distance", point)
		}
		if i > 0 && points[i-1].Name > point.Name {
			t.Fatalf("points are not sorted by name: %q > %q", points[i-1].Name, point.Name)
		}
	}
}

func TestNearestFallsBackToHaversine(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	svc := wizard.New(db, wizard.Deps{Router: &fakeRouter{err: errs.ErrUnavailable}})
	points, err := svc.Nearest(context.Background(), &mapbox.Point{Lat: 41.9010, Lng: 12.5140}, 5)
	if err != nil {
		t.Fatalf("Nearest() error = %v", err)
	}
	if len(points) != 5 {
		t.Fatalf("points = %d, want 5", len(points))
	}
	for _, point := range points {
		if !point.Approximate || point.Source != "haversine" {
			t.Fatalf("point = %+v, want approximate haversine", point)
		}
		if point.WalkingMinutes <= 0 || point.DistanceMeters <= 0 {
			t.Fatalf("point distances = %+v", point)
		}
	}
	if points[0].ID != "bar-x" {
		t.Fatalf("nearest = %q, want bar-x", points[0].ID)
	}
}

func TestNearestFiltersInactivePoints(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	if err := db.CreateIntermediary(context.Background(), inactiveRecord()); err != nil {
		t.Fatalf("CreateIntermediary() error = %v", err)
	}

	svc := wizard.New(db, wizard.Deps{Router: &fakeRouter{legs: make([]mapbox.Leg, 7)}})
	points, err := svc.Nearest(context.Background(), &mapbox.Point{Lat: 41.9, Lng: 12.5}, 10)
	if err != nil {
		t.Fatalf("Nearest() error = %v", err)
	}
	for _, point := range points {
		if point.ID == "inactive-point" {
			t.Fatal("inactive point was returned")
		}
	}
}

func TestReverseUsesGeocoderAndFallsBack(t *testing.T) {
	db := testutil.NewTestDB(t)
	svc := wizard.New(db, wizard.Deps{Geocoder: &fakeGeocoder{place: mapbox.Place{Zone: "San Lorenzo", PlaceName: "San Lorenzo, Roma", Source: "mapbox"}}})
	place, err := svc.Reverse(context.Background(), mapbox.Point{Lat: 41.9, Lng: 12.5})
	if err != nil || place.Zone != "San Lorenzo" || place.Source != "mapbox" {
		t.Fatalf("Reverse() = %+v, %v", place, err)
	}

	fallback := wizard.New(db, wizard.Deps{Geocoder: &fakeGeocoder{err: errs.ErrUnavailable}})
	place, err = fallback.Reverse(context.Background(), mapbox.Point{Lat: 41.9, Lng: 12.5})
	if err != nil || place.Source != "unavailable" || place.Zone != "" {
		t.Fatalf("fallback Reverse() = %+v, %v", place, err)
	}
}

func TestNearestReorderKeepsNames(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := wizard.New(db, wizard.Deps{})
	points, err := svc.Nearest(context.Background(), &mapbox.Point{Lat: 41.9, Lng: 12.5}, 5)
	if err != nil {
		t.Fatalf("Nearest() error = %v", err)
	}
	if len(points) != 5 {
		t.Fatalf("points = %d", len(points))
	}
	for _, point := range points {
		if point.Name == "" || point.Address == "" || point.Hours == "" {
			t.Fatalf("point = %+v", point)
		}
	}
}

func inactiveRecord() record.IntermediaryRecord {
	return record.IntermediaryRecord{
		ID:           "inactive-point",
		Name:         "Punto chiuso",
		Address:      "Via Chiusa 1, Roma",
		GeoLat:       41.9,
		GeoLng:       12.5,
		Hours:        "00:00-00:00",
		Capacity:     0,
		Status:       "inactive",
		AgreementRef: "none",
		Contact:      "closed@example.test",
	}
}
