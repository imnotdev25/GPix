import { useEffect } from "react";
import * as api from "../api";
import type { MediaItem } from "../types";

export default function Viewer({
  items,
  index,
  onClose,
  onIndex,
  onDeleted,
}: {
  items: MediaItem[];
  index: number;
  onClose: () => void;
  onIndex: (i: number) => void;
  onDeleted: () => void;
}) {
  const item = items[index];

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowLeft" && index > 0) onIndex(index - 1);
      if (e.key === "ArrowRight" && index < items.length - 1) onIndex(index + 1);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [index, items.length, onClose, onIndex]);

  const remove = async () => {
    if (!confirm(`Move "${item.name}" to trash?`)) return;
    try {
      await api.del(`/api/v1/media/${encodeURIComponent(item.key)}`);
      onDeleted();
    } catch (err) {
      alert("Delete failed: " + (err instanceof Error ? err.message : err));
    }
  };

  const isImage = item.class === "photo" || item.class === "doc";

  return (
    <div className="viewer" onClick={onClose}>
      <div className="viewer-bar" onClick={(e) => e.stopPropagation()}>
        <strong style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {item.name}
        </strong>
        <span className="muted">
          {index + 1} / {items.length}
        </span>
        <div className="spacer" />
        <a className="btn" href={api.originalUrl(item.key, true)} download={item.name}>
          Download
        </a>
        <button className="btn btn-danger" onClick={remove}>
          Delete
        </button>
        <button className="btn" onClick={onClose}>
          Close
        </button>
      </div>

      <div className="viewer-body" onClick={(e) => e.stopPropagation()}>
        {isImage ? (
          // Large render; encrypted items are decrypted server-side on demand.
          <img src={api.thumbUrl(item, 1536)} alt={item.name} />
        ) : item.class === "video" ? (
          <video src={api.originalUrl(item.key)} controls style={{ maxWidth: "100%", maxHeight: "100%" }} />
        ) : (
          <div style={{ textAlign: "center", color: "#fff" }}>
            <div style={{ fontSize: 48 }}>📄</div>
            <p>{item.name}</p>
            <a className="btn" href={api.originalUrl(item.key, true)} download={item.name}>
              Download original
            </a>
          </div>
        )}
      </div>

      {index > 0 && (
        <button
          className="viewer-nav prev"
          onClick={(e) => {
            e.stopPropagation();
            onIndex(index - 1);
          }}
        >
          ‹
        </button>
      )}
      {index < items.length - 1 && (
        <button
          className="viewer-nav next"
          onClick={(e) => {
            e.stopPropagation();
            onIndex(index + 1);
          }}
        >
          ›
        </button>
      )}
    </div>
  );
}
