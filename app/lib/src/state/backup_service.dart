import 'package:flutter/foundation.dart';
import 'package:photo_manager/photo_manager.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/api_client.dart';

/// BackupService backs up the device photo library to gpix, camera-roll style.
/// It remembers which assets were already uploaded (by the platform asset id)
/// so re-runs only copy what's new. Runs in the foreground; if "auto" is on it
/// starts a pass whenever the app opens.
class BackupService extends ChangeNotifier {
  BackupService(this.client) {
    _load();
  }

  final ApiClient client;

  static const _kEnabled = 'backup.enabled';
  static const _kVideos = 'backup.videos';
  static const _kDone = 'backup.done';

  bool _enabled = false;
  bool _includeVideos = false;
  bool running = false;
  bool _cancel = false;

  int total = 0; // device assets considered
  int pending = 0; // not yet backed up
  int done = 0; // uploaded in the current run
  int failed = 0; // failed in the current run
  String current = '';
  String? error;

  Set<String> _doneIds = <String>{};

  bool get enabled => _enabled;
  bool get includeVideos => _includeVideos;
  int get backedUp => _doneIds.length;

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    _enabled = prefs.getBool(_kEnabled) ?? false;
    _includeVideos = prefs.getBool(_kVideos) ?? false;
    _doneIds = (prefs.getStringList(_kDone) ?? const <String>[]).toSet();
    notifyListeners();
    if (_enabled) {
      // Fire-and-forget an automatic pass on launch.
      // ignore: unawaited_futures
      backupNow();
    }
  }

  Future<void> setEnabled(bool v) async {
    _enabled = v;
    (await SharedPreferences.getInstance()).setBool(_kEnabled, v);
    notifyListeners();
    if (v && !running) backupNow();
  }

  Future<void> setIncludeVideos(bool v) async {
    _includeVideos = v;
    (await SharedPreferences.getInstance()).setBool(_kVideos, v);
    notifyListeners();
  }

  Future<void> _persistDone() async {
    (await SharedPreferences.getInstance())
        .setStringList(_kDone, _doneIds.toList());
  }

  void stop() {
    _cancel = true;
  }

  /// Scans the library and uploads everything not already backed up.
  Future<void> backupNow() async {
    if (running) return;
    running = true;
    _cancel = false;
    error = null;
    done = 0;
    failed = 0;
    notifyListeners();

    try {
      final perm = await PhotoManager.requestPermissionExtend();
      if (!perm.isAuth && !perm.hasAccess) {
        error = 'Photo access not granted. Enable it in system settings.';
        return;
      }

      final type = _includeVideos ? RequestType.common : RequestType.image;
      final albums = await PhotoManager.getAssetPathList(onlyAll: true, type: type);
      if (albums.isEmpty) {
        total = 0;
        pending = 0;
        return;
      }
      final all = albums.first;
      total = await all.assetCountAsync;

      // Collect the pending assets (paged so we never hold the whole library).
      const pageSize = 200;
      final pages = (total / pageSize).ceil();
      final todo = <AssetEntity>[];
      for (var p = 0; p < pages; p++) {
        final batch = await all.getAssetListPaged(page: p, size: pageSize);
        for (final a in batch) {
          if (!_doneIds.contains(a.id)) todo.add(a);
        }
      }
      pending = todo.length;
      notifyListeners();

      for (final a in todo) {
        if (_cancel) break;
        final title = await a.titleAsync;
        current = title.isNotEmpty ? title : a.id;
        notifyListeners();

        final file = await a.originFile ?? await a.file;
        if (file == null) {
          failed++;
          notifyListeners();
          continue;
        }
        final name = title.isNotEmpty ? title : file.uri.pathSegments.last;
        try {
          await client.upload([UploadFile(file.path, name)]);
          _doneIds.add(a.id);
          done++;
          pending = pending > 0 ? pending - 1 : 0;
          if (done % 10 == 0) await _persistDone();
        } catch (e) {
          failed++;
          error = e.toString();
        }
        notifyListeners();
      }
      await _persistDone();
    } catch (e) {
      error = e.toString();
    } finally {
      running = false;
      current = '';
      notifyListeners();
    }
  }

  /// Forget the backed-up set (does not delete anything from gpix). The next
  /// pass will re-check every asset (dedup on the server still prevents dupes
  /// for unencrypted items).
  Future<void> resetState() async {
    _doneIds.clear();
    await _persistDone();
    notifyListeners();
  }
}
