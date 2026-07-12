import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'models.dart';

/// ApiClient talks to a gpix server's /api/v1 JSON API using a Bearer token.
class ApiClient {
  ApiClient({required String baseUrl, required this.token})
      : baseUrl = normalizeBaseUrl(baseUrl);

  final String baseUrl;
  final String token;

  static String normalizeBaseUrl(String u) {
    u = u.trim();
    while (u.endsWith('/')) {
      u = u.substring(0, u.length - 1);
    }
    if (!u.startsWith('http://') && !u.startsWith('https://')) {
      u = 'https://$u';
    }
    return u;
  }

  Map<String, String> get authHeaders => {'Authorization': 'Bearer $token'};

  Uri _uri(String path, [Map<String, String>? q]) {
    final u = Uri.parse('$baseUrl$path');
    return q == null ? u : u.replace(queryParameters: q);
  }

  /// Absolute thumbnail URL for use with an image widget (send authHeaders).
  String thumbUrl(MediaItem it, {int size = 256}) =>
      '$baseUrl${it.thumb}?size=$size';

  /// Absolute original-bytes URL (send authHeaders).
  String originalUrl(MediaItem it, {bool download = false}) =>
      '$baseUrl${it.original}${download ? '?download=1' : ''}';

  Future<PingInfo> ping() async {
    final r = await http.get(_uri('/api/v1/ping'), headers: authHeaders);
    _check(r.statusCode, r.body);
    return PingInfo.fromJson(jsonDecode(r.body) as Map<String, dynamic>);
  }

  Future<MediaPage> listMedia({int cursor = 0, int limit = 120}) async {
    final r = await http.get(
      _uri('/api/v1/media', {'cursor': '$cursor', 'limit': '$limit'}),
      headers: authHeaders,
    );
    _check(r.statusCode, r.body);
    return MediaPage.fromJson(jsonDecode(r.body) as Map<String, dynamic>);
  }

  Future<Uint8List> fetchOriginal(MediaItem it) async {
    final r = await http.get(_uri('${it.original}', {'download': '1'}),
        headers: authHeaders);
    _check(r.statusCode, r.body);
    return r.bodyBytes;
  }

  Future<void> deleteMedia(String key) async {
    final r = await http.delete(_uri('/api/v1/media/$key'), headers: authHeaders);
    _check(r.statusCode, r.body);
  }

  /// Uploads local files. Returns the per-file result maps ({name,status,key}).
  Future<List<Map<String, dynamic>>> upload(
    List<UploadFile> files, {
    String quality = 'original',
  }) async {
    final req = http.MultipartRequest('POST', _uri('/api/v1/upload'));
    req.headers.addAll(authHeaders);
    req.fields['quality'] = quality;
    for (final f in files) {
      req.files.add(await http.MultipartFile.fromPath('files', f.path,
          filename: f.name));
    }
    final streamed = await req.send();
    final body = await streamed.stream.bytesToString();
    _check(streamed.statusCode, body);
    final decoded = jsonDecode(body) as Map<String, dynamic>;
    return ((decoded['results'] as List?) ?? const [])
        .cast<Map<String, dynamic>>();
  }

  Future<List<ShareInfo>> listShares() async {
    final r = await http.get(_uri('/api/v1/shares'), headers: authHeaders);
    _check(r.statusCode, r.body);
    final list = (jsonDecode(r.body)['shares'] as List?) ?? const [];
    return list
        .map((e) => ShareInfo.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<ShareInfo> createShare({
    required List<String> keys,
    String title = '',
    String password = '',
    int expiresHours = 0,
    int maxDownloads = 0,
    bool allowOriginal = true,
  }) async {
    final r = await http.post(
      _uri('/api/v1/shares'),
      headers: {...authHeaders, 'Content-Type': 'application/json'},
      body: jsonEncode({
        'keys': keys,
        'title': title,
        'password': password,
        'expiresHours': expiresHours,
        'maxDownloads': maxDownloads,
        'allowOriginal': allowOriginal,
      }),
    );
    _check(r.statusCode, r.body);
    return ShareInfo.fromJson(jsonDecode(r.body) as Map<String, dynamic>);
  }

  Future<void> revokeShare(String token) async {
    final r =
        await http.delete(_uri('/api/v1/shares/$token'), headers: authHeaders);
    _check(r.statusCode, r.body);
  }

  Future<ImportProgress> importStatus() async {
    final r =
        await http.get(_uri('/api/v1/import/status'), headers: authHeaders);
    _check(r.statusCode, r.body);
    return ImportProgress.fromJson(jsonDecode(r.body) as Map<String, dynamic>);
  }

  Future<void> importStart({
    required String serverUrl,
    required String apiKey,
    int workers = 3,
  }) async {
    final r = await http.post(
      _uri('/api/v1/import/start'),
      headers: {...authHeaders, 'Content-Type': 'application/json'},
      body: jsonEncode(
          {'serverUrl': serverUrl, 'apiKey': apiKey, 'workers': workers}),
    );
    _check(r.statusCode, r.body);
  }

  Future<void> importStop() async {
    final r =
        await http.post(_uri('/api/v1/import/stop'), headers: authHeaders);
    _check(r.statusCode, r.body);
  }

  void _check(int status, String body) {
    if (status >= 200 && status < 300) return;
    String msg = 'HTTP $status';
    try {
      final m = jsonDecode(body);
      if (m is Map && m['error'] != null) msg = '${m['error']}';
    } catch (_) {}
    throw ApiException(status, msg);
  }
}

class ApiException implements Exception {
  ApiException(this.status, this.message);
  final int status;
  final String message;
  @override
  String toString() => message;
}

/// A local file selected for upload.
class UploadFile {
  UploadFile(this.path, this.name);
  final String path;
  final String name;
}
