# events-admin/Dockerfile
# Production build for the admin dashboard (Next.js, standalone output).
# Multi-stage so the final image ships only the compiled app + node_modules
# it actually needs — small and fast to start.
#
# NEXT_PUBLIC_API_URL is baked in AT BUILD TIME (Next inlines NEXT_PUBLIC_*
# vars into the client bundle), so it's passed as a build arg, not a runtime
# env. Rebuild if the API URL changes.

# ── deps ──────────────────────────────────────────────────────────────────────
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci

# ── build ─────────────────────────────────────────────────────────────────────
FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Public API base URL, inlined into the client bundle at build time.
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}

# Google Places key for the venue search on the event form. Needs BOTH halves
# of the pattern: the ARG to receive it from compose's build.args, and the ENV
# so it is present in the environment `npm run build` runs in. With either one
# missing Next inlines an empty string, the component's key check throws, and
# the form shows "Address search is not configured" — which is a build-time
# failure that looks exactly like a runtime one.
ARG NEXT_PUBLIC_GOOGLE_MAPS_KEY
ENV NEXT_PUBLIC_GOOGLE_MAPS_KEY=${NEXT_PUBLIC_GOOGLE_MAPS_KEY}

RUN npm run build

# ── runtime ───────────────────────────────────────────────────────────────────
FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
# Next listens on 3000 by default; we keep that and map it in compose.
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# Non-root for safety.
RUN addgroup --system --gid 1001 nodejs && adduser --system --uid 1001 nextjs

# Standalone output: a self-contained server + only the needed node_modules.
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]