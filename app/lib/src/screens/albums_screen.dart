import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import '../theme.dart';
import '../util/collections.dart';
import '../util/dialogs.dart';
import '../widgets/thumb_image.dart';
import 'library_screen.dart';

String _plural(int n, String word) => n == 1 ? '1 $word' : '$n ${word}s';

/// Albums and tags made in gpix. They're stored by the gpix server and aren't
/// synced with albums in Google Photos.
class AlbumsScreen extends StatefulWidget {
  const AlbumsScreen({super.key});

  @override
  State<AlbumsScreen> createState() => _AlbumsScreenState();
}

class _AlbumsScreenState extends State<AlbumsScreen> {
  List<Album>? _albums;
  List<TagSummary> _tags = const [];
  String? _error;

  ApiClient get _client => context.read<AppState>().client!;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _load());
  }

  Future<void> _load() async {
    try {
      final albums = await _client.listAlbums();
      List<TagSummary> tags = const [];
      try {
        tags = await _client.listTags();
      } catch (_) {
        // the albums still show without tags
      }
      if (!mounted) return;
      setState(() {
        _albums = albums;
        _tags = tags;
        _error = null;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _albums ??= const [];
        _error = e.toString();
      });
    }
  }

  Future<void> _open({int? albumId, String? tag, required String title}) async {
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => LibraryScreen(albumId: albumId, tag: tag, title: title),
      ),
    );
    if (mounted) _load(); // counts and covers may have changed
  }

  Future<void> _create() async {
    final name = await promptName(
      context,
      title: 'New album',
      action: 'Create album',
    );
    if (name == null || !mounted) return;
    try {
      final a = await _client.createAlbum(name);
      if (!mounted) return;
      await _load();
      if (mounted) _open(albumId: a.id, title: a.name);
    } catch (e) {
      if (mounted) showSnack(context, "Couldn't create the album: $e");
    }
  }

  Future<void> _rename(Album a) async {
    final name = await promptName(
      context,
      title: 'Rename album',
      action: 'Rename',
      initial: a.name,
    );
    if (name == null || name == a.name || !mounted) return;
    try {
      await _client.renameAlbum(a.id, name);
      if (mounted) _load();
    } catch (e) {
      if (mounted) showSnack(context, 'Rename failed: $e');
    }
  }

  Future<void> _delete(Album a) async {
    final ok = await confirmDestructive(
      context,
      title: 'Delete "${a.name}"?',
      message: 'Only the album goes. The photos in it stay in your library.',
      action: 'Delete album',
    );
    if (!ok || !mounted) return;
    try {
      await _client.deleteAlbum(a.id);
      if (!mounted) return;
      showSnack(context, 'Album deleted');
      _load();
    } catch (e) {
      if (mounted) showSnack(context, 'Delete failed: $e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Albums'),
        actions: [
          TextButton.icon(
            onPressed: _create,
            icon: const Icon(Icons.add, size: 20),
            label: const Text('New album'),
          ),
          IconButton(
            tooltip: 'Refresh',
            icon: const Icon(Icons.refresh),
            onPressed: _load,
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: RefreshIndicator(onRefresh: _load, child: _body()),
    );
  }

  Widget _body() {
    final albums = _albums;
    if (albums == null) return const Center(child: CircularProgressIndicator());
    final muted = context.gpix.muted;
    return LayoutBuilder(
      builder: (context, c) {
        final cols = (c.maxWidth / 200).floor().clamp(2, 6);
        final itemW = (c.maxWidth - 32 - (cols - 1) * 12) / cols;
        return CustomScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          slivers: [
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(16, 4, 16, 12),
              sliver: SliverToBoxAdapter(
                child: Text(
                  "Albums and tags you make here are stored by gpix. They aren't "
                  'synced with albums in Google Photos.',
                  style: TextStyle(color: muted),
                ),
              ),
            ),
            if (_error != null)
              SliverPadding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                sliver: SliverToBoxAdapter(
                  child: Text(
                    _error!,
                    style: TextStyle(color: Theme.of(context).colorScheme.error),
                  ),
                ),
              ),
            if (albums.isEmpty)
              SliverToBoxAdapter(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(24, 48, 24, 24),
                  child: Column(
                    children: [
                      Icon(Icons.photo_album_outlined, size: 44, color: muted),
                      const SizedBox(height: 12),
                      Text(
                        'No albums yet.\nIn the Library, select photos and '
                        'choose "Add to album", or make an empty one here.',
                        textAlign: TextAlign.center,
                        style: TextStyle(color: muted),
                      ),
                      const SizedBox(height: 16),
                      FilledButton.tonalIcon(
                        onPressed: _create,
                        icon: const Icon(Icons.add),
                        label: const Text('New album'),
                      ),
                    ],
                  ),
                ),
              )
            else
              SliverPadding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                sliver: SliverGrid.builder(
                  gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                    crossAxisCount: cols,
                    crossAxisSpacing: 12,
                    mainAxisSpacing: 16,
                    mainAxisExtent: itemW + 56,
                  ),
                  itemCount: albums.length,
                  itemBuilder: (context, i) {
                    final a = albums[i];
                    return _AlbumCard(
                      album: a,
                      client: _client,
                      onOpen: () => _open(albumId: a.id, title: a.name),
                      onRename: () => _rename(a),
                      onDelete: () => _delete(a),
                    );
                  },
                ),
              ),
            if (_tags.isNotEmpty)
              SliverPadding(
                padding: const EdgeInsets.fromLTRB(16, 28, 16, 0),
                sliver: SliverToBoxAdapter(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('Tags', style: Theme.of(context).textTheme.titleMedium),
                      const SizedBox(height: 10),
                      Wrap(
                        spacing: 8,
                        runSpacing: 8,
                        children: [
                          for (final t in _tags)
                            ActionChip(
                              label: Text('#${t.tag} · ${t.count}'),
                              onPressed: () => _open(tag: t.tag, title: '#${t.tag}'),
                            ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
            const SliverToBoxAdapter(child: SizedBox(height: 32)),
          ],
        );
      },
    );
  }
}

class _AlbumCard extends StatelessWidget {
  const _AlbumCard({
    required this.album,
    required this.client,
    required this.onOpen,
    required this.onRename,
    required this.onDelete,
  });
  final Album album;
  final ApiClient client;
  final VoidCallback onOpen;
  final VoidCallback onRename;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final cover = album.cover;
    final g = context.gpix;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        AspectRatio(
          aspectRatio: 1,
          child: Material(
            color: g.surface2,
            borderRadius: BorderRadius.circular(12),
            clipBehavior: Clip.antiAlias,
            child: InkWell(
              onTap: onOpen,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  if (cover != null && cover.className != 'file')
                    ThumbImage(item: cover, client: client, size: 512)
                  else
                    Center(
                      child: Icon(Icons.photo_album_outlined, size: 36, color: g.muted),
                    ),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(height: 6),
        Row(
          children: [
            Expanded(
              child: InkWell(
                onTap: onOpen,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      album.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                    Text(
                      _plural(album.count, 'item'),
                      style: TextStyle(color: g.muted, fontSize: 12),
                    ),
                  ],
                ),
              ),
            ),
            PopupMenuButton<String>(
              tooltip: 'Album options',
              icon: const Icon(Icons.more_vert, size: 20),
              onSelected: (v) {
                if (v == 'rename') {
                  onRename();
                } else if (v == 'delete') {
                  onDelete();
                }
              },
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'rename', child: Text('Rename')),
                PopupMenuItem(value: 'delete', child: Text('Delete album')),
              ],
            ),
          ],
        ),
      ],
    );
  }
}
