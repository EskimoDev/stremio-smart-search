Unicode true
!include "MUI2.nsh"
!ifndef VERSION
  !define VERSION "0.1.0-preview.2"
!endif
!ifndef ARCH
  !define ARCH "x64"
!endif
!ifndef BINARY
  !error "Pass /DBINARY=the compiled executable"
!endif
!ifndef OUTPUT
  !define OUTPUT "..\dist\StremioSmartSearch-${VERSION}-Windows-${ARCH}-Setup.exe"
!endif
Name "Stremio Smart Search"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\StremioSmartSearch"
RequestExecutionLevel user
SetCompressor /SOLID lzma
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\StremioSmartSearch.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open Smart Search setup"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Smart Search and Stremio launcher" Main
  SectionIn RO
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  IfFileExists "$INSTDIR\StremioSmartSearch.exe" 0 +2
    ExecWait '"$INSTDIR\StremioSmartSearch.exe" --quit'
  Sleep 500
  File /oname=StremioSmartSearch.exe "${BINARY}"
  File "..\LICENSE"
  File "..\DATA.md"
  File "..\START-HERE.txt"
  CreateDirectory "$SMPROGRAMS\Stremio Smart Search"
  CreateShortCut "$SMPROGRAMS\Stremio Smart Search\Stremio + Smart Search.lnk" "$INSTDIR\StremioSmartSearch.exe" "--launch"
  CreateShortCut "$SMPROGRAMS\Stremio Smart Search\Smart Search Settings.lnk" "$INSTDIR\StremioSmartSearch.exe"
  CreateShortCut "$DESKTOP\Stremio + Smart Search.lnk" "$INSTDIR\StremioSmartSearch.exe" "--launch"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "DisplayName" "Stremio Smart Search"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "Publisher" "EskimoDev"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "UninstallString" '"$INSTDIR\Uninstall.exe"'
SectionEnd

Section /o "Start helper when I sign in"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "StremioSmartSearch" '"$INSTDIR\StremioSmartSearch.exe" --no-browser'
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  ExecWait '"$INSTDIR\StremioSmartSearch.exe" --quit'
  Sleep 500
  Delete "$DESKTOP\Stremio + Smart Search.lnk"
  Delete "$SMPROGRAMS\Stremio Smart Search\Stremio + Smart Search.lnk"
  Delete "$SMPROGRAMS\Stremio Smart Search\Smart Search Settings.lnk"
  RMDir "$SMPROGRAMS\Stremio Smart Search"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "StremioSmartSearch"
  Delete "$INSTDIR\StremioSmartSearch.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\DATA.md"
  Delete "$INSTDIR\START-HERE.txt"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
