import assert from "node:assert/strict";
import React from "react";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import { TaskFailureView } from "../src/failure-view";
import { failureCopy, type TaskDiagnostics } from "../src/diagnostics";

const sample: TaskDiagnostics = {
  available: true,
  protocol: "app-run-diagnostics-v1",
  run_id: "dd80cadf8a249e7821ee217cbf6da81b",
  status: "failed",
  stage: "tool",
  code: "parameter_rejected",
  cause_confirmed: true,
  related_event_ids: [24, 25],
  operation_id: "dd80cadf8a249e7821ee217cbf6da81b/dsh/14",
  related_role: "tool",
  tool: "kb_search",
  safe_parameter: { name: "query", value: "豆" },
  recommended_action: "correct_input",
  retry_policy: "manual_after_correction",
  failure_version: "fvd1:" + "a".repeat(64),
  usage: {
    attempts: 2,
    unknown_usage_attempts: 0,
    prompt_tokens: 23667,
    completion_tokens: 218,
    cached_prompt_tokens: 10624,
    accounted_tokens: 23885,
    max_attempts: 40,
    token_budget: 200000,
  },
};

function view(diagnostics: TaskDiagnostics | null, error = false) {
  return renderToStaticMarkup(
    <TaskFailureView
      status={diagnostics?.status || "failed"}
      diagnostics={diagnostics}
      error={error}
      loading={false}
      retry={() => {}}
      onNewResearch={() => {}}
    />,
  );
}

test("the failed first screen names the rejected query, the impact and the next step", () => {
  const html = view(sample);
  assert.match(html, /执行已停止/);
  assert.match(html, /资料检索未完成/);
  assert.match(html, /检索词“豆”未通过资料库的查询校验/);
  assert.match(html, /2 次模型请求/);
  assert.match(html, /失败步骤[\s\S]*资料工具调用（已与终止记录对应）/);
  assert.match(html, /没有生成最终报告/);
  assert.match(html, /修改问题并新建研究/);
  assert.match(html, /查看错误详情/);
  // The raw HTTP status is not the explanation.
  assert.doesNotMatch(html, /HTTP 400|upstream request rejected|PRIVATE/);
  // Details stay collapsed until requested.
  assert.doesNotMatch(html, /错误详情来自已保存的执行记录/);
});

test("an unavailable or unsupported diagnostic keeps the task state and does not invent a cause", () => {
  const unavailable = view(null, true);
  assert.match(unavailable, /错误详情暂时无法读取/);
  assert.match(unavailable, /原任务状态保持不变/);
  assert.match(unavailable, /没有生成最终报告/);
  const unsupported = view({
    available: false,
    run_id: "dd80cadf8a249e7821ee217cbf6da81b",
    reason: "diagnostics_not_supported",
  });
  assert.match(unsupported, /当前执行服务尚未提供错误详情/);
  assert.doesNotMatch(unsupported, /parameter_rejected|HTTP/);
});

test("each failure class has its own headline and every copy names impact and next step", () => {
  const cases: Array<[string, string]> = [
    ["configuration_or_auth", "研究所需的连接或权限未就绪"],
    ["downstream_unavailable", "下游服务暂时不可用"],
    ["effect_unknown", "有一项执行结果需要核对"],
    ["protocol_error", "执行返回不完整"],
    ["budget_exhausted", "本次执行预算已用尽"],
    ["persistence_or_lease", "执行状态未能确认"],
    ["cancelled", "研究已停止"],
    ["unknown", "这次研究没有完成"],
  ];
  for (const [code, headline] of cases) {
    const copy = failureCopy({ ...sample, code, stage: "tool" });
    assert.equal(copy.headline, headline);
    assert.ok(copy.impact && copy.next, code + " must explain impact and next step");
    assert.doesNotMatch(copy.body, /HTTP|PRIVATE|https?:/);
    assert.match(view({ ...sample, code }), new RegExp(headline));
  }
});
