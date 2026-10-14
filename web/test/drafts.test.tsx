import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { DraftContent, type RetainedDraft } from "../src/drafts";

test("retained draft stays visibly incomplete and preserves exact feedback identity", () => {
  const draft: RetainedDraft = {
    phase: "retained_draft",
    task_status: "failed",
    operation_id: "fixture/110",
    run_id: "fixture",
    recorded_at: "2026-10-14T10:00:00Z",
    version: "a".repeat(64),
    input_sha256: "b".repeat(64),
    raw: "fixture draft",
    feedback: {
      validator: "historical-check-v2",
      status: "revision_required",
      draft_sha256: "b".repeat(64),
      total_issues: 2,
      issues_truncated: true,
      revise: [
        {
          path: "facts.f1",
          code: "date",
          message: "<script>bad()</script>日期待查",
        },
        { path: "facts.f2", code: "date" },
      ],
      source_review: [],
    },
    check: {
      validator: "current-check-v3",
      result_sha256: "b".repeat(64),
      classification_date: "2026-10-09",
      status: "needs_review",
      notice: "fixture review",
      facts: [],
      comparisons: [],
      issues: [],
      report: {
        schema: "futures-evidence-v1",
        as_of: "2026-10-09",
        questions: [],
        facts: [],
        comparisons: [],
      },
    },
  };
  const html = renderToStaticMarkup(
    <DraftContent draft={draft} onSource={() => {}} />,
  );
  assert.match(html, /送检草稿 · 尚无最终回答/);
  assert.match(html, /任务仍未完成/);
  assert.match(html, /historical-check-v2/);
  assert.match(html, /fixture\/110/);
  assert.match(html, /送检草稿原文（完整保留）/);
  assert.match(html, /提示未全部展开/);
  assert.equal((html.match(/日期待查/g) || []).length, 2);
  assert.doesNotMatch(html, /<script|原始模型输出|研究已完成/);
  // The historical BFF fixture contained a JSON null empty feedback list.
  const historical = structuredClone(draft);
  historical.feedback.source_review = null as unknown as [];
  const legacyHTML = renderToStaticMarkup(
    <DraftContent draft={historical} onSource={() => {}} />,
  );
  assert.match(legacyHTML, /尚无最终回答/);
});
