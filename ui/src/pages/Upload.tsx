import { useState } from "react";
import * as api from "../api";

export default function Upload() {
  const [files, setFiles] = useState<File[]>([]);
  const [quality, setQuality] = useState("original");
  const [busy, setBusy] = useState(false);
  const [pct, setPct] = useState(0);
  const [results, setResults] = useState<{ name: string; status: string }[]>([]);
  const [error, setError] = useState("");

  const send = async () => {
    if (!files.length) return;
    setBusy(true);
    setPct(0);
    setError("");
    setResults([]);
    try {
      const res = await api.uploadFiles(files, quality, setPct);
      setResults(res.results ?? []);
      setFiles([]);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="page-head">
        <span className="eyebrow">Add media</span>
        <h1>Upload</h1>
        <p className="sub">
          Files are uploaded through gpix — encryption and disguising apply if enabled, and
          duplicates are skipped.
        </p>
      </div>

      <div className="card">
        <label className="field">
          <span>Files</span>
          <input
            className="input"
            type="file"
            multiple
            disabled={busy}
            onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
          />
        </label>
        <label className="field">
          <span>Quality</span>
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

        {files.length > 0 && (
          <p className="muted">
            {files.length} file{files.length > 1 ? "s" : ""} ready ·{" "}
            {(files.reduce((n, f) => n + f.size, 0) / 1048576).toFixed(1)} MB
          </p>
        )}

        {busy && (
          <div style={{ margin: "12px 0" }}>
            <div className="progress">
              <span style={{ width: `${pct}%` }} />
            </div>
            <p className="muted">Uploading… {pct}%</p>
          </div>
        )}

        {error && <div className="banner banner-err">{error}</div>}

        <button className="btn btn-primary" onClick={send} disabled={busy || !files.length}>
          {busy ? "Uploading…" : "Upload"}
        </button>
      </div>

      {results.length > 0 && (
        <div className="card">
          <h2>Results</h2>
          <table>
            <tbody>
              {results.map((r, i) => (
                <tr key={i}>
                  <td>{r.name}</td>
                  <td className="muted">{r.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
