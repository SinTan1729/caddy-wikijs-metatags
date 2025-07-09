FROM caddy:2.10.0-builder AS builder
ADD . ./
RUN xcaddy build \
    --with github.com/SinTan1729/caddy-wikijs-meta-tags=.

FROM caddy:2.10.0

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

