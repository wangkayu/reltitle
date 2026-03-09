package config

import "testing"

func TestLoadAll_LoadsRouteFamilies(t *testing.T) {
	cfg, err := LoadAll("../../configs")
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(cfg.RouteFamilies.Families) == 0 {
		t.Fatalf("expected route families to be loaded")
	}
}
