import React, { useCallback, useEffect, useState } from "react";
import { AlertTriangle, ArrowUpRight, Download, RefreshCw } from "lucide-react";
import { api, Badge, date, Empty, Spinner } from "./shared";
import { codeLabels, stageLabels, toolLabels, type TaskDiagnostics } from "./diagnostics";

export type TaskFailureItem = {
  run_id: string;
  status: string;
  created_at: string;
  commodity?: string;
  period?: string;
  stage: string;
  code: string;
  cause_confirmed: boolean;
  operation_id?: string;
  tool?: string;
  failure_version: string;
};
type Coverage = {
  since: string;
  until: string;
  scanned: number;
  scan_limit: number;
  truncated: boolean;
  complete: boolean;
};
type FailurePage = {
  protocol: string;
  app: string;
  items: TaskFailureItem[];
  next_cursor: string;
  has_more: boolean;
  coverage: Coverage;
};

const ranges = [
  { days: 7, label: "最近 7 天" },
  { days: 30, label: "最近 30 天" },
  { days: 90, label: "最近 90 天" },
  { days: 366, label: "最近 366 天" },
];
const statuses = [
  { value: "", label: "全部状态" },
  { value: "failed", label: "执行失败" },
  { value: "waiting_unknown", label: "待核对" },
  { value: "cancelled", label: "已取消" },
];
const stages = [
  { value: "", label: "全部步骤" },
  { value: "tool", label: stageLabels.tool },
  { value: "model", label: stageLabels.model },
  { value: "execution", label: stageLabels.execution },
  { value: "cancel", label: stageLabels.cancel },
  { value: "unknown", label: stageLabels.unknown },
];
const categories = [
  { value: "", label: "全部分类" },
  ...Object.keys(codeLabels).map((value) => ({ value, label: codeLabels[value] })),
];

type FilterState = {
  days: number;
  status: string;
  stage: string;
  category: string;
  commodity: string;
};

const initialFilters: FilterState = {
  days: 90,
  status: "",
  stage: "",
  category: "",
  commodity: "",
};

function isoWindow(days: number) {
  const until = new Date();
  const since = new Date(until.getTime() - days * 24 * 60 * 60 * 1000);
  const iso = (value: Date) => value.toISOString().replace(/\.\d{3}Z$/, "Z");
  return { since: iso(since), until: iso(until) };
}

function queryFor(filters: FilterState) {
  const window = isoWindow(filters.days);
  const q = new URLSearchParams({ since: window.since, until: window.until, limit: "25" });
  for (const key of ["status", "stage", "category", "commodity"] as const) {
    const value = filters[key].trim();
    if (value) q.set(key, value);
  }
  return q;
}

async function fetchPage(filters: FilterState, after: string) {
  const q = queryFor(filters);
  if (after) q.set("after", after);
  return api<FailurePage>("/task-failures?" + q.toString());
}

function download(name: string, content: string, type: string) {
  const blob = new Blob([content], { type });
  const link = document.createElement("a");
  link.href = URL.createObjectURL(blob);
  link.download = name;
  link.click();
  URL.revokeObjectURL(link.href);
}

function summaryMarkdown(
  items: TaskFailureItem[],
  coverage: Coverage,
  filters: FilterState,
) {
  const scope = `${date(coverage.since)} 至 ${date(coverage.until)}，本页 ${items.length} 条；扫描 ${coverage.scanned} 条候选（上限 ${coverage.scan_limit}）${coverage.truncated ? "，已达到扫描上限" : ""}`;
  const rows = items.map(
    (item) =>
      `## ${item.commodity || "未标注品种"} · ${date(item.created_at)}\n\n- 任务：${item.run_id}\n- 状态：${item.status}\n- 阶段：${stageLabels[item.stage] || item.stage}\n- 分类：${codeLabels[item.code] || item.code}\n- 原因：${item.cause_confirmed ? "已与终止检查点对应" : "尚未确认终止原因"}\n- 操作：${item.operation_id || "未记录"}${item.tool ? `（${toolLabels[item.tool] || item.tool}）` : ""}\n- 失败版本：${item.failure_version || "未记录"}\n- 复盘结论：（手动填写）\n- 后续动作：（手动填写）\n`,
  );
  return `# 期货研究错误记录复盘摘要\n\n生成时间：${new Date().toLocaleString("zh-CN")}\n\n覆盖范围：${scope}\n\n筛选：${JSON.stringify(filters)}\n\n说明：本摘要来自只读执行记录，未重新执行研究，也不代表失败已解决。人工复盘状态不写入产品。\n\n${rows.join("\n")}`;
}

export function FailureRecords({
  onOpen,
}: {
  onOpen: (id: string) => void;
}) {
  const [applied, setApplied] = useState<FilterState>(initialFilters);
  const [draft, setDraft] = useState<FilterState>(initialFilters);
  const [items, setItems] = useState<TaskFailureItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [coverage, setCoverage] = useState<Coverage | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [reviewing, setReviewing] = useState("");

  useEffect(() => {
    let live = true;
    setLoading(true);
    setError(false);
    setItems([]);
    setCursor("");
    setCoverage(null);
    setHasMore(false);
    fetchPage(applied, "")
      .then((page) => {
        if (!live) return;
        setItems(page.items);
        setCursor(page.next_cursor || "");
        setCoverage(page.coverage);
        setHasMore(Boolean(page.has_more && page.next_cursor));
      })
      .catch(() => {
        if (live) setError(true);
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [applied, attempt]);

  const more = async () => {
    if (!cursor) return;
    setLoadingMore(true);
    setError(false);
    try {
      const page = await fetchPage(applied, cursor);
      setItems((old) => {
        const seen = new Set(old.map((item) => item.run_id));
        return [...old, ...page.items.filter((item) => !seen.has(item.run_id))];
      });
      setCursor(page.next_cursor || "");
      setCoverage(page.coverage);
      setHasMore(Boolean(page.has_more && page.next_cursor));
    } catch {
      setError(true);
    } finally {
      setLoadingMore(false);
    }
  };

  const review = useCallback(
    async (item: TaskFailureItem) => {
      setReviewing(item.run_id);
      let diagnostics: TaskDiagnostics | null = null;
      try {
        diagnostics = await api<TaskDiagnostics>(
          "/tasks/" + encodeURIComponent(item.run_id) + "/diagnostics",
        );
      } catch {
        diagnostics = null;
      }
      const record = {
        format: "futures-error-review-v1",
        generated_at: new Date().toISOString(),
        task: item,
        diagnostics:
          diagnostics && diagnostics.available ? diagnostics : null,
        diagnostics_available: Boolean(diagnostics && diagnostics.available),
        coverage,
        filters: applied,
        manual_review: {
          reviewed: false,
          conclusion: "",
          follow_up: "",
        },
        note: "只读摘要；未重新执行研究，不写入人工处置状态。",
      };
      download(
        (item.commodity || "研究") + "-错误复盘-" + item.run_id.slice(0, 8) + ".json",
        JSON.stringify(record, null, 2),
        "application/json;charset=utf-8",
      );
      setReviewing("");
    },
    [applied, coverage],
  );

  const exportPage = () => {
    if (!coverage) return;
    download(
      "期货错误记录-" + new Date().toISOString().slice(0, 10) + ".md",
      summaryMarkdown(items, coverage, applied),
      "text/markdown;charset=utf-8",
    );
  };

  return (
    <main className="failure-records" id="research-content" tabIndex={-1}>
      <header className="failure-records-header">
        <div className="record-label">手动复盘入口</div>
        <h1>错误记录</h1>
        <p>
          只读查看已受理任务的失败与待核对状态。分类和步骤来自执行记录；这里不是全库统计，
          查看、筛选和导出都不会重新执行研究。
        </p>
      </header>
      <form
        className="failure-filters"
        onSubmit={(event) => {
          event.preventDefault();
          setApplied({ ...draft });
        }}
      >
        <label>
          时间范围
          <select
            value={draft.days}
            onChange={(e) => setDraft({ ...draft, days: Number(e.target.value) })}
          >
            {ranges.map((range) => (
              <option key={range.days} value={range.days}>
                {range.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          状态
          <select
            value={draft.status}
            onChange={(e) => setDraft({ ...draft, status: e.target.value })}
          >
            {statuses.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          失败步骤
          <select
            value={draft.stage}
            onChange={(e) => setDraft({ ...draft, stage: e.target.value })}
          >
            {stages.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          分类
          <select
            value={draft.category}
            onChange={(e) => setDraft({ ...draft, category: e.target.value })}
          >
            {categories.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          品种
          <input
            value={draft.commodity}
            maxLength={100}
            placeholder="例如：豆粕"
            onChange={(e) => setDraft({ ...draft, commodity: e.target.value })}
          />
        </label>
        <div className="failure-filter-actions">
          <button className="button primary" type="submit" disabled={loading}>
            应用筛选
          </button>
          <button
            className="button quiet"
            type="button"
            disabled={!items.length || !coverage}
            onClick={exportPage}
          >
            导出本页 <Download size={14} />
          </button>
        </div>
      </form>
      <div className="failure-scope" role="status">
        {coverage ? (
          <>
            范围：{date(coverage.since)} 至 {date(coverage.until)} · 本页 {items.length} 条
            {coverage.scanned !== undefined
              ? ` · 已扫描 ${coverage.scanned} 条候选（上限 ${coverage.scan_limit}）`
              : ""}
            {coverage.truncated ? " · 已达到扫描上限，结果可能不完整" : ""}
          </>
        ) : (
          "正在读取错误记录…"
        )}
      </div>
      {loading ? (
        <div className="research-wait">
          <Spinner /> 正在读取错误记录…
        </div>
      ) : error ? (
        <div className="research-failure" role="alert">
          <h3>错误记录暂时无法读取</h3>
          <p>原任务状态不受影响；重新读取不会重新执行研究。</p>
          <button className="button" onClick={() => setAttempt((n) => n + 1)}>
            <RefreshCw size={14} /> 重新读取
          </button>
        </div>
      ) : !items.length ? (
        <Empty
          title="当前筛选没有错误记录"
          detail="这不代表没有发生过失败；请调整时间范围或筛选条件。成功任务中已恢复的错误不属于本页。"
        />
      ) : (
        <ul className="failure-list">
          {items.map((item) => (
            <li key={item.run_id} className="failure-card">
              <div className="failure-card-head">
                <b>{item.commodity || "未标注品种"}</b>
                <Badge state={item.status} />
                <time>{date(item.created_at)}</time>
              </div>
              <div className="failure-card-meta">
                <span>{stageLabels[item.stage] || "未定位的步骤"}</span>
                <span>{codeLabels[item.code] || "原因尚未确认"}</span>
                <span className={item.cause_confirmed ? "" : "unconfirmed"}>
                  {item.cause_confirmed ? "已与终止记录对应" : "终止原因未确认"}
                </span>
              </div>
              {item.period && <p className="failure-period">研究范围：{item.period}</p>}
              <p className="failure-op wrap-anywhere">
                任务：{item.run_id}
                {item.operation_id ? " · 操作：" + item.operation_id : ""}
              </p>
              <div className="failure-card-actions">
                <button className="text-button" onClick={() => onOpen(item.run_id)}>
                  查看任务与详情
                </button>
                <button
                  className="text-button"
                  disabled={reviewing === item.run_id}
                  onClick={() => void review(item)}
                >
                  {reviewing === item.run_id ? "正在整理…" : "导出复盘摘要"}
                  <ArrowUpRight size={12} />
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {hasMore && !loading && (
        <div className="failure-more">
          <button className="button" disabled={loadingMore} onClick={() => void more()}>
            {loadingMore ? "正在加载…" : "加载更多"}
          </button>
        </div>
      )}
      <p className="failure-note">
        <AlertTriangle size={13} />
        人工“已复盘/已解决”状态不保存在产品中；复盘摘要由你手动归档。查看、筛选、展开和导出都不会创建任务或调用模型。
      </p>
    </main>
  );
}
