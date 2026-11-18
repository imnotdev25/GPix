// Data models mirroring the gpix /api/v1 JSON shapes.

int _asInt(dynamic v) => (v is num) ? v.toInt() : int.tryParse('$v') ?? 0;
bool _asBool(dynamic v) => v == true;
String _asStr(dynamic v) => v == null ? '' : '$v';

class MediaItem {
  MediaItem({
    required this.key,
    required this.name,
    required this.className,
    required this.encrypted,
    required this.size,
    required this.mtime,
    required this.thumb,
    required this.original,
    this.thumbPending = false,
    this.tags = const [],
  });

  final String key;
  final String name;
  final String className; // photo | video | doc | file
  final bool encrypted;
  final int size;
  final DateTime? mtime;
  final String thumb; // relative path, e.g. /api/v1/media/<key>/thumb
  final String original;

  /// The server is still rendering this (encrypted) item's thumbnail; until it
  /// finishes, …/thumb returns a 1×1 transparent placeholder.
  final bool thumbPending;
  final List<String> tags;

  bool get isPhoto => className == 'photo';
  bool get isVideo => className == 'video';
  bool get isDoc => className == 'doc';
  bool get hasRenderedImage => isPhoto || isDoc;

  MediaItem copyWith({List<String>? tags}) => MediaItem(
    key: key,
    name: name,
    className: className,
    encrypted: encrypted,
    size: size,
    mtime: mtime,
    thumb: thumb,
    original: original,
    thumbPending: thumbPending,
    tags: tags ?? this.tags,
  );

  factory MediaItem.fromJson(Map<String, dynamic> j) => MediaItem(
    key: _asStr(j['key']),
    name: _asStr(j['name']),
    className: j['class'] == null ? 'file' : '${j['class']}',
    encrypted: _asBool(j['encrypted']),
    size: _asInt(j['size']),
    mtime: DateTime.tryParse(_asStr(j['mtime'])),
    thumb: _asStr(j['thumb']),
    original: _asStr(j['original']),
    thumbPending: _asBool(j['thumbPending']),
    tags: ((j['tags'] as List?) ?? const []).map(_asStr).toList(),
  );
}

class MediaPage {
  MediaPage({
    required this.items,
    required this.nextCursor,
    required this.total,
    this.prevCursor = '',
    this.offset = 0,
  });
  final List<MediaItem> items;
  final String nextCursor; // opaque; send back as `cursor`
  final String prevCursor;
  final int offset;
  final int total;

  factory MediaPage.fromJson(Map<String, dynamic> j) => MediaPage(
    items: ((j['items'] as List?) ?? const [])
        .map((e) => MediaItem.fromJson(e as Map<String, dynamic>))
        .toList(),
    nextCursor: _asStr(j['nextCursor']),
    prevCursor: _asStr(j['prevCursor']),
    offset: _asInt(j['offset']),
    total: _asInt(j['total']),
  );
}

class PingInfo {
  PingInfo({
    required this.app,
    required this.user,
    required this.encryption,
    required this.features,
  });
  final String app;
  final String user;
  final bool encryption;
  final Map<String, bool> features;

  bool feature(String name) => features[name] ?? false;

  factory PingInfo.fromJson(Map<String, dynamic> j) => PingInfo(
    app: j['app'] == null ? 'gpix' : '${j['app']}',
    user: _asStr(j['user']),
    encryption: _asBool(j['encryption']),
    features: ((j['features'] as Map?) ?? const {}).map(
      (k, v) => MapEntry('$k', v == true),
    ),
  );
}

class ShareInfo {
  ShareInfo({
    required this.token,
    required this.title,
    required this.url,
    required this.hasPassword,
    required this.downloads,
    required this.maxDownloads,
    required this.count,
    this.directUrl = '',
  });
  final String token;
  final String title;
  final String url;
  final bool hasPassword;
  final int downloads;
  final int maxDownloads;
  final int count;

  /// Starts the download directly (a .zip for several items). Empty when the
  /// link has a password or doesn't allow originals.
  final String directUrl;

  factory ShareInfo.fromJson(Map<String, dynamic> j) => ShareInfo(
    token: _asStr(j['token']),
    title: _asStr(j['title']),
    url: _asStr(j['url']),
    hasPassword: _asBool(j['hasPassword']),
    downloads: _asInt(j['downloads']),
    maxDownloads: _asInt(j['maxDownloads']),
    count: _asInt(j['count']),
    directUrl: _asStr(j['directUrl']),
  );
}

/// An album made in gpix (not synced with Google Photos albums).
class Album {
  Album({required this.id, required this.name, required this.count, this.cover});
  final int id;
  final String name;
  final int count;
  final MediaItem? cover; // newest item, if any

  factory Album.fromJson(Map<String, dynamic> j) => Album(
    id: _asInt(j['id']),
    name: _asStr(j['name']),
    count: _asInt(j['count']),
    cover: j['cover'] is Map<String, dynamic>
        ? MediaItem.fromJson(j['cover'] as Map<String, dynamic>)
        : null,
  );
}

class TagSummary {
  TagSummary(this.tag, this.count);
  final String tag;
  final int count;

  factory TagSummary.fromJson(Map<String, dynamic> j) =>
      TagSummary(_asStr(j['tag']), _asInt(j['count']));
}

/// How many items one month has (GET /api/v1/media/timeline), newest first.
class TimelineMonth {
  TimelineMonth(this.year, this.month, this.count);
  final int year;
  final int month; // 1–12
  final int count;

  factory TimelineMonth.fromJson(Map<String, dynamic> j) =>
      TimelineMonth(_asInt(j['year']), _asInt(j['month']), _asInt(j['count']));
}

/// A Google account linked to this gpix server.
class LinkedAccount {
  LinkedAccount({required this.id, required this.label, required this.active});
  final int id;
  final String label;
  final bool active;

  factory LinkedAccount.fromJson(Map<String, dynamic> j) => LinkedAccount(
    id: _asInt(j['id']),
    label: _asStr(j['label']),
    active: _asBool(j['active']),
  );
}

class ImportProgress {
  ImportProgress({
    required this.running,
    required this.server,
    required this.workers,
    required this.total,
    required this.done,
    required this.skipped,
    required this.failed,
    required this.current,
    required this.error,
  });
  final bool running;
  final String server;
  final int workers;
  final int total;
  final int done;
  final int skipped;
  final int failed;
  final String current;
  final String error;

  int get processed => done + skipped + failed;
  double get fraction => total > 0 ? (processed / total).clamp(0, 1) : 0;

  factory ImportProgress.fromJson(Map<String, dynamic> j) => ImportProgress(
    running: _asBool(j['running']),
    server: _asStr(j['server']),
    workers: _asInt(j['workers']),
    total: _asInt(j['total']),
    done: _asInt(j['done']),
    skipped: _asInt(j['skipped']),
    failed: _asInt(j['failed']),
    current: _asStr(j['current']),
    error: _asStr(j['error']),
  );

  static ImportProgress empty() => ImportProgress(
    running: false,
    server: '',
    workers: 0,
    total: 0,
    done: 0,
    skipped: 0,
    failed: 0,
    current: '',
    error: '',
  );
}

/// What "Delete gpix uploads" would remove (GET /api/v1/gpix-uploads/summary).
class GpixUploadSummary {
  GpixUploadSummary({
    required this.accountId,
    required this.accountLabel,
    required this.total,
    required this.photos,
    required this.videos,
    required this.docs,
    required this.files,
    required this.bytes,
    required this.tracked,
    required this.disguised,
    required this.trackingEnabled,
    required this.trackingSince,
    required this.sample,
  });

  final int accountId;
  final String accountLabel;
  final int total;
  final int photos;
  final int videos;
  final int docs;
  final int files;
  final int bytes;
  final int tracked; // matched via gpix's upload log
  final int disguised; // matched because gpix wrapped/encrypted them
  final bool trackingEnabled;
  final DateTime? trackingSince;
  final List<String> sample;

  factory GpixUploadSummary.fromJson(Map<String, dynamic> j) =>
      GpixUploadSummary(
        accountId: _asInt(j['accountId']),
        accountLabel: _asStr(j['accountLabel']),
        total: _asInt(j['total']),
        photos: _asInt(j['photos']),
        videos: _asInt(j['videos']),
        docs: _asInt(j['docs']),
        files: _asInt(j['files']),
        bytes: _asInt(j['bytes']),
        tracked: _asInt(j['tracked']),
        disguised: _asInt(j['disguised']),
        trackingEnabled: _asBool(j['trackingEnabled']),
        trackingSince: DateTime.tryParse(_asStr(j['trackingSince'])),
        sample: ((j['sample'] as List?) ?? const []).map(_asStr).toList(),
      );
}

/// Result of POST /api/v1/gpix-uploads/delete.
class GpixUploadDeleteResult {
  GpixUploadDeleteResult({
    required this.total,
    required this.deleted,
    required this.failed,
  });
  final int total;
  final int deleted;
  final int failed;

  factory GpixUploadDeleteResult.fromJson(Map<String, dynamic> j) =>
      GpixUploadDeleteResult(
        total: _asInt(j['total']),
        deleted: _asInt(j['deleted']),
        failed: _asInt(j['failed']),
      );
}
