# readeckobo

This tool acts as an Instapaper proxy, so your Kobo can sync with your
[Readeck](https://readeck.com) articles.

This is a Go port of the original [kobeck](https://github.com/Lukas0907/kobeck),
evolved to support multiple users, logging, performance improvements and more.

## ✨ Features

* 📚 Syncs non-archived articles from Readeck to Kobo
* 📰 Downloads article content and image for each bookmark
* 📋️ supports archiving, re-adding, favoriting, and deleting
* 📷️ Converts images to JPEG format for e-reader compatibility
* 👥 Supports multiple Kobo devices and readeck accounts

## 🚀 Quick Start (for Users)

Getting up and running is a breeze with Docker.

### 1. Configure `readeckobo`

First, copy `config.yaml.example` to `config.yaml` and edit it to match your setup.

```yaml
server:
  port: 8080
log_level: info
readeck:
  host: "https://your-readeck-instance.com"
users:
  - token: "a-random-uuid-token-for-a-kobo"
    readeck_access_token: "a-readeck-api-token"
```

### 2. Run with Docker

Once your configuration is ready, fire it up!

```sh
docker-compose build
docker-compose up -d
```

The server will be available at `http://localhost:8080`.

### 3. Generate a Device Token

For each Kobo device, you will need a unique token. This process involves
generating a token and then encrypting it for the Kobo device.

First, find your Kobo's serial number, which is available under
**Settings -> Device Information** on your e-reader.

With `readeckobo` running, use this command to generate and encrypt the token,
replacing `<YOUR_KOBO_SERIAL>` with your device's serial number:

```sh
docker-compose exec readeckobo bin/generate-encrypted-token.sh <YOUR_KOBO_SERIAL>
```

The script will output two important pieces of information:

1. A **plain text UUID token** to be used in your `config.yaml`.
2. An **encrypted token** to be used in your Kobo's configuration file.

### 4. Configure Your `readeckobo` and Kobo Device

Follow the output from the script to configure your services.

1. **Update `config.yaml`**: Add the plain text UUID token to the `users`
    section of your `config.yaml`.

    ```yaml
    users:
      - token: "<THE-PLAIN-TEXT-UUID-FROM-THE-SCRIPT>"
        readeck_access_token: "a-readeck-api-token"
    ```

2. **Update Your Kobo**: Mount your Kobo and find the
    `.kobo/Kobo/Kobo eReader.conf` file. Add or update these settings using the
    **encrypted** token from the script's output.

    ```ini
    [OneStoreServices]
    api_endpoint=https://readeckobo.example.com/instapaper-proxy/storeapi
    instapaper_env_url=https://readeckobo.example.com/instapaper-proxy/instapaper

    [Instapaper]
    AccessToken=@ByteArray(<THE-ENCRYPTED-TOKEN-FROM-THE-SCRIPT>)
    ```

Replace `readeckobo.example.com` with the hostname of your proxy instance.

### 5. Set Up a Reverse Proxy

`readeckobo` must be run behind a reverse proxy to handle HTTPS. It's crucial
to proxy three specific location blocks, as shown in our `nginx.conf.snippet`
example.

Your Kobo device periodically re-syncs its configuration from Kobo's servers,
which can overwrite your custom Instapaper endpoint. The proxy rules below
ensure this connection is preserved.

<!-- markdownlint-disable MD013 -->
| Location Block                                  | Proxies To               | Purpose                                                                                             |
| ----------------------------------------------- | ------------------------ | --------------------------------------------------------------------------------------------------- |
| `/instapaper-proxy/instapaper/`                 | `readeckobo` application | Handles the main Instapaper API requests (sync, download, etc.) to your `readeckobo` instance.      |
| `/instapaper-proxy/storeapi/`                   | `storeapi.kobo.com`      | Forwards general API requests to Kobo's servers.                                                    |
| `/instapaper-proxy/storeapi/v1/initialization`  | `storeapi.kobo.com`      | Intercepts the Kobo configuration response to rewrite the Instapaper URL back to your proxy endpoint. |
<!-- markdownlint-enable MD013 -->

Without these rules, your Kobo will eventually lose its connection to `readeckobo`.

## 🔒 A Quick Word on Security

A little security goes a long way.

* **Use HTTPS:** deploy behind a reverse proxy that provides HTTPS
* **Stay Local:** Keep it on your local private network
* **Kobo Password:** prevent unauthorized mounting with a Kobo password

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
