# Caddy `wikijs_metatags` handler module

This Caddy module automatically inserts `og:description` and `og:image` meta tags to WikiJS pages, hence the name `wikijs_metatags`.

It only performs replacements if the tags are empty, and uses the default image if none can be found in page. If there are images on the page,
the first one is used in the `og:image` meta tag. For description, only default one is used.

**Note:** This handler cannot perform replacements on compressed content. If your response comes from a proxied backend that supports compression,
you will either have to decompress it in a response handler chain before this handler runs, or disable from the backend. One easy way to ask the
backend to _not_ compress the response is to set the `Accept-Encoding` header to `identity`, for example: `header_up Accept-Encoding identity`
(in your Caddyfile, in the `reverse_proxy` block).

**Module name:** `http.handlers.wikijs_metatags`

## Usage

Since this is a very niche plugin, I haven't bothered submitting it to the Caddy plugin registry. You can, however, use it directly in your
docker compose file by referring to the image `ghcr.io/sintan1729/caddy-wikijs-metatags:latest` or `:0` for sticking to the current version.
The versioned releases are locked at the state when a version is released. The `latest` tag is built monthly, and whenever a new version of
caddy is released.

## JSON examples

Substring substitution:

```json
{
  "handler": "wikijs_metatags",
  "default_description": "Foo",
  "default_image_path": "/Bar.jpg",
  "insert_topic": false,
  "topic_regex": "\/([^\/]+)\/[^\/]+$"
}
```

## Caddyfile

This module has Caddyfile support. It registers the `wikijs_metatags` directive, by default to be after the standard `encode` directive.
Make sure to change it with the [order](https://caddyserver.com/docs/caddyfile/directives#directive-order) global option in case that is not
suitable for your needs.

Syntax:

```caddyfile
wikijs_metatags [<matcher>] {
	default_description <description>
	default_image_path <path>
	[insert_topic]
	[topic_regex <regex>]
}
```

- Here, `default_description` is a string, which will become the description if the page has none.
- The `default_image_path` can relative to the host, in which case hostname will be added automatically. It can also be a complete URL.
  In both cases, it must link to a `.jpg`, `.png`, `.gif`, or `.webp` image.
- If `insert_topic` is present, an attempt will be made to insert a topic after the description.
- If `insert_topic` is present, `topic_regex` will be used to extract the topic. It must match the URL being processed. The first capturing group
  will be used to get the topic. The above `json` example shows the regex that I use. My wiki page links look like
  `https://<hostname>/<language>/<topic>/<pagename>`, and the regex captures `<topic>` from it. Remember that the `https://` prefix is not
  part of the matched URL string.

## Limitation

Compressed responses (e.g. from an upstream proxy which gzipped the response body) will not be decoded before attempting to replace. To work around
this, you may send the `Accept-Encoding: identity` request header to the upstream to tell it not to compress the response. For example:

```caddyfile
      reverse_proxy localhost:8080 {
          header_up Accept-Encoding identity
      }
```

## Acknowledgement

Much of the code has shamelessly been borrowed from https://github.com/caddyserver/replace-response.
