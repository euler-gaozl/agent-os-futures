import assert from "node:assert/strict";
import React from "react";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Answer, sourceDocument } from "../src/markdown";
import { type SourceProof } from "../src/report";
import {
  sourceDetails,
  uniqueSources,
  loadSourceQuote,
  type Evidence,
  type Part,
} from "../src/evidence";

const record: Evidence = {
  operation_id: "run/read",
  run_id: "run",
  document_id: "document-1",
  title: "库存资料",
  source: "upload:document.md",
  kind: "evidence",
  preview:
    "来源：https://source.invalid/archive\n发布日期：2026-09-29\n统计范围：9月25日，主要油厂",
  preview_truncated: false,
  source_truncated: false,
  read_offset: 0,
  read_end: 400,
  has_more: false,
  recorded_at: "2026-10-13T00:00:00Z",
};

test("cross-receipt quotes reopen each pinned Unicode range and refuse gaps or changed receipts", async () => {
  const hash = "a".repeat(64),
    texts = ["前🙂中国2026年", "8月大豆进口增加。后记"],
    quote = "中国2026年8月大豆进口增加。";
  const firstEnd = 1200 + Array.from(texts[0]).length;
  const proof: SourceProof = {
    operation_id: "run/read/1",
    version: "b".repeat(64),
    quote_offset: 1202,
    quote,
    original_hash: hash,
    segments: [
      {
        operation_id: "run/read/1",
        version: "b".repeat(64),
        read_offset: 1200,
        read_end: firstEnd,
        quote_offset: 1202,
        quote_end: firstEnd,
      },
      {
        operation_id: "run/read/2",
        version: "c".repeat(64),
        read_offset: firstEnd,
        read_end: firstEnd + Array.from(texts[1]).length,
        quote_offset: firstEnd,
        quote_end: 1202 + Array.from(quote).length,
      },
    ],
  };
  const seen: string[] = [];
  const read = async (path: string): Promise<Part> => {
    seen.push(path);
    const url = new URL(path, "https://unused.invalid"),
      i = url.searchParams.get("operation_id") === "run/read/1" ? 0 : 1,
      s = proof.segments![i];
    assert.equal(url.searchParams.get("version"), s.version);
    assert.equal(url.searchParams.get("offset"), "0");
    return {
      ...record,
      operation_id: s.operation_id,
      original_hash: hash,
      version: s.version,
      read_offset: s.read_offset,
      read_end: s.read_end,
      text: texts[i],
      offset: 0,
      next_offset: null,
    };
  };
  const loaded = await loadSourceQuote(
    "/tasks/run/sources",
    record.document_id,
    proof,
    read,
  );
  assert.equal(seen.length, 2);
  assert.equal(loaded.quote_parts?.length, 2);
  assert.deepEqual(
    loaded.quote_parts?.map((p) => p.text),
    texts,
  );
  // Keep the first receipt's own metadata; do not invent one merged receipt.
  assert.equal(loaded.read_end, firstEnd);
  for (const mode of [
    "hash",
    "missing_hash",
    "version",
    "document",
    "content",
    "read_range",
  ]) {
    await assert.rejects(
      loadSourceQuote(
        "/tasks/run/sources",
        record.document_id,
        proof,
        async (path) => {
          const p = await read(path);
          if (p.operation_id !== "run/read/2") return p;
          if (mode === "hash") p.original_hash = "d".repeat(64);
          if (mode === "missing_hash") delete p.original_hash;
          if (mode === "version") p.version = "d".repeat(64);
          if (mode === "document") p.document_id = "foreign";
          if (mode === "content") p.text = p.text.replace("增加", "减少");
          if (mode === "read_range") p.read_offset--;
          return p;
        },
      ),
      mode,
    );
  }
  for (const mode of [
    "gap",
    "overlap",
    "duplicate",
    "length",
    "missing_hash",
    "too_many",
    "first_identity",
  ]) {
    const bad = structuredClone(proof),
      segments = bad.segments!;
    if (mode === "gap") segments[1].read_offset++;
    if (mode === "overlap") segments[1].read_offset--;
    if (mode === "duplicate")
      segments[1].operation_id = segments[0].operation_id;
    if (mode === "length") segments[1].quote_end--;
    if (mode === "missing_hash") delete bad.original_hash;
    if (mode === "too_many") bad.segments = Array(9).fill(segments[0]);
    if (mode === "first_identity") bad.operation_id = "other";
    await assert.rejects(
      loadSourceQuote(
        "/tasks/run/sources",
        record.document_id,
        bad,
        async () => {
          assert.fail("invalid proof must fail before reading");
        },
      ),
      mode,
    );
  }
});

test("fact reader pins the actual receipt/version and locates a Unicode quote beyond page one", async () => {
  const quote = "😀九月甲样本库存100万吨。",
    version = "a".repeat(64),
    text = Array.from("前".repeat(8190) + quote + "后记");
  const proof = {
    operation_id: "run/second-read",
    version,
    quote_offset: 8290,
    quote,
  };
  const seen: string[] = [];
  const read = async (path: string): Promise<Part> => {
    const url = new URL(path, "https://unused.invalid"),
      offset = Number(url.searchParams.get("offset"));
    seen.push(path);
    assert.equal(url.searchParams.get("operation_id"), proof.operation_id);
    assert.equal(url.searchParams.get("version"), version);
    const end = Math.min(offset + 8192, text.length);
    return {
      ...record,
      operation_id: proof.operation_id,
      text: text.slice(offset, end).join(""),
      offset,
      next_offset: end < text.length ? end : null,
      version,
      read_offset: 100,
      read_end: 100 + text.length,
    };
  };
  const result = await loadSourceQuote(
    "/tasks/run/sources",
    record.document_id,
    proof,
    read,
  );
  assert.equal(seen.length, 2);
  assert.match(seen[1], /offset=8190$/);
  assert.ok(result.text.startsWith(quote));
  assert.equal(result.operation_id, proof.operation_id);
  for (const mode of ["version", "document", "quote", "range"]) {
    await assert.rejects(
      loadSourceQuote(
        "/tasks/run/sources",
        record.document_id,
        proof,
        async (path) => {
          const part = await read(path);
          if (mode === "version") part.version = "b".repeat(64);
          if (mode === "document") part.document_id = "other";
          if (mode === "quote") part.text = part.text.replace("九月", "十月");
          if (mode === "range") part.read_end = 400;
          return part;
        },
      ),
    );
  }
});

test("archived document references become controlled buttons, not external navigation", () => {
  const html = renderToStaticMarkup(
    <Answer onSource={() => {}}>
      {
        "[库存来源](source://document/document-1) [外部来源](https://source.invalid/) [伪造](source://document/../../other)"
      }
    </Answer>,
  );
  assert.match(html, /<button[^>]*class="source-citation"/);
  assert.match(html, /库存来源/);
  assert.doesNotMatch(html, /href="source:|\.\.\/|onclick=/);
  assert.match(html, /href="https:\/\/source.invalid\/"/);
  for (const url of [
    "source://document/../x",
    "source://document/x?token=y",
    "source://document/x#y",
    "source://other/x",
    "SOURCE://document/x",
    "source://document/" + "x".repeat(121),
  ])
    assert.equal(sourceDocument(url), null);
  assert.equal(sourceDocument("source://document/document-1"), "document-1");
});
test("source labels come from retained first-page material; executable and credential URLs are rejected", () => {
  assert.deepEqual(sourceDetails(record), {
    url: "https://source.invalid/archive",
    published: "2026-09-29",
    period: "9月25日，主要油厂",
  });
  assert.deepEqual(sourceDetails({ ...record, read_offset: 400 }), {
    url: "",
    published: "",
    period: "",
  });
  for (const source of [
    "javascript:alert(1)",
    "file:///private",
    "https://user:secret@source.invalid/",
    "data:text/html,test",
  ])
    assert.equal(sourceDetails({ ...record, preview: "", source }).url, "");
  assert.equal(
    sourceDetails({ ...record, preview: "", source: "https://source.invalid/" })
      .url,
    "https://source.invalid/",
  );
});
test("listing deduplicates documents without losing the first-page provenance when it arrives later", () => {
  const later = {
    ...record,
    operation_id: "run/read2",
    read_offset: 400,
    preview: "后半段",
  };
  const second = {
    ...record,
    document_id: "document-2",
    operation_id: "run/read3",
  };
  assert.deepEqual(uniqueSources([later, second, record]), [record, second]);
  assert.deepEqual(uniqueSources([record, later]), [record]);
});
test("truncated metadata is never promoted to a complete source URL or date", () => {
  const preview = ("来源：https://source.invalid/" + "a".repeat(600)).slice(
    0,
    512,
  );
  assert.equal(
    sourceDetails({ ...record, preview, preview_truncated: true }).url,
    "",
  );
  assert.equal(
    sourceDetails({
      ...record,
      preview,
      preview_truncated: true,
      source: "https://source.invalid/complete",
    }).url,
    "https://source.invalid/complete",
  );
  assert.equal(
    sourceDetails({
      ...record,
      preview: "",
      source: "https://source.invalid/" + "a".repeat(2025),
      source_truncated: true,
    }).url,
    "",
  );
  const boundary = {
    ...record,
    preview: "来源：https://source.invalid/complete\n发布日期：2026-09",
    preview_truncated: true,
  };
  assert.equal(sourceDetails(boundary).url, "https://source.invalid/complete");
  assert.equal(sourceDetails(boundary).published, "");
  assert.equal(
    sourceDetails({ ...boundary, preview: boundary.preview + "\n" }).published,
    "2026-09",
  );
  // The original document_read may itself have stopped mid-line before the
  // API preview limit was reached. Its partial final line is not provenance.
  const short = {
    ...record,
    preview: preview.slice(0, 300),
    has_more: true,
    preview_truncated: false,
  };
  assert.equal(sourceDetails(short).url, "");
  assert.equal(
    sourceDetails({ ...short, source: "https://source.invalid/complete" }).url,
    "https://source.invalid/complete",
  );
  assert.equal(
    sourceDetails({ ...boundary, has_more: true, preview_truncated: false })
      .published,
    "",
  );
});
