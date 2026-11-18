import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'models.dart';

/// ApiClient talks to a gpix server's /api/v1 JSON API using a Bearer token.
class ApiClient {
  ApiClient({required String baseUrl, required String token})
      : baseUrl = normalizeBaseUrl(baseUrl),
        // Trim aggressively: a pasted token often carries a trailing newline or
        // spaces, and Dart's http client THROWS on header values containing
        // newlines — so the request would go out with no Authorization header.
        token = token.trim();

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
  /// For encrypted/disguised items it hints the display name so the server can
  /// render the real thumbnail locally without a warm library cache.
  String thumbUrl(MediaItem it, {int size = 256}) {
    final u = '$baseUrl${it.thumb}?size=$size';
    if (it.encrypted) {
      return '$u&enc=1&name=${Uri.encodeQueryComponent(it.name)}';
    }
    return u;
  }

  /// Absolute original-bytes URL (send authHeaders).
  String originalUrl(MediaItem it, {bool download = false}) =>
      '$baseUrl${it.original}${download ? '?download=1' : ''}';

  Future<PingInfo> ping() async {
    final r = await http.get(_uri('/api/v1/ping'), headers: authHeaders);
    _check(r.statusCode, r.body);
    return PingInfo.fromJson(jsonDecode(r.body) as Map<String, dynamic>);
  }

  /// A page of up to 50 items, newest first. [cursor] continues down,
  /// [before] (a page's prevCursor) goes back up, and [from] starts at the
  /// first item taken before that moment. [albumId] / [tag] narrow it.
  /// Throws ApiException 503 while the server's first listing is running.
  Future<MediaPage> listMedia({
    String cursor = '',
    String before = '',
    DateTime? from,
    int? albumId,
    String? tag,
  }) async {
    final q = <String, String>{'limit': '50'};
    if (cursor.isNotEmpty) q['cursor'] = cursor;
    if (before.isNotEmpty) q['before'] = before;
    if (from != null && cursor.isEmpty && before.isEmpty) {
      q['from'] = '${from.millisecondsSinceEpoch}';
    }
    _narrow(q, albumId, tag);
    final r = await http.get(_uri('/api/v1/media', q), headers: authHeaders);
    _check(r.statusCode, r.body);
    return MediaPage.fromJson(_obj(r.body));
  }

  /// Items per month, newest first, in this device's time zone.
  Future<List<TimelineMonth>> timeline({int? albumId, String? tag}) async {
    final q = <String, String>{
      'tzOffset': '${DateTime.now().timeZoneOffset.inMinutes}',
    };
    _narrow(q, albumId, tag);
    final r = await http.get(
      _uri('/api/v1/media/timeline', q),
      headers: authHeaders,
    );
    _check(r.statusCode, r.body);
    return _maps(_obj(r.body)['months']).map(TimelineMonth.fromJson).toList();
  }

  /// Moves items to the Google Photos trash in one request.
  Future<({Set<String> deleted, int failed})> deleteMany(
    List<String> keys,
  ) async {
    final res = await _postJson('/api/v1/media/delete', {'keys': keys});
    return (
      deleted: ((res['deleted'] as List?) ?? const []).map((e) => '$e').toSet(),
      failed: (res['failed'] as num?)?.toInt() ?? 0,
    );
  }

  // ---- albums and tags (stored by gpix) -------------------------------------

  Future<List<Album>> listAlbums() async {
    final r = await http.get(_uri('/api/v1/albums'), headers: authHeaders);
    _check(r.statusCode, r.body);
    return _maps(_obj(r.body)['albums']).map(Album.fromJson).toList();
  }

  Future<Album> createAlbum(String name, {List<String> keys = const []}) async =>
      Album.fromJson(
        await _postJson('/api/v1/albums', {'name': name, 'keys': keys}),
      );

  Future<void> renameAlbum(int id, String name) =>
      _postJson('/api/v1/albums/$id', {'name': name});

  Future<void> deleteAlbum(int id) async {
    final r = await http.delete(_uri('/api/v1/albums/$id'), headers: authHeaders);
    _check(r.statusCode, r.body);
  }

  Future<void> addToAlbum(int id, List<String> keys) =>
      _postJson('/api/v1/albums/$id/items', {'keys': keys});

  Future<void> removeFromAlbum(int id, List<String> keys) =>
      _postJson('/api/v1/albums/$id/items/remove', {'keys': keys});

  Future<List<TagSummary>> listTags() async {
    final r = await http.get(_uri('/api/v1/tags'), headers: authHeaders);
    _check(r.statusCode, r.body);
    return _maps(_obj(r.body)['tags']).map(TagSummary.fromJson).toList();
  }

  /// Adds tags; returns every key's tags afterwards.
  Future<Map<String, List<String>>> addTags(
    List<String> keys,
    List<String> tags,
  ) async => _tagMap(
    await _postJson('/api/v1/tags/add', {'keys': keys, 'tags': tags}),
    keys,
  );

  /// Removes tags; returns every key's tags afterwards.
  Future<Map<String, List<String>>> removeTags(
    List<String> keys,
    List<String> tags,
  ) async => _tagMap(
    await _postJson('/api/v1/tags/remove', {'keys': keys, 'tags': tags}),
    keys,
  );

  // ---- accounts and Google import --------------------------------------------

  Future<List<LinkedAccount>> listAccounts() async {
    final r = await http.get(_uri('/api/v1/accounts'), headers: authHeaders);
    _check(r.statusCode, r.body);
    return _maps(_obj(r.body)['accounts']).map(LinkedAccount.fromJson).toList();
  }

  /// Copies another Google account's library into the active one: a linked
  /// account ([accountId]) or a pasted GP_AUTH_DATA ([authData]).
  Future<void> importGoogleStart({
    int? accountId,
    String authData = '',
    int workers = 3,
  }) => _postJson('/api/v1/import/google-start', {
    if (accountId != null) 'accountId': accountId,
    if (accountId == null) 'authData': authData.trim(),
    'workers': workers,
  });

  Future<Uint8List> fetchOriginal(MediaItem it) async {
    final r = await http.get(_uri(it.original, {'download': '1'}),
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

  /// Counts what "Delete gpix uploads" would remove from the active account.
  Future<GpixUploadSummary> gpixUploadsSummary() async {
    final r = await http.get(_uri('/api/v1/gpix-uploads/summary'),
        headers: authHeaders);
    _check(r.statusCode, r.body);
    return GpixUploadSummary.fromJson(
        jsonDecode(r.body) as Map<String, dynamic>);
  }

  /// Trashes every gpix upload. The server refuses (HTTP 409) if the account
  /// or the count no longer matches [reviewed].
  Future<GpixUploadDeleteResult> deleteGpixUploads(
      GpixUploadSummary reviewed) async {
    final r = await http.post(
      _uri('/api/v1/gpix-uploads/delete'),
      headers: {...authHeaders, 'Content-Type': 'application/json'},
      body: jsonEncode({
        'confirm': 'DELETE',
        'accountId': reviewed.accountId,
        'expected': reviewed.total,
      }),
    );
    _check(r.statusCode, r.body);
    return GpixUploadDeleteResult.fromJson(
        jsonDecode(r.body) as Map<String, dynamic>);
  }

  void _narrow(Map<String, String> q, int? albumId, String? tag) {
    if (albumId != null) q['album'] = '$albumId';
    if (tag != null && tag.isNotEmpty) q['tag'] = tag;
  }

  Map<String, dynamic> _obj(String body) {
    if (body.trim().isEmpty) return const {};
    final d = jsonDecode(body);
    return d is Map<String, dynamic> ? d : const {};
  }

  List<Map<String, dynamic>> _maps(dynamic v) =>
      ((v as List?) ?? const []).whereType<Map<String, dynamic>>().toList();

  Map<String, List<String>> _tagMap(Map<String, dynamic> res, List<String> keys) {
    final raw = (res['tags'] as Map?) ?? const {};
    return {
      for (final k in keys)
        k: ((raw[k] as List?) ?? const []).map((e) => '$e').toList(),
    };
  }

  Future<Map<String, dynamic>> _postJson(String path, Object body) async {
    final r = await http.post(
      _uri(path),
      headers: {...authHeaders, 'Content-Type': 'application/json'},
      body: jsonEncode(body),
    );
    _check(r.statusCode, r.body);
    return _obj(r.body);
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
