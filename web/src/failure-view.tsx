import React, { useState } from "react";
import { FileText } from "lucide-react";
import { Badge } from "./shared";
import {
  codeLabels,
  failureCopy,
  stageLabels,
  toolLabels,
  type TaskDiagnostics,
} from "./diagnostics";

function usageLine(diagnostics: TaskDiagnostics | null) {
  const usage = diagnostics?.usage;
  if (!usage) return "";
  if (!usage.attempts) return "尚未完成模型请求";
  const unknown = usage.unknown_usage_attempts
    ? `，其中 ${usage.unknown_usage_attempts} 次用量未知`
    : "";
  return `${usage.attempts} 次模型请求${unknown}`;
}

function parameterLabel(diagnostics: TaskDiagnostics | null) {
  const parameter = diagnostics?.safe_parameter;
  if (!parameter) return "";
  const name =
    parameter.name === "query" || parameter.name === "keyword"
      ? "检索词"
      : parameter.name === "symbol"
        ? "品种"
        : "参数";
  return `${name}“${parameter.value}”`;
}

export function TaskFailureView({
  status,
  diagnostics,
  error,
  loading,
  retry,
  onNewResearch,
}: {
  status: string;
  diagnostics: TaskDiagnostics | null;
  error: boolean;
  loading: boolean;
  retry: () => void;
  onNewResearch: () => void;
}) {
  const [details, setDetails] = useState(false);
  const copy = failureCopy(diagnostics);
  const usage = usageLine(diagnostics);
  const confirmed = diagnostics?.cause_confirmed === true;
  const stage = diagnostics?.stage ? stageLabels[diagnostics.stage] : "";
  return (
    <section className="task-outcome" aria-label="执行结果与原因">
      <div className="outcome-heading">
        <FileText size={22} />
        <div>
          <div className="outcome-kicker">执行已停止</div>
          <h2>{copy.headline}</h2>
        </div>
        <Badge state={status} />
      </div>
      <p className="outcome-why">{copy.body}</p>
      <dl className="outcome-facts">
        {usage && (
          <>
            <dt>已完成</dt>
            <dd>{usage}</dd>
          </>
        )}
        {stage && (
          <>
            <dt>失败步骤</dt>
            <dd>
              {stage}
              {confirmed ? "（已与终止记录对应）" : "（最后记录的异常，终止原因未确认）"}
            </dd>
          </>
        )}
        <dt>影响</dt>
        <dd>{copy.impact}</dd>
        <dt>下一步</dt>
        <dd>{copy.next}</dd>
      </dl>
      {error && (
        <p className="outcome-read-error" role="alert">
          错误详情暂时无法读取；原任务状态保持不变。
          <button className="text-button" disabled={loading} onClick={retry}>
            {loading ? "正在重新读取…" : "重新读取"}
          </button>
        </p>
      )}
      {!error && diagnostics?.available === false && diagnostics.reason === "diagnostics_not_supported" && (
        <p className="outcome-read-error" role="status">
          当前执行服务尚未提供错误详情，原因仍以原任务记录为准。
        </p>
      )}
      <div className="outcome-actions">
        <button
          className="button"
          aria-expanded={details}
          onClick={() => setDetails(!details)}
        >
          {details ? "收起错误详情" : "查看错误详情"}
        </button>
        <button className="button primary" onClick={onNewResearch}>
          修改问题并新建研究
        </button>
      </div>
      {details && (
        <div className="outcome-details" role="region" aria-label="错误详情">
          {loading && <p>正在读取错误详情…</p>}
          {diagnostics && diagnostics.available ? (
            <dl>
              <dt>失败阶段</dt>
              <dd>{stageLabels[diagnostics.stage || "unknown"] || "未定位"}</dd>
              <dt>分类</dt>
              <dd>{codeLabels[diagnostics.code || "unknown"] || "原因尚未确认"}</dd>
              <dt>原因确认</dt>
              <dd>{confirmed ? "已与终止检查点对应" : "尚未确认终止原因"}</dd>
              {parameterLabel(diagnostics) && (
                <>
                  <dt>请求参数</dt>
                  <dd>{parameterLabel(diagnostics)}</dd>
                </>
              )}
              {diagnostics.tool && (
                <>
                  <dt>工具</dt>
                  <dd>
                    {toolLabels[diagnostics.tool] || diagnostics.tool}
                    {" · "}
                    <code>{diagnostics.tool}</code>
                  </dd>
                </>
              )}
              {diagnostics.operation_id && (
                <>
                  <dt>失败操作</dt>
                  <dd className="wrap-anywhere">{diagnostics.operation_id}</dd>
                </>
              )}
              {!!diagnostics.related_event_ids?.length && (
                <>
                  <dt>关联事件</dt>
                  <dd>{diagnostics.related_event_ids.join("、")}</dd>
                </>
              )}
              {diagnostics.failure_version && (
                <>
                  <dt>失败版本</dt>
                  <dd className="wrap-anywhere">{diagnostics.failure_version}</dd>
                </>
              )}
              {diagnostics.usage && (
                <>
                  <dt>用量</dt>
                  <dd>
                    {diagnostics.usage.attempts} 次模型请求，未知用量{" "}
                    {diagnostics.usage.unknown_usage_attempts} 次；输入{" "}
                    {diagnostics.usage.prompt_tokens}、输出{" "}
                    {diagnostics.usage.completion_tokens} token
                    {diagnostics.usage.token_budget
                      ? `；预算 ${diagnostics.usage.token_budget} token`
                      : ""}
                  </dd>
                </>
              )}
            </dl>
          ) : (
            !loading && <p>没有可展示的错误详情；不会因此推断具体原因。</p>
          )}
          <p className="outcome-note">
            错误详情来自已保存的执行记录，只读；查看或导出不会重新执行研究。
          </p>
        </div>
      )}
    </section>
  );
}
