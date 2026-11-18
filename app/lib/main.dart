import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'src/state/app_state.dart';
import 'src/screens/login_screen.dart';
import 'src/screens/home_screen.dart';
import 'src/theme.dart';

void main() {
  runApp(const GpixApp());
}

final _lightTheme = buildGpixTheme(Brightness.light);
final _darkTheme = buildGpixTheme(Brightness.dark);

class GpixApp extends StatelessWidget {
  const GpixApp({super.key});

  @override
  Widget build(BuildContext context) {
    return ChangeNotifierProvider(
      create: (_) => AppState()..load(),
      child: Consumer<AppState>(
        builder: (context, state, _) {
          return MaterialApp(
            title: 'gpix',
            debugShowCheckedModeBanner: false,
            theme: _lightTheme,
            darkTheme: _darkTheme,
            themeMode: state.themeMode,
            home: state.loading
                ? const Scaffold(body: Center(child: CircularProgressIndicator()))
                : (state.isSignedIn ? const HomeScreen() : const LoginScreen()),
          );
        },
      ),
    );
  }
}
