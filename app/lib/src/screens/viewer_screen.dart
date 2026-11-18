import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import '../theme.dart';
import '../util/collections.dart';
import '../util/dialogs.dart';
import '../util/download.dart';
import '../util/format.dart';
import '../widgets/thumb_image.dart';

/// Full-screen viewer. Swipe (touch) or ←/→ (keyboard) between items, "i" for
/// details, Esc to close. [onDeleted] lets the library drop deleted items
/// without reloading.
class ViewerScreen extends StatefulWidget {
  const ViewerScreen({
    super.key,
    required this.items,
    required this.initialIndex,
    this.onDeleted,
    this.onTagsChanged,
  });
  final List<MediaItem> items;
  final int initialIndex;
  final void Function(MediaItem item)? onDeleted;
  final void Function(MediaItem item, List<String> tags)? onTagsChanged;

  @override
  State<ViewerScreen> createState() => _ViewerScreenState();
}

class _ViewerScreenState extends State<ViewerScreen> {
  late final List<MediaItem> _items = List.of(widget.items);
  late int _index = widget.initialIndex.clamp(0, _items.length - 1);
  late PageController _controller = PageController(initialPage: _index);
  bool _showInfo = false; // side panel on wide screens

  ApiClient get _client => context.read<AppState>().client!;
  MediaItem get _current => _items[_index];

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _precache());
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  /// Warms the image cache for the neighbours so swiping is instant.
  void _precache() {
    if (!mounted) return;
    for (final i in [_index - 1, _index + 1]) {
      if (i < 0 || i >= _items.length) continue;
      final it = _items[i];
      // Pending items only have a placeholder yet; ThumbImage retries them.
      if (!it.hasRenderedImage || it.thumbPending) continue;
      precacheImage(
        CachedNetworkImageProvider(
          _client.thumbUrl(it, size: 2048),
          headers: _client.authHeaders,
        ),
        context,
        onError: (_, __) {},
      );
    }
  }

  void _onPageChanged(int i) {
    setState(() => _index = i);
    _precache();
  }

  void _go(int delta) {
    final target = _index + delta;
    if (target < 0 || target >= _items.length) return;
    _controller.animateToPage(
      target,
      duration: const Duration(milliseconds: 200),
      curve: Curves.easeOut,
    );
  }

  KeyEventResult _onKey(FocusNode _, KeyEvent e) {
    if (e is! KeyDownEvent && e is! KeyRepeatEvent) {
      return KeyEventResult.ignored;
    }
    final k = e.logicalKey;
    if (k == LogicalKeyboardKey.arrowLeft) {
      _go(-1);
    } else if (k == LogicalKeyboardKey.arrowRight) {
      _go(1);
    } else if (k == LogicalKeyboardKey.escape && e is KeyDownEvent) {
      Navigator.of(context).maybePop();
    } else if (k == LogicalKeyboardKey.keyI && e is KeyDownEvent) {
      _toggleInfo();
    } else {
      return KeyEventResult.ignored;
    }
    return KeyEventResult.handled;
  }

  bool get _wide => MediaQuery.sizeOf(context).width >= 900;

  void _toggleInfo() {
    if (_wide) {
      setState(() => _showInfo = !_showInfo);
    } else {
      showModalBottomSheet<void>(
        context: context,
        builder: (sheet) => SafeArea(
          child: _InfoPanel(
            item: _current,
            onEditTags: () {
              Navigator.pop(sheet);
              _editTags();
            },
          ),
        ),
      );
    }
  }

  Future<void> _delete() async {
    final item = _current;
    final ok = await confirmDestructive(
      context,
      title: 'Move to trash?',
      message:
          '"${item.name}" stays recoverable in the Google Photos trash '
          'for 60 days.',
      action: 'Move to trash',
    );
    if (!ok || !mounted) return;
    try {
      await _client.deleteMedia(item.key);
    } catch (e) {
      if (mounted) showSnack(context, 'Delete failed: $e');
      return;
    }
    widget.onDeleted?.call(item);
    if (!mounted) return;
    if (_items.length == 1) {
      Navigator.of(context).pop();
      return;
    }
    final old = _controller;
    setState(() {
      _items.removeAt(_index);
      _index = _index.clamp(0, _items.length - 1);
      _controller = PageController(initialPage: _index);
    });
    WidgetsBinding.instance.addPostFrameCallback((_) => old.dispose());
    showSnack(context, 'Moved to trash');
    _precache();
  }

  Future<void> _download() async {
    final item = _current;
    showSnack(context, 'Downloading ${item.name}…');
    try {
      final bytes = await _client.fetchOriginal(item);
      final saved = await saveBytes(item.name, bytes);
      if (mounted) {
        showSnack(context, saved == null ? 'Cancelled' : 'Saved ${item.name}');
      }
    } catch (e) {
      if (mounted) showSnack(context, 'Download failed: $e');
    }
  }

  Future<void> _editTags() async {
    final item = _current;
    final tags = await editTags(context, _client, [item.key], initial: item.tags);
    final t = tags?[item.key];
    if (t == null || !mounted) return;
    setState(() {
      final i = _items.indexWhere((x) => x.key == item.key);
      if (i >= 0) _items[i] = _items[i].copyWith(tags: t);
    });
    widget.onTagsChanged?.call(item, t);
  }

  Future<void> _addToAlbum() async {
    final album = await addToAlbum(context, _client, [_current.key]);
    if (album != null && mounted) showSnack(context, 'Added to ${album.name}');
  }

  @override
  Widget build(BuildContext context) {
    final wide = _wide;
    final item = _current;
    return Focus(
      autofocus: true,
      onKeyEvent: _onKey,
      child: Scaffold(
        backgroundColor: Colors.black,
        appBar: AppBar(
          backgroundColor: Colors.black,
          foregroundColor: Colors.white,
          titleTextStyle: const TextStyle(
            color: Colors.white,
            fontSize: 16,
            fontWeight: FontWeight.w600,
          ),
          title: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(item.name, overflow: TextOverflow.ellipsis),
              Text(
                '${_index + 1} of ${_items.length}',
                style: const TextStyle(
                  color: Colors.white60,
                  fontSize: 12,
                  fontWeight: FontWeight.w400,
                ),
              ),
            ],
          ),
          actions: [
            IconButton(
              tooltip: 'Details (i)',
              isSelected: wide && _showInfo,
              icon: const Icon(Icons.info_outline),
              selectedIcon: const Icon(Icons.info),
              onPressed: _toggleInfo,
            ),
            IconButton(
              tooltip: 'Share',
              icon: const Icon(Icons.share_outlined),
              onPressed: () => shareItems(context, _client, [item]),
            ),
            IconButton(
              tooltip: 'Download original',
              icon: const Icon(Icons.download_outlined),
              onPressed: _download,
            ),
            IconButton(
              tooltip: 'Move to trash',
              icon: const Icon(Icons.delete_outline),
              onPressed: _delete,
            ),
            PopupMenuButton<String>(
              tooltip: 'More',
              onSelected: (v) {
                if (v == 'album') {
                  _addToAlbum();
                } else if (v == 'tags') {
                  _editTags();
                }
              },
              itemBuilder: (_) => const [
                PopupMenuItem(
                  value: 'album',
                  child: ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(Icons.photo_album_outlined),
                    title: Text('Add to album…'),
                  ),
                ),
                PopupMenuItem(
                  value: 'tags',
                  child: ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(Icons.sell_outlined),
                    title: Text('Tags…'),
                  ),
                ),
              ],
            ),
            const SizedBox(width: 4),
          ],
        ),
        body: Row(
          children: [
            Expanded(
              child: Stack(
                children: [
                  PageView.builder(
                    controller: _controller,
                    itemCount: _items.length,
                    onPageChanged: _onPageChanged,
                    itemBuilder: (context, i) => _ViewerPage(
                      item: _items[i],
                      client: _client,
                      onDownload: _download,
                    ),
                  ),
                  if (wide && _index > 0)
                    Align(
                      alignment: Alignment.centerLeft,
                      child: _NavButton(
                        icon: Icons.chevron_left,
                        onTap: () => _go(-1),
                      ),
                    ),
                  if (wide && _index < _items.length - 1)
                    Align(
                      alignment: Alignment.centerRight,
                      child: _NavButton(
                        icon: Icons.chevron_right,
                        onTap: () => _go(1),
                      ),
                    ),
                ],
              ),
            ),
            if (wide && _showInfo)
              Container(
                width: 320,
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.surface,
                  border: Border(left: BorderSide(color: context.gpix.border)),
                ),
                child: _InfoPanel(item: item, onEditTags: _editTags),
              ),
          ],
        ),
      ),
    );
  }
}

class _NavButton extends StatelessWidget {
  const _NavButton({required this.icon, required this.onTap});
  final IconData icon;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Material(
        color: Colors.white.withValues(alpha: 0.12),
        shape: const CircleBorder(),
        child: IconButton(
          icon: Icon(icon, color: Colors.white),
          onPressed: onTap,
        ),
      ),
    );
  }
}

class _ViewerPage extends StatelessWidget {
  const _ViewerPage({
    required this.item,
    required this.client,
    required this.onDownload,
  });
  final MediaItem item;
  final ApiClient client;
  final VoidCallback onDownload;

  @override
  Widget build(BuildContext context) {
    if (item.hasRenderedImage) {
      return InteractiveViewer(
        minScale: 1,
        maxScale: 5,
        child: Center(
          child: item.thumbPending
              // Still rendering on the server (encrypted item): retry until
              // the real image arrives.
              ? ThumbImage(
                  item: item,
                  client: client,
                  size: 2048,
                  fit: BoxFit.contain,
                  placeholder: const Center(child: CircularProgressIndicator()),
                  error: const Center(
                    child: Icon(
                      Icons.broken_image_outlined,
                      size: 48,
                      color: Colors.white54,
                    ),
                  ),
                )
              : CachedNetworkImage(
                  imageUrl: client.thumbUrl(item, size: 2048),
                  httpHeaders: client.authHeaders,
                  fit: BoxFit.contain,
                  // Show the (already-cached) grid thumbnail instantly while the larger
                  // image loads, so the viewer is never blank.
                  placeholder: (_, __) => CachedNetworkImage(
                    imageUrl: client.thumbUrl(item, size: 256),
                    httpHeaders: client.authHeaders,
                    fit: BoxFit.contain,
                    errorWidget: (_, __, ___) =>
                        const Center(child: CircularProgressIndicator()),
                  ),
                  errorWidget: (_, __, ___) => const Center(
                    child: Icon(
                      Icons.broken_image_outlined,
                      size: 48,
                      color: Colors.white54,
                    ),
                  ),
                ),
        ),
      );
    }
    // Video / other files: no in-app playback yet — offer download.
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              item.isVideo
                  ? Icons.movie_outlined
                  : Icons.insert_drive_file_outlined,
              size: 72,
              color: Colors.white70,
            ),
            const SizedBox(height: 16),
            Text(
              item.name,
              textAlign: TextAlign.center,
              style: const TextStyle(color: Colors.white),
            ),
            const SizedBox(height: 24),
            FilledButton.icon(
              onPressed: onDownload,
              icon: const Icon(Icons.download),
              label: const Text('Download original'),
            ),
          ],
        ),
      ),
    );
  }
}

class _InfoPanel extends StatelessWidget {
  const _InfoPanel({required this.item, this.onEditTags});
  final MediaItem item;
  final VoidCallback? onEditTags;

  String get _type {
    if (item.isPhoto) return 'Photo';
    if (item.isVideo) return 'Video';
    if (item.isDoc) return 'Document';
    return 'File';
  }

  @override
  Widget build(BuildContext context) {
    final muted = context.gpix.muted;
    Widget row(IconData icon, String label, String value) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, size: 20, color: muted),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(label, style: TextStyle(color: muted, fontSize: 12)),
                const SizedBox(height: 2),
                SelectableText(value),
              ],
            ),
          ),
        ],
      ),
    );

    return ListView(
      shrinkWrap: true,
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
      children: [
        Text('Details', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        row(Icons.insert_drive_file_outlined, 'Name', item.name),
        if (item.mtime != null)
          row(
            Icons.event_outlined,
            'Date',
            formatDateTime(context, item.mtime!),
          ),
        row(Icons.straighten_outlined, 'Size', formatBytes(item.size)),
        row(Icons.category_outlined, 'Type', _type),
        row(
          item.encrypted ? Icons.lock_outline : Icons.lock_open_outlined,
          'Encryption',
          item.encrypted ? 'Encrypted with your gpix key' : 'Not encrypted',
        ),
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 8),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Icon(Icons.sell_outlined, size: 20, color: muted),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Tags', style: TextStyle(color: muted, fontSize: 12)),
                    const SizedBox(height: 6),
                    Wrap(
                      spacing: 6,
                      runSpacing: 6,
                      children: [
                        for (final t in item.tags) Chip(label: Text('#$t')),
                        if (onEditTags != null)
                          ActionChip(
                            avatar: const Icon(Icons.edit_outlined, size: 16),
                            label: Text(item.tags.isEmpty ? 'Add tags' : 'Edit'),
                            onPressed: onEditTags,
                          ),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
