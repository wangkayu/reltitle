package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

type fakeParser struct {
	parse func(title string) (*model.ParseResult, error)
}

func (f *fakeParser) Parse(title string) (*model.ParseResult, error) {
	return f.parse(title)
}

func newTestServer(out io.Writer, cfg config.BatchServiceConfig, factory NewParserFunc) *Server {
	s := NewServer("configs", strings.NewReader(""), out, log.New(io.Discard, "", 0))
	s.SetDependencies(
		func(_ string) (*config.Config, error) {
			cfg.ApplyDefaults()
			return &config.Config{
				BatchService: cfg,
			}, nil
		},
		factory,
	)
	if _, err := s.reloadRuntime(); err != nil {
		panic(err)
	}
	return s
}

func decodeLine(t *testing.T, line string) protocolResponse {
	t.Helper()
	var resp protocolResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, line=%s", err, line)
	}
	return resp
}

func toBatchResult(t *testing.T, v any) ParseBatchResult {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var out ParseBatchResult
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal batch result: %v", err)
	}
	return out
}

func TestProtocolParseBatch(t *testing.T) {
	var callCount atomic.Int32
	out := &bytes.Buffer{}
	s := newTestServer(out, config.BatchServiceConfig{
		BatchMaxSize:     16,
		WorkerCount:      4,
		RequestTimeoutMs: 5000,
		MaxInflight:      8,
	}, func(_ *config.Config) ParseEngine {
		return &fakeParser{
			parse: func(title string) (*model.ParseResult, error) {
				callCount.Add(1)
				return &model.ParseResult{
					RawTitle:           title,
					ContentKind:        "TV_EP",
					ContentConfidence:  0.9,
					TemplateID:         "TF_TEST",
					TemplateConfidence: 0.8,
				}, nil
			},
		}
	})

	req := `{"id":"req-1","method":"parse_batch","params":{"items":[{"title":"[A] X - 01","site":"s1"},{"title":"[A] X - 01","site":"s2"}],"options":{"best_pick":true,"include_raw":true,"include_score":true}}}`
	s.handleLine(context.Background(), req)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	resp := decodeLine(t, lines[0])
	if !resp.OK {
		t.Fatalf("response not ok: %+v", resp.Error)
	}
	if resp.ID != "req-1" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}

	result := toBatchResult(t, resp.Result)
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.BestIndex == nil || *result.BestIndex != 0 {
		t.Fatalf("unexpected best_index: %+v", result.BestIndex)
	}
	if result.Items[0].Parsed == nil || result.Items[0].Parsed.RawTitle != "[A] X - 01" {
		t.Fatalf("parsed field is not compatible with ParseResult output")
	}
	if result.Items[0].Raw == "" || result.Items[0].Score <= 0 {
		t.Fatalf("expected raw and score in response item: %+v", result.Items[0])
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected deduped parse call count=1, got %d", callCount.Load())
	}
}

func TestParseBatchDedup(t *testing.T) {
	var parseCalls atomic.Int32
	parser := &fakeParser{
		parse: func(title string) (*model.ParseResult, error) {
			parseCalls.Add(1)
			return &model.ParseResult{RawTitle: title, ContentConfidence: 0.7, TemplateConfidence: 0.7}, nil
		},
	}
	rt := &runtimeState{
		batch: config.BatchServiceConfig{
			BatchMaxSize:     16,
			WorkerCount:      3,
			RequestTimeoutMs: 5000,
			MaxInflight:      4,
		},
		parserPool: []ParseEngine{parser, parser, parser},
	}

	res, errResp := rt.parseBatch(context.Background(), ParseBatchParams{
		Items: []ParseBatchItemRequest{
			{Title: "t1"},
			{Title: "t2"},
			{Title: "t1"},
			{Title: "t2"},
			{Title: "t1"},
		},
	})
	if errResp != nil {
		t.Fatalf("parseBatch returned error: %+v", errResp)
	}
	if len(res.Items) != 5 {
		t.Fatalf("unexpected result count: %d", len(res.Items))
	}
	if parseCalls.Load() != 2 {
		t.Fatalf("expected 2 unique parse calls, got %d", parseCalls.Load())
	}
}

func TestParseBatchWorkerConcurrency(t *testing.T) {
	var current atomic.Int32
	var maxSeen atomic.Int32
	parser := &fakeParser{
		parse: func(title string) (*model.ParseResult, error) {
			n := current.Add(1)
			for {
				prev := maxSeen.Load()
				if n <= prev {
					break
				}
				if maxSeen.CompareAndSwap(prev, n) {
					break
				}
			}
			time.Sleep(80 * time.Millisecond)
			current.Add(-1)
			return &model.ParseResult{RawTitle: title, ContentConfidence: 0.8, TemplateConfidence: 0.8}, nil
		},
	}
	rt := &runtimeState{
		batch: config.BatchServiceConfig{
			BatchMaxSize:     16,
			WorkerCount:      4,
			RequestTimeoutMs: 5000,
			MaxInflight:      4,
		},
		parserPool: []ParseEngine{parser, parser, parser, parser},
	}

	_, errResp := rt.parseBatch(context.Background(), ParseBatchParams{
		Items: []ParseBatchItemRequest{
			{Title: "a"},
			{Title: "b"},
			{Title: "c"},
			{Title: "d"},
		},
	})
	if errResp != nil {
		t.Fatalf("parseBatch returned error: %+v", errResp)
	}
	if maxSeen.Load() < 2 {
		t.Fatalf("expected concurrent workers to run, max parallel=%d", maxSeen.Load())
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	inR, inW := io.Pipe()
	defer inR.Close()
	defer inW.Close()

	out := &bytes.Buffer{}
	s := NewServer("configs", inR, out, log.New(io.Discard, "", 0))
	s.SetDependencies(
		func(_ string) (*config.Config, error) {
			return &config.Config{
				BatchService: config.BatchServiceConfig{
					BatchMaxSize:     8,
					WorkerCount:      2,
					RequestTimeoutMs: 3000,
					MaxInflight:      2,
				},
			}, nil
		},
		func(_ *config.Config) ParseEngine {
			return &fakeParser{
				parse: func(title string) (*model.ParseResult, error) {
					time.Sleep(100 * time.Millisecond)
					return &model.ParseResult{RawTitle: title, ContentConfidence: 0.9, TemplateConfidence: 0.9}, nil
				},
			}
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	_, _ = fmt.Fprintln(inW, `{"id":"x1","method":"parse_batch","params":{"items":[{"title":"shutdown test"}]}}`)
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not shutdown gracefully")
	}
}

func TestErrorResponses(t *testing.T) {
	out := &bytes.Buffer{}
	s := newTestServer(out, config.BatchServiceConfig{
		BatchMaxSize:     4,
		WorkerCount:      1,
		RequestTimeoutMs: 1000,
		MaxInflight:      2,
	}, func(_ *config.Config) ParseEngine {
		return &fakeParser{
			parse: func(title string) (*model.ParseResult, error) {
				return &model.ParseResult{RawTitle: title}, nil
			},
		}
	})

	s.handleLine(context.Background(), "{")
	s.handleLine(context.Background(), `{"id":"u1","method":"unknown"}`)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d", len(lines))
	}

	resp0 := decodeLine(t, lines[0])
	if resp0.OK || resp0.Error == nil || resp0.Error.Code != "bad_request" {
		t.Fatalf("unexpected malformed-json response: %+v", resp0)
	}

	resp1 := decodeLine(t, lines[1])
	if resp1.OK || resp1.Error == nil || resp1.Error.Code != "method_not_found" {
		t.Fatalf("unexpected unknown-method response: %+v", resp1)
	}
}
