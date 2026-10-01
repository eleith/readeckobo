# Nginx

Use the [README quick start](../README.md) for a basic `config.yaml` and the
Kobo's Instapaper settings.

## Kobo article syncing

Add these locations to the HTTPS `server` block for **readeckobo's public
hostname** (the Instapaper article address, and the `api_endpoint` address when
using ebook sync).

The example assumes Nginx can reach `readeckobo` at `127.0.0.1:8080`; use your
app's address if different.

```nginx
# The bare Kobo Store endpoint.
location = /instapaper-proxy/storeapi {
    proxy_ssl_server_name on;
    proxy_ssl_name storeapi.kobo.com;
    proxy_set_header Host storeapi.kobo.com;
    proxy_pass https://storeapi.kobo.com/;
}

# Forward Store subpaths with a single leading slash upstream.
location /instapaper-proxy/storeapi/ {
    proxy_ssl_server_name on;
    proxy_ssl_name storeapi.kobo.com;
    proxy_set_header Host storeapi.kobo.com;
    proxy_pass https://storeapi.kobo.com/;
}

# Fetch Kobo Store initialization for the Instapaper URL rewrite.
location = /instapaper-proxy/storeapi/v1/initialization {
    proxy_ssl_server_name on;
    proxy_ssl_name storeapi.kobo.com;
    proxy_set_header Host storeapi.kobo.com;
    proxy_set_header Accept-Encoding "";
    proxy_set_header If-None-Match "";
    proxy_set_header If-Modified-Since "";
    proxy_set_header Range "";
    proxy_set_header If-Range "";
    proxy_pass https://storeapi.kobo.com/v1/initialization;

    sub_filter https://www.instapaper.com "$scheme://$host/instapaper-proxy/instapaper";
    sub_filter_types application/json;
}

# Instapaper article requests go to readeckobo.
location /instapaper-proxy/instapaper/ {
    proxy_pass http://127.0.0.1:8080/;
    proxy_set_header Host $host;
    client_max_body_size 128M;

    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Prefix /instapaper-proxy/instapaper;
}
```

Keep port 8080 private: `docker-compose.yml` exposes it on all interfaces by
default. Bind it to `127.0.0.1:8080:8080` if Nginx runs on the host, or use a
private network if it runs elsewhere.

## Kobo ebook syncing

For [Kobo ebook sync](KOBO_SYNC.md), add this location to readeckobo's public
HTTPS `server` block. The `proxy_pass` forwards the full
`/booksync/<device-token>/...` path, which selects the user's ebook
service.

```nginx
location /booksync/ {
    access_log off; # Request paths contain a device token.
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```
