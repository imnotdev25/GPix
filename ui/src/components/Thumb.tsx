import { useEffect, useState } from "react";
import { thumbUrl } from "../api";
import type { MediaItem } from "../types";

/** Thumb renders a media thumbnail. For encrypted photos/documents the server
 *  renders the real thumbnail in the background (Google only holds a blank
 *  placeholder), so this re-requests a few times until it resolves. */
export default function Thumb({ item, size = 512 }: { item: MediaItem; size?: number }) {
  const [bust, setBust] = useState(0);
  const [failed, setFailed] = useState(false);

  const mayBePending =
    item.encrypted && (item.class === "photo" || item.class === "doc");

  useEffect(() => {
    if (!mayBePending) return;
    const timers = [3000, 8000, 18000, 40000].map((d) =>
      window.setTimeout(() => setBust((b) => b + 1), d),
    );
    return () => timers.forEach(clearTimeout);
  }, [mayBePending, item.key]);

  if (failed || item.class === "file" || item.class === "video") {
    // Videos and plain files have no useful still to show inline.
    if (item.class === "video" && !failed) {
      return (
        <img
          src={thumbUrl(item, size, bust)}
          alt={item.name}
          loading="lazy"
          onError={() => setFailed(true)}
        />
      );
    }
    return (
      <div className="center" style={{ minHeight: 0, height: "100%" }}>
        <span style={{ fontSize: 26, opacity: 0.6 }}>
          {item.class === "video" ? "▶" : "📄"}
        </span>
      </div>
    );
  }

  return (
    <img
      src={thumbUrl(item, size, bust)}
      alt={item.name}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}
