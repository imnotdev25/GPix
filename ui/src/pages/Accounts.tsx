import { useEffect, useState } from "react";
import * as api from "../api";
import type { Account } from "../types";

export default function Accounts() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [label, setLabel] = useState("");
  const [authData, setAuthData] = useState("");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
      const res = await api.get<{ accounts: Account[] | null }>("/api/v1/accounts");
      setAccounts(res.accounts ?? []);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const switchTo = async (a: Account) => {
    setError("");
    try {
      await api.post(`/api/v1/accounts/${a.id}/switch`);
      setMsg(`Switched to ${a.label}.`);
      // The whole library changes with the account — reload to clear caches.
      window.location.assign("/library");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const link = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setMsg("");
    try {
      // The link form posts to the settings endpoint, which the SPA shares with
      // the legacy page; it accepts form-encoded values.
      const body = new URLSearchParams({ label, auth_data: authData });
      const res = await fetch("/settings/accounts/add", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body,
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setLabel("");
      setAuthData("");
      setMsg("Account linked.");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <>
      <div className="page-head">
        <span className="eyebrow">Settings</span>
        <h1>Accounts</h1>
        <p className="sub">
          Link several Google Photos accounts and switch between them. The active account backs the
          whole app — library, uploads, sharing, S3 and WebDAV all follow it.
        </p>
      </div>

      {msg && <div className="banner banner-ok">{msg}</div>}
      {error && <div className="banner banner-err">{error}</div>}

      <div className="card">
        <h2>Linked accounts</h2>
        {loading ? (
          <div className="center" style={{ minHeight: 70 }}>
            <div className="spinner" />
          </div>
        ) : (
          <table>
            <tbody>
              {accounts.map((a) => (
                <tr key={a.id}>
                  <td>
                    <strong>{a.label}</strong> {a.active && <span className="badge">● active</span>}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    {!a.active && (
                      <button className="btn btn-primary" onClick={() => switchTo(a)}>
                        Switch to this
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h2>Link another account</h2>
        <form onSubmit={link}>
          <label className="field">
            <span>Label</span>
            <input
              className="input"
              placeholder="e.g. Personal, Work"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          </label>
          <label className="field">
            <span>GP_AUTH_DATA</span>
            <input
              className="input"
              type="password"
              value={authData}
              onChange={(e) => setAuthData(e.target.value)}
              required
            />
          </label>
          <button className="btn btn-primary">Link account</button>
        </form>
        <p className="muted" style={{ marginBottom: 0 }}>
          Credentials live in <code>accounts.db</code> (mode 0600) — treat it like a password file.
        </p>
      </div>
    </>
  );
}
