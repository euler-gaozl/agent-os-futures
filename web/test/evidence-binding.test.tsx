import assert from "node:assert/strict";
import React from "react";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import {
  BindingSummary,
  TaskEvidencePanel,
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
test("manual association is secondary, closed by default and never offered without a report", () => {
  const render = (task: any) =>
    renderToStaticMarkup(
      <TaskEvidencePanel task={task} open={() => {}} refresh={() => {}} />,
    );
  const existing = render({
    id: "target",
    status: "completed",
    result: "report",
    evidence_binding: binding,
  });
  assert.match(existing, /事后补充的证据关联/);
  assert.doesNotMatch(existing, /按编号查找|保存后不能更换资料来源/);
  assert.doesNotMatch(existing, /发起修订/);
  // A failed task without a report has nothing to supplement.
  assert.equal(render({ id: "target", status: "failed", result: "" }), "");
  assert.equal(render({ id: "target", status: "running" }), "");
  // A report task exposes only a closed advanced entry.
  const advanced = render({ id: "target", status: "completed", result: "report" });
  assert.match(advanced, /<details/);
  assert.match(advanced, /关联已有研究资料（高级：按编号查找）/);
  assert.match(advanced, /首版没有可靠的候选任务查询/);
  assert.doesNotMatch(advanced, /<details open/);
  assert.match(advanced, /保存后不能更换资料来源；原报告保持原样/);
});
