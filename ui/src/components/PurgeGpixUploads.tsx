import { useState } from "react";
import * as api from "../api";
import type { GpixUploadSummary } from "../types";

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`;
}

function plural(n: number, word: string): string {
  return `${n.toLocaleString()} ${word}${n === 1 ? "" : "s"}`;
}

/** Danger-zone card: review, then trash, everything gpix itself uploaded. */
export default function PurgeGpixUploads() {
  const [summary, setSummary] = useState<GpixUploadSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [typed, setTyped] = useState("");
  const [error, setError] = useState("");
  const [result, setResult] = useState("");

  const review = async () => {
    setError("");
    setResult("");
    setTyped("");
    setLoading(true);
    try {
      setSummary(await api.get<GpixUploadSummary>("/api/v1/gpix-uploads/summary"));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  const close = () => {
    setSummary(null);
    setTyped("");
  };

  const doDelete = async () => {
    if (!summary || typed !== "DELETE") return;
    setError("");
    setDeleting(true);
    try {
      const res = await api.post<{ total: number; deleted: number; failed: number }>(
        "/api/v1/gpix-uploads/delete",
        { confirm: "DELETE", accountId: summary.accountId, expected: summary.total },
      );
      setResult(
        `Moved ${plural(res.deleted, "item")} to the Google Photos trash` +
          (res.failed ? `; ${res.failed.toLocaleString()} failed.` : "."),
      );
      setSummary(null);
      setTyped("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      // The library changed or the account switched: show the current numbers.
      if (err instanceof api.ApiError && err.status === 409) {
        try {
          setSummary(await api.get<GpixUploadSummary>("/api/v1/gpix-uploads/summary"));
          setTyped("");
        } catch {
          /* keep the original error */
        }
      }
    } finally {
      setDeleting(false);
    }
  };

  const since = summary?.trackingSince ? new Date(summary.trackingSince).toLocaleDateString() : "";
  const breakdown = summary
    ? [
        [summary.photos, "Photos"],
        [summary.videos, "Videos"],
        [summary.docs, "Documents"],
        [summary.files, "Other files"],
      ].filter(([n]) => (n as number) > 0)
    : [];

  return (
    <div className="card" style={{ borderColor: "var(--danger)" }}>
      <h2 style={{ color: "var(--danger)" }}>Delete gpix uploads</h2>
      <p className="muted">
        Move only what gpix uploaded to the Google Photos trash. Photos from your phone and other
        apps are left alone.
      </p>

      {error && <div className="banner banner-err">{error}</div>}
      {result && <div className="banner banner-ok">{result}</div>}

      {!summary && (
        <button className="btn btn-danger" onClick={review} disabled={loading}>
          {loading ? "Counting…" : "Review gpix uploads…"}
        </button>
      )}

      {summary && summary.total === 0 && (
        <>
          <p>
            Nothing to delete. No gpix uploads were found in <strong>{summary.accountLabel}</strong>.
          </p>
          <button className="btn" onClick={close}>
            Close
          </button>
        </>
      )}

      {summary && summary.total > 0 && (
        <>
          <p>
            <strong>{plural(summary.total, "item")}</strong> ({formatBytes(summary.bytes)}) in{" "}
            <strong>{summary.accountLabel}</strong> will move to the trash:
          </p>
          <table>
            <tbody>
              {breakdown.map(([n, label]) => (
                <tr key={label as string}>
                  <td>{label}</td>
                  <td style={{ textAlign: "right" }}>{(n as number).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="muted">
            {summary.tracked.toLocaleString()} from gpix's upload log,{" "}
            {summary.disguised.toLocaleString()} recognized as files gpix wrapped or encrypted.
            {summary.sample.length > 0 && (
              <>
                {" "}
                Including: {summary.sample.join(", ")}
                {summary.total > summary.sample.length ? ", …" : ""}
              </>
            )}
          </p>
          {summary.trackingEnabled && since && (
            <p className="muted">
              Plain photos and videos gpix uploaded before {since} weren't logged, so they can't be
              told apart from your own and won't be deleted.
            </p>
          )}
          <p className="muted">Items stay recoverable in the Google Photos trash for 60 days.</p>
          <label className="field">
            <span>
              Type <code>DELETE</code> to confirm
            </span>
            <input
              className="input"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <div className="row">
            <button
              className="btn btn-danger"
              onClick={doDelete}
              disabled={typed !== "DELETE" || deleting}
            >
              {deleting ? "Deleting…" : `Move ${plural(summary.total, "item")} to trash`}
            </button>
            <button className="btn" onClick={close} disabled={deleting}>
              Cancel
            </button>
          </div>
        </>
      )}
    </div>
  );
}
