import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/app_state.dart';
import '../state/backup_service.dart';
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
    _Dest('Upload', Icons.upload_outlined, Icons.upload),
    _Dest('Backup', Icons.backup_outlined, Icons.backup),
    _Dest('Shares', Icons.share_outlined, Icons.share),
    _Dest('Import', Icons.download_outlined, Icons.download),
    _Dest('Settings', Icons.settings_outlined, Icons.settings),
  ];

  final _screens = const [
    LibraryScreen(),
    UploadScreen(),
    BackupScreen(),
    SharesScreen(),
    ImportScreen(),
    SettingsScreen(),
  ];

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.of(context).size.width >= 720;
    final body = IndexedStack(index: _index, children: _screens);

    final Widget scaffold;
    if (wide) {
      scaffold = Scaffold(
        body: Row(
          children: [
            NavigationRail(
              selectedIndex: _index,
              onDestinationSelected: (i) => setState(() => _index = i),
              labelType: NavigationRailLabelType.all,
              destinations: [
                for (final d in _destinations)
                  NavigationRailDestination(
                    icon: Icon(d.icon),
                    selectedIcon: Icon(d.selected),
                    label: Text(d.label),
                  ),
              ],
            ),
            const VerticalDivider(width: 1),
            Expanded(child: body),
          ],
        ),
      );
    } else {
      scaffold = Scaffold(
        body: body,
        bottomNavigationBar: NavigationBar(
          selectedIndex: _index,
          onDestinationSelected: (i) => setState(() => _index = i),
          labelBehavior: NavigationDestinationLabelBehavior.onlyShowSelected,
          destinations: [
            for (final d in _destinations)
              NavigationDestination(
                icon: Icon(d.icon),
                selectedIcon: Icon(d.selected),
                label: d.label,
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
