import 'dart:async';
import 'dart:math' as math;

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
import 'viewer_screen.dart';

/// Columns for a grid of the given width: ~200px tiles, 3–6 columns, so few
/// enough thumbnails load at once on big desktop windows.
int _columnsFor(double width) => (width / 200).floor().clamp(3, 6);

String _plural(int n, String word) => n == 1 ? '1 $word' : '$n ${word}s';

/// The photo grid. With [albumId] or [tag] it shows only that album or tag
/// (pushed from the Albums screen).
class LibraryScreen extends StatefulWidget {
  const LibraryScreen({super.key, this.albumId, this.tag, this.title});
  final int? albumId;
  final String? tag;
  final String? title;

  @override
  State<LibraryScreen> createState() => _LibraryScreenState();
}

class _LibraryScreenState extends State<LibraryScreen> {
  final _scroll = ScrollController();
  final List<MediaItem> _items = [];
  final Set<String> _seen = {};
  final Set<String> _selected = {};
  bool _selectMode = false;
  String _cursor = '';
  String _prevCursor = ''; // set after jumping to a month: the page above
  DateTime? _from; // the jump target, or null for "newest first"
  int _total = 0;
  List<TimelineMonth> _months = const [];
  int _gen = 0; // bumped on restart, so late responses are ignored
  bool _loading = false;
  bool _hasMore = true;
  bool _busy = false; // bulk delete/download running
  bool _warming = false; // server returned 503: first listing still running
  Timer? _retry;
  String? _error;

  ApiClient get _client => context.read<AppState>().client!;

  @override
  void initState() {
    super.initState();
    _scroll.addListener(_onScroll);
    WidgetsBinding.instance.addPostFrameCallback((_) => _loadMore());
  }

  @override
  void dispose() {
    _retry?.cancel();
    _scroll.dispose();
    super.dispose();
  }

  void _onScroll() => _fill();

  Future<void> _refresh() => _restart(null);

  /// Starts the listing over: from the newest item, or from [from] (a month
  /// picked in "Jump to").
  Future<void> _restart(DateTime? from) async {
    _retry?.cancel();
    _gen++;
    setState(() {
      _warming = false;
      _items.clear();
      _seen.clear();
      _cursor = '';
      _prevCursor = '';
      _from = from;
      _hasMore = true;
      _loading = false;
      _error = null;
    });
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scroll.hasClients) _scroll.jumpTo(0);
    });
    await _loadMore();
  }

  Future<void> _loadMore() async {
    if (_loading || !_hasMore) return;
    _retry?.cancel();
    final gen = _gen;
    final first = _cursor.isEmpty;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final page = await _client.listMedia(
        cursor: _cursor,
        from: _from,
        albumId: widget.albumId,
        tag: widget.tag,
      );
      if (!mounted || gen != _gen) return;
      final before = _items.length;
      setState(() {
        _warming = false;
        for (final it in page.items) {
          if (_seen.add(it.key)) _items.add(it);
        }
        _total = page.total;
        if (first) _prevCursor = page.offset > 0 ? page.prevCursor : '';
        // Guard against a server handing back the same cursor with nothing new.
        final stuck = _items.length == before && page.nextCursor == _cursor;
        _cursor = page.nextCursor;
        _hasMore = page.nextCursor.isNotEmpty && !stuck;
      });
      if (first && _from == null) _loadTimeline();
      // Pages are small (≤50), so keep loading until the grid fills the window.
      WidgetsBinding.instance.addPostFrameCallback((_) => _fill());
    } on ApiException catch (e) {
      if (!mounted || gen != _gen) return;
      if (e.status == 503) {
        // The server's first full listing hasn't finished yet: try again soon.
        setState(() => _warming = true);
        _retry = Timer(const Duration(seconds: 5), _loadMore);
      } else {
        setState(() => _error = e.message);
      }
    } catch (e) {
      if (mounted && gen == _gen) setState(() => _error = e.toString());
    } finally {
      if (mounted && gen == _gen) setState(() => _loading = false);
    }
  }

  /// After a jump: loads the page above the first item shown.
  Future<void> _loadNewer() async {
    if (_loading || _prevCursor.isEmpty) return;
    final gen = _gen;
    setState(() => _loading = true);
    try {
      final page = await _client.listMedia(
        before: _prevCursor,
        albumId: widget.albumId,
        tag: widget.tag,
      );
      if (!mounted || gen != _gen) return;
      setState(() {
        final fresh = [
          for (final it in page.items)
            if (_seen.add(it.key)) it,
        ];
        _items.insertAll(0, fresh);
        _prevCursor = page.offset > 0 ? page.prevCursor : '';
        _total = page.total;
      });
    } catch (e) {
      if (mounted && gen == _gen) showSnack(context, "Couldn't load newer photos: $e");
    } finally {
      if (mounted && gen == _gen) setState(() => _loading = false);
    }
  }

  /// Months and their counts, for "Jump to". Optional: on failure the button
  /// just stays hidden.
  Future<void> _loadTimeline() async {
    try {
      final months = await _client.timeline(albumId: widget.albumId, tag: widget.tag);
      if (mounted) setState(() => _months = months);
    } catch (_) {}
  }

  Future<void> _pickMonth() async {
    final m = await showModalBottomSheet<TimelineMonth>(
      context: context,
      isScrollControlled: true,
      builder: (_) => _JumpSheet(months: _months),
    );
    if (m == null || !mounted) return;
    final newest = _months.isNotEmpty ? _months.first : null;
    if (newest != null && newest.year == m.year && newest.month == m.month) {
      _restart(null); // the newest month: just start at the top
      return;
    }
    // The first moment after that month (DateTime rolls month 13 over).
    _restart(DateTime(m.year, m.month + 1));
  }

  /// Loads another page while less than ~800px of content remains below the
  /// viewport (first page, tall windows, or a window that just grew).
  void _fill() {
    if (!mounted || !_scroll.hasClients || _error != null) return;
    if (_scroll.position.extentAfter < 800) _loadMore();
  }

  List<_DayGroup> _groups() {
    final groups = <_DayGroup>[];
    for (final it in _items) {
      final d = dayOf(it.mtime);
      if (groups.isEmpty || groups.last.day != d) {
        groups.add(_DayGroup(d));
      }
      groups.last.items.add(it);
    }
    return groups;
  }

  // ---- selection ----------------------------------------------------------

  void _toggle(MediaItem it) => setState(() {
    _selectMode = true;
    if (!_selected.remove(it.key)) _selected.add(it.key);
  });

  void _toggleDay(_DayGroup g) => setState(() {
    _selectMode = true;
    final all = g.items.every((it) => _selected.contains(it.key));
    for (final it in g.items) {
      if (all) {
        _selected.remove(it.key);
      } else {
        _selected.add(it.key);
      }
    }
  });

  void _selectAll() => setState(() {
    _selectMode = true;
    _selected.addAll(_items.map((it) => it.key));
  });

  void _clearSelection() => setState(() {
    _selectMode = false;
    _selected.clear();
  });

  List<MediaItem> get _selectedItems => [
    for (final it in _items)
      if (_selected.contains(it.key)) it,
  ];

  Future<void> _shareSelected() async {
    if (await shareItems(context, _client, _selectedItems)) _clearSelection();
  }

  /// Drops items from the view (deleted, removed from this album, untagged).
  void _drop(Set<String> keys) {
    if (keys.isEmpty) return;
    setState(() {
      final before = _items.length;
      _items.removeWhere((it) => keys.contains(it.key));
      _seen.removeAll(keys);
      _selected.removeAll(keys);
      if (_selected.isEmpty) _selectMode = false;
      _total = math.max(0, _total - (before - _items.length));
    });
  }

  /// Applies tags returned by the server. On a tag's page, items that lost
  /// the tag leave the view.
  void _applyTags(Map<String, List<String>> tags) {
    final gone = <String>{};
    setState(() {
      for (var i = 0; i < _items.length; i++) {
        final t = tags[_items[i].key];
        if (t == null) continue;
        _items[i] = _items[i].copyWith(tags: t);
        if (widget.tag != null && !t.contains(widget.tag)) gone.add(_items[i].key);
      }
    });
    _drop(gone);
  }

  Future<void> _addSelectedToAlbum() async {
    final keys = [for (final it in _selectedItems) it.key];
    if (keys.isEmpty) return;
    final album = await addToAlbum(context, _client, keys);
    if (album == null || !mounted) return;
    _clearSelection();
    showSnack(context, 'Added ${_plural(keys.length, 'item')} to ${album.name}');
  }

  Future<void> _tagSelected() async {
    final items = _selectedItems;
    if (items.isEmpty) return;
    final tags = await editTags(
      context,
      _client,
      [for (final it in items) it.key],
      initial: items.length == 1 ? items.first.tags : null,
    );
    if (tags == null || !mounted) return;
    _applyTags(tags);
    if (items.length > 1) {
      _clearSelection();
      showSnack(context, 'Tagged ${_plural(items.length, 'item')}');
    }
  }

  Future<void> _removeSelectedFromAlbum() async {
    final id = widget.albumId;
    final keys = [for (final it in _selectedItems) it.key];
    if (id == null || keys.isEmpty) return;
    try {
      await _client.removeFromAlbum(id, keys);
      if (!mounted) return;
      _drop(keys.toSet());
      showSnack(context, 'Removed ${_plural(keys.length, 'item')} from the album');
    } catch (e) {
      if (mounted) showSnack(context, "Couldn't remove them: $e");
    }
  }

  Future<void> _deleteSelected() async {
    final items = _selectedItems;
    if (items.isEmpty) return;
    final ok = await confirmDestructive(
      context,
      title: 'Move ${_plural(items.length, 'item')} to trash?',
      message: 'They stay recoverable in the Google Photos trash for 60 days.',
      action: 'Move to trash',
    );
    if (!ok || !mounted) return;
    setState(() => _busy = true);
    final keys = [for (final it in items) it.key];
    final deleted = <String>{};
    var failed = 0;
    // One request per 1000 items (the server takes up to 5000).
    for (var i = 0; i < keys.length; i += 1000) {
      final chunk = keys.sublist(i, math.min(i + 1000, keys.length));
      try {
        final res = await _client.deleteMany(chunk);
        deleted.addAll(res.deleted);
        failed += res.failed;
      } catch (_) {
        failed += chunk.length;
      }
    }
    if (!mounted) return;
    setState(() => _busy = false);
    _drop(deleted);
    showSnack(
      context,
      'Moved ${_plural(deleted.length, 'item')} to trash'
      '${failed > 0 ? ' · $failed failed' : ''}',
    );
  }

  Future<void> _downloadSelected() async {
    final items = _selectedItems;
    if (items.isEmpty) return;
    if (isDesktopPlatform) {
      final dir = await pickFolder();
      if (dir == null || !mounted) return;
      setState(() => _busy = true);
      showSnack(context, 'Downloading ${items.length} item(s)…');
      var saved = 0, failed = 0;
      for (final it in items) {
        try {
          await writeUnique(dir, it.name, await _client.fetchOriginal(it));
          saved++;
        } catch (_) {
          failed++;
        }
      }
      if (!mounted) return;
      setState(() => _busy = false);
      showSnack(
        context,
        'Saved $saved item(s)${failed > 0 ? ' · $failed failed' : ''}',
      );
      return;
    }
    // Mobile: the system save sheet handles one file at a time.
    setState(() => _busy = true);
    var saved = 0;
    for (final it in items) {
      try {
        final path = await saveBytes(it.name, await _client.fetchOriginal(it));
        if (path == null) break; // cancelled: stop asking for the rest
        saved++;
      } catch (e) {
        if (mounted) showSnack(context, 'Download failed: $e');
      }
      if (!mounted) return;
    }
    if (!mounted) return;
    setState(() => _busy = false);
    showSnack(context, 'Saved $saved item(s)');
  }

  Future<void> _openViewer(MediaItem item) async {
    final index = _items.indexOf(item);
    if (index < 0) return;
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => ViewerScreen(
          items: List.of(_items),
          initialIndex: index,
          onDeleted: (it) {
            if (mounted) _drop({it.key});
          },
          onTagsChanged: (it, tags) {
            if (mounted) _applyTags({it.key: tags});
          },
        ),
      ),
    );
  }

  // ---- build --------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    return PopScope(
      canPop: !_selectMode,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) _clearSelection();
      },
      child: CallbackShortcuts(
        bindings: {
          const SingleActivator(LogicalKeyboardKey.escape): () {
            if (_selectMode) _clearSelection();
          },
        },
        child: Focus(
          autofocus: true,
          child: Scaffold(
            appBar: _selectMode ? _selectionBar() : _normalBar(),
            body: RefreshIndicator(onRefresh: _refresh, child: _buildBody()),
          ),
        ),
      ),
    );
  }

  PreferredSizeWidget _normalBar() => AppBar(
    title: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(widget.title ?? 'Library', overflow: TextOverflow.ellipsis),
        if (_total > 0)
          Text(
            _plural(_total, 'item'),
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w400,
              color: context.gpix.muted,
            ),
          ),
      ],
    ),
    actions: [
      if (_months.length > 1)
        IconButton(
          tooltip: 'Jump to a month',
          icon: const Icon(Icons.calendar_month_outlined),
          onPressed: _pickMonth,
        ),
      if (_items.isNotEmpty)
        TextButton.icon(
          onPressed: () => setState(() => _selectMode = true),
          icon: const Icon(Icons.check_circle_outline, size: 20),
          label: const Text('Select'),
        ),
      IconButton(
        onPressed: _loading ? null : _refresh,
        icon: const Icon(Icons.refresh),
        tooltip: 'Refresh',
      ),
      const SizedBox(width: 4),
    ],
  );

  PreferredSizeWidget _selectionBar() {
    final none = _selected.isEmpty || _busy;
    final actions = [
      _BarAction('Share', Icons.share_outlined, none ? null : _shareSelected, primary: true),
      _BarAction('Add to album', Icons.photo_album_outlined,
          none ? null : _addSelectedToAlbum, primary: true),
      _BarAction('Tag', Icons.sell_outlined, none ? null : _tagSelected),
      if (widget.albumId != null)
        _BarAction('Remove from album', Icons.playlist_remove,
            none ? null : _removeSelectedFromAlbum),
      _BarAction('Download', Icons.download_outlined, none ? null : _downloadSelected),
      _BarAction('Move to trash', Icons.delete_outline, none ? null : _deleteSelected,
          primary: true),
      _BarAction('Select all loaded', Icons.select_all, _busy ? null : _selectAll),
    ];
    // Phones show the main actions and put the rest in a menu.
    final narrow = MediaQuery.sizeOf(context).width < 600;
    final shown = narrow ? actions.where((a) => a.primary).toList() : actions;
    final more = narrow ? actions.where((a) => !a.primary).toList() : <_BarAction>[];
    return AppBar(
      leading: IconButton(
        tooltip: 'Clear selection (Esc)',
        icon: const Icon(Icons.close),
        onPressed: _clearSelection,
      ),
      title: Text('${_selected.length} selected'),
      bottom: _busy
          ? const PreferredSize(
              preferredSize: Size.fromHeight(2),
              child: LinearProgressIndicator(minHeight: 2),
            )
          : null,
      actions: [
        for (final a in shown)
          IconButton(tooltip: a.label, icon: Icon(a.icon), onPressed: a.onTap),
        if (more.isNotEmpty)
          PopupMenuButton<int>(
            tooltip: 'More',
            onSelected: (i) => more[i].onTap?.call(),
            itemBuilder: (_) => [
              for (var i = 0; i < more.length; i++)
                PopupMenuItem(
                  value: i,
                  enabled: more[i].onTap != null,
                  child: ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(more[i].icon),
                    title: Text(more[i].label),
                  ),
                ),
            ],
          ),
        const SizedBox(width: 4),
      ],
    );
  }

  Widget _buildBody() {
    if (_items.isEmpty && _warming) {
      return const _MessageState(
        icon: Icons.hourglass_top_rounded,
        message:
            'Loading your library…\n'
            'The server is reading it for the first time.',
        action: SizedBox(
          width: 22,
          height: 22,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
      );
    }
    if (_items.isEmpty && _loading) return const _SkeletonGrid();
    if (_items.isEmpty && _error != null) {
      return _MessageState(
        icon: Icons.cloud_off_outlined,
        message: _error!,
        action: FilledButton.tonal(
          onPressed: _refresh,
          child: const Text('Retry'),
        ),
      );
    }
    if (_items.isEmpty) {
      if (widget.albumId != null) {
        return const _MessageState(
          icon: Icons.photo_album_outlined,
          message: 'This album is empty.\n'
              'In the Library, select photos and choose "Add to album".',
        );
      }
      if (widget.tag != null) {
        return _MessageState(
          icon: Icons.sell_outlined,
          message: 'Nothing is tagged #${widget.tag} any more.',
        );
      }
      return const _MessageState(
        icon: Icons.photo_library_outlined,
        message: 'No media yet.\nUpload something to get started.',
      );
    }
    final groups = _groups();
    return LayoutBuilder(
      builder: (context, c) {
        final cols = _columnsFor(c.maxWidth);
        // Metrics notifications arrive after layout (e.g. a window resize),
        // so it's safe to load more from here.
        return NotificationListener<ScrollMetricsNotification>(
          onNotification: (_) {
            _fill();
            return false;
          },
          child: CustomScrollView(
            controller: _scroll,
            physics: const AlwaysScrollableScrollPhysics(),
            slivers: [
              if (_prevCursor.isNotEmpty)
                SliverToBoxAdapter(
                  child: Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Center(
                      child: OutlinedButton.icon(
                        onPressed: _loading ? null : _loadNewer,
                        icon: const Icon(Icons.expand_less),
                        label: const Text('Show newer photos'),
                      ),
                    ),
                  ),
                ),
              for (final g in groups) ...[
                SliverToBoxAdapter(
                  child: _DayHeader(
                    label: dayLabel(context, g.day),
                    count: g.items.length,
                    selecting: _selectMode,
                    allSelected: g.items.every(
                      (it) => _selected.contains(it.key),
                    ),
                    onToggle: () => _toggleDay(g),
                  ),
                ),
                SliverPadding(
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  sliver: SliverGrid.builder(
                    gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                      crossAxisCount: cols,
                      crossAxisSpacing: 4,
                      mainAxisSpacing: 4,
                    ),
                    itemCount: g.items.length,
                    itemBuilder: (context, i) {
                      final it = g.items[i];
                      return _Tile(
                        key: ValueKey(it.key),
                        item: it,
                        client: _client,
                        selecting: _selectMode,
                        selected: _selected.contains(it.key),
                        onOpen: () => _openViewer(it),
                        onToggle: () => _toggle(it),
                      );
                    },
                  ),
                ),
              ],
              SliverToBoxAdapter(child: _footer()),
            ],
          ),
        );
      },
    );
  }

  Widget _footer() {
    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          children: [
            Text(_error!, textAlign: TextAlign.center),
            const SizedBox(height: 8),
            FilledButton.tonal(
              onPressed: _loadMore,
              child: const Text('Retry'),
            ),
          ],
        ),
      );
    }
    if (_hasMore) {
      return const Padding(
        padding: EdgeInsets.all(24),
        child: Center(
          child: SizedBox(
            height: 22,
            width: 22,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        ),
      );
    }
    return const SizedBox(height: 24);
  }
}

class _DayGroup {
  _DayGroup(this.day);
  final DateTime? day;
  final List<MediaItem> items = [];
}

class _DayHeader extends StatefulWidget {
  const _DayHeader({
    required this.label,
    required this.count,
    required this.selecting,
    required this.allSelected,
    required this.onToggle,
  });
  final String label;
  final int count;
  final bool selecting;
  final bool allSelected;
  final VoidCallback onToggle;

  @override
  State<_DayHeader> createState() => _DayHeaderState();
}

class _DayHeaderState extends State<_DayHeader> {
  bool _hover = false;

  @override
  Widget build(BuildContext context) {
    final show = widget.selecting || _hover;
    final muted = context.gpix.muted;
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 18, 8, 6),
        child: Row(
          children: [
            Text(
              widget.label,
              style: Theme.of(
                context,
              ).textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
            ),
            const SizedBox(width: 8),
            Text(
              '${widget.count}',
              style: TextStyle(color: muted, fontSize: 12),
            ),
            const Spacer(),
            AnimatedOpacity(
              opacity: show ? 1 : 0,
              duration: const Duration(milliseconds: 120),
              child: IgnorePointer(
                ignoring: !show,
                child: IconButton(
                  visualDensity: VisualDensity.compact,
                  tooltip: widget.allSelected ? 'Deselect day' : 'Select day',
                  onPressed: widget.onToggle,
                  icon: Icon(
                    widget.allSelected
                        ? Icons.check_circle
                        : Icons.radio_button_unchecked,
                    color: widget.allSelected ? GpixPalette.accent : muted,
                    size: 20,
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Tile extends StatefulWidget {
  const _Tile({
    super.key,
    required this.item,
    required this.client,
    required this.selecting,
    required this.selected,
    required this.onOpen,
    required this.onToggle,
  });
  final MediaItem item;
  final ApiClient client;
  final bool selecting;
  final bool selected;
  final VoidCallback onOpen;
  final VoidCallback onToggle;

  @override
  State<_Tile> createState() => _TileState();
}

class _TileState extends State<_Tile> {
  bool _hover = false;

  @override
  Widget build(BuildContext context) {
    final it = widget.item;
    final showCheck = widget.selecting || _hover || widget.selected;
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      child: Stack(
        fit: StackFit.expand,
        children: [
          GestureDetector(
            onTap: widget.selecting ? widget.onToggle : widget.onOpen,
            onLongPress: widget.onToggle,
            child: ColoredBox(
              color: context.gpix.surface2,
              child: AnimatedPadding(
                duration: const Duration(milliseconds: 120),
                padding: EdgeInsets.all(widget.selected ? 10 : 0),
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(widget.selected ? 8 : 4),
                  child: Stack(
                    fit: StackFit.expand,
                    children: [
                      ThumbImage(
                        item: it,
                        client: widget.client,
                        size: 256,
                        memCacheWidth: 400, // tiles are up to ~200 logical px
                        error: Center(
                          child: Icon(
                            _fallbackIcon(it),
                            color: Theme.of(
                              context,
                            ).colorScheme.onSurfaceVariant,
                          ),
                        ),
                      ),
                      if (_hover && !widget.selecting)
                        const ColoredBox(color: Color(0x14000000)),
                      if (it.isVideo)
                        const _Badge(
                          alignment: Alignment.bottomRight,
                          icon: Icons.play_arrow_rounded,
                        ),
                      if (it.isDoc)
                        const _Badge(
                          alignment: Alignment.bottomRight,
                          icon: Icons.description_outlined,
                        ),
                      if (it.encrypted)
                        const _Badge(
                          alignment: Alignment.topRight,
                          icon: Icons.lock_outline,
                        ),
                    ],
                  ),
                ),
              ),
            ),
          ),
          if (showCheck)
            Positioned(
              left: 4,
              top: 4,
              child: GestureDetector(
                onTap: widget.onToggle,
                child: Container(
                  width: 24,
                  height: 24,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: widget.selected
                        ? GpixPalette.accent
                        : Colors.black38,
                    border: Border.all(color: Colors.white, width: 1.5),
                  ),
                  child: widget.selected
                      ? const Icon(Icons.check, size: 16, color: Colors.white)
                      : null,
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _Badge extends StatelessWidget {
  const _Badge({required this.alignment, required this.icon});
  final Alignment alignment;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: alignment,
      child: Container(
        margin: const EdgeInsets.all(5),
        padding: const EdgeInsets.all(3),
        decoration: BoxDecoration(
          color: Colors.black54,
          borderRadius: BorderRadius.circular(6),
        ),
        child: Icon(icon, size: 14, color: Colors.white),
      ),
    );
  }
}

IconData _fallbackIcon(MediaItem it) {
  if (it.isVideo) return Icons.movie_outlined;
  if (it.isDoc) return Icons.description_outlined;
  if (it.isPhoto) return Icons.image_outlined;
  return Icons.insert_drive_file_outlined;
}

/// Pulsing placeholder tiles shown while the first page loads.
class _SkeletonGrid extends StatefulWidget {
  const _SkeletonGrid();

  @override
  State<_SkeletonGrid> createState() => _SkeletonGridState();
}

class _SkeletonGridState extends State<_SkeletonGrid>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 900),
  )..repeat(reverse: true);
  late final Animation<double> _fade = Tween<double>(
    begin: 0.45,
    end: 1,
  ).animate(_ctrl);

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final color = context.gpix.surface2;
    return LayoutBuilder(
      builder: (context, c) {
        final cols = _columnsFor(c.maxWidth);
        return FadeTransition(
          opacity: _fade,
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(12, 18, 12, 12),
            children: [
              Container(
                width: 120,
                height: 14,
                margin: const EdgeInsets.only(left: 4, bottom: 14, right: 200),
                decoration: BoxDecoration(
                  color: color,
                  borderRadius: BorderRadius.circular(4),
                ),
              ),
              GridView.builder(
                shrinkWrap: true,
                physics: const NeverScrollableScrollPhysics(),
                gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: cols,
                  crossAxisSpacing: 4,
                  mainAxisSpacing: 4,
                ),
                itemCount: cols * 4,
                itemBuilder: (_, __) => DecoratedBox(
                  decoration: BoxDecoration(
                    color: color,
                    borderRadius: BorderRadius.circular(4),
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _BarAction {
  const _BarAction(this.label, this.icon, this.onTap, {this.primary = false});
  final String label;
  final IconData icon;
  final VoidCallback? onTap;
  final bool primary; // stays in the bar on phones
}

const _monthNames = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
];

/// "Jump to": every month with photos, grouped by year, newest first.
class _JumpSheet extends StatelessWidget {
  const _JumpSheet({required this.months});
  final List<TimelineMonth> months;

  @override
  Widget build(BuildContext context) {
    final byYear = <int, List<TimelineMonth>>{};
    for (final m in months) {
      byYear.putIfAbsent(m.year, () => []).add(m);
    }
    final muted = context.gpix.muted;
    return SafeArea(
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(context).height * 0.75,
        ),
        child: ListView(
          shrinkWrap: true,
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
          children: [
            Text('Jump to', style: Theme.of(context).textTheme.titleMedium),
            for (final e in byYear.entries) ...[
              const SizedBox(height: 16),
              Text(
                '${e.key}',
                style: TextStyle(color: muted, fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final m in e.value)
                    ActionChip(
                      label: Text(
                        '${_monthNames[(m.month - 1).clamp(0, 11)]} · ${m.count}',
                      ),
                      onPressed: () => Navigator.pop(context, m),
                    ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _MessageState extends StatelessWidget {
  const _MessageState({required this.icon, required this.message, this.action});
  final IconData icon;
  final String message;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final muted = context.gpix.muted;
    return LayoutBuilder(
      builder: (context, c) => SingleChildScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        child: ConstrainedBox(
          constraints: BoxConstraints(minHeight: c.maxHeight),
          child: Center(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(icon, size: 44, color: muted),
                  const SizedBox(height: 12),
                  Text(
                    message,
                    textAlign: TextAlign.center,
                    style: TextStyle(color: muted),
                  ),
                  if (action != null) ...[const SizedBox(height: 16), action!],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
