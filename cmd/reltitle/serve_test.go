package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type servedResponse struct {
	ID     int64           `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func runServe(t *testing.T, buildVersion string, stdin string) (code int, responses []servedResponse, stderr string) {
	t.Helper()
	var stdout, errOut bytes.Buffer
	code = serveServe(buildVersion, strings.NewReader(stdin), &stdout, &errOut)
	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	for scanner.Scan() {
		var response servedResponse
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatalf("response is not JSON: %v\n%s", err, scanner.Text())
		}
		responses = append(responses, response)
	}
	return code, responses, errOut.String()
}

func TestServeHelloReportsProtocolVersionLineAndDigest(t *testing.T) {
	code, responses, stderr := runServe(t, "v0.0.3", `{"id":1,"op":"hello"}`+"\n")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if len(responses) != 1 || !responses[0].OK || responses[0].ID != 1 {
		t.Fatalf("responses = %+v", responses)
	}
	var hello helloResult
	if err := json.Unmarshal(responses[0].Result, &hello); err != nil {
		t.Fatal(err)
	}
	if hello.Protocol != ServeProtocol || hello.Version != "0.0.3" || hello.Line != "0.0" {
		t.Fatalf("hello = %+v", hello)
	}
	if !strings.HasPrefix(hello.ConfigDigest, "lexicon:sha256:") {
		t.Fatalf("config digest = %q", hello.ConfigDigest)
	}
}

func TestServeParseReturnsTheSameResultAsTheCLI(t *testing.T) {
	const title = "[Group] Some Show - 03 [1080p][简繁]"
	code, directStdout, _ := runCLI(t, "", "-title", title)
	if code != 0 {
		t.Fatalf("cli exit %d", code)
	}
	_, responses, _ := runServe(t, "0.0.3", `{"id":2,"op":"parse","title":"`+title+`"}`+"\n")
	if len(responses) != 1 || !responses[0].OK {
		t.Fatalf("responses = %+v", responses)
	}
	// The one-shot CLI indents its output; compare the decoded objects.
	var fromCLI, fromServe any
	if err := json.Unmarshal([]byte(directStdout), &fromCLI); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(responses[0].Result, &fromServe); err != nil {
		t.Fatal(err)
	}
	cliJSON, _ := json.Marshal(fromCLI)
	serveJSON, _ := json.Marshal(fromServe)
	if !bytes.Equal(cliJSON, serveJSON) {
		t.Fatalf("serve and CLI disagree\ncli:   %s\nserve: %s", cliJSON, serveJSON)
	}
}

func TestServeVocabularyCoversTheDictionary(t *testing.T) {
	_, responses, _ := runServe(t, "0.0.3", `{"id":3,"op":"vocabulary"}`+"\n")
	if len(responses) != 1 || !responses[0].OK {
		t.Fatalf("responses = %+v", responses)
	}
	var vocabulary vocabularyResult
	if err := json.Unmarshal(responses[0].Result, &vocabulary); err != nil {
		t.Fatal(err)
	}
	values := map[string]map[string]bool{}
	for _, dimension := range vocabulary.Dimensions {
		if len(dimension.Values) == 0 {
			t.Fatalf("dimension %s has no values", dimension.Dimension)
		}
		values[dimension.Dimension] = map[string]bool{}
		for _, value := range dimension.Values {
			values[dimension.Dimension][value] = true
		}
	}
	// Dimensions the lexicon emits must be present even when the canonical
	// dictionary does not list them.
	for _, dimension := range []string{"resolution", "video_codec", "content_rating"} {
		if len(values[dimension]) == 0 {
			t.Fatalf("vocabulary has no %s values: %+v", dimension, vocabulary.Dimensions)
		}
	}
	// A dimension the canonical dictionary defines reports exactly its values:
	// the parser's ad-hoc WxH and cropped-tier outputs are not options.
	if len(values["resolution"]) != 8 {
		t.Fatalf("resolution values = %v, want the 8 canonical tiers", values["resolution"])
	}
	for _, notCanonical := range []string{"1440x1080", "1008p", "1440p"} {
		if values["resolution"][notCanonical] {
			t.Fatalf("resolution lists non-canonical %q: %+v", notCanonical, values["resolution"])
		}
	}
	// content_rating outputs one value per fact: every age-limit marker is
	// AGE_RESTRICTED, so the vocabulary lists exactly these three.
	wantRatings := map[string]bool{"UNCENSORED": true, "CENSORED": true, "AGE_RESTRICTED": true}
	if len(values["content_rating"]) != len(wantRatings) {
		t.Fatalf("content_rating values = %v, want %v", values["content_rating"], wantRatings)
	}
	for value := range values["content_rating"] {
		if !wantRatings[value] {
			t.Fatalf("content_rating has unexpected value %q: %+v", value, values["content_rating"])
		}
	}
}

// TestServeVocabularyCoversEveryParsedValue holds the contract a consumer
// builds its option list on: for every published dimension, every value a
// parse can report is one of the vocabulary's values, and no free-form or
// structural dimension is published at all.
func TestServeVocabularyCoversEveryParsedValue(t *testing.T) {
	server, err := newServeServer("test")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	vocabulary := map[string]map[string]bool{}
	published := map[string]bool{}
	for _, dimension := range server.vocabulary {
		published[dimension.Dimension] = true
		vocabulary[dimension.Dimension] = map[string]bool{}
		for _, value := range dimension.Values {
			vocabulary[dimension.Dimension][value] = true
		}
	}
	for _, freeForm := range []string{"note", "title", "title_alias", "group", "contents", "pack_profile", "audio_object", "audio_channels", "sub_format"} {
		if published[freeForm] {
			t.Errorf("vocabulary publishes the free-form dimension %q", freeForm)
		}
	}

	file, err := os.Open("../../pkg/parser/testdata/golden.jsonl")
	if err != nil {
		t.Fatalf("open golden corpus: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<22)
	titles := 0
	for scanner.Scan() {
		var entry struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("golden line: %v", err)
		}
		if entry.Title == "" {
			continue
		}
		titles++
		result, err := server.parser.Parse(entry.Title)
		if err != nil {
			t.Fatalf("%s: parse failed: %v", entry.Title, err)
		}
		for _, attribute := range result.Attributes {
			if !vocabularyDimensions[attribute.Type] {
				continue
			}
			if !vocabulary[string(attribute.Type)][attribute.Normalized] {
				t.Errorf("%s: %s=%q is not a vocabulary value (%v)", entry.Title, attribute.Type, attribute.Normalized, vocabulary[string(attribute.Type)])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read golden corpus: %v", err)
	}
	if titles == 0 {
		t.Fatal("the golden corpus is empty")
	}
}

func TestServeAnswersBadInputWithoutDying(t *testing.T) {
	stdin := strings.Join([]string{
		`not json`,
		`{"id":4,"op":"nope"}`,
		`{"id":5,"op":"parse"}`,
		`{"id":6,"op":"parse","title":"[Group] Ok - 01"}`,
	}, "\n") + "\n"
	code, responses, _ := runServe(t, "0.0.3", stdin)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(responses) != 4 {
		t.Fatalf("got %d responses: %+v", len(responses), responses)
	}
	if responses[0].OK || responses[0].Error.Code != "bad_request" {
		t.Fatalf("bad json -> %+v", responses[0])
	}
	if responses[1].OK || responses[1].Error.Code != "unknown_op" {
		t.Fatalf("unknown op -> %+v", responses[1])
	}
	if responses[2].OK || responses[2].Error.Code != "missing_title" {
		t.Fatalf("missing title -> %+v", responses[2])
	}
	if !responses[3].OK || responses[3].ID != 6 {
		t.Fatalf("a valid request after bad ones -> %+v", responses[3])
	}
}

func TestServeStopsOnAnOversizedRequest(t *testing.T) {
	oversized := `{"id":7,"op":"parse","title":"` + strings.Repeat("x", maxRequestBytes) + `"}`
	code, responses, _ := runServe(t, "0.0.3", oversized+"\n")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if len(responses) != 1 || responses[0].OK || responses[0].Error.Code != "request_too_long" {
		t.Fatalf("responses = %+v", responses)
	}
}

func TestLineOf(t *testing.T) {
	for input, want := range map[string]string{
		"0.0.3":          "0.0",
		"v0.0.3":         "0.0",
		"0.1.0":          "0.1",
		"v0.0.4-test.17": "0.0",
		"0.0.0-dev":      "0.0",
		"dev":            "dev",
	} {
		if got := lineOf(input); got != want {
			t.Fatalf("lineOf(%q) = %q, want %q", input, got, want)
		}
	}
}
