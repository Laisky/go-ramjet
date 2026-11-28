# db . -t ppcelery/go-ramjet:latest
FROM node:24.21.0-bookworm AS nodebuild

WORKDIR /app
ADD . .

# Build the unified SPA UI
RUN corepack enable
RUN pnpm -C web install --frozen-lockfile
# Public browser configuration only; never pass Sentry upload/auth tokens here.
ARG VITE_CHAT_SENTRY_ENABLED=false
ARG VITE_CHAT_SENTRY_DSN
ARG VITE_CHAT_SENTRY_ENVIRONMENT
ARG VITE_CHAT_SENTRY_RELEASE
RUN VITE_CHAT_SENTRY_ENABLED="$VITE_CHAT_SENTRY_ENABLED" \
    VITE_CHAT_SENTRY_DSN="$VITE_CHAT_SENTRY_DSN" \
    VITE_CHAT_SENTRY_ENVIRONMENT="$VITE_CHAT_SENTRY_ENVIRONMENT" \
    VITE_CHAT_SENTRY_RELEASE="$VITE_CHAT_SENTRY_RELEASE" \
    pnpm -C web build

# =====================================

FROM golang:1.27.4-bookworm AS gobuild

# install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends g++ make gcc git \
    build-essential ca-certificates curl \
    && update-ca-certificates

ENV GO111MODULE=on
WORKDIR /goapp

COPY go.mod .
COPY go.sum .
RUN go mod download

# static build
ADD . .
COPY --from=nodebuild /app/web/dist ./web/dist
ENV GOOS=linux
ENV GOARCH=amd64
# Keep source paths: gojieba uses runtime.Caller to locate its dictionaries.
RUN go build -ldflags="-s -w" \
    && CGO_ENABLED=0 go build -o /goapp/crawler-browser-launcher ./cmd/crawler-browser-launcher

# Collect only Jieba runtime data and its license and notice files.
RUN mkdir -p /runtime-assets/gojieba/deps/cppjieba \
    && cd /go/pkg/mod/github.com/yanyiwu/gojieba@v1.4.7 \
    && cp -a deps/cppjieba/dict /runtime-assets/gojieba/deps/cppjieba/ \
    && find . -type f \( -iname '*license*' -o -iname '*copying*' -o -iname '*notice*' \) \
        -exec cp --parents {} /runtime-assets/gojieba/ \;

# =====================================

# copy executable file and certs to a pure container
FROM debian:bookworm

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates haveged wget curl gnupg \
    # --------------------------------------------
    # for google-chrome
    # --------------------------------------------
    # libappindicator1 fonts-liberation xdg-utils wget \
    # libasound2 libatk-bridge2.0-0 libatspi2.0-0 libcurl3-gnutls libcurl3-nss \
    # libcurl4 libcurl3 libdrm2 libgbm1 libgtk-3-0 libgtk-4-1 libnspr4 libnss3 \
    # libu2f-udev libvulkan1 libxkbcommon0 \
    # --------------------------------------------
    # general
    # --------------------------------------------
    && (update-ca-certificates 2>/dev/null || true) \
    && rm -rf /var/lib/apt/lists/*

# install google-chrome from the official Google APT repository
RUN mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://dl.google.com/linux/linux_signing_key.pub \
    | gpg --dearmor -o /etc/apt/keyrings/google-chrome.gpg \
    && chmod a+r /etc/apt/keyrings/google-chrome.gpg \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
    > /etc/apt/sources.list.d/google-chrome.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends google-chrome-stable \
    && rm -rf /var/lib/apt/lists/*

# install CJK + emoji + general Unicode fonts so headless Chrome can render
# non-Latin scripts (Chinese / Japanese / Korean) in generated PDFs. Without
# fonts-noto-cjk Chrome falls back to tofu/replacement glyphs for any CJK
# character regardless of the page's @font-face declarations.
RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-noto-cjk fonts-noto-color-emoji fonts-noto-core \
    && rm -rf /var/lib/apt/lists/*

# install pg_dump from PGDG
# Use the Debian bookworm codename directly to avoid needing lsb_release
RUN curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    | gpg --dearmor -o /etc/apt/keyrings/postgresql.gpg \
    && chmod a+r /etc/apt/keyrings/postgresql.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/postgresql.gpg] http://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" \
    > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends postgresql-client-17 \
    && rm -rf /var/lib/apt/lists/*

# The runtime needs OpenSSL shared libraries, not headers or static archives.
RUN apt-get update && apt-get install -y \
    libssl3 ca-certificates wget \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app

COPY --from=gobuild /etc/ssl/certs /etc/ssl/certs
# Preserve the default runtime.Caller dictionary path and third-party notices.
COPY --from=gobuild /runtime-assets/gojieba /go/pkg/mod/github.com/yanyiwu/gojieba@v1.4.7
COPY --from=gobuild /goapp/go-ramjet /app/go-ramjet
COPY --from=gobuild /goapp/crawler-browser-launcher /usr/local/bin/crawler-browser-launcher
COPY --from=gobuild /goapp/web/dist /app/web/dist

ENTRYPOINT ["/app/go-ramjet"]
