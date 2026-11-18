import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart' show kIsWeb;

bool get isDesktopPlatform =>
    !kIsWeb && (Platform.isMacOS || Platform.isWindows || Platform.isLinux);

/// Saves [bytes] to a user-chosen location. Returns where it was saved, or
/// null if cancelled. file_picker (13+) writes the bytes itself on every
/// platform.
Future<String?> saveBytes(String filename, Uint8List bytes) async {
  final name = filename.isEmpty ? 'download.bin' : filename;
  final uri = await FilePicker.saveFile(
    dialogTitle: 'Save file',
    fileName: name,
    bytes: bytes,
  );
  if (uri == null) return null;
  return uri.scheme == 'file' ? uri.toFilePath() : uri.toString();
}

/// Desktop only: asks for a folder to save several files into.
Future<String?> pickFolder() async {
  final uri = await FilePicker.pickDirectory(dialogTitle: 'Save to folder');
  // Desktop pickers return a file URI; anything else isn't a writable path.
  return uri != null && uri.scheme == 'file' ? uri.toFilePath() : null;
}

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
