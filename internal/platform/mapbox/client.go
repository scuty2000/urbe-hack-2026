/*
Package mapbox is a small server-side client for the Mapbox Geocoding and
Directions Matrix APIs. The finder wizard uses it to turn the browser position
into a found zone and to rank the active drop-off points by walking time.

The token is read from the server environment and never reaches the browser.
Calls are deliberately bounded and fail closed: a missing token, a non-OK
response or a timeout is reported as errs.ErrUnavailable so the wizard can fall
back to manual entry or the haversine distance.
*/
package mapbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

const (
	// defaultBaseURL is the public Mapbox API host.
	defaultBaseURL = "https://api.mapbox.com"
	// defaultTimeout bounds a single provider call when the caller passes zero.
	defaultTimeout = 5 * time.Second
	// maxResponseBytes bounds the JSON read from Mapbox.
	maxResponseBytes = 1 << 20
)

// Point is a WGS84 coordinate.
type Point struct {
	Lat float64
	Lng float64
}

// Place is the reverse-geocoded zone. Source is "mapbox" on a provider answer
// and "unavailable" when the wizard had to fall back to manual entry.
type Place struct {
	Zone      string
	PlaceName string
	Source    string
}

// Leg is one walking distance/duration pair from the origin.
type Leg struct {
	DistanceMeters  float64
	DurationSeconds float64
}

// Client calls Mapbox over HTTPS.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New returns a Mapbox client. An empty token makes every call return
// errs.ErrUnavailable; the wizard then uses its documented fallbacks.
func New(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: timeout},
	}
}

// Reverse resolves a neighborhood/locality for the coordinate.
func (c *Client) Reverse(ctx context.Context, at Point) (Place, error) {
	if c == nil || c.token == "" {
		return Place{}, fmt.Errorf("mapbox reverse: token is not configured: %w", errs.ErrUnavailable)
	}

	coordinate := formatCoordinate(at.Lng, at.Lat)
	endpoint := c.baseURL + "/geocoding/v5/mapbox.places/" + coordinate + ".json"
	query := url.Values{}
	query.Set("access_token", c.token)
	query.Set("types", "neighborhood,locality,place")
	query.Set("language", "it")
	query.Set("limit", "1")

	var decoded struct {
		Features []struct {
			Text      string `json:"text"`
			PlaceName string `json:"place_name"`
		} `json:"features"`
	}
	if err := c.getJSON(ctx, endpoint, query, &decoded); err != nil {
		return Place{}, err
	}

	place := Place{Source: "mapbox"}
	if len(decoded.Features) == 0 {
		return place, nil
	}
	place.Zone = strings.TrimSpace(decoded.Features[0].Text)
	place.PlaceName = strings.TrimSpace(decoded.Features[0].PlaceName)
	return place, nil
}

// Walking is the wizard Router port: one leg per destination, in order.
func (c *Client) Walking(ctx context.Context, origin Point, destinations []Point) ([]Leg, error) {
	return c.WalkingMatrix(ctx, origin, destinations)
}

// WalkingMatrix returns one leg per destination, in the same order. Up to 24
// destinations are supported by one Mapbox call; the wizard pre-filters above
// that limit.
func (c *Client) WalkingMatrix(ctx context.Context, origin Point, destinations []Point) ([]Leg, error) {
	if c == nil || c.token == "" {
		return nil, fmt.Errorf("mapbox matrix: token is not configured: %w", errs.ErrUnavailable)
	}
	if len(destinations) == 0 {
		return []Leg{}, nil
	}

	coordinates := make([]string, 0, len(destinations)+1)
	coordinates = append(coordinates, formatCoordinate(origin.Lng, origin.Lat))
	for _, destination := range destinations {
		coordinates = append(coordinates, formatCoordinate(destination.Lng, destination.Lat))
	}
	endpoint := c.baseURL + "/directions-matrix/v1/mapbox/walking/" + strings.Join(coordinates, ";")
	query := url.Values{}
	query.Set("access_token", c.token)
	query.Set("sources", "0")
	// Without an explicit destinations list Mapbox returns a square matrix
	// that includes the source itself as column 0. Ask only for the real
	// destinations so row 0 aligns one-to-one with the input slice.
	destinationIndexes := make([]string, len(destinations))
	for i := range destinations {
		destinationIndexes[i] = strconv.Itoa(i + 1)
	}
	query.Set("destinations", strings.Join(destinationIndexes, ";"))
	query.Set("annotations", "distance,duration")

	var decoded struct {
		Code      string       `json:"code"`
		Distances [][]*float64 `json:"distances"`
		Durations [][]*float64 `json:"durations"`
	}
	if err := c.getJSON(ctx, endpoint, query, &decoded); err != nil {
		return nil, err
	}
	if decoded.Code != "Ok" || len(decoded.Distances) == 0 || len(decoded.Durations) == 0 {
		return nil, fmt.Errorf("mapbox matrix: provider returned code %q: %w", decoded.Code, errs.ErrUnavailable)
	}
	if len(decoded.Distances[0]) < len(destinations) || len(decoded.Durations[0]) < len(destinations) {
		return nil, fmt.Errorf("mapbox matrix: incomplete matrix: %w", errs.ErrUnavailable)
	}

	legs := make([]Leg, 0, len(destinations))
	for i := range destinations {
		distance := decoded.Distances[0][i]
		duration := decoded.Durations[0][i]
		if distance == nil || duration == nil {
			return nil, fmt.Errorf("mapbox matrix: unreachable destination: %w", errs.ErrUnavailable)
		}
		legs = append(legs, Leg{DistanceMeters: *distance, DurationSeconds: *duration})
	}
	return legs, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, query url.Values, target any) error {
	requestURL := endpoint + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("mapbox request: %w", errs.ErrUnavailable)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mapbox request failed: %w", errs.ErrUnavailable)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("mapbox request: unexpected status %d: %w", resp.StatusCode, errs.ErrUnavailable)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(target); err != nil {
		return fmt.Errorf("mapbox request: decoding response: %w", errs.ErrUnavailable)
	}
	return nil
}

func formatCoordinate(lng, lat float64) string {
	return strconv.FormatFloat(lng, 'f', 6, 64) + "," + strconv.FormatFloat(lat, 'f', 6, 64)
}
