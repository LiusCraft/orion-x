# Orion X Manager Web UI

Manager admin console skeleton built with React + MUI.

## What is included

- Login page (`/login`) with manager auth API integration
- Registration page (`/register`) integrated with `POST /api/v1/auth/register`
- Token lifecycle handling (restore from local storage, auto refresh, manual refresh)
- Protected routes and unified auth failure redirect
- Role-based navigation (`admin` vs `normal_user`)
- Platform resources page (`/platform-resources`) with filters + create/edit/disable flow
- Provider-driven resource form templates (fields vary by `category` + `provider`)
- Dedicated required resource fields for `base_url` + `access_key` (not in provider template)
- Provider catalog panel with visual field-schema editor (supports nested path fields, add/edit/delete templates via backend APIs)
- Access key re-auth reveal flow (`POST /api/v1/admin/platform-resources/:id/access-key/reveal`)
- Basic layout for remaining manager pages (`tool market`, `voicebots/devices`)
- Legacy voice chat page available at `/voice-chat`

## Run locally

```bash
npm install
npm run dev
```

Dev mode uses Vite proxy by default:

- `/api` -> `http://127.0.0.1:8081`
- `/internal` -> `http://127.0.0.1:8081`

Optional environment variables:

```bash
VITE_MANAGER_API_BASE_URL=
VITE_MANAGER_API_PROXY_TARGET=http://127.0.0.1:8081
```

- `VITE_MANAGER_API_BASE_URL`: explicit API base URL (if set, requests bypass relative dev base).
- `VITE_MANAGER_API_PROXY_TARGET`: dev proxy target URL.

## Project structure

```text
src/
  api/         # HTTP client and API error model
  auth/        # session model, storage, auth provider and guards
  layout/      # app shell and role-based navigation
  pages/       # login + placeholder pages
  components/  # reusable page-level blocks
```
