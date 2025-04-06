package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/kbosompem/bb-whatsapp-pod/pkg/babashka" // Import the helper package
	"github.com/kbosompem/bb-whatsapp-pod/pkg/whatsapp"
)

var waClient *whatsapp.WhatsAppClient // Initialize lazily
var initErr error                     // Store potential init error

// setupLogging redirects standard log output to a file
func setupLogging() {
	logFile, err := os.OpenFile("pod.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// If we can't open the log file, log to stderr (which babashka might ignore or handle differently)
		log.SetOutput(os.Stderr)
		log.Printf("Error opening log file pod.log: %v", err)
		log.Println("Logging to stderr instead.")
		return
	}
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lshortfile) // Keep existing log format
	log.Println("--- Pod Started ---")
}

func main() {
	setupLogging()

	log.Println("Pod started. WhatsApp client will be initialized on first invoke.")

	log.Println("Starting read loop...")
	for {
		msg, err := babashka.ReadMessage()
		if err != nil {
			if err == io.EOF {
				log.Println("Received EOF from stdin, exiting.")
				if waClient != nil {
					waClient.Disconnect()
				}
				os.Exit(0)
			}
			// Log error, but difficult to report back to Babashka if ReadMessage failed
			log.Printf("ERROR reading message: %v", err)
			os.Exit(1) // Exit if we can't read messages
		}

		log.Printf("Received message. Op: %s, ID: %s, Var: %s", msg.Op, msg.Id, msg.Var)

		switch msg.Op {
		case "describe":
			log.Println("Handling describe op...")
			describeResp := handleDescribe()
			err = babashka.WriteDescribeResponse(describeResp)
			if err != nil {
				log.Printf("ERROR writing describe response: %v", err)
			}
		case "invoke":
			log.Println("Handling invoke op...")
			value, invokeErrMsg := handleInvoke(*msg) // Pass msg by value if needed or keep pointer
			if invokeErrMsg != "" {
				log.Printf("Invoke error: %s", invokeErrMsg)
				err = babashka.WriteErrorResponse(msg, fmt.Errorf(invokeErrMsg)) // Pass original msg and error
				if err != nil {
					log.Printf("ERROR writing error response: %v", err)
				}
			} else {
				log.Printf("Invoke success. Value: %s", value)
				err = babashka.WriteInvokeResponse(msg, value)
				if err != nil {
					log.Printf("ERROR writing invoke response: %v", err)
				}
			}
		case "shutdown":
			log.Println("Received shutdown op. Cleaning up and exiting...")
			if waClient != nil {
				waClient.Disconnect()
			}
			// Pod protocol doesn't require a response for shutdown, just exit cleanly.
			os.Exit(0)
		default:
			errMsg := fmt.Sprintf("Unknown operation: %s", msg.Op)
			log.Printf("Unknown op received: %s", msg.Op)
			err = babashka.WriteErrorResponse(msg, fmt.Errorf(errMsg))
			if err != nil {
				log.Printf("ERROR writing unknown op error response: %v", err)
			}
		}
	}
}

// handleDescribe now returns *babashka.DescribeResponse
func handleDescribe() *babashka.DescribeResponse {
	return &babashka.DescribeResponse{
		Format: "json", // Values passed in invoke args/results are JSON
		Namespaces: []babashka.Namespace{
			{
				Name: "pod.whatsapp",
				Vars: []babashka.Var{
					{Name: "login"}, // ArgLists not directly supported by babashka helper struct
					{Name: "logout"},
					{Name: "status"},
					{Name: "send-message"},
				},
			},
		},
	}
}

// handleInvoke takes babashka.Message, returns JSON string value and error message
func handleInvoke(msg babashka.Message) (value string, errMsg string) {
	log.Printf("Handling invoke for var: %s", msg.Var)
	parts := strings.SplitN(msg.Var, "/", 2)
	if len(parts) != 2 {
		errMsg = fmt.Sprintf("Invalid var format: %s", msg.Var)
		log.Printf("Error in handleInvoke: %s", errMsg)
		return "", errMsg
	}
	// namespace := parts[0] // Assuming single namespace
	funcName := parts[1]

	log.Printf("Parsed function name: %s", funcName)

	// Get the client instance (initializes on first call)
	client, clientErr := getWaClient()
	if clientErr != nil {
		errMsg = fmt.Sprintf("Failed to initialize WhatsApp client: %v", clientErr)
		log.Printf("Error in handleInvoke (getClient): %s", errMsg)
		return "", errMsg
	}
	if client == nil {
		errMsg = "WhatsApp client is not available after initialization attempt."
		log.Printf("Error in handleInvoke: %s", errMsg)
		return "", errMsg
	}

	log.Printf("Raw args string (should be JSON): %s", msg.Args)

	// Parse arguments JSON string from msg.Args into a slice of interface{}
	var args []interface{}
	if msg.Args != "" && msg.Args != "null" {
		errUnmarshal := json.Unmarshal([]byte(msg.Args), &args)
		if errUnmarshal != nil {
			errMsg = fmt.Sprintf("Error unmarshaling invoke args JSON: %v", errUnmarshal)
			log.Printf("Error in handleInvoke: %s", errMsg)
			return "", errMsg
		}
		log.Printf("Parsed JSON args: %+v", args)
	} else {
		log.Println("No arguments provided.")
	}

	var result interface{}
	var invokeErr error

	switch funcName {
	case "login":
		log.Println("Calling client.Login()...")
		result, invokeErr = client.Login()
	case "logout":
		log.Println("Calling client.Logout()...")
		result, invokeErr = client.Logout()
	case "status":
		log.Println("Calling client.Status()...")
		result, invokeErr = client.Status()
	case "send-message":
		log.Println("Handling send-message...")
		if len(args) != 2 {
			invokeErr = fmt.Errorf("send-message expects 2 arguments (phone-number, message), got %d", len(args))
		} else {
			phone, okPhone := args[0].(string)
			message, okMsg := args[1].(string)
			if !okPhone || !okMsg {
				invokeErr = fmt.Errorf("send-message arguments must be strings")
			} else {
				log.Printf("Calling client.SendMessage(%s, ...)", phone)
				result, invokeErr = client.SendMessage(phone, message)
			}
		}
	default:
		invokeErr = fmt.Errorf("Unknown function: %s", funcName)
	}

	if invokeErr != nil {
		errMsg = invokeErr.Error()
		log.Printf("Error invoking function '%s': %s", funcName, errMsg)
		return "", errMsg
	}

	log.Printf("Function '%s' executed successfully. Result: %+v", funcName, result)

	// Marshal the result back to a JSON string for the 'Value' field in the invoke response
	resultBytes, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		errMsg = fmt.Sprintf("Error marshaling result to JSON: %v", marshalErr)
		log.Printf("Error in handleInvoke after execution: %s", errMsg)
		return "", errMsg
	}

	log.Printf("Successfully marshaled result for '%s'.", funcName)
	return string(resultBytes), ""
}

// getWaClient remains the same
func getWaClient() (*whatsapp.WhatsAppClient, error) {
	if waClient == nil && initErr == nil { // Only initialize if nil and no previous error
		log.Println("Initializing WhatsApp client for the first time...")
		dbPath := "whatsapp.db"
		waClient, initErr = whatsapp.NewClient(dbPath)
		if initErr != nil {
			log.Printf("FATAL: Error initializing WhatsApp client: %v", initErr)
			// Keep initErr set so we don't retry
		} else {
			log.Println("WhatsApp client initialized successfully.")
		}
	}
	return waClient, initErr
}
