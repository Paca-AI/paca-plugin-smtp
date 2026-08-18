# com.paca.smtp

First-party Paca plugin that sends transactional email over an
admin-configured SMTP server: new-account invites, password-reset links, and
task-assignment/@mention notifications. It's the first of what's expected to
be a family of mail plugins (SES, SendGrid, Mailgun, …) — the app core has no
built-in notion of "email" at all; it only publishes generic domain events to
a plugin event stream, and this plugin is the one that happens to turn some
of them into email.

## What it does

- **Admin settings page** (`Email (SMTP)`, under Admin) — host/port/username/
  password/from-address form, a live "send test email" button, and a
  per-event toggle grid for which optional notification types get emailed.
  Connection settings save via an explicit button; event toggles save
  immediately on change.
- **Per-user preferences tab** (`Email Notifications`, on the profile page)
  — lets each user opt out of whichever optional event types the admin has
  enabled, for their own account.
- **Two mandatory emails**, sent regardless of the admin's event toggles,
  whenever SMTP is configured at all:
  - Welcome / set-your-password, when an account is created with an email
    on file.
  - Password reset, when an admin resets a user's password.
- **Four optional, togglable/opt-outable emails**: assigned to a task,
  mentioned in a comment, mentioned in a document, mentioned in a task
  description. All four default to enabled.
- Every email is rendered as an HTML+plain-text pair matching the web app's
  light-mode design tokens (fonts, colors, spacing), and reflects the
  workspace's custom brand name/logo/primary color live at send time if the
  admin has set them — never a value baked in earlier.

## Architecture

```
smtp/
├── backend/   — Go WASM plugin (runs inside the API host)
└── frontend/  — React micro-frontend (Module Federation remote)
```

### Backend (`backend/`)

- Written in Go, compiled to `wasip1/wasm` for production (`main.go` carries
  a `//go:build wasip1` tag, so a plain `go build ./...` without
  `GOOS=wasip1 GOARCH=wasm` fails with "function main is undeclared in the
  main package" — expected, not a real error; `go vet`/`go test` work fine
  without the cross-compile target).
- Registered as `com.paca.smtp` in the plugin registry.
- Owns its own schema (`plugin_data_com_paca_smtp`): a singleton
  `smtp_config` row (server settings + which optional events are enabled)
  and a `user_email_preferences` row per user who has opted out of anything
  (see `backend/migrations/0001_create_smtp_tables.sql`).
- `smtp_config.password_enc` is AES-256-GCM encrypted at rest
  (`backend/crypto.go`, keyed by the `ENCRYPTION_KEY` host config value) and
  declared in `plugin.json`'s `sensitiveFields` so other plugins can't read
  it back out via `db_query` against this plugin's schema.
- Event-driven (`backend/events.go`, `handleEvent`): subscribes to six
  topics on the host's plugin event stream (`ctx.On(topic, ...)`) and, for
  each one that fires, checks whether SMTP is actually configured, whether
  the topic is enabled (mandatory topics skip this check), and — for
  optional topics — whether the recipient has personally opted out, before
  rendering and sending.
- Two outbound-only host capabilities this plugin declares permission for
  and calls through private `go:wasmimport` bindings (`backend/mail_wasm.go`,
  `backend/password_token_wasm.go`) — see "Host permissions" below.

### Frontend (`frontend/`)

- Vite + React + TanStack Query, Module Federation remote exposing two entry
  points declared in `plugin.json`:
  - `./AdminSmtpSettingsPage` — `admin.page` extension point, reached
    through an admin-sidebar nav item this plugin registers.
  - `./EmailPreferencesTab` — `user.settings.tab` extension point, mounted
    as an additional card on the host's own profile page.
- `smtp-api.ts` — thin typed wrapper around `PluginApiClient` plus
  `OPTIONAL_EVENT_OPTIONS`, the four togglable topics' labels/descriptions
  shown in both the admin grid and the user preferences tab.

## Event topics

| Topic | Mandatory? | Payload |
|---|---|---|
| `user.created` | yes | `user_id`, `username`, `full_name`, `email?` |
| `user.password_reset` | yes | `user_id`, `username`, `full_name`, `email?` |
| `notification.assigned` | no (default on) | `recipient_user_id`, `recipient_email?`, `recipient_name`, `actor_name`, `link_url` |
| `notification.mentioned` | no (default on) | same shape as `notification.assigned` |
| `notification.doc_mentioned` | no (default on) | same shape as `notification.assigned` |
| `notification.task_description_mentioned` | no (default on) | same shape as `notification.assigned` |

None of these payloads ever carry a password or a password-set token — see
"Host permissions" below for how this plugin actually gets one. A
`recipient_email`/`email` field that's empty in the payload means the
account has no email on file; this plugin's `handleEvent` treats that as
"nothing to do" and no-ops rather than erroring.

## Host permissions

Declared in `plugin.json`'s `permissions` array — each one gates a specific
host function this plugin calls:

- **`email.send`** — dials the admin-configured SMTP server directly,
  unsandboxed, on this plugin's behalf (real SMTP needs a raw TCP/TLS
  socket, which WASI doesn't expose to guests). See
  `services/api/internal/platform/plugin/email.go`.
- **`users.password_set_token.issue`** — mints a single-use password-set
  token scoped to one `user_id`. The core app deliberately never embeds this
  token in `user.created`/`user.password_reset`'s event payload — a
  password-set token is a bearer credential, and the core has no idea which
  (if any) subscriber actually needs one, so it isn't handed to every
  plugin that merely subscribes to the topic. Instead, once this plugin has
  already decided (SMTP configured, mandatory topic) that it's actually
  going to deliver a link, it calls back into the host on demand
  (`backend/password_token_wasm.go`) and gets a token minted just for that
  request. See `services/api/internal/platform/plugin/password_token.go`.
  The resulting link is only redeemable while the account is in the
  must-change-password state (i.e. it hasn't already set a real password by
  some other means) — see `usersvc.Service.SetPasswordWithToken`.

`db.read`/`db.write` (this plugin's own schema) and `events.subscribe` are
the two other declared permissions, needed for the config/preferences
tables and the event topics above respectively.

## API endpoints

All routes are relative to `/api/v1/plugins/com.paca.smtp` and require an
authenticated, fresh-password session (`optionalAuthn` + `requireFreshPassword`).

| Method | Path | Permission | Description |
|---|---|---|---|
| `GET` | `/admin/config` | global `users.write` | Current SMTP settings (password never round-tripped) |
| `PATCH` | `/admin/config` | global `users.write` | Update settings; blank `password` keeps the existing one |
| `POST` | `/admin/test-email` | global `users.write` | Send a sample email to `{"to": "..."}` using the saved config |
| `GET` | `/me/preferences` | global `users.read` | The caller's available (admin-enabled) events and which they've disabled |
| `PATCH` | `/me/preferences` | global `users.read` | Update `{"disabled_events": [...]}` for the caller |

## Database schema

Tables live in the `plugin_data_com_paca_smtp` schema, created by
`backend/migrations/0001_create_smtp_tables.sql`:

```
smtp_config            (singleton row, id = TRUE)
  host            TEXT
  port            INT      DEFAULT 587
  username        TEXT
  password_enc    TEXT     -- AES-256-GCM, hex(nonce || ciphertext+tag)
  from_address    TEXT
  from_name       TEXT
  use_tls         BOOLEAN  DEFAULT TRUE
  enabled_events  JSONB    -- optional topics the admin has enabled
  updated_at      TIMESTAMPTZ

user_email_preferences
  user_id          UUID PRIMARY KEY
  disabled_events  JSONB DEFAULT '[]'
  updated_at       TIMESTAMPTZ
```

## Development

### Backend

```bash
cd backend

# Vet / test (native — no wasip1 target needed for these)
go vet ./...
go test ./...

# Build the WASM binary
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o backend.wasm .
```

Requires `ENCRYPTION_KEY` (a 32-byte hex string, same value the host also
uses elsewhere) and `PUBLIC_URL` allowlisted via `plugin.json`'s
`allowedConfigKeys` — both must be set on the host for this plugin to
encrypt the stored password and build set-password links.

### Frontend

```bash
cd frontend

npm install
npm run typecheck
npm run build   # outputs remoteEntry.js
```

Uses `@paca-ai/plugin-sdk-react`. Shared singletons (`react`, `react-dom`,
`@tanstack/react-query`) are provided by the host shell and must not be
bundled.

## Extension points

### `admin.page` — `AdminSmtpSettingsPage`

Routed via the `Email (SMTP)` admin-sidebar nav item declared in
`plugin.json`. Two independent sections: SMTP server settings (explicit Save
button, disabled while the form is unchanged) and notification events
(each toggle saves immediately).

### `user.settings.tab` — `EmailPreferencesTab`

Mounted as an additional card on the host's own profile page via the
`user.settings.tab` extension point. Lists whichever optional events the
admin has enabled; hidden entirely (with an explanatory message) if the
admin hasn't enabled any.
