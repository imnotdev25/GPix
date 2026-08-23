import 'dart:async';

import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import 'viewer_screen.dart';

class LibraryScreen extends StatefulWidget {
  const LibraryScreen({super.key});

  @override
  State<LibraryScreen> createState() => _LibraryScreenState();
}

class _LibraryScreenState extends State<LibraryScreen> {
  final _scroll = ScrollController();
  final List<MediaItem> _items = [];
  String _cursor = '';
  bool _loading = false;
  bool _hasMore = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _scroll.addListener(_onScroll);
    WidgetsBinding.instance.addPostFrameCallback((_) => _loadMore());
  }

  @override
  void dispose() {
    _scroll.dispose();
    super.dispose();
  }

  ApiClient get _client => context.read<AppState>().client!;

  void _onScroll() {
    if (_scroll.position.pixels >=
        _scroll.position.maxScrollExtent - 600) {
      _loadMore();
    }
  }

  Future<void> _refresh() async {
    setState(() {
      _items.clear();
      _cursor = '';
      _hasMore = true;
      _error = null;
    });
    await _loadMore();
  }

  Future<void> _loadMore() async {
    if (_loading || !_hasMore) return;
    setState(() => _loading = true);
    try {
      final page = await _client.listMedia(cursor: _cursor);
      setState(() {
        _items.addAll(page.items);
        _cursor = page.nextCursor;
        _hasMore = page.nextCursor.isNotEmpty;
        _error = null;
      });
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _openViewer(int index) async {
    final changed = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => ViewerScreen(items: List.of(_items), initialIndex: index),
      ),
    );
    if (changed == true) _refresh();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(_items.isNotEmpty ? 'Library · ${_items.length} loaded' : 'Library'),
        actions: [
          IconButton(
            onPressed: _refresh,
            icon: const Icon(Icons.refresh),
            tooltip: 'Refresh',
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: _buildBody(),
      ),
    );
  }

  Widget _buildBody() {
    if (_items.isEmpty && _loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_items.isEmpty && _error != null) {
      return _ErrorState(message: _error!, onRetry: _refresh);
    }
    if (_items.isEmpty) {
      return ListView(
        children: const [
          SizedBox(height: 120),
          Center(child: Text('No media yet. Upload something to get started.')),
        ],
      );
    }
    // 4–6 columns so fewer, larger thumbnails load at once (faster on desktop,
    // where tiny tiles otherwise meant ~10 columns and dozens of concurrent loads).
    final cols = (MediaQuery.of(context).size.width / 240).floor().clamp(4, 6);
    return GridView.builder(
      controller: _scroll,
      padding: const EdgeInsets.all(6),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: cols,
        crossAxisSpacing: 6,
        mainAxisSpacing: 6,
      ),
      itemCount: _items.length + (_hasMore ? 1 : 0),
      itemBuilder: (context, i) {
        if (i >= _items.length) {
          return const Center(
            child: Padding(
              padding: EdgeInsets.all(16),
              child: SizedBox(
                  height: 22,
                  width: 22,
                  child: CircularProgressIndicator(strokeWidth: 2)),
            ),
          );
        }
        return _Tile(
          item: _items[i],
          client: _client,
          onTap: () => _openViewer(i),
        );
      },
    );
  }
}

class _Tile extends StatelessWidget {
  const _Tile({required this.item, required this.client, required this.onTap});
  final MediaItem item;
  final ApiClient client;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Stack(
        fit: StackFit.expand,
        children: [
          _Thumb(item: item, client: client),
          if (item.isVideo)
            const Positioned(
              right: 4,
              bottom: 4,
              child: Icon(Icons.play_circle_fill,
                  size: 22, color: Colors.white),
            ),
          if (item.isDoc)
            const Positioned(
              right: 4,
              bottom: 4,
              child: Icon(Icons.description, size: 20, color: Colors.white),
            ),
          if (item.encrypted)
            const Positioned(
              left: 4,
              top: 4,
              child: Icon(Icons.lock, size: 16, color: Colors.white70),
            ),
        ],
      ),
    );
  }
}

/// _Thumb loads a media thumbnail. For encrypted photos/documents the server
/// renders the real thumbnail in the background (Google only has the blue
/// placeholder), so this re-fetches a few times until it resolves. HEIC can't
/// be decoded server-side, so those aren't retried.
class _Thumb extends StatefulWidget {
  const _Thumb({required this.item, required this.client});
  final MediaItem item;
  final ApiClient client;

  @override
  State<_Thumb> createState() => _ThumbState();
}

class _ThumbState extends State<_Thumb> {
  int _bust = 0;
  final List<Timer> _timers = [];

  bool get _mayBePending {
    final it = widget.item;
    if (!it.encrypted) return false;
    // Encrypted photos/docs are rendered server-side (HEIC included now).
    return it.className == 'photo' || it.className == 'doc';
  }

  @override
  void initState() {
    super.initState();
    if (_mayBePending) {
      for (final d in const [
        Duration(seconds: 3),
        Duration(seconds: 8),
        Duration(seconds: 18),
        Duration(seconds: 40),
      ]) {
        _timers.add(Timer(d, () {
          if (mounted) setState(() => _bust++);
        }));
      }
    }
  }

  @override
  void dispose() {
    for (final t in _timers) {
      t.cancel();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final url = widget.client.thumbUrl(widget.item, size: 256) +
        (_bust > 0 ? '&_r=$_bust' : '');
    return CachedNetworkImage(
      imageUrl: url,
      httpHeaders: widget.client.authHeaders,
      fit: BoxFit.cover,
      fadeInDuration: const Duration(milliseconds: 120),
      memCacheWidth: 320, // decode small: tiles are ~140px, saves memory/CPU
      placeholder: (_, __) => Container(color: scheme.surfaceContainerHighest),
      errorWidget: (_, __, ___) => Container(
        color: scheme.surfaceContainerHighest,
        child: Icon(_fallbackIcon(widget.item), color: scheme.onSurfaceVariant),
      ),
    );
  }

  IconData _fallbackIcon(MediaItem it) {
    if (it.isVideo) return Icons.movie;
    if (it.isDoc) return Icons.description;
    if (it.isPhoto) return Icons.image;
    return Icons.insert_drive_file;
  }
}

class _ErrorState extends StatelessWidget {
  const _ErrorState({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.cloud_off, size: 42),
            const SizedBox(height: 12),
            Text(message, textAlign: TextAlign.center),
            const SizedBox(height: 16),
            FilledButton.tonal(onPressed: onRetry, child: const Text('Retry')),
          ],
        ),
      ),
    );
  }
}
