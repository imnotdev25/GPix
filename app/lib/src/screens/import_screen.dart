import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';
import '../util/dialogs.dart';

class ImportScreen extends StatefulWidget {
  const ImportScreen({super.key});

  @override
  State<ImportScreen> createState() => _ImportScreenState();
}

class _ImportScreenState extends State<ImportScreen> {
  final _server = TextEditingController();
  final _apiKey = TextEditingController();
  final _authData = TextEditingController();
  double _workers = 3;
  Timer? _timer;
  ImportProgress _progress = ImportProgress.empty();
  List<LinkedAccount> _accounts = const [];
  bool _fromLinked = false; // Google source: a linked account, or pasted auth data
  int? _accountId;

  ApiClient get _client => context.read<AppState>().client!;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 2), (_) => _poll());
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _poll();
      _loadAccounts();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    _server.dispose();
    _apiKey.dispose();
    _authData.dispose();
    super.dispose();
  }

  Future<void> _poll() async {
    try {
      final p = await _client.importStatus();
      if (mounted) setState(() => _progress = p);
    } catch (_) {
      // ignore transient errors while polling
    }
  }

  Future<void> _loadAccounts() async {
    try {
      // The active account is the destination, so it can't be the source.
      final others = (await _client.listAccounts()).where((a) => !a.active).toList();
      if (!mounted) return;
      setState(() {
        _accounts = others;
        if (others.isNotEmpty) {
          _fromLinked = true;
          _accountId = others.first.id;
        }
      });
    } catch (_) {
      // pasting GP_AUTH_DATA still works
    }
  }

  Future<void> _startImmich() async {
    if (_server.text.trim().isEmpty || _apiKey.text.trim().isEmpty) {
      showSnack(context, 'Enter the Immich server URL and API key');
      return;
    }
    try {
      await _client.importStart(
        serverUrl: _server.text.trim(),
        apiKey: _apiKey.text.trim(),
        workers: _workers.round(),
      );
      _poll();
    } catch (e) {
      if (mounted) showSnack(context, 'Could not start: $e');
    }
  }

  Future<void> _startGoogle() async {
    if (_fromLinked ? _accountId == null : _authData.text.trim().isEmpty) {
      showSnack(context, 'Pick a linked account or paste its GP_AUTH_DATA');
      return;
    }
    try {
      await _client.importGoogleStart(
        accountId: _fromLinked ? _accountId : null,
        authData: _authData.text,
        workers: _workers.round(),
      );
      _poll();
    } catch (e) {
      if (mounted) showSnack(context, 'Could not start: $e');
    }
  }

  Future<void> _stop() async {
    try {
      await _client.importStop();
      _poll();
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    final p = _progress;
    final scheme = Theme.of(context).colorScheme;
    final title = Theme.of(context).textTheme.titleMedium;
    final muted = TextStyle(color: scheme.onSurfaceVariant);

    Widget section(List<Widget> children) => Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: children,
        ),
      ),
    );

    return Scaffold(
      appBar: AppBar(title: const Text('Import')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          const Text(
            'Copy a library into gpix from another Google account or from '
            'Immich. Encryption and disguising apply if enabled; '
            'already-imported items are skipped.',
          ),
          const SizedBox(height: 16),
          _ProgressCard(progress: p),
          if (p.running) ...[
            const SizedBox(height: 8),
            Align(
              alignment: Alignment.centerLeft,
              child: OutlinedButton.icon(
                onPressed: _stop,
                icon: const Icon(Icons.stop),
                label: const Text('Stop import'),
              ),
            ),
          ],
          const SizedBox(height: 16),
          Text('Parallel transfers: ${_workers.round()}'),
          Slider(
            value: _workers,
            min: 1,
            max: 10,
            divisions: 9,
            label: '${_workers.round()}',
            onChanged: p.running ? null : (v) => setState(() => _workers = v),
          ),
          const SizedBox(height: 8),
          section([
            Text('From another Google account', style: title),
            const SizedBox(height: 4),
            Text('Items are re-uploaded into the active account.', style: muted),
            const SizedBox(height: 12),
            SegmentedButton<bool>(
              segments: const [
                ButtonSegment(value: true, label: Text('Linked account')),
                ButtonSegment(value: false, label: Text('Paste auth data')),
              ],
              selected: {_fromLinked},
              onSelectionChanged: p.running
                  ? null
                  : (s) => setState(() => _fromLinked = s.first),
            ),
            const SizedBox(height: 8),
            if (_fromLinked) ...[
              if (_accounts.isEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Text(
                    'No other linked accounts. Link one in the web UI under '
                    'Accounts, or paste its GP_AUTH_DATA.',
                    style: muted,
                  ),
                )
              else
                for (final a in _accounts)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(
                      _accountId == a.id
                          ? Icons.radio_button_checked
                          : Icons.radio_button_unchecked,
                      color: _accountId == a.id ? scheme.primary : null,
                    ),
                    title: Text(a.label),
                    onTap: p.running ? null : () => setState(() => _accountId = a.id),
                  ),
            ] else
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: TextField(
                  controller: _authData,
                  obscureText: true,
                  enabled: !p.running,
                  decoration: const InputDecoration(
                    labelText: 'Source GP_AUTH_DATA',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            const SizedBox(height: 12),
            FilledButton.icon(
              onPressed: p.running ? null : _startGoogle,
              icon: const Icon(Icons.download),
              label: const Text('Start Google import'),
            ),
          ]),
          const SizedBox(height: 12),
          section([
            Text('From Immich', style: title),
            const SizedBox(height: 12),
            TextField(
              controller: _server,
              keyboardType: TextInputType.url,
              enabled: !p.running,
              decoration: const InputDecoration(
                labelText: 'Immich server URL',
                hintText: 'https://immich.example.com',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _apiKey,
              obscureText: true,
              enabled: !p.running,
              decoration: const InputDecoration(
                labelText: 'Immich API key',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 12),
            FilledButton.icon(
              onPressed: p.running ? null : _startImmich,
              icon: const Icon(Icons.download),
              label: const Text('Start Immich import'),
            ),
          ]),
        ],
      ),
    );
  }
}

class _ProgressCard extends StatelessWidget {
  const _ProgressCard({required this.progress});
  final ImportProgress progress;

  @override
  Widget build(BuildContext context) {
    final p = progress;
    final active = p.running || p.processed > 0 || p.error.isNotEmpty;
    if (!active) {
      return const Card(
        child: Padding(
          padding: EdgeInsets.all(16),
          child: Text('No import running.'),
        ),
      );
    }
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(p.running ? 'Running' : (p.error.isNotEmpty ? 'Error' : 'Finished'),
                    style: Theme.of(context).textTheme.titleMedium),
                if (p.workers > 0) Text('${p.workers} parallel'),
              ],
            ),
            const SizedBox(height: 10),
            LinearProgressIndicator(value: p.total > 0 ? p.fraction : null),
            const SizedBox(height: 10),
            Wrap(
              spacing: 16,
              runSpacing: 4,
              children: [
                Text('Done: ${p.done}'),
                Text('Skipped: ${p.skipped}'),
                Text('Failed: ${p.failed}'),
                Text('Total: ${p.total}'),
              ],
            ),
            if (p.running && p.current.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text('Copying: ${p.current}',
                  maxLines: 1, overflow: TextOverflow.ellipsis),
            ],
            if (p.error.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text('Last error: ${p.error}',
                  style: TextStyle(color: Theme.of(context).colorScheme.error)),
            ],
          ],
        ),
      ),
    );
  }
}
