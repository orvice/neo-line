# syntax=docker/dockerfile:1

ARG NODE_VERSION=22
ARG NGINX_VERSION=1.27

FROM node:${NODE_VERSION}-alpine AS build

WORKDIR /src

RUN corepack enable

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

FROM nginx:${NGINX_VERSION}-alpine

COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Backend upstream for the /v1 and /ping reverse proxy. Override at runtime.
ENV BACKEND_URL=http://neo-line:8080

EXPOSE 80
