import assert from "node:assert/strict";
import React from "react";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import { CheckedReport, anchorID, type ReportCheck } from "../src/report";

function check(id: string): ReportCheck {
  return {
    status: "needs_review",
    notice: "fixture",
    validator: "fixture-check-v1",
    result_sha256: "a".repeat(64),
    classification_date: "2026-10-20",
    report: {
      schema: "futures-evidence-v1",
      as_of: "2026-10-20",
      questions: [
        {
          id: "q1",
          question: "fixture question",
          fact_ids: [id],
          statement_ids: ["s1"],
          comparison_ids: [],
          missing: [],
        },
      ],
      facts: [
        {
          id,
          metric: "fixture",
          value: "1",
          unit: "吨",
          scope: "fixture",
          measure: "stock",
          nature: "reported",
          period: { start: "2026-01-01", end: "2026-01-31" },
          source: { document_id: "doc", quote: "quote", period_text: "2026-01" },
        },
      ],
      statements: [
        {
          id: "s1",
          text: "fixture statement",
          document_id: "doc",
          quote: "quote",
          content_kind: "publisher_text",
          limitations: [],
        },
      ],
      comparisons: [],
    },
    facts: [
      {
        id,
        group: "past",
        quote_matched: true,
        consistent: true,
        operation_id: "op",
        version: "b".repeat(64),
        quote_offset: 0,
      },
    ],
    statements: [],
    comparisons: [],
    issues: [],
  };
}

test("final record and historical drafts never share anchors", () => {
  const final = renderToStaticMarkup(
    <CheckedReport instance="final:task-a" check={check("f1")} raw="final" onSource={() => {}} />,
  );
  const draft = renderToStaticMarkup(
    <CheckedReport instance="draft:task-a/op/1:version" check={check("f1")} raw="draft" onSource={() => {}} />,
  );
  assert.match(final, /id="report-final-task-a-fact-f1"/);
  assert.match(final, /href="#report-final-task-a-fact-f1"/);
  assert.match(final, /id="report-final-task-a-statement-s1"/);
  assert.match(final, /href="#report-final-task-a-statement-s1"/);
  assert.match(draft, /id="report-draft-task-a-op-1-version-fact-f1"/);
  assert.match(draft, /href="#report-draft-task-a-op-1-version-fact-f1"/);
  assert.doesNotMatch(final, /id="report-fact-f1"/);
  assert.notEqual(anchorID("final:task-a", "fact", "f1"), anchorID("draft:task-a/op/1:version", "fact", "f1"));
});
