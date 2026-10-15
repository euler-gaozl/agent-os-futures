import React from "react";
import { FileText } from "lucide-react";

export function taskFailureMessage(error: string) {
  if (error === "model HTTP 503")
    return "模型服务暂时不可用，这次研究已停止，尚未生成最终回答。本任务不会自动重新提交；需要再次研究时，请返回工作台检查问题后手动新建任务。";
  if (error.includes("credential"))
    return "研究模型尚未连接。请由工作区管理员完成连接后继续。";
  if (error.includes("model output is incomplete or truncated"))
    return "模型回复达到单次输出上限，研究记录尚未生成。已读取的资料仍可在下方回查。";
  if (error.includes("model token budget exhausted"))
    return "本任务剩余预算不足以预留下一次模型请求，任务已停止。已读取的资料仍可在下方回查。";
  if (error.includes("context budget exceeded"))
    return "任务上下文超出当前执行配置的上限，尚未完成压缩恢复。已读取的资料仍可在下方回查。";
  return error || "执行过程中出现问题，研究进度已经保留。";
}

export function TaskFailureNotice({ error }: { error: string }) {
  return (
    <div className="research-failure">
      <FileText size={27} />
      <h3>这次研究还未完成</h3>
      <p>{taskFailureMessage(error)}</p>
    </div>
  );
}

export function TaskReadFailure({
  error,
  hasTask,
  reading,
  retry,
}: {
  error: unknown;
  hasTask: boolean;
  reading: boolean;
  retry: () => void;
}) {
  const status =
    error && typeof error === "object" && "status" in error
      ? error.status
      : undefined;
  const detail =
    status === 401
      ? "当前登录已失效，请重新连接工作台后再读取。"
      : status === 403
        ? "当前账号无权读取这项任务。"
        : status === 404
          ? "当前没有找到可读取的任务记录。"
          : "任务详情暂时无法读取，最新执行状态尚未确认。";
  return (
    <div className={hasTask ? "source-error" : "research-failure"} role="alert">
      <h3>暂时无法读取任务</h3>
      <p>{detail}</p>
      {hasTask && <p>下方保留上次读取的任务内容，请以重新读取后的状态为准。</p>}
      <button
        type="button"
        className="text-button"
        disabled={reading}
        onClick={retry}
      >
        {reading ? "正在重新读取…" : "重新读取"}
      </button>
      <p>重新读取只查看原任务，不会重新执行研究或创建新任务。</p>
    </div>
  );
}
