import { useCallback, useEffect, useState } from "react";
import { api } from "./shared";

export type SafeParameter = { name: string; value: string };
export type DiagnosticUsage = {
  attempts: number;
  unknown_usage_attempts: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_prompt_tokens: number;
  accounted_tokens: number;
  max_attempts: number;
  token_budget: number;
};
export type TaskDiagnostics = {
  available: boolean;
  reason?: string;
  protocol?: string;
  run_id: string;
  status?: string;
  failure_version?: string;
  reported_at?: string;
  stage?: string;
  code?: string;
  safe_message?: string;
  cause_confirmed?: boolean;
  related_event_ids?: number[];
  operation_id?: string;
  related_role?: string;
  tool?: string;
  safe_parameter?: SafeParameter;
  recommended_action?: string;
  retry_policy?: string;
  usage?: DiagnosticUsage;
};

export type DiagnosticsState = {
  diagnostics: TaskDiagnostics | null;
  error: boolean;
  loading: boolean;
  retry: () => void;
};

const noDiagnostics = (): DiagnosticsState => ({
  diagnostics: null,
  error: false,
  loading: false,
  retry: () => {},
});

// Diagnostics are fetched once per terminal task selection and on explicit
// retry. The six-second parent refresh re-renders the page but does not create
// another read, and a late reply for a previous task is discarded.
export function useTaskDiagnostics(id: string, status: string): DiagnosticsState {
  const [state, setState] = useState<{
    diagnostics: TaskDiagnostics | null;
    error: boolean;
    loading: boolean;
  }>({ diagnostics: null, error: false, loading: false });
  const [attempt, setAttempt] = useState(0);
  const terminal = ["failed", "cancelled", "waiting_unknown"].includes(status);
  useEffect(() => {
    if (!id || !terminal) {
      setState({ diagnostics: null, error: false, loading: false });
      return;
    }
    let live = true;
    setState({ diagnostics: null, error: false, loading: true });
    api<TaskDiagnostics>("/tasks/" + encodeURIComponent(id) + "/diagnostics")
      .then((value) => {
        if (!live) return;
        if (!value || value.run_id !== id || typeof value.available !== "boolean")
          throw new Error("诊断身份不一致");
        setState({ diagnostics: value, error: false, loading: false });
      })
      .catch(() => {
        if (live) setState({ diagnostics: null, error: true, loading: false });
      });
    return () => {
      live = false;
    };
  }, [id, terminal, attempt]);
  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  if (!id || !terminal) {
    const blank = noDiagnostics();
    return { ...blank, retry };
  }
  return { ...state, retry };
}

export const stageLabels: Record<string, string> = {
  tool: "资料工具调用",
  model: "模型请求",
  execution: "执行状态保存",
  cancel: "任务取消",
  unknown: "未定位的步骤",
};

export const toolLabels: Record<string, string> = {
  kb_search: "资料库检索",
  knowledge_search: "笔记与资料检索",
  document_read: "原文阅读",
  skill_file_read: "技能文件读取",
  sandbox_exec: "隔离执行",
  note_save: "笔记保存",
  note_revise: "笔记修订",
  note_retire: "笔记停用",
};

export const codeLabels: Record<string, string> = {
  parameter_rejected: "参数校验未通过",
  configuration_or_auth: "配置或鉴权问题",
  downstream_unavailable: "下游服务暂不可用",
  effect_unknown: "执行效果待核对",
  protocol_error: "执行协议异常",
  budget_exhausted: "预算或上下文限制",
  persistence_or_lease: "执行状态保存问题",
  cancelled: "任务已取消",
  unknown: "原因尚未确认",
};

const searchTools = new Set(["kb_search", "knowledge_search", "document_read"]);

export function failureCopy(diagnostics: TaskDiagnostics | null) {
  if (!diagnostics || diagnostics.available === false) {
    const unsupported = diagnostics?.reason === "diagnostics_not_supported";
    return {
      headline: "这次研究没有完成",
      body: unsupported
        ? "当前执行服务尚未提供错误详情；原任务状态和执行记录保持不变，具体原因还没有确认。"
        : "错误详情暂时无法读取；原任务状态和执行记录保持不变，具体原因还没有确认。",
      impact: "没有生成最终报告；已有执行记录保留。",
      next: "可以重新读取错误详情，或修改问题后新建研究。",
    };
  }
  const tool = diagnostics.tool || "";
  const parameter = diagnostics.safe_parameter?.value;
  switch (diagnostics.code) {
    case "parameter_rejected":
      return searchTools.has(tool)
        ? {
            headline: "资料检索未完成",
            body: parameter
              ? `检索词“${parameter}”未通过资料库的查询校验，任务在这一步结束。`
              : "资料库拒绝了这次查询请求（参数未通过校验），任务在这一步结束。",
            impact: "没有生成最终报告；此前的模型响应和已保存的阅读记录保留。",
            next: "修正检索方式或问题后，再发起新的研究。",
          }
        : {
            headline: "执行请求未通过参数校验",
            body: "工具请求的参数不符合要求，任务在这一步结束。",
            impact: "没有生成最终报告；已有执行记录保留。",
            next: "修正问题或研究范围后，再发起新的研究。",
          };
    case "configuration_or_auth":
      return {
        headline: "研究所需的连接或权限未就绪",
        body: "模型或资料服务的配置、凭证或授权被拒绝，任务在这一步结束。",
        impact: "没有生成最终报告；已有执行记录保留。",
        next: "请联系工作区管理员检查服务连接后再试。",
      };
    case "downstream_unavailable":
      return {
        headline: "下游服务暂时不可用",
        body: "模型或资料服务在本次执行中暂时不可用，任务在这一步结束。",
        impact: "没有生成最终报告；已有执行记录保留。",
        next: "稍后重新发起研究；系统不会自动重试或自动重新提交。",
      };
    case "effect_unknown":
      return {
        headline: "有一项执行结果需要核对",
        body: "外部服务的回执未能确认，任务已暂停，不能当作已经完成。",
        impact: "没有生成最终报告；执行记录保留，等待人工核对。",
        next: "请由工作区管理员核对回执后再决定下一步。",
      };
    case "protocol_error":
      return {
        headline: "执行返回不完整",
        body: "模型或工具返回的内容不完整或不符合执行协议，任务在这一步结束。",
        impact: "没有生成最终报告；已有执行记录保留。",
        next: "稍后重新发起研究；不要根据中断结果作结论。",
      };
    case "budget_exhausted":
      return {
        headline: "本次执行预算已用尽",
        body: "模型调用或上下文达到当前执行配置的上限，任务已停止。",
        impact: "没有生成最终报告；已有执行记录保留。",
        next: "缩小研究范围后新建研究；系统不会自动重试。",
      };
    case "persistence_or_lease":
      return {
        headline: "执行状态未能确认",
        body: "任务状态的保存或执行归属出现冲突，这一轮执行没有完成。",
        impact: "没有生成最终报告；执行记录需要核对。",
        next: "请由工作区管理员核对执行记录后再决定下一步。",
      };
    case "cancelled":
      return {
        headline: "研究已停止",
        body: "这次任务已被取消，取消发生在完成之前。",
        impact: "没有生成最终报告；已有的资料和执行记录保留。",
        next: "需要继续时，请修改问题后新建研究。",
      };
    default:
      return {
        headline: "这次研究没有完成",
        body: "执行记录没有对应到已确认的失败原因，不能据此推断具体故障。",
        impact: "没有生成最终报告；已有执行记录保留。",
        next: "查看错误详情，或由工作区管理员核对后再决定下一步。",
      };
  }
}
