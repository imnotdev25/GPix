import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'src/state/app_state.dart';
import 'src/screens/login_screen.dart';
import 'src/screens/home_screen.dart';

void main() {
  runApp(const GpixApp());
}

class GpixApp extends StatelessWidget {
  const GpixApp({super.key});

  @override
  Widget build(BuildContext context) {
    const seed = Color(0xFF3B6EF6);
    return ChangeNotifierProvider(
      create: (_) => AppState()..load(),
      child: Consumer<AppState>(
        builder: (context, state, _) {
          return MaterialApp(
            title: 'gpix',
            debugShowCheckedModeBanner: false,
            theme: ThemeData(
              useMaterial3: true,
              colorScheme: ColorScheme.fromSeed(seedColor: seed),
            ),
            darkTheme: ThemeData(
              useMaterial3: true,
              colorScheme: ColorScheme.fromSeed(
                seedColor: seed,
                brightness: Brightness.dark,
              ),
            ),
            themeMode: ThemeMode.system,
            home: state.loading
                ? const Scaffold(body: Center(child: CircularProgressIndicator()))
                : (state.isSignedIn ? const HomeScreen() : const LoginScreen()),
          );
        },
      ),
    );
  }
}
