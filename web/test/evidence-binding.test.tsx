import assert from "node:assert/strict";
import React from "react";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import {
  BindingSummary,
  TaskEvidenceBinding,
  type EvidenceBinding,
} from "../src/evidence-binding";

const binding: EvidenceBinding = {
  source_run_id: "source-task",
  source_result_sha256: "a".repeat(64),
  target_result_sha256: "b".repeat(64),
  mode: "retrospective",
  reason: "核对历史原文",
  receipt_count: 3,
  manifest_sha256: "c".repeat(64),
  created_at: "2026-10-18T00:00:00Z",
};
test("retrospective binding distinguishes original reading and does not promise content validation", () => {
  const html = renderToStaticMarkup(
    <BindingSummary binding={binding} open={() => {}} />,
  );
  assert.match(html, /事后补充的证据关联/);
  assert.match(html, /不代表本任务重新阅读/);
  assert.match(html, /source-task/);
  assert.match(html, /固定版本/);
  assert.match(html, new RegExp(binding.manifest_sha256));
});
test("existing association cannot be replaced in UI and running tasks cannot be linked", () => {
  const render = (task: any) =>
    renderToStaticMarkup(
      <TaskEvidenceBinding
        task={task}
        open={() => {}}
        revise={() => {}}
        refresh={() => {}}
      />,
    );
  const existing = render({
    id: "target",
    status: "completed",
    result: "report",
    evidence_binding: binding,
  });
  assert.doesNotMatch(existing, /核对源任务/);
  assert.match(existing, /基于此报告发起修订/);
  assert.doesNotMatch(
    render({ id: "target", status: "running" }),
    /补充证据关联/,
  );
  assert.match(
    render({ id: "target", status: "completed" }),
    /保存后关联固定版本且不可替换/,
  );
});
