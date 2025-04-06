# Babashka WhatsApp Pod

A Babashka pod for interacting with WhatsApp, allowing Babashka scripts to send and receive messages through WhatsApp.

## Features

- Login to WhatsApp by scanning a QR code
- Send WhatsApp messages to contacts
- Check connection status
- Logout from WhatsApp
- Persistent session storage

## Prerequisites

- [Babashka](https://github.com/babashka/babashka#installation)
- Go 1.17+

## Installation

Clone this repository:

```bash
git clone https://github.com/kbosompem/bb-whatsapp-pod.git
cd bb-whatsapp-pod
```

Build the pod:

```bash
go build -o bb-whatsapp-pod ./cmd/bb-whatsapp-pod
```

## Usage

### Loading the Pod in Babashka

```clojure
(require '[babashka.pods :as pods])
(pods/load-pod "./bb-whatsapp-pod")
(require '[pod.whatsapp :as wa])
```

### Logging in to WhatsApp

The pod generates a QR code that you can scan with your WhatsApp mobile app to log in:

```clojure
(def login-result (wa/login))
;; Display the QR code in terminal
(println (:qr_code login-result))
```

### Checking Status

You can check the connection status:

```clojure
(wa/status)
;; Returns: {:status "logged-in"} or other status values
```

### Sending a Message

Once logged in, you can send messages to WhatsApp contacts:

```clojure
(wa/send-message "1234567890" "Hello from Babashka!")
```

The first argument is the phone number (with country code but without the '+' sign), and the second argument is the message text.

### Logging Out

```clojure
(wa/logout)
```

## Example

See the `example.clj` file for a complete example:

```bash
# Run the example
bb example.clj
```

Note: This project uses `bb` as the alias for Babashka. If your system uses a different command, please adjust accordingly.

## Troubleshooting

### Connection Issues
If you encounter issues with the pod connecting to WhatsApp servers:
- Check your internet connection
- Delete the `whatsapp.db` file and try logging in again
- Ensure your WhatsApp app is up to date

### Pod Communication Issues
If the pod fails to load or communicate with Babashka:
- Ensure you've built the pod with the correct Go version
- Try using an absolute path to the pod: `(pods/load-pod "/full/path/to/bb-whatsapp-pod")`
- Check for permission issues with the executable: `chmod +x bb-whatsapp-pod`

## How It Works

The pod uses the [whatsmeow](https://github.com/tulir/whatsmeow) library to interact with WhatsApp. It implements the [Babashka pod protocol](https://github.com/babashka/babashka.pods#pod-protocol) for communication with Babashka.

When you request a QR code for login, the pod:

1. Connects to WhatsApp's servers
2. Generates a QR code
3. Returns the QR code as ASCII art to display in your terminal 
4. After scanning, establishes a persistent connection

The pod stores your session in a SQLite database file (`whatsapp.db`) in the current directory, allowing you to reuse the session without scanning the QR code again.

## Security Considerations

- Your WhatsApp session is stored in the `whatsapp.db` file
- Be careful not to expose this file to untrusted users or scripts
- The pod maintains your session as long as the `whatsapp.db` file exists
- Delete the `whatsapp.db` file to completely remove your session

## License

MIT

## Deployment

### Building and Releasing

This pod uses GitHub Actions to automatically build and release for multiple platforms. To create a new release:

1. Tag your commit with a version number:
```bash
git tag v1.0.0
git push origin v1.0.0
```

2. The GitHub Action will automatically:
   - Build the pod for multiple platforms (Linux, macOS, Windows)
   - Create a GitHub release with all binaries
   - Generate the necessary `pod.json` manifest

### Adding to Babashka Pod Registry

To add this pod to the [Babashka Pod Registry](https://github.com/babashka/pod-registry):

1. Fork the pod-registry repository
2. Add your pod information to `pod-registry.json`:
```json
{
  "bb-whatsapp-pod": {
    "name": "bb-whatsapp-pod",
    "description": "A Babashka pod for interacting with WhatsApp",
    "url": "https://github.com/kbosompem/bb-whatsapp-pod",
    "latest": {
      "version": "1.0.0",
      "pod_version": "1.0.0"
    }
  }
}
```

3. Create a pull request to the pod-registry repository

### Manual Installation

Users can install the pod manually by:

1. Downloading the appropriate binary for their platform from the GitHub releases page
2. Making it executable (Unix-like systems):
```bash
chmod +x bb-whatsapp-pod-*
```

3. Moving it to a directory in their PATH or using it from the current directory

### Version Updates

When releasing a new version:

1. Update version numbers in:
   - `pod.json`
   - Documentation
   - Submit a PR to update the pod-registry

2. Create a new git tag and push:
```bash
git tag v1.0.1
git push origin v1.0.1
```

3. The GitHub Action will handle the rest of the release process 