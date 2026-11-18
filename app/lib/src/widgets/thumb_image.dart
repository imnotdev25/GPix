import 'dart:async';

import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/models.dart';

/// Retry delays for thumbnails the server is still rendering.
const _backoff = [
  Duration(milliseconds: 1500),
  Duration(milliseconds: 2500),
  Duration(seconds: 4),
  Duration(seconds: 6),
  Duration(seconds: 9),
  Duration(seconds: 13),
  Duration(seconds: 20),
  Duration(seconds: 30),
];

/// Unique per app run, so cache-busted URLs never hit a placeholder that was
/// cached on disk by an earlier run.
final _session = DateTime.now().millisecondsSinceEpoch.toRadixString(36);

/// A media thumbnail at [size]. Items with `thumbPending` get a 1×1 placeholder
/// from the server until it finishes rendering, so those are re-requested on a
/// backoff with a cache-busting URL until a real image arrives. The plain URL
/// is never fetched while pending: flutter_cache_manager ignores
/// `Cache-Control: no-store` and would keep the placeholder for 30 days.
class ThumbImage extends StatefulWidget {
  const ThumbImage({
    super.key,
    required this.item,
    required this.client,
    required this.size,
    this.fit = BoxFit.cover,
    this.memCacheWidth,
    this.placeholder,
    this.error,
  });

  final MediaItem item;
  final ApiClient client;
  final int size;
  final BoxFit fit;
  final int? memCacheWidth;
  final Widget? placeholder; // shown while loading / still pending
  final Widget? error;

  @override
  State<ThumbImage> createState() => _ThumbImageState();
}

class _ThumbImageState extends State<ThumbImage> {
  int _attempt = 0;
  bool _resolved = false; // a real (>1×1) image arrived for the busted URL
  bool _gaveUp = false;
  Timer? _timer;
  ImageStream? _stream;
  ImageStreamListener? _listener;

  bool get _pending => widget.item.thumbPending && !_resolved;

  String get _baseUrl => widget.client.thumbUrl(widget.item, size: widget.size);
  String get _bustedUrl => '$_baseUrl&_p=$_session-$_attempt';

  ImageProvider get _pendingProvider => CachedNetworkImageProvider(
    _bustedUrl,
    headers: widget.client.authHeaders,
  );

  @override
  void initState() {
    super.initState();
    if (_pending) _watch();
  }

  @override
  void didUpdateWidget(ThumbImage old) {
    super.didUpdateWidget(old);
    final changed =
        old.item.key != widget.item.key ||
        old.size != widget.size ||
        old.item.thumbPending != widget.item.thumbPending;
    if (!changed) return;
    _stop();
    if (old.item.key != widget.item.key || old.size != widget.size) {
      _attempt = 0;
      _resolved = false;
      _gaveUp = false;
    }
    if (_pending) _watch();
  }

  @override
  void dispose() {
    _stop();
    super.dispose();
  }

  void _stop() {
    _timer?.cancel();
    _timer = null;
    if (_listener != null) _stream?.removeListener(_listener!);
    _stream = null;
    _listener = null;
  }

  /// Watches the current busted URL; on a placeholder, schedules the next try.
  void _watch() {
    _stop();
    final stream = _pendingProvider.resolve(ImageConfiguration.empty);
    final listener = ImageStreamListener(
      (info, _) {
        final real = info.image.width > 1 || info.image.height > 1;
        info.dispose();
        if (!mounted) return;
        if (real) {
          _stop();
          setState(() => _resolved = true);
        } else {
          _retryLater();
        }
      },
      onError: (_, __) {
        if (mounted) _retryLater();
      },
    );
    stream.addListener(listener);
    _stream = stream;
    _listener = listener;
  }

  void _retryLater() {
    if (_listener != null) _stream?.removeListener(_listener!);
    _stream = null;
    _listener = null;
    if (_attempt >= _backoff.length) {
      setState(() => _gaveUp = true);
      return;
    }
    // Drop the placeholder this attempt cached (memory + disk).
    CachedNetworkImage.evictFromCache(_bustedUrl);
    final delay = _backoff[_attempt];
    _timer = Timer(delay, () {
      if (!mounted) return;
      setState(() => _attempt++);
      _watch();
    });
  }

  @override
  Widget build(BuildContext context) {
    final placeholder = widget.placeholder ?? const SizedBox.shrink();
    if (!widget.item.thumbPending) {
      return CachedNetworkImage(
        imageUrl: _baseUrl,
        httpHeaders: widget.client.authHeaders,
        fit: widget.fit,
        fadeInDuration: const Duration(milliseconds: 120),
        memCacheWidth: widget.memCacheWidth,
        placeholder: (_, __) => placeholder,
        errorWidget: (_, __, ___) => widget.error ?? const SizedBox.shrink(),
      );
    }
    if (_gaveUp && !_resolved) return widget.error ?? placeholder;
    // Pending (or resolved this run): show whatever the busted URL returned.
    // Until it is real that is a transparent 1×1, so the placeholder shows
    // through.
    return Stack(
      fit: StackFit.expand,
      children: [
        if (!_resolved) placeholder,
        Image(
          image: _pendingProvider,
          fit: widget.fit,
          gaplessPlayback: true,
          errorBuilder: (_, __, ___) => const SizedBox.shrink(),
        ),
      ],
    );
  }
}
