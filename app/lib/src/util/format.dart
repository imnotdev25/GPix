import 'package:flutter/material.dart';

String formatBytes(int n) {
  if (n < 1024) return '$n B';
  const units = ['KB', 'MB', 'GB', 'TB'];
  double v = n / 1024;
  var i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return '${v.toStringAsFixed(v >= 10 ? 0 : 1)} ${units[i]}';
}

/// The local calendar day of [t], or null when unknown.
DateTime? dayOf(DateTime? t) {
  if (t == null) return null;
  final l = t.toLocal();
  return DateTime(l.year, l.month, l.day);
}

/// "Today", "Yesterday", "Wed, Nov 12", or "Wed, Nov 12, 2025" for other years.
String dayLabel(BuildContext context, DateTime? day) {
  if (day == null) return 'Unknown date';
  final now = DateTime.now();
  if (day == DateTime(now.year, now.month, now.day)) return 'Today';
  if (day == DateTime(now.year, now.month, now.day - 1)) return 'Yesterday';
  final s = MaterialLocalizations.of(context).formatMediumDate(day);
  return day.year == now.year ? s : '$s, ${day.year}';
}

/// Full date plus time of day, in local time.
String formatDateTime(BuildContext context, DateTime t) {
  final l = t.toLocal();
  final loc = MaterialLocalizations.of(context);
  return '${loc.formatFullDate(l)} · '
      '${loc.formatTimeOfDay(TimeOfDay.fromDateTime(l))}';
}
