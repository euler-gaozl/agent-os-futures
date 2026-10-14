import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { CheckedReport, type ReportCheck } from "../src/report";

function report(): ReportCheck {
  return {
    status: "consistent",
    notice: "不证明来源真实性或独立核验",
    validator: "futures-report-check-v1",
    result_sha256: "a".repeat(64),
    classification_date: "2026-10-09",
    report: {
      schema: "futures-evidence-v1",
      as_of: "2026-10-09",
      questions: [
        {
          id: "q",
          question: "库存统计覆盖什么范围？",
          fact_ids: ["f"],
          comparison_ids: [],
          missing: ["其他库存环节的原始调查"],
        },
      ],
      facts: [
        {
          id: "f",
          metric: "大豆到港",
          value: "1018.55",
          unit: "万吨",
          scope: "全国全样本油厂",
          measure: "flow",
          nature: "estimate",
          period: { start: "2026-09-01", end: "2026-09-30" },
          source: {
            document_id: "doc1",
            quote:
              '2026年9月份预估 <script>alert(1)</script><img src="https://tracking.invalid/x">',
            period_text: "2026年9月份",
          },
        },
      ],
      comparisons: [],
    },
    facts: [
      {
        id: "f",
        group: "已结束区间 · 第三方估计",
        quote_matched: true,
        consistent: true,
        operation_id: "task/read",
        version: "b".repeat(64),
        quote_offset: 4,
      },
    ],
    comparisons: [],
    issues: [],
  };
}
test("structured report preserves historical estimate labels and renders source as text", () => {
  const html = renderToStaticMarkup(
    <CheckedReport
      check={report()}
      raw={'{"test":true}'}
      onSource={() => {}}
    />,
  );
  assert.match(html, /已结束区间 · 第三方估计/);
  assert.match(html, /2026-09-01 — 2026-09-30/);
  assert.match(html, /不证明来源真实性/);
  assert.match(html, /原始模型输出/);
  assert.doesNotMatch(html, /<script|<img|src="https:\/\/tracking|事实已核实/);
  assert.match(html, /&lt;script&gt;/);
  assert.match(html, /回查任务当时保存的资料/);
});
test("failed comparison never displays a calculated quantity or verified badge", () => {
  const check = report();
  check.status = "needs_review";
  check.report!.comparisons = [{ id: "c1", current: "f", baseline: "other" }];
  check.comparisons = [
    { id: "c1", consistent: false, from: "", to: "", difference: "", unit: "" },
  ];
  check.facts[0].consistent = false;
  check.facts[0].quote_matched = false;
  check.issues = [
    { path: "facts.f", code: "anchor", message: "原文不支持字段关系" },
  ];
  const html = renderToStaticMarkup(
    <CheckedReport check={check} raw="raw" onSource={() => {}} />,
  );
  assert.match(html, /暂不用于比较/);
  assert.match(html, /未计算差额/);
  assert.match(html, /原文不支持字段关系/);
  assert.doesNotMatch(html, /后值减前值：|回查任务当时保存的资料/);
});
test("old answer remains readable but explicitly unverified", () => {
  const check = report();
  delete check.report;
  check.status = "unstructured";
  check.facts = [];
  check.issues = [
    { path: "report", code: "format", message: "未符合结构化资料格式" },
  ];
  const html = renderToStaticMarkup(
    <CheckedReport check={check} raw="历史回答原文" onSource={() => {}} />,
  );
  assert.match(html, /历史回答原文/);
  assert.match(html, /运行完成不等于研究核验完成/);
  assert.doesNotMatch(html, /结构与留存引用检查一致/);
});
