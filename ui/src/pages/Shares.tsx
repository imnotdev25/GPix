import { useEffect, useState } from "react";
import * as api from "../api";
import type { Share } from "../types";

export default function Shares() {
  const [shares, setShares] = useState<Share[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = async () => {
    setLoading(true);
    try {
      const res = await api.get<{ shares: Share[] | null }>("/api/v1/shares");
      setShares(res.shares ?? []);
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

  const revoke = async (s: Share) => {
    if (!confirm(`Revoke this link? Recipients lose access immediately.`)) return;
    try {
      await api.del(`/api/v1/shares/${encodeURIComponent(s.token)}`);
      load();
    } catch (err) {
      alert("Revoke failed: " + (err instanceof Error ? err.message : err));
    }
  };

  return (
    <>
      <div className="page-head row">
        <div>
          <span className="eyebrow">Public links</span>
          <h1>Shares</h1>
          <p className="sub">Links you've created. Create new ones from the Library (Select → Share).</p>
        </div>
        <div className="spacer" />
        <button className="btn" onClick={load} disabled={loading}>
          Refresh
        </button>
      </div>

      {error && <div className="banner banner-err">{error}</div>}

      <div className="card">
        {loading ? (
          <div className="center" style={{ minHeight: 80 }}>
            <div className="spinner" />
          </div>
        ) : shares.length === 0 ? (
          <p className="muted">No share links yet.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Title</th>
                <th>Items</th>
                <th>Password</th>
                <th>Downloads</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {shares.map((s) => (
                <tr key={s.token}>
                  <td>{s.title || "(untitled)"}</td>
                  <td>{s.count}</td>
                  <td>{s.hasPassword ? "yes" : "—"}</td>
                  <td>
                    {s.downloads}
                    {s.maxDownloads > 0 ? ` / ${s.maxDownloads}` : ""}
                  </td>
                  <td>
                    <div className="row">
                      <a className="btn" href={s.url} target="_blank" rel="noreferrer">
                        Open
                      </a>
                      <button
                        className="btn"
                        onClick={() => navigator.clipboard?.writeText(s.url)}
                      >
                        Copy
                      </button>
                      <button className="btn btn-danger" onClick={() => revoke(s)}>
                        Revoke
                      </button>
                    </div>
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
