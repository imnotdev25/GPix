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
  });

  final String key;
  final String name;
  final String className; // photo | video | doc | file
  final bool encrypted;
  final int size;
  final DateTime? mtime;
  final String thumb; // relative path, e.g. /api/v1/media/<key>/thumb
  final String original;

  bool get isPhoto => className == 'photo';
  bool get isVideo => className == 'video';
  bool get isDoc => className == 'doc';
  bool get hasRenderedImage => isPhoto || isDoc;

  factory MediaItem.fromJson(Map<String, dynamic> j) => MediaItem(
        key: _asStr(j['key']),
        name: _asStr(j['name']),
        className: j['class'] == null ? 'file' : '${j['class']}',
        encrypted: _asBool(j['encrypted']),
        size: _asInt(j['size']),
        mtime: DateTime.tryParse(_asStr(j['mtime'])),
        thumb: _asStr(j['thumb']),
        original: _asStr(j['original']),
      );
}

class MediaPage {
  MediaPage({required this.items, required this.nextCursor, required this.total});
  final List<MediaItem> items;
  final String nextCursor;
  final int total;

  factory MediaPage.fromJson(Map<String, dynamic> j) => MediaPage(
        items: ((j['items'] as List?) ?? const [])
            .map((e) => MediaItem.fromJson(e as Map<String, dynamic>))
            .toList(),
        nextCursor: _asStr(j['nextCursor']),
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
        features: ((j['features'] as Map?) ?? const {})
            .map((k, v) => MapEntry('$k', v == true)),
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
  });
  final String token;
  final String title;
  final String url;
  final bool hasPassword;
  final int downloads;
  final int maxDownloads;
  final int count;

  factory ShareInfo.fromJson(Map<String, dynamic> j) => ShareInfo(
        token: _asStr(j['token']),
        title: _asStr(j['title']),
        url: _asStr(j['url']),
        hasPassword: _asBool(j['hasPassword']),
        downloads: _asInt(j['downloads']),
        maxDownloads: _asInt(j['maxDownloads']),
        count: _asInt(j['count']),
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
