; Inno Setup script for the gpix Windows desktop app.
; Built by tool/package_windows.ps1, which passes the defines below. Needs
; Inno Setup 6.3 or newer (for "x64compatible").

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef BundleDir
  #define BundleDir "..\..\build\windows\x64\runner\Release"
#endif
#ifndef IconFile
  #define IconFile "..\..\windows\runner\resources\app_icon.ico"
#endif
#ifndef OutDir
  #define OutDir "..\..\out"
#endif
#ifndef OutName
  #define OutName "gpix-setup"
#endif

[Setup]
AppId={{6B1E2F4A-9C3D-4E7B-A8F1-2D5C7E9B0A13}
AppName=gpix
AppVersion={#AppVersion}
AppPublisher=gpix
DefaultDirName={autopf}\gpix
DefaultGroupName=gpix
DisableProgramGroupPage=yes
; Per-user install by default (no admin prompt); the user may choose all users.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile={#IconFile}
UninstallDisplayIcon={app}\gpix_app.exe
UninstallDisplayName=gpix
OutputDir={#OutDir}
OutputBaseFilename={#OutName}
WizardStyle=modern
Compression=lzma2
SolidCompression=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#BundleDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\gpix"; Filename: "{app}\gpix_app.exe"
Name: "{autodesktop}\gpix"; Filename: "{app}\gpix_app.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\gpix_app.exe"; Description: "{cm:LaunchProgram,gpix}"; Flags: nowait postinstall skipifsilent
