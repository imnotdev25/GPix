import 'package:flutter/material.dart';

/// gpix colors, shared with the web UI (ui/src/styles.css).
class GpixPalette {
  const GpixPalette({
    required this.bg,
    required this.surface,
    required this.surface2,
    required this.border,
    required this.text,
    required this.muted,
  });

  final Color bg;
  final Color surface;
  final Color surface2;
  final Color border;
  final Color text;
  final Color muted;

  static const accent = Color(0xFF3B6EF6);
  static const accent2 = Color(0xFF7C4DE6);
  static const ok = Color(0xFF2FBF71);
  static const danger = Color(0xFFE5484D);

  static const dark = GpixPalette(
    bg: Color(0xFF0B0D10),
    surface: Color(0xFF14181D),
    surface2: Color(0xFF1C2128),
    border: Color(0xFF262C34),
    text: Color(0xFFE8ECF1),
    muted: Color(0xFF9AA5B1),
  );

  static const light = GpixPalette(
    bg: Color(0xFFF7F8FA),
    surface: Color(0xFFFFFFFF),
    surface2: Color(0xFFEEF1F5),
    border: Color(0xFFDFE3E9),
    text: Color(0xFF12161B),
    muted: Color(0xFF5B6673),
  );
}

/// Extra palette entries that ColorScheme has no slot for.
@immutable
class GpixColors extends ThemeExtension<GpixColors> {
  const GpixColors({
    required this.ok,
    required this.muted,
    required this.border,
    required this.surface2,
  });

  final Color ok;
  final Color muted;
  final Color border;
  final Color surface2;

  @override
  GpixColors copyWith(
          {Color? ok, Color? muted, Color? border, Color? surface2}) =>
      GpixColors(
        ok: ok ?? this.ok,
        muted: muted ?? this.muted,
        border: border ?? this.border,
        surface2: surface2 ?? this.surface2,
      );

  @override
  GpixColors lerp(ThemeExtension<GpixColors>? other, double t) {
    if (other is! GpixColors) return this;
    return GpixColors(
      ok: Color.lerp(ok, other.ok, t)!,
      muted: Color.lerp(muted, other.muted, t)!,
      border: Color.lerp(border, other.border, t)!,
      surface2: Color.lerp(surface2, other.surface2, t)!,
    );
  }
}

extension GpixThemeContext on BuildContext {
  GpixColors get gpix => Theme.of(this).extension<GpixColors>()!;
}

ThemeData buildGpixTheme(Brightness brightness) {
  final p =
      brightness == Brightness.dark ? GpixPalette.dark : GpixPalette.light;
  final scheme = ColorScheme.fromSeed(
    seedColor: GpixPalette.accent,
    brightness: brightness,
  ).copyWith(
    primary: GpixPalette.accent,
    onPrimary: Colors.white,
    secondary: GpixPalette.accent2,
    onSecondary: Colors.white,
    error: GpixPalette.danger,
    onError: Colors.white,
    surface: p.surface,
    onSurface: p.text,
    onSurfaceVariant: p.muted,
    surfaceContainerLowest: p.bg,
    surfaceContainerLow: p.surface,
    surfaceContainer: p.surface,
    surfaceContainerHigh: p.surface2,
    surfaceContainerHighest: p.surface2,
    outline: p.border,
    outlineVariant: p.border,
    surfaceTint: Colors.transparent,
  );

  const radius = BorderRadius.all(Radius.circular(12));
  const smallRadius = BorderRadius.all(Radius.circular(9));
  final side = BorderSide(color: p.border);
  const smallShape = RoundedRectangleBorder(borderRadius: smallRadius);

  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorScheme: scheme,
    scaffoldBackgroundColor: p.bg,
    canvasColor: p.bg,
    dividerColor: p.border,
    dividerTheme: DividerThemeData(color: p.border, space: 1, thickness: 1),
    extensions: [
      GpixColors(
        ok: GpixPalette.ok,
        muted: p.muted,
        border: p.border,
        surface2: p.surface2,
      ),
    ],
    appBarTheme: AppBarTheme(
      backgroundColor: p.bg,
      foregroundColor: p.text,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: TextStyle(
          color: p.text, fontSize: 18, fontWeight: FontWeight.w600),
    ),
    cardTheme: CardThemeData(
      color: p.surface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(borderRadius: radius, side: side),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: p.surface,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: radius, side: side),
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: p.surface,
      surfaceTintColor: Colors.transparent,
      showDragHandle: true,
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: p.surface2,
      isDense: true,
      border: OutlineInputBorder(borderRadius: smallRadius, borderSide: side),
      enabledBorder:
          OutlineInputBorder(borderRadius: smallRadius, borderSide: side),
      focusedBorder: const OutlineInputBorder(
        borderRadius: smallRadius,
        borderSide: BorderSide(color: GpixPalette.accent, width: 1.5),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        shape: smallShape,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: p.text,
        side: side,
        shape: smallShape,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(shape: smallShape),
    ),
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: SegmentedButton.styleFrom(
        side: side,
        selectedBackgroundColor: p.surface2,
        selectedForegroundColor: p.text,
        foregroundColor: p.muted,
        shape: smallShape,
      ),
    ),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: p.surface2,
      contentTextStyle: TextStyle(color: p.text),
      actionTextColor: GpixPalette.accent,
      shape: RoundedRectangleBorder(borderRadius: smallRadius, side: side),
    ),
    navigationRailTheme: NavigationRailThemeData(
      backgroundColor: p.surface,
      indicatorColor: p.surface2,
      selectedIconTheme: IconThemeData(color: p.text),
      unselectedIconTheme: IconThemeData(color: p.muted),
      selectedLabelTextStyle: TextStyle(
          color: p.text, fontWeight: FontWeight.w600, fontSize: 13),
      unselectedLabelTextStyle: TextStyle(color: p.muted, fontSize: 13),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: p.surface,
      indicatorColor: p.surface2,
      surfaceTintColor: Colors.transparent,
      height: 64,
    ),
    listTileTheme: ListTileThemeData(iconColor: p.muted),
    progressIndicatorTheme: ProgressIndicatorThemeData(
      color: GpixPalette.accent,
      linearTrackColor: p.surface2,
    ),
  );
}

/// The gpix logo mark: a gradient rounded square with a "g", as in the web
/// sidebar.
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.size = 32});
  final double size;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(size * 0.3),
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [GpixPalette.accent, GpixPalette.accent2],
        ),
      ),
      child: Text(
        'g',
        style: TextStyle(
          color: Colors.white,
          fontWeight: FontWeight.w700,
          fontSize: size * 0.5,
          height: 1,
        ),
      ),
    );
  }
}
