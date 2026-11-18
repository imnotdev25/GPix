import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/app_state.dart';
import '../state/backup_service.dart';
import '../theme.dart';
import 'albums_screen.dart';
import 'library_screen.dart';
import 'upload_screen.dart';
import 'backup_screen.dart';
import 'shares_screen.dart';
import 'import_screen.dart';
import 'settings_screen.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  int _index = 0;
  late final BackupService _backup;

  @override
  void initState() {
    super.initState();
    _backup = BackupService(context.read<AppState>().client!);
  }

  @override
  void dispose() {
    _backup.dispose();
    super.dispose();
  }

  static const _destinations = [
    _Dest('Library', Icons.photo_library_outlined, Icons.photo_library),
    _Dest('Albums', Icons.photo_album_outlined, Icons.photo_album),
    _Dest('Upload', Icons.cloud_upload_outlined, Icons.cloud_upload),
    _Dest('Backup', Icons.backup_outlined, Icons.backup),
    _Dest('Shares', Icons.share_outlined, Icons.share),
    _Dest('Import', Icons.download_outlined, Icons.download),
    _Dest('Settings', Icons.settings_outlined, Icons.settings),
  ];
  static const _albumsIndex = 1;
  static const _importIndex = 5; // on phones Import lives in Settings instead

  // Bumped when the Albums tab is opened, so it reloads its counts.
  int _albumsGen = 0;

  List<Widget> get _screens => [
    const LibraryScreen(),
    AlbumsScreen(key: ValueKey(_albumsGen)),
    const UploadScreen(),
    const BackupScreen(),
    const SharesScreen(),
    const ImportScreen(),
    const SettingsScreen(),
  ];

  void _select(int i) => setState(() {
    if (i == _albumsIndex && _index != _albumsIndex) _albumsGen++;
    _index = i;
  });

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final wide = width >= 720;
    final extended = width >= 1200;
    final body = IndexedStack(index: _index, children: _screens);

    final Widget scaffold;
    if (wide) {
      scaffold = Scaffold(
        body: Row(
          children: [
            NavigationRail(
              extended: extended,
              minExtendedWidth: 200,
              selectedIndex: _index,
              onDestinationSelected: _select,
              labelType: extended
                  ? NavigationRailLabelType.none
                  : NavigationRailLabelType.all,
              leading: Padding(
                padding: const EdgeInsets.only(top: 8, bottom: 16),
                child: extended
                    ? const Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          BrandMark(size: 32),
                          SizedBox(width: 10),
                          Text('gpix',
                              style: TextStyle(
                                  fontSize: 18, fontWeight: FontWeight.w700)),
                        ],
                      )
                    : const BrandMark(size: 36),
              ),
              destinations: [
                for (final d in _destinations)
                  NavigationRailDestination(
                    icon: Icon(d.icon),
                    selectedIcon: Icon(d.selected),
                    label: Text(d.label),
                  ),
              ],
            ),
            VerticalDivider(
                width: 1, thickness: 1, color: Theme.of(context).dividerColor),
            Expanded(child: body),
          ],
        ),
      );
    } else {
      // Six tabs fit a phone; Import is reached from Settings.
      final tabs = [
        for (var i = 0; i < _destinations.length; i++)
          if (i != _importIndex) i,
      ];
      final sel = tabs.indexOf(_index);
      scaffold = Scaffold(
        body: body,
        bottomNavigationBar: NavigationBar(
          selectedIndex: sel < 0 ? tabs.length - 1 : sel,
          onDestinationSelected: (j) => _select(tabs[j]),
          labelBehavior: NavigationDestinationLabelBehavior.onlyShowSelected,
          destinations: [
            for (final i in tabs)
              NavigationDestination(
                icon: Icon(_destinations[i].icon),
                selectedIcon: Icon(_destinations[i].selected),
                label: _destinations[i].label,
              ),
          ],
        ),
      );
    }

    return ChangeNotifierProvider<BackupService>.value(
      value: _backup,
      child: scaffold,
    );
  }
}

class _Dest {
  const _Dest(this.label, this.icon, this.selected);
  final String label;
  final IconData icon;
  final IconData selected;
}
