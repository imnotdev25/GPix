import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import '../api/models.dart';

/// Shows a short message, replacing any message already on screen.
void showSnack(BuildContext context, String msg) {
  final m = ScaffoldMessenger.maybeOf(context);
  if (m == null) return;
  m
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(msg)));
}

/// Asks before a destructive action. Returns true to proceed.
Future<bool> confirmDestructive(
  BuildContext context, {
  required String title,
  required String message,
  required String action,
}) async {
  final ok = await showDialog<bool>(
    context: context,
    builder: (ctx) {
      final scheme = Theme.of(ctx).colorScheme;
      return AlertDialog(
        title: Text(title),
        content: Text(message),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          FilledButton(
            style: FilledButton.styleFrom(
                backgroundColor: scheme.error,
                foregroundColor: scheme.onError),
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(action),
          ),
        ],
      );
    },
  );
  return ok == true;
}

/// Asks for share options, creates one link for [items], and shows it.
/// Returns true when a link was created.
Future<bool> shareItems(
    BuildContext context, ApiClient client, List<MediaItem> items) async {
  if (items.isEmpty) return false;
  final opts = await showDialog<_ShareOptions>(
    context: context,
    builder: (_) => _ShareOptionsDialog(count: items.length),
  );
  if (opts == null || !context.mounted) return false;
  try {
    final share = await client.createShare(
      keys: [for (final it in items) it.key],
      title: items.length == 1 ? items.first.name : '${items.length} items',
      password: opts.password,
      expiresHours: opts.expiresHours,
      allowOriginal: opts.allowOriginal,
    );
    if (!context.mounted) return true;
    await showDialog<void>(
      context: context,
      builder: (_) => _ShareLinkDialog(
        url: share.url,
        directUrl: share.directUrl,
        count: items.length,
      ),
    );
    return true;
  } catch (e) {
    if (context.mounted) showSnack(context, 'Could not create link: $e');
    return false;
  }
}

class _ShareOptions {
  const _ShareOptions(this.password, this.expiresHours, this.allowOriginal);
  final String password;
  final int expiresHours;
  final bool allowOriginal;
}

const _expiryChoices = <(String, int)>[
  ('Never', 0),
  ('1 hour', 1),
  ('1 day', 24),
  ('7 days', 168),
  ('30 days', 720),
];

class _ShareOptionsDialog extends StatefulWidget {
  const _ShareOptionsDialog({required this.count});
  final int count;

  @override
  State<_ShareOptionsDialog> createState() => _ShareOptionsDialogState();
}

class _ShareOptionsDialogState extends State<_ShareOptionsDialog> {
  final _password = TextEditingController();
  int _hours = 0;
  bool _allowOriginal = true;

  @override
  void dispose() {
    _password.dispose();
    super.dispose();
  }

  void _submit() => Navigator.pop(
      context, _ShareOptions(_password.text, _hours, _allowOriginal));

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.count == 1
          ? 'Share link'
          : 'Share link for ${widget.count} items'),
      content: SizedBox(
        width: 380,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextField(
              controller: _password,
              decoration: const InputDecoration(
                labelText: 'Password (optional)',
                prefixIcon: Icon(Icons.lock_outline, size: 20),
              ),
              onSubmitted: (_) => _submit(),
            ),
            const SizedBox(height: 16),
            Text('Expires', style: Theme.of(context).textTheme.labelLarge),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final (label, hours) in _expiryChoices)
                  ChoiceChip(
                    label: Text(label),
                    selected: _hours == hours,
                    onSelected: (_) => setState(() => _hours = hours),
                  ),
              ],
            ),
            const SizedBox(height: 8),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Allow downloading originals'),
              value: _allowOriginal,
              onChanged: (v) => setState(() => _allowOriginal = v),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel')),
        FilledButton(onPressed: _submit, child: const Text('Create link')),
      ],
    );
  }
}

class _ShareLinkDialog extends StatelessWidget {
  const _ShareLinkDialog({
    required this.url,
    this.directUrl = '',
    this.count = 1,
  });
  final String url;
  final String directUrl;
  final int count;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    Widget box(String text) => Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: scheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(9),
      ),
      child: SelectableText(text),
    );
    return AlertDialog(
      title: const Text('Link ready'),
      content: SizedBox(
        width: 380,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            box(url),
            if (directUrl.isNotEmpty) ...[
              const SizedBox(height: 14),
              Text(
                count > 1
                    ? 'Direct download (a .zip, no page)'
                    : 'Direct download (no page)',
                style: TextStyle(color: scheme.onSurfaceVariant, fontSize: 13),
              ),
              const SizedBox(height: 6),
              Row(
                children: [
                  Expanded(child: box(directUrl)),
                  IconButton(
                    tooltip: 'Copy direct link',
                    icon: const Icon(Icons.copy, size: 18),
                    onPressed: () {
                      Clipboard.setData(ClipboardData(text: directUrl));
                      showSnack(context, 'Direct link copied');
                    },
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () {
            launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
          },
          child: const Text('Open'),
        ),
        FilledButton.icon(
          icon: const Icon(Icons.copy, size: 18),
          label: const Text('Copy link'),
          onPressed: () {
            final messenger = ScaffoldMessenger.maybeOf(context);
            Clipboard.setData(ClipboardData(text: url));
            Navigator.pop(context);
            messenger
              ?..hideCurrentSnackBar()
              ..showSnackBar(const SnackBar(content: Text('Link copied')));
          },
        ),
      ],
    );
  }
}
