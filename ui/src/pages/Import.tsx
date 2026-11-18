import { useEffect, useState } from "react";
import * as api from "../api";
import type { Account, ImportProgress } from "../types";
import { errMsg } from "../components/feedback";

type Source = "account" | "paste";

export default function Import() {
  const [p, setP] = useState<ImportProgress | null>(null);
  const [server, setServer] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [workers, setWorkers] = useState(3);
  const [authData, setAuthData] = useState("");
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [source, setSource] = useState<Source>("paste");
  const [accountId, setAccountId] = useState<number | null>(null);
  const [error, setError] = useState("");

  const poll = () => api.get<ImportProgress>("/api/v1/import/status").then(setP).catch(() => {});

  useEffect(() => {
    poll();
    const t = setInterval(poll, 2000);
    api
      .get<{ accounts: Account[] | null }>("/api/v1/accounts")
      .then((r) => {
        // The active account is the destination, so it can't be the source.
        const others = (r.accounts ?? []).filter((a) => !a.active);
        setAccounts(others);
        if (others.length) {
          setSource("account");
          setAccountId(others[0].id);
        }
      })
      .catch(() => {});
    return () => clearInterval(t);
  }, []);

  const startImmich = async () => {
    setError("");
    try {
      await api.post("/api/v1/import/start", { serverUrl: server, apiKey, workers });
      poll();
    } catch (err) {
      setError(errMsg(err));
    }
  };

  const startGoogle = async () => {
    setError("");
    try {
      const body =
        source === "account" ? { accountId, workers } : { authData: authData.trim(), workers };
      await api.post("/api/v1/import/google-start", body);
      poll();
    } catch (err) {
      setError(errMsg(err));
    }
  };

  const stop = () => api.post("/api/v1/import/stop").then(poll).catch(() => {});

  const running = p?.running ?? false;
  const processed = p ? p.done + p.skipped + p.failed : 0;
  const pct = p && p.total > 0 ? Math.min(100, Math.round((processed / p.total) * 100)) : 0;
  const googleReady = source === "account" ? accountId !== null : authData.trim() !== "";

  const workersField = (
    <label className="field">
      <span>
        Parallel transfers: <strong className="num">{workers}</strong>
      </span>
      <input type="range" min={1} max={10} value={workers} onChange={(e) => setWorkers(Number(e.target.value))} />
    </label>
  );

  return (
    <>
      <div className="page-head">
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
          <div className="row muted num">
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
        <h2>From another Google account</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Items are re-uploaded into the active account. Pick one of your linked accounts, or paste
          the other account's <code>GP_AUTH_DATA</code>.
        </p>
        <div className="segmented" role="tablist" aria-label="Source account">
          <button
            type="button"
            role="tab"
            aria-selected={source === "account"}
            className={source === "account" ? "on" : ""}
            onClick={() => setSource("account")}
          >
            Linked account
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={source === "paste"}
            className={source === "paste" ? "on" : ""}
            onClick={() => setSource("paste")}
          >
            Paste GP_AUTH_DATA
          </button>
        </div>

        {source === "account" ? (
          accounts.length ? (
            <div className="pick-list" role="radiogroup" aria-label="Linked account">
              {accounts.map((a) => (
                <button
                  key={a.id}
                  type="button"
                  role="radio"
                  aria-checked={accountId === a.id}
                  className={"pick" + (accountId === a.id ? " on" : "")}
                  onClick={() => setAccountId(a.id)}
                >
                  <span className="pick-dot" />
                  <span className="pick-name">{a.label}</span>
                </button>
              ))}
            </div>
          ) : (
            <p className="muted">
              No other linked accounts. Add one on the Accounts page, or paste its GP_AUTH_DATA.
            </p>
          )
        ) : (
          <label className="field">
            <span>Source GP_AUTH_DATA</span>
            <input
              className="input"
              type="password"
              autoComplete="off"
              value={authData}
              onChange={(e) => setAuthData(e.target.value)}
            />
          </label>
        )}
        {workersField}
        <button className="btn btn-primary" onClick={startGoogle} disabled={running || !googleReady}>
          Start Google import
        </button>
      </div>

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
            autoComplete="off"
            placeholder="Account Settings → API Keys"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
          />
        </label>
        {workersField}
        <button className="btn btn-primary" onClick={startImmich} disabled={running || !server || !apiKey}>
          Start Immich import
        </button>
      </div>
    </>
  );
}
