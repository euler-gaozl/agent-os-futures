import React, { useEffect, useRef, useState } from "react";
import { BookOpen } from "lucide-react";
import { api, Spinner } from "./shared";
import {
  CheckedReport,
  type ReportCheck,
  type OpenSource,
  type SourceProof,
} from "./report";
import { ResearchAnswer, SourceDialog } from "./evidence";

type DraftItem = {
  operation_id: string;
  run_id: string;
  recorded_at: string;
  version: string;
  input_sha256: string;
};
type DraftPage = { items: DraftItem[]; next_cursor: string };
type Issue = { path: string; code: string; message?: string };
export type RetainedDraft = DraftItem & {
  phase: "retained_draft";
  task_status: string;
  raw: string;
  feedback: {
    validator: string;
    status: string;
    draft_sha256: string;
    total_issues: number;
    issues_truncated: boolean;
    revise: Issue[];
    source_review: Issue[];
  };
  check: ReportCheck;
};

export function DraftContent({
  draft,
  onSource,
}: {
  draft: RetainedDraft;
  onSource: OpenSource;
}) {
  const feedback = draft.feedback;
  const completed = draft.task_status === "completed";
  const revise = Array.isArray(feedback.revise) ? feedback.revise : [];
  const sourceReview = Array.isArray(feedback.source_review)
    ? feedback.source_review
    : [];
  const explanations = new Map(
    [...revise, ...sourceReview]
      .filter((x) => x.message)
      .map((x) => [x.code, x.message]),
  );
  return (
    <>
      <div className="draft-warning" role="note">
        <strong>送检草稿 · {completed ? "历史版本" : "尚无最终回答"}</strong>
        <p>
          {completed
            ? "任务执行已完成。这是中途保存的草稿，不能替代最终回答；草稿中的事实、日期和引用可能有误，研究核验结果仍须分别查看。"
            : "这是任务中途保存的版本。任务仍未完成，草稿中的事实、日期和引用可能有误，不能当作已完成的研究报告。"}
        </p>
        <small>
          保存时间：{new Date(draft.recorded_at).toLocaleString("zh-CN")}
        </small>
      </div>
      <details className="draft-feedback">
        <summary>送检时的反馈 · {feedback.total_issues} 项检查提示</summary>
        <p>
          当时的检查器：{feedback.validator}。
          {feedback.status === "revision_required"
            ? "仍需修改草稿。"
            : "仍需核对原始来源。"}
          这些检查不证明资料真实或问题已研究完整。
        </p>
        {feedback.issues_truncated && (
          <p>当时返回的提示未全部展开；下面另有按当前检查器进行的回查。</p>
        )}
        {[
          { title: "需要修改", items: revise },
          { title: "需要回查来源", items: sourceReview },
        ].map((group) => (
          <div key={group.title}>
            <b>{group.title}</b>
            <ul>
              {group.items.map((item, i) => (
                <li key={i}>
                  <code>{item.path}</code>：
                  {item.message || explanations.get(item.code) || item.code}
                </li>
              ))}
            </ul>
          </div>
        ))}
        <details>
          <summary>版本定位信息</summary>
          <p>操作：{draft.operation_id}</p>
          <p>草稿摘要：{draft.input_sha256}</p>
        </details>
      </details>
      <p className="draft-recheck-note">
        当前检查器：{draft.check.validator}
        。下面回查这份草稿及任务留存的原文；回查不会改变任务状态或当时的反馈。
      </p>
      <CheckedReport
        check={draft.check}
        raw={draft.raw}
        onSource={onSource}
        rawLabel="送检草稿原文（完整保留）"
      />
    </>
  );
}

export function TaskRetainedWork({
  task,
  status,
  hasResult,
}: {
  task: string;
  status: string;
  hasResult: boolean;
}) {
  if (!["completed", "failed", "cancelled", "waiting_unknown"].includes(status))
    return null;
  return (
    <article className="research-document">
      <RetainedDrafts key={task + ":drafts:" + status} task={task} />
      {!hasResult && (
        <>
          <div className="document-heading">
            <BookOpen size={19} />
            <span>已保留的资料</span>
          </div>
          <ResearchAnswer key={task + ":retained"} task={task} />
        </>
      )}
    </article>
  );
}

export function RetainedDrafts({ task }: { task: string }) {
  const base = "/tasks/" + encodeURIComponent(task) + "/drafts";
  const [items, setItems] = useState<DraftItem[]>([]),
    [cursor, setCursor] = useState("");
  const [selected, setSelected] = useState<DraftItem | null>(null),
    [draft, setDraft] = useState<RetainedDraft | null>(null);
  const [busy, setBusy] = useState(true),
    [loading, setLoading] = useState(false),
    [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [source, setSource] = useState<{
    document: string;
    proof?: SourceProof;
  } | null>(null);
  const generation = useRef(0);
  useEffect(() => {
    const epoch = ++generation.current;
    let live = true;
    setBusy(true);
    setError("");
    setItems([]);
    setSelected(null);
    setCursor("");
    api<DraftPage>(base)
      .then((page) => {
        if (live) {
          setItems(page.items);
          setCursor(page.next_cursor);
          setSelected(page.items[0] || null);
        }
      })
      .catch((e) => {
        if (live)
          setError(
            e.status === 404
              ? "当前服务尚未配置草稿回查；仍可从资料回查阅读已保存的资料。"
              : e.message,
          );
      })
      .finally(() => {
        if (live) setBusy(false);
      });
    return () => {
      live = false;
      if (generation.current === epoch) generation.current++;
    };
  }, [base, retry]);
  useEffect(() => {
    let live = true;
    setDraft(null);
    setSource(null);
    if (!selected) {
      setLoading(false);
      return () => {
        live = false;
      };
    }
    setLoading(true);
    setError("");
    api<RetainedDraft>(
      base +
        "/record?operation_id=" +
        encodeURIComponent(selected.operation_id) +
        "&version=" +
        encodeURIComponent(selected.version),
    )
      .then((value) => {
        if (!live) return;
        if (
          value.phase !== "retained_draft" ||
          value.operation_id !== selected.operation_id ||
          value.version !== selected.version ||
          value.input_sha256 !== value.feedback.draft_sha256
        )
          throw new Error("草稿与检查记录不一致，请重新加载。");
        setDraft(value);
      })
      .catch((e) => {
        if (live) setError(e.message);
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [base, selected, retry]);
  const more = async () => {
    const epoch = generation.current;
    setBusy(true);
    setError("");
    try {
      const page = await api<DraftPage>(
        base + "?after=" + encodeURIComponent(cursor),
      );
      if (generation.current === epoch) {
        setItems((old) => [...old, ...page.items]);
        setCursor(page.next_cursor);
      }
    } catch (e) {
      if (generation.current === epoch) setError((e as Error).message);
    } finally {
      if (generation.current === epoch) setBusy(false);
    }
  };
  return (
    <section className="retained-drafts" aria-label="留存的研究草稿">
      <header>
        <h2>阶段草稿</h2>
        <p>保留每次送检版本及对应反馈，便于从已有工作继续核对。</p>
      </header>
      {!!items.length && (
        <label className="draft-selector">
          送检版本
          <select
            value={selected?.operation_id || ""}
            onChange={(e) =>
              setSelected(
                items.find((x) => x.operation_id === e.target.value) || null,
              )
            }
          >
            {items.map((item, i) => (
              <option key={item.operation_id} value={item.operation_id}>
                {i === 0 ? "最新 · " : "较早 · "}
                {new Date(item.recorded_at).toLocaleString("zh-CN")} ·{" "}
                {item.operation_id.split("/").at(-1)}
              </option>
            ))}
          </select>
        </label>
      )}
      {cursor && (
        <button
          className="text-button"
          disabled={busy}
          onClick={() => void more()}
        >
          加载更早的送检版本
        </button>
      )}
      {(busy || loading) && (
        <p>
          <Spinner /> 正在读取留存草稿和检查记录…
        </p>
      )}
      {error && (
        <div className="source-error" role="alert">
          <p>{error}</p>
          <button
            className="text-button"
            onClick={() => setRetry((x) => x + 1)}
          >
            重新读取
          </button>
        </div>
      )}
      {!busy && !error && !items.length && (
        <p>这次任务没有可回查的送检草稿。</p>
      )}
      {draft && draft.operation_id === selected?.operation_id && (
        <DraftContent
          draft={draft}
          onSource={(document, proof) => setSource({ document, proof })}
        />
      )}
      {source && (
        <SourceDialog
          key={task + ":" + source.document + ":" + source.proof?.operation_id}
          task={task}
          document={source.document}
          proof={source.proof}
          close={() => setSource(null)}
        />
      )}
    </section>
  );
}
