; Inno Setup script for LeGaJ (Windows installer)
; -------------------------------------------------
; Compile with:  ISCC.exe packaging\legaj.iss
; (ISCC is the Inno Setup command-line compiler: https://jrsoftware.org/isinfo.php)
;
; This expects build_windows.ps1 to have staged the application into
; packaging\staging\ with this layout:
;   staging\legaj.exe
;   staging\tools\legaj-tools.exe
;
; The app uses RELATIVE paths (references/, extension/, tools/), so it must run
; with its working directory set to the install folder, and that folder must be
; writable. We therefore install per-user into %LOCALAPPDATA% (no admin needed)
; and set the shortcut's working directory accordingly.

#define MyAppName "LeGaJ"
#define MyAppVersion "1.0.0"
#define MyAppPublisher "Roberto Montero"
#define MyAppURL "https://github.com/bot-bbio"
#define MyAppExeName "legaj.exe"

[Setup]
AppId={{B7A1E9C4-3F2D-4C8E-9A1B-5E6F7A8B9C0D}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
DefaultDirName={localappdata}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
; Per-user install — no administrator rights required.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputDir=dist
OutputBaseFilename=LeGaJ-Setup-{#MyAppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; Stage everything under packaging\staging before compiling (see build_windows.ps1).
Source: "staging\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Dirs]
; Writable working directories the app creates/uses at runtime.
Name: "{app}\references"
Name: "{app}\extension"
Name: "{app}\outputs"

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{userdesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; WorkingDir: "{app}"; Flags: nowait postinstall skipifsilent
