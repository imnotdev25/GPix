import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart' show kIsWeb;

bool get isDesktopPlatform =>
    !kIsWeb && (Platform.isMacOS || Platform.isWindows || Platform.isLinux);

/// Saves [bytes] to a user-chosen location. Returns the saved path/URI, or null
/// if cancelled.
///
/// file_picker's saveFile has two modes: on **desktop** it returns the chosen
/// path and you write the bytes yourself (passing `bytes` there throws
/// "bytes are not supported"); on **mobile/web** you must pass `bytes` and the
/// platform writes them.
Future<String?> saveBytes(String filename, Uint8List bytes) async {
  final name = filename.isEmpty ? 'download.bin' : filename;
  final mobile = !kIsWeb && (Platform.isAndroid || Platform.isIOS);

  final path = await FilePicker.platform.saveFile(
    dialogTitle: 'Save file',
    fileName: name,
    bytes: mobile ? bytes : null,
  );
  if (path == null) return null;

  if (!mobile) {
    // Desktop: file_picker only returned the chosen path — write it ourselves.
    await File(path).writeAsBytes(bytes);
  }
  return path;
}

/// Desktop only: asks for a folder to save several files into.
Future<String?> pickFolder() =>
    FilePicker.platform.getDirectoryPath(dialogTitle: 'Save to folder');

/// Writes [bytes] into [dir] as [name], adding " (1)", " (2)"… instead of
/// overwriting an existing file. Returns the path written.
Future<String> writeUnique(String dir, String name, Uint8List bytes) async {
  final safe = (name.isEmpty ? 'download.bin' : name)
      .replaceAll(RegExp(r'[\\/:*?"<>|]'), '_');
  final dot = safe.lastIndexOf('.');
  final stem = dot > 0 ? safe.substring(0, dot) : safe;
  final ext = dot > 0 ? safe.substring(dot) : '';
  final sep = Platform.pathSeparator;
  var path = '$dir$sep$safe';
  for (var i = 1; await File(path).exists(); i++) {
    path = '$dir$sep$stem ($i)$ext';
  }
  await File(path).writeAsBytes(bytes);
  return path;
}
