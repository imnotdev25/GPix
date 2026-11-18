import { useEffect, useState } from "react";
import * as api from "../api";
import type { ApiToken } from "../types";
import { errMsg, useConfirm, useToast } from "../components/feedback";

export default function Tokens() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [label, setLabel] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const ask = useConfirm();
  const toast = useToast();

  const load = async () => {
    setLoading(true);
    try {
      const res = await api.get<{ tokens: ApiToken[] | null }>("/api/v1/tokens");
      setTokens(res.tokens ?? []);
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

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const res = await api.post<{ secret: string }>("/api/v1/tokens", { label });
      setSecret(res.secret);
      setLabel("");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const revoke = async (t: ApiToken) => {
    const ok = await ask({
      title: `Revoke "${t.label}"?`,
      message: "Any app using this token stops working immediately.",
      confirmLabel: "Revoke token",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/api/v1/tokens/${t.id}`);
      toast("Token revoked", "ok");
      load();
    } catch (err) {
      toast("Revoke failed: " + errMsg(err), "err");
    }
  };

  return (
    <>
      <div className="page-head">
        <h1>App tokens</h1>
        <p className="sub">
          Tokens let the gpix mobile/desktop app (or any API client) sign in. A token grants full
          access to your library — treat it like a password.
        </p>
      </div>

      {secret && (
        <div className="card" style={{ borderColor: "var(--ok)" }}>
          <h2>New token — copy it now</h2>
          <p className="muted" style={{ marginTop: 0 }}>
            This is the only time the full token is shown.
          </p>
          <div className="row">
            <code style={{ flex: 1, wordBreak: "break-all" }}>{secret}</code>
            <button className="btn btn-primary" onClick={() => navigator.clipboard?.writeText(secret)}>
              Copy
            </button>
            <button className="btn" onClick={() => setSecret("")}>
              Done
            </button>
          </div>
        </div>
      )}

      {error && <div className="banner banner-err">{error}</div>}

      <div className="card">
        <form className="row" onSubmit={create}>
          <input
            className="input"
            style={{ flex: 1, minWidth: 200 }}
            placeholder="Label (e.g. My phone)"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
          <button className="btn btn-primary">Generate token</button>
        </form>
      </div>

      <div className="card">
        <h2>Your tokens</h2>
        {loading ? (
          <div className="center" style={{ minHeight: 70 }}>
            <div className="spinner" />
          </div>
        ) : tokens.length === 0 ? (
          <p className="muted">No tokens yet.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Label</th>
                <th>Token</th>
                <th>Last used</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {tokens.map((t) => (
                <tr key={t.id}>
                  <td>{t.label}</td>
                  <td>
                    <code>{t.preview}</code>
                  </td>
                  <td className="muted">
                    {t.lastUsed && !t.lastUsed.startsWith("0001")
                      ? new Date(t.lastUsed).toLocaleDateString()
                      : "never"}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-danger" onClick={() => revoke(t)}>
                      Revoke
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
