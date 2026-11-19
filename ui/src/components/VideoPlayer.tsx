import { useEffect, useRef, useState } from "react";
import * as api from "../api";
import type { MediaItem } from "../types";
import Icon from "./Icon";
import { useToast } from "./feedback";

// The parts of hls.js (served at /static/hls.min.js) this player uses.
type HlsLevel = { height: number; bitrate: number };
type HlsInstance = {
  levels: HlsLevel[];
  currentLevel: number;
  loadSource(url: string): void;
  attachMedia(video: HTMLVideoElement): void;
  on(event: string, cb: (event: string, data: { fatal?: boolean }) => void): void;
  destroy(): void;
};
type HlsClass = {
  new (config?: object): HlsInstance;
  isSupported(): boolean;
  Events: { MANIFEST_PARSED: string; ERROR: string };
};

let hlsLoading: Promise<HlsClass | null> | null = null;

/** Loads hls.js once, the first time a video is opened. */
function loadHls(): Promise<HlsClass | null> {
  const w = window as unknown as { Hls?: HlsClass };
  if (w.Hls) return Promise.resolve(w.Hls);
  if (!hlsLoading) {
    hlsLoading = new Promise((resolve) => {
      const s = document.createElement("script");
      s.src = "/static/hls.min.js";
      s.onload = () => resolve(w.Hls ?? null);
      s.onerror = () => {
        hlsLoading = null;
        resolve(null);
      };
      document.head.appendChild(s);
    });
  }
  return hlsLoading;
}

/** Plays a video. Google-hosted videos stream with HLS (adaptive quality, a
 *  manual quality picker and a stream link for VLC); encrypted ones play from
 *  the decrypted original. Any streaming failure falls back to the original. */
export default function VideoPlayer({ item }: { item: MediaItem }) {
  const ref = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<HlsInstance | null>(null);
  const [levels, setLevels] = useState<HlsLevel[]>([]);
  const [level, setLevel] = useState(-1);
  const [streamUrl, setStreamUrl] = useState("");
  const toast = useToast();
  const original = api.originalUrl(item.key);

  useEffect(() => {
    const v = ref.current;
    if (!v) return;
    let cancelled = false;
    setLevels([]);
    setLevel(-1);
    setStreamUrl("");

    const playOriginal = () => {
      hlsRef.current?.destroy();
      hlsRef.current = null;
      setLevels([]);
      v.src = original;
    };

    if (item.encrypted) {
      v.src = original;
    } else {
      (async () => {
        let s: { hls: string; hlsAbsolute: string };
        try {
          s = await api.get(`/api/v1/media/${encodeURIComponent(item.key)}/stream`);
        } catch {
          if (!cancelled) playOriginal();
          return;
        }
        if (cancelled) return;
        setStreamUrl(s.hlsAbsolute);
        const Hls = await loadHls();
        if (cancelled) return;
        if (Hls?.isSupported()) {
          const h = new Hls({ capLevelToPlayerSize: true });
          hlsRef.current = h;
          h.on(Hls.Events.MANIFEST_PARSED, () => {
            if (!cancelled) setLevels([...h.levels]);
          });
          h.on(Hls.Events.ERROR, (_e, d) => {
            if (d.fatal && !cancelled) playOriginal();
          });
          h.loadSource(s.hls);
          h.attachMedia(v);
        } else if (v.canPlayType("application/vnd.apple.mpegurl")) {
          // Safari plays HLS itself.
          v.onerror = () => {
            v.onerror = null;
            if (!cancelled) v.src = original;
          };
          v.src = s.hls;
        } else {
          playOriginal();
        }
      })();
    }

    return () => {
      cancelled = true;
      hlsRef.current?.destroy();
      hlsRef.current = null;
      v.onerror = null;
      v.removeAttribute("src");
      v.load();
    };
  }, [item.key, item.encrypted, original]);

  const pick = (i: number) => {
    setLevel(i);
    if (hlsRef.current) hlsRef.current.currentLevel = i;
  };

  const copyStream = async () => {
    try {
      await navigator.clipboard.writeText(streamUrl);
      toast("Stream link copied. In VLC: Media → Open Network Stream.", "ok");
    } catch {
      toast("Couldn't copy the link", "err");
    }
  };

  return (
    <div className="video-wrap">
      <video ref={ref} poster={api.thumbUrl(item, 1024)} controls playsInline />
      {(levels.length > 1 || streamUrl) && (
        <div className="video-tools" onPointerDown={(e) => e.stopPropagation()}>
          {levels.length > 1 && (
            <select
              className="video-quality"
              value={level}
              onChange={(e) => pick(Number(e.target.value))}
              aria-label="Video quality"
            >
              <option value={-1}>Auto</option>
              {levels.map((l, i) => (
                <option key={i} value={i}>
                  {l.height ? `${l.height}p` : `${Math.round(l.bitrate / 1000)} kbps`}
                </option>
              ))}
            </select>
          )}
          {streamUrl && (
            <button className="btn btn-sm video-btn" onClick={copyStream} title="Copy a stream link for VLC">
              <Icon name="copy" size={14} />
              Stream link
            </button>
          )}
        </div>
      )}
    </div>
  );
}
