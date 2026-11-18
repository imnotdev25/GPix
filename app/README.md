# gpix app

A cross-platform client for a self-hosted **gpix** server, built with **Flutter** and **Material 3**. One codebase runs on **Android, iOS, Linux, and macOS** (Windows/web work too). It talks to gpix's [`/api/v1`](../README.md#app-api-apiv1) JSON API using an API token.

## What it does

- **Log in** with your server URL + an API token (generate one in the gpix web UI → **App** tab).
- **Library** — infinite-scroll photo grid with cached thumbnails (encrypted items are shown **decrypted**).
- **Viewer** — full-screen, pinch-to-zoom images; download originals; create a share link; delete.
- **Upload** — pick photos/videos/files and back them up (server-side encryption/disguise applies).
- **Backup** (Android/iOS) — camera-roll style auto-backup: scans your photo library, skips what's already uploaded, and copies the rest to gpix with live progress; optional auto-run on app open.
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

# Generate the platform scaffolding around the existing lib/ (pick the ones you need).
flutter create --platforms=android,ios,linux,macos,windows --org com.gpix --project-name gpix_app .

flutter pub get

# Apply the gpix icon (assets/icon/gpix.png) to every platform folder present.
bash tool/apply_icons.sh
# Desktop runner tweaks (window title "gpix", Linux window icon):
bash tool/setup_linux.sh      # if you generated linux/
bash tool/setup_windows.sh    # if you generated windows/
bash tool/setup_macos.sh      # if you generated macos/ (sandbox entitlements)

# Run on a connected device / desktop:
flutter run                 # pick a device when prompted
# or target one explicitly:
flutter run -d linux
flutter run -d macos
flutter run -d windows
flutter run -d <android-device-id>
```

Build release artifacts:

```bash
flutter build apk           # Android
flutter build ipa           # iOS (needs Xcode signing)
flutter build linux         # Linux desktop
flutter build macos         # macOS desktop
flutter build windows       # Windows desktop (on a Windows machine)
```

### Linux packages

After `flutter build linux --release`, one script produces all three formats in `out/`:

```bash
bash tool/package_linux.sh v0.1.0
#   gpix_v0.1.0_linux-x64.tar.gz   extract, then ./install.sh (per-user, adds menu entry + icon)
#   gpix_v0.1.0_amd64.deb          sudo apt install ./gpix_v0.1.0_amd64.deb  → command: gpix-app
#   gpix_v0.1.0_linux-x64.AppImage chmod +x and run (needs GTK 3 on the host)
```

Set `FORMATS="tar deb"` to skip some. The `.deb` needs `dpkg-deb`; sized hicolor icons need ImageMagick; the AppImage step downloads `appimagetool` unless `APPIMAGETOOL` points at one.

### Windows installer

On Windows, after `flutter build windows --release`, with [Inno Setup 6.3+](https://jrsoftware.org/isinfo.php) installed (`choco install innosetup`):

```powershell
pwsh tool/package_windows.ps1 -Version v0.1.0
#   out/gpix_v0.1.0_windows-x64-setup.exe   installer: Start-menu shortcut, optional desktop icon
#   out/gpix_v0.1.0_windows-x64.zip         portable build
```

The script copies the MSVC runtime DLLs into the bundle so the app starts without the Visual C++ Redistributable. Both outputs are unsigned, so SmartScreen may warn on first run (**More info → Run anyway**).

### macOS `.dmg`

One command builds the release app (with the network entitlement) and packages a drag-to-Applications disk image at `build/gpix.dmg`:

```bash
cd app
bash tool/build_macos_dmg.sh
```

What it does: ensures the `macos/` platform exists → adds the network and file-dialog sandbox entitlements (`tool/setup_macos.sh`) → applies the gpix icon (`tool/apply_icons.sh`) → `flutter build macos --release` → stages the `.app` + an `Applications` symlink, gives the disk image the gpix volume icon, and writes a compressed DMG.

If Finder still shows the old Flutter icon after rebuilding, it's the icon cache: drag the old app to the Trash first, or run `killall Finder Dock`.

The result is **unsigned**, so Gatekeeper warns on first launch. Recipients can right-click the app → **Open**, or clear the quarantine flag:

```bash
xattr -dr com.apple.quarantine /Applications/gpix_app.app
```

To distribute without warnings you need an **Apple Developer ID** certificate and **notarization** (`codesign --deep --options runtime …` then `xcrun notarytool submit … && xcrun stapler staple`). That requires a paid Apple Developer account and signing secrets — out of scope for the local build here. For a prettier DMG (custom background, icon layout) you can also `brew install create-dmg` and point it at `build/macos/Build/Products/Release/gpix_app.app`.

> **Icon note:** use `bash tool/apply_icons.sh` rather than `dart run flutter_launcher_icons` directly. The plain command reads `pubspec.yaml`, which enables Android and iOS, so it fails with `PathNotFoundException: ... AppIcon.appiconset/Icon-App-20x20@1x.png` when only some platforms were generated. The script enables just the platform folders that exist. Linux isn't covered by flutter_launcher_icons; `tool/setup_linux.sh` and the packaging script handle its icon.

## Platform notes

- **macOS** is sandboxed and blocks both outbound network and file dialogs by default. Without the entitlements you get `SocketException: ... Operation not permitted, errno = 1` (can't reach the server) and the **Upload / Download file panels don't work**. After `flutter create`, run the helper (from `app/`):

  ```bash
  bash tool/setup_macos.sh      # then re-run: flutter run -d macos
  ```

  It adds `com.apple.security.network.client` **and** `com.apple.security.files.user-selected.read-write` to **both** `macos/Runner/DebugProfile.entitlements` (used by `flutter run`) and `Release.entitlements` (used by `flutter build`). By hand:

  ```xml
  <key>com.apple.security.network.client</key>
  <true/>
  <key>com.apple.security.files.user-selected.read-write</key>
  <true/>
  ```

- **Android + plain HTTP:** if your gpix server is `http://` (no TLS), Android blocks cleartext traffic on API 28+. Prefer **HTTPS**. If you must use HTTP, add `android:usesCleartextTraffic="true"` to `<application>` in `android/app/src/main/AndroidManifest.xml` (or a proper network-security-config). `INTERNET` permission is granted by default.
- **Photo backup permissions:** the Backup tab reads your photo library via `photo_manager`, which needs OS permission declarations the generated platform files don't include.
  - **Android** — add inside `<manifest>` in `android/app/src/main/AndroidManifest.xml`:

    ```xml
    <uses-permission android:name="android.permission.READ_MEDIA_IMAGES"/>
    <uses-permission android:name="android.permission.READ_MEDIA_VIDEO"/>
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" android:maxSdkVersion="32"/>
    ```

  - **iOS** — add to `ios/Runner/Info.plist`:

    ```xml
    <key>NSPhotoLibraryUsageDescription</key>
    <string>gpix backs up your photos and videos to your own server.</string>
    ```

  Backup runs while the app is open; true OS-scheduled background backup (Android WorkManager / iOS BGTaskScheduler) is a future addition.
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
