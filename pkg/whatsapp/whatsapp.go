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
