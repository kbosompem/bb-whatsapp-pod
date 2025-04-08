# Babashka WhatsApp Pod

A Babashka pod for interacting with WhatsApp, allowing Babashka scripts to send and receive messages through WhatsApp.

## Features

- Login to WhatsApp by scanning a QR code
- Send WhatsApp messages to contacts
- Get list of groups and send messages to groups
- Check connection status
- Logout from WhatsApp
- Persistent session storage
- Contact management (get info, profile pictures)
- Status management (get/set status messages)
- Presence management (online/offline status)

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

### Working with Groups

You can manage WhatsApp groups with various functions:

```clojure
;; Get a list of all groups you're in
(let [groups-result (wa/get-groups)]
  (when (:success groups-result)
    (doseq [group (:groups groups-result)]
      (println "Group:" (:name group))
      (println "JID:" (:jid group))
      (println "Participants:" (:participants group)))))

;; Send a message to a group
(wa/send-group-message "1234567890@g.us" "Hello group!")

;; Create a new group
(let [group-info {:name "My New Group"
                  :participants ["1234567890@s.whatsapp.net" "0987654321@s.whatsapp.net"]}]
  (wa/create-group group-info))

;; Leave a group
(wa/leave-group "1234567890@g.us")

;; Get a group's invite link
(let [link-result (wa/get-group-invite-link "1234567890@g.us")]
  (when (:success link-result)
    (println "Invite link:" (:message link-result))))

;; Join a group using an invite link
(wa/join-group-with-link "https://chat.whatsapp.com/...")

;; Change a group's name
(wa/set-group-name "1234567890@g.us" "New Group Name")
```

Note: Some group management features are not available in the current version of the WhatsApp API:
- Setting group description/topic
- Adding/removing participants
- Promoting/demoting group admins

### Working with Media

You can upload and send various types of media files:

```clojure
;; Upload a file
(let [upload-result (wa/upload "image.jpg" "image/jpeg")]
  (when (:success upload-result)
    (println "Uploaded successfully!")))

;; Send an image
(wa/send-image "1234567890@s.whatsapp.net" "image.jpg" "Check out this image!")

;; Send a document
(wa/send-document "1234567890@s.whatsapp.net" "document.pdf" "Here's the document you requested")

;; Send a video
(wa/send-video "1234567890@s.whatsapp.net" "video.mp4" "Check out this video!")

;; Send an audio file
(wa/send-audio "1234567890@s.whatsapp.net" "audio.mp3")
```

Note: The following media types are supported:
- Images (JPEG, PNG, GIF)
- Documents (PDF, DOC, XLS, etc.)
- Videos (MP4)
- Audio (MP3)

### Contact Management

Get information about a contact:

```clojure
(let [contact-result (wa/get-contact-info "1234567890@s.whatsapp.net")]
  (when (:success contact-result)
    (let [contact (:contact contact-result)]
      (println "Name:" (:name contact))
      (println "Push Name:" (:push_name contact))
      (println "JID:" (:jid contact)))))
```

Get a contact's profile picture:

```clojure
(let [pic-result (wa/get-profile-picture "1234567890@s.whatsapp.net")]
  (when (:success pic-result)
    (let [media (:media pic-result)]
      (println "Profile picture URL:" (:url media)))))
```

Note: The following contact management features are not available in the current version of the WhatsApp API:
- Setting profile picture
- Blocking/unblocking contacts
- Getting blocked contacts list
- Getting all contacts list
- Updating contact name
- Deleting contacts

### Status Management

Set your status message:

```clojure
(wa/set-status "Available for work!")
```

Get a contact's status:

```clojure
(let [status-result (wa/get-status "1234567890@s.whatsapp.net")]
  (when (:success status-result)
    (let [status (:status status-result)]
      (println "Status:" (:text status))
      (println "Last updated:" (:timestamp status)))))
```

### Presence Management

Set your online/offline status:

```clojure
;; Set online
(wa/set-presence true)

;; Set offline
(wa/set-presence false)
```

Subscribe to a contact's presence updates:

```clojure
(wa/subscribe-presence "1234567890@s.whatsapp.net")
```

### Logging Out

```clojure
(wa/logout)
```

## Error Handling

All functions return a result map with a `:success` boolean field and an optional `:message` string field for error messages. Always check the `:success` field before proceeding with the result:

```clojure
(let [result (wa/send-message "1234567890" "Hello")]
  (if (:success result)
    (println "Message sent successfully!")
    (println "Failed to send message:" (:message result))))
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## TODO

### Message Management
- [x] Send messages to contacts
- [x] Send messages to groups
- [x] Send media messages (images)
- [ ] Send other media types (audio, video, documents)
- [ ] Get message history (not available in current API)
- [ ] Get unread messages (not available in current API)
- [ ] Mark messages as read
- [ ] Delete messages (not available in current API)

### Group Management
- [x] Get list of groups
- [x] Create new groups
- [x] Leave groups
- [x] Get group invite links
- [x] Join groups with invite links
- [x] Change group names
- [ ] Set group descriptions (not available in current API)
- [ ] Add/remove participants (not available in current API)
- [ ] Promote/demote admins (not available in current API)

### Contact Management
- [x] Get contact information
- [x] Get profile pictures
- [x] Set status messages
- [x] Get contact status
- [x] Set online/offline status
- [x] Subscribe to presence updates
- [ ] Set profile picture (not available in current API)
- [ ] Block/unblock contacts (not available in current API)
- [ ] Get blocked contacts list (not available in current API)
- [ ] Get all contacts list (not available in current API)
- [ ] Update contact names (not available in current API)
- [ ] Delete contacts (not available in current API)

### General Improvements
- [ ] Add support for message reactions
- [ ] Add support for message replies
- [ ] Add support for message forwarding
- [ ] Add support for message editing
- [ ] Add support for message pinning
- [ ] Add support for message starring
- [ ] Add support for message search
- [ ] Add support for message filtering
- [ ] Add support for message archiving
- [ ] Add support for message backup
- [ ] Add support for message restore
- [ ] Add support for message export
- [ ] Add support for message import
- [ ] Add support for message scheduling
- [ ] Add support for message templates
- [ ] Add support for message automation
- [ ] Add support for message analytics
- [ ] Add support for message monitoring
- [ ] Add support for message moderation
- [ ] Add support for message encryption
- [ ] Add support for message decryption
- [ ] Add support for message verification
- [ ] Add support for message validation
- [ ] Add support for message sanitization
- [ ] Add support for message formatting
- [ ] Add support for message parsing
- [ ] Add support for message rendering
- [ ] Add support for message translation
- [ ] Add support for message localization
- [ ] Add support for message internationalization
- [ ] Add support for message accessibility
- [ ] Add support for message security
- [ ] Add support for message privacy
- [ ] Add support for message compliance
- [ ] Add support for message auditing
- [ ] Add support for message logging
- [ ] Add support for message tracking
- [ ] Add support for message reporting
- [ ] Add support for message analytics
- [ ] Add support for message monitoring
- [ ] Add support for message moderation
- [ ] Add support for message automation
- [ ] Add support for message scheduling
- [ ] Add support for message templates
- [ ] Add support for message backup
- [ ] Add support for message restore
- [ ] Add support for message export
- [ ] Add support for message import
- [ ] Add support for message archiving
- [ ] Add support for message search
- [ ] Add support for message filtering
- [ ] Add support for message pinning
- [ ] Add support for message starring
- [ ] Add support for message editing
- [ ] Add support for message forwarding
- [ ] Add support for message replies
- [ ] Add support for message reactions