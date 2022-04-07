package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/redis/taskqueue"
	"github.com/cschleiden/go-workflows/internal/core"
	"github.com/cschleiden/go-workflows/internal/history"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
)

func (rb *redisBackend) CreateWorkflowInstance(ctx context.Context, event history.WorkflowEvent) error {
	if err := createInstance(ctx, rb.rdb, event.WorkflowInstance, false); err != nil {
		return err
	}

	// Create event stream
	eventData, err := json.Marshal(event.HistoryEvent)
	if err != nil {
		return err
	}

	_, err = rb.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: pendingEventsKey(event.WorkflowInstance.InstanceID),
		ID:     "*",
		Values: map[string]interface{}{
			"event": string(eventData),
		},
	}).Result()
	if err != nil {
		return errors.Wrap(err, "could not create event stream")
	}

	// Queue workflow instance task
	if _, err := rb.workflowQueue.Enqueue(ctx, event.WorkflowInstance.InstanceID, nil); err != nil {
		if err != taskqueue.ErrTaskAlreadyInQueue {
			return errors.Wrap(err, "could not queue workflow task")
		}
	}

	return nil
}

func (rb *redisBackend) GetWorkflowInstanceHistory(ctx context.Context, instance *core.WorkflowInstance) ([]history.Event, error) {
	msgs, err := rb.rdb.XRange(ctx, historyKey(instance.InstanceID), "-", "+").Result()
	if err != nil {
		return nil, err
	}

	var events []history.Event
	for _, msg := range msgs {
		var event history.Event
		if err := json.Unmarshal([]byte(msg.Values["event"].(string)), &event); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal event")
		}

		events = append(events, event)
	}

	return events, nil
}

func (rb *redisBackend) GetWorkflowInstanceState(ctx context.Context, instance *core.WorkflowInstance) (backend.WorkflowState, error) {
	instanceState, err := readInstance(ctx, rb.rdb, instance.InstanceID)
	if err != nil {
		return backend.WorkflowStateActive, err
	}

	return instanceState.State, nil
}

func (rb *redisBackend) CancelWorkflowInstance(ctx context.Context, instance *core.WorkflowInstance) error {
	panic("unimplemented")
}

type instanceState struct {
	Instance    *core.WorkflowInstance `json:"instance,omitempty"`
	State       backend.WorkflowState  `json:"state,omitempty"`
	CreatedAt   time.Time              `json:"created_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

func createInstance(ctx context.Context, rdb redis.UniversalClient, instance *core.WorkflowInstance, ignoreDuplicate bool) error {
	key := instanceKey(instance.InstanceID)

	b, err := json.Marshal(&instanceState{
		Instance:  instance,
		State:     backend.WorkflowStateActive,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return errors.Wrap(err, "could not marshal instance state")
	}

	ok, err := rdb.SetNX(ctx, key, string(b), 0).Result()
	if err != nil {
		return errors.Wrap(err, "could not store instance")
	}

	if !ignoreDuplicate && !ok {
		return errors.New("workflow instance already exists")
	}

	return nil
}

func updateInstance(ctx context.Context, rdb redis.UniversalClient, instanceID string, state *instanceState) error {
	key := instanceKey(instanceID)

	b, err := json.Marshal(state)
	if err != nil {
		return errors.Wrap(err, "could not marshal instance state")
	}

	cmd := rdb.Set(ctx, key, string(b), 0)
	if err := cmd.Err(); err != nil {
		return errors.Wrap(err, "could not update instance")
	}

	return nil
}

func readInstance(ctx context.Context, rdb redis.UniversalClient, instanceID string) (*instanceState, error) {
	key := instanceKey(instanceID)
	cmd := rdb.Get(ctx, key)

	if err := cmd.Err(); err != nil {
		return nil, errors.Wrap(err, "could not read instance")
	}

	var state instanceState
	if err := json.Unmarshal([]byte(cmd.Val()), &state); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal instance state")
	}

	return &state, nil
}
