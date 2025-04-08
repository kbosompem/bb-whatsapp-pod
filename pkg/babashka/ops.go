package babashka

import (
	"bufio"
	"fmt"
	"os"

	"github.com/jackpal/bencode-go"
)

func debug(v interface{}) {
	fmt.Fprintf(os.Stderr, "debug: %+q\n", v)
}

type Message struct {
	Op   string
	Id   string
	Args string
	Var  string
}

type Namespace struct {
	Name string "name"
	Vars []Var  "vars"
}

type Var struct {
	Name string "name"
	Code string `bencode:"code,omitempty"`
}

type DescribeResponse struct {
	Format     string      "format"
	Namespaces []Namespace "namespaces"
}

// Add new operations for group functionality
var whatsappNamespace = Namespace{
	Name: "pod.whatsapp",
	Vars: []Var{
		{Name: "login", Code: "Login"},
		{Name: "logout", Code: "Logout"},
		{Name: "status", Code: "Status"},
		{Name: "send-message", Code: "SendMessage"},
		{Name: "get-groups", Code: "GetGroups"},
		{Name: "send-group-message", Code: "SendGroupMessage"},
		{Name: "upload", Code: "Upload"},
		{Name: "send-image", Code: "SendImage"},
		{Name: "send-document", Code: "SendDocument"},
		{Name: "send-video", Code: "SendVideo"},
		{Name: "send-audio", Code: "SendAudio"},
		{Name: "get-contact-info", Code: "GetContactInfo"},
		{Name: "get-profile-picture", Code: "GetProfilePicture"},
		{Name: "set-status", Code: "SetStatus"},
		{Name: "get-status", Code: "GetStatus"},
		{Name: "set-presence", Code: "SetPresence"},
		{Name: "subscribe-presence", Code: "SubscribePresence"},
		{Name: "get-chat-history", Code: "GetChatHistory"},
		{Name: "get-unread-messages", Code: "GetUnreadMessages"},
		{Name: "mark-message-as-read", Code: "MarkMessageAsRead"},
		{Name: "delete-message", Code: "DeleteMessage"},
		{Name: "create-group", Code: "CreateGroup"},
		{Name: "leave-group", Code: "LeaveGroup"},
		{Name: "get-group-invite-link", Code: "GetGroupInviteLink"},
		{Name: "join-group-with-link", Code: "JoinGroupWithLink"},
		{Name: "set-group-name", Code: "SetGroupName"},
		{Name: "set-group-topic", Code: "SetGroupTopic"},
		{Name: "add-group-participants", Code: "AddGroupParticipants"},
		{Name: "remove-group-participants", Code: "RemoveGroupParticipants"},
		{Name: "promote-group-participants", Code: "PromoteGroupParticipants"},
		{Name: "demote-group-participants", Code: "DemoteGroupParticipants"},
	},
}

type InvokeResponse struct {
	Id     string   "id"
	Value  string   "value" // stringified json response
	Status []string "status"
}

type ErrorResponse struct {
	Id        string   "id"
	Status    []string "status"
	ExMessage string   "ex-message"
	ExData    string   "ex-data,omitempty"
}

func ReadMessage() (*Message, error) {
	reader := bufio.NewReader(os.Stdin)
	message := &Message{}
	if err := bencode.Unmarshal(reader, &message); err != nil {
		return nil, err
	}

	return message, nil
}

func WriteDescribeResponse(describeResponse *DescribeResponse) error {
	return writeResponse(*describeResponse)
}

func WriteInvokeResponse(inputMessage *Message, value string) error {
	response := InvokeResponse{Id: inputMessage.Id, Status: []string{"done"}, Value: value}

	return writeResponse(response)
}

func WriteErrorResponse(inputMessage *Message, err error) error {
	errorMessage := string(err.Error())
	errorResponse := ErrorResponse{
		Id:        inputMessage.Id,
		Status:    []string{"done", "error"},
		ExMessage: errorMessage,
	}
	return writeResponse(errorResponse)
}

func writeResponse(response interface{}) error {
	writer := bufio.NewWriter(os.Stdout)
	if err := bencode.Marshal(writer, response); err != nil {
		return err
	}

	return writer.Flush() // Ensure flush returns error
}
