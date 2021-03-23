package queues

import (
	"encoding/json"
	"strings"
	"time"

	cdata "github.com/pip-services3-go/pip-services3-commons-go/data"
)

/*
MessageEnvelope allows adding additional information to messages. A correlation id, message id, and a message type
are added to the data being sent/received. Additionally, a MessageEnvelope can reference a lock token.
Side note: a MessageEnvelope"s message is stored as a buffer, so strings are converted
using utf8 conversions.
*/
type MessageEnvelope struct {
	reference interface{}

	//The unique business transaction id that is used to trace calls across components.
	CorrelationId string `json:"correlation_id"`
	// The message"s auto-generated ID.
	MessageId string `json:"message_id"`
	// String value that defines the stored message"s type.
	MessageType string `json:"message_type"`
	// The time at which the message was sent.
	SentTime time.Time `json:"sent_time"`
	//The stored message.
	Message []byte `json:"message"`
}

// NewMessageEnvelope method are creates an empty MessageEnvelope
// Returns: *MessageEnvelope new instance
func NewEmptyMessageEnvelope() *MessageEnvelope {
	c := MessageEnvelope{}
	return &c
}

// NewMessageEnvelope method are creates a new MessageEnvelope, which adds a correlation id, message id, and a type to the
// data being sent/received.
//   - correlationId     (optional) transaction id to trace execution through call chain.
//   - messageType       a string value that defines the message"s type.
//   - message           the data being sent/received.
// Returns: *MessageEnvelope new instance
func NewMessageEnvelope(correlationId string, messageType string, message []byte) *MessageEnvelope {
	c := MessageEnvelope{}
	c.CorrelationId = correlationId
	c.MessageType = messageType
	c.MessageId = cdata.IdGenerator.NextLong()
	c.Message = message
	return &c
}

// GetReference method are returns the lock token that this MessageEnvelope references.
func (c *MessageEnvelope) GetReference() interface{} {
	return c.reference
}

// SetReference method are sets a lock token reference for this MessageEnvelope.
//   - value     the lock token to reference.
func (c *MessageEnvelope) SetReference(value interface{}) {
	c.reference = value
}

// GetMessageAsString method are returns the information stored in this message as a string.
func (c *MessageEnvelope) GetMessageAsString() string {
	return string(c.Message)
}

// SetMessageAsString method are stores the given string.
//   - value    the string to set. Will be converted to a bufferg.
func (c *MessageEnvelope) SetMessageAsString(value string) {
	c.Message = []byte(value)
}

// GetMessageAsJson method are returns the value that was stored in this message as a JSON string.
// See  SetMessageAsJson
func (c *MessageEnvelope) GetMessageAsJson() interface{} {
	if c.Message == nil {
		return nil
	}

	var result interface{}
	err := json.Unmarshal(c.Message, &result)
	if err != nil {
		return nil
	}

	return result
}

// SetMessageAsJson method are stores the given value as a JSON string.
//   - value     the value to convert to JSON and store in this message.
// See  GetMessageAsJson
func (c *MessageEnvelope) SetMessageAsJson(value interface{}) {
	if value == nil {
		c.Message = []byte{}
	} else {
		message, err := json.Marshal(value)
		if err == nil {
			c.Message = message
		}
	}
}

// String method are convert"s this MessageEnvelope to a string, using the following format:
// <correlation_id>,<MessageType>,<message.toString>
// If any of the values are nil, they will be replaced with ---.
// Returns the generated string.
func (c *MessageEnvelope) String() string {
	builder := strings.Builder{}
	builder.WriteString("[")
	if c.CorrelationId == "" {
		builder.WriteString("---")
	} else {
		builder.WriteString(c.CorrelationId)
	}
	builder.WriteString(",")
	if c.MessageType == "" {
		builder.WriteString("---")
	} else {
		builder.WriteString(c.MessageType)
	}
	builder.WriteString(",")
	if c.Message == nil {
		builder.WriteString("---")
	} else {
		builder.Write(c.Message)
	}
	builder.WriteString("]")
	return builder.String()
}
