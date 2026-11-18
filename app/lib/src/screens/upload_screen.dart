import 'package:desktop_drop/desktop_drop.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../state/app_state.dart';
import '../theme.dart';
import '../util/dialogs.dart';
import '../util/download.dart';
import '../util/format.dart';

enum _Status { queued, uploading, done, duplicate, failed }

class _Entry {
  _Entry(this.path, this.name, this.size);
  final String path;
  final String name;
  final int size;
  _Status status = _Status.queued;
  String detail = '';
}

class UploadScreen extends StatefulWidget {
  const UploadScreen({super.key});

  @override
  State<UploadScreen> createState() => _UploadScreenState();
}

class _UploadScreenState extends State<UploadScreen> {
  final List<_Entry> _entries = [];
  List<_Entry> _batch = const []; // entries in the current upload run
  String _quality = 'original';
  bool _busy = false;
  bool _stop = false;
  bool _dragging = false;

  ApiClient get _client => context.read<AppState>().client!;

  void _add(Iterable<_Entry> files) {
    final known = {for (final e in _entries) e.path};
    setState(() {
      for (final f in files) {
        if (known.add(f.path)) _entries.add(f);
      }
    });
  }

  Future<void> _pick() async {
    // Picks several files (the default). On Android each is copied into the
    // app cache, so it has a real path.
    final files = await FilePicker.pickFiles();
    _add([
      for (final f in files)
        if (f.path != null) _Entry(f.path!, f.name, f.lengthSync() ?? 0),
    ]);
  }

  Future<void> _onDrop(DropDoneDetails d) async {
    setState(() => _dragging = false);
    final files = <_Entry>[];
    var skipped = 0;
    for (final x in d.files) {
      if (x is DropItemDirectory) {
        skipped++;
        continue;
      }
      try {
        files.add(_Entry(x.path, x.name, await x.length()));
      } catch (_) {
        skipped++;
      }
    }
    _add(files);
    if (skipped > 0 && mounted) {
      showSnack(context, 'Skipped $skipped folder(s) or unreadable item(s)');
    }
  }

  _Status _statusFor(String s) {
    if (s.startsWith('already')) return _Status.duplicate;
    if (s.startsWith('uploaded')) return _Status.done;
    return _Status.failed;
  }

  Future<void> _upload() async {
    final todo = _entries
        .where((e) => e.status == _Status.queued || e.status == _Status.failed)
        .toList();
    if (todo.isEmpty) return;
    setState(() {
      _busy = true;
      _stop = false;
      _batch = todo;
      for (final e in todo) {
        e.status = _Status.queued;
        e.detail = '';
      }
    });
    for (final e in todo) {
      if (_stop || !mounted) break;
      setState(() => e.status = _Status.uploading);
      try {
        final res = await _client.upload([UploadFile(e.path, e.name)],
            quality: _quality);
        final status = res.isEmpty ? 'error' : '${res.first['status']}';
        e.status = _statusFor(status);
        e.detail = status;
      } catch (err) {
        e.status = _Status.failed;
        e.detail = err.toString();
      }
      if (mounted) setState(() {});
    }
    if (!mounted) return;
    final done = todo.where((e) => e.status == _Status.done).length;
    final dup = todo.where((e) => e.status == _Status.duplicate).length;
    final failed = todo.where((e) => e.status == _Status.failed).length;
    setState(() => _busy = false);
    showSnack(
      context,
      [
        if (_stop) 'Stopped',
        '$done uploaded',
        if (dup > 0) '$dup already in library',
        if (failed > 0) '$failed failed',
      ].join(' · '),
    );
  }

  double get _progress {
    final total = _batch.fold<int>(0, (n, e) => n + e.size);
    final finished = _batch
        .where((e) => e.status != _Status.queued && e.status != _Status.uploading);
    if (total == 0) {
      return _batch.isEmpty ? 0 : finished.length / _batch.length;
    }
    return finished.fold<int>(0, (n, e) => n + e.size) / total;
  }

  @override
  Widget build(BuildContext context) {
    final pending = _entries
        .where((e) => e.status == _Status.queued || e.status == _Status.failed)
        .length;
    final finished = _entries
        .where((e) => e.status == _Status.done || e.status == _Status.duplicate)
        .length;

    final body = ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _DropZone(
          dragging: _dragging,
          desktop: isDesktopPlatform,
          onPick: _busy ? null : _pick,
        ),
        const SizedBox(height: 16),
        Row(
          children: [
            Text('Quality', style: Theme.of(context).textTheme.labelLarge),
            const SizedBox(width: 12),
            Expanded(
              child: SingleChildScrollView(
                scrollDirection: Axis.horizontal,
                child: SegmentedButton<String>(
                  showSelectedIcon: false,
                  segments: const [
                    ButtonSegment(value: 'original', label: Text('Original')),
                    ButtonSegment(value: 'saver', label: Text('Storage saver')),
                    ButtonSegment(value: 'quota', label: Text('Use quota')),
                  ],
                  selected: {_quality},
                  onSelectionChanged:
                      _busy ? null : (s) => setState(() => _quality = s.first),
                ),
              ),
            ),
          ],
        ),
        if (_entries.isNotEmpty) ...[
          const SizedBox(height: 20),
          Row(
            children: [
              Expanded(
                child: Text(_summary(pending, finished),
                    style: Theme.of(context).textTheme.titleSmall),
              ),
              if (finished > 0 && !_busy)
                TextButton(
                  onPressed: () => setState(() => _entries.removeWhere((e) =>
                      e.status == _Status.done ||
                      e.status == _Status.duplicate)),
                  child: const Text('Clear finished'),
                ),
            ],
          ),
          if (_busy) ...[
            const SizedBox(height: 8),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(value: _progress, minHeight: 6),
            ),
          ],
          const SizedBox(height: 8),
          Card(
            clipBehavior: Clip.antiAlias,
            child: Column(
              children: [
                for (var i = 0; i < _entries.length; i++) ...[
                  if (i > 0) const Divider(height: 1),
                  _EntryTile(
                    entry: _entries[i],
                    onRemove: _busy || _entries[i].status == _Status.uploading
                        ? null
                        : () => setState(() => _entries.removeAt(i)),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              FilledButton.icon(
                onPressed: _busy || pending == 0 ? null : _upload,
                icon: const Icon(Icons.cloud_upload_outlined),
                label: Text(pending == 0
                    ? 'Upload'
                    : 'Upload $pending file${pending == 1 ? '' : 's'}'),
              ),
              if (_busy) ...[
                const SizedBox(width: 12),
                OutlinedButton(
                  onPressed:
                      _stop ? null : () => setState(() => _stop = true),
                  child: Text(_stop ? 'Stopping…' : 'Stop after this file'),
                ),
              ],
            ],
          ),
        ],
      ],
    );

    return Scaffold(
      appBar: AppBar(title: const Text('Upload')),
      body: isDesktopPlatform
          ? DropTarget(
              onDragEntered: (_) => setState(() => _dragging = true),
              onDragExited: (_) => setState(() => _dragging = false),
              onDragDone: _onDrop,
              child: body,
            )
          : body,
    );
  }

  String _summary(int pending, int finished) {
    final failed = _entries.where((e) => e.status == _Status.failed).length;
    return [
      '${_entries.length} file${_entries.length == 1 ? '' : 's'}',
      if (finished > 0) '$finished done',
      if (failed > 0) '$failed failed',
    ].join(' · ');
  }
}

class _EntryTile extends StatelessWidget {
  const _EntryTile({required this.entry, required this.onRemove});
  final _Entry entry;
  final VoidCallback? onRemove;

  @override
  Widget build(BuildContext context) {
    final muted = context.gpix.muted;
    final scheme = Theme.of(context).colorScheme;
    final (Widget icon, String label) = switch (entry.status) {
      _Status.queued => (Icon(Icons.schedule, color: muted), 'Queued'),
      _Status.uploading => (
          const SizedBox(
              width: 20,
              height: 20,
              child: CircularProgressIndicator(strokeWidth: 2)),
          'Uploading…'
        ),
      _Status.done => (
          Icon(Icons.check_circle_outline, color: context.gpix.ok),
          _cap(entry.detail.isEmpty ? 'uploaded' : entry.detail)
        ),
      _Status.duplicate => (
          Icon(Icons.copy_all_outlined, color: muted),
          'Already in library'
        ),
      _Status.failed => (
          Icon(Icons.error_outline, color: scheme.error),
          entry.detail.isEmpty ? 'Failed' : _cap(entry.detail)
        ),
    };
    return ListTile(
      dense: true,
      leading: SizedBox(width: 24, child: Center(child: icon)),
      title: Text(entry.name, overflow: TextOverflow.ellipsis),
      subtitle: Text('${formatBytes(entry.size)} · $label',
          maxLines: 2, overflow: TextOverflow.ellipsis),
      trailing: onRemove == null
          ? null
          : IconButton(
              tooltip: 'Remove',
              icon: const Icon(Icons.close, size: 18),
              onPressed: onRemove,
            ),
    );
  }

  static String _cap(String s) =>
      s.isEmpty ? s : s[0].toUpperCase() + s.substring(1);
}

class _DropZone extends StatelessWidget {
  const _DropZone(
      {required this.dragging, required this.desktop, required this.onPick});
  final bool dragging;
  final bool desktop;
  final VoidCallback? onPick;

  @override
  Widget build(BuildContext context) {
    final g = context.gpix;
    return AnimatedContainer(
      duration: const Duration(milliseconds: 150),
      padding: const EdgeInsets.symmetric(vertical: 32, horizontal: 20),
      decoration: BoxDecoration(
        color: dragging
            ? GpixPalette.accent.withValues(alpha: 0.08)
            : Theme.of(context).colorScheme.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: dragging ? GpixPalette.accent : g.border,
          width: dragging ? 2 : 1,
        ),
      ),
      child: Column(
        children: [
          Icon(Icons.cloud_upload_outlined,
              size: 40, color: dragging ? GpixPalette.accent : g.muted),
          const SizedBox(height: 10),
          Text(
            desktop
                ? (dragging ? 'Drop to add' : 'Drag photos, videos or files here')
                : 'Pick photos, videos or files to back up',
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.titleSmall,
          ),
          const SizedBox(height: 4),
          Text(
            'Encryption and file wrapping apply if enabled on the server; '
            'duplicates are skipped.',
            textAlign: TextAlign.center,
            style: TextStyle(color: g.muted, fontSize: 12),
          ),
          const SizedBox(height: 14),
          FilledButton.icon(
            onPressed: onPick,
            icon: const Icon(Icons.add_photo_alternate_outlined),
            label: const Text('Choose files'),
          ),
        ],
      ),
    );
  }
}
