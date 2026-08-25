!macro NSIS_HOOK_PREINSTALL
  ; Release the desktop executable and its sidecar before NSIS overwrites them.
  nsExec::ExecToLog 'taskkill /F /T /IM mkrouter-desktop.exe'
  nsExec::ExecToLog 'taskkill /F /T /IM mkrouter-backend.exe'
  nsExec::ExecToLog 'taskkill /F /T /IM mkrouter-core.exe'
  nsExec::ExecToLog 'taskkill /F /T /IM mkrouter.exe'
  nsExec::ExecToLog 'taskkill /F /T /IM mkswitch-desktop.exe'
  nsExec::ExecToLog 'taskkill /F /T /IM mkswitch.exe'
  Sleep 1200
!macroend
