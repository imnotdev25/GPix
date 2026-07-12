import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';

/// Saves [bytes] to a user-chosen location. Returns the saved path/URI, or null
/// if cancelled. Handles the file_picker desktop/mobile split: on desktop it
/// returns a path we must write to; on mobile it writes the bytes itself.
Future<String?> saveBytes(String filename, Uint8List bytes) async {
  final result = await FilePicker.platform.saveFile(
    dialogTitle: 'Save file',
    fileName: filename.isEmpty ? 'download.bin' : filename,
    bytes: bytes,
  );
  if (result == null) return null;
  try {
    final f = File(result);
    if (!await f.exists() || (await f.length()) == 0) {
      await f.writeAsBytes(bytes);
    }
  } catch (_) {
    // On mobile the return value may be a content URI (already written by the
    // platform), which File() can't open — that's fine.
  }
  return result;
}
