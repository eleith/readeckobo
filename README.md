# readeckobo

`readeckobo` is a Go application that acts as a bridge between Kobo e-readers and a [Readeck](https://readeck.com) instance. It emulates the Instapaper API to allow Kobo devices to sync articles from Readeck.

This project is a Go port of the original Python-based [kobeck](https://github.com/Lukas0907/kobeck).

## Features

*   Fetches and lists articles from Readeck for Kobo devices.
*   Downloads article content in a Kobo-compatible format.
*   Handles actions like archiving, re-adding, favoriting, and deleting articles.
*   Converts images to JPEG format for better compatibility.

## API Endpoints

`readeckobo` proxies the following Kobo (Instapaper-like) API endpoints to Readeck:

| Kobo Endpoint (`readeckobo`) | Readeck API Endpoint (Proxied) | Description                                      |
| :--------------------------- | :----------------------------- | :----------------------------------------------- |
| `POST /api/kobo/get`         | `GET /api/bookmarks/sync`      | Fetches updated/deleted bookmarks.               |
| `POST /api/kobo/download`    | `GET /api/bookmarks`           | Finds a bookmark by URL.                         |
|                              | `GET /api/bookmarks/{id}/article` | Fetches the article content.                     |
| `POST /api/kobo/send`        | `PATCH /api/bookmarks/{id}`    | Updates bookmark status (archive, favorite, delete). |
|                              | `POST /api/bookmarks`          | Creates a new bookmark (for "add" action).       |
| `GET /api/convert-image`     | (External Image URL)           | Fetches and converts external images to JPEG.    |

## Configuration

The application is configured using a `config.yaml` file. An example is provided in `config.yaml.example`:

```yaml
readeck:
  host: "https://your-readeck-instance.com"
  access_token: "your-encrypted-readeck-access-token"
server:
  port: 8080
```

-   `readeck.host`: The URL of your Readeck instance.
-   `readeck.access_token`: Your Readeck access token, encrypted using the `generate-access-token` script.
-   `server.port`: The port on which the `readeckobo` server will run.

### Generating the Encrypted Access Token

The `access_token` in `config.yaml` must be encrypted. Use the `generate-access-token` script located in the `bin` directory:

```sh
./bin/generate-access-token <READECK_TOKEN> <KOBO_SERIAL>
```

-   `<READECK_TOKEN>`: Your plain-text Readeck access token.
-   `<KOBO_SERIAL>`: The serial number of your Kobo device.

The output of this script is the encrypted token to be used in `config.yaml`.

### Finding Your Kobo Serial Number

You can find your Kobo's serial number in a few ways:

1.  **On the device (most reliable):**
    *   Go to `Settings` > `Device Information`. The serial number will be listed there.

2.  **On the original packaging:**
    *   The serial number is usually printed on a sticker on the box your Kobo came in.

3.  **In your email:**
    *   If you purchased your Kobo from the Kobo store online, your serial number may be in the purchase confirmation email.

## Usage

### Building and Running Locally

To build and run the application locally:

1.  **Install dependencies:**
    ```sh
    go mod tidy
    ```

2.  **Build the application:**
    ```sh
    make build
    ```

3.  **Run the application:**
    ```sh
    ./readeckobo
    ```

### Using the Makefile

The `Makefile` provides several useful targets:

-   `make build`: Build the application binary.
-   `make test`: Run all unit tests.
-   `make lint`: Run the linter.
-   `make vendor`: Vendor all dependencies.
-   `make ci`: Run all CI checks (linting and testing).

### Running with Docker

To run the application using Docker:

1.  **Build the Docker image:**
    ```sh
    docker-compose build
    ```

2.  **Run the container:**
    ```sh
    docker-compose up
    ```

The server will be available at `http://localhost:8080`.