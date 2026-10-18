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

export function TaskEvidenceBinding({
  task,
  open,
  revise,
  refresh,
}: {
  task: any;
  open: (id: string) => void;
  revise: () => void;
  refresh: () => void;
}) {
  const [sourceID, setSourceID] = useState("");
  const [source, setSource] = useState<any>(null);
  const [reason, setReason] = useState("补充既有报告的来源关联");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const terminal = ["completed", "failed", "cancelled"].includes(task.status);
  const inspect = async () => {
    setBusy(true);
    setError("");
    setSource(null);
    try {
      const value = await api<any>(
        "/tasks/" + encodeURIComponent(sourceID.trim()),
      );
      if (
        !value ||
        value.id !== sourceID.trim() ||
        !/^[a-f0-9]{64}$/.test(value.result_sha256 || "") ||
        !["completed", "failed", "cancelled"].includes(value.status)
      )
        throw new Error("需要已结束且支持版本核对的源任务。");
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
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="evidence-lineage" aria-label="修订与证据来源">
      {task.evidence_binding ? (
        <BindingSummary binding={task.evidence_binding} open={open} />
      ) : (
        terminal && (
          <details className="source-task">
            <summary>为旧报告补充证据关联</summary>
            <p>
              选择实际提供原文的较早任务。保存后关联固定版本且不可替换，原报告和执行记录不改写。
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
                  setError("");
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
                核对源任务
              </button>
            </div>
            {source && (
              <div>
                <p>
                  {source.commodity || "研究"} · {date(source.created_at)} ·{" "}
                  {source.id}
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
                <button
                  className="text-button"
                  disabled={busy || !reason.trim()}
                  onClick={() => void bind()}
                >
                  保存证据关联
                </button>
              </div>
            )}
            {error && <p role="alert">{error}</p>}
          </details>
        )
      )}
      {terminal && task.result && (
        <button className="text-button" onClick={revise}>
          基于此报告发起修订
        </button>
      )}
    </section>
  );
}
