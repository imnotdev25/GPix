import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../state/app_state.dart';

class ImportScreen extends StatefulWidget {
  const ImportScreen({super.key});

  @override
  State<ImportScreen> createState() => _ImportScreenState();
}

class _ImportScreenState extends State<ImportScreen> {
  final _server = TextEditingController();
  final _apiKey = TextEditingController();
  double _workers = 3;
  Timer? _timer;
  ImportProgress _progress = ImportProgress.empty();

  ApiClient get _client => context.read<AppState>().client!;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 2), (_) => _poll());
    WidgetsBinding.instance.addPostFrameCallback((_) => _poll());
  }

  @override
  void dispose() {
    _timer?.cancel();
    _server.dispose();
    _apiKey.dispose();
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

  Future<void> _start() async {
    if (_server.text.isEmpty || _apiKey.text.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
          content: Text('Enter the Immich server URL and API key')));
      return;
    }
    try {
      await _client.importStart(
        serverUrl: _server.text,
        apiKey: _apiKey.text,
        workers: _workers.round(),
      );
      _poll();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Could not start: $e')));
      }
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
    return Scaffold(
      appBar: AppBar(title: const Text('Import from Immich')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          const Text(
            'Copy your photos and videos from an existing Immich server into '
            'gpix. Encryption/disguise apply if enabled; already-imported items '
            'are skipped.',
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _server,
            keyboardType: TextInputType.url,
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
            decoration: const InputDecoration(
              labelText: 'Immich API key',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          Text('Parallel transfers: ${_workers.round()}'),
          Slider(
            value: _workers,
            min: 1,
            max: 10,
            divisions: 9,
            label: '${_workers.round()}',
            onChanged: p.running
                ? null
                : (v) => setState(() => _workers = v),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              FilledButton.icon(
                onPressed: p.running ? null : _start,
                icon: const Icon(Icons.download),
                label: const Text('Start import'),
              ),
              const SizedBox(width: 12),
              OutlinedButton(
                onPressed: p.running ? _stop : null,
                child: const Text('Stop'),
              ),
            ],
          ),
          const SizedBox(height: 24),
          _ProgressCard(progress: p),
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
