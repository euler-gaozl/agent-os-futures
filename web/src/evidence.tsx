import React, { useEffect, useRef, useState } from "react";
import { ArrowUpRight, BookOpen, ChevronDown, FileText } from "lucide-react";
import { api, Modal, Spinner } from "./shared";
import { ReportReview, type SourceProof, type OpenSource } from "./report";

export type Evidence = {
  operation_id: string;
  run_id: string;
  document_id: string;
  title: string;
  source: string;
  kind: string;
  preview: string;
  preview_truncated: boolean;
  source_truncated: boolean;
  read_offset: number;
  read_end: number;
  has_more: boolean;
  recorded_at: string;
};
type Page = { items: Evidence[]; next_cursor: string };
export type Part = Evidence & {
  text: string;
  offset: number;
  next_offset: number | null;
  version: string;
};

// Open the exact immutable receipt used by the fact check, including a quote
// beyond its first page. Offsets from the backend count Unicode code points.
export async function loadSourceQuote(
  base: string,
  document: string,
  proof: SourceProof,
  read = api<Part>,
): Promise<Part> {
  if (
    !proof.operation_id ||
    !/^[a-f0-9]{64}$/.test(proof.version) ||
    !Number.isSafeInteger(proof.quote_offset) ||
    proof.quote_offset < 0 ||
    !proof.quote ||
    Array.from(proof.quote).length > 2400
  )
    throw new Error("引用定位信息无效。");
  const path =
    base +
    "/record?operation_id=" +
    encodeURIComponent(proof.operation_id) +
    "&version=" +
    proof.version +
    "&limit=8192&offset=";
  const validate = (p: Part, offset: number) => {
    if (
      p.document_id !== document ||
      p.operation_id !== proof.operation_id ||
      p.version !== proof.version ||
      p.offset !== offset ||
      !Number.isSafeInteger(p.read_offset) ||
      !Number.isSafeInteger(p.read_end) ||
      p.read_offset < 0 ||
      p.read_end < p.read_offset
    )
      throw new Error("保存的阅读记录与引用版本不一致。");
  };
  let part = await read(path + "0");
  validate(part, 0);
  const offset = proof.quote_offset - part.read_offset,
    size = Array.from(proof.quote).length;
  if (offset < 0 || proof.quote_offset + size > part.read_end)
    throw new Error("引用超出该阅读片段。");
  if (offset + size > Array.from(part.text).length) {
    const next = await read(path + offset);
    validate(next, offset);
    if (
      next.read_offset !== part.read_offset ||
      next.read_end !== part.read_end
    )
      throw new Error("保存的阅读范围发生变化。");
    part = next;
  }
  if (
    Array.from(part.text)
      .slice(offset - part.offset, offset - part.offset + size)
      .join("") !== proof.quote
  )
    throw new Error("指定位置没有匹配的留存原文。");
  return part;
}

function SourceText({ part, proof }: { part: Part; proof?: SourceProof }) {
  const highlight = useRef<HTMLElement>(null);
  useEffect(() => {
    highlight.current?.scrollIntoView({ block: "center" });
  }, [part.operation_id, part.offset, proof]);
  const text = Array.from(part.text),
    start = proof ? proof.quote_offset - part.read_offset - part.offset : -1;
  const size = proof ? Array.from(proof.quote).length : 0;
  if (
    !proof ||
    start < 0 ||
    text.slice(start, start + size).join("") !== proof.quote
  )
    return <pre className="source-text">{part.text}</pre>;
  return (
    <pre className="source-text">
      {text.slice(0, start).join("")}
      <mark ref={highlight} aria-label="本条事实引用">
        {proof.quote}
      </mark>
      {text.slice(start + size).join("")}
    </pre>
  );
}

export function sourceDetails(item: Evidence) {
  // These are labels from the saved material, not newly verified metadata.
  const previewLines = item.preview.split("\n");
  if ((item.preview_truncated || item.has_more) && !item.preview.endsWith("\n"))
    previewLines.pop();
  const lines = item.read_offset === 0 ? previewLines.slice(0, 16) : [];
  const value = (label: string) =>
    lines
      .find((line) => line.startsWith(label + "："))
      ?.slice(label.length + 1)
      .trim() || "";
  const candidate = value("来源") || (item.source_truncated ? "" : item.source);
  let url = "";
  try {
    const parsed = new URL(candidate);
    if (
      ["http:", "https:"].includes(parsed.protocol) &&
      !parsed.username &&
      !parsed.password
    )
      url = parsed.href;
  } catch {
    /* Local uploaded material has no external URL. */
  }
  return { url, published: value("发布日期"), period: value("统计范围") };
}

export function uniqueSources(items: Evidence[]) {
  const docs = new Map<string, Evidence>();
  for (const item of items) {
    const old = docs.get(item.document_id);
    if (!old || item.read_offset < old.read_offset)
      docs.set(item.document_id, item);
  }
  return [...docs.values()];
}

export function SourceDialog({
  task,
  document,
  proof,
  close,
}: {
  task: string;
  document: string;
  proof?: SourceProof;
  close: () => void;
}) {
  const [records, setRecords] = useState<Evidence[]>([]),
    [cursor, setCursor] = useState("");
  const [selected, setSelected] = useState(proof?.operation_id || ""),
    [part, setPart] = useState<Part | null>(null);
  const [pinned, setPinned] = useState(!!proof);
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [listing, setListing] = useState(true);
  const generation = useRef(0),
    mounted = useRef(true);
  const base = "/tasks/" + encodeURIComponent(task) + "/sources";
  useEffect(() => {
    mounted.current = true;
    let live = true;
    if (proof) {
      setListing(false);
      return () => {
        mounted.current = false;
        generation.current++;
      };
    }
    api<Page>(base + "?document_id=" + encodeURIComponent(document))
      .then((page) => {
        if (!live) return;
        setRecords(page.items);
        setCursor(page.next_cursor);
        const first = [...page.items].sort(
          (a, b) => a.read_offset - b.read_offset,
        )[0];
        if (first) setSelected(first.operation_id);
        else setError("这份引用不在本次任务的已保存阅读记录中。");
      })
      .catch((e) => {
        if (live) setError(e.message);
      })
      .finally(() => {
        if (live) setListing(false);
      });
    return () => {
      live = false;
      mounted.current = false;
      generation.current++;
    };
  }, [base, document, proof]);
  useEffect(() => {
    if (!selected) return;
    const version = ++generation.current;
    setPart(null);
    setBusy(true);
    setError("");
    const reading =
      proof && pinned
        ? loadSourceQuote(base, document, proof)
        : api<Part>(
            base +
              "/record?operation_id=" +
              encodeURIComponent(selected) +
              (proof ? "&version=" + proof.version : ""),
          );
    reading
      .then((value) => {
        if (mounted.current && version === generation.current) setPart(value);
      })
      .catch((e) => {
        if (mounted.current && version === generation.current)
          setError(e.message);
      })
      .finally(() => {
        if (mounted.current && version === generation.current) setBusy(false);
      });
    return () => {
      generation.current++;
    };
  }, [base, selected, document, proof, pinned]);
  const moreRecords = async () => {
    setListing(true);
    setError("");
    try {
      const page = await api<Page>(
        base +
          "?document_id=" +
          encodeURIComponent(document) +
          "&after=" +
          encodeURIComponent(cursor),
      );
      if (mounted.current) {
        setRecords((old) => [...old, ...page.items]);
        setCursor(page.next_cursor);
      }
    } catch (e) {
      if (mounted.current) setError((e as Error).message);
    } finally {
      if (mounted.current) setListing(false);
    }
  };
  const moreText = async () => {
    if (!part || part.next_offset === null) return;
    const version = generation.current;
    setBusy(true);
    setError("");
    try {
      const next = await api<Part>(
        base +
          "/record?operation_id=" +
          encodeURIComponent(selected) +
          "&offset=" +
          part.next_offset +
          "&version=" +
          part.version,
      );
      if (mounted.current && version === generation.current)
        setPart((old) =>
          old
            ? { ...next, text: old.text + next.text, offset: old.offset }
            : next,
        );
    } catch (e) {
      if (mounted.current && version === generation.current)
        setError((e as Error).message);
    } finally {
      if (mounted.current && version === generation.current) setBusy(false);
    }
  };
  const record = part || records.find((x) => x.operation_id === selected);
  const meta = record ? sourceDetails(record) : null;
  return (
    <Modal title="阅读原文" close={close} wide>
      <div
        className="modal-body source-reader"
        tabIndex={0}
        role="region"
        aria-label="保存的原文"
      >
        {record && (
          <>
            <h3>{record.title}</h3>
            <p className="source-description">
              任务当时保存的阅读片段。保留原资料中的说法，不代表其已被独立核实。
            </p>
            <dl className="source-metadata">
              {meta?.published && (
                <>
                  <dt>资料标注发布日期</dt>
                  <dd>{meta.published}</dd>
                </>
              )}
              {meta?.period && (
                <>
                  <dt>统计范围</dt>
                  <dd>{meta.period}</dd>
                </>
              )}
              <dt>本次阅读位置</dt>
              <dd>
                第 {record.read_offset + 1}–{record.read_end} 个字符
                {record.has_more
                  ? " · 此次读取后原文还有内容"
                  : " · 已读到该文档末尾"}
              </dd>
            </dl>
            {meta?.url && (
              <a
                className="source-original"
                href={meta.url}
                target="_blank"
                rel="noopener noreferrer"
              >
                打开资料标注的来源网页 <ArrowUpRight size={14} />
              </a>
            )}
          </>
        )}
        {records.length > 1 && (
          <label className="source-segments">
            已保存的阅读片段
            <select
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
            >
              {[...records]
                .sort((a, b) => a.read_offset - b.read_offset)
                .map((item) => (
                  <option key={item.operation_id} value={item.operation_id}>
                    {item.read_offset + 1}–{item.read_end} 字符
                  </option>
                ))}
            </select>
          </label>
        )}
        {cursor && (
          <button
            className="text-button"
            disabled={listing}
            onClick={() => void moreRecords()}
          >
            加载更早的阅读片段
          </button>
        )}
        {(busy || listing) && (
          <p className="source-description">
            <Spinner /> 正在读取已保存资料…
          </p>
        )}
        {error && (
          <p className="source-error" role="alert">
            {error}
          </p>
        )}
        {part && proof && (
          <p className="source-description">
            已锁定引用的阅读回执与版本 · 第 {proof.quote_offset + 1} 个字符
          </p>
        )}
        {part && proof && pinned && part.offset > 0 && (
          <button className="text-button" onClick={() => setPinned(false)}>
            从此片段开头阅读
          </button>
        )}
        {part && <SourceText part={part} proof={proof} />}
        {part?.next_offset !== null && part && (
          <button
            className="button"
            disabled={busy}
            onClick={() => void moreText()}
          >
            继续读取此片段
          </button>
        )}
        {record && (
          <details className="source-details">
            <summary>来源定位信息</summary>
            <p>文档标识：{record.document_id}</p>
            <p>
              阅读时间：{new Date(record.recorded_at).toLocaleString("zh-CN")}
            </p>
          </details>
        )}
      </div>
    </Modal>
  );
}

export function ResearchAnswer({
  task,
  children,
}: {
  task: string;
  children?: string;
}) {
  const [items, setItems] = useState<Evidence[]>([]),
    [cursor, setCursor] = useState("");
  const [busy, setBusy] = useState(true),
    [error, setError] = useState(""),
    [open, setOpen] = useState(false),
    [source, setSource] = useState<{
      document: string;
      proof?: SourceProof;
    } | null>(null);
  const openSource: OpenSource = (document, proof) =>
    setSource({ document, proof });
  const mounted = useRef(true);
  const base = "/tasks/" + encodeURIComponent(task) + "/sources";
  useEffect(() => {
    mounted.current = true;
    let live = true;
    api<Page>(base)
      .then((page) => {
        if (live) {
          setItems(page.items);
          setCursor(page.next_cursor);
        }
      })
      .catch((e) => {
        if (live)
          setError(
            e.status === 404
              ? "当前服务暂不支持阅读记录回查，请更新工作区服务。"
              : e.message,
          );
      })
      .finally(() => {
        if (live) setBusy(false);
      });
    return () => {
      live = false;
      mounted.current = false;
    };
  }, [base]);
  const more = async () => {
    setBusy(true);
    setError("");
    try {
      const page = await api<Page>(
        base + "?after=" + encodeURIComponent(cursor),
      );
      if (mounted.current) {
        setItems((old) => [...old, ...page.items]);
        setCursor(page.next_cursor);
      }
    } catch (e) {
      if (mounted.current) setError((e as Error).message);
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  const docs = uniqueSources(items);
  return (
    <>
      <section className="evidence-section" aria-label="资料回查">
        <button
          className="evidence-toggle"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          <BookOpen size={17} />
          <span>
            资料回查
            <small>
              {busy
                ? "正在读取"
                : error
                  ? "暂不可用"
                  : `${docs.length} 份已加载的阅读资料`}
            </small>
          </span>
          <ChevronDown size={16} />
        </button>
        {error && (
          <p className="source-error" role="alert">
            {error}
          </p>
        )}
        {open && (
          <div className="evidence-content">
            <p className="source-description">
              这里列出本次任务实际保存的文档阅读记录，可与报告逐项核对。列表不代表资料库全量或事实核验结论。
            </p>
            {!busy && !docs.length && !error && (
              <p>
                本次任务没有可展示的文档阅读记录；其他采集工具的记录尚未在此列出。
              </p>
            )}
            <div className="source-grid">
              {docs.map((item) => {
                const details = sourceDetails(item);
                return (
                  <button
                    className="source-card"
                    key={item.document_id}
                    onClick={() => openSource(item.document_id)}
                  >
                    <FileText size={18} />
                    <span>
                      <b>{item.title.replace(/\.md$/i, "")}</b>
                      <small>
                        {details.published
                          ? "发布于 " + details.published
                          : "查看保存的阅读片段"}
                      </small>
                    </span>
                    <ArrowUpRight size={14} />
                  </button>
                );
              })}
            </div>
            {cursor && (
              <button
                className="text-button"
                disabled={busy}
                onClick={() => void more()}
              >
                加载更多阅读记录
              </button>
            )}
          </div>
        )}
      </section>
      {children && (
        <ReportReview task={task} raw={children} onSource={openSource} />
      )}
      {source && (
        <SourceDialog
          key={
            task +
            ":" +
            source.document +
            ":" +
            source.proof?.operation_id +
            ":" +
            source.proof?.quote_offset
          }
          task={task}
          document={source.document}
          proof={source.proof}
          close={() => setSource(null)}
        />
      )}
    </>
  );
}
