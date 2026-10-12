import { useEffect, useRef, useState, type ReactNode } from "react";
import { X, LoaderCircle, FileText, ArrowUpRight } from "lucide-react";
export const labels: Record<string, string> = {
  healthy: "检查通过",
  unhealthy: "验活失败",
  unchecked: "待检测",
  drifted: "工具定义已变化",
  rejected: "安全检查未通过",
  queued: "排队中",
  running: "执行中",
  waiting_children: "等待子任务",
  waiting_unknown: "待核对",
  completed: "已完成",
  failed: "执行失败",
  cancelled: "已取消",
  ready: "可检索",
  external_pending: "正在构建索引",
  unknown: "回执待核对",
};
export async function api<T = any>(
  path: string,
  body?: unknown,
  key?: string,
): Promise<T> {
  if (body && typeof body === "object" && "summary" in body && body.summary)
    throw new Error("目录摘要不能作为资源定义保存，请先读取完整版本。");
  const token = sessionStorage.getItem("admin-token");
  const response = await fetch("/api" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      "Content-Type": "application/json",
      "X-AgentOS-Client": "web",
      "Idempotency-Key": key || crypto.randomUUID(),
      ...(token ? { Authorization: "Bearer " + token } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    const error = await response
      .json()
      .catch(() => ({ error: response.statusText }));
    throw Object.assign(
      new Error(
        error.error ||
          error.findings?.map((f: any) => f.Message || f.message).join("；") ||
          response.statusText,
      ),
      {
        status: response.status,
        noteRejectedRequestId: error.note_rejected_request_id,
      },
    );
  }
  return response.json();
}
export function Badge({ state }: { state: string }) {
  return (
    <span className={"badge " + state}>
      <i />
      {labels[state] || state}
    </span>
  );
}
export async function downloadArtifact(
  hash: string,
  name: string,
  knowledge = false,
) {
  const token = sessionStorage.getItem("admin-token");
  const response = await fetch(
    (knowledge ? "/api/knowledge/artifacts/" : "/api/artifacts/") +
      encodeURIComponent(hash),
    {
      headers: {
        "X-AgentOS-Client": "web",
        ...(token ? { Authorization: "Bearer " + token } : {}),
      },
    },
  );
  if (!response.ok) throw new Error("下载失败：" + response.status);
  const url = URL.createObjectURL(await response.blob());
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function Modal({
  title,
  children,
  close,
  wide = false,
}: {
  title: string;
  children: ReactNode;
  close: () => void;
  wide?: boolean;
}) {
  const panel = useRef<HTMLElement>(null);
  const closeRef = useRef(close);
  closeRef.current = close;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    panel.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        closeRef.current();
        return;
      }
      if (event.key !== "Tab") return;
      const items = Array.from(
        panel.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary, [tabindex="0"]',
        ) || [],
      ).filter((el) => el.getClientRects().length > 0);
      const first = items[0],
        last = items[items.length - 1];
      if (!first) {
        event.preventDefault();
        panel.current?.focus();
        return;
      }
      if (
        event.shiftKey &&
        (document.activeElement === first ||
          document.activeElement === panel.current)
      ) {
        event.preventDefault();
        last.focus();
      } else if (
        !event.shiftKey &&
        (document.activeElement === last ||
          document.activeElement === panel.current)
      ) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      if (previous?.isConnected) previous.focus();
    };
  }, []);
  return (
    <div
      className="overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) close();
      }}
    >
      <section
        ref={panel}
        tabIndex={-1}
        className={"modal " + (wide ? "wide" : "")}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header>
          <h2>{title}</h2>
          <button className="icon-button" aria-label="关闭" onClick={close}>
            <X size={18} />
          </button>
        </header>
        {children}
      </section>
    </div>
  );
}
export function Empty({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <div className="empty-symbol">
        <FileText size={25} />
      </div>
      <h3>{title}</h3>
      <p>{detail}</p>
      {action}
    </div>
  );
}
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function Login({ error, retry }: { error: string; retry: () => void }) {
  const [token, setToken] = useState("");
  return (
    <div className="login">
      <div className="logo-symbol">a</div>
      <h1>连接你的工作区</h1>
      <p>{error}</p>
      <Field label="访问令牌">
        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
      </Field>
      <button
        className="button primary"
        onClick={() => {
          sessionStorage.setItem("admin-token", token);
          retry();
        }}
      >
        连接工作区 <ArrowUpRight size={16} />
      </button>
    </div>
  );
}
export function Spinner() {
  return <LoaderCircle size={16} className="spin" />;
}
export const date = (v: string) =>
  new Date(v).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
export function useInterval(callback: () => void, ms: number) {
  useEffect(() => {
    const id = setInterval(callback, ms);
    return () => clearInterval(id);
  }, [callback, ms]);
}
export async function base64(file: File) {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let value = "";
  for (let i = 0; i < bytes.length; i += 8192)
    value += String.fromCharCode(...bytes.subarray(i, i + 8192));
  return btoa(value);
}
