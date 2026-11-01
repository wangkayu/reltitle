package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	if !values["content_rating"]["UNCENSORED"] {
		t.Fatalf("content_rating has no UNCENSORED: %+v", values["content_rating"])
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
