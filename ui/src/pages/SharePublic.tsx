import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import * as api from "../api";
import Icon from "../components/Icon";
import { errMsg } from "../components/feedback";

type Item = { name: string; isVideo: boolean; thumb: string; raw: string };
type Info = {
  state: "ok" | "password" | "expired" | "missing";
  title?: string;
  allowOriginal: boolean;
  download?: string;
  items: Item[];
};

/** The page someone opens from a share link (/s/:token). No sign-in needed. */
export default function SharePublic({ token }: { token: string }) {
  const [info, setInfo] = useState<Info | null>(null);
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const [pwError, setPwError] = useState("");
  const [busy, setBusy] = useState(false);
  const base = `/s/${encodeURIComponent(token)}`;

  useEffect(() => {
    api
      .get<Info>(`${base}/info`)
      .then(setInfo)
      .catch((err) => setError(errMsg(err)));
  }, [base]);

  useEffect(() => {
    document.title = info?.title ? `${info.title} · gpix` : "Shared · gpix";
  }, [info]);

  const unlock = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setPwError("");
    try {
      setInfo(await api.post<Info>(`${base}/unlock`, { password }));
    } catch (err) {
      setPwError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const page = (body: ReactNode) => (
    <div className="share-page">
      <header className="share-top">
        <img className="brand-logo" src="/gpix.svg" alt="" />
        <strong>gpix</strong>
      </header>
      <main className="share-main">{body}</main>
      <footer className="share-bottom muted small">Shared with gpix</footer>
    </div>
  );

  if (error) {
    return page(
      <div className="share-card">
        <h1>Couldn't open this link</h1>
        <p className="muted">{error}</p>
      </div>,
    );
  }
  if (!info) {
    return page(
      <div className="center" style={{ minHeight: 240 }}>
        <div className="spinner" />
      </div>,
    );
  }
  if (info.state === "missing" || info.state === "expired") {
    return page(
      <div className="share-card">
        <Icon name="x" size={28} />
        <h1>Link unavailable</h1>
        <p className="muted">
          {info.state === "missing"
            ? "This link doesn't exist."
            : "This link has expired or reached its download limit."}
        </p>
      </div>,
    );
  }
  if (info.state === "password") {
    return page(
      <form className="share-card" onSubmit={unlock}>
        <Icon name="lock" size={28} />
        <h1>Password required</h1>
        <p className="muted">The person who shared this protected it with a password.</p>
        <input
          className="input"
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoFocus
          required
        />
        {pwError && <div className="banner banner-err">{pwError}</div>}
        <button className="btn btn-primary wide" disabled={busy}>
          {busy ? "Checking…" : "View"}
        </button>
      </form>,
    );
  }

  const n = info.items.length;
  return page(
    <>
      <div className="share-head">
        <div>
          <h1>{info.title || "Shared media"}</h1>
          <p className="muted num">{n === 1 ? "1 item" : `${n} items`}</p>
        </div>
        {info.download && (
          <a className="btn btn-primary" href={info.download}>
            <Icon name="download" size={16} />
            {n === 1 ? "Download" : `Download all (${n}) as .zip`}
          </a>
        )}
      </div>
      <div className="share-grid">
        {info.items.map((it, i) => (
          <div className="share-tile" key={i}>
            <a className="share-thumb" href={`${it.raw}?inline=1`} target="_blank" rel="noopener" title={`Open ${it.name}`}>
              <img src={`${it.thumb}?size=512`} alt={it.name} loading="lazy" />
              {it.isVideo && (
                <span className="share-play">
                  <Icon name="play" size={28} />
                </span>
              )}
            </a>
            <div className="share-row">
              <span className="share-name" title={it.name}>
                {it.name}
              </span>
              {info.allowOriginal && (
                <a
                  className="btn btn-ghost btn-icon btn-sm"
                  href={it.raw}
                  download={it.name}
                  aria-label={`Download ${it.name}`}
                  title="Download"
                >
                  <Icon name="download" size={16} />
                </a>
              )}
            </div>
          </div>
        ))}
      </div>
    </>,
  );
}
