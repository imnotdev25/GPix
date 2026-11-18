// In-app replacements for the browser's alert/confirm/prompt: a Modal, a
// promise-based confirm dialog (useConfirm) and toast messages (useToast).
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import Icon from "./Icon";

export function errMsg(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function Modal({
  title,
  onClose,
  children,
  footer,
  sheet = false,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  /** On phones, slide up from the bottom instead of centering. */
  sheet?: boolean;
}) {
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    // Capture phase so Esc closes only the dialog, not the viewer behind it.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        closeRef.current();
      }
    };
    window.addEventListener("keydown", onKey, true);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey, true);
      document.body.style.overflow = prevOverflow;
    };
  }, []);

  return createPortal(
    <div
      className={"modal-backdrop" + (sheet ? " sheet" : "")}
      onClick={(e) => {
        // Portals still bubble React events to their owner (e.g. the viewer).
        e.stopPropagation();
        if (e.target === e.currentTarget) closeRef.current();
      }}
    >
      <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-head">
          <h2>{title}</h2>
          <button className="btn btn-ghost btn-icon" onClick={onClose} aria-label="Close">
            <Icon name="x" />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

// ---- confirm -----------------------------------------------------------------

export type ConfirmOptions = {
  title: string;
  message?: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  /** Require typing this word before the confirm button enables. */
  typeToConfirm?: string;
};

type ConfirmRequest = ConfirmOptions & { resolve: (ok: boolean) => void };

const ConfirmContext = createContext<(o: ConfirmOptions) => Promise<boolean>>(() =>
  Promise.resolve(false),
);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [req, setReq] = useState<ConfirmRequest | null>(null);
  const [typed, setTyped] = useState("");

  const confirm = useCallback(
    (o: ConfirmOptions) =>
      new Promise<boolean>((resolve) => {
        setTyped("");
        setReq({ ...o, resolve });
      }),
    [],
  );

  const finish = (ok: boolean) => {
    req?.resolve(ok);
    setReq(null);
  };
  const ready = !req?.typeToConfirm || typed === req.typeToConfirm;

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {req && (
        <Modal
          title={req.title}
          onClose={() => finish(false)}
          footer={
            <>
              <button className="btn" onClick={() => finish(false)}>
                Cancel
              </button>
              <button
                className={"btn " + (req.danger ? "btn-danger" : "btn-primary")}
                disabled={!ready}
                onClick={() => finish(true)}
                autoFocus={!req.typeToConfirm}
              >
                {req.confirmLabel ?? "Confirm"}
              </button>
            </>
          }
        >
          {req.message && <p className="modal-text">{req.message}</p>}
          {req.typeToConfirm && (
            <label className="field">
              <span>
                Type <code>{req.typeToConfirm}</code> to confirm
              </span>
              <input
                className="input"
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && ready) finish(true);
                }}
                autoFocus
                autoComplete="off"
                spellCheck={false}
              />
            </label>
          )}
        </Modal>
      )}
    </ConfirmContext.Provider>
  );
}

export const useConfirm = () => useContext(ConfirmContext);

// ---- toasts ------------------------------------------------------------------

export type ToastKind = "ok" | "err" | "info";
type ToastItem = { id: number; text: string; kind: ToastKind };

const ToastContext = createContext<(text: string, kind?: ToastKind) => void>(() => {});
let toastSeq = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const dismiss = useCallback(
    (id: number) => setToasts((list) => list.filter((t) => t.id !== id)),
    [],
  );
  const toast = useCallback(
    (text: string, kind: ToastKind = "info") => {
      const id = ++toastSeq;
      setToasts((list) => [...list.slice(-3), { id, text, kind }]);
      window.setTimeout(() => dismiss(id), kind === "err" ? 7000 : 4000);
    },
    [dismiss],
  );

  return (
    <ToastContext.Provider value={toast}>
      {children}
      {createPortal(
        <div className="toasts" aria-live="polite">
          {toasts.map((t) => (
            <div key={t.id} className={"toast " + t.kind} role={t.kind === "err" ? "alert" : "status"}>
              <Icon name={t.kind === "err" ? "info" : "check"} size={16} />
              <span>{t.text}</span>
              <button
                className="btn btn-ghost btn-icon btn-sm"
                onClick={() => dismiss(t.id)}
                aria-label="Dismiss"
              >
                <Icon name="x" size={14} />
              </button>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

export const useToast = () => useContext(ToastContext);
