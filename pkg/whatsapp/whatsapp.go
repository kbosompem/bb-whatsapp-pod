package whatsapp

import (
	"context"
	"fmt"
	"log" // Import standard log package
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// WhatsAppClient wraps the whatsmeow client and related state
type WhatsAppClient struct {
	Client       *whatsmeow.Client
	dbContainer  *sqlstore.Container
	jid          types.JID
	loginStatus  string      // "not-logged-in", "qr-pending", "logged-in", "login-failed", "connecting"
	qrCodeStr    string      // Stores the QR code string when received
	qrChan       chan string // Channel to signal QR code availability
	loginMutex   sync.Mutex  // Protect concurrent login attempts
	lastMessage  *MessageInfo
	messageMutex sync.Mutex
}

// Result types for pod responses
type StatusResult struct {
	Status      string       `json:"status"`
	LastMessage *MessageInfo `json:"last_message,omitempty"`
}

type LoginResult struct {
	Status  string `json:"status"`
	QrCode  string `json:"qr_code,omitempty"` // Changed: Now returns the actual QR code string
	Message string `json:"message,omitempty"`
}

type SendResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type MessageInfo struct {
	ChatID      string `json:"chat_id"`
	Content     string `json:"content"`
	Sender      string `json:"sender"`
	IsFromMe    bool   `json:"is_from_me"`
	MessageType string `json:"message_type"`
	Timestamp   int64  `json:"timestamp"`
}

// GroupInfo represents information about a WhatsApp group
type GroupInfo struct {
	JID          string   `json:"jid"`
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// GroupResult represents the result of group operations
type GroupResult struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Groups  []GroupInfo `json:"groups,omitempty"`
}

// MediaInfo represents information about uploaded media
type MediaInfo struct {
	URL        string `json:"url"`
	DirectURL  string `json:"direct_url"`
	Mimetype   string `json:"mimetype"`
	FileSHA256 []byte `json:"file_sha256"`
	FileLength uint64 `json:"file_length"`
	MediaKey   []byte `json:"media_key"`
}

// UploadResult represents the result of media upload operations
type UploadResult struct {
	Success bool       `json:"success"`
	Message string     `json:"message,omitempty"`
	Media   *MediaInfo `json:"media,omitempty"`
}

// ContactInfo represents information about a WhatsApp contact
type ContactInfo struct {
	JID          string `json:"jid"`
	Name         string `json:"name"`
	PushName     string `json:"push_name"`
	Status       string `json:"status"`
	LastSeen     int64  `json:"last_seen,omitempty"`
	IsOnline     bool   `json:"is_online,omitempty"`
	ProfilePicID string `json:"profile_pic_id,omitempty"`
}

// ContactResult represents the result of contact operations
type ContactResult struct {
	Success bool         `json:"success"`
	Message string       `json:"message,omitempty"`
	Contact *ContactInfo `json:"contact,omitempty"`
}

// StatusInfo represents information about a WhatsApp status
type StatusInfo struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

// StatusUpdateResult represents the result of status update operations
type StatusUpdateResult struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Status  *StatusInfo `json:"status,omitempty"`
}

// PresenceInfo represents information about a contact's presence
type PresenceInfo struct {
	JID      string `json:"jid"`
	IsOnline bool   `json:"is_online"`
	LastSeen int64  `json:"last_seen,omitempty"`
}

// PresenceResult represents the result of presence operations
type PresenceResult struct {
	Success  bool          `json:"success"`
	Message  string        `json:"message,omitempty"`
	Presence *PresenceInfo `json:"presence,omitempty"`
}

// MessageHistoryInfo represents information about a message in chat history
type MessageHistoryInfo struct {
	ID          string `json:"id"`
	ChatID      string `json:"chat_id"`
	Content     string `json:"content"`
	Sender      string `json:"sender"`
	IsFromMe    bool   `json:"is_from_me"`
	MessageType string `json:"message_type"`
	Timestamp   int64  `json:"timestamp"`
	IsRead      bool   `json:"is_read"`
}

// MessageHistoryResult represents the result of message history operations
type MessageHistoryResult struct {
	Success  bool                 `json:"success"`
	Message  string               `json:"message,omitempty"`
	Messages []MessageHistoryInfo `json:"messages,omitempty"`
}

// GroupCreateInfo represents information needed to create a group
type GroupCreateInfo struct {
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// GroupCreateResult represents the result of group creation
type GroupCreateResult struct {
	Success bool       `json:"success"`
	Message string     `json:"message,omitempty"`
	Group   *GroupInfo `json:"group,omitempty"`
}

// NewClient initializes the whatsmeow client
func NewClient(dbPath string) (*WhatsAppClient, error) {
	// Configure whatsmeow components to use Noop logger
	dbLogger := waLog.Noop
	clientLogger := waLog.Noop

	log.Printf("[whatsapp] Initializing DB with path: %s", dbPath) // Use standard log
	container, err := sqlstore.New("sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)", dbPath), dbLogger)
	if err != nil {
		log.Printf("[whatsapp] Error connecting database: %v", err) // Use standard log
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}
	log.Println("[whatsapp] Database container created.")

	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		log.Printf("[whatsapp] Error getting device store: %v", err) // Use standard log
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	log.Println("[whatsapp] Device store retrieved.")

	client := whatsmeow.NewClient(deviceStore, clientLogger)
	log.Println("[whatsapp] Whatsmeow client created.")

	wac := &WhatsAppClient{
		Client:      client,
		dbContainer: container,
		loginStatus: "not-logged-in",
		qrChan:      make(chan string, 1), // Buffered channel for QR code
	}

	wac.Client.AddEventHandler(wac.eventHandler)
	log.Println("[whatsapp] Event handler added.")

	return wac, nil
}

// eventHandler handles incoming events from whatsmeow client
func (wac *WhatsAppClient) eventHandler(evt interface{}) {
	log.Printf("[EventHandler] Received event: %T", evt)
	switch v := evt.(type) {
	case *events.Message:
		wac.handleMessage(v)
	case *events.Connected:
		log.Println("[EventHandler] Connected event")
		if wac.Client.Store.ID != nil {
			wac.jid = *wac.Client.Store.ID
			log.Printf("[EventHandler] Already logged in with JID: %s", wac.jid)
			wac.loginStatus = "logged-in"
			select {
			case wac.qrChan <- "logged-in":
			default:
			}
		} else {
			log.Println("[EventHandler] Connected, but not logged in yet.")
		}
	case *events.PushName:
		log.Printf("[EventHandler] Push name update for %s: %s", v.JID, v.NewPushName)
	case *events.StreamReplaced:
		log.Println("[EventHandler] Stream replaced event received")
		wac.loginStatus = "not-logged-in"
	case *events.Disconnected:
		log.Println("[EventHandler] Disconnected event")
		if wac.loginStatus != "logged-out" {
			wac.loginStatus = "not-logged-in"
		}
	case *events.QR:
		log.Println("[EventHandler] QR event")
		if wac.loginStatus != "logged-in" {
			wac.loginStatus = "qr-pending"
		}
		if len(v.Codes) > 0 {
			qrCode := v.Codes[0]
			wac.qrCodeStr = qrCode
			log.Println("[EventHandler] QR code captured. Sending to login channel.")
			select {
			case wac.qrChan <- qrCode:
				log.Println("[EventHandler] Sent QR code to channel")
			default:
				log.Println("[EventHandler] QR channel was full/closed.")
			}
		} else {
			log.Println("[EventHandler] QR event with no codes.")
		}
	case *events.PairSuccess:
		log.Printf("[EventHandler] PairSuccess event! JID: %s, Platform: %s", v.ID, v.Platform)
		wac.jid = v.ID
		wac.loginStatus = "logged-in"
		select {
		case wac.qrChan <- "logged-in":
		default:
		}
	case *events.ClientOutdated:
		log.Printf("[EventHandler] ERROR: Client is outdated. Please update the pod.")
		wac.loginStatus = "login-failed"
		// Signal login failure via the channel
		select {
		case wac.qrChan <- "login-failed":
		default:
		}
	case *events.OfflineSyncCompleted:
		log.Println("[EventHandler] Offline sync completed")
	case *events.HistorySync: // Handle history sync progress
		if v.Data != nil && v.Data.Progress != nil {
			log.Printf("[EventHandler] History sync progress: %.2f%%", *v.Data.Progress)
		}
	}
}

// handleMessage processes incoming messages
func (wac *WhatsAppClient) handleMessage(msg *events.Message) {
	log.Printf("[MessageHandler] Received message from %s", msg.Info.Sender)

	var content string
	if msg.Message.GetConversation() != "" {
		content = msg.Message.GetConversation()
	} else if msg.Message.GetExtendedTextMessage() != nil {
		content = msg.Message.GetExtendedTextMessage().GetText()
	} else {
		content = "[Media or other content type]"
	}

	messageInfo := &MessageInfo{
		ChatID:      msg.Info.Chat.String(),
		Content:     content,
		Sender:      msg.Info.Sender.String(),
		IsFromMe:    msg.Info.IsFromMe,
		MessageType: "text",
		Timestamp:   msg.Info.Timestamp.Unix(),
	}

	wac.messageMutex.Lock()
	wac.lastMessage = messageInfo
	wac.messageMutex.Unlock()

	log.Printf("[MessageHandler] Processed message: %+v", messageInfo)
}

// Login initiates the WhatsApp login process
func (wac *WhatsAppClient) Login() (interface{}, error) {
	wac.loginMutex.Lock() // Prevent concurrent login attempts
	defer wac.loginMutex.Unlock()

	if wac.Client.IsLoggedIn() {
		wac.loginStatus = "logged-in"
		return LoginResult{Status: "logged-in", Message: "Already logged in"}, nil
	}

	// If already connecting or pending QR from a *previous* call, report status
	// (Mutex prevents true concurrency, but state might persist)
	if wac.loginStatus == "connecting" || wac.loginStatus == "qr-pending" {
		// If QR is pending, maybe return the stored QR code?
		if wac.loginStatus == "qr-pending" && wac.qrCodeStr != "" {
			return LoginResult{Status: wac.loginStatus, Message: "Login pending, scan QR code", QrCode: wac.qrCodeStr}, nil
		}
		return LoginResult{Status: wac.loginStatus, Message: "Login already in progress"}, nil
	}

	// Reset state for new login attempt
	wac.loginStatus = "connecting"
	wac.qrCodeStr = ""
	// Clear the channel in case of old data
	select {
	case <-wac.qrChan:
	default:
	}

	go func() {
		err := wac.Client.Connect()
		if err != nil {
			if !strings.Contains(err.Error(), "disconnect called") {
				log.Printf("[Login Connect GoRoutine] ERROR: Connection failed: %v", err)
				if wac.loginStatus != "logged-in" {
					wac.loginStatus = "login-failed"
					// Signal failure via channel
					select {
					case wac.qrChan <- "login-failed":
					default:
					}
				}
			}
			return
		}
		log.Println("[Login Connect GoRoutine] Connect() returned successfully, waiting for QR/Login event...")
	}()

	// Wait for QR code, login success, or failure signal from event handler via channel
	select {
	case resultSignal := <-wac.qrChan:
		log.Printf("[Login] Received signal from qrChan: %s", resultSignal)
		switch resultSignal {
		case "logged-in":
			wac.loginStatus = "logged-in"
			return LoginResult{Status: "logged-in"}, nil
		case "login-failed":
			wac.loginStatus = "login-failed"
			return LoginResult{Status: "login-failed", Message: "Login process failed"}, fmt.Errorf("login failed")
		default: // Assume it's the QR code string
			wac.loginStatus = "qr-pending"
			wac.qrCodeStr = resultSignal // Store it again just in case
			return LoginResult{Status: "qr-pending", Message: "Scan QR code", QrCode: resultSignal}, nil
		}
	case <-time.After(65 * time.Second): // Timeout waiting for event
		log.Printf("[Login] WARN: Login timed out after 65 seconds waiting for event.")
		if wac.loginStatus == "connecting" || wac.loginStatus == "qr-pending" {
			wac.loginStatus = "login-failed"
			wac.Client.Disconnect() // Clean up connection attempt
		}
		return LoginResult{Status: "timeout", Message: "Login timed out"}, fmt.Errorf("login timed out")
	case <-wac.interruptForShutdown():
		log.Println("[Login] WARN: Login interrupted by shutdown signal.")
		return LoginResult{Status: "interrupted"}, fmt.Errorf("login interrupted")
	}
}

// interruptForShutdown creates a channel that closes on SIGINT/SIGTERM
func (wac *WhatsAppClient) interruptForShutdown() <-chan struct{} {
	c := make(chan struct{})
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		<-signals
		log.Println("[Interrupt] Received interrupt signal, shutting down...")
		close(c)
	}()
	return c
}

// Logout logs the client out
func (wac *WhatsAppClient) Logout() (interface{}, error) {
	log.Printf("INFO: Logging out...")
	// Set status first, so disconnect event doesn't reset to not-logged-in
	wac.loginStatus = "logged-out"
	err := wac.Client.Logout()
	if err != nil {
		log.Printf("ERROR: Error logging out: %v", err)
		return StatusResult{Status: "logout-failed"}, err
	}
	log.Printf("INFO: Logout successful.")
	wac.jid = types.JID{}
	return StatusResult{Status: "logged-out"}, nil
}

// Status returns the current connection status and last message
func (wac *WhatsAppClient) Status() (interface{}, error) {
	wac.messageMutex.Lock()
	lastMsg := wac.lastMessage
	wac.messageMutex.Unlock()

	return StatusResult{
		Status:      wac.loginStatus,
		LastMessage: lastMsg,
	}, nil
}

// SendMessage sends a message to the specified phone number
func (wac *WhatsAppClient) SendMessage(phone string, message string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	recipient := types.JID{
		User:   phone,
		Server: "s.whatsapp.net",
	}

	msg := &waProto.Message{
		Conversation: &message,
	}

	ts := time.Now()
	_, err := wac.Client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: fmt.Sprintf("Message sent (server timestamp: %v)", ts),
	}, nil
}

// Disconnect cleans up the client connection
func (wac *WhatsAppClient) Disconnect() {
	if wac.Client != nil {
		log.Printf("INFO: Disconnecting WhatsApp client...")
		wac.Client.Disconnect()
	}
	if wac.dbContainer != nil {
		log.Printf("INFO: Closing database connection...")
		err := wac.dbContainer.Close()
		if err != nil {
			log.Printf("ERROR: Error closing database: %v", err)
		}
	}
	log.Printf("INFO: Cleanup complete.")
}

// GetGroups returns a list of all groups the user is in
func (wac *WhatsAppClient) GetGroups() (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	groups, err := wac.Client.GetJoinedGroups()
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	groupInfos := make([]GroupInfo, len(groups))
	for i, group := range groups {
		participants := make([]string, len(group.Participants))
		for j, participant := range group.Participants {
			participants[j] = participant.JID.String()
		}

		groupInfos[i] = GroupInfo{
			JID:          group.JID.String(),
			Name:         group.Name,
			Participants: participants,
		}
	}

	return GroupResult{
		Success: true,
		Groups:  groupInfos,
	}, nil
}

// SendGroupMessage sends a message to a WhatsApp group
func (wac *WhatsAppClient) SendGroupMessage(groupJID string, message string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	recipient, err := types.ParseJID(groupJID)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	msg := &waProto.Message{
		Conversation: &message,
	}

	ts := time.Now()
	_, err = wac.Client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: fmt.Sprintf("Message sent to group (server timestamp: %v)", ts),
	}, nil
}

// Upload uploads a media file to WhatsApp servers
func (wac *WhatsAppClient) Upload(filePath string, mimeType string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return UploadResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return UploadResult{Success: false, Message: err.Error()}, err
	}

	// Upload the file
	uploaded, err := wac.Client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return UploadResult{Success: false, Message: err.Error()}, err
	}

	mediaInfo := &MediaInfo{
		URL:        uploaded.URL,
		DirectURL:  uploaded.DirectPath,
		Mimetype:   mimeType,
		FileSHA256: uploaded.FileSHA256,
		FileLength: uploaded.FileLength,
		MediaKey:   uploaded.MediaKey,
	}

	return UploadResult{
		Success: true,
		Media:   mediaInfo,
	}, nil
}

// SendImage sends an image to a contact or group
func (wac *WhatsAppClient) SendImage(recipient string, filePath string, caption string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Parse recipient JID
	recipientJID, err := types.ParseJID(recipient)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Read the image file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Upload the image
	uploaded, err := wac.Client.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Create the image message
	msg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:        &uploaded.URL,
			Mimetype:   proto.String("image/jpeg"),
			Caption:    proto.String(caption),
			FileSHA256: uploaded.FileSHA256,
			FileLength: proto.Uint64(uploaded.FileLength),
			MediaKey:   uploaded.MediaKey,
			DirectPath: proto.String(uploaded.DirectPath),
		},
	}

	// Send the message
	ts := time.Now()
	_, err = wac.Client.SendMessage(context.Background(), recipientJID, msg)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: fmt.Sprintf("Image sent (server timestamp: %v)", ts),
	}, nil
}

// GetContactInfo retrieves information about a contact
func (wac *WhatsAppClient) GetContactInfo(jid string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return ContactResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	contactJID, err := types.ParseJID(jid)
	if err != nil {
		return ContactResult{Success: false, Message: err.Error()}, err
	}

	// Get contact info from the store
	contact, err := wac.Client.Store.Contacts.GetContact(contactJID)
	if err != nil {
		return ContactResult{Success: false, Message: err.Error()}, err
	}

	contactInfo := &ContactInfo{
		JID:          contactJID.String(),
		Name:         contact.FullName,
		PushName:     contact.PushName,
		Status:       "",    // Not available in current API
		LastSeen:     0,     // Not available in current API
		IsOnline:     false, // Not available in current API
		ProfilePicID: "",    // Not available in current API
	}

	return ContactResult{
		Success: true,
		Contact: contactInfo,
	}, nil
}

// GetProfilePicture retrieves a contact's profile picture
func (wac *WhatsAppClient) GetProfilePicture(jid string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return UploadResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	contactJID, err := types.ParseJID(jid)
	if err != nil {
		return UploadResult{Success: false, Message: err.Error()}, err
	}

	pic, err := wac.Client.GetProfilePictureInfo(contactJID, &whatsmeow.GetProfilePictureParams{})
	if err != nil {
		return UploadResult{Success: false, Message: err.Error()}, err
	}

	if pic == nil {
		return UploadResult{Success: false, Message: "No profile picture found"}, nil
	}

	mediaInfo := &MediaInfo{
		URL:        pic.URL,
		DirectURL:  pic.DirectPath,
		Mimetype:   "image/jpeg",
		FileSHA256: nil, // Not available in ProfilePictureInfo
		FileLength: 0,   // Not available in ProfilePictureInfo
		MediaKey:   nil, // Not available in ProfilePictureInfo
	}

	return UploadResult{
		Success: true,
		Media:   mediaInfo,
	}, nil
}

// SetProfilePicture sets your own profile picture
func (wac *WhatsAppClient) SetProfilePicture(filePath string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Note: SetProfilePicture is not available in the current API version
	return SendResult{Success: false, Message: "Setting profile picture is not supported in the current API version"}, fmt.Errorf("not supported")
}

// SetStatus sets your status message
func (wac *WhatsAppClient) SetStatus(text string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return StatusUpdateResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	err := wac.Client.SetStatusMessage(text)
	if err != nil {
		return StatusUpdateResult{Success: false, Message: err.Error()}, err
	}

	statusInfo := &StatusInfo{
		Text:      text,
		Timestamp: time.Now().Unix(),
	}

	return StatusUpdateResult{
		Success: true,
		Status:  statusInfo,
	}, nil
}

// GetStatus gets a contact's status
func (wac *WhatsAppClient) GetStatus(jid string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return StatusUpdateResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	contactJID, err := types.ParseJID(jid)
	if err != nil {
		return StatusUpdateResult{Success: false, Message: err.Error()}, err
	}

	// Get contact info from the store
	_, err = wac.Client.Store.Contacts.GetContact(contactJID)
	if err != nil {
		return StatusUpdateResult{Success: false, Message: err.Error()}, err
	}

	statusInfo := &StatusInfo{
		Text:      "", // Not available in current API
		Timestamp: time.Now().Unix(),
	}

	return StatusUpdateResult{
		Success: true,
		Status:  statusInfo,
	}, nil
}

// SetPresence sets your online/offline status
func (wac *WhatsAppClient) SetPresence(isOnline bool) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return PresenceResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	presence := types.PresenceUnavailable
	if isOnline {
		presence = types.PresenceAvailable
	}

	err := wac.Client.SendPresence(presence)
	if err != nil {
		return PresenceResult{Success: false, Message: err.Error()}, err
	}

	presenceInfo := &PresenceInfo{
		JID:      wac.jid.String(),
		IsOnline: isOnline,
		LastSeen: time.Now().Unix(),
	}

	return PresenceResult{
		Success:  true,
		Presence: presenceInfo,
	}, nil
}

// SubscribePresence subscribes to a contact's presence updates
func (wac *WhatsAppClient) SubscribePresence(jid string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return PresenceResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	contactJID, err := types.ParseJID(jid)
	if err != nil {
		return PresenceResult{Success: false, Message: err.Error()}, err
	}

	err = wac.Client.SubscribePresence(contactJID)
	if err != nil {
		return PresenceResult{Success: false, Message: err.Error()}, err
	}

	presenceInfo := &PresenceInfo{
		JID:      contactJID.String(),
		IsOnline: false, // Initial state
	}

	return PresenceResult{
		Success:  true,
		Presence: presenceInfo,
	}, nil
}

// GetChatHistory retrieves chat history with a contact or group
func (wac *WhatsAppClient) GetChatHistory(jid string, limit int) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return MessageHistoryResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := types.ParseJID(jid)
	if err != nil {
		return MessageHistoryResult{Success: false, Message: err.Error()}, err
	}

	// Note: Message history retrieval is not directly available in the current API version
	// We can only access messages that are received while the client is running
	return MessageHistoryResult{
		Success: false,
		Message: "Message history retrieval is not supported in the current API version",
	}, fmt.Errorf("not supported")
}

// GetUnreadMessages retrieves all unread messages
func (wac *WhatsAppClient) GetUnreadMessages() (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return MessageHistoryResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Note: Unread message retrieval is not directly available in the current API version
	// We can only access messages that are received while the client is running
	return MessageHistoryResult{
		Success: false,
		Message: "Unread message retrieval is not supported in the current API version",
	}, fmt.Errorf("not supported")
}

// MarkMessageAsRead marks a message as read
func (wac *WhatsAppClient) MarkMessageAsRead(messageID string, chatJID string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Parse the chat JID
	parsedChatJID, err := types.ParseJID(chatJID)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Parse the message ID into the required type
	parsedMessageID := types.MessageID(messageID)

	// Mark the message as read
	err = wac.Client.MarkRead([]types.MessageID{parsedMessageID}, time.Now(), parsedChatJID, parsedChatJID, types.ReceiptTypeRead)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: "Message marked as read",
	}, nil
}

// DeleteMessage deletes a message
func (wac *WhatsAppClient) DeleteMessage(messageID string, forEveryone bool) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Note: Message deletion is not directly available in the current API version
	return SendResult{
		Success: false,
		Message: "Message deletion is not supported in the current API version",
	}, fmt.Errorf("not supported")
}

// CreateGroup creates a new WhatsApp group
func (wac *WhatsAppClient) CreateGroup(info *GroupCreateInfo) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupCreateResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Convert participant strings to JIDs
	participants := make([]types.JID, len(info.Participants))
	for i, p := range info.Participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return GroupCreateResult{Success: false, Message: fmt.Sprintf("Invalid participant JID: %s", p)}, err
		}
		participants[i] = jid
	}

	// Create the group using the ReqCreateGroup struct
	req := whatsmeow.ReqCreateGroup{
		Name:         info.Name,
		Participants: participants,
	}

	group, err := wac.Client.CreateGroup(req)
	if err != nil {
		return GroupCreateResult{Success: false, Message: err.Error()}, err
	}

	// Convert participants to strings for response
	participantStrings := make([]string, 0)
	for _, p := range participants {
		participantStrings = append(participantStrings, p.String())
	}

	groupInfo := &GroupInfo{
		JID:          group.JID.String(),
		Name:         info.Name,
		Participants: participantStrings,
	}

	return GroupCreateResult{
		Success: true,
		Group:   groupInfo,
	}, nil
}

// LeaveGroup leaves a WhatsApp group
func (wac *WhatsAppClient) LeaveGroup(groupJID string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	err = wac.Client.LeaveGroup(jid)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	return GroupResult{Success: true, Message: "Successfully left the group"}, nil
}

// GetGroupInviteLink gets the invite link for a group
func (wac *WhatsAppClient) GetGroupInviteLink(groupJID string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	link, err := wac.Client.GetGroupInviteLink(jid, false)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	return GroupResult{Success: true, Message: link}, nil
}

// JoinGroupWithLink joins a group using an invite link
func (wac *WhatsAppClient) JoinGroupWithLink(link string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := wac.Client.JoinGroupWithLink(link)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	return GroupResult{Success: true, Message: "Successfully joined the group"}, nil
}

// SetGroupName changes a group's name
func (wac *WhatsAppClient) SetGroupName(groupJID string, name string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	err = wac.Client.SetGroupName(jid, name)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	return GroupResult{Success: true, Message: "Group name updated successfully"}, nil
}

// SetGroupTopic changes a group's description/topic
func (wac *WhatsAppClient) SetGroupTopic(groupJID string, topic string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	// Note: SetGroupTopic is not available in the current API version
	return GroupResult{Success: false, Message: "Setting group topic is not supported in the current API version"}, fmt.Errorf("not supported")
}

// AddGroupParticipants adds participants to a group
func (wac *WhatsAppClient) AddGroupParticipants(groupJID string, participants []string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	// Note: AddGroupParticipants is not available in the current API version
	return GroupResult{Success: false, Message: "Adding group participants is not supported in the current API version"}, fmt.Errorf("not supported")
}

// RemoveGroupParticipants removes participants from a group
func (wac *WhatsAppClient) RemoveGroupParticipants(groupJID string, participants []string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	// Note: RemoveGroupParticipants is not available in the current API version
	return GroupResult{Success: false, Message: "Removing group participants is not supported in the current API version"}, fmt.Errorf("not supported")
}

// PromoteGroupParticipants promotes participants to admin status
func (wac *WhatsAppClient) PromoteGroupParticipants(groupJID string, participants []string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	// Note: PromoteGroupParticipants is not available in the current API version
	return GroupResult{Success: false, Message: "Promoting group participants is not supported in the current API version"}, fmt.Errorf("not supported")
}

// DemoteGroupParticipants demotes admins to regular participants
func (wac *WhatsAppClient) DemoteGroupParticipants(groupJID string, participants []string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return GroupResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	_, err := types.ParseJID(groupJID)
	if err != nil {
		return GroupResult{Success: false, Message: err.Error()}, err
	}

	// Note: DemoteGroupParticipants is not available in the current API version
	return GroupResult{Success: false, Message: "Demoting group participants is not supported in the current API version"}, fmt.Errorf("not supported")
}

// SendDocument sends a document to a contact or group
func (wac *WhatsAppClient) SendDocument(recipient string, filePath string, caption string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Parse recipient JID
	recipientJID, err := types.ParseJID(recipient)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Upload the document
	uploaded, err := wac.Client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Create the document message
	msg := &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:        &uploaded.URL,
			Mimetype:   proto.String("application/octet-stream"),
			FileName:   proto.String(fileInfo.Name()),
			Caption:    proto.String(caption),
			FileSHA256: uploaded.FileSHA256,
			FileLength: proto.Uint64(uploaded.FileLength),
			MediaKey:   uploaded.MediaKey,
			DirectPath: proto.String(uploaded.DirectPath),
		},
	}

	// Send the message
	ts := time.Now()
	_, err = wac.Client.SendMessage(context.Background(), recipientJID, msg)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: fmt.Sprintf("Document sent (server timestamp: %v)", ts),
	}, nil
}

// SendVideo sends a video to a contact or group
func (wac *WhatsAppClient) SendVideo(recipient string, filePath string, caption string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Parse recipient JID
	recipientJID, err := types.ParseJID(recipient)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Read the video file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Upload the video
	uploaded, err := wac.Client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Create the video message
	msg := &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			URL:        &uploaded.URL,
			Mimetype:   proto.String("video/mp4"),
			Caption:    proto.String(caption),
			FileSHA256: uploaded.FileSHA256,
			FileLength: proto.Uint64(uploaded.FileLength),
			MediaKey:   uploaded.MediaKey,
			DirectPath: proto.String(uploaded.DirectPath),
		},
	}

	// Send the message
	ts := time.Now()
	_, err = wac.Client.SendMessage(context.Background(), recipientJID, msg)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: fmt.Sprintf("Video sent (server timestamp: %v)", ts),
	}, nil
}

// SendAudio sends an audio file to a contact or group
func (wac *WhatsAppClient) SendAudio(recipient string, filePath string) (interface{}, error) {
	if !wac.Client.IsLoggedIn() {
		return SendResult{Success: false, Message: "Not logged in"}, fmt.Errorf("not logged in")
	}

	// Parse recipient JID
	recipientJID, err := types.ParseJID(recipient)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Read the audio file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Upload the audio
	uploaded, err := wac.Client.Upload(context.Background(), data, whatsmeow.MediaAudio)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	// Create the audio message
	msg := &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			URL:        &uploaded.URL,
			Mimetype:   proto.String("audio/mpeg"),
			FileSHA256: uploaded.FileSHA256,
			FileLength: proto.Uint64(uploaded.FileLength),
			MediaKey:   uploaded.MediaKey,
			DirectPath: proto.String(uploaded.DirectPath),
		},
	}

	// Send the message
	ts := time.Now()
	_, err = wac.Client.SendMessage(context.Background(), recipientJID, msg)
	if err != nil {
		return SendResult{Success: false, Message: err.Error()}, err
	}

	return SendResult{
		Success: true,
		Message: fmt.Sprintf("Audio sent (server timestamp: %v)", ts),
	}, nil
}
