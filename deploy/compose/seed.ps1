# Local compose does not create Supabase users.
# Auth stays on Supabase cloud. This script does not call Supabase.
Write-Host "Auth is still Supabase cloud."
Write-Host "This script does not call Supabase and does not create a user."
Write-Host "The compose gateway boots with local HS256 so /readyz does not fetch a JWKS document."
Write-Host "Sign-in for the React app still uses the Supabase project in frontend/.env. Do not commit that file."
Write-Host "The current UI speaks Socket.IO. The gateway speaks native WebSockets. Protocol cutover is Phase 9."
exit 0
