package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/wangkayu/reltitle/configs"
	"github.com/wangkayu/reltitle/pkg/config"
	"github.com/wangkayu/reltitle/pkg/model"
	"github.com/wangkayu/reltitle/pkg/parser"
)

// ServeProtocol names the request/response contract of `reltitle serve`. A
// consumer accepts only this protocol and only its own version line; anything
// else is a different parser it does not know how to prompt.
const ServeProtocol = "rt-serve-1"

const (
	// maxRequestBytes bounds one request line. A parse request carries one
	// title, so anything larger is a hostile or broken client.
	maxRequestBytes = 8 << 10
	// maxResponseBytes bounds one response line.
	maxResponseBytes = 256 << 10
	// maxServeTitleBytes bounds the title of one parse request.
	maxServeTitleBytes = 4096
)

// serveRequest is one line of stdin. Unknown fields are rejected: the protocol
// has exactly these fields.
type serveRequest struct {
	ID    int64  `json:"id"`
	Op    string `json:"op"`
	Title string `json:"title,omitempty"`
}

type serveResponse struct {
	ID     int64       `json:"id"`
	OK     bool        `json:"ok"`
	Result any         `json:"result,omitempty"`
	Error  *serveError `json:"error,omitempty"`
}

type serveError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// helloResult is what a consumer reads before it trusts any parse: the
// protocol it speaks, the build's version and line, and the digest of the
// dictionary bundle it carries.
type helloResult struct {
	Protocol     string `json:"protocol"`
	Version      string `json:"version"`
	Line         string `json:"line"`
	ConfigDigest string `json:"config_digest"`
}

// dimensionVocabulary is one attribute dimension's canonical values.
type dimensionVocabulary struct {
	Dimension string   `json:"dimension"`
	Values    []string `json:"values"`
}

type vocabularyResult struct {
	Dimensions []dimensionVocabulary `json:"dimensions"`
}

type serveServer struct {
	parser       *parser.Parser
	buildVersion string
	configDigest string
	vocabulary   []dimensionVocabulary
}

// newServeServer loads the embedded configuration once; every request is then
// served from that one bundle, so version, digest and parse results always
// describe the same build.
func newServeServer(buildVersion string) (*serveServer, error) {
	cfg, err := config.LoadAllFS(configs.FS)
	if err != nil {
		return nil, err
	}
	digest, err := config.BundleDigest(configs.FS)
	if err != nil {
		return nil, err
	}
	return &serveServer{
		parser:       parser.New(cfg),
		buildVersion: buildVersion,
		configDigest: digest,
		vocabulary:   vocabularyOf(cfg),
	}, nil
}

// serveServe runs the line protocol on stdin/stdout until stdin closes. Every
// request is answered on one line; a malformed request is answered with an
// error and keeps the process alive, while a broken stdin ends it.
func serveServe(buildVersion string, stdin io.Reader, stdout, stderr io.Writer) int {
	server, err := newServeServer(buildVersion)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 4096), maxRequestBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		response := server.respond(append([]byte(nil), line...))
		if len(response) > maxResponseBytes {
			response = marshalResponse(failure(0, "response_too_large", "the response exceeds the protocol limit"))
		}
		if _, err := stdout.Write(append(response, '\n')); err != nil {
			fmt.Fprintf(stderr, "write response: %v\n", err)
			return 1
		}
	}
	if err := scanner.Err(); err != nil {
		stdout.Write(append(marshalResponse(failure(0, "request_too_long", "a request line exceeds the protocol limit")), '\n'))
		fmt.Fprintf(stderr, "read request: %v\n", err)
		return 1
	}
	return 0
}

// respond turns one request line into one response line. A panic inside the
// parser becomes that request's error, never the process's death.
func (s *serveServer) respond(raw []byte) (response []byte) {
	var request serveRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return marshalResponse(failure(0, "bad_request", "the request is not a JSON object of this protocol"))
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			response = marshalResponse(failure(request.ID, "internal", fmt.Sprintf("parser panicked: %v", recovered)))
		}
	}()
	switch request.Op {
	case "hello":
		return marshalResponse(success(request.ID, helloResult{
			Protocol:     ServeProtocol,
			Version:      strings.TrimPrefix(s.buildVersion, "v"),
			Line:         lineOf(s.buildVersion),
			ConfigDigest: s.configDigest,
		}))
	case "vocabulary":
		return marshalResponse(success(request.ID, vocabularyResult{Dimensions: s.vocabulary}))
	case "parse":
		title := strings.TrimSpace(request.Title)
		if title == "" {
			return marshalResponse(failure(request.ID, "missing_title", "a parse request needs a title"))
		}
		if len(title) > maxServeTitleBytes {
			return marshalResponse(failure(request.ID, "title_too_long", "the title exceeds the protocol limit"))
		}
		result, err := s.parser.Parse(title)
		if err != nil {
			return marshalResponse(failure(request.ID, "parse_failed", err.Error()))
		}
		return marshalResponse(success(request.ID, result))
	default:
		return marshalResponse(failure(request.ID, "unknown_op", "unknown operation"))
	}
}

func success(id int64, result any) serveResponse {
	return serveResponse{ID: id, OK: true, Result: result}
}

func failure(id int64, code, message string) serveResponse {
	return serveResponse{ID: id, OK: false, Error: &serveError{Code: code, Message: message}}
}

func marshalResponse(response serveResponse) []byte {
	raw, err := json.Marshal(response)
	if err != nil {
		return []byte(`{"id":0,"ok":false,"error":{"code":"internal","message":"encode response"}}`)
	}
	return raw
}

// vocabularyDimensions lists the attribute dimensions a consumer may build an
// option list from: each one reports a closed set of values the parser can
// emit. Structural dimensions (contents, pack_kind, pack_profile), free-form
// ones (note, title, group) and dimensions whose values are open or
// synthesized in code (version, audio_object, audio_channels, sub_format,
// release_status) stay out, because their values cannot be enumerated here.
var vocabularyDimensions = map[model.AttrType]bool{
	model.AttrContentRating:  true,
	model.AttrSourceType:     true,
	model.AttrResolution:     true,
	model.AttrVideoCodec:     true,
	model.AttrAudioCodec:     true,
	model.AttrBitDepth:       true,
	model.AttrContainer:      true,
	model.AttrPlatform:       true,
	model.AttrPlatformRegion: true,
	model.AttrHDR:            true,
	model.AttrSubFlag:        true,
	model.AttrSubLang:        true,
	model.AttrSubLangGroup:   true,
	model.AttrAudioFlag:      true,
	model.AttrAudioLang:      true,
	model.AttrEdition:        true,
}

// vocabularyOf lists the canonical values of every published attribute
// dimension. A dimension that dict_canonical_values.yaml defines reports
// exactly those values; a dimension it does not define (content_rating,
// edition, …) reports the values the lexicon emits and dict_note_promotion.yaml
// produces. Promotions with a token-derived value cannot be listed and are
// skipped. Dimensions and values are sorted so the answer is stable across
// builds; a consumer reorders them for its own display.
func vocabularyOf(cfg *config.Config) []dimensionVocabulary {
	values := make(map[model.AttrType]map[string]bool)
	canonical := make(map[model.AttrType]bool)
	add := func(dimension model.AttrType, value string) {
		value = strings.TrimSpace(value)
		if dimension == "" || value == "" || !vocabularyDimensions[dimension] {
			return
		}
		if values[dimension] == nil {
			values[dimension] = make(map[string]bool)
		}
		values[dimension][value] = true
	}
	for dimension, raw := range cfg.DictCanonicalValues.Canonical {
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		canonical[model.AttrType(dimension)] = true
		for _, item := range items {
			if value, ok := item.(string); ok {
				add(model.AttrType(dimension), value)
			}
		}
	}
	for _, entry := range cfg.Lexicon.Entries {
		for _, emit := range entry.Emit {
			if !canonical[model.AttrType(emit.Type)] {
				add(model.AttrType(emit.Type), emit.Normalized)
			}
		}
		for _, clause := range entry.AnyOf {
			for _, emit := range clause.Emit {
				if !canonical[model.AttrType(emit.Type)] {
					add(model.AttrType(emit.Type), emit.Normalized)
				}
			}
		}
	}
	for _, rule := range cfg.DictNotePromotion.Rules {
		if !rule.PromoteTo.NormalizedFromToken && !canonical[model.AttrType(rule.PromoteTo.Type)] {
			add(model.AttrType(rule.PromoteTo.Type), rule.PromoteTo.Normalized)
		}
	}
	dimensions := make([]string, 0, len(values))
	for dimension := range values {
		dimensions = append(dimensions, string(dimension))
	}
	sort.Strings(dimensions)
	out := make([]dimensionVocabulary, 0, len(dimensions))
	for _, dimension := range dimensions {
		entries := make([]string, 0, len(values[model.AttrType(dimension)]))
		for value := range values[model.AttrType(dimension)] {
			entries = append(entries, value)
		}
		sort.Strings(entries)
		out = append(out, dimensionVocabulary{Dimension: dimension, Values: entries})
	}
	return out
}
