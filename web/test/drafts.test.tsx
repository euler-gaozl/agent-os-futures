import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import {
  DraftContent,
  TaskRetainedWork,
  type RetainedDraft,
} from "../src/drafts";

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

test("completed task keeps its unstructured historical draft distinct from final output and current review", () => {
  const draft: RetainedDraft = {
    phase: "retained_draft",
    task_status: "completed",
    operation_id: "synthetic/42",
    run_id: "synthetic",
    recorded_at: "2026-10-15T01:00:00Z",
    version: "a".repeat(64),
    input_sha256: "b".repeat(64),
    raw: '{"schema":"futures-evidence-v1"',
    feedback: {
      validator: "futures-report-check-v4",
      status: "revision_required",
      draft_sha256: "b".repeat(64),
      total_issues: 1,
      issues_truncated: false,
      revise: [{ path: "report", code: "format", message: "历史泛化格式提示" }],
      source_review: [],
    },
    check: {
      validator: "futures-report-check-v5",
      result_sha256: "b".repeat(64),
      classification_date: "2026-10-09",
      status: "unstructured",
      notice: "检查不证明研究质量已通过。",
      facts: [],
      comparisons: [],
      issues: [
        {
          path: "report",
          code: "format",
          message: "JSON 未结束：UTF-8 第 31 字节",
        },
      ],
    },
  };
  const before = structuredClone(draft);
  const html = renderToStaticMarkup(
    <DraftContent draft={draft} onSource={() => {}} />,
  );
  assert.match(html, /送检草稿 · 历史版本/);
  assert.match(html, /任务执行已完成/);
  assert.match(html, /不能替代最终回答/);
  assert.match(html, /送检时的反馈/);
  assert.match(html, /当时的检查器：futures-report-check-v4/);
  assert.match(html, /当前检查器：futures-report-check-v5/);
  assert.match(html, /历史泛化格式提示/);
  assert.match(html, /JSON 未结束/);
  assert.match(html, /运行完成不等于研究核验完成/);
  assert.doesNotMatch(html, /尚无最终回答|任务仍未完成|研究已核验/);
  assert.deepEqual(draft, before);
});

test("terminal task details expose drafts even when a final result exists and keep source browsing available once", () => {
  const completed = renderToStaticMarkup(
    <TaskRetainedWork
      task="synthetic-completed"
      status="completed"
      hasResult
    />,
  );
  assert.match(completed, /留存的研究草稿/);
  assert.match(completed, /阶段草稿/);
  assert.doesNotMatch(completed, /已保留的资料|aria-label="资料回查"/);
  for (const status of ["failed", "cancelled", "waiting_unknown"]) {
    const html = renderToStaticMarkup(
      <TaskRetainedWork
        task="synthetic-incomplete"
        status={status}
        hasResult={false}
      />,
    );
    assert.match(html, /阶段草稿/);
    assert.match(html, /已保留的资料/);
    assert.equal((html.match(/aria-label="资料回查"/g) || []).length, 1);
  }
  for (const status of ["queued", "running", "waiting_children"]) {
    assert.equal(
      renderToStaticMarkup(
        <TaskRetainedWork
          task="synthetic-running"
          status={status}
          hasResult={false}
        />,
      ),
      "",
    );
  }
});
