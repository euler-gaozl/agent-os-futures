import React, { useState } from "react";
import { api, date } from "./shared";

export type EvidenceBinding = {
  source_run_id: string;
  source_result_sha256: string;
  target_result_sha256: string;
  mode: "submission" | "retrospective";
  reason: string;
  receipt_count: number;
  manifest_sha256: string;
  created_at: string;
};

export function BindingSummary({
  binding,
  open,
}: {
  binding: EvidenceBinding;
  open: (id: string) => void;
}) {
  return (
    <div className="source-task">
      <b>
        {binding.mode === "retrospective"
          ? "事后补充的证据关联"
          : "提交时关联的证据"}
      </b>
      <p>
        保留 {binding.receipt_count}{" "}
        份固定版本的阅读回执。关联不代表本任务重新阅读，也不代表报告内容已通过复核。
      </p>
      <button
        className="text-button"
        onClick={() => open(binding.source_run_id)}
      >
        查看源任务 · {binding.source_run_id.slice(0, 8)}
      </button>
      <small>
        {date(binding.created_at)} · {binding.reason}
      </small>
      <details>
        <summary>版本与核对信息</summary>
        <dl>
          <dt>源任务</dt>
          <dd>{binding.source_run_id}</dd>
          <dt>源报告摘要</dt>
          <dd>{binding.source_result_sha256}</dd>
          <dt>证据清单摘要</dt>
          <dd>{binding.manifest_sha256}</dd>
        </dl>
      </details>
    </div>
  );
}

export function TaskLineage({
  task,
  open,
}: {
  task: any;
  open: (id: string) => void;
}) {
  if (!task.evidence_binding) return null;
  return (
    <div className="task-lineage">
      <span>
        资料关联：使用了 {date(task.evidence_binding.created_at)} 保存的{" "}
        {task.evidence_binding.receipt_count} 份阅读回执。
      </span>
      <button
        className="text-button"
        onClick={() => open(task.evidence_binding.source_run_id)}
      >
        查看源任务
      </button>
    </div>
  );
}

// Manual association is deliberately secondary. It exists only for a task that
// already produced a report, is closed by default and is never offered for a
// failed task without a report (there is no report to supplement).
export function TaskEvidencePanel({
  task,
  open,
  refresh,
}: {
  task: any;
  open: (id: string) => void;
  refresh: () => void;
}) {
  const [sourceID, setSourceID] = useState("");
  const [source, setSource] = useState<any>(null);
  const [receipts, setReceipts] = useState<number | null>(null);
  const [reason, setReason] = useState("补充既有报告的来源关联");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const terminal = ["completed", "failed", "cancelled"].includes(task.status);
  if (task.evidence_binding)
    return (
      <section className="evidence-lineage" aria-label="资料关联">
        <BindingSummary binding={task.evidence_binding} open={open} />
      </section>
    );
  if (!terminal || !task.result) return null;
  const inspect = async () => {
    setBusy(true);
    setError("");
    setSource(null);
    setReceipts(null);
    try {
      const value = await api<any>(
        "/tasks/" + encodeURIComponent(sourceID.trim()),
      );
      if (
        !value ||
        value.id !== sourceID.trim() ||
        !/^[a-f0-9]{64}$/.test(value.result_sha256 || "") ||
        !["completed", "failed", "cancelled"].includes(value.status) ||
        value.commodity !== task.commodity
      )
        throw new Error("需要同一品种、已结束且支持版本核对的源任务。");
      const page = await api<{ items: any[] }>(
        "/tasks/" +
          encodeURIComponent(value.id) +
          "/sources?limit=1",
      ).catch(() => null);
      const count = page && Array.isArray(page.items) ? page.items.length : 0;
      setReceipts(count);
      setSource(value);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const bind = async () => {
    if (!source) return;
    setBusy(true);
    setError("");
    try {
      await api("/tasks/" + encodeURIComponent(task.id) + "/evidence-binding", {
        source_run_id: source.id,
        expected_source_result_sha256: source.result_sha256,
        reason: reason.trim(),
      });
      setSaved(true);
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <details className="source-task evidence-more">
      <summary>关联已有研究资料（高级：按编号查找）</summary>
      <p>
        常规阅读不需要这一步。首版没有可靠的候选任务查询；只有明确知道来源任务编号时才使用。
        已有关联不能更换。关联后可回看这次研究保存的原文；保存后不能更换资料来源；原报告保持原样。
      </p>
      <div className="button-row">
        <input
          aria-label="证据源任务 ID"
          value={sourceID}
          disabled={busy}
          placeholder="源任务完整 ID"
          onChange={(e) => {
            setSourceID(e.target.value);
            setSource(null);
            setReceipts(null);
            setError("");
            setSaved(false);
          }}
        />
        <button
          className="text-button"
          disabled={
            busy ||
            !/^[A-Za-z0-9][A-Za-z0-9_-]{0,119}$/.test(sourceID.trim()) ||
            sourceID.trim() === task.id
          }
          onClick={() => void inspect()}
        >
          按编号查找
        </button>
      </div>
      {source && (
        <div className="evidence-candidate">
          <p>
            {source.commodity || "研究"} · {date(source.created_at)} ·{" "}
            <span className="wrap-anywhere">{source.id}</span>
          </p>
          <p>
            {receipts
              ? "该任务有已保存的阅读记录，可作为来源候选。"
              : "该任务没有已保存的阅读记录，不能作为来源。"}
          </p>
          <button className="text-button" onClick={() => open(source.id)}>
            查看源报告
          </button>
          <label>
            关联原因
            <input
              aria-label="关联原因"
              value={reason}
              maxLength={500}
              disabled={busy}
              onChange={(e) => setReason(e.target.value)}
            />
          </label>
          <p className="evidence-warning">
            关联后可回看这次研究保存的原文。保存后不能更换资料来源；原报告保持原样。
          </p>
          <button
            className="button"
            disabled={busy || !reason.trim() || !receipts || saved}
            onClick={() => void bind()}
          >
            {saved ? "已保存关联" : "保存证据关联"}
          </button>
        </div>
      )}
      {saved && (
        <p role="status">关联已保存；原报告和执行记录没有改写。</p>
      )}
      {error && <p role="alert">{error}</p>}
    </details>
  );
}
