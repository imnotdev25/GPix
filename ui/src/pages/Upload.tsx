import { useEffect, useRef, useState } from "react";
import type { DragEvent } from "react";
import * as api from "../api";
import Icon from "../components/Icon";
import type { IconName } from "../components/Icon";
import { errMsg, useToast } from "../components/feedback";
import { formatBytes } from "../format";

type State = "queued" | "uploading" | "done" | "skipped" | "failed";
type Entry = { id: number; file: File; state: State; pct: number; detail: string };

const stateLabel: Record<State, string> = {
  queued: "Waiting",
  uploading: "Uploading",
  done: "Uploaded",
  skipped: "Already in library",
  failed: "Failed",
};

// Server statuses: "uploaded", "uploaded (encrypted)", "uploaded (disguised)",
// "already in library", or an error message.
function classify(status: string): State {
  const s = status.toLowerCase();
  if (s.startsWith("already")) return "skipped";
  if (s.startsWith("uploaded")) return "done";
  return "failed";
}

function fileIcon(f: File): IconName {
  if (f.type.startsWith("image/")) return "photos";
  if (f.type.startsWith("video/")) return "video";
  return "file";
}

const CONCURRENCY = 2;
let seq = 0;

export default function Upload() {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [quality, setQuality] = useState("original");
  const [busy, setBusy] = useState(false);
  const [drag, setDrag] = useState(false);
  const input = useRef<HTMLInputElement | null>(null);
  const toast = useToast();

  // A file dropped outside the drop zone would otherwise open in the tab.
  useEffect(() => {
    const stop = (e: Event) => e.preventDefault();
    window.addEventListener("dragover", stop);
    window.addEventListener("drop", stop);
    return () => {
      window.removeEventListener("dragover", stop);
      window.removeEventListener("drop", stop);
    };
  }, []);

  const patch = (id: number, p: Partial<Entry>) =>
    setEntries((list) => list.map((e) => (e.id === id ? { ...e, ...p } : e)));

  const add = (files: FileList | null) => {
    const list = Array.from(files ?? []);
    if (!list.length) return;
    setEntries((prev) => [
      ...prev,
      ...list.map((file): Entry => ({ id: ++seq, file, state: "queued", pct: 0, detail: "" })),
    ]);
  };

  const start = async () => {
    const queue = entries.filter((e) => e.state === "queued" || e.state === "failed");
    if (!queue.length || busy) return;
    queue.forEach((e) => patch(e.id, { state: "queued", pct: 0, detail: "" }));
    setBusy(true);
    let next = 0;
    let ok = 0;
    let skipped = 0;
    let failed = 0;
    const worker = async () => {
      while (next < queue.length) {
        const e = queue[next++];
        patch(e.id, { state: "uploading", pct: 0 });
        try {
          const res = await api.uploadFiles([e.file], quality, (pct) => patch(e.id, { pct }));
          const status = res.results?.[0]?.status ?? "no response from server";
          const state = classify(status);
          if (state === "done") ok++;
          else if (state === "skipped") skipped++;
          else failed++;
          patch(e.id, {
            state,
            pct: 100,
            detail:
              state === "failed"
                ? status
                : state === "done" && status !== "uploaded"
                  ? status.slice("uploaded ".length)
                  : "",
          });
        } catch (err) {
          failed++;
          patch(e.id, { state: "failed", detail: errMsg(err) });
        }
      }
    };
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, queue.length) }, worker));
    setBusy(false);
    const parts = [`${ok} uploaded`];
    if (skipped) parts.push(`${skipped} already in library`);
    if (failed) parts.push(`${failed} failed`);
    toast(parts.join(" · "), failed ? "err" : "ok");
  };

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDrag(false);
    add(e.dataTransfer.files);
  };

  const pending = entries.filter((e) => e.state === "queued" || e.state === "failed").length;
  const finished = entries.filter((e) => e.state === "done" || e.state === "skipped").length;
  const totalBytes = entries.reduce((n, e) => n + e.file.size, 0);
  const overall = entries.length
    ? Math.round(
        entries.reduce(
          (n, e) => n + (e.state === "queued" ? 0 : e.state === "uploading" ? e.pct : 100),
          0,
        ) / entries.length,
      )
    : 0;

  return (
    <>
      <div className="page-head">
        <h1>Upload</h1>
        <p className="sub">
          Files go through gpix, so encryption and disguising apply if enabled, and duplicates are
          skipped.
        </p>
      </div>

      <input
        ref={input}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          add(e.target.files);
          e.target.value = "";
        }}
      />
      <div
        className={"dropzone" + (drag ? " drag" : "")}
        role="button"
        tabIndex={0}
        onClick={() => input.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            input.current?.click();
          }
        }}
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={onDrop}
      >
        <Icon name="upload" size={34} />
        <strong>Drop files here</strong>
        <span className="muted">or click to choose: photos, videos or any other file</span>
      </div>

      {entries.length > 0 && (
        <div className="card">
          <div className="row">
            <h2 style={{ margin: 0 }}>
              {entries.length} file{entries.length === 1 ? "" : "s"}
            </h2>
            <span className="muted">
              {formatBytes(totalBytes)}
              {finished ? ` · ${finished} done` : ""}
            </span>
            <div className="spacer" />
            <label className="inline-field">
              <span className="muted">Quality</span>
              <select
                className="input"
                value={quality}
                disabled={busy}
                onChange={(e) => setQuality(e.target.value)}
              >
                <option value="original">Original</option>
                <option value="saver">Storage saver</option>
                <option value="quota">Use quota</option>
              </select>
            </label>
          </div>

          {busy && (
            <div className="progress" style={{ margin: "14px 0 4px" }} aria-label={`${overall}% done`}>
              <span style={{ width: `${overall}%` }} />
            </div>
          )}

          <ul className="filelist">
            {entries.map((e) => (
              <li key={e.id} className={"file-row " + e.state}>
                <span className="file-icon">
                  <Icon name={fileIcon(e.file)} size={18} />
                </span>
                <div className="file-main">
                  <div className="file-name" title={e.file.name}>
                    {e.file.name}
                  </div>
                  <div className="file-sub">
                    {formatBytes(e.file.size)} ·{" "}
                    {e.state === "uploading"
                      ? e.pct >= 100
                        ? "Sending to Google Photos…"
                        : `Uploading ${e.pct}%`
                      : stateLabel[e.state]}
                    {e.detail ? ` · ${e.detail}` : ""}
                  </div>
                  {e.state === "uploading" && (
                    <div className="progress thin">
                      <span style={{ width: `${e.pct}%` }} />
                    </div>
                  )}
                </div>
                {(e.state === "done" || e.state === "skipped") && (
                  <span className="file-status ok" title={stateLabel[e.state]}>
                    <Icon name="check" size={16} />
                  </span>
                )}
                {e.state === "failed" && (
                  <span className="file-status err" title="Failed">
                    <Icon name="info" size={16} />
                  </span>
                )}
                {!busy && e.state !== "uploading" && (
                  <button
                    className="btn btn-ghost btn-icon btn-sm"
                    onClick={() => setEntries((l) => l.filter((x) => x.id !== e.id))}
                    aria-label={`Remove ${e.file.name}`}
                  >
                    <Icon name="x" size={14} />
                  </button>
                )}
              </li>
            ))}
          </ul>

          <div className="row">
            <button className="btn btn-primary" onClick={start} disabled={busy || !pending}>
              <Icon name="upload" size={16} />
              {busy ? "Uploading…" : `Upload ${pending || ""}`.trim()}
            </button>
            {!busy && finished > 0 && (
              <button
                className="btn"
                onClick={() =>
                  setEntries((l) => l.filter((e) => e.state !== "done" && e.state !== "skipped"))
                }
              >
                Clear finished
              </button>
            )}
          </div>
        </div>
      )}
    </>
  );
}
