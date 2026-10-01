# readeckobo

This tool acts as an Instapaper proxy, so your Kobo can sync with your
[Readeck](https://readeck.com) articles.

This started as a Go port of [kobeck](https://github.com/Lukas0907/kobeck), and
then evolved to support multiple users, logging, performance improvements, ebook
syncing pass through and more.

## ✨ Features

* 🗞️ Syncs non-archived articles from Readeck to Kobo
* 📑  Downloads article content and image for each bookmark
* 📋️ Supports archiving, re-adding, favoriting, and deleting
* 📷️ Converts images to JPEG format for e-reader compatibility
* 👥 Supports multiple Kobo devices and readeck accounts
* 📚 Supports passthrough ebook library syncing (ex: Komga)

## 🚀 Quick Start (for Users)

### 1. Choose how you will deploy

Choose a public HTTPS hostname (for example, `readeckobo.example.com`) and one
deployment method:

* [Cloudflare Tunnel](docs/CLOUDFLARE.md): forward requests to `readeckobo`.
* [Nginx](docs/NGINX.md): proxy Kobo Store and article requests.

### 2. Generate a device token

Copy `config.yaml.example` to `config.yaml`, then build the image. Find your
Kobo's serial number under **Settings -> Device Information**. Replace
`<YOUR_KOBO_SERIAL>` below with that number (without angle brackets):

```sh
docker-compose build
docker-compose run --rm --entrypoint /app/bin/generate-encrypted-token.sh readeckobo <YOUR_KOBO_SERIAL>
```

This will generate two strings:

* a plaintext token for a user in your `config.yaml`
* an encrypted token for Kobo

### 3. Configure `readeckobo` and Kobo

Edit `config.yaml` with your Readeck host and API token and the plaintext token
from step 2:

```yaml
server:
  port: 8080
log_level: info
readeck:
  host: "https://readeck.example.com"
users:
  - token: "<THE-PLAIN-TEXT-TOKEN-FROM-THE-SCRIPT>"
    readeck_access_token: "a-readeck-api-token"
```

For more config options, see [docs/CONFIG.md](docs/CONFIG.md). To sync ebooks in
addition to readeck articles (using an ebook library provider like Komga), see
the [Kobo ebook sync guide](docs/KOBO_SYNC.md).

Now, mount your Kobo and edit `.kobo/Kobo/Kobo eReader.conf`, using the encrypted
token. Replace `readeckobo.example.com` with the hostname you
chose in step 1:

```ini
[OneStoreServices]
api_endpoint=https://readeckobo.example.com/instapaper-proxy/storeapi
instapaper_env_url=https://readeckobo.example.com

[Instapaper]
AccessToken=@ByteArray(<THE-ENCRYPTED-TOKEN-FROM-THE-SCRIPT>)
```

### 4. Run the app

```sh
docker-compose up -d
```

The app is now available locally at `http://localhost:8080`

## 🔒 A Quick Word on Security

A little security goes a long way.

* **Use HTTPS:** follow either deployment guide above, and keep the app's HTTP
  port private.
* **Protect tokens:** don't share config files or request URLs containing device
  tokens. Browser login or bot challenges may prevent Kobo sync.
* **Kobo Password:** prevent unauthorized mounting with a Kobo password.

## 🧑‍💻 For Developers

### Building and Running Locally

```sh
# Build the docker image
docker-compose build

# Run the server
docker-compose up
```

The server will be available at `http://localhost:8080`.

### API Endpoints

`readeckobo` emulates the Instapaper API for Kobo devices. Here's a quick overview:

<!-- markdownlint-disable MD013 -->
| Endpoint                   | Description |
| -------------------------- | ----------- |
| `POST /api/kobo/get`       | syncs non-archived articles from Readeck. |
| `POST /api/kobo/download` | downloads the content of an article for offline reading. |
| `POST /api/kobo/send`     | handles archiving, favoriting, deleting, or adding new articles. |
| `GET /api/convert-image`  | a helper endpoint to convert all article images to JPEG |
| `GET /booksync`  | an optional proxy passthrough to a user's `book_sync_url`  |
<!-- markdownlint-enable MD013 -->

### Testing

The `scripts/e2e-tests/` directory has simple shell scripts for testing each API
endpoint. They're great for checking if everything is working as expected.

```sh
# Run the 'get' test
./scripts/e2e-tests/01-test-get.sh <YOUR_DEVICE_TOKEN>
```

### Makefile Targets

The `Makefile` has some handy targets:

* `make build`: Build the application binary.
* `make test`: Run all unit tests.
* `make lint`: Run the linter.
* `make vendor`: Vendor all dependencies.
* `make ci`: Run all CI checks (linting and testing).
