import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import * as api from "../api";
import type { Account } from "../types";
import Icon from "../components/Icon";
import { errMsg, useConfirm, useToast } from "../components/feedback";

export default function Accounts() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [label, setLabel] = useState("");
  const [authData, setAuthData] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<number | null>(null);
  const [editLabel, setEditLabel] = useState("");
  const ask = useConfirm();
  const toast = useToast();

  const load = async () => {
    try {
      const res = await api.get<{ accounts: Account[] | null }>("/api/v1/accounts");
      setAccounts(res.accounts ?? []);
      setError("");
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const switchTo = async (a: Account) => {
    try {
      await api.post(`/api/v1/accounts/${a.id}/switch`);
      // The whole library changes with the account; reload to clear caches.
      window.location.assign("/library");
    } catch (err) {
      toast("Couldn't switch: " + errMsg(err), "err");
    }
  };

  const link = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.post("/api/v1/accounts", { label: label.trim(), authData: authData.trim() });
      setLabel("");
      setAuthData("");
      toast("Account linked", "ok");
      load();
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const rename = async (a: Account) => {
    const next = editLabel.trim();
    if (!next || next === a.label) {
      setEditing(null);
      return;
    }
    try {
      await api.post(`/api/v1/accounts/${a.id}`, { label: next });
      setEditing(null);
      load();
    } catch (err) {
      toast("Rename failed: " + errMsg(err), "err");
    }
  };

  const remove = async (a: Account) => {
    const ok = await ask({
      title: `Unlink "${a.label}"?`,
      message: "gpix forgets this account's sign-in. Its photos stay in Google Photos.",
      confirmLabel: "Unlink",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/api/v1/accounts/${a.id}`);
      toast("Account unlinked", "ok");
      load();
    } catch (err) {
      toast("Unlink failed: " + errMsg(err), "err");
    }
  };

  return (
    <>
      <div className="page-head">
        <h1>Accounts</h1>
        <p className="sub">
          Link several Google Photos accounts and switch between them. The active account backs the
          whole app: library, uploads, sharing, S3 and WebDAV all follow it.
        </p>
      </div>

      {error && <div className="banner banner-err">{error}</div>}

      <div className="card">
        <h2>Linked accounts</h2>
        {loading ? (
          <div className="center" style={{ minHeight: 70 }}>
            <div className="spinner" />
          </div>
        ) : (
          <div className="account-list">
            {accounts.map((a) => (
              <div className="account-row" key={a.id}>
                {editing === a.id ? (
                  <form
                    className="row nowrap account-edit"
                    onSubmit={(e) => {
                      e.preventDefault();
                      rename(a);
                    }}
                  >
                    <input
                      className="input"
                      value={editLabel}
                      onChange={(e) => setEditLabel(e.target.value)}
                      maxLength={60}
                      autoFocus
                      aria-label="Account name"
                    />
                    <button className="btn btn-primary btn-sm">Save</button>
                    <button className="btn btn-sm" type="button" onClick={() => setEditing(null)}>
                      Cancel
                    </button>
                  </form>
                ) : (
                  <>
                    <div className="account-name">
                      <strong>{a.label}</strong>
                      {a.active && <span className="pill on">Active</span>}
                    </div>
                    <div className="spacer" />
                    {!a.active && (
                      <button className="btn btn-primary btn-sm" onClick={() => switchTo(a)}>
                        Switch to this
                      </button>
                    )}
                    <button
                      className="btn btn-ghost btn-icon btn-sm"
                      onClick={() => {
                        setEditing(a.id);
                        setEditLabel(a.label);
                      }}
                      aria-label={`Rename ${a.label}`}
                      title="Rename"
                    >
                      <Icon name="edit" size={15} />
                    </button>
                    {!a.active && (
                      <button
                        className="btn btn-ghost btn-icon btn-sm danger"
                        onClick={() => remove(a)}
                        aria-label={`Unlink ${a.label}`}
                        title="Unlink"
                      >
                        <Icon name="trash" size={15} />
                      </button>
                    )}
                  </>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="card">
        <h2>Link another account</h2>
        <form onSubmit={link}>
          <label className="field">
            <span>Name</span>
            <input
              className="input"
              placeholder="e.g. Personal, Work"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              maxLength={60}
            />
          </label>
          <label className="field">
            <span>GP_AUTH_DATA</span>
            <input
              className="input"
              type="password"
              autoComplete="off"
              value={authData}
              onChange={(e) => setAuthData(e.target.value)}
              required
            />
          </label>
          <button className="btn btn-primary" disabled={busy || !authData.trim()}>
            {busy ? "Linking…" : "Link account"}
          </button>
        </form>
        <p className="muted" style={{ marginBottom: 0 }}>
          Credentials live in <code>accounts.db</code> (mode 0600); treat it like a password file.
        </p>
      </div>
    </>
  );
}
