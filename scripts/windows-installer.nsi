Unicode true
!include "MUI2.nsh"
!ifndef VERSION
  !define VERSION "0.1.0-dev"
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
BrandingText " "
; Short component names: the list is narrow and cuts longer ones off.
; The full explanation shows beside it, under Description.
!define MUI_COMPONENTSPAGE_SMALLDESC
!define MUI_ICON "..\assets\icon.ico"
!define MUI_UNICON "..\assets\icon.ico"
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

Section "Smart Search (required)" Main
  SectionIn RO
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  IfFileExists "$INSTDIR\StremioSmartSearch.exe" 0 +2
    ExecWait '"$INSTDIR\StremioSmartSearch.exe" --quit'
  Sleep 500
  File /oname=StremioSmartSearch.exe "${BINARY}"
  File "..\LICENSE"
  File "..\THIRD-PARTY-NOTICES.txt"
  File "..\DATA.md"
  File "..\START-HERE.txt"
  CreateDirectory "$SMPROGRAMS\Stremio Smart Search"
  CreateShortCut "$SMPROGRAMS\Stremio Smart Search\Stremio + Smart Search.lnk" "$INSTDIR\StremioSmartSearch.exe" "--launch"
  CreateShortCut "$SMPROGRAMS\Stremio Smart Search\Smart Search Settings.lnk" "$INSTDIR\StremioSmartSearch.exe"
  CreateShortCut "$SMPROGRAMS\Stremio Smart Search\Quit Smart Search.lnk" "$INSTDIR\StremioSmartSearch.exe" "--quit"
  CreateShortCut "$DESKTOP\Stremio + Smart Search.lnk" "$INSTDIR\StremioSmartSearch.exe" "--launch"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "DisplayName" "Stremio Smart Search"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "DisplayIcon" '"$INSTDIR\StremioSmartSearch.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "Publisher" "EskimoDev"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch" "UninstallString" '"$INSTDIR\Uninstall.exe"'
SectionEnd

Section /o "Start when I sign in" SignIn
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "StremioSmartSearch" '"$INSTDIR\StremioSmartSearch.exe" --no-browser'
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${Main} "The helper, plus Stremio + Smart Search, Smart Search Settings and Quit Smart Search shortcuts."
  !insertmacro MUI_DESCRIPTION_TEXT ${SignIn} "Start the helper quietly when you sign in to Windows. You can change this later in settings."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  SetShellVarContext current
  ExecWait '"$INSTDIR\StremioSmartSearch.exe" --quit'
  Sleep 500
  Delete "$DESKTOP\Stremio + Smart Search.lnk"
  Delete "$SMPROGRAMS\Stremio Smart Search\Stremio + Smart Search.lnk"
  Delete "$SMPROGRAMS\Stremio Smart Search\Smart Search Settings.lnk"
  Delete "$SMPROGRAMS\Stremio Smart Search\Quit Smart Search.lnk"
  RMDir "$SMPROGRAMS\Stremio Smart Search"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\StremioSmartSearch"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "StremioSmartSearch"
  Delete "$INSTDIR\StremioSmartSearch.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\DATA.md"
  Delete "$INSTDIR\START-HERE.txt"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
