import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import * as api from "../api";
import type { Share } from "../types";
import Icon from "../components/Icon";
import { errMsg, useConfirm, useToast } from "../components/feedback";
import { parseDate } from "../format";

const shortDate = (d: Date) =>
  d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });

function expiry(s: Share): string {
  const d = parseDate(s.expiresAt);
  if (!d) return "Never";
  if (d.getTime() < Date.now()) return "Expired";
  return shortDate(d);
}

export default function Shares() {
  const [shares, setShares] = useState<Share[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const ask = useConfirm();
  const toast = useToast();

  const load = async () => {
    setLoading(true);
    try {
      const res = await api.get<{ shares: Share[] | null }>("/api/v1/shares");
      setShares(res.shares ?? []);
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

  const revoke = async (s: Share) => {
    const ok = await ask({
      title: `Revoke "${s.title || "untitled link"}"?`,
      message: "Anyone with the link loses access immediately.",
      confirmLabel: "Revoke link",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/api/v1/shares/${encodeURIComponent(s.token)}`);
      setShares((list) => list.filter((x) => x.token !== s.token));
      toast("Link revoked", "ok");
    } catch (err) {
      toast("Revoke failed: " + errMsg(err), "err");
    }
  };

  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      toast("Link copied", "ok");
    } catch {
      toast("Couldn't copy the link", "err");
    }
  };

  return (
    <>
      <div className="page-head row">
        <div>
          <h1>Shares</h1>
          <p className="sub">Links you've created. Make new ones from the Library or the viewer.</p>
        </div>
        <div className="spacer" />
        <button className="btn btn-icon" onClick={load} disabled={loading} aria-label="Refresh" title="Refresh">
          <Icon name="refresh" size={16} />
        </button>
      </div>

      {error && <div className="banner banner-err">{error}</div>}

      {loading && !shares.length ? (
        <div className="center" style={{ minHeight: 120 }}>
          <div className="spinner" />
        </div>
      ) : shares.length === 0 ? (
        <div className="empty">
          <Icon name="share" size={40} />
          <h2>No share links yet</h2>
          <p className="muted">Select photos in the Library, then choose Share.</p>
          <Link className="btn btn-primary" to="/library">
            <Icon name="photos" size={16} />
            Go to Library
          </Link>
        </div>
      ) : (
        <div className="card table-wrap">
          <table>
            <thead>
              <tr>
                <th>Link</th>
                <th>Items</th>
                <th>Expires</th>
                <th>Downloads</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {shares.map((s) => {
                const created = parseDate(s.createdAt);
                const exp = expiry(s);
                return (
                  <tr key={s.token}>
                    <td>
                      <div className="cell-title">
                        {s.hasPassword && (
                          <span title="Password protected">
                            <Icon name="lock" size={13} />
                          </span>
                        )}
                        <strong>{s.title || "Untitled"}</strong>
                      </div>
                      {created && <div className="muted">Created {shortDate(created)}</div>}
                    </td>
                    <td>{s.count}</td>
                    <td className={exp === "Expired" ? "text-danger" : ""}>{exp}</td>
                    <td>
                      {s.downloads}
                      {s.maxDownloads > 0 ? ` / ${s.maxDownloads}` : ""}
                    </td>
                    <td>
                      <div className="row nowrap" style={{ justifyContent: "flex-end" }}>
                        <button className="btn btn-sm" onClick={() => copy(s.url)}>
                          <Icon name="copy" size={14} />
                          Copy
                        </button>
                        <a
                          className="btn btn-sm btn-icon"
                          href={s.url}
                          target="_blank"
                          rel="noreferrer"
                          aria-label="Open link"
                          title="Open link"
                        >
                          <Icon name="external" size={14} />
                        </a>
                        <button
                          className="btn btn-sm btn-icon danger"
                          onClick={() => revoke(s)}
                          aria-label="Revoke link"
                          title="Revoke link"
                        >
                          <Icon name="trash" size={14} />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
