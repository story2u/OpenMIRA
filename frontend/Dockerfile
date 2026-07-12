FROM node:22-alpine AS deps

WORKDIR /app
RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:22-alpine AS builder

WORKDIR /app
RUN corepack enable

ARG NEXT_PUBLIC_REVENUECAT_WEB_API_KEY=""
ARG NEXT_PUBLIC_REVENUECAT_OFFERING_ID="default"
ENV NEXT_PUBLIC_REVENUECAT_WEB_API_KEY=$NEXT_PUBLIC_REVENUECAT_WEB_API_KEY \
    NEXT_PUBLIC_REVENUECAT_OFFERING_ID=$NEXT_PUBLIC_REVENUECAT_OFFERING_ID

COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM node:22-alpine AS runner

WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/next.config.mjs ./next.config.mjs
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules

USER nextjs

EXPOSE 3000

CMD ["npm", "run", "start"]
