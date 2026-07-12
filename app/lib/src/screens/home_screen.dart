import 'package:flutter/material.dart';

import 'library_screen.dart';
import 'upload_screen.dart';
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

  static const _destinations = [
    _Dest('Library', Icons.photo_library_outlined, Icons.photo_library),
    _Dest('Upload', Icons.upload_outlined, Icons.upload),
    _Dest('Shares', Icons.share_outlined, Icons.share),
    _Dest('Import', Icons.download_outlined, Icons.download),
    _Dest('Settings', Icons.settings_outlined, Icons.settings),
  ];

  final _screens = const [
    LibraryScreen(),
    UploadScreen(),
    SharesScreen(),
    ImportScreen(),
    SettingsScreen(),
  ];

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.of(context).size.width >= 720;
    final body = IndexedStack(index: _index, children: _screens);

    if (wide) {
      return Scaffold(
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
    }

    return Scaffold(
      body: body,
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
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
}

class _Dest {
  const _Dest(this.label, this.icon, this.selected);
  final String label;
  final IconData icon;
  final IconData selected;
}
