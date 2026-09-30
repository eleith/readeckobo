# Cloudflare Tunnel

Use the [README quick start](../README.md) for `config.yaml` and the Kobo's Instapaper settings. This guide publishes that same setup over HTTPS without Nginx.

## Publish the app

Before starting `readeckobo`, add your public origin to `config.yaml` (alongside
`server`, `readeck`, and `users`):

```yaml
public_url: "https://readeckobo.example.com"
```

If you are changing an existing deployment, recreate the Docker service to
reload the bind-mounted config file (`docker-compose up -d --force-recreate
readeckobo`), or restart your non-Docker process.

Before making the hostname public, add a [Cloudflare WAF custom rule](https://developers.cloudflare.com/waf/custom-rules/) with **Block** action for `(http.host eq "readeckobo.example.com" and http.request.uri.path eq "/api/convert-image")`. This direct, unauthenticated endpoint fetches a caller-provided URL and is not needed by the prefixed Kobo article routes. The Tunnel can still forward all other paths without path-specific routing rules.

Add a [published application route](https://developers.cloudflare.com/cloudflare-one/networks/routes/add-routes/) to a connected Cloudflare Tunnel:

| Route setting | Example |
| :--- | :--- |
| Public hostname | `readeckobo.example.com` |
| Service URL | `http://localhost:8080` |

Use the actual address `cloudflared` can reach: `localhost` works when it runs on the same host as the published app port, not when it runs in a separate container. Forward **all paths** to the app without a path rule. Go proxies Kobo Store traffic and rewrites initialization so Kobo continues sending Instapaper requests to `readeckobo`.

## Verify and secure

Check that initialization points back to your hostname:

```sh
curl --compressed -fsS https://readeckobo.example.com/instapaper-proxy/storeapi/v1/initialization \
  | grep -F 'https://readeckobo.example.com/instapaper-proxy/instapaper'
```

Then check sync on a real Kobo. Browser login or bot challenges at the edge may block its requests. `docker-compose.yml` publishes port 8080 on all interfaces: restrict direct HTTP access with a firewall, or bind the port to `127.0.0.1:8080:8080` if `cloudflared` runs on the host. If it runs in another container or host, use a private network instead. Keep device tokens and request URLs containing them private.

For Kobo ebook syncing with a compatible service, see [Kobo ebook sync](KOBO_SYNC.md). The same Tunnel route forwards those requests to readeckobo.
