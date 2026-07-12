import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';

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

  String _featuresLabel(PingInfo info) {
    final on = <String>[];
    for (final k in ['upload', 'shares', 'import']) {
      if (info.feature(k)) on.add(k);
    }
    return on.isEmpty ? 'none' : on.join(', ');
  }
}
