import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import {
  TaskFailureNotice,
  TaskReadFailure,
  taskFailureMessage,
} from "../src/task-status";
import {
  taskReadReducer,
  TaskReadState,
  watchTask,
} from "../src/task-polling";

type Task = { id: string; status: string; result?: string; error?: string };
type Timer = ReturnType<typeof setTimeout>;
function pending<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function clock() {
  let sequence = 0;
  const timers = new Map<Timer, { run: () => void; delay: number }>();
  return {
    timers,
    schedule(run: () => void, delay: number) {
      const id = ++sequence as unknown as Timer;
      timers.set(id, { run, delay });
      return id;
    },
    cancel(id: Timer) {
      timers.delete(id);
    },
    fire() {
      assert.equal(timers.size, 1);
      const [id, timer] = [...timers][0];
      timers.delete(id);
      timer.run();
    },
  };
}
const settle = async () => {
  await Promise.resolve();
  await Promise.resolve();
};

test("same-task reread retains the stale warning until a successful GET", async () => {
  const timer = clock();
  const reads = [pending<Task>(), pending<Task>(), pending<Task>()];
  let state: TaskReadState<Task> = {
    selected: "",
    result: null,
    error: null,
    reading: false,
  };
  let index = 0;
  const start = () => {
    state = taskReadReducer(state, { type: "select", task: "same" });
    return watchTask({
      task: "same",
      read: () => reads[index++].promise,
      onTask: (task) => {
        state = taskReadReducer(state, { type: "received", task });
      },
      onError: (error) => {
        state = taskReadReducer(state, { type: "failed", error });
      },
      onReading: (reading) => {
        state = taskReadReducer(state, { type: "reading", reading });
      },
      schedule: timer.schedule,
      cancel: timer.cancel,
    });
  };
  const stopFirst = start();
  const retained = { id: "same", status: "running", result: "saved" };
  reads[0].resolve(retained);
  await settle();
  timer.fire();
  const failure = Object.assign(new Error("unavailable"), { status: 503 });
  reads[1].reject(failure);
  await settle();
  assert.equal(state.error, failure);
  stopFirst();
  const stopRetry = start();
  assert.equal(state.result, retained);
  assert.equal(state.error, failure);
  assert.equal(state.reading, true);
  assert.equal(timer.timers.size, 0);
  const html = renderToStaticMarkup(
    <TaskReadFailure
      error={state.error}
      hasTask={Boolean(state.result)}
      reading={state.reading}
      retry={() => {}}
    />,
  );
  assert.match(html, /上次读取的任务内容/);
  assert.match(html, /正在重新读取/);
  assert.match(html, /disabled=""/);
  reads[2].resolve({ id: "same", status: "completed", result: "final" });
  await settle();
  assert.equal(state.error, null);
  assert.equal(state.result?.result, "final");
  assert.equal(state.reading, false);
  stopRetry();
});

test("changing selection clears the previous task and its read failure", () => {
  const state: TaskReadState<Task> = {
    selected: "old",
    result: { id: "old", status: "running", result: "saved" },
    error: new Error("old read failed"),
    reading: true,
  };
  for (const task of ["new", ""]) {
    assert.deepEqual(taskReadReducer(state, { type: "select", task }), {
      selected: task,
      result: null,
      error: null,
      reading: false,
    });
  }
});

test("model 503 is a stopped task, not a promised provider retry or GET failure", () => {
  const html = renderToStaticMarkup(
    <TaskFailureNotice error="model HTTP 503" />,
  );
  assert.match(html, /模型服务暂时不可用/);
  assert.match(html, /这次研究已停止/);
  assert.match(html, /不会自动重新提交/);
  assert.match(html, /手动新建任务/);
  assert.doesNotMatch(
    html,
    /繁忙|免费|已经保存|已有草稿|暂时无法读取任务|<button/,
  );
  assert.equal(taskFailureMessage("upstream HTTP 503"), "upstream HTTP 503");
  assert.match(
    taskFailureMessage("model token budget exhausted"),
    /剩余预算不足/,
  );
});

test("detail read failure exposes a read-only retry and labels preserved content", () => {
  for (const hasTask of [false, true]) {
    const html = renderToStaticMarkup(
      <TaskReadFailure
        error={{ status: 503 }}
        hasTask={hasTask}
        reading={false}
        retry={() => {}}
      />,
    );
    assert.match(html, /role="alert"/);
    assert.match(html, /暂时无法读取任务/);
    assert.match(html, /最新执行状态尚未确认/);
    assert.match(html, /重新读取只查看原任务/);
    assert.match(html, /不会重新执行研究或创建新任务/);
    assert.equal(html.includes("上次读取的任务内容"), hasTask);
    assert.doesNotMatch(html, /模型服务暂时不可用|这次研究已停止|研究已完成/);
  }
  const busy = renderToStaticMarkup(
    <TaskReadFailure
      error={new Error("raw private upstream detail")}
      hasTask
      reading
      retry={() => {}}
    />,
  );
  assert.match(busy, /disabled=""/);
  assert.match(busy, /正在重新读取/);
  assert.doesNotMatch(busy, /raw private upstream detail/);
});

test("actual polling keeps last task on GET failure, then performs one delayed read", async () => {
  const timer = clock(),
    calls: unknown[][] = [],
    values: Task[] = [],
    errors: unknown[] = [];
  const reads = [pending<Task>(), pending<Task>(), pending<Task>()];
  let index = 0;
  const stop = watchTask({
    task: "synthetic/one",
    read: (...args: [string]) => {
      calls.push(args);
      return reads[index++].promise;
    },
    onTask: (task) => values.push(task),
    onError: (error) => errors.push(error),
    onReading: () => {},
    schedule: timer.schedule,
    cancel: timer.cancel,
  });
  const last = {
    id: "synthetic/one",
    status: "running",
    result: "saved content",
  };
  reads[0].resolve(last);
  await settle();
  assert.equal([...timer.timers.values()][0].delay, 2500);
  timer.fire();
  assert.equal(timer.timers.size, 0);
  reads[1].reject(Object.assign(new Error("unavailable"), { status: 503 }));
  await settle();
  assert.deepEqual(values, [last]);
  assert.equal(last.status, "running");
  assert.equal(errors.length, 1);
  assert.equal([...timer.timers.values()][0].delay, 6000);
  timer.fire();
  reads[2].resolve({
    id: "synthetic/one",
    status: "failed",
    error: "model HTTP 503",
  });
  await settle();
  assert.equal(values.at(-1)?.status, "failed");
  assert.equal(timer.timers.size, 0);
  // The production reader supplies only a path to api(), so it is a GET.
  assert.deepEqual(
    calls,
    Array.from({ length: 3 }, () => ["/tasks/synthetic%2Fone"]),
  );
  stop();
});

test("selection change or unmount closes late success and error responses", async () => {
  for (const outcome of ["success", "error"]) {
    const timer = clock(),
      old = pending<Task>(),
      current = pending<Task>();
    const values: Task[] = [],
      errors: unknown[] = [],
      loading: boolean[] = [],
      paths: string[] = [];
    const common = {
      onTask: (t: Task) => values.push(t),
      onError: (e: unknown) => errors.push(e),
      onReading: (v: boolean) => loading.push(v),
      schedule: timer.schedule,
      cancel: timer.cancel,
    };
    const stopOld = watchTask({
      ...common,
      task: "old",
      read: (p) => {
        paths.push(p);
        return old.promise;
      },
    });
    stopOld();
    const stopCurrent = watchTask({
      ...common,
      task: "current",
      read: (p) => {
        paths.push(p);
        return current.promise;
      },
    });
    current.resolve({ id: "current", status: "completed" });
    await settle();
    const before = {
      values: [...values],
      errors: [...errors],
      loading: [...loading],
    };
    if (outcome === "success") old.resolve({ id: "old", status: "running" });
    else old.reject(new Error("old rejected"));
    await settle();
    assert.deepEqual({ values, errors, loading }, before);
    assert.deepEqual(paths, ["/tasks/old", "/tasks/current"]);
    assert.equal(timer.timers.size, 0);
    stopCurrent();
  }
});

test("manual read restart cancels the old timer without modifying retained task", async () => {
  const timer = clock(),
    first = pending<Task>(),
    retry = pending<Task>(),
    values: Task[] = [];
  const readPaths: string[] = [];
  const common = {
    task: "same",
    onTask: (t: Task) => values.push(t),
    onError: () => {},
    onReading: () => {},
    schedule: timer.schedule,
    cancel: timer.cancel,
  };
  const stopFirst = watchTask({
    ...common,
    read: (p) => {
      readPaths.push(p);
      return first.promise;
    },
  });
  first.resolve({ id: "same", status: "running", result: "retained" });
  await settle();
  assert.equal(timer.timers.size, 1);
  // This is the cleanup/start pair used by the effect's manual-read counter.
  stopFirst();
  assert.equal(timer.timers.size, 0);
  const stopRetry = watchTask({
    ...common,
    read: (p) => {
      readPaths.push(p);
      return retry.promise;
    },
  });
  assert.deepEqual(values, [
    { id: "same", status: "running", result: "retained" },
  ]);
  retry.resolve({ id: "same", status: "completed", result: "final" });
  await settle();
  assert.deepEqual(readPaths, ["/tasks/same", "/tasks/same"]);
  assert.equal(timer.timers.size, 0);
  stopRetry();
});

test("malformed or mismatched detail cannot replace a previously displayed task", async () => {
  for (const invalid of [
    null,
    { id: "another", status: "completed" },
    { id: "same" },
  ]) {
    const timer = clock(),
      errors: unknown[] = [];
    const retained = { id: "same", status: "running", result: "retained" };
    let displayed: Task = retained;
    const stop = watchTask({
      task: "same",
      read: async () => invalid as Task,
      onTask: (value) => {
        displayed = value;
      },
      onError: (error) => errors.push(error),
      onReading: () => {},
      schedule: timer.schedule,
      cancel: timer.cancel,
    });
    await settle();
    assert.equal(displayed, retained);
    assert.equal(errors.length, 1);
    assert.equal([...timer.timers.values()][0].delay, 6000);
    stop();
    assert.equal(timer.timers.size, 0);
  }
});
