Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
# Per-user install: %LOCALAPPDATA%\Programs, the uninstall entry under HKCU and the
# current user's shortcuts, so neither installing nor the in-app update asks for an
# administrator. The two have to move together: wails_tools.nsh picks the install
# directory and the registry hive from WAILS_INSTALL_SCOPE but the shell context
# and the manifest's execution level from REQUEST_EXECUTION_LEVEL, and either one
# left at its admin default makes an unelevated installer write to Program Files or
# HKLM and fail.
!define WAILS_INSTALL_SCOPE "user"
!define REQUEST_EXECUTION_LEVEL "user"
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

# /UPDATE is passed only by the app's own updater (internal/updater/install_windows.go),
# together with /S and /D=<the running install>. It makes the installer wait for the
# app to exit before writing over it and start the new version afterwards; a silent
# install without it (CI's own check, a scripted install) behaves as before.
Var IsUpdate

Function .onInit
   !insertmacro wails.checkArchitecture
   ${GetParameters} $R0
   ClearErrors
   ${GetOptions} $R0 "/UPDATE" $R1
   ${IfNot} ${Errors}
       StrCpy $IsUpdate "1"
   ${EndIf}
FunctionEnd

# The app launches the installer as the last thing it does before exiting, so the exe
# can still be held open for a moment. A running exe cannot be opened for writing;
# retry until it can, for up to 30 seconds, after which File reports the failure.
Function WaitForAppExit
    StrCpy $R2 0
    ${DoWhile} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        ClearErrors
        FileOpen $R3 "$INSTDIR\${PRODUCT_EXECUTABLE}" a
        ${IfNot} ${Errors}
            FileClose $R3
            ${Break}
        ${EndIf}
        IntOp $R2 $R2 + 1
        ${If} $R2 >= 60
            ${Break}
        ${EndIf}
        Sleep 500
    ${Loop}
FunctionEnd

Function .onInstSuccess
    ${If} $IsUpdate == "1"
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
    ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    ${If} $IsUpdate == "1"
        Call WaitForAppExit
    ${EndIf}

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # TinySegmenter's modified BSD asks for its notice to accompany a binary
    # redistribution. Handing out this installer on its own is such a redistribution,
    # and the copies CI leaves beside the exe only reach someone who takes the whole
    # artifact zip, so the notices ride in the payload and land next to the app.
    File "..\..\..\LICENSE"
    File "..\..\..\THIRD_PARTY_NOTICES.md"

    # The built-in embedding stack lands beside the exe, where the app looks for it
    # (internal/embed/sidecar_windows.go): llama-server.exe and its DLLs, and the model
    # GGUF that the app seeds into %AppData% on first launch. `pnpm sidecar` stages them
    # before `wails build` into places `-clean` leaves alone. They are absent in a plain
    # checkout, so /nonfatal keeps a local `wails build -nsis` working, at the cost of an
    # installer without built-in embedding; CI installs the result and asserts they are
    # there (.github/workflows/build.yml).
    File /nonfatal "..\..\sidecar\windows-${ARCH}\llama-server.exe"
    File /nonfatal "..\..\sidecar\windows-${ARCH}\*.dll"
    File /nonfatal "..\..\sidecar\.downloads\ruri-v3-30m-q8_0.gguf"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
