import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { MouseEvent, PointerEvent as RPointerEvent } from "react";
import { Link } from "react-router-dom";
import * as api from "../api";
import type { MediaItem, MediaPage } from "../types";
import Thumb from "../components/Thumb";
import Icon from "../components/Icon";
import ShareDialog from "../components/ShareDialog";
import { errMsg, useConfirm, useToast } from "../components/feedback";
import { parseDate } from "../format";
import Viewer from "./Viewer";

type Day = { key: string; label: string; start: number; end: number };

function dayKey(d: Date | null): string {
  return d ? `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}` : "unknown";
}

function dayLabel(d: Date | null): string {
  if (!d) return "Unknown date";
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const that = new Date(d);
  that.setHours(0, 0, 0, 0);
  const diff = Math.round((today.getTime() - that.getTime()) / 86_400_000);
  if (diff === 0) return "Today";
  if (diff === 1) return "Yesterday";
  const opts: Intl.DateTimeFormatOptions = { weekday: "short", day: "numeric", month: "short" };
  if (d.getFullYear() !== today.getFullYear()) opts.year = "numeric";
  return d.toLocaleDateString(undefined, opts);
}

/** Splits the (newest-first) listing into runs of items from the same day. */
function groupDays(items: MediaItem[]): Day[] {
  const days: Day[] = [];
  items.forEach((it, i) => {
    const d = parseDate(it.mtime);
    const k = dayKey(d);
    const last = days[days.length - 1];
    if (last && last.key === k) last.end = i + 1;
    else days.push({ key: k, label: dayLabel(d), start: i, end: i + 1 });
  });
  return days;
}

const plural = (n: number, word: string) => `${n.toLocaleString()} ${word}${n === 1 ? "" : "s"}`;

export default function Library() {
  const [items, setItems] = useState<MediaItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [viewing, setViewing] = useState<number | null>(null);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sharing, setSharing] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [focusIdx, setFocusIdx] = useState(0);
  const press = useRef<{ timer: number; x: number; y: number; fired: boolean } | null>(null);
  const sentinel = useRef<HTMLDivElement | null>(null);
  const loadingRef = useRef(false);
  const lastIndex = useRef<number | null>(null);
  const ask = useConfirm();
  const toast = useToast();

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
        setError(errMsg(err));
      } finally {
        loadingRef.current = false;
        setLoading(false);
        setLoaded(true);
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
    const io = new IntersectionObserver(
      (e) => {
        if (e[0].isIntersecting) load();
      },
      { rootMargin: "800px 0px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [hasMore, load]);

  const days = useMemo(() => groupDays(items), [items]);
  const selecting = selectMode || selected.size > 0;

  const clearSelection = () => {
    setSelected(new Set());
    setSelectMode(false);
    lastIndex.current = null;
  };

  // Shift-click selects everything between the last clicked tile and this one.
  const toggleAt = (i: number, range: boolean) => {
    const from = lastIndex.current;
    lastIndex.current = i;
    setSelected((prev) => {
      const next = new Set(prev);
      if (range && from !== null && from !== i) {
        const [a, b] = from < i ? [from, i] : [i, from];
        for (let j = a; j <= b; j++) next.add(items[j].key);
      } else if (next.has(items[i].key)) {
        next.delete(items[i].key);
      } else {
        next.add(items[i].key);
      }
      return next;
    });
  };

  const toggleDay = (day: Day) => {
    const keys = items.slice(day.start, day.end).map((it) => it.key);
    setSelected((prev) => {
      const next = new Set(prev);
      const all = keys.every((k) => prev.has(k));
      keys.forEach((k) => (all ? next.delete(k) : next.add(k)));
      return next;
    });
  };

  const onTileClick = (e: MouseEvent, i: number) => {
    if (press.current?.fired) {
      press.current = null; // this click ends a long-press
      return;
    }
    setFocusIdx(i);
    if (selecting || e.shiftKey || e.metaKey || e.ctrlKey) toggleAt(i, e.shiftKey);
    else setViewing(i);
  };

  // Long-press on a touch screen starts selecting, like a phone gallery.
  const startPress = (e: RPointerEvent, i: number) => {
    if (e.pointerType !== "touch") {
      press.current = null;
      return;
    }
    const timer = window.setTimeout(() => {
      if (press.current) press.current.fired = true;
      navigator.vibrate?.(10);
      lastIndex.current = null;
      toggleAt(i, false);
    }, 450);
    press.current = { timer, x: e.clientX, y: e.clientY, fired: false };
  };
  const movePress = (e: RPointerEvent) => {
    const p = press.current;
    if (p && !p.fired && Math.hypot(e.clientX - p.x, e.clientY - p.y) > 10) {
      window.clearTimeout(p.timer);
      press.current = null;
    }
  };
  const endPress = () => {
    const p = press.current;
    if (p && !p.fired) {
      window.clearTimeout(p.timer);
      press.current = null;
    }
  };

  // Arrow keys move between photos; the grid is a single tab stop.
  const moveFocus = (from: number, key: string) => {
    const tiles = Array.from(document.querySelectorAll<HTMLElement>(".tile[data-index]"));
    const cur = tiles[from];
    if (!cur) return;
    let to = from;
    if (key === "ArrowLeft") to = from - 1;
    else if (key === "ArrowRight") to = from + 1;
    else if (key === "Home") to = 0;
    else if (key === "End") to = tiles.length - 1;
    else {
      // Up/down: the nearest tile in the next row, across day headings too.
      const r = cur.getBoundingClientRect();
      const cx = r.left + r.width / 2;
      let best = -1;
      let bestScore = Infinity;
      tiles.forEach((t, j) => {
        const q = t.getBoundingClientRect();
        const dy = key === "ArrowDown" ? q.top - r.top : r.top - q.top;
        if (dy < r.height / 2) return;
        const score = dy * 4 + Math.abs(q.left + q.width / 2 - cx);
        if (score < bestScore) {
          bestScore = score;
          best = j;
        }
      });
      if (best < 0) return;
      to = best;
    }
    if (to < 0 || to >= tiles.length) return;
    setFocusIdx(to);
    tiles[to].focus();
  };

  // Esc clears the selection (the viewer and dialogs handle their own Esc).
  useEffect(() => {
    if (!selecting || viewing !== null) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !document.querySelector(".modal-backdrop")) clearSelection();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selecting, viewing]);

  const deleteSelected = async () => {
    const keys = [...selected];
    if (!keys.length) return;
    const ok = await ask({
      title: `Move ${plural(keys.length, "item")} to trash?`,
      message: "They stay recoverable in the Google Photos trash for 60 days.",
      confirmLabel: "Move to trash",
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      const res = await api.post<{ deleted: string[]; failed: number }>("/api/v1/media/delete", {
        keys,
      });
      const gone = new Set(res.deleted);
      setItems((prev) => prev.filter((it) => !gone.has(it.key)));
      clearSelection();
      toast(
        res.failed
          ? `Moved ${gone.size} to trash; ${res.failed} failed`
          : `Moved ${plural(gone.size, "item")} to trash`,
        res.failed ? "err" : "ok",
      );
    } catch (err) {
      toast("Delete failed: " + errMsg(err), "err");
    } finally {
      setBusy(false);
    }
  };

  const downloadSelected = () => {
    const list = items.filter((it) => selected.has(it.key));
    // Staggered so browsers don't drop the later downloads.
    list.forEach((it, n) =>
      window.setTimeout(() => {
        const a = document.createElement("a");
        a.href = api.originalUrl(it.key, true);
        a.download = it.name;
        document.body.appendChild(a);
        a.click();
        a.remove();
      }, n * 400),
    );
    toast(
      list.length > 1
        ? `Downloading ${list.length} files. Your browser may ask to allow multiple downloads.`
        : "Download started",
    );
  };

  const onViewerDeleted = (key: string) => {
    const idx = items.findIndex((it) => it.key === key);
    const next = items.filter((it) => it.key !== key);
    setItems(next);
    if (!next.length) setViewing(null);
    else setViewing(Math.min(Math.max(idx, 0), next.length - 1));
  };

  const n = items.length;

  return (
    <>
      <div className="page-head row">
        <div>
          <h1>Library</h1>
          <p className="sub num">
            {n > 0
              ? `${plural(n, "item")} loaded${hasMore ? " · scroll for more" : ""}`
              : "Photos, videos and documents on this server"}
          </p>
        </div>
        <div className="spacer" />
        <Link className="btn" to="/upload" aria-label="Upload">
          <Icon name="upload" size={16} />
          <span className="hide-sm">Upload</span>
        </Link>
        <button
          className={"btn" + (selecting ? " on" : "")}
          onClick={() => (selecting ? clearSelection() : setSelectMode(true))}
          aria-pressed={selecting}
          aria-label={selecting ? "Done selecting" : "Select"}
        >
          <Icon name="select" size={16} />
          <span className="hide-sm">{selecting ? "Done" : "Select"}</span>
        </button>
        <button
          className="btn btn-icon"
          onClick={() => {
            clearSelection();
            load(true);
          }}
          disabled={loading}
          aria-label="Refresh"
          title="Refresh"
        >
          <Icon name="refresh" size={16} />
        </button>
      </div>

      {error && (
        <div className="banner banner-err row">
          <span>{error}</span>
          <div className="spacer" />
          <button className="btn btn-sm" onClick={() => load(n === 0)}>
            Retry
          </button>
        </div>
      )}

      {!loaded && (
        <div className="grid" aria-hidden="true">
          {Array.from({ length: 12 }, (_, i) => (
            <div key={i} className="tile skeleton" />
          ))}
        </div>
      )}

      {loaded && n === 0 && !error && !loading && (
        <div className="empty">
          <Icon name="photos" size={40} />
          <h2>No photos yet</h2>
          <p className="muted">Upload photos, videos or any other file to get started.</p>
          <Link className="btn btn-primary" to="/upload">
            <Icon name="upload" size={16} />
            Upload
          </Link>
        </div>
      )}

      {days.map((day) => {
        const dayItems = items.slice(day.start, day.end);
        const allOn = dayItems.every((it) => selected.has(it.key));
        return (
          <section className={"day" + (selecting ? " selecting" : "")} key={`${day.key}-${day.start}`}>
            <div className="day-head">
              <button
                className={"day-check" + (selecting && allOn ? " on" : "")}
                onClick={() => toggleDay(day)}
                aria-label={`Select all from ${day.label}`}
                title="Select this day"
              >
                <Icon name="check" size={13} />
              </button>
              <h2>{day.label}</h2>
              <span className="muted">{dayItems.length}</span>
            </div>
            <div className="grid">
              {dayItems.map((it, j) => {
                const i = day.start + j;
                const on = selected.has(it.key);
                const named = it.class === "doc" || it.class === "file";
                return (
                  <div
                    key={it.key + i}
                    className={
                      "tile" +
                      (on ? " selected" : "") +
                      (selecting ? " selecting" : "") +
                      (named ? " named" : "")
                    }
                    role="button"
                    data-key={it.key}
                    data-index={i}
                    tabIndex={i === Math.min(focusIdx, n - 1) ? 0 : -1}
                    aria-label={it.name}
                    aria-pressed={selecting ? on : undefined}
                    onFocus={() => setFocusIdx(i)}
                    onClick={(e) => onTileClick(e, i)}
                    onPointerDown={(e) => startPress(e, i)}
                    onPointerMove={movePress}
                    onPointerUp={endPress}
                    onPointerCancel={endPress}
                    onContextMenu={(e) => {
                      if (press.current) e.preventDefault();
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        if (selecting) toggleAt(i, e.shiftKey);
                        else setViewing(i);
                      } else if (
                        ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"].includes(e.key)
                      ) {
                        e.preventDefault();
                        moveFocus(i, e.key);
                      }
                    }}
                  >
                    <Thumb item={it} />
                    <button
                      className="tile-check"
                      tabIndex={-1}
                      aria-hidden="true"
                      onClick={(e) => {
                        e.stopPropagation();
                        toggleAt(i, e.shiftKey);
                      }}
                    >
                      <Icon name="check" size={14} />
                    </button>
                    <div className="tile-badges">
                      {it.encrypted && (
                        <span className="tile-badge" title="Encrypted">
                          <Icon name="lock" size={12} />
                        </span>
                      )}
                      {it.class === "video" && (
                        <span className="tile-badge" title="Video">
                          <Icon name="play" size={12} />
                        </span>
                      )}
                    </div>
                    <div className="tile-meta">{it.name}</div>
                  </div>
                );
              })}
            </div>
          </section>
        );
      })}

      <div ref={sentinel} style={{ height: 1 }} />
      {loaded &&
        loading &&
        (n > 0 ? (
          <div className="grid grid-more" aria-hidden="true">
            {Array.from({ length: 6 }, (_, i) => (
              <div key={i} className="tile skeleton" />
            ))}
          </div>
        ) : (
          <div className="center" style={{ minHeight: 90 }}>
            <div className="spinner" />
          </div>
        ))}

      {selecting && (
        <div className="selbar" role="toolbar" aria-label="Selection">
          <button className="btn btn-ghost btn-icon" onClick={clearSelection} aria-label="Clear selection">
            <Icon name="x" />
          </button>
          <strong>{selected.size} selected</strong>
          <div className="spacer" />
          <button
            className="btn btn-ghost"
            disabled={!selected.size}
            onClick={() => setSharing([...selected])}
            aria-label="Share"
          >
            <Icon name="share" size={16} />
            <span className="hide-sm">Share</span>
          </button>
          <button
            className="btn btn-ghost"
            disabled={!selected.size}
            onClick={downloadSelected}
            aria-label="Download"
          >
            <Icon name="download" size={16} />
            <span className="hide-sm">Download</span>
          </button>
          <button
            className="btn btn-danger"
            disabled={!selected.size || busy}
            onClick={deleteSelected}
            aria-label="Move to trash"
          >
            <Icon name="trash" size={16} />
            <span className="hide-sm">{busy ? "Deleting…" : "Delete"}</span>
          </button>
        </div>
      )}

      {sharing && (
        <ShareDialog
          keys={sharing}
          defaultTitle={
            sharing.length === 1
              ? (items.find((it) => it.key === sharing[0])?.name ?? "")
              : `${sharing.length} items`
          }
          onClose={() => setSharing(null)}
          onCreated={clearSelection}
        />
      )}

      {viewing !== null && items[viewing] && (
        <Viewer
          items={items}
          index={viewing}
          onClose={() => setViewing(null)}
          onIndex={setViewing}
          onDeleted={onViewerDeleted}
        />
      )}
    </>
  );
}
