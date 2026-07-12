import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart' show kIsWeb;

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
