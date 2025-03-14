package command

import (
	"github.com/benbjohnson/clock"
	"github.com/cschleiden/go-workflows/internal/core"
	"github.com/cschleiden/go-workflows/internal/history"
	"github.com/cschleiden/go-workflows/internal/payload"
)

type ScheduleActivityCommand struct {
	command

	Name     string
	Inputs   []payload.Payload
	Metadata *core.WorkflowMetadata
}

var _ Command = (*ScheduleActivityCommand)(nil)

func NewScheduleActivityCommand(id int64, name string, inputs []payload.Payload, metadata *core.WorkflowMetadata) *ScheduleActivityCommand {
	return &ScheduleActivityCommand{
		command: command{
			id:    id,
			name:  "ScheduleActivity",
			state: CommandState_Pending,
		},
		Name:     name,
		Inputs:   inputs,
		Metadata: metadata,
	}
}

func (c *ScheduleActivityCommand) Execute(clock clock.Clock) *CommandResult {
	switch c.state {
	case CommandState_Pending:
		c.state = CommandState_Committed

		event := history.NewPendingEvent(
			clock.Now(),
			history.EventType_ActivityScheduled,
			&history.ActivityScheduledAttributes{
				Name:     c.Name,
				Inputs:   c.Inputs,
				Metadata: c.Metadata,
			},
			history.ScheduleEventID(c.id))

		return &CommandResult{
			Events:         []*history.Event{event},
			ActivityEvents: []*history.Event{event},
		}
	}

	return nil
}
