// Dialogs for albums and tags (stored by gpix, not synced with Google Photos).

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/models.dart';

/// Lets the user pick an album, or make a new one, and adds [keys] to it.
/// Returns that album, or null if cancelled.
Future<Album?> addToAlbum(
  BuildContext context,
  ApiClient client,
  List<String> keys,
) => showDialog<Album>(
  context: context,
  builder: (_) => _AlbumPicker(client: client, keys: keys),
);

/// Edits tags. With one key and [initial] (its current tags) it also lists
/// and removes tags; with several keys it only adds. Returns every key's tags
/// after the change, or null if nothing changed.
Future<Map<String, List<String>>?> editTags(
  BuildContext context,
  ApiClient client,
  List<String> keys, {
  List<String>? initial,
}) async {
  Map<String, List<String>>? result;
  await showDialog<void>(
    context: context,
    builder: (_) => _TagDialog(
      client: client,
      keys: keys,
      initial: initial,
      onChanged: (m) => result = m,
    ),
  );
  return result;
}

/// Asks for a name (new album, rename). Returns it trimmed, or null.
Future<String?> promptName(
  BuildContext context, {
  required String title,
  required String action,
  String initial = '',
}) => showDialog<String>(
  context: context,
  builder: (_) => _NameDialog(title: title, action: action, initial: initial),
);

/// "beach, #Family " -> ["beach", "family"], the way the server stores tags.
List<String> parseTags(String s) => s
    .split(',')
    .map(
      (t) => t
          .trim()
          .replaceFirst(RegExp(r'^#+'), '')
          .replaceAll(RegExp(r'\s+'), ' ')
          .toLowerCase(),
    )
    .where((t) => t.isNotEmpty)
    .toList();

Widget _errorText(BuildContext context, String msg) => Padding(
  padding: const EdgeInsets.only(top: 10),
  child: Text(msg, style: TextStyle(color: Theme.of(context).colorScheme.error)),
);

// ---- add to album -------------------------------------------------------------

class _AlbumPicker extends StatefulWidget {
  const _AlbumPicker({required this.client, required this.keys});
  final ApiClient client;
  final List<String> keys;

  @override
  State<_AlbumPicker> createState() => _AlbumPickerState();
}

class _AlbumPickerState extends State<_AlbumPicker> {
  final _name = TextEditingController();
  List<Album>? _albums;
  int? _choice; // null: a new album
  bool _busy = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final list = await widget.client.listAlbums();
      if (!mounted) return;
      setState(() {
        _albums = list;
        _choice = list.isEmpty ? null : list.first.id;
      });
    } catch (e) {
      if (mounted) {
        setState(() {
          _albums = const [];
          _error = e.toString();
        });
      }
    }
  }

  Future<void> _submit() async {
    final name = _name.text.trim();
    final choice = _choice;
    if (choice == null && name.isEmpty) {
      setState(() => _error = 'Give the new album a name.');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final Album album;
      if (choice == null) {
        album = await widget.client.createAlbum(name, keys: widget.keys);
      } else {
        await widget.client.addToAlbum(choice, widget.keys);
        album = _albums!.firstWhere((a) => a.id == choice);
      }
      if (mounted) Navigator.pop(context, album);
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e.toString();
        });
      }
    }
  }

  Widget _option({
    required bool on,
    required String title,
    String? trailing,
    required VoidCallback onTap,
  }) => ListTile(
    contentPadding: EdgeInsets.zero,
    dense: true,
    leading: Icon(
      on ? Icons.radio_button_checked : Icons.radio_button_unchecked,
      color: on ? Theme.of(context).colorScheme.primary : null,
    ),
    title: Text(title, overflow: TextOverflow.ellipsis),
    trailing: trailing == null ? null : Text(trailing),
    onTap: _busy ? null : onTap,
  );

  @override
  Widget build(BuildContext context) {
    final albums = _albums;
    final n = widget.keys.length;
    return AlertDialog(
      title: Text(n == 1 ? 'Add to album' : 'Add $n items to an album'),
      content: SizedBox(
        width: 380,
        child: albums == null
            ? const SizedBox(
                height: 80,
                child: Center(child: CircularProgressIndicator()),
              )
            : SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    for (final a in albums)
                      _option(
                        on: _choice == a.id,
                        title: a.name,
                        trailing: '${a.count}',
                        onTap: () => setState(() => _choice = a.id),
                      ),
                    _option(
                      on: _choice == null,
                      title: 'New album…',
                      onTap: () => setState(() => _choice = null),
                    ),
                    if (_choice == null) ...[
                      const SizedBox(height: 8),
                      TextField(
                        controller: _name,
                        autofocus: true,
                        maxLength: 100,
                        enabled: !_busy,
                        decoration: const InputDecoration(
                          labelText: 'Album name',
                          hintText: 'e.g. Summer 2026',
                        ),
                        onSubmitted: (_) => _submit(),
                      ),
                    ],
                    if (_error != null) _errorText(context, _error!),
                  ],
                ),
              ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _busy || albums == null ? null : _submit,
          child: Text(_busy ? 'Adding…' : 'Add'),
        ),
      ],
    );
  }
}

// ---- tags ----------------------------------------------------------------------

class _TagDialog extends StatefulWidget {
  const _TagDialog({
    required this.client,
    required this.keys,
    required this.initial,
    required this.onChanged,
  });
  final ApiClient client;
  final List<String> keys;
  final List<String>? initial;
  final void Function(Map<String, List<String>> tags) onChanged;

  @override
  State<_TagDialog> createState() => _TagDialogState();
}

class _TagDialogState extends State<_TagDialog> {
  final _text = TextEditingController();
  late List<String> _tags = List.of(widget.initial ?? const <String>[]);
  List<String> _known = const [];
  bool _busy = false;
  String? _error;

  bool get _single => widget.initial != null && widget.keys.length == 1;

  @override
  void initState() {
    super.initState();
    _loadKnown();
  }

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  Future<void> _loadKnown() async {
    try {
      final list = await widget.client.listTags();
      if (mounted) setState(() => _known = [for (final t in list) t.tag]);
    } catch (_) {
      // suggestions are optional
    }
  }

  Future<void> _change({required bool add, required List<String> tags}) async {
    if (tags.isEmpty || _busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final next = add
          ? await widget.client.addTags(widget.keys, tags)
          : await widget.client.removeTags(widget.keys, tags);
      widget.onChanged(next);
      if (!mounted) return;
      if (!_single) {
        Navigator.pop(context);
        return;
      }
      setState(() {
        _tags = next[widget.keys.first] ?? const [];
        _text.clear();
        _busy = false;
      });
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e.toString();
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final suggestions = _known.where((t) => !_tags.contains(t)).take(12).toList();
    return AlertDialog(
      title: Text(_single ? 'Tags' : 'Tag ${widget.keys.length} items'),
      content: SizedBox(
        width: 380,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (_single) ...[
                if (_tags.isEmpty)
                  const Text('No tags yet.')
                else
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      for (final t in _tags)
                        InputChip(
                          label: Text('#$t'),
                          deleteButtonTooltipMessage: 'Remove $t',
                          onDeleted: _busy
                              ? null
                              : () => _change(add: false, tags: [t]),
                        ),
                    ],
                  ),
                const SizedBox(height: 12),
              ],
              TextField(
                controller: _text,
                autofocus: true,
                enabled: !_busy,
                decoration: const InputDecoration(
                  labelText: 'Add tags, separated by commas',
                  hintText: 'beach, family',
                ),
                onSubmitted: (v) => _change(add: true, tags: parseTags(v)),
              ),
              if (suggestions.isNotEmpty) ...[
                const SizedBox(height: 14),
                Text('Your tags', style: Theme.of(context).textTheme.labelLarge),
                const SizedBox(height: 6),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    for (final t in suggestions)
                      ActionChip(
                        label: Text('#$t'),
                        onPressed: _busy
                            ? null
                            : () => _change(add: true, tags: [t]),
                      ),
                  ],
                ),
              ],
              if (_error != null) _errorText(context, _error!),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(_single ? 'Done' : 'Cancel'),
        ),
        ListenableBuilder(
          listenable: _text,
          builder: (context, _) => FilledButton(
            onPressed: _busy || _text.text.trim().isEmpty
                ? null
                : () => _change(add: true, tags: parseTags(_text.text)),
            child: Text(_busy ? 'Saving…' : 'Add'),
          ),
        ),
      ],
    );
  }
}

// ---- name ----------------------------------------------------------------------

class _NameDialog extends StatefulWidget {
  const _NameDialog({
    required this.title,
    required this.action,
    required this.initial,
  });
  final String title;
  final String action;
  final String initial;

  @override
  State<_NameDialog> createState() => _NameDialogState();
}

class _NameDialogState extends State<_NameDialog> {
  late final _c = TextEditingController(text: widget.initial);

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  void _submit() {
    final v = _c.text.trim();
    if (v.isNotEmpty) Navigator.pop(context, v);
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.title),
      content: SizedBox(
        width: 360,
        child: TextField(
          controller: _c,
          autofocus: true,
          maxLength: 100,
          decoration: const InputDecoration(labelText: 'Name'),
          onSubmitted: (_) => _submit(),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        ListenableBuilder(
          listenable: _c,
          builder: (context, _) => FilledButton(
            onPressed: _c.text.trim().isEmpty ? null : _submit,
            child: Text(widget.action),
          ),
        ),
      ],
    );
  }
}
