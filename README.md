# EventsProject — Setup Guide

A full-stack events booking platform.

- **Backend:** Go + Gin + PostgreSQL + Redis (`events-db/`)
- **Mobile App:** React Native + Expo + NativeWind (`EventsApp/`)

> All commands are written for **PowerShell** on Windows.

---

## Prerequisites

Install these before anything else.

| Tool | Version | Download |
|------|---------|----------|
| Go | 1.25+ | https://go.dev/dl |
| Node.js | 18+ | https://nodejs.org |
| npm | 9+ | bundled with Node |
| PostgreSQL | 15+ | https://www.postgresql.org/download |
| Redis | 7+ | https://redis.io/download |
| golang-migrate CLI | latest | https://github.com/golang-migrate/migrate |
| Expo Go (phone) | latest | App Store / Google Play |

---

## Backend Setup (`events-db/`)

### 1. Navigate to the backend

```powershell
cd events-db
```

### 2. Install Go dependencies

```powershell
go mod download
```

All dependencies are declared in `go.mod`. The key direct dependencies are:

| Package | Purpose |
|---------|---------|
| `github.com/gin-gonic/gin` | HTTP router |
| `github.com/gin-contrib/cors` | CORS middleware |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/redis/go-redis/v9` | Redis client |
| `github.com/golang-jwt/jwt/v5` | JWT auth |
| `github.com/rs/zerolog` | Structured logging |
| `github.com/joho/godotenv` | `.env` file loading |
| `github.com/google/uuid` | UUID generation |
| `github.com/go-playground/validator/v10` | Request validation |
| `github.com/sendgrid/sendgrid-go` | Email (SendGrid) |
| `github.com/svix/svix-webhooks` | Svix webhook verification |
| `github.com/skip2/go-qrcode` | QR code generation |
| `github.com/aws/aws-sdk-go-v2` | Cloudflare R2 storage |
| `github.com/elastic/go-elasticsearch/v8` | Elasticsearch (search) |
| `golang.org/x/crypto` | bcrypt password hashing |
| `github.com/stretchr/testify` | Test assertions |

### 3. Install Air (live reload for development)

`air` watches your Go files and automatically restarts the server on changes so you don't have to stop and re-run manually. Install it once globally:

```powershell
go install github.com/air-verse/air@latest
```

### 4. Create the database

```powershell
psql -U postgres -c "CREATE DATABASE events_db;"
```

### 5. Run migrations

```powershell
migrate -path ./migrations -database "postgresql://postgres:yourpassword@localhost:5432/events_db?sslmode=disable" up
```

This runs all 9 migrations in order:

| Migration | What it creates |
|-----------|----------------|
| `000001` | Enums and extensions (uuid-ossp, pg_trgm) |
| `000002` | `users` table |
| `000003` | `categories` and `organizers` tables |
| `000004` | `events` table |
| `000005` | `bookings` table |
| `000006` | `payments` and supporting tables |
| `000007` | Stitch payment fields on `payments` |
| `000008` | Guest bookings table |
| `000009` | `refresh_tokens` table |

### 6. Create `.env` file

Create `events-db/.env` with the following variables:

```env
# Server
PORT=8080
ENV=development

# Database
DATABASE_URL=postgresql://postgres:yourpassword@localhost:5432/events_db

# Redis
REDIS_ADDR=localhost:6379

# Auth
JWT_SECRET=your-secret-key-must-be-at-least-32-characters

# Frontend (used for CORS and email links)
FRONTEND_URL=http://localhost:3000

# SendGrid (email)
SENDGRID_API_KEY=SG.your-api-key
EMAIL_FROM_ADDRESS=noreply@yourdomain.com
EMAIL_FROM_NAME=EventsApp

# QR Code encryption
QR_ENCRYPTION_KEY=your-32-byte-hex-key

# Stitch payments (South African payment gateway)
STITCH_CLIENT_ID=your-stitch-client-id
STITCH_CLIENT_SECRET=your-stitch-client-secret
STITCH_WEBHOOK_SECRET=whsec_your-webhook-secret
STITCH_ENVIRONMENT=sandbox

# Cloudflare R2 (file storage)
R2_ACCOUNT_ID=your-account-id
R2_ACCESS_KEY=your-access-key
R2_SECRET_KEY=your-secret-key
R2_BUCKET=your-bucket-name
R2_URL=https://your-account.r2.cloudflarestorage.com

# Feature flags
ENABLE_EMAIL_VERIFICATION=false
ENABLE_WAITLIST=false
```

### 7. Start Redis

```powershell
# If installed as a Windows service
net start Redis

# If running the Redis executable directly
redis-server
```

### 8. Run the server

**With Air (recommended — auto-restarts on file changes):**

```powershell
air
```

**Without Air (one-off run):**

```powershell
go run cmd/api/main.go
```

The server starts on `http://localhost:8080`. Check it's healthy:

```powershell
Invoke-RestMethod http://localhost:8080/health
```

Expected response:
```json
{"postgres":"ok","redis":"ok","status":"ok"}
```

---

### Running Tests

The tests require a separate test database running on port **5433**. Start it with Docker:

```powershell
docker run -d --name test-postgres `
  -e POSTGRES_USER=testuser `
  -e POSTGRES_PASSWORD=testpass `
  -e POSTGRES_DB=test_db `
  -p 5433:5432 postgres:15
```

Then run all tests:

```powershell
go test ./...
```

Run only the payment/webhook tests:

```powershell
go test ./internal/services/... -v -run "TestWebhook|TestPayment"
```

If testify is missing (it's in `go.mod` already, but if not fetched):

```powershell
go get github.com/stretchr/testify@latest
```

---

## Mobile App Setup (`EventsApp/`)

### 1. Navigate to the app directory

```powershell
cd EventsApp
```

### 2. Create the `.npmrc` file to avoid peer dependency conflicts

```powershell
"legacy-peer-deps=true" | Out-File -FilePath .npmrc -Encoding utf8
```

### 3. Install dependencies

```powershell
npm install --legacy-peer-deps
```

### 4. Install Expo packages

These must use `npx expo install` (not plain `npm install`) so Expo picks compatible versions:

```powershell
npx expo install expo-router react-native-safe-area-context react-native-screens expo-linking expo-constants expo-status-bar
npx expo install expo-secure-store
npx expo install expo-web-browser expo-auth-session
npx expo install expo-camera
npx expo install expo-image
npx expo install axios zustand
```

### 5. Install dev dependencies

```powershell
npm install --save-dev tailwindcss
```

### 6. Installed packages summary

| Package | Version | Purpose | Install method |
|---------|---------|---------|----------------|
| `expo` | ~55.0.5 | Core Expo SDK | included |
| `expo-router` | ~55.0.4 | File-based navigation | `npx expo install` |
| `expo-secure-store` | ~55.0.8 | Encrypted token storage | `npx expo install` |
| `expo-camera` | ~55.0.9 | QR code scanning | `npx expo install` |
| `expo-image` | ~55.0.6 | Optimised image component | `npx expo install` |
| `expo-constants` | ~55.0.7 | App config access | `npx expo install` |
| `expo-linking` | ~55.0.7 | Deep linking | `npx expo install` |
| `expo-status-bar` | ~55.0.4 | Status bar control | `npx expo install` |
| `expo-web-browser` | ~55.0.9 | OAuth browser | `npx expo install` |
| `expo-auth-session` | ~55.0.7 | OAuth session handling | `npx expo install` |
| `react-native` | 0.83.2 | Core RN framework | included |
| `react-native-screens` | ~4.23.0 | Native screen containers | `npx expo install` |
| `react-native-safe-area-context` | ~5.6.2 | Safe area insets | `npx expo install` |
| `nativewind` | ^4.2.2 | Tailwind for React Native | `npm install` |
| `axios` | ^1.13.6 | HTTP client | `npx expo install` |
| `zustand` | ^5.0.11 | State management | `npx expo install` |
| `tailwindcss` | ^3.3.2 | CSS utility classes (dev) | `npm install --save-dev` |
| `typescript` | ~5.9.2 | TypeScript (dev) | included |
| `@types/react` | ~19.2.2 | React types (dev) | included |

### 7. Update the API base URL

Open `lib/api.ts` and replace the IP address with your machine's local IP:

```typescript
export const API_BASE_URL = 'http://YOUR_LOCAL_IP:8080';
```

Find your local IP in PowerShell:

```powershell
ipconfig
```

Look for **IPv4 Address** under your active network adapter (e.g. `192.168.1.5`).

> Do not use `localhost` — the phone/emulator cannot reach your machine's localhost. You must use the actual LAN IP.

### 8. Config files

These files should already exist in the repo. If any are missing, create them:

**`babel.config.js`** — required for NativeWind:
```javascript
module.exports = function (api) {
  api.cache(true);
  return {
    presets: [
      ['babel-preset-expo', { jsxImportSource: 'nativewind' }],
      'nativewind/babel',
    ],
  };
};
```

**`metro.config.js`** — required for NativeWind:
```javascript
const { getDefaultConfig } = require('expo/metro-config');
const { withNativeWind } = require('nativewind/metro');

const config = getDefaultConfig(__dirname);

module.exports = withNativeWind(config, { input: './global.css' });
```

**`global.css`** — Tailwind directives:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

### 9. Start the app

```powershell
npx expo start
```

Scan the QR code with the **Expo Go** app on your phone, or press:
- `a` — open Android emulator
- `i` — open iOS simulator (macOS only)

---

## Project Structure

```
EventsProject/
├── events-db/                  # Go backend
│   ├── cmd/api/main.go         # Entry point
│   ├── internal/
│   │   ├── config/             # Env config loading
│   │   ├── handlers/           # HTTP handlers
│   │   ├── services/           # Business logic
│   │   ├── repository/         # Database queries
│   │   ├── middleware/         # Auth, rate limit, CORS, logging
│   │   ├── models/             # Data structs
│   │   ├── ratelimit/          # Redis token bucket
│   │   ├── workers/            # Booking expiry background job
│   │   └── database/           # Postgres connection
│   ├── migrations/             # 9 SQL migration files
│   └── go.mod
│
└── EventsApp/                  # React Native app
    ├── app/
    │   └── _layout.tsx         # Root layout (expo-router)
    ├── lib/
    │   └── api.ts              # Axios client + interceptors
    ├── store/
    │   └── authStore.ts        # Zustand auth state
    ├── assets/                 # Icons and images
    ├── global.css              # Tailwind entry point
    ├── babel.config.js
    ├── metro.config.js
    ├── tailwind.config.js
    └── package.json
```

---

## Common Issues

**`npm install` peer dependency errors**
Always use `npx expo install` for Expo packages, and make sure `.npmrc` has `legacy-peer-deps=true`.

**App can't connect to the API**
Make sure `API_BASE_URL` in `lib/api.ts` uses your machine's LAN IP from `ipconfig`, not `localhost`.

**`migrate: no migration`**
Make sure you're running `migrate` from inside the `events-db/` directory and the `-path` flag points to `./migrations`.

**Redis connection refused**
Start Redis before running the server. The server will still start but rate limiting will be disabled.

**`JWT_SECRET must be at least 32 characters`**
Your `.env` JWT_SECRET is too short. Use a random 32+ character string.

**`air: command not found`**
Run `go install github.com/air-verse/air@latest` and make sure your Go bin directory is on your PATH:
```powershell
$env:PATH += ";$env:GOPATH\bin"
```
To make this permanent, add it to your PowerShell profile (`$PROFILE`).
