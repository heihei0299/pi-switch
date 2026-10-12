# Management API

`pi-switch webui start|status|stop` is retained as a CLI compatibility name; it manages the HTTP API and does not launch a browser UI. `GET /` returns an HTTP 410 retirement notice, `GET /healthz` is available for probes, and management endpoints remain under `/api/*`.

The API reads its bind host and port from `settings.web` (default `127.0.0.1:43110`). Loopback binds are unauthenticated for local use. A non-loopback bind requires HTTP Basic authentication with username `admin`; the password comes from `PI_SWITCH_WEBUI_PASSWORD` or the mode-0600 password file at `~/.pi-switch/webui_password` (overridable with `PI_SWITCH_WEBUI_PASSWORD_FILE`). Use `--generate-password` to create a password when starting an exposed listener. Startup refuses an exposed listener without a password.

## Security

Keep the API on loopback unless remote access is required. For remote access, use a TLS reverse proxy and a strong password; Basic authentication alone does not encrypt traffic. Health probes remain unauthenticated, while management routes require authentication on non-loopback binds.
