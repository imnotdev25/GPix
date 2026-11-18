import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { MouseEvent, PointerEvent as RPointerEvent, ReactNode } from "react";
import { Link } from "react-router-dom";
import * as api from "../api";
import type { MediaItem, MediaPage, TimelineMonth } from "../types";
import Thumb from "../components/Thumb";
import Icon from "../components/Icon";
import ShareDialog from "../components/ShareDialog";
import TimelineRail from "../components/TimelineRail";
import { AddToAlbumDialog, TagDialog } from "../components/CollectionDialogs";
import { errMsg, useConfirm, useToast } from "../components/feedback";
import { parseDate } from "../format";
import Viewer from "./Viewer";

type Day = { key: string; label: string; start: number; end: number };
type Mode = "reset" | "more" | "newer";
export type LibraryFilter = { album?: number; tag?: string };

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

/** The photo grid. Also used, with a filter, for an album or a tag. */
export default function Library({
  filter,
  title = "Library",
  back,
  actions,
  empty,
}: {
  filter?: LibraryFilter;
  title?: string;
  back?: { to: string; label: string };
  actions?: ReactNode;
  empty?: ReactNode;
}) {
  const fq = filter?.album ? `album=${filter.album}` : filter?.tag ? `tag=${encodeURIComponent(filter.tag)}` : "";

  const [items, setItems] = useState<MediaItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [prevCursor, setPrevCursor] = useState("");
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [warming, setWarming] = useState(false);
  const [error, setError] = useState("");
  const [months, setMonths] = useState<TimelineMonth[]>([]);
  const [topIndex, setTopIndex] = useState(0);
  const [viewing, setViewing] = useState<number | null>(null);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sharing, setSharing] = useState<string[] | null>(null);
  const [albumFor, setAlbumFor] = useState<string[] | null>(null);
  const [tagFor, setTagFor] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [focusIdx, setFocusIdx] = useState(0);
  const press = useRef<{ timer: number; x: number; y: number; fired: boolean } | null>(null);
  const sentinel = useRef<HTMLDivElement | null>(null);
  const loadingRef = useRef(false);
  const lastIndex = useRef<number | null>(null);
  const retryTimer = useRef(0);
  const anchor = useRef<{ key: string; top: number } | null>(null);
  const ask = useConfirm();
  const toast = useToast();

  const loadTimeline = () => {
    const q = new URLSearchParams(fq);
    try {
      q.set("tz", Intl.DateTimeFormat().resolvedOptions().timeZone);
    } catch {
      /* no Intl time zone: the server groups by UTC */
    }
    api
      .get<{ months: TimelineMonth[] }>(`/api/v1/media/timeline?${q}`)
      .then((r) => setMonths(r.months ?? []))
      .catch(() => {});
  };

  // reset: the first page (or the page starting at `from`); more: the next
  // page down; newer: the page above, after jumping to an older month.
  const fetchPage = async (mode: Mode, from?: number): Promise<boolean> => {
    if (loadingRef.current) return false;
    loadingRef.current = true;
    setLoading(true);
    const q = new URLSearchParams(fq);
    q.set("limit", "50");
    if (mode === "more" && cursor) q.set("cursor", cursor);
    if (mode === "newer") {
      q.set("before", prevCursor);
      const first = document.querySelector<HTMLElement>('.tile[data-index="0"]');
      if (first) anchor.current = { key: first.dataset.key ?? "", top: first.getBoundingClientRect().top };
    }
    if (mode === "reset" && from) q.set("from", String(from));
    try {
      const page = await api.get<MediaPage>(`/api/v1/media?${q}`);
      const got = page.items ?? [];
      setWarming(false);
      setError("");
      setTotal(page.total ?? 0);
      if (mode === "reset") {
        setItems(got);
        setOffset(page.offset ?? 0);
        setPrevCursor(page.prevCursor ?? "");
        setCursor(page.nextCursor);
        setHasMore(!!page.nextCursor);
        loadTimeline();
      } else if (mode === "more") {
        setItems((prev) => {
          const seen = new Set(prev.map((i) => i.key));
          return [...prev, ...got.filter((i) => !seen.has(i.key))];
        });
        setCursor(page.nextCursor);
        // Stop when the server says so, or when a page brings nothing: an
        // endless "next page" is what made a small library flicker.
        setHasMore(!!page.nextCursor && got.length > 0);
      } else {
        setItems((prev) => {
          const seen = new Set(prev.map((i) => i.key));
          return [...got.filter((i) => !seen.has(i.key)), ...prev];
        });
        setOffset(page.offset ?? 0);
        setPrevCursor(page.prevCursor ?? "");
      }
      return true;
    } catch (err) {
      anchor.current = null;
      if (err instanceof api.ApiError && err.status === 503) {
        // The server's first full listing is still running: try again shortly.
        setWarming(true);
        window.clearTimeout(retryTimer.current);
        retryTimer.current = window.setTimeout(() => fetchRef.current(mode, from), 4000);
      } else {
        setError(errMsg(err));
        if (mode === "more") setHasMore(false);
      }
      return false;
    } finally {
      loadingRef.current = false;
      setLoading(false);
      setLoaded(true);
    }
  };
  const fetchRef = useRef(fetchPage);
  fetchRef.current = fetchPage;

  useEffect(() => {
    fetchRef.current("reset");
    return () => window.clearTimeout(retryTimer.current);
  }, []);

  // Infinite scroll, a page of 50 at a time.
  const n = items.length;
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasMore || !loaded || n === 0) return;
    const io = new IntersectionObserver(
      (e) => {
        if (e[0].isIntersecting) fetchRef.current("more");
      },
      { rootMargin: "800px 0px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [hasMore, loaded, cursor, n > 0]);

  // Keep the view still when newer photos are added above it.
  useLayoutEffect(() => {
    const a = anchor.current;
    if (!a) return;
    anchor.current = null;
    const el = document.querySelector<HTMLElement>(`.tile[data-key="${CSS.escape(a.key)}"]`);
    if (el) window.scrollBy(0, el.getBoundingClientRect().top - a.top);
  }, [items]);

  // Which photo is at the top of the screen (for the date scrubber).
  useEffect(() => {
    let raf = 0;
    const measure = () => {
      raf = 0;
      const grid = document.querySelector<HTMLElement>(".day .grid");
      if (!grid) return;
      const x = grid.getBoundingClientRect().left + 8;
      const y = window.innerWidth <= 820 ? 110 : 80; // below the sticky bars
      const el = document.elementFromPoint(x, y)?.closest<HTMLElement>(".tile[data-index]");
      if (el) setTopIndex(Number(el.dataset.index));
      else if (window.scrollY < 40) setTopIndex(0);
    };
    const onScroll = () => {
      if (!raf) raf = requestAnimationFrame(measure);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.removeEventListener("scroll", onScroll);
      cancelAnimationFrame(raf);
    };
  }, []);

  const scrollToIndex = (i: number) => {
    const tile = document.querySelector<HTMLElement>(`.tile[data-index="${i}"]`);
    if (!tile) return;
    const day = tile.closest<HTMLElement>(".day");
    const target = day && day.querySelector(".tile[data-index]") === tile ? day : tile;
    target.scrollIntoView({ block: "start" });
  };

  // Scrubber: jump to a month, using what's loaded when it covers that month.
  const jumpToMonth = (m: TimelineMonth) => {
    const from = new Date(m.year, m.month, 1).getTime(); // first moment after that month (local time)
    const newest = n ? parseDate(items[0].mtime) : null;
    const covered = n > 0 && (offset === 0 || (newest !== null && newest.getTime() >= from));
    if (covered) {
      const i = items.findIndex((it) => (parseDate(it.mtime)?.getTime() ?? 0) < from);
      if (i >= 0) {
        scrollToIndex(i);
        return;
      }
      if (!hasMore) {
        scrollToIndex(n - 1);
        return;
      }
    }
    fetchRef.current("reset", from).then((ok) => {
      if (ok) window.scrollTo({ top: 0 });
    });
  };

  const days = useMemo(() => groupDays(items), [items]);
  const selecting = selectMode || selected.size > 0;

  const clearSelection = () => {
    setSelected(new Set());
    setSelectMode(false);
    lastIndex.current = null;
  };

  const refresh = () => {
    clearSelection();
    setHasMore(true);
    fetchRef.current("reset");
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
      const res = await api.post<{ deleted: string[]; failed: number }>("/api/v1/media/delete", { keys });
      const gone = new Set(res.deleted);
      setItems((prev) => prev.filter((it) => !gone.has(it.key)));
      setTotal((t) => Math.max(0, t - gone.size));
      clearSelection();
      toast(
        res.failed ? `Moved ${gone.size} to trash; ${res.failed} failed` : `Moved ${plural(gone.size, "item")} to trash`,
        res.failed ? "err" : "ok",
      );
    } catch (err) {
      toast("Delete failed: " + errMsg(err), "err");
    } finally {
      setBusy(false);
    }
  };

  const removeFromAlbum = async () => {
    if (!filter?.album) return;
    const keys = [...selected];
    try {
      await api.post(`/api/v1/albums/${filter.album}/items/remove`, { keys });
      const gone = new Set(keys);
      setItems((prev) => prev.filter((it) => !gone.has(it.key)));
      setTotal((t) => Math.max(0, t - keys.length));
      clearSelection();
      toast(`Removed ${plural(keys.length, "item")} from the album`, "ok");
    } catch (err) {
      toast("Couldn't remove them: " + errMsg(err), "err");
    }
  };

  const applyTags = (next: Record<string, string[]>) =>
    setItems((prev) => prev.map((it) => (it.key in next ? { ...it, tags: next[it.key] } : it)));

  const downloadSelected = () => {
    const list = items.filter((it) => selected.has(it.key));
    // Staggered so browsers don't drop the later downloads.
    list.forEach((it, j) =>
      window.setTimeout(() => {
        const a = document.createElement("a");
        a.href = api.originalUrl(it.key, true);
        a.download = it.name;
        document.body.appendChild(a);
        a.click();
        a.remove();
      }, j * 400),
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
    setTotal((t) => Math.max(0, t - 1));
    if (!next.length) setViewing(null);
    else setViewing(Math.min(Math.max(idx, 0), next.length - 1));
  };

  const count = total || n;
  const showRail = months.length > 1 && total > 0;
  const railPos = total > 0 ? (offset + topIndex) / total : 0;
  const showSkeleton = !loaded || (warming && n === 0);

  return (
    <div className={showRail ? "library with-rail" : "library"}>
      <div className="page-head row">
        <div>
          {back && (
            <Link className="back-link" to={back.to}>
              <Icon name="chevronLeft" size={14} />
              {back.label}
            </Link>
          )}
          <h1>{title}</h1>
          <p className="sub num">
            {!loaded || warming
              ? " "
              : count > 0
                ? plural(count, "item")
                : filter
                  ? "No items yet"
                  : "Photos, videos and documents on this server"}
          </p>
        </div>
        <div className="spacer" />
        {actions}
        {!filter && (
          <Link className="btn" to="/upload" aria-label="Upload">
            <Icon name="upload" size={16} />
            <span className="hide-sm">Upload</span>
          </Link>
        )}
        <button
          className={"btn" + (selecting ? " on" : "")}
          onClick={() => (selecting ? clearSelection() : setSelectMode(true))}
          aria-pressed={selecting}
          aria-label={selecting ? "Done selecting" : "Select"}
        >
          <Icon name="select" size={16} />
          <span className="hide-sm">{selecting ? "Done" : "Select"}</span>
        </button>
        <button className="btn btn-icon" onClick={refresh} disabled={loading} aria-label="Refresh" title="Refresh">
          <Icon name="refresh" size={16} />
        </button>
      </div>

      {warming && (
        <div className="banner banner-info">
          <div className="spinner" />
          <span>Loading your library for the first time. This can take a minute for a large library.</span>
        </div>
      )}

      {error && (
        <div className="banner banner-err row">
          <span>{error}</span>
          <div className="spacer" />
          <button
            className="btn btn-sm"
            onClick={() => {
              setHasMore(true);
              fetchRef.current(n === 0 ? "reset" : "more");
            }}
          >
            Retry
          </button>
        </div>
      )}

      {showSkeleton && (
        <div className="grid" aria-hidden="true">
          {Array.from({ length: 12 }, (_, i) => (
            <div key={i} className="tile skeleton" />
          ))}
        </div>
      )}

      {loaded && !warming && n === 0 && !error && !loading &&
        (empty ?? (
          <div className="empty">
            <Icon name="photos" size={40} />
            <h2>No photos yet</h2>
            <p className="muted">Upload photos, videos or any other file to get started.</p>
            <Link className="btn btn-primary" to="/upload">
              <Icon name="upload" size={16} />
              Upload
            </Link>
          </div>
        ))}

      {offset > 0 && n > 0 && (
        <button className="btn newer" onClick={() => fetchRef.current("newer")} disabled={loading}>
          <Icon name="chevronLeft" size={16} />
          Show newer photos
        </button>
      )}

      {days.map((day) => {
        const dayItems = items.slice(day.start, day.end);
        const allOn = dayItems.every((it) => selected.has(it.key));
        return (
          <section className={"day" + (selecting ? " selecting" : "")} key={day.key}>
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
                    key={it.key}
                    className={
                      "tile" + (on ? " selected" : "") + (selecting ? " selecting" : "") + (named ? " named" : "")
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
                      } else if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"].includes(e.key)) {
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
        n > 0 && (
          <div className="grid grid-more" aria-hidden="true">
            {Array.from({ length: 6 }, (_, i) => (
              <div key={i} className="tile skeleton" />
            ))}
          </div>
        )}

      {showRail && <TimelineRail months={months} total={total} position={railPos} onJump={jumpToMonth} />}

      {selecting && (
        <div className="selbar" role="toolbar" aria-label="Selection">
          <button className="btn btn-ghost btn-icon" onClick={clearSelection} aria-label="Clear selection">
            <Icon name="x" />
          </button>
          <strong className="num">{selected.size} selected</strong>
          <div className="spacer" />
          <button className="btn btn-ghost" disabled={!selected.size} onClick={() => setSharing([...selected])} aria-label="Share">
            <Icon name="share" size={16} />
            <span className="hide-sm">Share</span>
          </button>
          <button className="btn btn-ghost" disabled={!selected.size} onClick={() => setAlbumFor([...selected])} aria-label="Add to album">
            <Icon name="album" size={16} />
            <span className="hide-sm">Album</span>
          </button>
          <button className="btn btn-ghost" disabled={!selected.size} onClick={() => setTagFor([...selected])} aria-label="Tag">
            <Icon name="tag" size={16} />
            <span className="hide-sm">Tag</span>
          </button>
          {filter?.album && (
            <button className="btn btn-ghost" disabled={!selected.size} onClick={removeFromAlbum} aria-label="Remove from album">
              <Icon name="x" size={16} />
              <span className="hide-sm">Remove</span>
            </button>
          )}
          <button className="btn btn-ghost" disabled={!selected.size} onClick={downloadSelected} aria-label="Download">
            <Icon name="download" size={16} />
            <span className="hide-sm">Download</span>
          </button>
          <button className="btn btn-danger" disabled={!selected.size || busy} onClick={deleteSelected} aria-label="Move to trash">
            <Icon name="trash" size={16} />
            <span className="hide-sm">{busy ? "Deleting…" : "Delete"}</span>
          </button>
        </div>
      )}

      {sharing && (
        <ShareDialog
          keys={sharing}
          defaultTitle={
            sharing.length === 1 ? (items.find((it) => it.key === sharing[0])?.name ?? "") : `${sharing.length} items`
          }
          onClose={() => setSharing(null)}
          onCreated={clearSelection}
        />
      )}
      {albumFor && <AddToAlbumDialog keys={albumFor} onClose={() => setAlbumFor(null)} onDone={clearSelection} />}
      {tagFor && (
        <TagDialog
          keys={tagFor}
          initial={tagFor.length === 1 ? (items.find((it) => it.key === tagFor[0])?.tags ?? []) : undefined}
          onClose={() => setTagFor(null)}
          onDone={(t) => {
            applyTags(t);
            if (tagFor.length > 1) clearSelection();
          }}
        />
      )}

      {viewing !== null && items[viewing] && (
        <Viewer
          items={items}
          index={viewing}
          onClose={() => setViewing(null)}
          onIndex={setViewing}
          onDeleted={onViewerDeleted}
          onTagsChanged={(key, tags) => applyTags({ [key]: tags })}
        />
      )}
    </div>
  );
}
