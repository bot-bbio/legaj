; LeGaJ V2 installer (R38). Compile with Inno Setup 6:
;
;   ISCC.exe /DMyAppVersion=2.0.0 /DWebView2Bootstrapper=<path> packaging\legaj.iss
;
; - Installs per user to %LOCALAPPDATA%\Programs\LeGaJ: no admin prompt.
; - Runs the WebView2 bootstrapper only if WebView2 is missing. The
;   bootstrapper is bundled only when WebView2Bootstrapper is defined (the
;   release workflow downloads and signature-checks it); verify.ps1 compiles
;   without it.
; - The uninstaller asks "Keep your applications and documents?" (default
;   Yes). Data lives in %LOCALAPPDATA%\LeGaJ, outside the install folder.

#ifndef MyAppVersion
  #define MyAppVersion "0.0.0-dev"
#endif
#define MyAppName "LeGaJ"
#define MyAppPublisher "Roberto Montero"
#define MyAppURL "https://github.com/bot-bbio/legaj"
#define MyAppExeName "legaj.exe"
; Held by the running app (internal/instance.MutexName): Setup and the
; uninstaller wait for LeGaJ to close instead of replacing a running exe.
#define MyAppMutex "Local\LeGaJ-single-instance-3b9c6f1e-legaj-v2"
; WebView2 Runtime's EdgeUpdate client ID (Microsoft's documented check).
#define WebView2ClientKey "Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

[Setup]
; A new AppId for V2. V1 installed into %LOCALAPPDATA%\LeGaJ, which is V2's
; data folder; V2 must never install there or inherit V1's folder.
AppId={{6E2B0F4A-9C1D-4B7E-8A3F-2D5C7E9B1A40}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
AppMutex={#MyAppMutex}
DefaultDirName={localappdata}\Programs\{#MyAppName}
UsePreviousAppDir=no
DisableDirPage=yes
DisableProgramGroupPage=yes
DefaultGroupName={#MyAppName}
PrivilegesRequired=lowest
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
OutputDir=dist
OutputBaseFilename=LeGaJ-Setup-{#MyAppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "..\build\bin\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
#ifdef WebView2Bootstrapper
Source: "{#WebView2Bootstrapper}"; DestDir: "{tmp}"; DestName: "MicrosoftEdgeWebview2Setup.exe"; Flags: deleteafterinstall; Check: NeedsWebView2
#endif

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
#ifdef WebView2Bootstrapper
Filename: "{tmp}\MicrosoftEdgeWebview2Setup.exe"; Parameters: "/silent /install"; StatusMsg: "Installing Microsoft Edge WebView2 (LeGaJ's window needs it)..."; Flags: waituntilterminated; Check: NeedsWebView2
#endif
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[Code]
// WebView2 counts as installed when either the machine-wide or the
// per-user EdgeUpdate client reports a real version.
function HasWebView2Version(Root: Integer; Key: String): Boolean;
var
  Version: String;
begin
  Result := RegQueryStringValue(Root, Key, 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0');
end;

function NeedsWebView2(): Boolean;
begin
  Result := not (HasWebView2Version(HKLM, 'SOFTWARE\WOW6432Node\{#WebView2ClientKey}')
    or HasWebView2Version(HKLM, 'SOFTWARE\{#WebView2ClientKey}')
    or HasWebView2Version(HKCU, 'Software\{#WebView2ClientKey}'));
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Keep: Boolean;
  ResultCode: Integer;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;
  // The exe is gone, so a "Start LeGaJ with Windows" entry would be dead.
  RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', '{#MyAppName}');

  // A silent uninstall always keeps the data (the default answer).
  Keep := True;
  if not UninstallSilent() then
    Keep := MsgBox('Keep your applications and documents?' + #13#10 + #13#10 +
      'Yes keeps them in ' + ExpandConstant('{localappdata}\{#MyAppName}') +
      ' for a later reinstall. No deletes them, your saved Gemini key and LeGaJ''s browser data.',
      mbConfirmation, MB_YESNO or MB_DEFBUTTON1) = IDYES;
  if Keep then
    Exit;
  DelTree(ExpandConstant('{localappdata}\{#MyAppName}'), True, True, True);
  // WebView2's data for legaj.exe (Wails' default location).
  DelTree(ExpandConstant('{userappdata}\{#MyAppExeName}'), True, True, True);
  // The Gemini key in Windows Credential Manager (internal/secrets).
  Exec(ExpandConstant('{sys}\cmdkey.exe'), '/delete:LeGaJ:gemini-api-key', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;
