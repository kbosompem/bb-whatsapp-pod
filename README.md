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
- qrencode (for displaying QR codes in terminal)
  - macOS: `brew install qrencode`
  - Ubuntu/Debian: `sudo apt-get install qrencode`
  - Fedora: `sudo dnf install qrencode`
  - Windows: Available through [Chocolatey](https://chocolatey.org/): `choco install qrencode`
  - From source: [libqrencode](https://github.com/fukuchi/libqrencode)

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

The pod generates a QR code that you can scan with your WhatsApp mobile app to log in. Make sure you have `qrencode` installed to see the QR code in your terminal:

```clojure
(let [login-result (wa/login)]
  (when (= (:status login-result) "qr-pending")
    (println "--- QR CODE ---")
    (-> (process ["qrencode" "-t" "ANSI256" "-o" "-" (:qr_code login-result)] {:out :inherit})
        deref)
    (println "---------------")
    (println "\nPlease scan the QR code string above using WhatsApp on your phone (Link a device).")
    (println "Press Enter here after you have scanned the QR code...")
    (read-line)
    (println "Checking status after scanning...")))
```

If you don't see the QR code properly in your terminal:
- Ensure `qrencode` is installed and available in your PATH
- Try adjusting your terminal font size or window width
- Use a terminal that supports Unicode characters
- For Windows users: Use Windows Terminal or a modern terminal emulator

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

### QR Code Display Issues
If the QR code doesn't display correctly:
- Verify `qrencode` is installed: `qrencode --version`
- Try running `qrencode -t ANSI "test"` to test QR code generation
- Use a monospace font in your terminal
- Ensure your terminal window is wide enough
- For Windows: Use Windows Terminal instead of cmd.exe

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
