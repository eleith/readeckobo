# readeckobo

Got a Kobo e-reader and a [Readeck](https://readeck.com) account? This tool is
pretends to be Instapaper, so your Kobo can sync with your Readeck articles.

This project is a Go port of the original
[kobeck](https://github.com/Lukas0907/kobeck) (written in python), but with a
few more bells and whistles like multi-user support and better logging.

## ✨ Features

* 📚 Fetches and lists un-archived articles from Readeck for Kobo devices.
* 📥 Downloads article content in a Kobo-compatible format.
* ✅ Handles actions like archiving, re-adding, favoriting, and deleting
articles by updating Readeck.
* 🖼️ Converts images to JPEG format on the fly for better e-reader
compatibility.
* 🤝 Supports multiple Kobo devices with different tokens.

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
    api_endpoint=https://readeckobo.example.com/storeapi
    instapaper_env_url=https://readeckobo.example.com/instapaper

    [Instapaper]
    AccessToken=@ByteArray(<THE-ENCRYPTED-TOKEN-FROM-THE-SCRIPT>)
    ```

Replace `readeckobo.example.com` with the hostname of your proxy instance.

### 5. Set Up a Reverse Proxy

`readeckobo` is designed to be run behind a reverse proxy. This is how you'll
handle HTTPS and expose it to the internet safely.

We've included an Nginx example in `nginx.conf.snippet`. Examples for Traefik,
Caddy, or others would be welcome contributions!

## 🔒 A Quick Word on Security

A little security goes a long way.

* **Use HTTPS:** Seriously. Run this behind a reverse proxy that provides HTTPS.
* **Stay Local:** For best security, don't expose `readeckobo` to the public
internet. Keep it on your local network.
* **Lock Down Your Kobo:** Set a password on your Kobo to prevent someone from
grabbing your proxy tokens.
* **Lost Device?** If your Kobo goes on an adventure without you, remove its
token from `config.yaml` and restart the server.

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
| `POST /api/kobo/get`       | **Fetches article list.** Syncs new, updated, and deleted articles from Readeck. |
| `POST /api/kobo/download` | **Downloads article content.** Grabs the content of an article for offline reading. |
| `POST /api/kobo/send`     | **Sends updates.** Handles archiving, favoriting, deleting, or adding articles. |
| `GET /api/convert-image`  | **Converts images.** A helper endpoint to convert images to JPEG on the fly. |
<!-- markdownlint-enable MD013 -->

### Testing

The `scripts/e2e-tests/` directory has simple shell scripts for testing each API
endpoint. They're great for checking if everything is working as expected.

```sh
# Make the scripts executable
chmod +x scripts/e2e-tests/*.sh

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
