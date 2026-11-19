# gpix

Your Google Photos library, on your terms.

gpix is a self-hosted Google Photos client written in Go. One binary, several ways to use it:

- **Web UI** — browse a fast photo grid with a date scrubber, albums and tags, view and zoom, stream videos with seek, upload from your browser, select to share, download or delete. Dark/light theme, fully responsive. All assets embedded.
- **S3-compatible gateway** — point `aws`, `mc`, `boto3`, `rclone`, or any S3 client at your library. AWS Signature V4 auth, keys generated and rotated from the web UI.
- **WebDAV gateway** — mount your library in Finder, Windows Explorer, or `rclone`. Basic auth with your account's email and password, or a revocable app password.
- **Client-side encryption** — optionally encrypt every upload with a gpix-managed key before it leaves your machine. Google stores only opaque video; no one without your key can see your media. Inside gpix, encrypted photos still display as photos (decrypted on the fly, with thumbnails gpix renders itself).
- **Password-protected sharing** — create expiring, optionally password-protected public links to a single item or a whole **gallery** of selected items, with a direct download link. Encrypted items are decrypted server-side for the recipient.
- **Accounts and single sign-on** — the first visitor creates an account in the web UI; sign-up closes once `GPIX_MAX_USERS` accounts exist. Optional Logto/OIDC login with an email allowlist.
- **Import** — copy a library in from another Google account (linked, or by its `GP_AUTH_DATA`) or from an existing Immich server, with live progress and resumable, dedup'd runs.
- **Cross-platform app + JSON API** — a `/api/v1` JSON API (Bearer-token auth) powers a **Flutter app** for Android, iOS, Windows, Linux, and macOS: browse, albums and tags, full-screen view, upload/back up, share, and imports. Material 3 UI, one codebase. See [`app/`](app).
- **CLI** — upload files or whole folders from the terminal.
- **Telegram bot** — `/upload`, `/get`, `/list`, `/info` against your library from any chat (optional, off by default; enable with `TG_BOT_ENABLED=true`).
- **Universal file storage** — PDFs, archives, executables, any file gets transparently wrapped as a 1-second MP4 with the original bytes preserved. Uploaded as a "video", recovered byte-identical on download. Text/markdown documents additionally render to a readable **image** in the UI while staying downloadable as the original. Effectively unlimited cloud storage for arbitrary files.

gpix talks the mobile Google Photos protocol directly, which means **uploads count as original quality without consuming your storage quota** (Pixel device profile). Everything else — dedup, video streaming, thumbnails — is wired into the same fast path.

---

## Why

The official Google Photos app and web UI are great for casual use. They're less great when you want to:

- Bulk-upload photos from a server or VPS without a phone
- Stream a 4 GB video to VLC with proper seeking
- Use Google Photos as backing storage for anything that isn't a photo or video
- Drive uploads from a script or a Telegram chat
- Self-host a clean, fast UI for your own library without trusting a third party

gpix does those.

---

## Quick start

```bash
go build .      # or: go run .
```

On first run gpix wants:

1. A Google Photos `auth_data` string in `.env` (see [Auth](#auth-getting-gp_auth_data) below).
2. That's it for the web UI: open it and create your account (see [Web UI](#web-ui)).
3. Telegram bot credentials plus `TG_BOT_ENABLED=true` in `.env` if you want the bot (see [Telegram bot](#telegram-bot)).

Then:

```bash
go run .                        # web UI + gateways (+ bot when TG_BOT_ENABLED=true)
go run . -mode web              # web UI and gateways only
go run . -mode bot              # bot only (needs TG_BOT_ENABLED=true)
go run . -cli ./photos          # CLI uploader
go run . -hashpw                # bcrypt hash, only for older GPIX_PASSWORD_HASH setups
```

Web UI default: binds `0.0.0.0:8080`, reachable at `http://<your-host>:8080` (or `http://localhost:8080` on the same machine).

---

## Auth: getting `GP_AUTH_DATA`

gpix talks the same mobile protocol the Android Google Photos app uses. To prove to Google that you're a legitimate client, you need a long-lived master token plus device fingerprint, packaged as a query-string blob called `auth_data`.

There's no built-in login flow. You extract this once from a real Android app session. Two paths:

**ReVanced + adb (no root):**

1. Install Google Photos ReVanced (grab the patched APK from [revanced-magisk-module releases](https://github.com/j-hc/revanced-magisk-module/releases/latest)) and GmsCore on any Android device or emulator.
2. Plug in via USB, enable USB debugging.
3. Run `adb logcat | grep "auth%2Fphotos.native"`.
4. Sign into Google Photos in the app.
5. Copy the line that starts with `androidId=` — that's your `auth_data`.

**Rooted device + HTTP Toolkit:**

1. Configure HTTP Toolkit to intercept traffic from the Google Photos app.
2. Look for the POST to `https://android.googleapis.com/auth` with body containing `service=oauth2:...photos.native`.
3. Copy the entire form body.

Either way you end up with one long string like:

```
androidId=3fe3659f2757ca27&app=com.google.android.apps.photos&...&Token=aas_et/AKpp...
```

Paste it into `.env`:

```env
GP_AUTH_DATA=androidId=...&Token=aas_et/...
```

The token is account-bound and effectively permanent. gpix exchanges it for short-lived OAuth bearers on demand, caching them in memory. `GP_AUTH_DATA` seeds the first linked account; you can link more Google accounts later in the web UI (**Settings → Accounts**) and switch between them.

---

## Web UI

The web UI is the main way most people will use gpix.

### Setup

All settings come from environment variables, usually in a `.env` file next to the binary. `.env.sample` lists every variable with a comment; only `GP_AUTH_DATA` is required.

```bash
cp .env.sample .env
# set GP_AUTH_DATA
go run .
```

Open `http://localhost:8080`. On a fresh server the first visitor creates an account with an email and a password (at least 8 characters). Sign-up stays open until `GPIX_MAX_USERS` accounts exist (default `1`; `0` means no limit), then closes. `GPIX_SIGNUP_ALLOWLIST` (comma-separated emails and/or `@domains`) limits who may sign up. Both apply to password sign-up and to single sign-on alike.

```env
GP_AUTH_DATA=androidId=...&Token=aas_et/...
GPIX_MAX_USERS=1
GPIX_SIGNUP_ALLOWLIST=you@example.com
```

**Upgrading from an older version:**

- `gpix-web.conf` is no longer read. If one exists, gpix logs a warning at startup that lists the variables to move into `.env`. The `-config` flag is deprecated.
- `GPIX_USERNAME` and `GPIX_PASSWORD_HASH`, if set, are imported once as an account (sign in with that username), after which you can remove both lines.

Runtime state lives in `GPIX_DATA_DIR` (default: the working directory): `secret.key`, `gateways.json`, `encryption.key`, the `*.db` files (accounts, users, shares, uploads, albums, imports, API tokens, and a `cache_<id>.db` listing snapshot per linked account), and `gpix-thumbcache/`. The **Telegram bot is off by default**: `gpix` (or `-mode all`) runs just the web UI and gateways unless `TG_BOT_ENABLED=true`.

On first run gpix generates a 32-byte `secret.key` in the data folder (used to sign session cookies and media-share tokens). Keep it safe; rotating it logs everyone out.

### Features

- **Library** — newest first, 50 items per page loaded as you scroll, grouped by day. Photos are dated from their camera metadata (EXIF date for JPEG/TIFF, creation time for MP4/MOV/HEIC-style files), falling back to the file's modified time.
- **Timeline scrubber** — a rail on the right edge with year marks; drag or click to jump to any month.
- **Albums and tags** — stored by gpix (`albums.db`), not synced with albums in Google Photos.
- **Select** — click, shift-click a range, or long-press on touch screens; then share, add to an album, tag, download, or move to trash.
- **Photo view** — full-screen viewer with zoom, swipe and keyboard navigation, a details panel with tags, and download of the original.
- **Video view** — videos play in the viewer. The older `/view/{key}` page adds HLS adaptive streaming with a quality picker and seeking, and a signed stream URL you can open in VLC.
- **Upload** — drag-and-drop multi-file, with progress for each file
- **Delete** — move items to Google Photos trash from the UI. **Delete gpix uploads** (in Connections) removes only what gpix itself uploaded, after showing how many items it will remove.
- **Disguised files** show with file icon + extension badge instead of a video player

Sessions last 30 days by default (`GPIX_SESSION_DAYS`).

---

## Gateways: S3 & WebDAV

gpix can expose the same library — including disguised non-media files — over two standard protocols, each on its own port, running alongside the web UI. Uploads still go through the original-quality Pixel path, and downloads transparently un-disguise wrapped files. The mapping is flat: one bucket / one root collection of objects keyed by filename.

### Turn it on

Both gateways are **off by default**. Enable the ones you want in `.env` and restart gpix:

```env
# S3 API on :9000 (all interfaces)
GPIX_S3_LISTEN=0.0.0.0:9000
# Optional and cosmetic; any signed region is accepted
# GPIX_S3_BUCKET=gpix
# GPIX_S3_REGION=us-east-1

# WebDAV on :8081 (all interfaces)
GPIX_WEBDAV_LISTEN=0.0.0.0:8081
```

Use `127.0.0.1` instead of `0.0.0.0` if you want an endpoint reachable only from the same machine. Don't put comments at the end of a value line in `.env`; they become part of the value.

That's it — no credentials needed in `.env`. Run gpix as usual:

```bash
go run . -mode web      # or -mode all
```

> **Network exposure.** Binding to `0.0.0.0` listens on every interface, so the ports are reachable from your whole LAN — anyone who can route to the host can hit them. SigV4 (S3) and Basic auth (WebDAV) gate access, but the traffic itself is **plain HTTP with no transport encryption**. For anything beyond a trusted local network, put gpix behind a reverse proxy with TLS, restrict with a firewall, or use an SSH tunnel and bind to `127.0.0.1` instead.

### Generate & save credentials (web UI)

Open the web UI → **Connections** (under Settings). For each gateway you'll see whether it's running, its endpoint, and controls to mint credentials:

- **S3** — click **Make keys**. gpix creates an **Access Key ID** (public, like `GPIX…`) and a **Secret Access Key**, shown masked until you click the eye icon; copy buttons sit next to each value. **Save the secret in your client now.** **Make new keys** rotates the pair (old keys stop working instantly); **Remove keys** disables S3 auth entirely. The card also shows ready-to-run setup for the AWS CLI, rclone and Python (boto3).
- **WebDAV** — any gpix account's email and password always works. Optionally click **Generate app password** to mint a separate, revocable password you can paste into a client without exposing your main one.

Credentials are stored in `gateways.json` in the data folder (file mode `0600`, git-ignored). Rotating a key in the UI takes effect immediately — no restart.

### Use it — S3

```bash
export AWS_ACCESS_KEY_ID=GPIX...           # from the Connections page
export AWS_SECRET_ACCESS_KEY=...           # the secret you copied

aws --endpoint-url http://127.0.0.1:9000 s3 ls s3://gpix/
aws --endpoint-url http://127.0.0.1:9000 s3 cp ./report.pdf s3://gpix/
aws --endpoint-url http://127.0.0.1:9000 s3 cp s3://gpix/report.pdf ./out.pdf
aws --endpoint-url http://127.0.0.1:9000 s3 rm s3://gpix/report.pdf
```

Works the same with `mc` (MinIO client), `s3cmd`, `boto3`, or `rclone`'s S3 backend; configure them for **path-style** addressing. Supported operations: list buckets, list objects (v1 & v2, with `prefix`/`delimiter`), HEAD/GET (incl. `Range`), PUT, DELETE, and batch delete. Multipart upload, ACLs, versioning, and tagging are **not** implemented, so configure clients for single-part puts (`boto3`: a large `multipart_threshold`).

Listings come from gpix's cached library listing, so they're fast; nothing is downloaded or decrypted until a client reads an object. One known limitation: a listing reports the size stored in Google Photos, which for encrypted or disguised items is larger than the original file that a GET returns. Sync tools that compare sizes (for example `rclone sync`) may then see a mismatch; use `--ignore-size` there.

### Use it — WebDAV

```bash
# rclone
rclone config create gpix webdav url http://127.0.0.1:8081 vendor other \
  user you@example.com pass <app-password-or-account-password>
rclone ls gpix:
rclone copy ./report.pdf gpix:

# curl
curl -u you@example.com:<password> -T report.pdf http://127.0.0.1:8081/report.pdf
curl -u you@example.com:<password> http://127.0.0.1:8081/report.pdf -o out.pdf
```

**Finder (macOS):** *Go → Connect to Server* → `http://127.0.0.1:8081`.
**Windows:** *Map network drive* → same URL.

> **Heads-up on duplicates.** Google Photos allows multiple items with the same filename; the gateways expose only the newest one per name. Treat object keys as filenames, and prefer unique names when uploading.

### Testing the gateways

The protocol layers run against any `store.Backend`. A standalone harness wires them to an **in-memory** backend so you can test with real clients without touching Google Photos:

```bash
# S3 + WebDAV on an in-memory store, no Google auth required
go run ./cmd/gpix-gateway-test \
  -s3 127.0.0.1:9000 -dav 127.0.0.1:8081 \
  -access test -secret testsecret -bucket gpix -user gpix -pass gpix

# In another shell:
go test ./pkg/s3/...                       # SigV4 unit tests (AWS test vectors)
pip install boto3 && python3 test/s3_smoke.py
./test/webdav_smoke.sh
```

---

## CLI

```bash
go run . -cli photo.jpg
go run . -cli -quality saver photo.jpg
go run . -cli -recursive ./vacation
```

| Flag | Meaning |
|---|---|
| `-auth <str>` | auth_data, defaults to `$GP_AUTH_DATA` |
| `-quality original\|saver\|quota` | upload quality / quota behavior |
| `-profile pixel-xl\|pixel-5` | device fingerprint for the session |
| `-concurrency <n>` | parallel uploads (default 1) |
| `-recursive` | descend into directories |
| `-force` | skip the dedup check |
| `-delete-after` | delete local file after successful upload |

Output: `OK <path>\t<media_key>` or `SKIP <path>\t<media_key>` per file on stdout; events on stderr.

The CLI is the fastest way to bulk-upload from a server. No web UI, no bot, just files in → media keys out.

---

## Telegram bot

If you want to push files to Google Photos from a Telegram chat (or pull files out into a chat), gpix can run a bot.

### Setup

1. Open [@BotFather](https://t.me/BotFather), run `/newbot`, save the token.
2. Visit [my.telegram.org/apps](https://my.telegram.org/apps), create an app, save `api_id` and `api_hash`.

Add to `.env` (the bot is off until `TG_BOT_ENABLED=true`, and only runs with `-mode all` or `-mode bot`):

```env
TG_BOT_ENABLED=true
TG_BOT_TOKEN=123456789:ABC...
TG_API_ID=12345678
TG_API_HASH=abcd...
TG_OWNER_ID=987654321
```

The bot only honors commands from `TG_OWNER_ID` — every other message is silently ignored.

### Commands

| Command | Behavior |
|---|---|
| `/upload` | Reply to any file/photo/video with `/upload` → bot uploads it to Google Photos, replies with the media key. Non-media files are auto-disguised. |
| `/get <media_key>` | Bot fetches the file from Google Photos and sends it back to the chat. Auto-unwraps disguised files. |
| `/list [n]` | Shows the N most recent items in the library. |
| `/info` | Account email, device profile, concurrency. |

File size cap: 2 GB for bot accounts; 4 GB for user accounts with Telegram Premium.

---

## Disguised files

Google Photos only accepts photos and videos. gpix gets around this with a simple trick: wrap arbitrary files in a tiny valid MP4 container, then append the original bytes after the container's declared end. Google's pipeline stops reading at the MP4 trailer and stores the rest of the bytes verbatim at original quality.

When you upload `report.pdf` through the web UI or `/upload` in the bot:

1. gpix detects it's not media (MIME + extension + magic-byte sniff).
2. Builds a payload: `[3 KB precompiled wrapper.mp4][16-byte magic][filename length][filename][payload length][payload]`.
3. Uploads as `report.pdf.mp4`. Google Photos sees a 1-second solid-color video. The trailing PDF bytes are preserved.

When you download it:

1. gpix scans the first 8 KB of the file from Google's servers.
2. Finds the magic marker, parses the header, strips the wrapper.
3. Returns the original `report.pdf` with the right Content-Type and filename.

The UI marks disguised items by their original extension rather than as video players. **Text and markdown documents** (and source/config/data text files) go a step further: gpix renders their content to a readable monospace **image**, so they appear as a viewable page in the grid and on the view screen (the filename keeps its real extension so you can tell it's a document). The image is generated locally from the decrypted original and cached; **Download** always returns the original file byte-for-byte. (PDFs and other binary documents still show as file cards — rendering those needs a PDF rasteriser that isn't pure-Go.)

**Caveat:** This is obfuscation, not encryption. Anyone with the media key and this format spec can recover the bytes. For real privacy, turn on **encryption** (below), which layers AES-256 on top of the disguise.

---

## Encryption

Disguising hides *what kind* of file something is, but the bytes are still there for anyone who can read them. Encryption fixes that: with it on, gpix encrypts every upload with a key that never leaves your machine, then disguises the **ciphertext** as a video. Google — and anyone else without your key — only ever stores and sees an opaque 1-second clip. **Only you can see your media.**

### How it works

- **Cipher:** AES-256-GCM in a chunked stream (the age/Tink "STREAM" construction). A fresh 256-bit content key is derived per file via HKDF-SHA256 from your master key and a random salt; the header is authenticated and the final chunk is tagged, so tampering, reordering, and truncation are all detected.
- **Key:** a single 32-byte master key gpix generates and stores in `encryption.key`, next to `secret.key` (mode `0600`). It is never uploaded.
- **Flow:** `original → encrypt → disguise as .mp4 → Google Photos`. On download (web UI, S3, or WebDAV) gpix detects the encrypted blob and decrypts it transparently, returning the original bytes and filename.

### Turn it on

Web UI → **Connections** → **Media encryption** → *Turn encryption on*. (Or seed it with `GPIX_ENCRYPT_UPLOADS=true` in `.env` for the first run.) From then on, every new upload is encrypted. The panel shows a short **key fingerprint** so you can confirm which key is active, and a **Download key backup** button.

> **Back up `encryption.key`.** If you lose it, everything encrypted with it is **permanently unreadable** — gpix can't recover it and neither can Google. Use the backup button (or copy the file) and store it somewhere safe. Restoring it later (drop it back next to `secret.key`) restores access; the fingerprint lets you verify it's the right one.

### Thumbnails for encrypted items

Google only has a blank thumbnail for an encrypted or disguised item, so gpix renders its own:

- One download and decrypt produces every size gpix uses (256, 512 and 2048 px), smallest first.
- Rendering starts in a background queue as soon as the library page is sent, before the browser asks for the images. The grid shows a placeholder and fills each tile in when its thumbnail is ready.
- Files uploaded through gpix are thumbnailed from the local copy right after the upload, so nothing is downloaded again.
- Thumbnails are cached **decrypted** under `gpix-thumbcache/` in the data folder. Delete that folder to purge them.
- `GPIX_THUMB_GEN=false` turns rendering off to save CPU. Thumbnails already rendered are still served; new encrypted items then show a placeholder.

### Trade-offs

- **Inside gpix, encrypted items look normal** — gpix holds the key, so encrypted photos render as photos and encrypted videos play in-page. Only *Google* ever sees the opaque blank video; encryption matters at upload, not for viewing in gpix. Thumbnail generation covers JPEG/PNG/GIF in pure Go; HEIC/WebP/RAW fall back to a blank Google thumbnail. Encrypted videos play via progressive download (no adaptive HLS/seek), and one heuristic edge remains: an encrypted video whose original name ends in `.mp4` is treated as a normal video (and will look blank) — non-`.mp4` videos and all photos are fine.
- Encryption applies to **new** uploads only; existing library items are untouched. Toggling encryption off later still lets you open previously-encrypted items (the key is unchanged).
- The **Telegram bot is not encryption-aware** yet: it won't encrypt its uploads and `/get` returns the still-encrypted blob. Use the web UI, S3, or WebDAV for encrypted media.

---

## Sharing

Create public links to individual items — optionally password-protected and expiring by time or download count. Links are stored in a small SQLite database (`shares.db`, in the data folder). Because decryption happens server-side, you can share an **encrypted** item and the recipient sees the photo without ever touching your key.

**Create one:** open any photo → **Share** → set an optional password, expiry, max downloads, and whether downloading the original is allowed → **Create link**.

**Create a multi-item share:** in the library grid click **Select**, tick any number of photos/videos/documents, then **Share** → set the same options (plus an optional title). The recipient gets one link to a **gallery** of all the items. Manage and revoke links under **Shares**.

**Recipient:** opens `https://<your-server>/s/<token>`, enters the password if set, and views the item(s) — a single photo, or a thumbnail gallery for multi-item shares. When downloads are allowed, each photo has its own download button, and **Download all** fetches everything as one `.zip`. Links honor their expiry and download cap automatically.

**Direct download link:** `https://<your-server>/s/<token>/download` starts the download without showing a page: the file itself for a one-item share, a `.zip` for several. It's offered only when the link has no password and allows downloading originals, and you can copy it from the share dialog or the Shares page.

Share URLs are built from `SERVER_URL` when set, otherwise from the request host — so set `SERVER_URL` if gpix sits behind a reverse proxy.

---

## Login & access (Logto SSO)

Besides email-and-password accounts (see [Setup](#setup)), gpix can sign you in via **[Logto](https://logto.io)** (or any standards-compliant OIDC provider) using the authorization-code + PKCE flow. Password login keeps working, so this is additive.

**Set up:** in Logto, create a **Traditional Web** application, and set its **Redirect URI** to `<SERVER_URL>/auth/logto/callback`. Then configure gpix:

```env
GPIX_LOGTO_ENDPOINT=https://your-tenant.logto.app
GPIX_LOGTO_CLIENT_ID=...
GPIX_LOGTO_CLIENT_SECRET=...
# Used to build the callback URL
SERVER_URL=https://photos.example.com
# Optional
GPIX_SIGNUP_ALLOWLIST=you@example.com,@example.com
GPIX_MAX_USERS=5
```

A **"Continue with single sign-on"** button then appears on the sign-in page.

- **Allowlist** — `GPIX_SIGNUP_ALLOWLIST` is a comma-separated list of exact emails (`a@b.com`) and/or domains (`@b.com`). It's checked when an account is created, by password sign-up or a *first* SSO sign-in (empty list = anyone). Already-registered users always pass.
- **Max users** — `GPIX_MAX_USERS` caps how many accounts can exist, counting password and SSO accounts together (default `1`; `0` = unlimited). Once reached, sign-up closes and new SSO sign-ins are rejected with "Sign-up is closed".
- **SSO only** — set `GPIX_DISABLE_PASSWORD_LOGIN=true` to hide the email/password form and allow only Logto sign-in. It's ignored unless Logto is configured, so you can't accidentally lock yourself out.
- Accounts are stored in `users.db` in the data folder. Sessions are the same signed cookies for both kinds of login, so nothing else in the app changes.

> gpix is single-tenant: everyone who signs in shares the same linked Google Photos accounts. Accounts control **who may use this gpix**, not separate per-user libraries.

---

## Import

Copy a library into gpix from another source. Imports go through the same path as a normal upload, so **encryption and disguising apply if enabled**, and gpix's SHA-1 dedup skips anything already present. Start one from the web UI → **Import** (or the app); progress (done / skipped / failed / total + current file) updates live, the job keeps running in the background if you navigate away, and there's a **Stop** button. Pick how many **parallel transfers** to run (1–10).

### From another Google account

Choose one of your linked accounts (link more under **Settings → Accounts**), or paste the other account's `GP_AUTH_DATA`. Its items are downloaded and re-uploaded into the active account.

### From Immich

Migrate an existing [Immich](https://immich.app) library into gpix. gpix connects to your Immich server as a **client** (with an Immich API key), lists every asset, downloads each original, and uploads it into gpix.

Enter your Immich **server URL** and an **API key** (create one in Immich under *Account Settings → API Keys*), then **Start Immich import**. Re-running is safe — imported asset IDs are recorded in `import.db`, so a second run only copies what's new.

> Notes: the importer enumerates the whole library via Immich's **timeline-bucket** API (`/api/timeline/buckets` + `/api/timeline/bucket`), which is robust across Immich versions — an earlier build used search pagination and stopped after one page. Duplicates are skipped two ways: by **Immich asset ID** (in `import.db`, for cheap re-runs) and by gpix's own **SHA-1** content dedup against your Google Photos library (effective for unencrypted items; encrypted uploads use a random salt so only the asset-ID check applies). Parallelism is capped at 10 — higher is faster but uses more bandwidth and disk (each worker buffers a temp file).

---

## App API (`/api/v1`)

gpix exposes a JSON API for native clients — the **gpix app** (Flutter, cross-platform) and any script or tool. It reuses the same library cache, decrypt-on-serve, thumbnail, and upload pipeline as the web UI, so encrypted items are served **decrypted** to an authenticated client.

**Auth — API tokens.** Web UI → **Settings → App tokens** → *Generate token*. The full token is shown **once** (copy it then); only a SHA-256 hash is stored, and you can revoke it anytime. Send it as a bearer header on every request:

```
Authorization: Bearer gpix_xxxxxxxx…
```

Tokens live in `api-tokens.db` (git-ignored, in the data folder).

**Endpoints:**

| Method & path | Purpose |
| --- | --- |
| `GET /api/v1/ping` | Validate server + token; returns user, whether encryption is on, and feature flags. |
| `GET /api/v1/media` | A page of the library, newest first. Query: `limit` (max 50), `cursor` (the previous page's `nextCursor`), `before` (a page's `prevCursor`, to go back up), `from` (Unix ms: start at the first item taken before it), `album`, `tag`. Returns `{items, nextCursor, prevCursor, offset, total}`; items carry `key`, `name`, `class` (photo/video/doc/file), `encrypted`, `size`, `mtime`, `tags`, `thumbPending` and `thumb`/`original` URLs. An empty `nextCursor` means the end. Returns **503** while the server's first listing is still running; retry after a few seconds. |
| `GET /api/v1/media/timeline` | Item count per month, newest first: `{months: [{year, month, count}], total}`. Group by `tz` (an IANA zone) or `tzOffset` (minutes east of UTC); `album` / `tag` narrow it. |
| `GET /api/v1/media/{key}/thumb?size=` | Thumbnail (rendered by gpix for disguised items). `size` ∈ 64/128/256/512/1024/2048. While gpix is still rendering one, it returns a 1×1 placeholder with `X-Thumb-Pending: 1`; ask again shortly. |
| `GET /api/v1/media/{key}/original?download=1` | Decrypted original bytes; `download=1` forces an attachment. |
| `POST /api/v1/upload` | Multipart (`files`, optional `quality`, optional `mtime` in ms before each file), uploaded through the encryption/disguise path. |
| `DELETE /api/v1/media/{key}` | Move an item to trash. |
| `POST /api/v1/media/delete` | Move up to 5000 items to trash in one request: `{keys}` → `{deleted, failed}`. |
| `GET /api/v1/albums` · `POST /api/v1/albums` | List albums / create one (`{name, keys}`). |
| `GET /api/v1/albums/{id}` · `POST /api/v1/albums/{id}` · `DELETE /api/v1/albums/{id}` | Get / rename (`{name}`) / delete an album (its items stay in the library). |
| `POST /api/v1/albums/{id}/items` · `POST /api/v1/albums/{id}/items/remove` | Add / remove items: `{keys}`. |
| `GET /api/v1/tags` · `POST /api/v1/tags/add` · `POST /api/v1/tags/remove` | List tags with counts / add or remove tags: `{keys, tags}` → each key's tags afterwards. |
| `GET /api/v1/shares` · `POST /api/v1/shares` · `DELETE /api/v1/shares/{token}` | List / create / revoke share links. Shares include `directUrl` when a direct download link is available. |
| `GET /api/v1/accounts` · `POST /api/v1/accounts/{id}/switch` | List linked Google accounts / switch the active one. |
| `GET /api/v1/import/status` · `POST /api/v1/import/start` · `POST /api/v1/import/google-start` · `POST /api/v1/import/stop` | Drive the importers. `google-start` takes `{accountId}` (a linked account) or `{authData}`, plus `workers`. |
| `GET /api/v1/gpix-uploads/summary` · `POST /api/v1/gpix-uploads/delete` | Count what gpix uploaded / move it to trash (the delete repeats the reviewed account and count). |

The web UI also uses `/api/v1/auth/*` (`me`, `login`, `logout`, `signup`; `me` reports `signupOpen` and `firstRun`), `/api/v1/gateways*`, `/api/v1/encryption/toggle`, `/api/v1/tokens*` and `/api/v1/delete-all`.

**Quick check:**

```bash
curl -H "Authorization: Bearer <token>" https://your-server/api/v1/ping
curl -H "Authorization: Bearer <token>" "https://your-server/api/v1/media?limit=50"
```

> The cross-platform **gpix app** (Android / iOS / Windows / Linux / macOS, built with Flutter + Material 3) is built on this API — see [`app/`](app) and its [README](app/README.md) for build/run instructions.

---

## Immich-compatible API (archived)

An experimental Immich-compatible REST API once lived here. It has been **archived** — the code is kept under [`_archive/immich/`](_archive/immich) (a directory the Go toolchain ignores, so it is not compiled or wired into the binary) but is no longer built or served. To revive it, move the package back under `pkg/`, restore the `immich_listen` config plumbing, and re-add the startup block in `main.go`.

---

## Performance & caching

Listing a large Google Photos library is the slow part of the photo grid, S3 `ListObjects` and WebDAV `PROPFIND`. gpix keeps **one shared, background-refreshed listing cache** per linked account (`pkg/library`) that all of them read from, so they return from memory instead of re-walking Google on every request. Uploads and deletes made through gpix show up in it right away.

It's also backed by a **SQLite snapshot** (`cache_<account id>.db`, via `pkg/cachedb`): on startup gpix serves the listing immediately from the last saved snapshot and refreshes it in the background (stale-while-revalidate). On a fresh install there's no snapshot yet, so the first full listing can take a while for a large library; the web UI shows "Loading your library" and retries until it's done, and the API returns 503 meanwhile. The log reports its progress.

Thumbnails are served on demand: Google's thumbnail for normal items, and gpix's own for encrypted items (see [Thumbnails for encrypted items](#thumbnails-for-encrypted-items)). Clients load previews first and fetch full originals only when opened.

`SERVER_URL` sets the externally-reachable base URL used for share links and redirects; leave it unset to derive from the request host.

---

## Logging

gpix logs to stderr.

```env
# debug | info (default) | warn | error. The -log flag wins when given.
GPIX_LOG_LEVEL=info
# text (default), or json for log collectors
GPIX_LOG_FORMAT=text
```

- **Requests:** one line per HTTP request with method, path, status, bytes, duration, client IP, user and a request id. The id is also returned in the `X-Request-Id` response header, so a failing request can be found in the log. Share and signed-link tokens are shortened to their first 4 characters.
- **Levels:** at `info` you see sign-ins and other changes, client errors, slow requests (over 10 seconds; downloads and streams excluded) and server errors. Routine page and thumbnail loads only appear at `debug`.
- **Library listing:** a line when the first listing starts, "still listing" every 15 seconds while it runs, the item count when it finishes, and a warning if it fails.
- **Thumbnails:** failed renders are warnings, at most 5 a minute; the rest go to `debug`.
- **Startup:** one line with the listen address, data folder, account limit, thumbnail generation, and whether S3 and WebDAV are on.

---

## Build for Linux

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags nodynamic -ldflags="-s -w" -o gpix .
```

Fully static (the SQLite and image dependencies are pure Go), ~25 MB stripped. Keep `-tags nodynamic`: without it the HEIC decoder links against glibc, and the binary won't start on musl systems such as Alpine. Drop the binary on any Linux box with `.env` next to it; gpix creates its state files (`secret.key`, `gateways.json`, `encryption.key`, the `*.db` files, `gpix-thumbcache/`) on first run.

---

## Docker

Prebuilt multi-arch images (`linux/amd64`, `linux/arm64`) are published to GitHub Container Registry by `.github/workflows/docker.yml`: `:latest` and `:<version>` for each `v*` release tag, and `:edge` for the latest `master`.

```bash
docker run --rm -p 8080:8080 -p 9000:9000 -p 8081:8081 \
  -v "$PWD/data:/data" ghcr.io/imnotdev25/gpix:latest
```

To build it yourself: a multi-stage `Dockerfile` builds the React UI and a static binary into a minimal Alpine image (web assets are embedded, so only CA certs are added). The container's working directory is `/data`, which you mount: gpix reads `/data/.env` and keeps all its state there (`secret.key`, `gateways.json`, `encryption.key`, the `*.db` files, `gpix-thumbcache/`).

```bash
# Put your settings in ./data/.env first:
mkdir -p data
cp .env.sample data/.env    # then set GP_AUTH_DATA and anything else you need

# Build and run
docker build -t gpix .
docker run --rm -p 8080:8080 -p 9000:9000 -p 8081:8081 \
  -v "$PWD/data:/data" gpix
```

Or with Compose (`docker compose up -d` — see `docker-compose.yml`). The image listens on `8080` (web); the S3 (`9000`) and WebDAV (`8081`) ports only answer once you set `GPIX_S3_LISTEN` / `GPIX_WEBDAV_LISTEN` in `.env`. With `0.0.0.0` listen addresses, publishing the ports exposes them on your host. It runs as a non-root user, so make sure the mounted `./data` directory is writable by UID `10001` (or add `--user "$(id -u):$(id -g)"`). The default command is `-mode web`; to add the Telegram bot, set `TG_BOT_ENABLED=true` and use `-mode all`.

---

## Releases & CI

**CI** (`.github/workflows/ci.yaml`) runs on every push and pull request: it builds, vets, and tests the Go server, and analyzes + builds the Flutter app. Since the app's platform folders are generated (and git-ignored), CI runs `flutter create` first — exactly what a fresh checkout does.

**Releases** (`.github/workflows/release.yml`) run when you push a version tag (or from the Actions tab via *workflow_dispatch*):

```bash
git tag v0.1.0
git push origin v0.1.0
```

That publishes a GitHub Release containing:

- **Server binaries** — Linux and macOS for amd64 + arm64, and Windows for amd64, cross-compiled from a single runner (the server is CGO-free), with `checksums-server.txt`.
- **App builds** — an Android APK; a Windows installer and portable `.zip`; a Linux `.deb`, AppImage and `.tar.gz`; and a macOS `.dmg`. The desktop builds come from `.github/workflows/desktop.yml`. They're unsigned, so Windows SmartScreen and macOS Gatekeeper warn on first launch; the release notes say how to get past that.

iOS is intentionally excluded — App Store / device installs require code signing; build it locally per [`app/README.md`](app/README.md).

---

## Trust model

- **Your photos stay in your Google account.** If you stop using gpix tomorrow, everything is still there in the regular Google Photos app/web.
- **No third party.** The binary talks directly to Google. No relay servers, no analytics, no telemetry.
- **Auth tokens stay local.** `GP_AUTH_DATA` lives in `.env` on your machine and never leaves it except to authenticate to Google.
- **API tokens grant full access.** An `/api/v1` token (used by the app) can read, upload, delete, and share your whole library — treat it like a password. Only a SHA-256 hash is stored server-side (`api-tokens.db`), and any token is revocable from the web UI → **Settings → App tokens**. The app keeps its copy in the device's local preferences.
- **Single shared library.** gpix is single-tenant: every account on the server (up to `GPIX_MAX_USERS`) shares the same linked Google Photos accounts. Accounts control *who may sign in*, not separate per-user libraries.
- **Binds `0.0.0.0` by default** (all interfaces) for convenience. Switch to `127.0.0.1` to keep it local, or put it behind a reverse proxy with TLS for remote access. The S3 and WebDAV gateways follow the same rule — see the network-exposure note above.

---

## License

[MIT](LICENSE).
