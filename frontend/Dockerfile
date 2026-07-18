FROM node:22-alpine AS deps

WORKDIR /workspace
RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/package.json
COPY packages/radar-agent/package.json ./packages/radar-agent/package.json
COPY packages/radar-api/package.json ./packages/radar-api/package.json
COPY packages/radar-contracts/package.json ./packages/radar-contracts/package.json
COPY mobile/radar/patches/expo-modules-jsi@57.0.3.patch ./mobile/radar/patches/expo-modules-jsi@57.0.3.patch
RUN pnpm install --frozen-lockfile --filter my-project...

FROM node:22-alpine AS builder

WORKDIR /workspace
RUN corepack enable

ARG NEXT_PUBLIC_REVENUECAT_WEB_API_KEY=""
ARG NEXT_PUBLIC_REVENUECAT_OFFERING_ID="default"
ENV NEXT_PUBLIC_REVENUECAT_WEB_API_KEY=$NEXT_PUBLIC_REVENUECAT_WEB_API_KEY \
    NEXT_PUBLIC_REVENUECAT_OFFERING_ID=$NEXT_PUBLIC_REVENUECAT_OFFERING_ID

COPY --from=deps /workspace/node_modules ./node_modules
COPY --from=deps /workspace/package.json ./package.json
COPY --from=deps /workspace/pnpm-lock.yaml ./pnpm-lock.yaml
COPY --from=deps /workspace/pnpm-workspace.yaml ./pnpm-workspace.yaml
COPY --from=deps /workspace/frontend/node_modules ./frontend/node_modules
COPY --from=deps /workspace/packages/radar-agent/node_modules ./packages/radar-agent/node_modules
COPY --from=deps /workspace/packages/radar-api/node_modules ./packages/radar-api/node_modules
COPY --from=deps /workspace/packages/radar-contracts/node_modules ./packages/radar-contracts/node_modules
COPY frontend ./frontend
COPY packages/radar-agent ./packages/radar-agent
COPY packages/radar-api ./packages/radar-api
COPY packages/radar-contracts ./packages/radar-contracts
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm --dir frontend build

FROM node:22-alpine AS runner

WORKDIR /workspace/frontend
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

COPY --from=builder /workspace/node_modules /workspace/node_modules
COPY --from=builder /workspace/frontend/package.json ./package.json
COPY --from=builder /workspace/frontend/next.config.mjs ./next.config.mjs
COPY --from=builder /workspace/frontend/public ./public
COPY --from=builder /workspace/frontend/.next ./.next
COPY --from=builder /workspace/frontend/node_modules ./node_modules
COPY --from=builder /workspace/packages /workspace/packages

USER nextjs

EXPOSE 3000

CMD ["npm", "run", "start"]
