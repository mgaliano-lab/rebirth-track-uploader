; Instalador de Rebirth Track Uploader (NSIS 3, Unicode). Por usuario: sin permisos de admin.
; makensis -DVERSION=1.2.3 -DSRC=<carpeta con RebirthTrackUploader.exe> -DOUT=<instalador.exe> rebirthtrack.nsi
; Si existe la variable SIGN_CMD, firma el instalador y el desinstalador (Authenticode).
Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef SRC
  !define SRC "../dist"
!endif
!ifndef OUT
  !define OUT "RebirthTrackUploader-Setup.exe"
!endif
!define APP "Rebirth Track Uploader"
!define EXE "RebirthTrackUploader.exe"
!define PUBLISHER "Rebirth Knights"
!define REGKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\RebirthTrackUploader"

; Versión numérica para los metadatos del .exe (sin sufijos tipo -dev.abc1234).
!ifndef NUMVER
  !define NUMVER "${VERSION}"
!endif
!searchparse /noerrors "${NUMVER}" "" VMAJ "." VMIN "." VPAT
!ifndef VMAJ
  !define VMAJ 0
!endif
!ifndef VMIN
  !define VMIN 0
!endif
!ifndef VPAT
  !define VPAT 0
!endif

Name "${APP}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\RebirthTrack"
InstallDirRegKey HKCU "${REGKEY}" "InstallLocation"
BrandingText "${APP} ${VERSION} · rebirthknights.fun"

VIProductVersion "${VMAJ}.${VMIN}.${VPAT}.0"
VIAddVersionKey "ProductName" "${APP}"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "FileDescription" "Instalador de ${APP}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "© Rebirth Knights"

!ifdef SIGN_CMD
  !finalize '${SIGN_CMD} "%1"' = 0
  !uninstfinalize '${SIGN_CMD} "%1"' = 0
!endif

!include "MUI2.nsh"
!include "LogicLib.nsh"
!define MUI_ICON "../assets/icon.ico"
!define MUI_UNICON "../assets/icon.ico"
!define MUI_ABORTWARNING

!define MUI_WELCOMEPAGE_TITLE "Rebirth Track Uploader ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "La app de la guild Rebirth Knights que instala el addon Rebirth Track y sube a rebirthknights.fun los registros de combate de tus bandas de World of Warcraft.$\r$\n$\r$\nEn el próximo paso te explicamos exactamente qué hace en tu PC y qué sube. No necesita permisos de administrador."
!insertmacro MUI_PAGE_WELCOME

!define MUI_LICENSEPAGE_TEXT_TOP "Leé qué hace la app en tu PC y qué sube a internet:"
!define MUI_LICENSEPAGE_TEXT_BOTTOM "Si estás de acuerdo, elegí «Acepto». Igual vas a poder pausar, revisar y borrar lo subido cuando quieras."
!define MUI_LICENSEPAGE_BUTTON "&Acepto"
!insertmacro MUI_PAGE_LICENSE "que-hace.txt"

!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Abrir Rebirth Track Uploader"
!define MUI_FINISHPAGE_LINK "rebirthknights.fun"
!define MUI_FINISHPAGE_LINK_LOCATION "https://rebirthknights.fun"
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"

Section "Rebirth Track Uploader (requerido)" SecApp
  SectionIn RO
  ; Si estaba abierta, se cierra para poder reemplazar el ejecutable. En una actualización
  ; automática (/S) la app ya se está cerrando sola: se le da un momento para terminar el lote.
  ${If} ${Silent}
    Sleep 3000
  ${EndIf}
  nsExec::Exec 'taskkill /IM "${EXE}" /F'
  Sleep 500
  SetOutPath "$INSTDIR"
  File "${SRC}/${EXE}"
  File "/oname=que-hace.txt" "que-hace.txt"
  WriteUninstaller "$INSTDIR\Desinstalar.exe"
  CreateDirectory "$SMPROGRAMS\Rebirth Track"
  CreateShortcut "$SMPROGRAMS\Rebirth Track\${APP}.lnk" "$INSTDIR\${EXE}"
  CreateShortcut "$SMPROGRAMS\Rebirth Track\Qué hace y qué sube.lnk" "$INSTDIR\que-hace.txt"
  CreateShortcut "$SMPROGRAMS\Rebirth Track\Desinstalar.lnk" "$INSTDIR\Desinstalar.exe"
  WriteRegStr HKCU "${REGKEY}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${REGKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${REGKEY}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKCU "${REGKEY}" "URLInfoAbout" "https://rebirthknights.fun"
  WriteRegStr HKCU "${REGKEY}" "DisplayIcon" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "${REGKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${REGKEY}" "UninstallString" '"$INSTDIR\Desinstalar.exe"'
  WriteRegDWORD HKCU "${REGKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${REGKEY}" "NoRepair" 1
SectionEnd

Section "Iniciar con Windows (minimizada en la bandeja)" SecAutostart
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "RebirthTrackUploader" '"$INSTDIR\${EXE}" --minimized'
SectionEnd

Section "Acceso directo en el escritorio" SecDesktop
  CreateShortcut "$DESKTOP\${APP}.lnk" "$INSTDIR\${EXE}"
SectionEnd

; Actualización automática (la app corre el instalador nuevo con /S): se mantiene lo que el
; usuario ya tenía (inicio con Windows y acceso directo) y al terminar se vuelve a abrir la app.
Function .onInit
  ${If} ${Silent}
    ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "RebirthTrackUploader"
    ${If} $0 == ""
      SectionSetFlags ${SecAutostart} 0
    ${EndIf}
    ${IfNot} ${FileExists} "$DESKTOP\${APP}.lnk"
      SectionSetFlags ${SecDesktop} 0
    ${EndIf}
  ${EndIf}
FunctionEnd

; El panel se muestra en una ventana propia con WebView2 (el motor de Edge; viene con Windows
; 10/11). Si falta, la app igual anda (abre el panel en el navegador) y acá se ofrece instalarlo
; con el instalador oficial de Microsoft.
!define WEBVIEW2_KEY "Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
Function HasWebView2
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\${WEBVIEW2_KEY}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKLM "SOFTWARE\${WEBVIEW2_KEY}" "pv"
  ${EndIf}
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "Software\${WEBVIEW2_KEY}" "pv"
  ${EndIf}
FunctionEnd

Function .onInstSuccess
  ${If} ${Silent}
    Exec '"$INSTDIR\${EXE}" --minimized'
    Return
  ${EndIf}
  Call HasWebView2
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    MessageBox MB_YESNO|MB_ICONINFORMATION "Para ver el panel en su propia ventana hace falta Microsoft Edge WebView2, que a esta PC le falta.$\r$\n$\r$\nLa app funciona igual (el panel se abre en el navegador). ¿Descargar el instalador oficial de Microsoft ahora?" IDNO webview2_skip
      ExecShell "open" "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
    webview2_skip:
  ${EndIf}
FunctionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecApp} "La app (bandeja del sistema + panel local)."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecAutostart} "Que arranque sola al prender la PC, sin abrir ventanas. Se puede cambiar después desde el panel."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Ícono en el escritorio."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "un.Rebirth Track Uploader" UnSecApp
  SectionIn RO
  nsExec::Exec 'taskkill /IM "${EXE}" /F'
  Sleep 500
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "RebirthTrackUploader"
  Delete "$DESKTOP\${APP}.lnk"
  RMDir /r "$SMPROGRAMS\Rebirth Track"
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\que-hace.txt"
  Delete "$INSTDIR\Desinstalar.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "${REGKEY}"
SectionEnd

Section /o "un.Borrar también la configuración (vinculación y progreso)" UnSecData
  RMDir /r "$APPDATA\RebirthTrack"
SectionEnd
