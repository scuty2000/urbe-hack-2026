package main

import "testing"

func TestApplyMapboxToken(t *testing.T) {
	t.Setenv("MAPBOX_TOKEN", " provider-token ")

	var cfg WebAPIConfiguration
	applyMapboxToken(&cfg)
	if cfg.Mapbox.Token != "provider-token" {
		t.Fatalf("token = %q, want provider-token", cfg.Mapbox.Token)
	}

	cfg.Mapbox.Token = "api-token"
	applyMapboxToken(&cfg)
	if cfg.Mapbox.Token != "api-token" {
		t.Fatalf("token = %q, want the API_MAPBOX_TOKEN value to win", cfg.Mapbox.Token)
	}
}
