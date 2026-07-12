import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/api_client.dart';

/// AppState holds the current session (server URL + token) and persists it.
class AppState extends ChangeNotifier {
  static const _kBaseUrl = 'baseUrl';
  static const _kToken = 'token';

  ApiClient? _client;
  bool loading = true;

  ApiClient? get client => _client;
  bool get isSignedIn => _client != null;
  String get baseUrl => _client?.baseUrl ?? '';

  Future<void> load() async {
    final prefs = await SharedPreferences.getInstance();
    final url = prefs.getString(_kBaseUrl);
    final token = prefs.getString(_kToken);
    if (url != null && url.isNotEmpty && token != null && token.isNotEmpty) {
      _client = ApiClient(baseUrl: url, token: token);
    }
    loading = false;
    notifyListeners();
  }

  /// Validates the given credentials against the server, then persists them.
  Future<void> signIn(String url, String token) async {
    final probe = ApiClient(baseUrl: url, token: token);
    await probe.ping(); // throws on failure
    final prefs = await SharedPreferences.getInstance();
    // Persist the normalized/trimmed values the client actually uses.
    await prefs.setString(_kBaseUrl, probe.baseUrl);
    await prefs.setString(_kToken, probe.token);
    _client = probe;
    notifyListeners();
  }

  Future<void> signOut() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_kToken);
    _client = null;
    notifyListeners();
  }
}
