import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/backup_service.dart';

class BackupScreen extends StatelessWidget {
  const BackupScreen({super.key});

  bool get _mobile => !kIsWeb && (Platform.isAndroid || Platform.isIOS);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Backup')),
      body: _mobile ? const _BackupBody() : const _DesktopNotice(),
    );
  }
}

class _DesktopNotice extends StatelessWidget {
  const _DesktopNotice();
  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Padding(
        padding: EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.photo_library_outlined, size: 42),
            SizedBox(height: 12),
            Text(
              'Photo-library backup is available on Android and iOS.\n'
              'On desktop, use the Upload tab to send files.',
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}

class _BackupBody extends StatelessWidget {
  const _BackupBody();

  @override
  Widget build(BuildContext context) {
    final b = context.watch<BackupService>();
    final theme = Theme.of(context);
    final progressed = b.done + b.failed;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Card(
          child: Column(
            children: [
              SwitchListTile(
                title: const Text('Auto-backup'),
                subtitle: const Text(
                    'Back up new photos to gpix each time you open the app.'),
                value: b.enabled,
                onChanged: (v) => context.read<BackupService>().setEnabled(v),
              ),
              const Divider(height: 1),
              SwitchListTile(
                title: const Text('Include videos'),
                subtitle: const Text('Also back up videos (uses more data).'),
                value: b.includeVideos,
                onChanged: b.running
                    ? null
                    : (v) => context.read<BackupService>().setIncludeVideos(v),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text('Status', style: theme.textTheme.titleMedium),
                    Text(b.running ? 'Backing up…' : 'Idle',
                        style: theme.textTheme.bodyMedium),
                  ],
                ),
                const SizedBox(height: 10),
                if (b.running)
                  LinearProgressIndicator(
                    value: b.pending + progressed > 0
                        ? progressed / (b.pending + progressed)
                        : null,
                  ),
                const SizedBox(height: 10),
                Wrap(spacing: 16, runSpacing: 4, children: [
                  Text('Backed up: ${b.backedUp}'),
                  if (b.running || b.pending > 0) Text('Pending: ${b.pending}'),
                  if (b.done > 0) Text('This run: ${b.done}'),
                  if (b.failed > 0) Text('Failed: ${b.failed}'),
                ]),
                if (b.running && b.current.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text('Uploading: ${b.current}',
                      maxLines: 1, overflow: TextOverflow.ellipsis),
                ],
                if (b.error != null) ...[
                  const SizedBox(height: 8),
                  Text(b.error!,
                      style: TextStyle(color: theme.colorScheme.error)),
                ],
                const SizedBox(height: 14),
                Row(
                  children: [
                    if (!b.running)
                      FilledButton.icon(
                        onPressed: () =>
                            context.read<BackupService>().backupNow(),
                        icon: const Icon(Icons.backup_outlined),
                        label: const Text('Back up now'),
                      )
                    else
                      OutlinedButton.icon(
                        onPressed: () => context.read<BackupService>().stop(),
                        icon: const Icon(Icons.stop),
                        label: const Text('Stop'),
                      ),
                    const Spacer(),
                    TextButton(
                      onPressed: b.running
                          ? null
                          : () => _confirmReset(context),
                      child: const Text('Reset'),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 12),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 4),
          child: Text(
            'Backup runs while the app is open. Photos already uploaded are '
            'skipped. Encryption/disguising apply if enabled on your server.',
            style: TextStyle(fontSize: 12, color: Colors.grey),
          ),
        ),
      ],
    );
  }

  Future<void> _confirmReset(BuildContext context) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Reset backup state?'),
        content: const Text(
            'Forgets which photos were backed up (nothing is deleted). The next '
            'run re-checks every photo.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Reset')),
        ],
      ),
    );
    if (ok == true && context.mounted) {
      context.read<BackupService>().resetState();
    }
  }
}
