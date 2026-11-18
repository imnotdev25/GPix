import { useEffect, useState } from "react";
import * as api from "../api";
import type { Gateways } from "../types";
import PurgeGpixUploads from "../components/PurgeGpixUploads";
import S3Card from "../components/S3Card";
import { errMsg, useConfirm, useToast } from "../components/feedback";

export default function Connections() {
  const [g, setG] = useState<Gateways | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const ask = useConfirm();
  const toast = useToast();

  const load = () =>
    api
      .get<Gateways>("/api/v1/gateways")
      .then(setG)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));

  useEffect(() => {
    load();
  }, []);

  const act = async (path: string, body: unknown, ok: string) => {
    setError("");
    try {
      const res = await api.post<Gateways>(path, body);
      if (res && typeof res === "object" && "S3Endpoint" in res) setG(res);
      else load();
      setNotice(ok);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const toggleEncryption = async () => {
    if (!g) return;
    await act("/api/v1/encryption/toggle", { enable: !g.EncryptionEnabled }, "Encryption updated.");
    load();
  };

  const deleteAll = async () => {
    const ok = await ask({
      title: "Delete your entire library?",
      message:
        "Every item in the active account, including photos from your phone and other apps, moves to the Google Photos trash. It stays recoverable there for 60 days.",
      confirmLabel: "Delete everything",
      danger: true,
      typeToConfirm: "DELETE",
    });
    if (!ok) return;
    try {
      const res = await api.post<{ total: number; failed: number }>("/api/v1/delete-all", {
        confirm: "DELETE",
      });
      toast(
        `Moved ${res.total - res.failed} item(s) to trash${res.failed ? `; ${res.failed} failed` : ""}.`,
        res.failed ? "err" : "ok",
      );
    } catch (err) {
      toast("Delete failed: " + errMsg(err), "err");
    }
  };

  if (!g) {
    return (
      <div className="center">
        <div className="spinner" />
      </div>
    );
  }

  return (
    <>
      <div className="page-head">
        <h1>Connections</h1>
        <p className="sub">
          Expose your library over S3-compatible and WebDAV endpoints, and control media
          encryption.
        </p>
      </div>

      {notice && <div className="banner banner-ok">{notice}</div>}
      {error && <div className="banner banner-err">{error}</div>}

      {g.EncryptionAvailable && (
        <div className="card">
          <div className="row">
            <h2 style={{ margin: 0 }}>Media encryption</h2>
            <div className="spacer" />
            <span className={g.EncryptionEnabled ? "badge" : "muted"}>
              {g.EncryptionEnabled ? "● encrypting new uploads" : "○ off"}
            </span>
          </div>
          <p className="muted">
            When on, each upload is encrypted with a key only you hold, then disguised as a video.
            Google only ever stores an opaque clip.
          </p>
          <p className="muted">
            Key fingerprint: <code>{g.EncryptionFingerprint}</code> · {g.EncryptionKeyCount} key
            {g.EncryptionKeyCount === 1 ? "" : "s"} loaded
          </p>
          <div className="row">
            <button className="btn btn-primary" onClick={toggleEncryption}>
              {g.EncryptionEnabled ? "Turn encryption off" : "Turn encryption on"}
            </button>
            <a className="btn" href="/settings/gateways/encryption-key">
              Download key backup
            </a>
            <a className="btn" href="/settings/gateways">
              Manage decryption keys
            </a>
          </div>
        </div>
      )}

      <S3Card g={g} act={act} />

      <div className="card">
        <div className="row">
          <h2 style={{ margin: 0 }}>WebDAV</h2>
          <div className="spacer" />
          <span className="muted">{g.WebDAVEnabled ? g.WebDAVEndpoint : "disabled"}</span>
        </div>
        <label className="field">
          <span>Username</span>
          <input className="input" readOnly value={g.WebDAVUsername} />
        </label>
        {g.HasWebDAVPass && (
          <label className="field">
            <span>App password</span>
            <input className="input" readOnly value={g.WebDAVPassword} />
          </label>
        )}
        <div className="row">
          <button
            className="btn btn-primary"
            onClick={() =>
              act("/api/v1/gateways/regenerate", { kind: "webdav" }, "New WebDAV password — copy it now.")
            }
          >
            {g.HasWebDAVPass ? "Regenerate" : "Generate"} app password
          </button>
          {g.HasWebDAVPass && (
            <button
              className="btn"
              onClick={() => act("/api/v1/gateways/clear", { kind: "webdav" }, "WebDAV password cleared.")}
            >
              Clear
            </button>
          )}
        </div>
      </div>

      <PurgeGpixUploads />

      <div className="card" style={{ borderColor: "var(--danger)" }}>
        <h2 style={{ color: "var(--danger)" }}>Danger zone</h2>
        <p className="muted">
          Move <strong>every</strong> item in your library to the Google Photos trash. Recoverable
          there for 60 days — but there is no undo from here.
        </p>
        <button className="btn btn-danger" onClick={deleteAll}>
          Delete all photos…
        </button>
      </div>
    </>
  );
}
