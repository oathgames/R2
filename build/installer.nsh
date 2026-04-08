!macro customInstall
  ; Launch app after install — guaranteed to fire (unlike runAfterFinish)
  ${StdUtils.ExecShellAsUser} $0 "$INSTDIR\${APP_EXECUTABLE_FILENAME}" "open" ""
!macroend
