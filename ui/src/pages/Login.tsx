import { useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router-dom";
import * as api from "../api";
import type { Me } from "../types";
import { errMsg } from "../components/feedback";

type Mode = "signin" | "signup";

export default function Login({ me, onSignedIn }: { me: Me; onSignedIn: () => void }) {
  const setup = !!me.firstRun && !!me.signupOpen;
  const [mode, setMode] = useState<Mode>(setup ? "signup" : "signin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const signup = mode === "signup";

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    if (signup) {
      if (password.length < 8) {
        setError("Use at least 8 characters for the password.");
        return;
      }
      if (password !== confirm) {
        setError("The passwords don't match.");
        return;
      }
    }
    setBusy(true);
    try {
      if (signup) await api.post("/api/v1/auth/signup", { email: email.trim(), password });
      else await api.post("/api/v1/auth/login", { username: email.trim(), password });
      onSignedIn();
    } catch (err) {
      setError(errMsg(err));
      setBusy(false);
    }
  };

  const switchTo = (m: Mode) => {
    setMode(m);
    setError("");
    setConfirm("");
  };

  return (
    <div className="login-wrap">
      <div className="card login-card">
        <div style={{ textAlign: "center", marginBottom: 18 }}>
          <img className="login-logo" src="/gpix.svg" alt="" />
          <h1 style={{ fontSize: 22, margin: 0 }}>{setup ? "Set up gpix" : "gpix"}</h1>
          <p className="sub">
            {setup
              ? "Create the first account on this server."
              : signup
                ? "Create your account"
                : "Sign in to your library"}
          </p>
        </div>

        {me.passwordLogin && (
          <form onSubmit={submit}>
            <label className="field">
              <span>{signup ? "Email" : "Email or username"}</span>
              <input
                className="input"
                type={signup ? "email" : "text"}
                autoComplete={signup ? "email" : "username"}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoFocus
              />
            </label>
            <label className="field">
              <span>Password</span>
              <input
                className="input"
                type="password"
                autoComplete={signup ? "new-password" : "current-password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </label>
            {signup && (
              <label className="field">
                <span>Confirm password</span>
                <input
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  required
                />
              </label>
            )}
            {error && <div className="banner banner-err">{error}</div>}
            <button className="btn btn-primary" style={{ width: "100%" }} disabled={busy}>
              {busy ? (signup ? "Creating account…" : "Signing in…") : signup ? "Create account" : "Sign in"}
            </button>
            {setup && (
              <p className="muted small" style={{ marginBottom: 0 }}>
                Sign-up closes once the server reaches its account limit (GPIX_MAX_USERS).
              </p>
            )}
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

        {me.passwordLogin && me.signupOpen && !setup && (
          <p className="login-switch">
            {signup ? (
              <>
                Have an account?{" "}
                <button type="button" className="text-link" onClick={() => switchTo("signin")}>
                  Sign in
                </button>
              </>
            ) : (
              <>
                New here?{" "}
                <button type="button" className="text-link" onClick={() => switchTo("signup")}>
                  Create an account
                </button>
              </>
            )}
          </p>
        )}

        <p className="login-foot">
          <Link to="/">What is gpix?</Link>
        </p>
      </div>
    </div>
  );
}
