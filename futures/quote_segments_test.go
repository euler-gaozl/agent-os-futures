package futures

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/euler-gaozl/agent-os-sdk/serviceclient"
)

func quoteParts() []retainedPart {
	texts := []string{"前文🙂中国2026年", "8月大豆进口增加。", "后文库存下降。"}
	parts := []retainedPart{}
	offset := 1200
	for i, text := range texts {
		end := offset + utf8.RuneCountInString(text)
		parts = append(parts, retainedPart{OperationID: "run/read/" + string(rune('a'+i)), DocumentID: "doc", OriginalHash: strings.Repeat("a", 64), Version: strings.Repeat(string(rune('b'+i)), 64), Text: text, ReadOffset: offset, ReadEnd: end})
		offset = end
	}
	return parts
}

func TestJoinedQuotePinsUnicodeRangesAndRejectsUnprovenJoins(t *testing.T) {
	quote := "中国2026年8月大豆进口增加。后文库存"
	for _, mode := range []string{"valid", "different_hash", "missing_hash", "bad_hash", "document", "gap", "overlap", "range", "version", "duplicate", "conflicting_overlap", "matching_overlap"} {
		t.Run(mode, func(t *testing.T) {
			parts := quoteParts()
			switch mode {
			case "different_hash":
				parts[1].OriginalHash = strings.Repeat("f", 64)
			case "missing_hash":
				parts[1].OriginalHash = ""
			case "bad_hash":
				parts[1].OriginalHash = "not-a-hash"
			case "document":
				parts[1].DocumentID = "another"
			case "gap":
				parts[1].ReadOffset++
				parts[1].ReadEnd++
			case "overlap":
				parts[1].ReadOffset--
				parts[1].ReadEnd--
			case "range":
				parts[1].ReadEnd++
			case "version":
				parts[1].Version = "bad"
			case "duplicate":
				parts[1].OperationID = parts[0].OperationID
			case "conflicting_overlap", "matching_overlap":
				other := parts[1]
				other.OperationID = "run/other"
				if mode == "conflicting_overlap" {
					other.Text = strings.Replace(other.Text, "增加", "减少", 1)
				}
				parts = append(parts, other)
			}
			// Retrieval order is newest first, not document order.
			for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
				parts[i], parts[j] = parts[j], parts[i]
			}
			proof, ok := joinedQuote(parts, ReportSource{DocumentID: "doc", Quote: quote})
			want := mode == "valid" || mode == "matching_overlap"
			if ok != want {
				t.Fatalf("match=%v want=%v proof=%+v", ok, want, proof)
			}
			if !ok {
				return
			}
			if proof.Offset != 1203 || len(proof.Segments) != 3 || proof.OriginalHash != strings.Repeat("a", 64) {
				t.Fatal(proof)
			}
			var got strings.Builder
			for _, s := range proof.Segments {
				for _, p := range parts {
					if p.OperationID == s.OperationID {
						got.WriteString(string([]rune(p.Text)[s.QuoteOffset-p.ReadOffset : s.QuoteEnd-p.ReadOffset]))
					}
				}
			}
			if got.String() != quote {
				t.Fatal("proof does not reconstruct quote")
			}
		})
	}
}

func TestReportEvidenceReturnsReopenableCompositeWithoutAdditionalReads(t *testing.T) {
	parts := quoteParts()
	requests := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" {
			t.Fatal("reader wrote")
		}
		if strings.HasSuffix(r.URL.Path, "/record") {
			for _, p := range parts {
				if p.OperationID == r.URL.Query().Get("operation_id") {
					json.NewEncoder(w).Encode(p)
					return
				}
			}
			t.Fatal("wrong receipt")
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []retainedPart{parts[2], parts[1], parts[0]}, "next_cursor": ""})
	}))
	defer peer.Close()
	r := &reportEvidenceReader{server: &Server{Client: serviceclient.Client{}}, base: peer.URL, cache: map[string][]retainedPart{}}
	source := ReportSource{DocumentID: "doc", Quote: "中国2026年8月大豆进口增加。后文库存"}
	proof, err := r.quote(context.Background(), source)
	if err != nil || len(proof.Segments) != 3 || requests != 4 {
		t.Fatal(proof, err, requests)
	}
	again, err := r.quote(context.Background(), source)
	if err != nil || !reflect.DeepEqual(proof, again) || requests != 4 {
		t.Fatal("cache changed or refetched proof", err)
	}
}
