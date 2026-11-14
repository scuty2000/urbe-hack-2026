package mapbox_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
)

func TestReverseParsesFirstFeature(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"features":[{"text":"San Lorenzo","place_name":"San Lorenzo, Roma, Lazio, Italia"}]}`))
	}))
	defer server.Close()

	client := mapbox.New(server.URL, "test-token", time.Second)
	place, err := client.Reverse(context.Background(), mapbox.Point{Lat: 41.9028, Lng: 12.4964})
	if err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
	if place.Zone != "San Lorenzo" || place.PlaceName == "" || place.Source != "mapbox" {
		t.Fatalf("Reverse() = %+v", place)
	}
	if gotPath != "/geocoding/v5/mapbox.places/12.496400,41.902800.json" {
		t.Fatalf("path = %q", gotPath)
	}
	for _, want := range []string{"access_token=test-token", "language=it", "limit=1", "types=neighborhood%2Clocality%2Cplace"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestReverseEmptyFeaturesReturnsPlaceWithoutZone(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"features":[]}`))
	}))
	defer server.Close()

	place, err := mapbox.New(server.URL, "token", time.Second).Reverse(context.Background(), mapbox.Point{Lat: 1, Lng: 2})
	if err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
	if place.Zone != "" || place.Source != "mapbox" {
		t.Fatalf("Reverse() = %+v", place)
	}
}

func TestWalkingMatrixParsesFirstRow(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"code":"Ok","distances":[[120.5,340.0]],"durations":[[90.0,240.0]]}`))
	}))
	defer server.Close()

	legs, err := mapbox.New(server.URL, "test-token", time.Second).WalkingMatrix(
		context.Background(),
		mapbox.Point{Lat: 41.9, Lng: 12.5},
		[]mapbox.Point{{Lat: 41.91, Lng: 12.51}, {Lat: 41.92, Lng: 12.52}},
	)
	if err != nil {
		t.Fatalf("WalkingMatrix() error = %v", err)
	}
	if len(legs) != 2 || legs[0].DistanceMeters != 120.5 || legs[1].DurationSeconds != 240 {
		t.Fatalf("legs = %+v", legs)
	}
	if !strings.Contains(gotPath, "/directions-matrix/v1/mapbox/walking/12.500000,41.900000;12.510000,41.910000;12.520000,41.920000") {
		t.Fatalf("path = %q", gotPath)
	}
	for _, want := range []string{"access_token=test-token", "sources=0", "destinations=1%3B2", "annotations=distance%2Cduration"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestMissingTokenFailsClosedWithoutRequest(t *testing.T) {
	t.Parallel()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()

	_, err := mapbox.New(server.URL, "", time.Second).Reverse(context.Background(), mapbox.Point{})
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if called {
		t.Fatal("missing token still called the provider")
	}
}

func TestProviderErrorsAreUnavailableAndHideToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := mapbox.New(server.URL, "super-secret-token", time.Second).Reverse(context.Background(), mapbox.Point{Lat: 1, Lng: 2})
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatal("error leaks the token")
	}
}

func TestMatrixUnreachableLegFallsBack(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"Ok","distances":[[null]],"durations":[[null]]}`))
	}))
	defer server.Close()

	_, err := mapbox.New(server.URL, "token", time.Second).WalkingMatrix(
		context.Background(),
		mapbox.Point{Lat: 1, Lng: 2},
		[]mapbox.Point{{Lat: 3, Lng: 4}},
	)
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"features":[]}`))
	}))
	defer server.Close()

	_, err := mapbox.New(server.URL, "token", 10*time.Millisecond).Reverse(context.Background(), mapbox.Point{Lat: 1, Lng: 2})
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
