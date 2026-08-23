import { useCallback, useEffect, useRef, useState } from "react";
import * as api from "../api";
import type { MediaItem, MediaPage } from "../types";
import Thumb from "../components/Thumb";
import Viewer from "./Viewer";

export default function Library() {
  const [items, setItems] = useState<MediaItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [viewing, setViewing] = useState<number | null>(null);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const sentinel = useRef<HTMLDivElement | null>(null);
  const loadingRef = useRef(false);

  const load = useCallback(
    async (reset = false) => {
      if (loadingRef.current) return;
      loadingRef.current = true;
      setLoading(true);
      try {
        const c = reset ? "" : cursor;
        const page = await api.get<MediaPage>(
          "/api/v1/media" + (c ? `?cursor=${encodeURIComponent(c)}` : ""),
        );
        const got = page.items ?? [];
        setItems((prev) => (reset ? got : [...prev, ...got]));
        setCursor(page.nextCursor);
        setHasMore(!!page.nextCursor);
        setError("");
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        loadingRef.current = false;
        setLoading(false);
      }
    },
    [cursor],
  );

  useEffect(() => {
    load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Infinite scroll.
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasMore) return;
    const io = new IntersectionObserver((e) => {
      if (e[0].isIntersecting) load();
    });
    io.observe(el);
    return () => io.disconnect();
  }, [hasMore, load]);

  const toggle = (key: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(key) ? next.delete(key) : next.add(key);
      return next;
    });

  const createShare = async () => {
    const keys = [...selected];
    if (!keys.length) return;
    const password = prompt("Password for this link (leave blank for none):") ?? "";
    const hoursStr = prompt("Expire after how many hours? (0 = never)", "0") ?? "0";
    try {
      const share = await api.post<{ url: string }>("/api/v1/shares", {
        keys,
        title: keys.length === 1 ? items.find((i) => i.key === keys[0])?.name : `${keys.length} items`,
        password,
        expiresHours: parseInt(hoursStr, 10) || 0,
        allowOriginal: true,
      });
      await navigator.clipboard?.writeText(share.url).catch(() => {});
      alert(`Share link created (copied to clipboard):\n${share.url}`);
      setSelected(new Set());
      setSelectMode(false);
    } catch (err) {
      alert("Could not create link: " + (err instanceof Error ? err.message : err));
    }
  };

  return (
    <>
      <div className="page-head row">
        <div>
          <span className="eyebrow">Your collection</span>
          <h1>Library</h1>
          <p className="sub">
            {items.length > 0 ? `${items.length} loaded` : "Photos, videos and documents on this server"}
          </p>
        </div>
        <div className="spacer" />
        {selectMode && selected.size > 0 && (
          <button className="btn btn-primary" onClick={createShare}>
            Share {selected.size}
          </button>
        )}
        <button
          className="btn"
          onClick={() => {
            setSelectMode((s) => !s);
            setSelected(new Set());
          }}
        >
          {selectMode ? "Cancel" : "Select"}
        </button>
        <button className="btn" onClick={() => load(true)} disabled={loading}>
          Refresh
        </button>
      </div>

      {error && <div className="banner banner-err">{error}</div>}

      {items.length === 0 && !loading && !error && (
        <div className="center">
          <p className="muted">Nothing here yet — upload something to get started.</p>
        </div>
      )}

      <div className="grid">
        {items.map((it, i) => (
          <div
            key={it.key + i}
            className={"tile" + (selected.has(it.key) ? " selected" : "")}
            onClick={() => (selectMode ? toggle(it.key) : setViewing(i))}
            title={it.name}
          >
            <Thumb item={it} />
            {it.encrypted && <span className="tile-lock">🔒</span>}
            {it.class === "video" && <span className="tile-tag">video</span>}
            {it.class === "doc" && <span className="tile-tag">doc</span>}
            <div className="tile-meta">{it.name}</div>
          </div>
        ))}
      </div>

      <div ref={sentinel} style={{ height: 1 }} />
      {loading && (
        <div className="center" style={{ minHeight: 90 }}>
          <div className="spinner" />
        </div>
      )}

      {viewing !== null && items[viewing] && (
        <Viewer
          items={items}
          index={viewing}
          onClose={() => setViewing(null)}
          onIndex={setViewing}
          onDeleted={() => {
            setViewing(null);
            load(true);
          }}
        />
      )}
    </>
  );
}
