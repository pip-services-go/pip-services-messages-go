package queues

import (
	"sync"
	"time"

	"github.com/pip-services3-go/pip-services3-components-go/auth"
	ccon "github.com/pip-services3-go/pip-services3-components-go/connect"
)

/*
Message queue that sends and receives messages within the same process by using shared memory.

This queue is typically used for testing to mock real queues.

 Configuration parameters

- name:                        name of the message queue

 References

- *:logger:*:*:1.0           (optional)  ILogger components to pass log messages
- *:counters:*:*:1.0         (optional)  ICounters components to pass collected measurements

See MessageQueue
See MessagingCapabilities

 Example

    let queue = new MessageQueue("myqueue");

    queue.send("123", new MessageEnvelop(null, "mymessage", "ABC"));

    queue.receive("123", (err, message) => {
        if (message != null) {
           ...
           queue.complete("123", message);
        }
    });
*/
type MemoryMessageQueue struct {
	MessageQueue
	messages          []MessageEnvelope
	lockTokenSequence int
	lockedMessages    map[int]*LockedMessage //lockedMessages { [id: number]: LockedMessage; } = {};
	opened            bool
	/* Used to stop the listening process. */
	cancel bool
}

/*
Creates a new instance of the message queue.

- name  (optional) a queue name.

See MessagingCapabilities
*/
func NewMemoryMessageQueue(name string) *MemoryMessageQueue {
	mmq := MemoryMessageQueue{}
	mmq.MessageQueue = *NewMessageQueue(name)

	mmq.MessageQueue.IMessageQueue = &mmq

	mmq.messages = make([]MessageEnvelope, 0)
	mmq.lockTokenSequence = 0
	mmq.lockedMessages = make(map[int]*LockedMessage, 0)
	mmq.opened = false
	mmq.cancel = false
	mmq.Capabilities = NewMessagingCapabilities(true, true, true, true, true, true, true, false, true)
	return &mmq
}

/*
Checks if the component is opened.

Return true if the component has been opened and false otherwise.
*/
func (c *MemoryMessageQueue) IsOpen() bool {
	return c.opened
}

/*
Opens the component with given connection and credential parameters.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
- connection        connection parameters
- credential        credential parameters
- callback 			callback function that receives error or null no errors occured.
*/
func (c *MemoryMessageQueue) OpenWithParams(correlationId string, connection *ccon.ConnectionParams, credential *auth.CredentialParams) (err error) {
	c.opened = true
	return nil
}

/*
Closes component and frees used resources.
 *
- correlationId 	(optional) transaction id to trace execution through call chain.
- callback 			callback function that receives error or null no errors occured.
*/
func (c *MemoryMessageQueue) Close(correlationId string) (err error) {
	c.opened = false
	c.cancel = true
	c.Logger.Trace(correlationId, "Closed queue %s", c)
	return nil
}

/*
Clears component state.
 *
- correlationId 	(optional) transaction id to trace execution through call chain.
- callback 			callback function that receives error or null no errors occured.
*/
func (c *MemoryMessageQueue) Clear(correlationId string) (err error) {
	c.messages = c.messages[:0]
	c.lockedMessages = make(map[int]*LockedMessage, 0)
	c.cancel = false
	return nil
}

/*
Reads the current number of messages in the queue to be delivered.
 *
- callback      callback function that receives number of messages or error.
*/
func (c *MemoryMessageQueue) ReadMessageCount() (count int64, err error) {
	count = (int64)(len(c.messages))
	return count, nil
}

/*
Sends a message into the queue.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
- envelope          a message envelop to be sent.
- callback          (optional) callback function that receives error or null for success.
*/
func (c *MemoryMessageQueue) Send(correlationId string, envelope *MessageEnvelope) (err error) {

	envelope.Sent_time = time.Now()
	// Add message to the queue
	c.messages = append(c.messages, *envelope)
	c.Counters.IncrementOne("queue." + c.GetName() + ".sentmessages")
	c.Logger.Debug(envelope.Correlation_id, "Sent message %s via %s", envelope.ToString(), c.ToString())
	return nil
}

/*
Peeks a single incoming message from the queue without removing it.
If there are no messages available in the queue it returns null.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
- callback          callback function that receives a message or error.
*/
func (c *MemoryMessageQueue) Peek(correlationId string) (result *MessageEnvelope, err error) {
	var message MessageEnvelope
	// Pick a message
	if len(c.messages) > 0 {
		message = c.messages[0]
		c.Logger.Trace(message.Correlation_id, "Peeked message %s on %s", message, c.ToString())
		return &message, nil
	}
	return nil, nil
}

/*
Peeks multiple incoming messages from the queue without removing them.
If there are no messages available in the queue it returns an empty list.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
- messageCount      a maximum number of messages to peek.
- callback          callback function that receives a list with messages or error.
*/
func (c *MemoryMessageQueue) PeekBatch(correlationId string, messageCount int64) (result []MessageEnvelope, err error) {

	var messages []MessageEnvelope = make([]MessageEnvelope, 0, 0)
	if messageCount <= (int64)(len(c.messages)) {
		messages = c.messages[0:messageCount]
	}
	c.Logger.Trace(correlationId, "Peeked %d messages on %s", len(messages), c.ToString())
	return messages, nil
}

/*
Receives an incoming message and removes it from the queue.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
- waitTimeout       a timeout in milliseconds to wait for a message to come.
- callback          callback function that receives a message or error.
*/
func (c *MemoryMessageQueue) Receive(correlationId string, waitTimeout time.Duration) (result *MessageEnvelope, err error) {
	err = nil
	var message *MessageEnvelope
	var messageReceived bool = false

	var checkIntervalMs time.Duration = 100 * time.Millisecond
	var i time.Duration = 0

	var wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		localWg := sync.WaitGroup{}

		for i < waitTimeout && !messageReceived {
			i = i + checkIntervalMs

			localWg.Add(1)
			time.AfterFunc(checkIntervalMs, func() {
				if len(c.messages) == 0 {
					localWg.Done()
					return
				}
				// Get message from the queue
				// shift queue
				var msg MessageEnvelope
				message = nil
				for len(c.messages) > 0 {
					msg, c.messages = c.messages[0], c.messages[1:]
					message = &msg
				}

				if message != nil {
					// Generate and set locked token
					lockedToken := c.lockTokenSequence
					c.lockTokenSequence++
					message.SetReference(lockedToken)

					// Add messages to locked messages list
					var lockedMessage LockedMessage = LockedMessage{}
					var now time.Time = time.Now()
					now = (now.Add(waitTimeout))
					lockedMessage.ExpirationTime = now
					lockedMessage.Message = message
					lockedMessage.Timeout = waitTimeout
					c.lockedMessages[lockedToken] = &lockedMessage

					messageReceived = true

					c.Counters.IncrementOne("queue." + c.GetName() + ".receivedmessages")
					c.Logger.Debug(message.Correlation_id, "Received message %s via %s", message, c.ToString())
				}
				localWg.Done()
			})

			localWg.Wait()
		}

		wg.Done()
	}()

	wg.Wait()

	return message, err
}

/*
Renews a lock on a message that makes it invisible from other receivers in the queue.
This method is usually used to extend the message processing time.
 *
- message       a message to extend its lock.
- lockTimeout   a locking timeout in milliseconds.
- callback      (optional) callback function that receives an error or null for success.
*/
func (c *MemoryMessageQueue) RenewLock(message *MessageEnvelope, lockTimeout time.Duration) (err error) {

	reference := message.GetReference()
	if reference == nil {
		return nil
	}
	// Get message from locked queue
	lockedToken, ok := reference.(int)
	if !ok {
		return nil
	}
	lockedMessage, ok := c.lockedMessages[lockedToken]
	// If lock is found, extend the lock
	if ok {
		var now time.Time = time.Now()
		// Todo: Shall we skip if the message already expired?
		if lockedMessage.ExpirationTime.Unix() > now.Unix() {
			now = now.Add(lockedMessage.Timeout)
			lockedMessage.ExpirationTime = now
		}
	}

	c.Logger.Trace(message.Correlation_id, "Renewed lock for message %s at %s", message, c.ToString())
	return nil
}

/*
Permanently removes a message from the queue.
This method is usually used to remove the message after successful processing.
 *
- message   a message to remove.
- callback  (optional) callback function that receives an error or null for success.
*/
func (c *MemoryMessageQueue) Complete(message *MessageEnvelope) (err error) {

	reference := message.GetReference()
	if reference == nil {
		return nil
	}

	lockKey, ok := reference.(int)
	if !ok {
		return nil
	}
	delete(c.lockedMessages, lockKey)
	message.SetReference(nil)
	c.Logger.Trace(message.Correlation_id, "Completed message %s at %s", message, c.ToString())
	return nil
}

/*
Returnes message into the queue and makes it available for all subscribers to receive it again.
This method is usually used to return a message which could not be processed at the moment
to repeat the attempt. Messages that cause unrecoverable errors shall be removed permanently
or/and send to dead letter queue.
 *
- message   a message to return.
- callback  (optional) callback function that receives an error or null for success.
*/
func (c *MemoryMessageQueue) Abandon(message *MessageEnvelope) (err error) {

	reference := message.GetReference()
	if reference == nil {
		return nil
	}

	// Get message from locked queue
	lockedToken, ok := reference.(int)
	if !ok {
		return nil
	}
	lockedMessage, ok := c.lockedMessages[lockedToken]
	if ok {
		// Remove from locked messages
		delete(c.lockedMessages, lockedToken)
		message.SetReference(nil)
		// Skip if it is already expired
		if lockedMessage.ExpirationTime.Unix() <= time.Now().Unix() {
			return nil
		}
	} else { // Skip if it absent
		return nil
	}
	c.Logger.Trace(message.Correlation_id, "Abandoned message %s at %s", message, c.ToString())
	return c.Send(message.Correlation_id, message)
}

/*
Permanently removes a message from the queue and sends it to dead letter queue.
 *
- message   a message to be removed.
- callback  (optional) callback function that receives an error or null for success.
*/
func (c *MemoryMessageQueue) MoveToDeadLetter(message *MessageEnvelope) (err error) {
	reference := message.GetReference()
	if reference == nil {
		return nil
	}

	lockedToken, ok := reference.(int)
	if !ok {
		return nil
	}

	delete(c.lockedMessages, lockedToken)
	message.SetReference(nil)
	c.Counters.IncrementOne("queue." + c.GetName() + ".deadmessages")
	c.Logger.Trace(message.Correlation_id, "Moved to dead message %s at %s", message, c.ToString())
	return nil
}

/*
Listens for incoming messages and blocks the current thread until queue is closed.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
- receiver          a receiver to receive incoming messages.
 *
See IMessageReceiver
See receive
*/
func (c *MemoryMessageQueue) Listen(correlationId string, receiver IMessageReceiver) {

	var timeoutInterval time.Duration = 1000 * time.Millisecond
	c.Logger.Trace("", "Started listening messages at %s", c.ToString())
	c.cancel = false

	go func() {
		for !c.cancel {

			var message *MessageEnvelope

			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				result, err := c.Receive(correlationId, timeoutInterval)
				message = result
				if err != nil {
					c.Logger.Error(correlationId, err, "Failed to receive the message")
				}
				wg.Done()
			}()
			wg.Wait()
			wg.Add(1)
			go func() {
				if message != nil && !c.cancel {
					err := receiver.ReceiveMessage(message, c)
					if err != nil {
						c.Logger.Error(correlationId, err, "Failed to process the message")
					}
					wg.Done()
				}
			}()
			wg.Wait()
			select {
			case <-time.After(timeoutInterval):
			}
		}

	}()
}

/*
Ends listening for incoming messages.
When c method is call listen unblocks the thread and execution continues.
 *
- correlationId     (optional) transaction id to trace execution through call chain.
*/
func (c *MemoryMessageQueue) EndListen(correlationId string) {
	c.cancel = true
}
