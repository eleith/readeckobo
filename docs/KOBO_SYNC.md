# Kobo ebook sync alongside Readeck articles

Use a per-device readeckobo API endpoint to sync Readeck articles alongside
books from a Kobo-compatible service (like [Komga](https://komga.org/docs/guides/kobo/)).

Readeckobo can forward Kobo API requests to the selected user's ebook service so
you can use both readeckobo for articles and your ebook library service of
choice.

## Configure the ebook service

For each Kobo device, set `book_sync_url` to that user's full **Kobo API
endpoint** on the ebook service, including any authentication path. For
example, with Komga:

```yaml
users:
  - token: "<READECKOBO-PLAIN-DEVICE-TOKEN>"
    readeck_access_token: "<READECK-API-TOKEN>"
    book_sync_url: "https://komga.example.com/kobo/<KOMGA_API_KEY>"
```

Reload readeckobo after changing `config.yaml`

## Deploy readeckobo

[Cloudflare Tunnel](CLOUDFLARE.md) forwards `/booksync/` to readeckobo with its
normal app route. With [Nginx](NGINX.md#kobo-ebook-syncing), add the
`/booksync/` location to readeckobo's public HTTPS server. The plain device
token in the route selects the matching user's `book_sync_url`; readeckobo
forwards the remaining path and query to that service. The same public hostname
also serves article requests.

Check the initialization response before changing the Kobo:

```text
https://readeckobo.example.com/booksync/<READECKOBO-PLAIN-DEVICE-TOKEN>/v1/initialization
```

Its `instapaper_env_url` should point to readeckobo while its
`image_host` and image templates should point to your ebook library service.

## Configure the Kobo

In `.kobo/Kobo/Kobo eReader.conf`, set the per-device API endpoint:

```ini
[OneStoreServices]
api_endpoint=https://readeckobo.example.com/booksync/<READECKOBO-PLAIN-DEVICE-TOKEN>
```
