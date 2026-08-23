!macro NSIS_HOOK_PREINSTALL
  ; Release the desktop executable and its sidecar before NSIS overwrites them.
  nsExec::ExecToLog 'taskkill /F /T /IM lite-router-desktop.exe'
  nsExec::ExecToLog 'taskkill /F /T /IM lite-router.exe'
  Sleep 1200
!macroend
