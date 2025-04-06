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
