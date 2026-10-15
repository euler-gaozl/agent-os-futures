import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { ResearchAnswer } from "./evidence";
import { TaskRetainedWork } from "./drafts";
import { TaskFailureNotice, TaskReadFailure } from "./task-status";
import { taskReadReducer, watchTask } from "./task-polling";
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
import "./futures.css";
import "./report.css";
type Action = {
  id: string;
  revision: number;
  name: string;
  description: string;
};
function Futures() {
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
                <header>
                  <div className="record-label">已保存的研究任务</div>
                  <h1>{result.commodity} · 基本面研究</h1>
                  <div className="research-meta">
                    <Badge state={result.status} />
                    <span>{date(result.created_at)}</span>
                  </div>
                </header>
                <div className="research-progress">
                  <div>
                    <span className="progress-number">1</span>
                    <b>接收研究任务</b>
                    <Check size={15} />
                  </div>
                  <div
                    className={
                      ["queued"].includes(result.status) ? "pending" : ""
                    }
                  >
                    <span className="progress-number">2</span>
                    <b>执行研究任务</b>
                    {result.status === "running" ? (
                      <Spinner />
                    ) : result.status === "completed" ? (
                      <Check size={15} />
                    ) : null}
                  </div>
                  <div
                    className={result.status === "completed" ? "" : "pending"}
                  >
                    <span className="progress-number">3</span>
                    <b>生成研究记录</b>
                    {result.status === "completed" && <Check size={15} />}
                  </div>
                </div>
                {result.result ? (
                  <article className="research-document">
                    <div className="document-heading">
                      <FileText size={19} />
                      <span>研究记录</span>
                      <button
                        className="text-button"
                        onClick={async () => {
                          let structured = false;
                          try {
                            structured =
                              JSON.parse(result.result).schema ===
                              "futures-evidence-v1";
                          } catch {
                            /* Older answers retain Markdown. */
                          }
                          let content = result.result;
                          if (structured) {
                            try {
                              const check = await api(
                                "/tasks/" +
                                  encodeURIComponent(result.id) +
                                  "/report-check",
                              );
                              content = JSON.stringify(
                                {
                                  format: "futures-research-export-v1",
                                  task_id: result.id,
                                  raw_result: result.result,
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
                          link.download =
                            result.commodity +
                            (structured ? "-研究记录.json" : "-研究记录.md");
                          link.click();
                          URL.revokeObjectURL(link.href);
                        }}
                      >
                        导出记录 <ArrowUpRight size={13} />
                      </button>
                    </div>
                    <ResearchAnswer key={result.id} task={result.id}>
                      {result.result}
                    </ResearchAnswer>
                  </article>
                ) : result.status === "failed" ? (
                  <TaskFailureNotice error={result.error || ""} />
                ) : result.status === "waiting_unknown" ? (
                  <div className="research-failure">
                    <h3>有一项执行结果需要核对</h3>
                    <p>
                      当前任务已暂停。管理员核实外部服务回执后，才能继续处理。
                    </p>
                  </div>
                ) : result.status === "cancelled" ? (
                  <Empty
                    title="研究已停止"
                    detail="已有资料和执行记录仍保留在工作区。"
                  />
                ) : (
                  <div className="research-wait">
                    <div className="research-spinner" />
                    <h3>正在整理这次研究</h3>
                    <p>已完成的进度会自动保存，你可以稍后回来查看。</p>
                  </div>
                )}
                <TaskRetainedWork
                  task={result.id}
                  status={result.status}
                  hasResult={Boolean(result.result)}
                />
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
createRoot(document.getElementById("app")!).render(<Futures />);
