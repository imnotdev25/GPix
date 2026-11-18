import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import * as api from "../api";
import type { Album, TagSummary } from "../types";
import Icon from "./Icon";
import { Modal, errMsg, useToast } from "./feedback";

/** Put items into an existing album or a new one. */
export function AddToAlbumDialog({
  keys,
  onClose,
  onDone,
}: {
  keys: string[];
  onClose: () => void;
  onDone?: () => void;
}) {
  const [albums, setAlbums] = useState<Album[] | null>(null);
  const [choice, setChoice] = useState<number | "new">("new");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const toast = useToast();

  useEffect(() => {
    api
      .get<{ albums: Album[] }>("/api/v1/albums")
      .then((r) => {
        const list = r.albums ?? [];
        setAlbums(list);
        if (list.length) setChoice(list[0].id);
      })
      .catch((err) => {
        setAlbums([]);
        setError(errMsg(err));
      });
  }, []);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (choice === "new" && !name.trim()) {
      setError("Give the new album a name.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const a =
        choice === "new"
          ? await api.post<Album>("/api/v1/albums", { name: name.trim(), keys })
          : await api.post<Album>(`/api/v1/albums/${choice}/items`, { keys });
      toast(`Added ${keys.length === 1 ? "1 item" : `${keys.length} items`} to ${a.name}`, "ok");
      onDone?.();
      onClose();
    } catch (err) {
      setError(errMsg(err));
      setBusy(false);
    }
  };

  return (
    <Modal
      title={keys.length === 1 ? "Add to album" : `Add ${keys.length} items to an album`}
      onClose={onClose}
      footer={
        <>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" type="submit" form="album-form" disabled={busy || albums === null}>
            {busy ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      <form id="album-form" onSubmit={submit}>
        {albums === null ? (
          <div className="center" style={{ minHeight: 80 }}>
            <div className="spinner" />
          </div>
        ) : (
          <div className="pick-list" role="radiogroup" aria-label="Album">
            {albums.map((a) => (
              <button
                key={a.id}
                type="button"
                role="radio"
                aria-checked={choice === a.id}
                className={"pick" + (choice === a.id ? " on" : "")}
                onClick={() => setChoice(a.id)}
              >
                <span className="pick-dot" />
                <span className="pick-name">{a.name}</span>
                <span className="muted num">{a.count}</span>
              </button>
            ))}
            <button
              type="button"
              role="radio"
              aria-checked={choice === "new"}
              className={"pick" + (choice === "new" ? " on" : "")}
              onClick={() => setChoice("new")}
            >
              <span className="pick-dot" />
              <span className="pick-name">New album…</span>
            </button>
          </div>
        )}
        {choice === "new" && (
          <label className="field">
            <span>Album name</span>
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={100}
              placeholder="e.g. Summer 2026"
              autoFocus
            />
          </label>
        )}
        {error && <div className="banner banner-err">{error}</div>}
      </form>
    </Modal>
  );
}

/** Add tags to items; with `initial` (one item) it also lists and removes tags. */
export function TagDialog({
  keys,
  initial,
  onClose,
  onDone,
}: {
  keys: string[];
  initial?: string[];
  onClose: () => void;
  onDone?: (tags: Record<string, string[]>) => void;
}) {
  const single = initial !== undefined;
  const [tags, setTags] = useState<string[]>(initial ?? []);
  const [text, setText] = useState("");
  const [known, setKnown] = useState<TagSummary[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const toast = useToast();

  useEffect(() => {
    api
      .get<{ tags: TagSummary[] }>("/api/v1/tags")
      .then((r) => setKnown(r.tags ?? []))
      .catch(() => {});
  }, []);

  const parse = (s: string) =>
    s
      .split(",")
      .map((t) => t.trim().replace(/^#+/, "").replace(/\s+/g, " ").toLowerCase())
      .filter(Boolean);

  const send = async (op: "add" | "remove", list: string[]) => {
    const res = await api.post<{ tags: Record<string, string[]> }>(`/api/v1/tags/${op}`, { keys, tags: list });
    const out: Record<string, string[]> = {};
    for (const k of keys) out[k] = res.tags?.[k] ?? [];
    onDone?.(out);
    if (keys.length === 1) setTags(out[keys[0]]);
  };

  const add = async (list: string[]) => {
    if (!list.length) return;
    setBusy(true);
    setError("");
    try {
      await send("add", list);
      setText("");
      if (!single) {
        toast(`Tagged ${keys.length} items`, "ok");
        onClose();
      }
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (t: string) => {
    setBusy(true);
    setError("");
    try {
      await send("remove", [t]);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const suggestions = known
    .map((k) => k.tag)
    .filter((t) => !tags.includes(t))
    .slice(0, 12);

  return (
    <Modal
      title={single ? "Tags" : `Tag ${keys.length} items`}
      onClose={onClose}
      footer={
        <>
          <button className="btn" type="button" onClick={onClose}>
            {single ? "Done" : "Cancel"}
          </button>
          <button className="btn btn-primary" type="submit" form="tag-form" disabled={busy || !text.trim()}>
            {busy ? "Saving…" : "Add"}
          </button>
        </>
      }
    >
      {single &&
        (tags.length ? (
          <div className="chips" style={{ marginBottom: 14 }}>
            {tags.map((t) => (
              <span className="chip" key={t}>
                #{t}
                <button
                  className="chip-x"
                  type="button"
                  onClick={() => remove(t)}
                  disabled={busy}
                  aria-label={`Remove tag ${t}`}
                >
                  <Icon name="x" size={12} />
                </button>
              </span>
            ))}
          </div>
        ) : (
          <p className="modal-text">No tags yet.</p>
        ))}
      <form
        id="tag-form"
        onSubmit={(e) => {
          e.preventDefault();
          add(parse(text));
        }}
      >
        <label className="field">
          <span>Add tags, separated by commas</span>
          <input
            className="input"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="beach, family"
            autoComplete="off"
            autoFocus
          />
        </label>
      </form>
      {suggestions.length > 0 && (
        <div className="field">
          <span>Your tags</span>
          <div className="chips">
            {suggestions.map((t) => (
              <button key={t} type="button" className="chip" onClick={() => add([t])} disabled={busy}>
                #{t}
              </button>
            ))}
          </div>
        </div>
      )}
      {error && <div className="banner banner-err">{error}</div>}
    </Modal>
  );
}
