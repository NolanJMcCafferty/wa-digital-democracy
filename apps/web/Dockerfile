# syntax=docker/dockerfile:1

FROM node:24-alpine AS deps
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Local-dev target used by infra/docker-compose.yml. Runs `next dev` so
# NODE_ENV stays "development" and admin pages skip the Clerk sign-in
# flow via the local-dev bypass in src/lib/adminAuth.ts and src/proxy.ts.
# Defined before `build`/`web` so the production stage remains the final
# (default) target — Railway builds this Dockerfile without specifying a
# target and uses the last stage.
FROM node:24-alpine AS web-dev
WORKDIR /app
ENV NODE_ENV=development
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
RUN corepack enable
COPY --from=deps /app/node_modules ./node_modules
COPY . .
EXPOSE 3000
CMD ["sh", "-c", "./node_modules/.bin/next dev -H 0.0.0.0 -p ${PORT:-3000}"]

FROM node:24-alpine AS build
WORKDIR /app
ENV SKIP_BUILD_STATIC_PARAMS=1
RUN corepack enable
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm build

FROM node:24-alpine AS web
WORKDIR /app
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
RUN corepack enable
COPY --from=build /app ./
EXPOSE 3000
CMD ["sh", "-c", "./node_modules/.bin/next start -H 0.0.0.0 -p ${PORT:-3000}"]
