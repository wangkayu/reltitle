package config

import "testing"

func TestLoadAll_LoadsBatchServiceConfig(t *testing.T) {
	cfg, err := LoadAll("../../configs")
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if cfg.BatchService.BatchMaxSize <= 0 {
		t.Fatalf("batch_max_size not loaded")
	}
	if cfg.BatchService.WorkerCount <= 0 {
		t.Fatalf("worker_count not loaded")
	}
	if cfg.BatchService.RequestTimeoutMs <= 0 {
		t.Fatalf("request_timeout_ms not loaded")
	}
	if cfg.BatchService.MaxInflight <= 0 {
		t.Fatalf("max_inflight not loaded")
	}
}
