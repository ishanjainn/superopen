export type DiffSpan = { text: string; changed: boolean };

export type DiffLine = {
  kind: "ctx" | "add" | "del" | "chg-del" | "chg-add";
  text: string;
  oldNo?: number;
  newNo?: number;
  spans?: DiffSpan[];
};

export type DiffHunk = {
  header: string;
  lines: DiffLine[];
};

const HUNK = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

export function diffStats(diff: string): { plus: number; minus: number } {
  let plus = 0;
  let minus = 0;
  for (const line of diff.replace(/\r\n/g, "\n").split("\n")) {
    if (line.startsWith("+++") || line.startsWith("---")) continue;
    if (line.startsWith("+")) plus += 1;
    else if (line.startsWith("-")) minus += 1;
  }
  return { plus, minus };
}

export function parseUnifiedDiff(diff: string): DiffHunk[] {
  const hunks: DiffHunk[] = [];
  let cur: DiffHunk | null = null;
  let oldNo = 0;
  let newNo = 0;
  for (const raw of diff.replace(/\r\n/g, "\n").split("\n")) {
    const m = raw.match(HUNK);
    if (m) {
      if (cur) hunks.push(pairChanges(cur));
      cur = { header: raw, lines: [] };
      oldNo = Number(m[1]);
      newNo = Number(m[2]);
      continue;
    }
    if (!cur) continue;
    if (raw.startsWith("\\")) continue;
    if (raw.startsWith("+") && !raw.startsWith("+++")) {
      cur.lines.push({ kind: "add", text: raw.slice(1), newNo: newNo++ });
    } else if (raw.startsWith("-") && !raw.startsWith("---")) {
      cur.lines.push({ kind: "del", text: raw.slice(1), oldNo: oldNo++ });
    } else if (raw.startsWith(" ") || raw === "") {
      const text = raw.startsWith(" ") ? raw.slice(1) : raw;
      cur.lines.push({
        kind: "ctx",
        text,
        oldNo: oldNo++,
        newNo: newNo++,
      });
    }
  }
  if (cur) hunks.push(pairChanges(cur));
  return hunks;
}

function pairChanges(hunk: DiffHunk): DiffHunk {
  const lines: DiffLine[] = [];
  for (let i = 0; i < hunk.lines.length; i++) {
    const a = hunk.lines[i];
    const b = hunk.lines[i + 1];
    if (a.kind === "del" && b?.kind === "add") {
      const [as, bs] = wordSpans(a.text, b.text);
      lines.push({ ...a, kind: "chg-del", spans: as });
      lines.push({ ...b, kind: "chg-add", spans: bs });
      i += 1;
    } else {
      lines.push(a);
    }
  }
  return { ...hunk, lines };
}

function wordSpans(a: string, b: string): [DiffSpan[], DiffSpan[]] {
  let i = 0;
  while (i < a.length && i < b.length && a[i] === b[i]) i += 1;
  let j = 0;
  while (
    j < a.length - i &&
    j < b.length - i &&
    a[a.length - 1 - j] === b[b.length - 1 - j]
  ) {
    j += 1;
  }
  return [spans(a, i, j), spans(b, i, j)];
}

function spans(s: string, prefix: number, suffix: number): DiffSpan[] {
  const midEnd = s.length - suffix;
  const out: DiffSpan[] = [];
  if (prefix > 0) out.push({ text: s.slice(0, prefix), changed: false });
  if (midEnd > prefix) out.push({ text: s.slice(prefix, midEnd), changed: true });
  if (suffix > 0) out.push({ text: s.slice(midEnd), changed: false });
  return out;
}
