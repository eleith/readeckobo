# readeckobo

`readeckobo` bridges Kobo's Instapaper app and a [Readeck](https://readeck.com)
service, enabling you to use your Readeck account on your Kobo e-reader.

It acts as a proxy for Instapaper's Kobo API, translating requests and responses.
as a proxy for Instapaper's Kobo API, translating requests and responses.

This project is a Go-based port of the original Python-based
[kobeck](https://github.com/Lukas0907/kobeck), with significant deviations to
support multiple users, improve logging, and leverage newer Readeck APIs.

## ✨ Features

* 📚 Fetches and lists un-archived articles from Readeck for Kobo devices.
* 📥 Downloads article content in a Kobo-compatible format.
* ✅ Handles actions like archiving, re-adding, favoriting, and deleting
articles by updating Readeck.
* 🖼️ Converts images to JPEG format on the fly for better e-reader
compatibility.
* 🤝 Supports multiple Kobo devices with different tokens.

## ⚙️ Configuration (For Users)

The application is configured using a `config.yaml` file. An example is provided
in `config.yaml.example`:

```yaml
server:
  port: 8080
log_level: info
readeck:
  host: "https://your-readeck-instance.com"
users:
  - token: "a-random-uuid-token-for-a-kobo"
    readeck_access_token: "a-readeck-api-token"
  - token: "another-random-uuid-token-for-another-kobo"
    readeck_access_token: "another-readeck-api-token"
```

### Generating a Device Token

User tokens are just random UUID. this allows the proxy to support multiple kobo
devices that want to connect to different readeck accounts.

you can use the `generate-token`
script located in the `bin` directory:

```sh
docker-compose build
docker-compose up
docker-compose exec readeckobo bin/generate-token
```

### Configure Kobo

Mount your kobo and edit the `./kobo/Kobo/Kobo eReader.conf` file.

The file has a large list of settings, look for the following settings and
update them or add them if they don't already exist

```toml
[OneStoreServices]
api_endpoint=https://readeckobo.example.com/storeapi
instapaper_env_url=https://readeckobo.example.com/instapaper

[Instapaper]
AccessToken=@ByteArray(<GENERATED-DEVICE-TOKEN>)
```

Replace `readeckobo.example.com` with the hostname of your proxy instance.

## 🚀 Deployment / Reverse Proxy Setup

`readeckobo` must be run behind a reverse proxy.

checkout the nginx snippet at `nginx.conf.snippet` for how to configure the
proxy. there are no examples yet for caddy, traefik, etc etc.

## 🔒 Security Considerations

Please be aware that using this integration requires some attention to keep your
readeck and kobo secure.

* **HTTPS is strongly recommended.** Always run `readeckobo` behind a reverse
proxy that provides HTTPS.
* **Network Exposure:** For the best security, do not expose the `readeckobo`
server to the public internet. Keep it on your local network
* **Device Security:** Anyone with physical access to your Kobo device can
potentially access your Readeck account by mounting it an extracting the proxy
tokens. It is recommended to set a password on your Kobo device to prevent
unauthorized mounting.
* **Token Revocation:** If you lose your device, you
should immediately remove the corresponding user entry from your `config.yaml`
and restart the `readeckobo` server.

### Building and Running Locally
when you are ready to deploy, setup your reverse proxy, get your
docker-compose.yml file in place and then run!

```sh
 docker-compose build
 docker-compose up -d
```

The server will be available at `http://localhost:8080`.

## 🧑‍💻 Development (For Developers)

### API Endpoints

<!-- markdownlint-disable MD013 -->
| Endpoint                   | Description |
| -------------------------- | ----------- |
| `POST /api/kobo/get`       | **Fetches the list of articles.** The Kobo device calls this to sync the list of new, updated, and deleted articles. Under the hood, `readeckobo` calls Readeck's `GET /api/bookmarks/sync` to get a list of changes and then `POST /api/bookmarks/sync` to fetch detailed metadata for the updated articles. It filters out archived articles before returning the list to the device. |
| `POST /api/kobo/download` | **Downloads article content.** The Kobo device calls this with an article URL to get its content for offline reading. Under the hood, `readeckobo` searches for the bookmark by its URL (by calling Readeck's `GET /api/bookmarks`) and then fetches the article content (by calling `GET /api/bookmarks/{id}/article`). It also processes images for Kobo compatibility. |
| `POST /api/kobo/send`     | **Sends updates to Readeck.** The Kobo device calls this to report actions like archiving, favoriting, deleting, or adding an article. Under the hood, `readeckobo` translates these actions into the appropriate Readeck API calls (`PATCH /api/bookmarks/{id}` for updates or `POST /api/bookmarks` for additions). |
| `GET /api/convert-image`  | **Converts images for display.** This is an internal helper endpoint used by the downloaded article content to convert images to a Kobo-friendly JPEG format on the fly. |
<!-- markdownlint-enable MD013 -->

### Testing with E2E Scripts

The `scripts/e2e-tests/` directory contains simple shell scripts to test each
API endpoint. These are useful for verifying that your `readeckobo` setup and
connection to Readeck are working correctly.

Each script requires parameters, such as your device token. Run a script without
arguments to see its usage instructions.

Example:

```sh
# Make sure the scripts are executable
chmod +x scripts/e2e-tests/*.sh

# Run the 'get' test
./scripts/e2e-tests/01-test-get.sh <YOUR_DEVICE_TOKEN>
```

### Using the Makefile

The `Makefile` provides several useful targets:

* `make build`: Build the application binary.
* `make test`: Run all unit tests.
* `make lint`: Run the linter.
* `make vendor`: Vendor all dependencies.
* `make ci`: Run all CI checks (linting and testing).
