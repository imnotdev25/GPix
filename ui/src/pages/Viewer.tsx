import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import * as api from "../api";
import type { MediaItem } from "../types";
import Icon from "../components/Icon";
import ShareDialog from "../components/ShareDialog";
import { TagDialog } from "../components/CollectionDialogs";
import { errMsg, useConfirm, useToast } from "../components/feedback";
import { formatBytes, formatDateTime, parseDate } from "../format";

const INFO_KEY = "gpix-viewer-info";
const kindLabel = { photo: "Photo", video: "Video", doc: "Document", file: "File" } as const;

// Sizes the server renders (thumbSize in pkg/web/handlers_media.go). The grid
// already loaded PREVIEW, so it shows instantly while FULL loads.
const PREVIEW = 512;
const FULL = 2048;
const MAX_ZOOM = 4;
const EASE = "cubic-bezier(0.16, 1, 0.3, 1)";

type View = { s: number; x: number; y: number };
type Size = { w: number; h: number };
type Gesture = {
  sx: number;
  sy: number;
  view: View;
  moved: boolean;
  axis: "" | "x" | "y";
  onImg: boolean;
  type: string;
  pinch?: { dist: number; mx: number; my: number };
};

const FIT: View = { s: 1, x: 0, y: 0 };
const fullLoaded = new Set<string>(); // FULL urls already in the browser cache

const reducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
const tileFor = (key: string) =>
  document.querySelector<HTMLElement>(`.tile[data-key="${CSS.escape(key)}"]`);
const onScreen = (r: DOMRect) => r.width > 0 && r.bottom > 0 && r.top < window.innerHeight;

/** Transform + clip that make an element laid out at `from` appear at `to`;
 *  `cover` crops it to `to`'s shape the way the square grid tile does. */
function flipFrame(from: DOMRect, to: DOMRect, cover: boolean, radius: number): Keyframe {
  const k = cover ? Math.max(to.width / from.width, to.height / from.height) : to.width / from.width;
  const ix = cover ? (from.width - to.width / k) / 2 : 0;
  const iy = cover ? (from.height - to.height / k) / 2 : 0;
  const dx = to.left + to.width / 2 - (from.left + from.width / 2);
  const dy = to.top + to.height / 2 - (from.top + from.height / 2);
  return {
    transform: `translate(${dx}px, ${dy}px) scale(${k})`,
    clipPath: `inset(${iy}px ${ix}px round ${radius / k}px)`,
    opacity: 1,
  };
}

function initialInfo(): boolean {
  try {
    return localStorage.getItem(INFO_KEY) === "1";
  } catch {
    return false;
  }
}

export default function Viewer({
  items,
  index,
  onClose,
  onIndex,
  onDeleted,
  onTagsChanged,
}: {
  items: MediaItem[];
  index: number;
  onClose: () => void;
  onIndex: (i: number) => void;
  onDeleted: (key: string) => void;
  onTagsChanged?: (key: string, tags: string[]) => void;
}) {
  const item = items[index];
  const isImage = item.class === "photo" || item.class === "doc";

  const [info, setInfo] = useState(initialInfo);
  const [sharing, setSharing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [tagging, setTagging] = useState(false);
  const [idle, setIdle] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [smooth, setSmooth] = useState(true);
  const [zoom, setZoom] = useState<View & { key: string }>({ key: item.key, ...FIT });
  const [nat, setNat] = useState<(Size & { key: string; capped: boolean }) | null>(null);
  const [fullFor, setFullFor] = useState<string | null>(null);
  const [fullErr, setFullErr] = useState<string | null>(null);
  const [prevErr, setPrevErr] = useState<string | null>(null);
  const [stage, setStage] = useState<Size>({ w: 0, h: 0 });

  const rootRef = useRef<HTMLDivElement>(null);
  const backdropRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const mediaRef = useRef<HTMLDivElement>(null);
  const imgRef = useRef<HTMLImageElement>(null);
  const opened = useRef(false);
  const closing = useRef(false);
  const hiddenTile = useRef<HTMLElement | null>(null);
  const dir = useRef(0);
  const shownKey = useRef(item.key);
  const pts = useRef(new Map<number, { x: number; y: number }>());
  const gest = useRef<Gesture | null>(null);
  const lastTap = useRef<{ t: number; x: number; y: number } | null>(null);
  const tapTimer = useRef(0);
  const idleTimer = useRef(0);
  const ask = useConfirm();
  const toast = useToast();

  const view = zoom.key === item.key ? zoom : FIT;
  const zoomed = view.s > 1;
  const keyRef = useRef(item.key);
  keyRef.current = item.key;
  const viewRef = useRef(view);
  viewRef.current = view;
  const infoRef = useRef(info);
  infoRef.current = info;

  const hasPrev = index > 0;
  const hasNext = index < items.length - 1;
  const date = parseDate(item.mtime);
  const download = api.originalUrl(item.key, true);
  const fullUrl = api.thumbUrl(item, FULL);
  const showFull = fullFor === item.key || fullLoaded.has(fullUrl);
  const src = showFull ? fullUrl : api.thumbUrl(item, PREVIEW);
  const failedBoth = isImage && fullErr === item.key && prevErr === item.key;
  const pad = stage.w < 600 ? 0 : 16;

  // One fitted box for both the preview and the full image, so swapping them
  // never shifts the layout. Small originals aren't blown up past 1:1.
  let box: Size | null = null;
  if (isImage && nat && nat.key === item.key && stage.w > 0) {
    let f = Math.min((stage.w - pad * 2) / nat.w, (stage.h - pad * 2) / nat.h);
    if (nat.capped) f = Math.min(f, 1);
    box = { w: Math.round(nat.w * f), h: Math.round(nat.h * f) };
  }

  // ---- zoom & pan -----------------------------------------------------------

  const clampView = (v: View): View => {
    const img = imgRef.current;
    const st = stageRef.current;
    if (!img || !st || v.s <= 1.01) return FIT;
    const s = Math.min(v.s, MAX_ZOOM);
    const mx = Math.max(0, (img.offsetWidth * s - st.clientWidth) / 2);
    const my = Math.max(0, (img.offsetHeight * s - st.clientHeight) / 2);
    return { s, x: clamp(v.x, -mx, mx), y: clamp(v.y, -my, my) };
  };

  const applyView = (v: View, animate = false) => {
    setSmooth(animate);
    setZoom({ key: keyRef.current, ...clampView(v) });
  };

  // Untransformed centre of the image, in viewport coordinates.
  const layoutCenter = () => {
    const img = imgRef.current!;
    const r = mediaRef.current!.getBoundingClientRect();
    return { x: r.left + img.offsetLeft + img.offsetWidth / 2, y: r.top + img.offsetTop + img.offsetHeight / 2 };
  };

  /** Zoom to scale `s2`, keeping the point under (px, py) where it is. */
  const zoomAt = (s2: number, px: number, py: number, animate = false) => {
    if (!imgRef.current || !mediaRef.current) return;
    const v = viewRef.current;
    const s = clamp(s2, 1, MAX_ZOOM);
    const c = layoutCenter();
    const f = 1 - s / v.s;
    applyView({ s, x: v.x + (px - c.x - v.x) * f, y: v.y + (py - c.y - v.y) * f }, animate);
  };

  const zoomAtCenter = (s2: number, animate = true) => {
    const r = stageRef.current?.getBoundingClientRect();
    if (r) zoomAt(s2, r.left + r.width / 2, r.top + r.height / 2, animate);
  };

  const toggleZoomAt = (px: number, py: number) => {
    if (viewRef.current.s > 1) applyView(FIT, true);
    else zoomAt(2.5, px, py, true);
  };

  // ---- navigation, chrome, close ----------------------------------------------

  const prev = () => {
    if (!hasPrev || closing.current) return;
    dir.current = -1;
    onIndex(index - 1);
  };
  const next = () => {
    if (!hasNext || closing.current) return;
    dir.current = 1;
    onIndex(index + 1);
  };

  const toggleInfo = () =>
    setInfo((v) => {
      try {
        localStorage.setItem(INFO_KEY, v ? "0" : "1");
      } catch {
        /* storage unavailable */
      }
      return !v;
    });

  // Hide the toolbar and arrows after a moment without mouse movement.
  const wake = () => {
    setIdle(false);
    window.clearTimeout(idleTimer.current);
    if (!window.matchMedia("(hover: hover)").matches) return;
    idleTimer.current = window.setTimeout(() => {
      const busy = rootRef.current?.querySelector(".viewer-bar:hover, .viewer-bar:focus-within, .viewer-nav:hover");
      if (!infoRef.current && !busy) setIdle(true);
    }, 2500);
  };

  const close = () => {
    if (closing.current) return;
    closing.current = true;
    setLeaving(true);
    window.clearTimeout(tapTimer.current);
    const tile = tileFor(item.key);
    const finish = () => {
      if (hiddenTile.current) hiddenTile.current.style.visibility = "";
      hiddenTile.current = null;
      onClose();
      tile?.focus({ preventScroll: true });
    };
    if (reducedMotion()) {
      finish();
      return;
    }
    const img = imgRef.current;
    const media = mediaRef.current;
    const back = backdropRef.current;
    back?.animate([{ opacity: back.style.opacity || "1" }, { opacity: 0 }], { duration: 320, easing: EASE, fill: "forwards" });

    // Fly the photo back into its grid tile.
    if (img && media && box && viewRef.current.s <= 1 && tile) {
      tile.scrollIntoView({ block: "nearest" });
      const t = tile.getBoundingClientRect();
      if (onScreen(t)) {
        const start = img.getBoundingClientRect(); // includes any swipe offset
        media.style.transform = "";
        const f = img.getBoundingClientRect();
        const r = parseFloat(getComputedStyle(tile).borderRadius) || 0;
        tile.style.visibility = "hidden";
        hiddenTile.current = tile;
        img
          .animate([flipFrame(f, start, false, 4), flipFrame(f, t, true, r)], {
            duration: 380,
            easing: EASE,
            fill: "forwards",
          })
          .finished.then(finish, finish);
        return;
      }
    }
    const el = media ?? rootRef.current;
    if (!el) {
      finish();
      return;
    }
    el.animate([{ opacity: 1 }, { opacity: 0, transform: "scale(.96)" }], {
      duration: 200,
      easing: "ease-out",
      fill: "forwards",
    }).finished.then(finish, finish);
  };

  // ---- effects ------------------------------------------------------------------

  useLayoutEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const measure = () => setStage({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Opening: grow the photo out of the tile that was clicked.
  useLayoutEffect(() => {
    if (opened.current) return;
    if (isImage && !box && !failedBoth) return; // wait for the first image to load
    opened.current = true;
    if (reducedMotion()) return;
    backdropRef.current?.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 320, easing: "ease-out" });
    const img = imgRef.current;
    const tile = tileFor(item.key);
    if (img && box && tile) {
      const t = tile.getBoundingClientRect();
      if (onScreen(t)) {
        const f = img.getBoundingClientRect();
        const r = parseFloat(getComputedStyle(tile).borderRadius) || 0;
        tile.style.visibility = "hidden";
        hiddenTile.current = tile;
        const show = () => {
          if (hiddenTile.current === tile && !closing.current) {
            tile.style.visibility = "";
            hiddenTile.current = null;
          }
        };
        img
          .animate([flipFrame(f, t, true, r), flipFrame(f, f, false, 4)], { duration: 460, easing: EASE })
          .finished.then(show, show);
        return;
      }
    }
    mediaRef.current?.animate([{ opacity: 0, transform: "scale(.97)" }, { opacity: 1, transform: "none" }], {
      duration: 260,
      easing: EASE,
    });
  });

  // Moving between items slides the new one in from the side it came from.
  useLayoutEffect(() => {
    if (shownKey.current === item.key) return;
    shownKey.current = item.key;
    if (reducedMotion() || !dir.current) return;
    mediaRef.current?.animate(
      [{ opacity: 0, transform: `translateX(${dir.current * 56}px)` }, { opacity: 1, transform: "none" }],
      { duration: 320, easing: EASE },
    );
  }, [item.key]);

  // Load the full-size render, plus the neighbours' so arrowing is instant.
  useEffect(() => {
    let live = true;
    const load = (it: MediaItem, own: boolean) => {
      const url = api.thumbUrl(it, FULL);
      if (fullLoaded.has(url)) return;
      const im = new Image();
      im.onload = () => {
        fullLoaded.add(url);
        if (live && own) setFullFor(it.key);
      };
      if (own) im.onerror = () => live && setFullErr(it.key);
      im.src = url;
    };
    if (isImage) load(item, true);
    for (const j of [index - 1, index + 1]) {
      const it = items[j];
      if (it && (it.class === "photo" || it.class === "doc")) load(it, false);
    }
    return () => {
      live = false;
    };
  }, [item, index, items, isImage]);

  useEffect(() => {
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    rootRef.current?.focus({ preventScroll: true }); // keyboard focus moves into the dialog
    wake();
    return () => {
      document.body.style.overflow = prevOverflow;
      window.clearTimeout(idleTimer.current);
      window.clearTimeout(tapTimer.current);
      if (hiddenTile.current) hiddenTile.current.style.visibility = "";
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Keyboard. Re-registered every render so it always sees the current item.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (closing.current || document.querySelector(".modal-backdrop")) return;
      wake();
      if (e.key === "Escape") {
        if (viewRef.current.s > 1) applyView(FIT, true);
        else close();
        return;
      }
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "VIDEO") return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "ArrowLeft") prev();
      else if (e.key === "ArrowRight") next();
      else if (e.key === "i") toggleInfo();
      else if (isImage && (e.key === "+" || e.key === "=")) zoomAtCenter(viewRef.current.s * 1.6);
      else if (isImage && e.key === "-") zoomAtCenter(viewRef.current.s / 1.6);
      else if (isImage && e.key === "0") applyView(FIT, true);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  // Trackpad pinch / ctrl+wheel zooms; the wheel pans while zoomed. Needs a
  // non-passive listener to stop the page from zooming instead.
  const wheelRef = useRef({ zoomAt, applyView });
  wheelRef.current = { zoomAt, applyView };
  useEffect(() => {
    const st = stageRef.current;
    if (!st) return;
    const onWheel = (e: WheelEvent) => {
      if (!imgRef.current || closing.current) return;
      const v = viewRef.current;
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault();
        wheelRef.current.zoomAt(v.s * Math.exp(-clamp(e.deltaY, -50, 50) * 0.008), e.clientX, e.clientY);
      } else if (v.s > 1) {
        e.preventDefault();
        wheelRef.current.applyView({ s: v.s, x: v.x - e.deltaX, y: v.y - e.deltaY });
      }
    };
    st.addEventListener("wheel", onWheel, { passive: false });
    return () => st.removeEventListener("wheel", onWheel);
  }, []);

  // ---- pointer gestures ---------------------------------------------------------
  // Mouse: click the photo to zoom, drag to pan, click outside to close.
  // Touch: swipe sideways to move, swipe down to close, pinch or double-tap to
  // zoom, single tap to show/hide the toolbar.

  const resetSwipe = (animate: boolean) => {
    const m = mediaRef.current;
    const b = backdropRef.current;
    const anim = animate && !reducedMotion();
    if (m?.style.transform) {
      if (anim) m.animate([{ transform: m.style.transform }, { transform: "none" }], { duration: 260, easing: EASE });
      m.style.transform = "";
    }
    if (b?.style.opacity) {
      if (anim) b.animate([{ opacity: b.style.opacity }, { opacity: 1 }], { duration: 260, easing: EASE });
      b.style.opacity = "";
    }
  };

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (closing.current || (e.pointerType === "mouse" && e.button !== 0)) return;
    const target = e.target as HTMLElement;
    if (target.closest("button, a, video, .viewer-file")) return;
    pts.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    try {
      stageRef.current?.setPointerCapture(e.pointerId);
    } catch {
      /* pointer already gone */
    }
    if (pts.current.size === 2 && isImage && imgRef.current) {
      const [a, b] = [...pts.current.values()];
      const mx = (a.x + b.x) / 2;
      const my = (a.y + b.y) / 2;
      resetSwipe(false);
      gest.current = {
        sx: mx,
        sy: my,
        view: viewRef.current,
        moved: true,
        axis: "",
        onImg: true,
        type: e.pointerType,
        pinch: { dist: Math.hypot(a.x - b.x, a.y - b.y) || 1, mx, my },
      };
      setDragging(true);
      return;
    }
    if (pts.current.size > 1) return;
    gest.current = {
      sx: e.clientX,
      sy: e.clientY,
      view: viewRef.current,
      moved: false,
      axis: "",
      onImg: target === imgRef.current,
      type: e.pointerType,
    };
  };

  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (!pts.current.has(e.pointerId)) return;
    pts.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    const g = gest.current;
    if (!g) return;
    if (g.pinch) {
      if (pts.current.size < 2) return;
      const [a, b] = [...pts.current.values()];
      const s = clamp(g.view.s * (Math.hypot(a.x - b.x, a.y - b.y) / g.pinch.dist), 1, MAX_ZOOM);
      const c = layoutCenter();
      const f = 1 - s / g.view.s;
      applyView({
        s,
        x: g.view.x + (g.pinch.mx - c.x - g.view.x) * f + ((a.x + b.x) / 2 - g.pinch.mx),
        y: g.view.y + (g.pinch.my - c.y - g.view.y) * f + ((a.y + b.y) / 2 - g.pinch.my),
      });
      return;
    }
    const dx = e.clientX - g.sx;
    const dy = e.clientY - g.sy;
    if (!g.moved) {
      if (Math.hypot(dx, dy) < 8) return;
      g.moved = true;
      setDragging(true);
    }
    if (g.view.s > 1) {
      applyView({ s: g.view.s, x: g.view.x + dx, y: g.view.y + dy });
      return;
    }
    if (g.type === "mouse") return;
    if (!g.axis) g.axis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
    const m = mediaRef.current;
    if (!m) return;
    if (g.axis === "x") {
      const edge = (dx > 0 && !hasPrev) || (dx < 0 && !hasNext);
      m.style.transform = `translateX(${edge ? dx * 0.3 : dx}px)`;
    } else {
      const d = Math.max(0, dy);
      m.style.transform = `translateY(${d}px) scale(${1 - Math.min(d / 1500, 0.2)})`;
      if (backdropRef.current) backdropRef.current.style.opacity = String(1 - Math.min(d / 450, 0.75));
    }
  };

  const onTap = (e: ReactPointerEvent<HTMLDivElement>, g: Gesture) => {
    const now = performance.now();
    const last = lastTap.current;
    const quick = last && now - last.t < 300 && Math.hypot(e.clientX - last.x, e.clientY - last.y) < 30;
    if (g.type === "mouse") {
      lastTap.current = { t: now, x: e.clientX, y: e.clientY };
      if (quick) return; // a double-click is one toggle, not two
      if (g.onImg && isImage) toggleZoomAt(e.clientX, e.clientY);
      else if (viewRef.current.s > 1) applyView(FIT, true);
      else close();
      return;
    }
    window.clearTimeout(tapTimer.current);
    if (quick) {
      lastTap.current = null;
      if (isImage) toggleZoomAt(e.clientX, e.clientY);
      return;
    }
    lastTap.current = { t: now, x: e.clientX, y: e.clientY };
    tapTimer.current = window.setTimeout(() => setIdle((v) => !v), 280);
  };

  const onPointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (!pts.current.delete(e.pointerId)) return;
    const g = gest.current;
    if (g?.pinch) {
      if (pts.current.size === 0) {
        gest.current = null;
        setDragging(false);
      }
      return;
    }
    gest.current = null;
    setDragging(false);
    if (!g || e.type === "pointercancel") {
      resetSwipe(true);
      return;
    }
    const dx = e.clientX - g.sx;
    const dy = e.clientY - g.sy;
    if (!g.moved) {
      onTap(e, g);
      return;
    }
    if (g.view.s > 1 || g.type === "mouse") return;
    if (g.axis === "x" && dx > 70 && hasPrev) prev();
    else if (g.axis === "x" && dx < -70 && hasNext) next();
    else if (g.axis === "y" && dy > 110) close();
    else resetSwipe(true);
  };

  // ---- actions ------------------------------------------------------------------

  const remove = async () => {
    const ok = await ask({
      title: "Move to trash?",
      message: `"${item.name}" stays recoverable in the Google Photos trash for 60 days.`,
      confirmLabel: "Move to trash",
      danger: true,
    });
    if (!ok) return;
    setDeleting(true);
    try {
      const res = await api.post<{ deleted: string[]; failed: number }>("/api/v1/media/delete", {
        keys: [item.key],
      });
      if (!res.deleted.includes(item.key)) throw new Error("the server couldn't delete it");
      toast("Moved to trash", "ok");
      dir.current = 1;
      onDeleted(item.key);
    } catch (err) {
      toast("Delete failed: " + errMsg(err), "err");
    } finally {
      setDeleting(false);
    }
  };

  const onImgLoad = () => {
    const img = imgRef.current;
    // A 1×1 image is the server's "still rendering" placeholder: keep the
    // spinner up until the full-size image arrives.
    if (!img || img.naturalWidth <= 1) return;
    const requested = img.getAttribute("src") === fullUrl ? FULL : PREVIEW;
    setNat({
      key: item.key,
      w: img.naturalWidth,
      h: img.naturalHeight,
      // Smaller than requested means the original itself is that small.
      capped: Math.max(img.naturalWidth, img.naturalHeight) < requested - 1,
    });
  };

  const onImgError = () => {
    if (imgRef.current?.getAttribute("src") === fullUrl) setFullErr(item.key);
    else setPrevErr(item.key);
  };

  return (
    <div
      ref={rootRef}
      className={
        "viewer" +
        (idle ? " idle" : "") +
        (leaving ? " leaving" : "") +
        (zoomed ? " zoomed" : "") +
        (dragging ? " dragging" : "")
      }
      role="dialog"
      aria-modal="true"
      aria-label={item.name}
      tabIndex={-1}
      onPointerMove={(e) => {
        if (e.pointerType === "mouse") wake();
      }}
    >
      <div className="viewer-backdrop" ref={backdropRef} />

      <div className="viewer-main">
        <div
          className="viewer-stage"
          ref={stageRef}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
        >
          <div className="viewer-media" key={item.key} ref={mediaRef} style={{ padding: pad }}>
            {isImage && !failedBoth ? (
              <img
                ref={imgRef}
                src={src}
                alt={item.name}
                draggable={false}
                className={smooth && !dragging ? "smooth" : undefined}
                style={{
                  width: box?.w,
                  height: box?.h,
                  opacity: box ? 1 : 0,
                  transform: zoomed ? `translate(${view.x}px, ${view.y}px) scale(${view.s})` : undefined,
                }}
                onLoad={onImgLoad}
                onError={onImgError}
              />
            ) : item.class === "video" ? (
              <video src={api.originalUrl(item.key)} poster={api.thumbUrl(item, 1024)} controls playsInline />
            ) : (
              <div className="viewer-file">
                <Icon name={failedBoth ? "photos" : "file"} size={56} />
                <p>{item.name}</p>
                <p className="muted">
                  {failedBoth ? "Couldn't load a preview" : item.size > 0 ? formatBytes(item.size) : ""}
                </p>
                <a className="btn btn-primary" href={download} download={item.name}>
                  <Icon name="download" size={16} />
                  Download original
                </a>
              </div>
            )}
          </div>

          {isImage && !box && !failedBoth && (
            <div className="viewer-loading" aria-label="Loading">
              <div className="spinner" />
            </div>
          )}

          {hasPrev && (
            <button className="viewer-nav prev" onClick={prev} aria-label="Previous (←)" title="Previous (←)">
              <Icon name="chevronLeft" size={22} />
            </button>
          )}
          {hasNext && (
            <button className="viewer-nav next" onClick={next} aria-label="Next (→)" title="Next (→)">
              <Icon name="chevronRight" size={22} />
            </button>
          )}
        </div>

        {info && (
          <aside className="viewer-info" aria-label="Details">
            <h3>Details</h3>
            <dl>
              <dt>Name</dt>
              <dd>{item.name}</dd>
              <dt>Date</dt>
              <dd>{formatDateTime(date)}</dd>
              <dt>Type</dt>
              <dd>{kindLabel[item.class]}</dd>
              <dt>Size</dt>
              <dd>{item.size > 0 ? formatBytes(item.size) : "Unknown"}</dd>
              <dt>Storage</dt>
              <dd>
                {item.encrypted ? (
                  <>
                    <Icon name="lock" size={13} /> Encrypted by gpix
                  </>
                ) : (
                  "Not encrypted"
                )}
              </dd>
              <dt>Tags</dt>
              <dd>
                <div className="chips">
                  {(item.tags ?? []).map((t) => (
                    <span key={t} className="chip">
                      #{t}
                    </span>
                  ))}
                  <button className="chip chip-add" onClick={() => setTagging(true)}>
                    <Icon name="tag" size={12} />
                    {item.tags?.length ? "Edit" : "Add tags"}
                  </button>
                </div>
              </dd>
            </dl>
          </aside>
        )}
      </div>

      <div className="viewer-bar">
        <button className="btn btn-ghost btn-icon" onClick={close} aria-label="Close (Esc)" title="Close (Esc)">
          <Icon name="x" />
        </button>
        <div className="viewer-title">
          <strong title={item.name}>{item.name}</strong>
          <span className="num">
            {date ? `${formatDateTime(date)} · ` : ""}
            {index + 1} of {items.length}
          </span>
        </div>
        <div className="spacer" />
        {isImage && !failedBoth && (
          <button
            className={"btn btn-ghost btn-icon" + (zoomed ? " on" : "")}
            onClick={() => (zoomed ? applyView(FIT, true) : zoomAtCenter(2.5))}
            aria-label={zoomed ? "Fit to screen (0)" : "Zoom in (+)"}
            title={zoomed ? "Fit to screen (0)" : "Zoom in (+)"}
          >
            <Icon name={zoomed ? "zoomOut" : "zoomIn"} />
          </button>
        )}
        <button
          className={"btn btn-ghost btn-icon" + (info ? " on" : "")}
          onClick={toggleInfo}
          aria-pressed={info}
          aria-label="Details (i)"
          title="Details (i)"
        >
          <Icon name="info" />
        </button>
        <button className="btn btn-ghost btn-icon" onClick={() => setSharing(true)} aria-label="Share" title="Share">
          <Icon name="share" />
        </button>
        <a
          className="btn btn-ghost btn-icon"
          href={download}
          download={item.name}
          aria-label="Download original"
          title="Download original"
        >
          <Icon name="download" />
        </a>
        <button
          className="btn btn-ghost btn-icon danger"
          onClick={remove}
          disabled={deleting}
          aria-label="Move to trash"
          title="Move to trash"
        >
          <Icon name="trash" />
        </button>
      </div>

      {sharing && <ShareDialog keys={[item.key]} defaultTitle={item.name} onClose={() => setSharing(false)} />}
      {tagging && (
        <TagDialog
          keys={[item.key]}
          initial={item.tags ?? []}
          onClose={() => setTagging(false)}
          onDone={(t) => onTagsChanged?.(item.key, t[item.key] ?? [])}
        />
      )}
    </div>
  );
}
