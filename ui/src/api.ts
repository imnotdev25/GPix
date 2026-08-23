// Thin client for the gpix /api/v1 JSON API. The SPA is same-origin and
// authenticates with the session cookie, so every call sends credentials.

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function handle<T>(res: Response): Promise<T> {
  if (res.ok) {
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }
  let msg = `HTTP ${res.status}`;
  try {
    const body = await res.json();
    if (body?.error) msg = body.error;
  } catch {
    /* non-JSON error body */
  }
  throw new ApiError(res.status, msg);
}

const base: RequestInit = { credentials: "same-origin" };

export function get<T>(path: string): Promise<T> {
  return fetch(path, base).then((r) => handle<T>(r));
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return fetch(path, {
    ...base,
    method: "POST",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  }).then((r) => handle<T>(r));
}

export function del<T>(path: string): Promise<T> {
  return fetch(path, { ...base, method: "DELETE" }).then((r) => handle<T>(r));
}

export function uploadFiles(
  files: File[],
  quality: string,
  onProgress?: (pct: number) => void,
): Promise<{ results: { name: string; status: string; key: string }[] }> {
  // XHR (not fetch) so we get upload progress events.
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.append("quality", quality);
    for (const f of files) form.append("files", f, f.name);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/v1/upload");
    xhr.withCredentials = true;
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100));
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch {
          reject(new ApiError(xhr.status, "bad response"));
        }
      } else {
        let msg = `HTTP ${xhr.status}`;
        try {
          const b = JSON.parse(xhr.responseText);
          if (b?.error) msg = b.error;
        } catch {
          /* ignore */
        }
        reject(new ApiError(xhr.status, msg));
      }
    };
    xhr.onerror = () => reject(new ApiError(0, "network error"));
    xhr.send(form);
  });
}

/** Thumbnail URL; encrypted items hint their name so the server can render
 *  the real (decrypted) thumbnail without relying on a warm library cache. */
export function thumbUrl(
  item: { key: string; name: string; encrypted: boolean },
  size = 512,
  bust = 0,
): string {
  let u = `/api/v1/media/${encodeURIComponent(item.key)}/thumb?size=${size}`;
  if (item.encrypted) u += `&enc=1&name=${encodeURIComponent(item.name)}`;
  if (bust) u += `&_r=${bust}`;
  return u;
}

export function originalUrl(key: string, download = false): string {
  return `/api/v1/media/${encodeURIComponent(key)}/original${download ? "?download=1" : ""}`;
}
