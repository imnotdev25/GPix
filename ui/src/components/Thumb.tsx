import { useEffect, useRef, useState } from "react";
import type { SyntheticEvent } from "react";
import { thumbUrl } from "../api";
import type { MediaItem } from "../types";
import Icon from "./Icon";

// While gpix is still rendering an encrypted item's thumbnail the server sends
// a 1×1 placeholder. Retry on this schedule until the real image arrives.
const RETRY_MS = [1500, 2500, 4000, 6000, 9000, 13000, 20000, 30000];

/** Thumb renders a media thumbnail that fades in once decoded. It only
 *  re-requests while it is still showing a placeholder, so a thumbnail that
 *  has loaded never reloads (no flicker). */
export default function Thumb({ item, size = 512 }: { item: MediaItem; size?: number }) {
  const [bust, setBust] = useState(0);
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const timer = useRef(0);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const retry = () => {
    if (bust >= RETRY_MS.length) {
      setWaiting(false);
      setFailed(true);
      return;
    }
    setWaiting(true);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setBust((b) => b + 1), RETRY_MS[bust]);
  };

  const onLoad = (e: SyntheticEvent<HTMLImageElement>) => {
    if (e.currentTarget.naturalWidth <= 1) {
      retry(); // gpix is still rendering it
      return;
    }
    setWaiting(false);
    setLoaded(true);
  };

  const onError = () => {
    if (item.encrypted && bust < RETRY_MS.length) retry();
    else setFailed(true);
  };

  if (failed || item.class === "file") {
    const dot = item.name.lastIndexOf(".");
    const ext = dot > 0 ? item.name.slice(dot + 1, dot + 6).toUpperCase() : "";
    const icon =
      item.class === "video"
        ? "video"
        : item.class === "doc"
          ? "doc"
          : item.class === "photo"
            ? "photos"
            : "file";
    return (
      <div className="thumb-fallback">
        <Icon name={icon} size={30} />
        {ext && <span>{ext}</span>}
      </div>
    );
  }

  return (
    <>
      <img
        src={thumbUrl(item, size, bust)}
        alt=""
        loading="lazy"
        decoding="async"
        draggable={false}
        className={loaded ? "loaded" : undefined}
        onLoad={onLoad}
        onError={onError}
      />
      {waiting && <span className="thumb-wait" aria-hidden="true" />}
    </>
  );
}
