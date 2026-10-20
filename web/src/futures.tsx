import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { ResearchAnswer } from "./evidence";
import { TaskRetainedWork } from "./drafts";
import { TaskReadFailure } from "./task-status";
import { taskReadReducer, watchTask } from "./task-polling";
import { taskSectionKeys } from "./task-keys";
import { TaskEvidencePanel } from "./evidence-binding";
import { TaskFailureView } from "./failure-view";
import { EvidenceCheckChip } from "./report";
import { useTaskDiagnostics } from "./diagnostics";
import {
  ArrowLeft,
  ArrowUp,
  ArrowUpRight,
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  Clock3,
  Compass,
  FileCheck2,
  FileText,
  Layers,
  Menu,
  Plus,
  Search,
  Sparkles,
  X,
} from "lucide-react";
import {
  api,
  Badge,
  date,
  Empty,
  labels,
  Login,
  Modal,
  Spinner,
} from "./shared";
type Action = {
  id: string;
  revision: number;
  name: string;
  description: string;
};
const executionNotes: Record<string, string> = {
  queued: "等待执行",
  running: "正在执行",
  waiting_children: "等待子任务",
  waiting_unknown: "执行暂停，等待核对",
  completed: "执行已结束；研究核验见证据检查",
  failed: "执行已停止",
  cancelled: "执行已取消",
};
function executionNote(status: string) {
  return executionNotes[status] || "";
}

// The failure step is shown as unfinished, never as a normal completion.
function ResearchProgress({ status }: { status: string }) {
  const stopped = ["failed", "waiting_unknown"].includes(status);
  const cancelled = status === "cancelled";
  return (
    <div className="research-progress" aria-label="执行步骤">
      <div>
        <span className="progress-number">1</span>
        <b>接收研究任务</b>
        <Check size={15} />
      </div>
      <div
        className={
          (["queued"].includes(status) ? "pending " : "") +
          (stopped ? "failed " : "") +
          (cancelled ? "pending" : "")
        }
      >
        <span className="progress-number">2</span>
        <b>执行研究任务</b>
        {status === "running" ? (
          <Spinner />
        ) : status === "completed" ? (
          <Check size={15} />
        ) : stopped ? (
          <span className="step-note">未完成 · 执行已停止</span>
        ) : cancelled ? (
          <span className="step-note">已取消</span>
        ) : null}
      </div>
      <div className={status === "completed" ? "" : "pending"}>
        <span className="progress-number">3</span>
        <b>生成研究记录</b>
        {status === "completed" ? (
          <Check size={15} />
        ) : (
          <span className="step-note">未生成</span>
        )}
      </div>
    </div>
  );
}
export function Futures() {
  const [actions, setActions] = useState<Action[]>([]),
    [tasks, setTasks] = useState<any[]>([]),
    [action, setAction] = useState<Action | null>(null),
    [commodity, setCommodity] = useState("甲醇"),
    [period, setPeriod] = useState("最近三个月，并补充可比历史数据"),
    [asOf, setAsOf] = useState(() =>
      new Intl.DateTimeFormat("en-CA", {
        timeZone: "Asia/Shanghai",
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(new Date()),
    ),
    [question, setQuestion] = useState(""),
    [selected, setSelected] = useState(""),
    [readRetry, setReadRetry] = useState(0),
    [error, setError] = useState(""),
    [connected, setConnected] = useState(false),
    [busy, setBusy] = useState(false),
    [ready, setReady] = useState<any>(null),
    [showScope, setShowScope] = useState(false),
    [mobileNav, setMobileNav] = useState(false),
    [notice, setNotice] = useState(""),
    [historySearch, setHistorySearch] = useState(""),
    [revisionSource, setRevisionSource] = useState(""),
    [processOpen, setProcessOpen] = useState(false),
    [authRequired, setAuthRequired] = useState(false);
  const [{ result, error: readError, reading }, dispatchRead] = useReducer(
    taskReadReducer<any>,
    { selected: "", result: null, error: null, reading: false },
  );
  const key = useRef(crypto.randomUUID()),
    input = useRef<HTMLTextAreaElement>(null);
  const load = useCallback(async () => {
    const [a, t, r] = await Promise.all([
      api<Action[]>("/agents"),
      api<any[]>("/tasks"),
      api("/readiness"),
    ]);
    setActions(
      [...a].sort(
        (left, right) =>
          Number(right.id === "published-supply-inventory") -
          Number(left.id === "published-supply-inventory"),
      ),
    );
    setTasks(t);
    setReady(r);
    setConnected(true);
    setError("");
    setAuthRequired(false);
  }, []);
  const connectionError = (e: any) => {
    setError(e.message);
    if (e.status === 401) {
      setAuthRequired(true);
      setConnected(false);
    }
  };
  useEffect(() => {
    void load().catch(connectionError);
  }, [load]);
  useEffect(() => {
    const id = setInterval(() => void load().catch(connectionError), 6000);
    return () => clearInterval(id);
  }, [load]);
  useEffect(() => {
    setProcessOpen(false);
  }, [selected]);
  useEffect(() => {
    dispatchRead({ type: "select", task: selected });
    if (!selected) return;
    // A same-task GET retry keeps its last successful detail and retained work.
    // Selecting another task clears the old detail; cleanup rejects late replies.
    return watchTask({
      task: selected,
      read: (path) => api(path),
      onTask: (value) => dispatchRead({ type: "received", task: value }),
      onError: (error) => dispatchRead({ type: "failed", error }),
      onReading: (reading) => dispatchRead({ type: "reading", reading }),
    });
  }, [selected, readRetry]);
  const choose = (a: Action) => {
    setAction(a);
    key.current = crypto.randomUUID();
    input.current?.focus();
  };
  const fresh = () => {
    setSelected("");
    setRevisionSource("");
    setMobileNav(false);
    key.current = crypto.randomUUID();
  };
  const run = async () => {
    const a = action || actions[0];
    if (!a || !commodity.trim() || !question.trim() || !asOf) return;
    setBusy(true);
    setNotice("");
    try {
      const r = await api(
        "/tasks",
        {
          agent: { id: a.id, revision: a.revision },
          commodity,
          period,
          question,
          as_of: asOf,
          ...(revisionSource ? { evidence_source_run: revisionSource } : {}),
        },
        key.current,
      );
      setSelected(r.id);
      key.current = crypto.randomUUID();
      await load();
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const sidebar = (
    <>
      <a
        href="#"
        className="research-brand"
        onClick={(e) => {
          e.preventDefault();
          fresh();
        }}
      >
        <span className="research-mark">
          <Layers size={23} />
        </span>
        <b>
          研序<small>中国期货研究</small>
        </b>
      </a>
      <button className="new-research" onClick={fresh}>
        <Plus size={17} />
        开始新研究<span>↗</span>
      </button>
      <div className="research-nav">
        <button className={!selected ? "active" : ""} onClick={fresh}>
          <Compass size={17} />
          研究工作台
        </button>
        <div className="history-label">
          研究记录 <span>{tasks.length}</span>
        </div>
        <label className="history-search">
          <Search size={15} />
          <input
            aria-label="搜索研究记录"
            placeholder="按品种或时间查找"
            value={historySearch}
            onChange={(e) => setHistorySearch(e.target.value)}
          />
        </label>
        {tasks.length ? (
          <div className="task-history">
            {tasks
              .filter((t) =>
                (t.commodity + " " + date(t.created_at)).includes(
                  historySearch.trim(),
                ),
              )
              .map((t) => (
                <button
                  key={t.id}
                  className={selected === t.id ? "selected" : ""}
                  onClick={() => {
                    setSelected(t.id);
                    setMobileNav(false);
                  }}
                >
                  <FileText size={15} />
                  <span>
                    {t.commodity} · 基本面研究
                    <small>
                      {date(t.created_at)} · {labels[t.status] || t.status}
                    </small>
                  </span>
                  <i className={"task-dot " + t.status} />
                </button>
              ))}
            {!tasks.some((t) =>
              (t.commodity + " " + date(t.created_at)).includes(
                historySearch.trim(),
              ),
            ) && <p className="history-empty">没有匹配的记录</p>}
          </div>
        ) : (
          <p className="history-empty">你的研究记录会保存在这里。</p>
        )}
      </div>
      <div className="research-sidebar-footer">
        <div className="private-note">
          <BookOpen size={16} />
          <p>
            资料与研究记录
            <small>来源、统计口径与历史变化，留在工作区以便回溯。</small>
          </p>
        </div>
        <div className="research-profile">
          <span>G</span>
          <p>
            我的研究空间<small>个人工作区</small>
          </p>
        </div>
      </div>
    </>
  );
  const restartFrom = (task: any) => {
    setRevisionSource("");
    setCommodity(task.commodity || commodity);
    setPeriod(task.period || period);
    setAsOf(task.as_of || asOf);
    setQuestion("");
    setSelected("");
    key.current = crypto.randomUUID();
  };
  const reviseTask = (task: any) => {
    setRevisionSource(task.id);
    setCommodity(task.commodity || commodity);
    setPeriod(task.period || period);
    setAsOf(task.as_of || asOf);
    setQuestion("");
    setSelected("");
    key.current = crypto.randomUUID();
  };
  const exportRecord = async (task: any) => {
    let structured = false;
    try {
      structured = JSON.parse(task.result).schema === "futures-evidence-v1";
    } catch {
      /* Older answers retain Markdown. */
    }
    let content = task.result;
    if (structured) {
      try {
        const check = await api(
          "/tasks/" + encodeURIComponent(task.id) + "/report-check",
        );
        content = JSON.stringify(
          {
            format: "futures-research-export-v1",
            task_id: task.id,
            raw_result: task.result,
            check,
          },
          null,
          2,
        );
      } catch (e) {
        setNotice("导出未完成：" + (e as Error).message);
        return;
      }
    }
    const blob = new Blob([content], {
      type: structured
        ? "application/json;charset=utf-8"
        : "text/markdown;charset=utf-8",
    });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = task.commodity + (structured ? "-研究记录.json" : "-研究记录.md");
    link.click();
    URL.revokeObjectURL(link.href);
  };
  const sectionKeys = taskSectionKeys(result || { id: selected });
  const diagnosticsState = useTaskDiagnostics(
    result?.id || "",
    result?.status || "",
  );
  const terminalStatus = Boolean(
    result &&
      ["completed", "failed", "cancelled", "waiting_unknown"].includes(
        result.status,
      ),
  );
  if (!connected)
    return authRequired ? (
      <Login error={error} retry={() => void load().catch(connectionError)} />
    ) : error ? (
      <div className="connection-screen">
        <Layers size={32} />
        <h1>工作台暂时无法连接</h1>
        <p>{error}</p>
        <button
          className="button primary"
          onClick={() => void load().catch(connectionError)}
        >
          重新连接
        </button>
      </div>
    ) : (
      <div className="loading-screen">
        <Spinner />
        打开研究工作台…
      </div>
    );
  return (
    <div className="research-shell">
      <a className="skip-link" href="#research-content">
        跳转到研究内容
      </a>
      <aside className="research-sidebar">{sidebar}</aside>
      {mobileNav && (
        <Modal title="研究工作区" close={() => setMobileNav(false)}>
          <div className="modal-body research-drawer">{sidebar}</div>
        </Modal>
      )}
      <div className="research-main">
        <header className="research-topbar">
          <button
            className="mobile-menu icon-button"
            aria-label="展开导航"
            aria-expanded={mobileNav}
            onClick={() => setMobileNav(!mobileNav)}
          >
            <Menu size={19} />
          </button>
          <span>{selected ? "研究任务" : "中国期货 · 基本面研究"}</span>
          <div>
            <span className="workspace-label">
              <i />
              个人资料库
            </span>
            <button className="icon-button" aria-label="新研究" onClick={fresh}>
              <Plus size={17} />
            </button>
          </div>
        </header>
        {error && (
          <div className="connection-banner" role="alert">
            {error}
            <button
              className="text-button"
              onClick={() => void load().catch(connectionError)}
            >
              重试连接
            </button>
          </div>
        )}
        {!selected ? (
          <main className="research-home" id="research-content" tabIndex={-1}>
            <div className="research-greeting">
              <span /> 你的基本面研究工作台
            </div>
            <h1>
              从一个问题，
              <br />
              开始这次研究。
            </h1>
            <p className="hero-subtitle">
              收集真实资料，核对关键变化，保留每一份证据。
            </p>
            <section className="research-composer">
              {revisionSource && (
                <div className="source-task">
                  <b>修订源报告 · {revisionSource.slice(0, 8)}</b>
                  <p>
                    填写需要核查或修改的问题。提交时将固定关联源任务的原文证据，并保存为新任务。
                  </p>
                  <button
                    className="text-button"
                    onClick={() => {
                      setRevisionSource("");
                      key.current = crypto.randomUUID();
                    }}
                  >
                    取消修订关联
                  </button>
                </div>
              )}
              <textarea
                ref={input}
                aria-label="研究问题"
                value={question}
                onChange={(e) => {
                  setQuestion(e.target.value);
                  key.current = crypto.randomUUID();
                }}
                placeholder="例如：整理甲醇最近的供给和库存，核对港口与内地的数据变化…"
              />
              <div className="composer-bottom">
                <div className="composer-tags">
                  <button
                    aria-expanded={showScope}
                    onClick={() => setShowScope(!showScope)}
                  >
                    <span className="commodity-dot" />
                    {commodity}
                    <ChevronDown size={12} />
                  </button>
                  <button onClick={() => setShowScope(!showScope)}>
                    <Clock3 size={13} />
                    {period || "设置时间范围"} · 基准 {asOf}
                  </button>
                  {(action || actions[0]) && (
                    <button
                      className="selected-action"
                      title="当前研究方式"
                      onClick={() => {
                        setAction(null);
                        key.current = crypto.randomUUID();
                      }}
                    >
                      {(action || actions[0])?.name}
                      {action && <X size={11} />}
                    </button>
                  )}
                </div>
                <button
                  className="submit-research"
                  title="开始研究"
                  aria-label="开始研究"
                  disabled={
                    busy ||
                    !question.trim() ||
                    !commodity.trim() ||
                    !actions.length
                  }
                  onClick={() => void run()}
                >
                  {busy ? <Spinner /> : <ArrowUp size={18} />}
                  <span>{busy ? "正在提交" : "开始研究"}</span>
                </button>
              </div>
              {showScope && (
                <div className="scope-fields">
                  <label>
                    研究品种
                    <input
                      value={commodity}
                      onChange={(e) => {
                        setCommodity(e.target.value);
                        key.current = crypto.randomUUID();
                      }}
                      placeholder="输入中国期货品种"
                    />
                  </label>
                  <label>
                    资料时间范围
                    <input
                      value={period}
                      onChange={(e) => {
                        setPeriod(e.target.value);
                        key.current = crypto.randomUUID();
                      }}
                      placeholder="例如：2026 年 7–10 月"
                    />
                  </label>
                  <label>
                    时间分类基准日
                    <input
                      type="date"
                      value={asOf}
                      onChange={(e) => {
                        setAsOf(e.target.value);
                        key.current = crypto.randomUUID();
                      }}
                    />
                    <small>
                      用于区分过去和未来，不代表当时已知数据的回测。
                    </small>
                  </label>
                </div>
              )}
            </section>
            <div className="suggestion-label">
              <span>选择研究方式</span>
              <small>选好研究方式，再补充你关心的问题</small>
            </div>
            {!actions.length && (
              <Empty
                title="研究方式尚未发布"
                detail="工作区管理员发布研究配置后，即可在这里开始。"
              />
            )}
            <div className="research-action-grid">
              {actions.map((a, i) => {
                const Icon = [Layers, FileCheck2, Clock3][i % 3];
                return (
                  <button
                    key={a.id}
                    aria-pressed={(action || actions[0])?.id === a.id}
                    className={
                      "research-action " +
                      ((action || actions[0])?.id === a.id ? "chosen" : "")
                    }
                    onClick={() => {
                      choose(a);
                      if (!question)
                        setQuestion(
                          a.name +
                            "：请围绕核心问题整理关键证据、同口径比较和来源。",
                        );
                      key.current = crypto.randomUUID();
                    }}
                  >
                    <span className={"action-icon action-" + (i % 3)}>
                      <Icon size={20} />
                    </span>
                    <h3>
                      {a.name}
                      <ArrowUpRight size={14} />
                    </h3>
                    <p>{a.description}</p>
                  </button>
                );
              })}
            </div>
            <div className="research-status-note">
              {ready?.knowledge_available ? (
                <>
                  <BookOpen size={13} />
                  资料库已连接 · 数据覆盖需逐项核对
                </>
              ) : (
                <>
                  <FileText size={13} />
                  资料服务暂不可用，请稍后重试
                </>
              )}
            </div>
            <footer className="research-principles">
              <span>
                <Check size={12} />
                注明来源
              </span>
              <span>
                <Check size={12} />
                区分计划与实绩
              </span>
              <span>
                <Check size={12} />
                保留历史口径
              </span>
            </footer>
          </main>
        ) : (
          <main className="research-result" id="research-content" tabIndex={-1}>
            <button className="back-link" onClick={fresh}>
              <ArrowLeft size={15} />
              开始新研究
            </button>
            {readError !== null && (
              <TaskReadFailure
                error={readError}
                hasTask={Boolean(result)}
                reading={reading}
                retry={() => setReadRetry((value) => value + 1)}
              />
            )}
            {result ? (
              <>
                <header className="task-header">
                  <div className="record-label">研究任务</div>
                  <h1>{result.commodity} · 基本面研究</h1>
                  <dl className="task-scope">
                    <div>
                      <dt>研究范围</dt>
                      <dd>{result.period || "未填写"}</dd>
                    </div>
                    <div>
                      <dt>分类基准日</dt>
                      <dd>{result.as_of || "未设置"}</dd>
                    </div>
                    <div>
                      <dt>提交时间</dt>
                      <dd>{date(result.created_at)}</dd>
                    </div>
                  </dl>
                  <div className="research-meta">
                    <Badge state={result.status} />
                    <span className="status-note">
                      {executionNote(result.status)}
                    </span>
                    {result.result && (
                      <EvidenceCheckChip task={result.id} raw={result.result} />
                    )}
                  </div>
                </header>
                {(() => {
                  const related = (tasks || [])
                    .filter(
                      (item) =>
                        item.id !== result.id &&
                        item.commodity === result.commodity &&
                        item.status === "completed",
                    )
                    .slice(0, 3);
                  if (!related.length) return null;
                  return (
                    <div className="related-tasks">
                      <small>同品种历史任务（人工核对，不是自动证据链）</small>
                      <div className="button-row">
                        {related.map((item) => (
                          <button
                            key={item.id}
                            className="text-button"
                            onClick={() => setSelected(item.id)}
                          >
                            {date(item.created_at)} · {item.id.slice(0, 8)}
                          </button>
                        ))}
                      </div>
                    </div>
                  );
                })()}
                <div className="task-actions" role="group" aria-label="任务操作">
                  {result.result && (
                    <button
                      className="button primary"
                      onClick={() => reviseTask(result)}
                    >
                      补充问题，生成新报告
                    </button>
                  )}
                  {result.result && (
                    <button
                      className="button"
                      onClick={() => void exportRecord(result)}
                    >
                      导出记录 <ArrowUpRight size={13} />
                    </button>
                  )}
                  {terminalStatus && (
                    <button
                      className="button quiet"
                      aria-expanded={processOpen}
                      onClick={() => setProcessOpen(!processOpen)}
                    >
                      {processOpen ? "收起生成过程" : "查看生成过程"}
                    </button>
                  )}
                </div>
                {result.result ? (
                  <article className="research-document">
                    <div className="document-heading">
                      <FileText size={19} />
                      <span>研究记录</span>
                      <small className="document-note">
                        记录已生成；研究核验以证据检查为准
                      </small>
                    </div>
                    <ResearchAnswer
                      key={sectionKeys.report}
                      task={result.id}
                      instance={"final:" + result.id}
                    >
                      {result.result}
                    </ResearchAnswer>
                  </article>
                ) : terminalStatus ? (
                  <TaskFailureView
                    status={result.status}
                    diagnostics={diagnosticsState.diagnostics}
                    error={diagnosticsState.error}
                    loading={diagnosticsState.loading}
                    retry={diagnosticsState.retry}
                    onNewResearch={() => restartFrom(result)}
                  />
                ) : (
                  <div className="research-wait">
                    <div className="research-spinner" />
                    <h3>正在整理这次研究</h3>
                    <p>已完成的进度会自动保存，你可以稍后回来查看。</p>
                  </div>
                )}
                {result.result && (
                  <TaskEvidencePanel
                    task={result}
                    open={setSelected}
                    refresh={() => setReadRetry((value) => value + 1)}
                  />
                )}
                {terminalStatus && processOpen && (
                  <section className="generation-panel" aria-label="生成过程">
                    <h2>生成过程</h2>
                    <p className="panel-note">
                      执行步骤、留存的检查草稿与资料，只读；展开或导出不会重新执行研究。
                    </p>
                    <ResearchProgress status={result.status} />
                    <TaskRetainedWork
                      key={sectionKeys.retained}
                      task={result.id}
                      status={result.status}
                      hasResult={Boolean(result.result)}
                    />
                  </section>
                )}
                {!terminalStatus && (
                  <ResearchProgress status={result.status} />
                )}
                {["running", "queued", "waiting_children"].includes(
                  result.status,
                ) && (
                  <button
                    className="back-link"
                    onClick={() =>
                      void api("/tasks/" + result.id + "/cancel", {}).catch(
                        (e) => setNotice(e.message),
                      )
                    }
                  >
                    停止研究
                  </button>
                )}
              </>
            ) : readError === null ? (
              <div className="research-wait">
                <Spinner />
              </div>
            ) : null}
          </main>
        )}
      </div>
      {notice && (
        <div className="toast" role="status">
          <span>{notice}</span>
          <button
            className="icon-button"
            aria-label="关闭通知"
            onClick={() => setNotice("")}
          >
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  );
}
