// The records page is a read-only work list: it must show the bounded scope,
// distinguish confirmed from unconfirmed causes, and re-query only when the
// submitted filters change.
import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { JSDOM } from "jsdom";

function page(items: any[], hasMore = false) {
  return {
    protocol: "app-run-failures-v1",
    app: "futures",
    items,
    next_cursor: hasMore ? "next-cursor" : "",
    has_more: hasMore,
    coverage: {
      since: "2026-07-20T00:00:00Z",
      until: "2026-10-20T00:00:00Z",
      scanned: 3,
      scan_limit: 200,
      truncated: false,
      complete: !hasMore,
    },
  };
}

const items = [
  {
    run_id: "dd80cadf8a249e7821ee217cbf6da81b",
    status: "failed",
    created_at: "2026-10-20T22:06:57+08:00",
    commodity: "豆粕",
    period: "2026-07-20 至 2026-10-20",
    stage: "tool",
    code: "parameter_rejected",
    cause_confirmed: true,
    operation_id: "dd80cadf8a249e7821ee217cbf6da81b/dsh/14",
    tool: "kb_search",
    failure_version: "fvd1:" + "a".repeat(64),
  },
  {
    run_id: "cafebabecafebabecafebabecafebabe",
    status: "waiting_unknown",
    created_at: "2026-10-19T10:00:00+08:00",
    commodity: "甲醇",
    stage: "unknown",
    code: "unknown",
    cause_confirmed: false,
    failure_version: "",
  },
];

test("the error records page shows scope, classes and one row per failure", async () => {
  const dom = new JSDOM("<!doctype html><html><body><div id='app'></div></body></html>", {
    url: "http://127.0.0.1:8098/",
  });
  const win = dom.window as unknown as Window & typeof globalThis;
  for (const key of [
    "window", "document", "navigator", "HTMLElement", "Element", "Node",
    "Event", "MouseEvent", "KeyboardEvent", "getComputedStyle",
    "sessionStorage", "localStorage", "Blob", "URL",
  ]) {
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: (win as any)[key] });
  }
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;

  const requested: string[] = [];
  let response = page(items, true);
  globalThis.fetch = ((input: RequestInfo | URL) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    requested.push(url);
    return Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(response),
    } as Response);
  }) as typeof fetch;

  const { FailureRecords } = await import("../src/failure-records");
  const { createRoot } = await import("react-dom/client");
  const { act } = await import("react");
  const container = document.getElementById("app")!;
  const root = createRoot(container);
  const settle = async (rounds = 4) => {
    for (let i = 0; i < rounds; i++)
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 15));
      });
  };
  const opened: string[] = [];
  try {
    await act(async () => root.render(<FailureRecords onOpen={(id) => opened.push(id)} />));
    await settle(6);
    assert.equal(container.querySelectorAll(".failure-card").length, 2);
    const text = container.textContent || "";
    assert.match(text, /参数校验未通过/);
    assert.match(text, /原因尚未确认|终止原因未确认/);
    assert.match(text, /已扫描 3 条候选（上限 200）/);
    assert.match(text, /本页 2 条/);
    assert.match(text, /不会重新执行研究/);
    assert.equal(container.querySelectorAll(".failure-card .badge").length, 2);
    // Open the first failure without creating anything.
    const openButton = [...container.querySelectorAll("button")].find((b) =>
      (b.textContent || "").includes("查看任务与详情"),
    )!;
    openButton.dispatchEvent(new win.MouseEvent("click", { bubbles: true }));
    assert.deepEqual(opened, [items[0].run_id]);
    assert.equal(requested.filter((u) => u.startsWith("/api/task-failures")).length, 1);

    // Applying a category filter issues exactly one new read and keeps it in
    // the query, never filtering the already loaded page in the browser.
    const category = [...container.querySelectorAll("select")].find(
      (s) => s.closest("label")?.textContent?.includes("分类"),
    ) as HTMLSelectElement;
    await act(async () => {
      category.value = "parameter_rejected";
      category.dispatchEvent(new win.Event("change", { bubbles: true }));
    });
    response = page([items[0]], false);
    await act(async () => {
      container.querySelector("form")!.dispatchEvent(
        new win.Event("submit", { bubbles: true, cancelable: true }),
      );
    });
    await settle(6);
    const list = requested.filter((u) => u.startsWith("/api/task-failures"));
    assert.equal(list.length, 2);
    assert.match(list[1], /category=parameter_rejected/);
    assert.equal(container.querySelectorAll(".failure-card").length, 1);

    // Empty results have an explicit scope, not a "no failures" claim.
    response = page([], false);
    await act(async () => {
      container.querySelector("form")!.dispatchEvent(
        new win.Event("submit", { bubbles: true, cancelable: true }),
      );
    });
    await settle(6);
    assert.match(container.textContent || "", /当前筛选没有错误记录/);
    assert.match(container.textContent || "", /不代表没有发生过失败/);
  } finally {
    await act(async () => root.unmount());
    dom.window.close();
  }
});
