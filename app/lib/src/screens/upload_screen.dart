import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../state/app_state.dart';

class UploadScreen extends StatefulWidget {
  const UploadScreen({super.key});

  @override
  State<UploadScreen> createState() => _UploadScreenState();
}

class _UploadScreenState extends State<UploadScreen> {
  final List<UploadFile> _selected = [];
  String _quality = 'original';
  bool _busy = false;
  List<Map<String, dynamic>> _results = [];

  ApiClient get _client => context.read<AppState>().client!;

  Future<void> _pick() async {
    final res = await FilePicker.platform.pickFiles(allowMultiple: true);
    if (res == null) return;
    setState(() {
      for (final f in res.files) {
        if (f.path != null) _selected.add(UploadFile(f.path!, f.name));
      }
    });
  }

  Future<void> _upload() async {
    if (_selected.isEmpty) return;
    setState(() {
      _busy = true;
      _results = [];
    });
    try {
      final results = await _client.upload(_selected, quality: _quality);
      setState(() {
        _results = results;
        _selected.clear();
      });
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Upload failed: $e')));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Upload')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Row(
            children: [
              FilledButton.icon(
                onPressed: _busy ? null : _pick,
                icon: const Icon(Icons.add_photo_alternate_outlined),
                label: const Text('Choose files'),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: DropdownButtonFormField<String>(
                  value: _quality,
                  decoration: const InputDecoration(
                    labelText: 'Quality',
                    border: OutlineInputBorder(),
                    isDense: true,
                  ),
                  items: const [
                    DropdownMenuItem(value: 'original', child: Text('Original')),
                    DropdownMenuItem(value: 'saver', child: Text('Storage saver')),
                    DropdownMenuItem(value: 'quota', child: Text('Use quota')),
                  ],
                  onChanged: _busy
                      ? null
                      : (v) => setState(() => _quality = v ?? 'original'),
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          if (_selected.isNotEmpty) ...[
            Text('${_selected.length} file(s) ready',
                style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            ..._selected.map((f) => ListTile(
                  dense: true,
                  leading: const Icon(Icons.insert_drive_file_outlined),
                  title: Text(f.name, overflow: TextOverflow.ellipsis),
                  trailing: IconButton(
                    icon: const Icon(Icons.close),
                    onPressed: _busy
                        ? null
                        : () => setState(() => _selected.remove(f)),
                  ),
                )),
            const SizedBox(height: 12),
            FilledButton.icon(
              onPressed: _busy ? null : _upload,
              icon: _busy
                  ? const SizedBox(
                      height: 18,
                      width: 18,
                      child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.cloud_upload_outlined),
              label: Text(_busy ? 'Uploading…' : 'Upload'),
            ),
          ],
          if (_results.isNotEmpty) ...[
            const SizedBox(height: 24),
            Text('Results', style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            ..._results.map((r) => ListTile(
                  dense: true,
                  leading: Icon(
                    '${r['status']}'.contains('error')
                        ? Icons.error_outline
                        : Icons.check_circle_outline,
                    color: '${r['status']}'.contains('error')
                        ? Theme.of(context).colorScheme.error
                        : Colors.green,
                  ),
                  title: Text('${r['name']}', overflow: TextOverflow.ellipsis),
                  subtitle: Text('${r['status']}'),
                )),
          ],
          if (_selected.isEmpty && _results.isEmpty)
            const Padding(
              padding: EdgeInsets.only(top: 60),
              child: Center(
                child: Text('Pick photos or videos to back them up to gpix.'),
              ),
            ),
        ],
      ),
    );
  }
}
