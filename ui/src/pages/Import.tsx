import { useEffect, useState } from "react";
import * as api from "../api";
import type { ImportProgress } from "../types";

export default function Import() {
  const [p, setP] = useState<ImportProgress | null>(null);
  const [server, setServer] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [workers, setWorkers] = useState(3);
  const [authData, setAuthData] = useState("");
  const [error, setError] = useState("");

  const poll = () => api.get<ImportProgress>("/api/v1/import/status").then(setP).catch(() => {});

  useEffect(() => {
    poll();
    const t = setInterval(poll, 2000);
    return () => clearInterval(t);
  }, []);

  const startImmich = async () => {
    setError("");
    try {
      await api.post("/api/v1/import/start", { serverUrl: server, apiKey, workers });
      poll();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const startGoogle = async () => {
    setError("");
    try {
      await api.post("/api/v1/import/google-start", { authData, workers });
      poll();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const stop = () => api.post("/api/v1/import/stop").then(poll).catch(() => {});

  const running = p?.running ?? false;
  const processed = p ? p.done + p.skipped + p.failed : 0;
  const pct = p && p.total > 0 ? Math.min(100, Math.round((processed / p.total) * 100)) : 0;

  return (
    <>
      <div className="page-head">
        <span className="eyebrow">Migrate</span>
        <h1>Import</h1>
        <p className="sub">
          Copy a library into gpix from Immich or another Google account. Encryption and
          disguising apply if enabled; already-imported items are skipped.
        </p>
      </div>

      {error && <div className="banner banner-err">{error}</div>}

      {p && (running || processed > 0 || p.error) && (
        <div className="card">
          <div className="row">
            <h2 style={{ margin: 0 }}>{running ? "Running" : p.error ? "Error" : "Finished"}</h2>
            <div className="spacer" />
            {p.workers > 0 && <span className="muted">{p.workers} parallel</span>}
            {running && (
              <button className="btn" onClick={stop}>
                Stop
              </button>
            )}
          </div>
          <div className="progress" style={{ margin: "12px 0" }}>
            <span style={{ width: `${pct}%` }} />
          </div>
          <div className="row muted">
            <span>Done: {p.done}</span>
            <span>Skipped: {p.skipped}</span>
            <span>Failed: {p.failed}</span>
            <span>Total: {p.total || "?"}</span>
          </div>
          {running && p.current && (
            <p className="muted" style={{ marginBottom: 0 }}>
              Copying: {p.current}
            </p>
          )}
          {p.error && <div className="banner banner-err" style={{ marginTop: 10 }}>{p.error}</div>}
        </div>
      )}

      <div className="card">
        <h2>From Immich</h2>
        <label className="field">
          <span>Immich server URL</span>
          <input
            className="input"
            placeholder="https://immich.example.com"
            value={server}
            onChange={(e) => setServer(e.target.value)}
          />
        </label>
        <label className="field">
          <span>API key</span>
          <input
            className="input"
            type="password"
            placeholder="Account Settings → API Keys"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
          />
        </label>
        <label className="field">
          <span>Parallel transfers: {workers}</span>
          <input
            type="range"
            min={1}
            max={10}
            value={workers}
            onChange={(e) => setWorkers(Number(e.target.value))}
          />
        </label>
        <button className="btn btn-primary" onClick={startImmich} disabled={running}>
          Start Immich import
        </button>
      </div>

      <div className="card">
        <h2>From another Google account</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Paste that account's <code>GP_AUTH_DATA</code>. Items are re-uploaded into the active
          account.
        </p>
        <label className="field">
          <span>Source GP_AUTH_DATA</span>
          <input
            className="input"
            type="password"
            value={authData}
            onChange={(e) => setAuthData(e.target.value)}
          />
        </label>
        <button className="btn btn-primary" onClick={startGoogle} disabled={running}>
          Start Google import
        </button>
      </div>
    </>
  );
}
