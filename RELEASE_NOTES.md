# Release Notes

- Fix invite acceptance redirect — anonymous visitors landing on `/auth/invite?token=...` no longer get force-logged-out to `/login` before they can choose between Sign in and Create account. The session probe now uses a raw `fetch` so a 401 means "anonymous", not "expired session".
- CI: cut Docker release build time roughly in half. The frontend stage is now pinned to `BUILDPLATFORM` so `npm ci` and `vite build` run natively instead of being emulated for arm64 (the static output is platform-independent anyway), and proxy/processing images now build on parallel runners with per-image cache scopes.
- CI: extracted the frontend bundle into a single shared job and let the four binary matrix shards download the artifact instead of rebuilding it each time.
