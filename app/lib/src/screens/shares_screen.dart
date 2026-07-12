import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';

class SharesScreen extends StatefulWidget {
  const SharesScreen({super.key});

  @override
  State<SharesScreen> createState() => _SharesScreenState();
}

class _SharesScreenState extends State<SharesScreen> {
  List<ShareInfo> _shares = [];
  bool _loading = true;
  String? _error;

  ApiClient get _client => context.read<AppState>().client!;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _load());
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final shares = await _client.listShares();
      setState(() => _shares = shares);
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _revoke(ShareInfo s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Revoke link?'),
        content: Text(s.title.isEmpty ? s.token : s.title),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Revoke')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _client.revokeShare(s.token);
      _load();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Revoke failed: $e')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Shares'),
        actions: [
          IconButton(onPressed: _load, icon: const Icon(Icons.refresh)),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: _buildBody(),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading && _shares.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null && _shares.isEmpty) {
      return ListView(children: [
        const SizedBox(height: 100),
        Center(child: Text(_error!)),
      ]);
    }
    if (_shares.isEmpty) {
      return ListView(children: const [
        SizedBox(height: 100),
        Center(
            child: Text('No share links yet.\nCreate one from a photo.',
                textAlign: TextAlign.center)),
      ]);
    }
    return ListView.separated(
      itemCount: _shares.length,
      separatorBuilder: (_, __) => const Divider(height: 1),
      itemBuilder: (context, i) {
        final s = _shares[i];
        return ListTile(
          leading: Icon(s.hasPassword ? Icons.lock : Icons.link),
          title: Text(s.title.isEmpty ? '(${s.count} item(s))' : s.title,
              overflow: TextOverflow.ellipsis),
          subtitle: Text(
            '${s.count} item(s) · ${s.downloads}'
            '${s.maxDownloads > 0 ? '/${s.maxDownloads}' : ''} downloads',
          ),
          trailing: PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'open') {
                launchUrl(Uri.parse(s.url),
                    mode: LaunchMode.externalApplication);
              } else if (v == 'copy') {
                Clipboard.setData(ClipboardData(text: s.url));
                ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Link copied')));
              } else if (v == 'revoke') {
                _revoke(s);
              }
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'open', child: Text('Open')),
              PopupMenuItem(value: 'copy', child: Text('Copy link')),
              PopupMenuItem(value: 'revoke', child: Text('Revoke')),
            ],
          ),
        );
      },
    );
  }
}
