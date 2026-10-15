type Timer = ReturnType<typeof setTimeout>;

export type TaskReadState<T> = {
  selected: string;
  result: T | null;
  error: unknown;
  reading: boolean;
};
type TaskReadEvent<T> =
  | { type: "select"; task: string }
  | { type: "received"; task: T }
  | { type: "failed"; error: unknown }
  | { type: "reading"; reading: boolean };

export function taskReadReducer<T>(
  state: TaskReadState<T>,
  event: TaskReadEvent<T>,
): TaskReadState<T> {
  switch (event.type) {
    case "select":
      // Restarting the same GET does not make its last detail current again.
      // Keep the failure visible until a successful read or another selection.
      if (state.selected === event.task) return state;
      return {
        selected: event.task,
        result: null,
        error: null,
        reading: false,
      };
    case "received":
      return { ...state, result: event.task, error: null };
    case "failed":
      return { ...state, error: event.error };
    case "reading":
      return { ...state, reading: event.reading };
  }
}

// One selected task, one pending GET or timer. Cleanup closes this generation;
// late responses cannot replace the next selection or schedule another poll.
export function watchTask<T extends { id: string; status: string }>({
  task,
  read,
  onTask,
  onError,
  onReading,
  schedule = setTimeout,
  cancel = clearTimeout,
}: {
  task: string;
  read: (path: string) => Promise<T>;
  onTask: (task: T) => void;
  onError: (error: unknown) => void;
  onReading: (reading: boolean) => void;
  schedule?: (run: () => void, delay: number) => Timer;
  cancel?: (timer: Timer) => void;
}) {
  let alive = true;
  let timer: Timer | undefined;
  const later = (delay: number) => {
    timer = schedule(() => {
      timer = undefined;
      void poll();
    }, delay);
  };
  const poll = async () => {
    if (!alive) return;
    onReading(true);
    try {
      const value = await read("/tasks/" + encodeURIComponent(task));
      if (!alive) return;
      if (!value || value.id !== task || typeof value.status !== "string")
        throw new Error("task detail identity or status is invalid");
      onTask(value);
      if (!["completed", "failed", "cancelled"].includes(value.status))
        later(2500);
    } catch (error) {
      if (!alive) return;
      onError(error);
      later(6000);
    } finally {
      if (alive) onReading(false);
    }
  };
  void poll();
  return () => {
    alive = false;
    if (timer !== undefined) cancel(timer);
  };
}
