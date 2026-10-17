package futures

// Qualitative source evidence is deliberately separate from numeric facts.
// A statement can only carry an explanatory claim (source nature, sample
// definition, coverage, a limitation) that is quoted verbatim from a document
// retained in this task. It has no value/unit/period and never participates in
// comparisons, so it cannot be used to smuggle a number past fact checks.
type ReportStatement struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	DocumentID  string   `json:"document_id"`
	Quote       string   `json:"quote"`
	ContentKind string   `json:"content_kind"`
	Limitations []string `json:"limitations"`
}

type StatementCheck struct {
	ID             string         `json:"id"`
	ContentKind    string         `json:"content_kind"`
	QuoteMatched   bool           `json:"quote_matched"`
	OperationID    string         `json:"operation_id,omitempty"`
	Version        string         `json:"version,omitempty"`
	QuoteOffset    int            `json:"quote_offset"`
	OriginalHash   string         `json:"original_hash,omitempty"`
	Segments       []QuoteSegment `json:"segments,omitempty"`
	SourceURL      string         `json:"source_url,omitempty"`
	PublishedLabel string         `json:"published_label,omitempty"`
	CapturedLabel  string         `json:"captured_label,omitempty"`
	Consistent     bool           `json:"consistent"`
}
