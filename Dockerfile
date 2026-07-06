FROM caddy:2.11.4-builder-alpine AS builder
# Don't change to caddy:2-builder to get automated updates
ADD . ./
RUN xcaddy build \
    --with github.com/SinTan1729/caddy-wikijs-metatags=.

FROM caddy:2.11.4-alpine
COPY --from=builder /usr/bin/caddy /usr/bin/caddy

