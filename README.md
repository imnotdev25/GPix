# gpix

Your Google Photos library, on your terms.

gpix is a self-hosted Google Photos client written in Go. One binary, several ways to use it:

- **Web UI** — browse, view, stream videos with seek, upload from your browser, delete items. Minimalist black-and-white design, dark/light theme, fully responsive. All assets embedded.
- **S3-compatible gateway** — point `aws`, `mc`, `boto3`, `rclone`, or any S3 client at your library. AWS Signature V4 auth, keys generated and rotated from the web UI.
- **WebDAV gateway** — mount your library in Finder, Windows Explorer, or `rclone`. Basic auth with your login password or a revocable app password.
- **CLI** — upload files or whole folders from the terminal.
- **Telegram bot** — `/upload`, `/get`, `/list`, `/info` against your library from any chat.
- **Universal file storage** — PDFs, archives, executables, any file gets transparently wrapped as a 1-second MP4 with the original bytes preserved. Uploaded as a "video", recovered byte-identical on download. Effectively unlimited cloud storage for arbitrary files.

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
# Build
go build .

# Or run without building
go run .
```

On first run gpix wants:

1. A Google Photos `auth_data` string in `.env` (see [Auth](#auth-getting-gp_auth_data) below).
2. A web config + bcrypt password hash if you want the web UI (see [Web UI](#web-ui)).
3. Telegram bot credentials in `.env` if you want the bot (see [Telegram bot](#telegram-bot)).

Then:

```bash
go run .                        # bot + web together
go run . -mode web              # web UI only
go run . -mode bot              # bot only
go run . -cli ./photos          # CLI uploader
go run . -hashpw                # generate a bcrypt password hash
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

The token is account-bound and effectively permanent. gpix exchanges it for short-lived OAuth bearers on demand, caching them in memory.

---

## Web UI

The web UI is the main way most people will use gpix.

### Setup

```bash
# Generate a password hash
go run . -hashpw
# (paste password, copy the printed $2a$12$... hash)

# Create config
cp gpix-web.conf.example gpix-web.conf
# Edit: set username, paste hash into password_hash
```

Config:

```toml
listen = 0.0.0.0:8080
username = you
password_hash = $2a$12$replace_me
device_profile = pixel-xl
max_concurrent_uploads = 2
session_days = 30
stream_token_ttl_minutes = 60
```

On first run gpix generates a 32-byte `secret.key` next to the config (used to sign session cookies and media-share tokens). Keep it safe; rotating it logs everyone out.

### Features

- **Library browse** with paginated grid, lazy-loaded thumbnails
- **Photo view** — full-resolution display, click to download original
- **Video view** — Plyr player with HLS adaptive streaming, manual quality picker (192p → 1920p), proper seeking
- **Stream URLs** — copy a signed URL straight into VLC for any quality level
- **Upload** — drag-and-drop multi-file, live progress via Server-Sent Events
- **Delete** — move items to Google Photos trash from the UI
- **Disguised files** show with file icon + extension badge instead of a video player

Single-user. Sign in once, session lasts 30 days by default.

---

## Gateways: S3 & WebDAV

gpix can expose the same library — including disguised non-media files — over two standard protocols, each on its own port, running alongside the web UI. Uploads still go through the original-quality Pixel path, and downloads transparently un-disguise wrapped files. The mapping is flat: one bucket / one root collection of objects keyed by filename.

### Turn it on

Both gateways are **off by default**. Enable the ones you want by adding a `*_listen` line to `gpix-web.conf` and restarting gpix:

```toml
# gpix-web.conf
s3_listen     = 0.0.0.0:9000     # S3 API on :9000 (all interfaces)
s3_bucket     = gpix             # cosmetic; default "gpix"
s3_region     = us-east-1        # cosmetic; any signed region is accepted

webdav_listen = 0.0.0.0:8081     # WebDAV on :8081 (all interfaces)
```

Use `127.0.0.1` instead of `0.0.0.0` if you want an endpoint reachable only from the same machine.

That's it — no credentials needed in the config file. Run gpix as usual:

```bash
go run . -mode web      # or -mode all
```

> **Network exposure.** Binding to `0.0.0.0` (the default here) listens on every interface, so the ports are reachable from your whole LAN — anyone who can route to the host can hit them. SigV4 (S3) and Basic auth (WebDAV) gate access, but the traffic itself is **plain HTTP with no transport encryption**. For anything beyond a trusted local network, put gpix behind a reverse proxy with TLS, restrict with a firewall, or use an SSH tunnel and bind to `127.0.0.1` instead.

### Generate & save credentials (web UI)

Open the web UI → **Connections** (top nav). For each gateway you'll see its endpoint URL and controls to mint credentials:

- **S3** — click **Generate keys**. gpix creates an **Access Key ID** (public, like `GPIX…`) and a **Secret Access Key**. Use **Show** to reveal the secret and **Copy** to grab it. The secret is shown masked by default; **save it in your client now**. **Regenerate** rotates the pair (old keys stop working instantly); **Clear** disables S3 auth entirely.
- **WebDAV** — your normal login username/password always works. Optionally click **Generate app password** to mint a separate, revocable password you can paste into a client without exposing your main one.

Credentials are stored in `gateways.json` next to `secret.key` (file mode `0600`, git-ignored). Rotating a key in the UI takes effect immediately — no restart.

### Use it — S3

```bash
export AWS_ACCESS_KEY_ID=GPIX...           # from the Connections page
export AWS_SECRET_ACCESS_KEY=...           # the secret you copied

aws --endpoint-url http://127.0.0.1:9000 s3 ls s3://gpix/
aws --endpoint-url http://127.0.0.1:9000 s3 cp ./report.pdf s3://gpix/
aws --endpoint-url http://127.0.0.1:9000 s3 cp s3://gpix/report.pdf ./out.pdf
aws --endpoint-url http://127.0.0.1:9000 s3 rm s3://gpix/report.pdf
```

Works the same with `mc` (MinIO client), `s3cmd`, `boto3`, or `rclone`'s S3 backend. Supported operations: list buckets, list objects (v1 & v2, with `prefix`/`delimiter`), HEAD/GET (incl. `Range`), PUT, DELETE, and batch delete. Multipart upload, ACLs, versioning, and tagging are **not** implemented, so configure clients for single-part puts (`boto3`: a large `multipart_threshold`).

### Use it — WebDAV

```bash
# rclone
rclone config create gpix webdav url http://127.0.0.1:8081 vendor other \
  user your-username pass <app-password-or-login-password>
rclone ls gpix:
rclone copy ./report.pdf gpix:

# curl
curl -u your-username:<password> -T report.pdf http://127.0.0.1:8081/report.pdf
curl -u your-username:<password> http://127.0.0.1:8081/report.pdf -o out.pdf
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

Add to `.env`:

```env
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

The UI clearly marks disguised items — they show as file cards with the original extension, not video players.

**Caveat:** This is obfuscation, not encryption. Anyone with the media key and this format spec can recover the bytes. Don't store secrets in there without a separate encryption layer.

---

## Build for Linux

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o gpix .
```

Fully static, ~25 MB stripped. Drop the binary on any Linux box with `.env` and `gpix-web.conf` next to it.

---

## Docker

A multi-stage `Dockerfile` builds a static binary into a minimal Alpine image (web assets are embedded, so only CA certs are added). All runtime state — `gpix-web.conf`, `.env`, `secret.key`, `gateways.json` — lives in `/data`, which you mount.

```bash
# Put your config and auth in ./data first:
mkdir -p data
cp gpix-web.conf.example data/gpix-web.conf   # then edit it
printf 'GP_AUTH_DATA=androidId=...&Token=aas_et/...\n' > data/.env

# Build and run
docker build -t gpix .
docker run --rm -p 8080:8080 -p 9000:9000 -p 8081:8081 \
  -v "$PWD/data:/data" gpix
```

Or with Compose (`docker compose up -d` — see `docker-compose.yml`). The image listens on `8080` (web), `9000` (S3), and `8081` (WebDAV); with `0.0.0.0` listen addresses in the config, publishing the ports exposes them on your host. It runs as a non-root user, so make sure the mounted `./data` directory is writable by UID `10001` (or add `--user "$(id -u):$(id -g)"`). Default `CMD` runs web only; use `-mode all` to add the Telegram bot.

---

## Trust model

- **Your photos stay in your Google account.** If you stop using gpix tomorrow, everything is still there in the regular Google Photos app/web.
- **No third party.** The binary talks directly to Google. No relay servers, no analytics, no telemetry.
- **Auth tokens stay local.** `GP_AUTH_DATA` lives in `.env` on your machine and never leaves it except to authenticate to Google.
- **Web UI is single-user.** The default config binds `0.0.0.0` (all interfaces) for convenience. Switch to `127.0.0.1` to keep it local, or put it behind a reverse proxy with TLS for remote access. The S3 and WebDAV gateways follow the same rule — see the network-exposure note above.

---

## License

[MIT](LICENSE).
