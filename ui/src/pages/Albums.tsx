import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import * as api from "../api";
import type { Album, TagSummary } from "../types";
import Icon from "../components/Icon";
import Thumb from "../components/Thumb";
import { Modal, errMsg, useConfirm, useToast } from "../components/feedback";
import Library from "./Library";

function NameDialog({
  title,
  initial = "",
  action,
  onSave,
  onClose,
}: {
  title: string;
  initial?: string;
  action: string;
  onSave: (name: string) => Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    setError("");
    try {
      await onSave(name.trim());
    } catch (err) {
      setError(errMsg(err));
      setBusy(false);
    }
  };

  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" type="submit" form="name-form" disabled={busy || !name.trim()}>
            {busy ? "Saving…" : action}
          </button>
        </>
      }
    >
      <form id="name-form" onSubmit={submit}>
        <label className="field">
          <span>Name</span>
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={100}
            autoFocus
          />
        </label>
        {error && <div className="banner banner-err">{error}</div>}
      </form>
    </Modal>
  );
}

/** The albums and tags made in gpix. */
export default function Albums() {
  const [albums, setAlbums] = useState<Album[] | null>(null);
  const [tags, setTags] = useState<TagSummary[]>([]);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    api
      .get<{ albums: Album[] }>("/api/v1/albums")
      .then((r) => setAlbums(r.albums ?? []))
      .catch((err) => {
        setAlbums([]);
        setError(errMsg(err));
      });
    api
      .get<{ tags: TagSummary[] }>("/api/v1/tags")
      .then((r) => setTags(r.tags ?? []))
      .catch(() => {});
  }, []);

  return (
    <>
      <div className="page-head row">
        <div>
          <h1>Albums</h1>
          <p className="sub">
            Albums and tags you make in gpix. They're stored by gpix and aren't synced with albums in
            Google Photos.
          </p>
        </div>
        <div className="spacer" />
        <button className="btn btn-primary" onClick={() => setCreating(true)}>
          <Icon name="album" size={16} />
          New album
        </button>
      </div>

      {error && <div className="banner banner-err">{error}</div>}

      {albums === null ? (
        <div className="center" style={{ minHeight: 160 }}>
          <div className="spinner" />
        </div>
      ) : albums.length === 0 ? (
        <div className="empty">
          <Icon name="album" size={40} />
          <h2>No albums yet</h2>
          <p className="muted">Select photos in the Library and choose Album, or make an empty album here.</p>
        </div>
      ) : (
        <div className="album-grid">
          {albums.map((a) => (
            <Link key={a.id} to={`/albums/${a.id}`} className="album-card">
              <div className="album-cover">
                {a.cover ? <Thumb item={a.cover} /> : <Icon name="album" size={32} />}
              </div>
              <div className="album-name">{a.name}</div>
              <div className="album-count num">{a.count === 1 ? "1 item" : `${a.count.toLocaleString()} items`}</div>
            </Link>
          ))}
        </div>
      )}

      {tags.length > 0 && (
        <section>
          <h2>Tags</h2>
          <div className="chips">
            {tags.map((t) => (
              <Link key={t.tag} className="chip" to={`/tags/${encodeURIComponent(t.tag)}`}>
                #{t.tag} <span className="muted num">{t.count}</span>
              </Link>
            ))}
          </div>
        </section>
      )}

      {creating && (
        <NameDialog
          title="New album"
          action="Create album"
          onClose={() => setCreating(false)}
          onSave={async (name) => {
            const a = await api.post<Album>("/api/v1/albums", { name });
            navigate(`/albums/${a.id}`);
          }}
        />
      )}
    </>
  );
}

/** One album: the library grid limited to its items. */
export function AlbumPage() {
  const { id = "" } = useParams();
  const [album, setAlbum] = useState<Album | null>(null);
  const [error, setError] = useState("");
  const [renaming, setRenaming] = useState(false);
  const ask = useConfirm();
  const toast = useToast();
  const navigate = useNavigate();

  useEffect(() => {
    setAlbum(null);
    setError("");
    api
      .get<Album>(`/api/v1/albums/${encodeURIComponent(id)}`)
      .then(setAlbum)
      .catch((err) => setError(errMsg(err)));
  }, [id]);

  if (error) {
    return (
      <div className="empty">
        <Icon name="album" size={40} />
        <h2>Album not found</h2>
        <p className="muted">{error}</p>
        <Link className="btn" to="/albums">
          Back to albums
        </Link>
      </div>
    );
  }
  if (!album) {
    return (
      <div className="center" style={{ minHeight: 160 }}>
        <div className="spinner" />
      </div>
    );
  }

  const remove = async () => {
    const ok = await ask({
      title: `Delete "${album.name}"?`,
      message: "Only the album goes. The photos in it stay in your library.",
      confirmLabel: "Delete album",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/api/v1/albums/${album.id}`);
      toast("Album deleted", "ok");
      navigate("/albums");
    } catch (err) {
      toast("Delete failed: " + errMsg(err), "err");
    }
  };

  return (
    <>
      <Library
        key={album.id}
        filter={{ album: album.id }}
        title={album.name}
        back={{ to: "/albums", label: "Albums" }}
        actions={
          <>
            <button
              className="btn btn-icon"
              onClick={() => setRenaming(true)}
              aria-label="Rename album"
              title="Rename album"
            >
              <Icon name="edit" size={16} />
            </button>
            <button className="btn btn-icon danger" onClick={remove} aria-label="Delete album" title="Delete album">
              <Icon name="trash" size={16} />
            </button>
          </>
        }
        empty={
          <div className="empty">
            <Icon name="album" size={40} />
            <h2>This album is empty</h2>
            <p className="muted">In the Library, select photos and choose Album to add them here.</p>
            <Link className="btn btn-primary" to="/library">
              <Icon name="photos" size={16} />
              Go to Library
            </Link>
          </div>
        }
      />
      {renaming && (
        <NameDialog
          title="Rename album"
          initial={album.name}
          action="Rename"
          onClose={() => setRenaming(false)}
          onSave={async (name) => {
            const a = await api.post<Album>(`/api/v1/albums/${album.id}`, { name });
            setAlbum(a);
            setRenaming(false);
          }}
        />
      )}
    </>
  );
}

/** Everything carrying one tag. */
export function TagPage() {
  const { tag = "" } = useParams();
  return (
    <Library
      key={tag}
      filter={{ tag }}
      title={`#${tag}`}
      back={{ to: "/albums", label: "Albums" }}
      empty={
        <div className="empty">
          <Icon name="tag" size={40} />
          <h2>Nothing tagged #{tag}</h2>
          <p className="muted">Items show up here once they carry this tag.</p>
        </div>
      }
    />
  );
}
