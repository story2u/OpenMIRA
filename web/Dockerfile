# syntax=docker/dockerfile:1

ARG NODE_VERSION=24

FROM node:${NODE_VERSION}-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:${NODE_VERSION}-alpine AS build
WORKDIR /app
ARG GO_WEB_VERSION=0.1.0-slim
ARG GO_WEB_COMMIT=unknown
ARG GO_WEB_BUILD_TIME=unknown
ENV NEXT_PUBLIC_GO_WEB_VERSION=${GO_WEB_VERSION}
ENV NEXT_PUBLIC_GO_WEB_COMMIT=${GO_WEB_COMMIT}
ENV NEXT_PUBLIC_GO_WEB_BUILD_TIME=${GO_WEB_BUILD_TIME}
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:${NODE_VERSION}-alpine AS runner
WORKDIR /app
ARG GO_WEB_VERSION=0.1.0-slim
ARG GO_WEB_COMMIT=unknown
ARG GO_WEB_BUILD_TIME=unknown
ENV NODE_ENV=production
ENV NEXT_PUBLIC_GO_WEB_VERSION=${GO_WEB_VERSION}
ENV NEXT_PUBLIC_GO_WEB_COMMIT=${GO_WEB_COMMIT}
ENV NEXT_PUBLIC_GO_WEB_BUILD_TIME=${GO_WEB_BUILD_TIME}
COPY --from=build /app/package.json /app/package-lock.json ./
COPY --from=build /app/next.config.mjs ./next.config.mjs
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/.next ./.next
EXPOSE 3000
CMD ["npm", "run", "start"]
