import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import '../util/download.dart';

class ViewerScreen extends StatefulWidget {
  const ViewerScreen({super.key, required this.items, required this.initialIndex});
  final List<MediaItem> items;
  final int initialIndex;

  @override
  State<ViewerScreen> createState() => _ViewerScreenState();
}

class _ViewerScreenState extends State<ViewerScreen> {
  late final PageController _controller =
      PageController(initialPage: widget.initialIndex);
  late int _index = widget.initialIndex;

  ApiClient get _client => context.read<AppState>().client!;
  MediaItem get _current => widget.items[_index];

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _snack(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _download(MediaItem item) async {
    _snack('Downloading ${item.name}…');
    try {
      final bytes = await _client.fetchOriginal(item);
      final saved = await saveBytes(item.name, bytes);
      _snack(saved == null ? 'Cancelled' : 'Saved ${item.name}');
    } catch (e) {
      _snack('Download failed: $e');
    }
  }

  Future<void> _createLink(MediaItem item) async {
    final opts = await showDialog<_ShareOpts>(
      context: context,
      builder: (_) => const _ShareOptsDialog(),
    );
    if (opts == null) return;
    try {
      final share = await _client.createShare(
        keys: [item.key],
        title: item.name,
        password: opts.password,
        expiresHours: opts.expiresHours,
        allowOriginal: true,
      );
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (_) => _ShareLinkDialog(url: share.url),
      );
    } catch (e) {
      _snack('Could not create link: $e');
    }
  }

  Future<void> _delete(MediaItem item) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Move to trash?'),
        content: Text(item.name),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Delete')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _client.deleteMedia(item.key);
      if (!mounted) return;
      Navigator.pop(context, true); // back to library, which refreshes
    } catch (e) {
      _snack('Delete failed: $e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(_current.name, overflow: TextOverflow.ellipsis),
        actions: [
          IconButton(
            tooltip: 'Download',
            icon: const Icon(Icons.download),
            onPressed: () => _download(_current),
          ),
          PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'link') _createLink(_current);
              if (v == 'delete') _delete(_current);
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'link', child: Text('Create share link')),
              PopupMenuItem(value: 'delete', child: Text('Delete')),
            ],
          ),
        ],
      ),
      body: PageView.builder(
        controller: _controller,
        itemCount: widget.items.length,
        onPageChanged: (i) => setState(() => _index = i),
        itemBuilder: (context, i) => _ViewerPage(
          item: widget.items[i],
          client: _client,
          onDownload: () => _download(widget.items[i]),
        ),
      ),
    );
  }
}

class _ViewerPage extends StatelessWidget {
  const _ViewerPage(
      {required this.item, required this.client, required this.onDownload});
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
          child: CachedNetworkImage(
            imageUrl: client.thumbUrl(item, size: 2048),
            httpHeaders: client.authHeaders,
            fit: BoxFit.contain,
            placeholder: (_, __) =>
                const Center(child: CircularProgressIndicator()),
            errorWidget: (_, __, ___) => const Center(
                child: Icon(Icons.broken_image, size: 48)),
          ),
        ),
      );
    }
    // Video / other files: no in-app playback yet — offer download.
    final scheme = Theme.of(context).colorScheme;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(item.isVideo ? Icons.movie : Icons.insert_drive_file,
                size: 72, color: scheme.primary),
            const SizedBox(height: 16),
            Text(item.name, textAlign: TextAlign.center),
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

class _ShareOpts {
  const _ShareOpts(this.password, this.expiresHours);
  final String password;
  final int expiresHours;
}

class _ShareOptsDialog extends StatefulWidget {
  const _ShareOptsDialog();
  @override
  State<_ShareOptsDialog> createState() => _ShareOptsDialogState();
}

class _ShareOptsDialogState extends State<_ShareOptsDialog> {
  final _password = TextEditingController();
  final _hours = TextEditingController();

  @override
  void dispose() {
    _password.dispose();
    _hours.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Create share link'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          TextField(
            controller: _password,
            decoration: const InputDecoration(
              labelText: 'Password (optional)',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _hours,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              labelText: 'Expires in hours (0 = never)',
              border: OutlineInputBorder(),
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel')),
        FilledButton(
          onPressed: () => Navigator.pop(
            context,
            _ShareOpts(_password.text, int.tryParse(_hours.text) ?? 0),
          ),
          child: const Text('Create'),
        ),
      ],
    );
  }
}

class _ShareLinkDialog extends StatelessWidget {
  const _ShareLinkDialog({required this.url});
  final String url;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Share link'),
      content: SelectableText(url),
      actions: [
        TextButton(
          onPressed: () {
            Clipboard.setData(ClipboardData(text: url));
            Navigator.pop(context);
          },
          child: const Text('Copy'),
        ),
        FilledButton(
          onPressed: () {
            launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
            Navigator.pop(context);
          },
          child: const Text('Open'),
        ),
      ],
    );
  }
}
