package queue

import "sync"

// Registry owns the set of per-session SessionQueues, keyed by userID+sessionID.
// It is the single place that knows the queue key format and how queues are
// created and looked up; callers never see a key or a sync.Map.
type Registry struct {
	queues sync.Map // key: userID+"|"+sessionID -> *SessionQueue
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// GetOrCreate returns the SessionQueue for (userID, sessionID), creating it
// with processor if it doesn't exist yet. processor is fixed at creation
// time and reused for every message queued against this session.
func (r *Registry) GetOrCreate(userID, sessionID string, processor Processor) *SessionQueue {
	key := userID + "|" + sessionID
	if v, ok := r.queues.Load(key); ok {
		return v.(*SessionQueue)
	}
	actual, _ := r.queues.LoadOrStore(key, NewSessionQueue(processor))
	return actual.(*SessionQueue)
}
