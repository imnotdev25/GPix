import { useState } from "react";
import { Link } from "react-router-dom";
import * as api from "../api";
import type { Me } from "../types";

export default function Login({ me, onSignedIn }: { me: Me; onSignedIn: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.post("/api/v1/auth/login", { username, password });
      onSignedIn();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login-wrap">
      <div className="card login-card">
        <div style={{ textAlign: "center", marginBottom: 18 }}>
          <img className="login-logo" src="/gpix.svg" alt="" />
          <h1 style={{ fontSize: 22, margin: 0 }}>gpix</h1>
          <p className="sub">Sign in to your library</p>
        </div>

        {me.passwordLogin && (
          <form onSubmit={submit}>
            <label className="field">
              <span>Username</span>
              <input
                className="input"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoFocus
              />
            </label>
            <label className="field">
              <span>Password</span>
              <input
                className="input"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            {error && <div className="banner banner-err">{error}</div>}
            <button className="btn btn-primary" style={{ width: "100%" }} disabled={busy}>
              {busy ? "Signing in…" : "Sign in"}
            </button>
          </form>
        )}

        {me.logtoEnabled && (
          <>
            {me.passwordLogin && (
              <div className="muted" style={{ textAlign: "center", margin: "14px 0" }}>
                or
              </div>
            )}
            <a className="btn" style={{ display: "block", textAlign: "center" }} href="/auth/logto/login">
              Continue with single sign-on
            </a>
          </>
        )}

        {!me.passwordLogin && !me.logtoEnabled && (
          <p className="muted">No sign-in method is enabled on this server.</p>
        )}

        <p className="login-foot">
          <Link to="/">What is gpix?</Link>
        </p>
      </div>
    </div>
  );
}
