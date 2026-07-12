# gpix app

A cross-platform client for a self-hosted **gpix** server, built with **Flutter** and **Material 3**. One codebase runs on **Android, iOS, Linux, and macOS** (Windows/web work too). It talks to gpix's [`/api/v1`](../README.md#app-api-apiv1) JSON API using an API token.

## What it does

- **Log in** with your server URL + an API token (generate one in the gpix web UI → **App** tab).
- **Library** — infinite-scroll photo grid with cached thumbnails (encrypted items are shown **decrypted**).
- **Viewer** — full-screen, pinch-to-zoom images; download originals; create a share link; delete.
- **Upload** — pick photos/videos/files and back them up (server-side encryption/disguise applies).
- **Shares** — list, open, copy, and revoke share links.
- **Import** — drive the Immich → gpix importer (parallel workers) with live progress.
- **Settings** — connection status, encryption indicator, sign out.

## Prerequisites

- [Flutter SDK](https://docs.flutter.dev/get-started/install) 3.3+ (Dart 3.3+).
- Platform toolchains for whatever you build: Android Studio/SDK, Xcode (iOS/macOS), or Linux desktop build deps (`clang`, `cmake`, `ninja-build`, `libgtk-3-dev`).

## Build & run

This folder ships the Dart source + `pubspec.yaml` only. Generate the platform projects once, then run:

```bash
cd app

# Generate android/ ios/ linux/ macos/ scaffolding around the existing lib/.
flutter create --platforms=android,ios,linux,macos --org com.gpix --project-name gpix_app .

flutter pub get

# Run on a connected device / desktop:
flutter run                 # pick a device when prompted
# or target one explicitly:
flutter run -d linux
flutter run -d macos
flutter run -d <android-device-id>
```

Build release artifacts:

```bash
flutter build apk           # Android
flutter build ipa           # iOS (needs Xcode signing)
flutter build linux         # Linux desktop
flutter build macos         # macOS desktop
```

## Platform notes

- **macOS** is sandboxed and blocks outbound network by default. After `flutter create`, add the network-client entitlement to **both** `macos/Runner/DebugProfile.entitlements` and `macos/Runner/Release.entitlements`:

  ```xml
  <key>com.apple.security.network.client</key>
  <true/>
  ```

- **Android + plain HTTP:** if your gpix server is `http://` (no TLS), Android blocks cleartext traffic on API 28+. Prefer **HTTPS**. If you must use HTTP, add `android:usesCleartextTraffic="true"` to `<application>` in `android/app/src/main/AndroidManifest.xml` (or a proper network-security-config). `INTERNET` permission is granted by default.
- **Self-signed TLS:** the app uses the platform's default TLS validation. Use a real certificate (e.g. Let's Encrypt / a reverse proxy) or install your CA on the device.

## Security note

The server URL and token are stored with `shared_preferences` (plain platform prefs). For stronger at-rest protection of the token, swap in [`flutter_secure_storage`](https://pub.dev/packages/flutter_secure_storage) in `lib/src/state/app_state.dart`. Revoke a token anytime from the gpix web UI → **App** tab.

## Project layout

```
lib/
  main.dart                     app root + Material 3 theme
  src/api/models.dart           JSON models (media, share, import, ping)
  src/api/api_client.dart       /api/v1 client (Bearer auth)
  src/state/app_state.dart      session (server URL + token), persisted
  src/screens/                  login, home, library, viewer, upload, shares, import, settings
  src/util/download.dart        save-to-device helper
```

## Not yet implemented (v1)

- In-app **video playback** (videos offer download instead — cross-platform players are heavy, especially on Linux).
- Multi-select share creation from the grid (create links from the viewer for now).
- Background/auto backup.
