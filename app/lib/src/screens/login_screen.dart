import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/app_state.dart';
import '../theme.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _url = TextEditingController();
  final _token = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _url.dispose();
    _token.dispose();
    super.dispose();
  }

  Future<void> _connect() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await context.read<AppState>().signIn(_url.text, _token.text);
      // On success the app root swaps to HomeScreen automatically.
    } catch (e) {
      setState(() => _error = _friendly(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  String _friendly(Object e) {
    final s = e.toString();
    if (s.contains('invalid token') ||
        s.contains('missing bearer') ||
        s.contains('401')) {
      return 'That token was rejected. Re-copy it from the web UI → App tab.';
    }
    if (s.contains('Invalid HTTP header')) {
      return 'The token has invalid characters — re-copy it (no spaces or newlines).';
    }
    if (s.contains('HandshakeException') ||
        s.toLowerCase().contains('certificate')) {
      return 'TLS problem reaching the server. If it runs over plain HTTP, '
          'type the full http:// URL (not just the host).';
    }
    if (s.contains('Connection refused') || s.contains('Connection failed')) {
      return 'Connection refused — is gpix running and reachable at that URL?';
    }
    if (s.contains('SocketException') || s.contains('Failed host lookup')) {
      return 'Could not reach the server. Check the URL and your network.';
    }
    return s;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Center(child: BrandMark(size: 56)),
                const SizedBox(height: 12),
                Text('gpix',
                    textAlign: TextAlign.center,
                    style: Theme.of(context).textTheme.headlineMedium),
                const SizedBox(height: 4),
                Text('Connect to your server',
                    textAlign: TextAlign.center,
                    style: Theme.of(context).textTheme.bodyMedium),
                const SizedBox(height: 28),
                TextField(
                  controller: _url,
                  keyboardType: TextInputType.url,
                  autocorrect: false,
                  decoration: const InputDecoration(
                    labelText: 'Server URL',
                    hintText: 'https://gpix.example.com',
                    border: OutlineInputBorder(),
                  ),
                ),
                const SizedBox(height: 14),
                TextField(
                  controller: _token,
                  obscureText: true,
                  autocorrect: false,
                  enableSuggestions: false,
                  decoration: const InputDecoration(
                    labelText: 'API token',
                    hintText: 'gpix_…  (App tab in the web UI)',
                    border: OutlineInputBorder(),
                  ),
                ),
                if (_error != null) ...[
                  const SizedBox(height: 14),
                  Text(_error!,
                      style: TextStyle(
                          color: Theme.of(context).colorScheme.error)),
                ],
                const SizedBox(height: 22),
                FilledButton(
                  onPressed: _busy ? null : _connect,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 6),
                    child: _busy
                        ? const SizedBox(
                            height: 20,
                            width: 20,
                            child: CircularProgressIndicator(strokeWidth: 2))
                        : const Text('Connect'),
                  ),
                ),
                const SizedBox(height: 12),
                Text(
                  'Generate a token in the gpix web UI under the “App” tab.',
                  textAlign: TextAlign.center,
                  style: Theme.of(context)
                      .textTheme
                      .bodySmall
                      ?.copyWith(color: Theme.of(context).hintColor),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
