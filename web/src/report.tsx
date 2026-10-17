import React, { useEffect, useState } from "react";
import { api, Spinner } from "./shared";
import { Answer } from "./markdown";
export type SourceProof = {
  operation_id: string;
  version: string;
  quote_offset: number;
  quote: string;
};
export type OpenSource = (id: string, proof?: SourceProof) => void;
export type DraftMatch = {
  status: "matched" | "changed" | "not_found" | "unavailable" | "not_final";
};
export function DraftMatchNotice({ match }: { match: DraftMatch }) {
  const messages: Record<DraftMatch["status"], string> = {
    matched: "最终回答与最后送检稿逐字一致。内容与来源仍须分别核对。",
    changed:
      "最终回答在最后送检后被改动。先前的草稿检查不能代表这份回答；下方保留送检稿与检查反馈，可对照复核。",
    not_found: "本任务未找到留存的送检稿，无法核对最终回答是否经过草稿检查。",
    unavailable:
      "暂时无法核对最终回答与最后送检稿是否一致。原回答保持显示，不视为已经通过。",
    not_final: "任务尚无完成状态的最终回答，留存草稿不能视为最终交付。",
  };
  return (
    <p
      className="draft-match-notice"
      role={match.status === "changed" ? "alert" : "status"}
    >
      {messages[match.status] ?? messages.unavailable}
    </p>
  );
}

type Fact = {
  id: string;
  metric: string;
  value: string;
  unit: string;
  scope: string;
  measure: string;
  nature: string;
  period: { start: string; end: string };
  source: { document_id: string; quote: string; period_text: string };
  evidence?: {
    content_kind: string;
    period_basis: string;
    period_note: string;
    limitations: string[];
  };
};
export type ReportCheck = {
  status: string;
  notice: string;
  validator: string;
  result_sha256: string;
  classification_date: string;
  report?: {
    schema: string;
    as_of: string;
    questions: {
      id: string;
      question: string;
      fact_ids: string[];
      comparison_ids: string[];
      missing: string[];
      findings?: {
        kind: string;
        text: string;
        fact_ids: string[];
        comparison_ids: string[];
      }[];
    }[];
    facts: Fact[];
    comparisons: { id: string; current: string; baseline: string }[];
  };
  facts: {
    id: string;
    group: string;
    quote_matched: boolean;
    consistent: boolean;
    operation_id?: string;
    version?: string;
    quote_offset: number;
    source_url?: string;
    published_label?: string;
    captured_label?: string;
  }[];
  comparisons: {
    id: string;
    from: string;
    to: string;
    difference: string;
    unit: string;
    consistent: boolean;
  }[];
  issues: { path: string; code: string; message: string }[];
};
const nature: Record<string, string> = {
  reported: "来源报数",
  estimate: "第三方估计",
  plan: "披露计划",
};
const evidenceKind: Record<string, string> = {
  publisher_text: "发布正文（来源仍需核对）",
  ai_summary: "AI 摘要线索 · 未确认",
  unverified: "来源性质未确认",
};
const periodBasis: Record<string, string> = {
  explicit: "原文明确标注（待核对）",
  inferred: "日期含推断 · 未确认",
  unknown: "统计期未确认",
};
const measure: Record<string, string> = {
  stock: "时点库存",
  flow: "区间流量",
  cumulative_flow: "累计流量",
  capacity: "产能",
  ratio: "比率",
  commitment: "未执行合同",
};
export function CheckedReport({
  check,
  raw,
  onSource,
  rawLabel = "原始模型输出（完整保留）",
}: {
  check: ReportCheck;
  raw: string;
  onSource: OpenSource;
  rawLabel?: string;
}) {
  const report = check.report;
  return (
    <section className="checked-report" aria-label="资料结构核对">
      <header
        className={
          "report-check-status " +
          (check.status === "consistent" ? "consistent" : "pending")
        }
      >
        <b>
          {check.status === "consistent"
            ? "结构与留存引用检查一致"
            : "这份记录仍有待核项"}
        </b>
        <p>{check.notice}</p>
        {check.classification_date && (
          <small>时间分类基准日：{check.classification_date}</small>
        )}
      </header>
      {!!check.issues.length && (
        <details className="report-issues">
          <summary>{check.issues.length} 项待核记录 · 展开查看</summary>
          <ul>
            {check.issues.map((i, n) => (
              <li key={n}>
                <code>{i.path}</code> {i.message}
              </li>
            ))}
          </ul>
        </details>
      )}
      {report ? (
        <>
          <section className="report-questions">
            <h3>核心问题与资料解读</h3>
            <p>解读由 Agent 根据关联资料整理；来源限制和待核项仍适用。</p>
            {report.questions.map((q) => (
              <article key={q.id}>
                <h4>{q.question}</h4>
                {!!q.findings?.length && (
                  <div className="report-findings">
                    {q.findings.map((finding, i) => (
                      <div className="report-finding" key={i}>
                        <b>
                          {finding.kind === "conflict"
                            ? "来源冲突"
                            : finding.kind === "limitation"
                              ? "证据边界"
                              : finding.kind === "observation"
                                ? "资料观察"
                                : "类型待核"}{" "}
                          · 待核
                        </b>
                        <p>{finding.text}</p>
                        <div className="fact-links">
                          {finding.fact_ids?.map((id) => (
                            <a key={id} href={"#report-fact-" + id}>
                              依据 {id}
                            </a>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
                <div className="fact-links">
                  {q.fact_ids.map((id) => (
                    <a key={id} href={"#report-fact-" + id}>
                      资料 {id}
                    </a>
                  ))}
                </div>
                {q.missing.length > 0 && (
                  <>
                    <b>本次记录列出的待查资料</b>
                    <ul>
                      {q.missing.map((m, i) => (
                        <li key={i}>{m}</li>
                      ))}
                    </ul>
                    <small>
                      这里不代表资料库全量、外部数据不存在，或调查已经完成。
                    </small>
                  </>
                )}
              </article>
            ))}
          </section>
          <h3>报数、线索与来源限制</h3>
          {!report.facts.length && (
            <p>尚未提供可逐项回查的报数，请继续补充关键资料。</p>
          )}
          <div className="report-facts">
            {report.facts.map((f, n) => {
              const state = check.facts[n];
              const issues = check.issues.filter(
                (i) => i.path === "facts." + f.id,
              );
              return (
                <article
                  id={"report-fact-" + f.id}
                  key={f.id + ":" + n}
                  className={
                    state?.consistent ? "fact-card" : "fact-card needs-review"
                  }
                >
                  {f.evidence && (
                    <div className="evidence-declaration">
                      <b>
                        资料性质（Agent 标注）：
                        {evidenceKind[f.evidence.content_kind] || "性质待核"}
                      </b>
                      <p>
                        {periodBasis[f.evidence.period_basis] || "日期依据待核"}
                        {f.evidence.period_note
                          ? "：" + f.evidence.period_note
                          : ""}
                      </p>
                      {!!f.evidence.limitations?.length && (
                        <ul>
                          {f.evidence.limitations.map((note, i) => (
                            <li key={i}>{note}</li>
                          ))}
                        </ul>
                      )}
                    </div>
                  )}
                  <div className="fact-kind">
                    <span>
                      Agent 标注：{nature[f.nature] || "属性待核"} ·{" "}
                      {measure[f.measure] || "统计类型待核"}
                    </span>
                    <span>
                      {state?.consistent
                        ? "引用与字段规则一致"
                        : "待核，暂不用于比较"}
                    </span>
                  </div>
                  <h4>
                    {f.metric}{" "}
                    <strong>
                      {f.value} <small>{f.unit}</small>
                    </strong>
                  </h4>
                  <dl>
                    <dt>样本范围（资料标注）</dt>
                    <dd>{f.scope}</dd>
                    <dt>统计区间</dt>
                    <dd>
                      {!f.period.start ||
                      !f.period.end ||
                      f.evidence?.period_basis === "unknown"
                        ? "统计日期尚未确认"
                        : f.period.start === f.period.end
                          ? f.period.start
                          : f.period.start + " — " + f.period.end}
                    </dd>
                    <dt>时间分组</dt>
                    <dd>{state?.group || "待核"}</dd>
                    <dt>原始出处（资料标注）</dt>
                    <dd>
                      {state?.source_url ? (
                        <a
                          href={state.source_url}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {state.source_url}
                        </a>
                      ) : (
                        "本条留存记录未提供可用网址"
                      )}
                    </dd>
                    <dt>发布 / 原始采集</dt>
                    <dd>
                      {state?.published_label || "本条留存记录未提供发布时间"} /{" "}
                      {state?.captured_label || "本条留存记录未提供采集时间"}
                    </dd>
                  </dl>
                  {issues.map((i, k) => (
                    <p className="fact-issue" key={k}>
                      {i.message}
                    </p>
                  ))}
                  <details>
                    <summary>
                      查看引用片段
                      {state?.quote_matched
                        ? " · 已匹配留存原文"
                        : " · 尚未匹配"}
                    </summary>
                    <blockquote>{f.source.quote}</blockquote>
                    {state?.quote_matched &&
                      state.operation_id &&
                      state.version && (
                        <button
                          className="source-citation"
                          onClick={() =>
                            onSource(f.source.document_id, {
                              operation_id: state.operation_id!,
                              version: state.version!,
                              quote_offset: state.quote_offset,
                              quote: f.source.quote,
                            })
                          }
                        >
                          回查任务当时保存的资料
                        </button>
                      )}
                    {state?.operation_id && (
                      <p className="receipt-label">
                        阅读回执：{state.operation_id} · 第{" "}
                        {(state.quote_offset ?? 0) + 1} 字符
                      </p>
                    )}
                  </details>
                </article>
              );
            })}
          </div>
          {!!report.comparisons.length && (
            <section className="report-comparisons">
              <h3>同口径对照</h3>
              <p>
                仅计算检查一致且原文明确写明同口径的报数；保留两个原始统计窗口，不改称某月增减。
              </p>
              {check.comparisons.map((c, n) => (
                <article key={c.id + ":" + n}>
                  {c.consistent ? (
                    <>
                      <b>
                        {c.from} → {c.to}
                      </b>
                      <p>
                        后值减前值：{c.difference} {c.unit}
                      </p>
                      <small>
                        这是来源标注口径下的算术差，不是因果或交易判断。
                      </small>
                    </>
                  ) : (
                    <p>比较 {c.id} 尚未满足口径条件，未计算差额。</p>
                  )}
                </article>
              ))}
            </section>
          )}
          <details className="report-original">
            <summary>{rawLabel}</summary>
            <pre>{raw}</pre>
          </details>
        </>
      ) : (
        <>
          <p className="source-description">
            以下保留原始回答，尚未通过结构化资料检查；运行完成不等于研究核验完成。
          </p>
          <Answer onSource={onSource}>{raw}</Answer>
        </>
      )}
    </section>
  );
}
export function ReportReview({
  task,
  raw,
  onSource,
}: {
  task: string;
  raw: string;
  onSource: OpenSource;
}) {
  const [check, setCheck] = useState<ReportCheck | null>(null),
    [draftMatch, setDraftMatch] = useState<DraftMatch | null>(null),
    [error, setError] = useState(""),
    [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let live = true;
    setCheck(null);
    setDraftMatch(null);
    setError("");
    api<ReportCheck>("/tasks/" + encodeURIComponent(task) + "/report-check")
      .then((v) => {
        if (live) setCheck(v);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    api<DraftMatch>("/tasks/" + encodeURIComponent(task) + "/draft-match")
      .then((v) => {
        if (live) setDraftMatch(v);
      })
      .catch(() => {
        if (live) setDraftMatch({ status: "unavailable" });
      });
    return () => {
      live = false;
    };
  }, [task, raw, attempt]);
  if (error)
    return (
      <>
        {draftMatch && <DraftMatchNotice match={draftMatch} />}
        <p className="source-error" role="alert">
          资料核对服务暂不可用：{error}{" "}
          <button
            className="text-button"
            onClick={() => setAttempt((n) => n + 1)}
          >
            重试核对
          </button>
        </p>
        <Answer onSource={onSource}>{raw}</Answer>
      </>
    );
  if (!check)
    return (
      <p className="source-description" role="status">
        <Spinner /> 正在核对结构、时间与本次留存引用…
      </p>
    );
  return (
    <>
      {draftMatch && <DraftMatchNotice match={draftMatch} />}
      {draftMatch?.status === "unavailable" && (
        <button className="text-button" onClick={() => setAttempt((n) => n + 1)}>
          重新核对送检稿
        </button>
      )}
      <CheckedReport check={check} raw={raw} onSource={onSource} />
    </>
  );
}
