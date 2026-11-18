import { useState } from "react";
import type { FormEvent } from "react";
import * as api from "../api";
import Icon from "./Icon";
import { Modal, errMsg, useToast } from "./feedback";

const expiries = [
  { label: "Never", hours: 0 },
  { label: "1 hour", hours: 1 },
  { label: "1 day", hours: 24 },
  { label: "7 days", hours: 168 },
  { label: "30 days", hours: 720 },
];

/** Create a public link for one or more items, then show it with Copy/Open. */
export default function ShareDialog({
  keys,
  defaultTitle = "",
  onClose,
  onCreated,
}: {
  keys: string[];
  defaultTitle?: string;
  onClose: () => void;
  onCreated?: () => void;
}) {
  const [title, setTitle] = useState(defaultTitle);
  const [password, setPassword] = useState("");
  const [hours, setHours] = useState(0);
  const [maxDownloads, setMaxDownloads] = useState("");
  const [allowOriginal, setAllowOriginal] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [url, setUrl] = useState("");
  const toast = useToast();
  const n = keys.length;

  const copy = async (link: string, afterCreate = false) => {
    try {
      await navigator.clipboard.writeText(link);
      toast(afterCreate ? "Link created and copied" : "Link copied", "ok");
    } catch {
      toast(
        afterCreate ? "Link created" : "Couldn't copy; select the link and copy it manually",
        afterCreate ? "ok" : "err",
      );
    }
  };

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const share = await api.post<{ url: string }>("/api/v1/shares", {
        keys,
        title: title.trim(),
        password,
        expiresHours: hours,
        maxDownloads: parseInt(maxDownloads, 10) || 0,
        allowOriginal,
      });
      setUrl(share.url);
      onCreated?.();
      copy(share.url, true);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  if (url) {
    return (
      <Modal
        title="Share link ready"
        onClose={onClose}
        footer={
          <button className="btn btn-primary" onClick={onClose}>
            Done
          </button>
        }
      >
        <p className="modal-text">
          Anyone with this link can view {n === 1 ? "this item" : `these ${n} items`}
          {password ? " after entering the password" : ""}.
        </p>
        <div className="link-box">
          <code title={url}>{url}</code>
          <button className="btn btn-sm" onClick={() => copy(url)}>
            <Icon name="copy" size={15} />
            Copy
          </button>
          <a
            className="btn btn-sm btn-ghost btn-icon"
            href={url}
            target="_blank"
            rel="noreferrer"
            aria-label="Open link"
            title="Open link"
          >
            <Icon name="external" size={15} />
          </a>
        </div>
        <p className="muted">You can revoke it any time under Shares.</p>
      </Modal>
    );
  }

  return (
    <Modal
      title={n === 1 ? "Share 1 item" : `Share ${n} items`}
      onClose={onClose}
      footer={
        <>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" type="submit" form="share-form" disabled={busy}>
            <Icon name="share" size={16} />
            {busy ? "Creating…" : "Create link"}
          </button>
        </>
      }
    >
      <form id="share-form" onSubmit={create}>
        <label className="field">
          <span>Title shown to recipients</span>
          <input
            className="input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Optional"
          />
        </label>
        <label className="field">
          <span>Password</span>
          <input
            className="input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="None"
            autoComplete="new-password"
          />
        </label>
        <div className="field">
          <span>Expires</span>
          <div className="segmented full" role="radiogroup" aria-label="Expires">
            {expiries.map((x) => (
              <button
                type="button"
                key={x.hours}
                role="radio"
                aria-checked={hours === x.hours}
                className={hours === x.hours ? "on" : ""}
                onClick={() => setHours(x.hours)}
              >
                {x.label}
              </button>
            ))}
          </div>
        </div>
        <label className="field">
          <span>Download limit</span>
          <input
            className="input"
            type="number"
            min={0}
            inputMode="numeric"
            value={maxDownloads}
            onChange={(e) => setMaxDownloads(e.target.value)}
            placeholder="Unlimited"
          />
        </label>
        <label className="check">
          <input
            type="checkbox"
            checked={allowOriginal}
            onChange={(e) => setAllowOriginal(e.target.checked)}
          />
          <span>Allow downloading the original files</span>
        </label>
        {error && <div className="banner banner-err">{error}</div>}
      </form>
    </Modal>
  );
}
