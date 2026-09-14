# Multi-stage Dockerfile for the optimus-fe SPA.
# Build context is this frontend repository root.

FROM oven/bun:1.3 AS build
WORKDIR /src
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY . ./
RUN bun run build

FROM nginx:1.27-alpine
RUN apk add --no-cache wget
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/templates/default.conf.template
ENV BACKEND_URL=http://optimus-be:8080
EXPOSE 80
