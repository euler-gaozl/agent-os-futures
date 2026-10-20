// Sibling sections of one task detail must never share a React key. The
// evidence binding section and the retained-work section are mounted next to
// each other, so each derives its own stable, role-prefixed identity. The
// evidence manifest participates because a new binding changes what the
// section may display; the task status participates so a terminal transition
// resets retained-draft state instead of reusing the running mount.
export type TaskKeyIdentity = {
  id: string;
  status?: string;
  result?: string;
  evidence_binding?: { manifest_sha256?: string };
};

export function taskSectionKeys(task: TaskKeyIdentity) {
  const binding = task.evidence_binding?.manifest_sha256 || "unbound";
  return {
    evidence: "evidence:" + task.id + ":" + binding,
    retained:
      "retained:" +
      task.id +
      ":" +
      (task.status || "unknown") +
      (task.result ? ":final" : ":none"),
    report: "report:" + task.id + ":" + binding,
  };
}
