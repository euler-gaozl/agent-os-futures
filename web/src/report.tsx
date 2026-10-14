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
const measure: Record<string, string> = {
  stock: "时点库存",
  flow: "区间流量",
  cumulative_flow: "累计流量",
  capacity: "产能",
};
export function CheckedReport({
  check,
  raw,
  onSource,
}: {
  check: ReportCheck;
  raw: string;
  onSource: OpenSource;
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
            <h3>待核研究问题</h3>
            <p>由 Agent 整理的调查线索，问题中的假设不作为结论。</p>
            {report.questions.map((q) => (
              <article key={q.id}>
                <h4>{q.question}</h4>
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
          <h3>来源报数与口径</h3>
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
                      {f.period.start === f.period.end
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
            <summary>原始模型输出（完整保留）</summary>
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
    [error, setError] = useState(""),
    [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let live = true;
    setCheck(null);
    setError("");
    api<ReportCheck>("/tasks/" + encodeURIComponent(task) + "/report-check")
      .then((v) => {
        if (live) setCheck(v);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, [task, raw, attempt]);
  if (error)
    return (
      <>
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
  return <CheckedReport check={check} raw={raw} onSource={onSource} />;
}
