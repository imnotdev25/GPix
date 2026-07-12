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
  String _cursor = '0';
  bool _loading = false;
  bool _hasMore = true;
  int _total = 0;
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
      _cursor = '0';
      _hasMore = true;
      _error = null;
    });
    await _loadMore();
  }

  Future<void> _loadMore() async {
    if (_loading || !_hasMore) return;
    setState(() => _loading = true);
    try {
      final page =
          await _client.listMedia(cursor: int.tryParse(_cursor) ?? 0);
      setState(() {
        _items.addAll(page.items);
        _total = page.total;
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
        title: Text(_total > 0 ? 'Library · $_total' : 'Library'),
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
    return GridView.builder(
      controller: _scroll,
      padding: const EdgeInsets.all(4),
      gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
        maxCrossAxisExtent: 140,
        crossAxisSpacing: 4,
        mainAxisSpacing: 4,
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
    final scheme = Theme.of(context).colorScheme;
    return InkWell(
      onTap: onTap,
      child: Stack(
        fit: StackFit.expand,
        children: [
          CachedNetworkImage(
            imageUrl: client.thumbUrl(item, size: 256),
            httpHeaders: client.authHeaders,
            fit: BoxFit.cover,
            fadeInDuration: const Duration(milliseconds: 120),
            memCacheWidth: 320, // decode small: tiles are ~140px, saves memory/CPU
            placeholder: (_, __) => Container(color: scheme.surfaceContainerHighest),
            errorWidget: (_, __, ___) => Container(
              color: scheme.surfaceContainerHighest,
              child: Icon(_fallbackIcon(item), color: scheme.onSurfaceVariant),
            ),
          ),
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
