package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

type ParseEngine interface {
	Parse(rawTitle string) (*model.ParseResult, error)
}

type LoadConfigFunc func(configDir string) (*config.Config, error)
type NewParserFunc func(cfg *config.Config) ParseEngine

type runtimeState struct {
	cfg        *config.Config
	batch      config.BatchServiceConfig
	parserPool []ParseEngine
}

type inflightLimiter struct {
	sem chan struct{}
}

type Server struct {
	configDir string
	in        io.Reader
	out       io.Writer
	logger    *log.Logger

	loadConfig LoadConfigFunc
	newParser  NewParserFunc

	runtimeMu sync.RWMutex
	runtime   *runtimeState

	limiter atomic.Pointer[inflightLimiter]
	writeMu sync.Mutex
	wg      sync.WaitGroup
}

func NewServer(configDir string, in io.Reader, out io.Writer, logger *log.Logger) *Server {
	if strings.TrimSpace(configDir) == "" {
		configDir = "configs"
	}
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	if logger == nil {
		logger = log.New(io.Discard, "", log.LstdFlags)
	}
	return &Server{
		configDir:  configDir,
		in:         in,
		out:        out,
		logger:     logger,
		loadConfig: config.LoadAll,
		newParser: func(cfg *config.Config) ParseEngine {
			return parser.New(cfg)
		},
	}
}

func (s *Server) SetDependencies(loadConfig LoadConfigFunc, newParser NewParserFunc) {
	if loadConfig != nil {
		s.loadConfig = loadConfig
	}
	if newParser != nil {
		s.newParser = newParser
	}
}

func (s *Server) Run(ctx context.Context) error {
	if _, err := s.reloadRuntime(); err != nil {
		return err
	}

	lineCh := make(chan string)
	scanErrCh := make(chan error, 1)
	go s.scanInput(ctx, lineCh, scanErrCh)

	for {
		select {
		case <-ctx.Done():
			s.logger.Printf("shutdown requested: %v", ctx.Err())
			s.wg.Wait()
			return nil
		case line, ok := <-lineCh:
			if !ok {
				s.wg.Wait()
				err := <-scanErrCh
				if err != nil {
					return err
				}
				return nil
			}
			limiter, err := s.acquireInflight(ctx)
			if err != nil {
				return nil
			}
			s.wg.Add(1)
			go func(line string, limiter *inflightLimiter) {
				defer s.wg.Done()
				defer s.releaseInflight(limiter)
				s.handleLine(ctx, line)
			}(line, limiter)
		}
	}
}

func (s *Server) scanInput(ctx context.Context, lineCh chan<- string, scanErrCh chan<- error) {
	defer close(lineCh)
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		select {
		case <-ctx.Done():
			scanErrCh <- nil
			return
		case lineCh <- line:
		}
	}
	scanErrCh <- scanner.Err()
}

func (s *Server) handleLine(parent context.Context, line string) {
	var req protocolRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		s.writeProtocol(protocolResponse{
			OK: false,
			Error: &protocolError{
				Code:    "bad_request",
				Message: "invalid JSON request",
			},
		})
		return
	}

	req.ID = strings.TrimSpace(req.ID)
	req.Method = strings.TrimSpace(req.Method)
	if req.ID == "" {
		s.writeProtocol(protocolResponse{
			ID: req.ID,
			OK: false,
			Error: &protocolError{
				Code:    "bad_request",
				Message: "id is required",
			},
		})
		return
	}
	if req.Method == "" {
		s.writeProtocol(protocolResponse{
			ID: req.ID,
			OK: false,
			Error: &protocolError{
				Code:    "bad_request",
				Message: "method is required",
			},
		})
		return
	}

	switch req.Method {
	case "health":
		s.handleHealth(req.ID)
	case "reload":
		s.handleReload(req.ID)
	case "parse_batch":
		s.handleParseBatch(parent, req)
	default:
		s.writeError(req.ID, "method_not_found", fmt.Sprintf("unsupported method: %s", req.Method))
	}
}

func (s *Server) handleHealth(id string) {
	rt := s.currentRuntime()
	if rt == nil {
		s.writeError(id, "internal_error", "runtime not initialized")
		return
	}

	s.writeProtocol(protocolResponse{
		ID: id,
		OK: true,
		Result: map[string]any{
			"status":             "ok",
			"batch_max_size":     rt.batch.BatchMaxSize,
			"worker_count":       rt.batch.WorkerCount,
			"request_timeout_ms": rt.batch.RequestTimeoutMs,
			"max_inflight":       rt.batch.MaxInflight,
		},
	})
}

func (s *Server) handleReload(id string) {
	rt, err := s.reloadRuntime()
	if err != nil {
		s.writeError(id, "reload_failed", err.Error())
		return
	}
	s.writeProtocol(protocolResponse{
		ID: id,
		OK: true,
		Result: map[string]any{
			"reloaded":           true,
			"batch_max_size":     rt.batch.BatchMaxSize,
			"worker_count":       rt.batch.WorkerCount,
			"request_timeout_ms": rt.batch.RequestTimeoutMs,
			"max_inflight":       rt.batch.MaxInflight,
		},
	})
}

func (s *Server) handleParseBatch(parent context.Context, req protocolRequest) {
	rt := s.currentRuntime()
	if rt == nil {
		s.writeError(req.ID, "internal_error", "runtime not initialized")
		return
	}

	var params ParseBatchParams
	if len(req.Params) == 0 {
		s.writeError(req.ID, "bad_request", "params are required")
		return
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, "bad_request", "invalid params")
		return
	}

	reqCtx := parent
	cancel := func() {}
	if rt.batch.RequestTimeoutMs > 0 {
		reqCtx, cancel = context.WithTimeout(parent, time.Duration(rt.batch.RequestTimeoutMs)*time.Millisecond)
	}
	defer cancel()

	result, pErr := rt.parseBatch(reqCtx, params)
	if pErr != nil {
		s.writeError(req.ID, pErr.Code, pErr.Message)
		return
	}
	s.writeProtocol(protocolResponse{
		ID:     req.ID,
		OK:     true,
		Result: result,
	})
}

func (s *Server) writeError(id string, code string, message string) {
	s.writeProtocol(protocolResponse{
		ID: id,
		OK: false,
		Error: &protocolError{
			Code:    code,
			Message: message,
		},
	})
}

func (s *Server) writeProtocol(resp protocolResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Printf("marshal response failed: %v", err)
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.out.Write(data)
	_, _ = s.out.Write([]byte("\n"))
}

func (s *Server) currentRuntime() *runtimeState {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtime
}

func (s *Server) reloadRuntime() (*runtimeState, error) {
	cfg, err := s.loadConfig(s.configDir)
	if err != nil {
		return nil, err
	}
	cfg.BatchService.ApplyDefaults()
	workerCount := cfg.BatchService.WorkerCount
	if workerCount <= 0 {
		workerCount = 1
	}

	pool := make([]ParseEngine, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		pool = append(pool, s.newParser(cfg))
	}
	rt := &runtimeState{
		cfg:        cfg,
		batch:      cfg.BatchService,
		parserPool: pool,
	}

	s.runtimeMu.Lock()
	s.runtime = rt
	s.runtimeMu.Unlock()
	s.updateLimiter(cfg.BatchService.MaxInflight)

	s.logger.Printf("runtime loaded: workers=%d batch_max=%d timeout_ms=%d max_inflight=%d",
		rt.batch.WorkerCount, rt.batch.BatchMaxSize, rt.batch.RequestTimeoutMs, rt.batch.MaxInflight)
	return rt, nil
}

func (s *Server) updateLimiter(maxInflight int) {
	if maxInflight <= 0 {
		maxInflight = 1
	}
	s.limiter.Store(&inflightLimiter{
		sem: make(chan struct{}, maxInflight),
	})
}

func (s *Server) acquireInflight(ctx context.Context) (*inflightLimiter, error) {
	limiter := s.limiter.Load()
	if limiter == nil {
		return nil, errors.New("limiter unavailable")
	}
	select {
	case limiter.sem <- struct{}{}:
		return limiter, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) releaseInflight(limiter *inflightLimiter) {
	if limiter == nil {
		return
	}
	select {
	case <-limiter.sem:
	default:
	}
}

func (rt *runtimeState) parseBatch(ctx context.Context, params ParseBatchParams) (*ParseBatchResult, *protocolError) {
	itemCount := len(params.Items)
	if itemCount == 0 {
		return nil, &protocolError{Code: "invalid_params", Message: "items must not be empty"}
	}
	if itemCount > rt.batch.BatchMaxSize {
		return nil, &protocolError{
			Code:    "batch_too_large",
			Message: fmt.Sprintf("items exceed batch_max_size=%d", rt.batch.BatchMaxSize),
		}
	}

	type task struct {
		key     string
		title   string
		indices []int
	}
	type taskResult struct {
		key    string
		parsed *model.ParseResult
		score  float64
		err    error
	}

	keyByIndex := make([]string, itemCount)
	taskMap := make(map[string]*task, itemCount)
	order := make([]string, 0, itemCount)
	for i := range params.Items {
		title := strings.TrimSpace(params.Items[i].Title)
		if title == "" {
			return nil, &protocolError{
				Code:    "invalid_params",
				Message: fmt.Sprintf("items[%d].title is required", i),
			}
		}
		keyByIndex[i] = title
		if t, ok := taskMap[title]; ok {
			t.indices = append(t.indices, i)
			continue
		}
		taskMap[title] = &task{
			key:     title,
			title:   title,
			indices: []int{i},
		}
		order = append(order, title)
	}

	tasks := make([]task, 0, len(order))
	for _, key := range order {
		tasks = append(tasks, *taskMap[key])
	}

	workers := rt.batch.WorkerCount
	if workers <= 0 {
		workers = 1
	}
	if workers > len(rt.parserPool) {
		workers = len(rt.parserPool)
	}
	if workers <= 0 {
		workers = 1
	}

	jobs := make(chan task)
	results := make(chan taskResult, len(tasks))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		engine := rt.parserPool[i%len(rt.parserPool)]
		wg.Add(1)
		go func(p ParseEngine) {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				parsed, err := p.Parse(job.title)
				res := taskResult{
					key:    job.key,
					parsed: parsed,
					score:  scoreForResult(parsed),
					err:    err,
				}
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}(engine)
	}

	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- task:
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	resultByKey := make(map[string]taskResult, len(tasks))
	for len(resultByKey) < len(tasks) {
		select {
		case <-ctx.Done():
			<-feedDone
			return nil, &protocolError{
				Code:    "request_timeout",
				Message: "parse_batch request timed out",
			}
		case tr, ok := <-results:
			if !ok {
				if len(resultByKey) < len(tasks) {
					return nil, &protocolError{
						Code:    "internal_error",
						Message: "batch processing interrupted",
					}
				}
				break
			}
			resultByKey[tr.key] = tr
		}
	}

	out := ParseBatchResult{
		Items: make([]ParseBatchItemResult, itemCount),
	}
	bestScore := -1.0
	bestIndex := -1
	for i := range params.Items {
		item := ParseBatchItemResult{
			Index: i,
			Site:  strings.TrimSpace(params.Items[i].Site),
		}
		if params.Options.IncludeRaw {
			item.Raw = params.Items[i].Title
		}

		tr := resultByKey[keyByIndex[i]]
		if tr.err != nil {
			item.Error = &protocolError{
				Code:    "parse_failed",
				Message: tr.err.Error(),
			}
			out.Items[i] = item
			continue
		}

		item.Parsed = tr.parsed
		if params.Options.IncludeScore {
			item.Score = tr.score
		}
		out.Items[i] = item
		if params.Options.BestPick && tr.score > bestScore {
			bestScore = tr.score
			bestIndex = i
		}
	}
	if params.Options.BestPick {
		out.BestIndex = &bestIndex
	}

	return &out, nil
}

func scoreForResult(res *model.ParseResult) float64 {
	if res == nil {
		return 0
	}
	content := clamp01(res.ContentConfidence)
	template := clamp01(res.TemplateConfidence)
	score := 0.6*content + 0.4*template
	if score <= 0 {
		if len(res.Attributes) > 0 {
			score = 0.5
		} else {
			score = 0.1
		}
	}
	return math.Round(score*1000) / 1000
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
