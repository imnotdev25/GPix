import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import '../util/format.dart';
import 'import_screen.dart';

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  PingInfo? _info;
  String? _error;
  bool _loading = true;

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
      final info = await _client.ping();
      setState(() => _info = info);
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final app = context.read<AppState>();
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: ListView(
        children: [
          ListTile(
            leading: const Icon(Icons.dns_outlined),
            title: const Text('Server'),
            subtitle: Text(app.baseUrl),
          ),
          if (_loading)
            const ListTile(
              leading: SizedBox(
                  height: 24,
                  width: 24,
                  child: CircularProgressIndicator(strokeWidth: 2)),
              title: Text('Checking connection…'),
            )
          else if (_error != null)
            ListTile(
              leading: Icon(Icons.error_outline,
                  color: Theme.of(context).colorScheme.error),
              title: const Text('Not connected'),
              subtitle: Text(_error!),
              trailing: TextButton(onPressed: _load, child: const Text('Retry')),
            )
          else if (_info != null) ...[
            ListTile(
              leading: const Icon(Icons.person_outline),
              title: const Text('Signed in as'),
              subtitle: Text(_info!.user.isEmpty ? '—' : _info!.user),
            ),
            ListTile(
              leading: Icon(
                  _info!.encryption ? Icons.lock : Icons.lock_open_outlined),
              title: const Text('Encryption'),
              subtitle:
                  Text(_info!.encryption ? 'On (items stored encrypted)' : 'Off'),
            ),
            ListTile(
              leading: const Icon(Icons.extension_outlined),
              title: const Text('Features'),
              subtitle: Text(_featuresLabel(_info!)),
            ),
          ],
          const Divider(),
          ListTile(
            leading: const Icon(Icons.palette_outlined),
            title: const Text('Appearance'),
            subtitle: Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Align(
                alignment: Alignment.centerLeft,
                child: SegmentedButton<ThemeMode>(
                  showSelectedIcon: false,
                  segments: const [
                    ButtonSegment(
                        value: ThemeMode.system,
                        icon: Icon(Icons.brightness_auto_outlined, size: 18),
                        label: Text('System')),
                    ButtonSegment(
                        value: ThemeMode.light,
                        icon: Icon(Icons.light_mode_outlined, size: 18),
                        label: Text('Light')),
                    ButtonSegment(
                        value: ThemeMode.dark,
                        icon: Icon(Icons.dark_mode_outlined, size: 18),
                        label: Text('Dark')),
                  ],
                  selected: {context.select<AppState, ThemeMode>((s) => s.themeMode)},
                  onSelectionChanged: (s) => app.setThemeMode(s.first),
                ),
              ),
            ),
          ),
          // Phones have no Import tab (the bar has room for six), so it's here.
          if (MediaQuery.sizeOf(context).width < 720)
            ListTile(
              leading: const Icon(Icons.download_outlined),
              title: const Text('Import'),
              subtitle: const Text(
                  'Copy a library in from another Google account or Immich'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(builder: (_) => const ImportScreen()),
              ),
            ),
          const Divider(),
          ListTile(
            leading: Icon(Icons.delete_sweep_outlined,
                color: Theme.of(context).colorScheme.error),
            title: const Text('Delete gpix uploads'),
            subtitle: const Text(
                'Move only what gpix uploaded to the Google Photos trash'),
            trailing: _purging
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : null,
            onTap: _purging ? null : _purgeGpixUploads,
          ),
          ListTile(
            leading: Icon(Icons.logout,
                color: Theme.of(context).colorScheme.error),
            title: const Text('Sign out'),
            onTap: () async {
              final ok = await showDialog<bool>(
                context: context,
                builder: (_) => AlertDialog(
                  title: const Text('Sign out?'),
                  content: const Text(
                      'You will need your server URL and token to sign back in.'),
                  actions: [
                    TextButton(
                        onPressed: () => Navigator.pop(context, false),
                        child: const Text('Cancel')),
                    FilledButton(
                        onPressed: () => Navigator.pop(context, true),
                        child: const Text('Sign out')),
                  ],
                ),
              );
              if (ok == true) app.signOut();
            },
          ),
          const SizedBox(height: 12),
          const Center(
            child: Padding(
              padding: EdgeInsets.all(12),
              child: Text('gpix app · 0.1.0',
                  style: TextStyle(color: Colors.grey)),
            ),
          ),
        ],
      ),
    );
  }

  bool _purging = false;

  void _snack(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  /// Review → typed confirmation → delete. On a 409 (library changed or the
  /// account switched) the user is shown the new numbers and asked again.
  Future<void> _purgeGpixUploads() async {
    setState(() => _purging = true);
    try {
      var summary = await _client.gpixUploadsSummary();
      while (true) {
        if (!mounted) return;
        if (summary.total == 0) {
          _snack('No gpix uploads found in ${summary.accountLabel}.');
          return;
        }
        final ok = await showDialog<bool>(
          context: context,
          builder: (_) => _PurgeConfirmDialog(summary: summary),
        );
        if (ok != true) return;
        try {
          final res = await _client.deleteGpixUploads(summary);
          _snack('Moved ${res.deleted} item(s) to the Google Photos trash'
              '${res.failed > 0 ? '; ${res.failed} failed' : ''}.');
          return;
        } on ApiException catch (e) {
          if (e.status != 409) rethrow;
          _snack(e.message);
          summary = await _client.gpixUploadsSummary();
        }
      }
    } catch (e) {
      _snack('Delete failed: $e');
    } finally {
      if (mounted) setState(() => _purging = false);
    }
  }

  String _featuresLabel(PingInfo info) {
    final on = <String>[];
    for (final k in ['upload', 'shares', 'import']) {
      if (info.feature(k)) on.add(k);
    }
    return on.isEmpty ? 'none' : on.join(', ');
  }
}


/// Shows what will be deleted and requires typing DELETE before enabling the
/// destructive button. Pops `true` to proceed.
class _PurgeConfirmDialog extends StatefulWidget {
  const _PurgeConfirmDialog({required this.summary});
  final GpixUploadSummary summary;

  @override
  State<_PurgeConfirmDialog> createState() => _PurgeConfirmDialogState();
}

class _PurgeConfirmDialogState extends State<_PurgeConfirmDialog> {
  final _ctrl = TextEditingController();

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.summary;
    final rows = <(String, int)>[
      ('Photos', s.photos),
      ('Videos', s.videos),
      ('Documents', s.docs),
      ('Other files', s.files),
    ].where((r) => r.$2 > 0);
    final since = s.trackingSince;
    final muted = TextStyle(color: Theme.of(context).hintColor, fontSize: 13);
    final ready = _ctrl.text == 'DELETE';

    return AlertDialog(
      title: const Text('Delete gpix uploads?'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('${s.total} item(s) (${formatBytes(s.bytes)}) in '
                '${s.accountLabel} will move to the Google Photos trash:'),
            const SizedBox(height: 8),
            for (final r in rows)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Row(children: [
                  Expanded(child: Text(r.$1)),
                  Text('${r.$2}'),
                ]),
              ),
            const SizedBox(height: 8),
            Text(
                '${s.tracked} from gpix\'s upload log, ${s.disguised} recognized '
                'as files gpix wrapped or encrypted.',
                style: muted),
            if (s.sample.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                  'Including: ${s.sample.join(', ')}'
                  '${s.total > s.sample.length ? ', …' : ''}',
                  style: muted),
            ],
            if (s.trackingEnabled && since != null) ...[
              const SizedBox(height: 4),
              Text(
                  'Plain photos and videos gpix uploaded before '
                  '${MaterialLocalizations.of(context).formatMediumDate(since.toLocal())} '
                  'weren\'t logged and won\'t be deleted.',
                  style: muted),
            ],
            const SizedBox(height: 4),
            Text('Recoverable from the trash for 60 days.', style: muted),
            const SizedBox(height: 12),
            TextField(
              controller: _ctrl,
              autocorrect: false,
              enableSuggestions: false,
              decoration: const InputDecoration(
                labelText: 'Type DELETE to confirm',
                border: OutlineInputBorder(),
              ),
              onChanged: (_) => setState(() {}),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel')),
        FilledButton(
          style: FilledButton.styleFrom(
              backgroundColor: Theme.of(context).colorScheme.error,
              foregroundColor: Theme.of(context).colorScheme.onError),
          onPressed: ready ? () => Navigator.pop(context, true) : null,
          child: Text('Move ${s.total} to trash'),
        ),
      ],
    );
  }
}
