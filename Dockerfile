FROM caddy:2.11.2-builder AS builder
# Don't change to caddy:2-builder to get automated updates
ADD . ./
RUN xcaddy build \
    --with github.com/SinTan1729/caddy-wikijs-metatags=.

FROM caddy

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

