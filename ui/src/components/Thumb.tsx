import { useEffect, useState } from "react";
import { thumbUrl } from "../api";
import type { MediaItem } from "../types";
import Icon from "./Icon";

/** Thumb renders a media thumbnail that fades in once decoded. For encrypted
 *  photos/documents the server renders the real thumbnail in the background
 *  (Google only holds a blank placeholder), so this re-requests a few times
 *  until it resolves. */
export default function Thumb({ item, size = 512 }: { item: MediaItem; size?: number }) {
  const [bust, setBust] = useState(0);
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const mayBePending = item.encrypted && (item.class === "photo" || item.class === "doc");

  useEffect(() => {
    if (!mayBePending) return;
    const timers = [3000, 8000, 18000, 40000].map((d) =>
      window.setTimeout(() => setBust((b) => b + 1), d),
    );
    return () => timers.forEach(clearTimeout);
  }, [mayBePending, item.key]);

  // A retry may succeed where the first request failed.
  useEffect(() => {
    if (bust) setFailed(false);
  }, [bust]);

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
    <img
      src={thumbUrl(item, size, bust)}
      alt=""
      loading="lazy"
      decoding="async"
      draggable={false}
      className={loaded ? "loaded" : undefined}
      onLoad={() => setLoaded(true)}
      onError={() => setFailed(true)}
    />
  );
}
