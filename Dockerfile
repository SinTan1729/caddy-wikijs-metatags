FROM caddy:2.10.2-builder AS builder
ADD . ./
RUN xcaddy build \
    --with github.com/SinTan1729/caddy-wikijs-metatags=.

FROM caddy

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

