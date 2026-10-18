package futures

import (
	"unicode/utf8"
)

// Each half-open range counts Unicode code points in the original document.
// Receipt versions pin the actual read, OriginalHash pins its document version.
// Neither proves the publisher's claims true.
type QuoteSegment struct {
	OperationID string `json:"operation_id"`
	Version     string `json:"version"`
	ReadOffset  int    `json:"read_offset"`
	ReadEnd     int    `json:"read_end"`
	QuoteOffset int    `json:"quote_offset"`
	QuoteEnd    int    `json:"quote_end"`
}

// Join only neighbours actually read at the same document version. No live
// Knowledge reads, gap filling or fuzzy matching. Whitespace-only matches are
// explicit (MatchMode=whitespace) and keep their rune offsets. Eight loaded
// receipts bound the search; increasing ends prevent cycles.
func joinedQuote(parts []retainedPart, source ReportSource) (quoteProof, bool) {
	if len(parts) > 8 || source.Quote == "" {
		return quoteProof{}, false
	}
	valid := func(p retainedPart) bool {
		return p.DocumentID == source.DocumentID && receiptHash.MatchString(p.OriginalHash) &&
			receiptHash.MatchString(p.Version) && p.OperationID != "" && len(p.OperationID) <= 512 &&
			p.ReadOffset >= 0 && p.ReadEnd > p.ReadOffset && p.ReadEnd <= 2147483646 &&
			utf8.RuneCountInString(p.Text) == p.ReadEnd-p.ReadOffset
	}
	conflict := map[string]bool{}
	for i, a := range parts {
		if !valid(a) {
			continue
		}
		for _, b := range parts[i+1:] {
			if !valid(b) || a.OriginalHash != b.OriginalHash {
				continue
			}
			start, end := max(a.ReadOffset, b.ReadOffset), min(a.ReadEnd, b.ReadEnd)
			if start < end && string([]rune(a.Text)[start-a.ReadOffset:end-a.ReadOffset]) != string([]rune(b.Text)[start-b.ReadOffset:end-b.ReadOffset]) {
				conflict[a.OriginalHash] = true
			}
		}
	}
	var visit func([]retainedPart, string) (quoteProof, bool)
	visit = func(path []retainedPart, text string) (quoteProof, bool) {
		first, last := path[0], path[len(path)-1]
		if len(path) > 1 {
			if at, endAt, mode, ok := quoteMatchRange(text, source.Quote); ok {
				start := first.ReadOffset + at
				end := first.ReadOffset + endAt
				proof := quoteProof{Offset: start, OriginalHash: first.OriginalHash, MatchMode: mode}
				for _, p := range path {
					a, b := max(start, p.ReadOffset), min(end, p.ReadEnd)
					if a >= b {
						continue
					}
					if proof.Operation == "" {
						proof.Operation, proof.Version = p.OperationID, p.Version
					}
					proof.Segments = append(proof.Segments, QuoteSegment{p.OperationID, p.Version, p.ReadOffset, p.ReadEnd, a, b})
				}
				if len(proof.Segments) >= 2 {
					return proof, true
				}
			}
		}
		for _, p := range parts {
			if !valid(p) || p.OriginalHash != first.OriginalHash || p.ReadOffset != last.ReadEnd {
				continue
			}
			duplicate := false
			for _, old := range path {
				if old.OperationID == p.OperationID {
					duplicate = true
				}
			}
			if duplicate {
				continue
			}
			if proof, ok := visit(append(path, p), text+p.Text); ok {
				return proof, true
			}
		}
		return quoteProof{}, false
	}
	for _, p := range parts {
		if valid(p) && !conflict[p.OriginalHash] {
			if proof, ok := visit([]retainedPart{p}, p.Text); ok {
				return proof, true
			}
		}
	}
	return quoteProof{}, false
}
