// Package queue serializes concurrent messages to the same session and
// batches messages that arrive while a previous turn is still processing.
package queue

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const batchWindow = 10 * time.Second

// Processor executes one conversation turn for the given (possibly joined) content.
type Processor func(content string) (string, error)

type pendingMsg struct {
	content   string
	arrivedAt time.Time
	respCh    chan result
}

type result struct {
	response string
	err      error
}

// SessionQueue serializes processing for a single session: concurrent callers
// are queued, and any that arrive while a turn is in flight are joined into
// the next batch instead of racing on session state.
type SessionQueue struct {
	mu        sync.Mutex
	processor Processor
	working   bool
	pending   []*pendingMsg
}

func NewSessionQueue(processor Processor) *SessionQueue {
	return &SessionQueue{processor: processor}
}

// Enqueue submits content for processing and blocks until a response is ready.
// If the session is idle, processing starts immediately. If a turn is already
// in flight, this call's content joins the pending list and is delivered as
// part of the next batch once the current turn finishes.
func (q *SessionQueue) Enqueue(content string) (string, error) {
	msg := &pendingMsg{content: content, arrivedAt: time.Now(), respCh: make(chan result, 1)}

	q.mu.Lock()
	q.pending = append(q.pending, msg)
	if q.working {
		q.mu.Unlock()
		r := <-msg.respCh
		return r.response, r.err
	}
	q.working = true
	q.mu.Unlock()

	q.drain()

	r := <-msg.respCh
	return r.response, r.err
}

// drain processes pending messages in arrival-gap batches until none remain.
func (q *SessionQueue) drain() {
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.working = false
			q.mu.Unlock()
			return
		}
		batch := firstBatch(q.pending)
		q.pending = q.pending[len(batch):]
		q.mu.Unlock()

		q.processBatch(batch)
	}
}

// firstBatch returns the leading run of messages whose consecutive arrival
// gaps are all within batchWindow.
func firstBatch(pending []*pendingMsg) []*pendingMsg {
	end := 1
	for end < len(pending) && pending[end].arrivedAt.Sub(pending[end-1].arrivedAt) <= batchWindow {
		end++
	}
	return pending[:end]
}

func (q *SessionQueue) processBatch(batch []*pendingMsg) {
	contents := make([]string, len(batch))
	for i, m := range batch {
		contents[i] = m.content
	}

	response, err := q.runProcessor(strings.Join(contents, "\n\n"))

	for _, m := range batch {
		m.respCh <- result{response: response, err: err}
	}
}

// runProcessor calls the processor, recovering from panics so a single bad
// turn returns an error to its batch instead of wedging the queue for the
// rest of the session's lifetime.
func (q *SessionQueue) runProcessor(content string) (response string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in queue processor: %v", r)
		}
	}()
	return q.processor(content)
}
