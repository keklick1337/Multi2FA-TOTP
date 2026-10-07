; Multi2FA TOTP installer for Windows. Built by packaging/docker/build-windows.sh:
;   makensis -DVERSION=1.0.0 -DBUILD=1 -DARCH=amd64|arm64 -DSRC=<app dir> -DROOT=<repo> -DOUTFILE=<setup.exe> installer.nsi
; Installs per machine into Program Files with Start menu entry, uninstaller and optional
; desktop shortcut, file association and multi2fa-cli on PATH.

Unicode true
SetCompressor /SOLID lzma

!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"

!define NAME "Multi2FA TOTP"
!define EXE "Multi2FA-TOTP.exe"
!define PUBLISHER "keklick1337"
!define URL "https://github.com/keklick1337/Multi2FA-TOTP"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Multi2FA-TOTP"

Name "${NAME}"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\${NAME}"
InstallDirRegKey HKLM "${UNINST_KEY}" "InstallLocation"
RequestExecutionLevel admin
BrandingText "${NAME} ${VERSION}"

VIProductVersion "${VERSION}.${BUILD}"
VIAddVersionKey "ProductName" "${NAME}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileVersion" "${VERSION}.${BUILD}"
VIAddVersionKey "FileDescription" "${NAME} installer"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "LegalCopyright" "Copyright (c) 2026 ${PUBLISHER}"

!define MUI_ICON "${ROOT}\assets\icon.ico"
!define MUI_UNICON "${ROOT}\assets\icon.ico"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_SMALLDESC
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_LINK "${URL}"
!define MUI_FINISHPAGE_LINK_LOCATION "${URL}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${ROOT}\LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_LANGUAGE "Ukrainian"
!insertmacro MUI_LANGUAGE "German"
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "French"

Function .onInit
    ${If} "${ARCH}" == "arm64"
        ${IfNot} ${IsNativeARM64}
            MessageBox MB_ICONSTOP "This installer is for Windows on ARM. Please download the amd64 (x64) version."
            Abort
        ${EndIf}
    ${ElseIfNot} ${RunningX64}
        MessageBox MB_ICONSTOP "${NAME} needs 64-bit Windows."
        Abort
    ${EndIf}
    SetRegView 64
    SetShellVarContext all
    !insertmacro MUI_LANGDLL_DISPLAY
FunctionEnd

Function un.onInit
    SetRegView 64
    SetShellVarContext all
FunctionEnd

Section "!${NAME}" SecApp
    SectionIn RO
    SetOutPath "$INSTDIR"
    File "${SRC}\${EXE}"
    File "${SRC}\multi2fa-cli.exe"
    File "${SRC}\icon.ico"
    File "${SRC}\README.txt"
    File "${ROOT}\LICENSE"

    CreateShortCut "$SMPROGRAMS\${NAME}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0
    WriteUninstaller "$INSTDIR\Uninstall.exe"

    WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "${NAME}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
    WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "${PUBLISHER}"
    WriteRegStr HKLM "${UNINST_KEY}" "URLInfoAbout" "${URL}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${EXE},0"
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
    WriteRegStr HKLM "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
    WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
    SectionGetSize ${SecApp} $0
    WriteRegDWORD HKLM "${UNINST_KEY}" "EstimatedSize" $0
SectionEnd

Section "Desktop shortcut" SecDesktop
    CreateShortCut "$DESKTOP\${NAME}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0
SectionEnd

Section "Open .m2fa and .m2fab files with ${NAME}" SecAssoc
    WriteRegStr HKLM "Software\Classes\.m2fa" "" "Multi2FA.Vault"
    WriteRegStr HKLM "Software\Classes\Multi2FA.Vault" "" "Multi2FA TOTP vault"
    WriteRegStr HKLM "Software\Classes\Multi2FA.Vault\DefaultIcon" "" "$INSTDIR\${EXE},0"
    WriteRegStr HKLM "Software\Classes\Multi2FA.Vault\shell\open\command" "" '"$INSTDIR\${EXE}" "%1"'
    WriteRegStr HKLM "Software\Classes\.m2fab" "" "Multi2FA.Backup"
    WriteRegStr HKLM "Software\Classes\Multi2FA.Backup" "" "Multi2FA TOTP backup"
    WriteRegStr HKLM "Software\Classes\Multi2FA.Backup\DefaultIcon" "" "$INSTDIR\${EXE},0"
    WriteRegStr HKLM "Software\Classes\Multi2FA.Backup\shell\open\command" "" '"$INSTDIR\${EXE}" "%1"'
    WriteRegDWORD HKLM "${UNINST_KEY}" "FileAssociation" 1
    System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
SectionEnd

; PowerShell edits PATH: NSIS strings are limited to 1024 characters and could truncate it.
Section "Add multi2fa-cli to PATH" SecPath
    nsExec::ExecToLog `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$p = [Environment]::GetEnvironmentVariable('Path', 'Machine'); if (($$p -split ';') -notcontains '$INSTDIR') { [Environment]::SetEnvironmentVariable('Path', ($$p.TrimEnd(';') + ';$INSTDIR'), 'Machine') }"`
    Pop $0
    WriteRegDWORD HKLM "${UNINST_KEY}" "AddedToPath" 1
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecApp} "The app and the multi2fa-cli command line tool."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Create a shortcut on the desktop."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecAssoc} "Double-click vaults and backups to open them."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecPath} "Run multi2fa-cli from any terminal."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
    ReadRegDWORD $0 HKLM "${UNINST_KEY}" "AddedToPath"
    ${If} $0 == 1
        nsExec::ExecToLog `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$p = [Environment]::GetEnvironmentVariable('Path', 'Machine'); $$n = ($$p -split ';' | Where-Object { $$_ -and $$_ -ne '$INSTDIR' }) -join ';'; [Environment]::SetEnvironmentVariable('Path', $$n, 'Machine')"`
        Pop $0
    ${EndIf}
    ReadRegDWORD $0 HKLM "${UNINST_KEY}" "FileAssociation"
    ${If} $0 == 1
        DeleteRegKey HKLM "Software\Classes\.m2fa"
        DeleteRegKey HKLM "Software\Classes\.m2fab"
        DeleteRegKey HKLM "Software\Classes\Multi2FA.Vault"
        DeleteRegKey HKLM "Software\Classes\Multi2FA.Backup"
        System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
    ${EndIf}

    Delete "$SMPROGRAMS\${NAME}.lnk"
    Delete "$DESKTOP\${NAME}.lnk"
    Delete "$INSTDIR\${EXE}"
    Delete "$INSTDIR\multi2fa-cli.exe"
    Delete "$INSTDIR\icon.ico"
    Delete "$INSTDIR\README.txt"
    Delete "$INSTDIR\LICENSE"
    Delete "$INSTDIR\Uninstall.exe"
    RMDir "$INSTDIR"
    DeleteRegKey HKLM "${UNINST_KEY}"
    ; Vaults in %AppData%\Multi2FA are left alone on purpose.
SectionEnd
