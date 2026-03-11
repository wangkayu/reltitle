package serve

import (
	"encoding/json"

	"title-parser/pkg/model"
)

type RequestEnvelope struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type protocolRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type protocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type protocolResponse struct {
	ID     string         `json:"id"`
	OK     bool           `json:"ok"`
	Result any            `json:"result,omitempty"`
	Error  *protocolError `json:"error,omitempty"`
}

type ParseBatchParams struct {
	Items   []ParseBatchItemRequest `json:"items"`
	Options ParseBatchOptions       `json:"options,omitempty"`
}

type ParseBatchItemRequest struct {
	Title string `json:"title"`
	Site  string `json:"site,omitempty"`
}

type ParseBatchOptions struct {
	BestPick     bool `json:"best_pick,omitempty"`
	IncludeRaw   bool `json:"include_raw,omitempty"`
	IncludeScore bool `json:"include_score,omitempty"`
}

type ParseBatchResult struct {
	Items     []ParseBatchItemResult `json:"items"`
	BestIndex *int                   `json:"best_index,omitempty"`
}

type ParseBatchItemResult struct {
	Index  int                `json:"index"`
	Site   string             `json:"site,omitempty"`
	Parsed *model.ParseResult `json:"parsed,omitempty"`
	Raw    string             `json:"raw,omitempty"`
	Score  float64            `json:"score,omitempty"`
	Error  *protocolError     `json:"error,omitempty"`
}
