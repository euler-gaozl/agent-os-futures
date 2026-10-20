// Real client render of the Futures task page. The previous defect was a
// sibling-key collision that appended one more evidence section on every
// six-second parent refresh. This test mounts the actual parent component,
// drives its refresh interval ten times and switches tasks, in a DOM.
import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { JSDOM } from "jsdom";

const tasks = [
  {
    id: "bad00000000000000000000000000001",
    status: "failed",
    commodity: "豆粕",
    period: "2026-07-20 至 2026-10-20",
    as_of: "2026-10-20",
    created_at: "2026-10-20T22:06:57+08:00",
    result: "",
    error: "service HTTP 400: {\"error\":\"upstream request rejected\"}",
  },
  {
    id: "good0000000000000000000000000002",
    status: "completed",
    commodity: "甲醇",
    period: "2026-08-01 至 2026-10-20",
    as_of: "2026-10-20",
    created_at: "2026-10-20T23:05:22+08:00",
    result: "# 甲醇研究记录\n\n本次没有取得可核验的报数。",
    error: "",
  },
];

function json(value: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(value),
  } as Response);
}

function fixtures(): Promise<Response>[] {
  const calls: Promise<Response>[] = [];
  globalThis.fetch = ((input: RequestInfo | URL) => {
    const url = String(input);
    const path = url.replace(/^https?:\/\/[^/]+/, "");
    let out: Promise<Response>;
    if (path === "/api/agents")
      out = json([
        {
          id: "published-supply-inventory",
          revision: 1,
          name: "供给与库存",
          description: "整理供给与库存资料",
        },
      ]);
    else if (path === "/api/tasks") out = json(tasks);
    else if (path === "/api/readiness")
      out = json({ knowledge_available: true, model_configured: true });
    else if (path === "/api/tasks/" + tasks[0].id) out = json(tasks[0]);
    else if (path === "/api/tasks/" + tasks[1].id) out = json(tasks[1]);
    else if (path.includes("/sources")) out = json({ items: [], next_cursor: "" });
    else if (path.includes("/drafts"))
      out = json({ items: [], next_cursor: "" });
    else if (path.includes("/report-check"))
      out = json({
        status: "needs_review",
        notice: "fixture",
        validator: "fixture-check-v1",
        result_sha256: "a".repeat(64),
        classification_date: "2026-10-20",
        facts: [],
        comparisons: [],
        issues: [],
      });
    else if (path.includes("/draft-match")) out = json({ status: "not_found" });
    else out = json({});
    calls.push(out);
    return out;
  }) as typeof fetch;
  return calls;
}

test("the real Futures parent keeps one evidence section across ten parent refreshes and a task switch", async () => {
  const dom = new JSDOM("<!doctype html><html><body><div id='app'></div></body></html>", {
    url: "http://127.0.0.1:8098/",
  });
  const win = dom.window as unknown as Window & typeof globalThis;
  for (const key of [
    "window",
    "document",
    "navigator",
    "HTMLElement",
    "Element",
    "Node",
    "Event",
    "MouseEvent",
    "KeyboardEvent",
    "getComputedStyle",
    "sessionStorage",
    "localStorage",
    "Blob",
    "URL",
  ]) {
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value: (win as any)[key],
    });
  }
  (globalThis as any).window = win;
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  fixtures();

  // Deterministic stand-in for the six-second parent refresh interval. The
  // production component still owns the effect and its cleanup.
  const intervals = new Map<number, () => void>();
  let nextId = 1;
  (globalThis as any).setInterval = ((fn: () => void) => {
    const id = nextId++;
    intervals.set(id, fn);
    return id as any;
  }) as typeof setInterval;
  (globalThis as any).clearInterval = ((id: number) => {
    intervals.delete(id);
  }) as typeof clearInterval;

  const errors: string[] = [];
  const originalError = console.error;
  console.error = (...args: unknown[]) => {
    errors.push(args.map((a) => String(a)).join(" "));
  };

  const { Futures } = await import("../src/futures.tsx");
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
  const clickButton = (predicate: (text: string) => boolean) => {
    const button = [...container.querySelectorAll("button")].find((b) =>
      predicate((b.textContent || "").trim()),
    );
    assert.ok(button, "expected button to exist");
    button!.dispatchEvent(
      new win.MouseEvent("click", { bubbles: true, cancelable: true }),
    );
  };
  try {
    await act(async () => root.render(<Futures />));
    await settle();
    assert.equal(intervals.size, 1, "the parent refresh interval is mounted");
    clickButton((t) => t.includes("豆粕"));
    await settle(8);
    const failedDetail = container.querySelector(".research-result header h1");
    assert.match(failedDetail?.textContent || "", /豆粕/);
    assert.equal(container.querySelectorAll(".task-outcome").length, 1);
    assert.equal(container.querySelectorAll(".task-actions").length, 1);

    clickButton((t) => t.includes("查看生成过程"));
    await settle(6);
    assert.equal(container.querySelectorAll(".generation-panel").length, 1);
    assert.equal(
      container.querySelectorAll(".draft-empty").length,
      1,
      "zero saved drafts stay one compact line",
    );

    for (let i = 0; i < 10; i++) {
      assert.equal(intervals.size, 1, "refresh-" + i + " keeps one interval");
      for (const run of [...intervals.values()]) run();
      await settle(3);
      assert.equal(
        container.querySelectorAll(".task-outcome").length,
        1,
        "refresh-" + i + " must not append a failure panel",
      );
      assert.equal(
        container.querySelectorAll(".generation-panel").length,
        1,
        "refresh-" + i + " must not append a generation panel",
      );
      assert.equal(
        container.querySelectorAll(".draft-empty").length,
        1,
        "refresh-" + i + " must not append a draft line",
      );
    }

    clickButton((t) => t.includes("甲醇"));
    await settle(8);
    assert.match(
      container.querySelector(".research-result header h1")?.textContent || "",
      /甲醇/,
    );
    assert.equal(container.querySelectorAll(".task-outcome").length, 0);
    assert.equal(container.querySelectorAll(".research-document").length, 1);
    assert.equal(container.querySelectorAll(".generation-panel").length, 0);
    assert.equal(
      container.querySelectorAll(".task-actions").length,
      1,
      "one action area per task",
    );
    assert.equal(
      errors.filter((e) => /same key|Encountered two children/i.test(e)).length,
      0,
      "no duplicate-key warning: " + errors.join(" | "),
    );
  } finally {
    console.error = originalError;
    await act(async () => root.unmount());
    dom.window.close();
  }
});
