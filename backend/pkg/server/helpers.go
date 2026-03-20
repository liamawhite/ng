package server

import (
	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
)

func partOfEdges(sourceID, targetID string) []graph.Edge {
	if targetID == "" {
		return nil
	}
	return []graph.Edge{{SourceID: sourceID, Predicate: "part_of", TargetID: targetID}}
}

func taskStatusToString(s api.TaskStatus) string {
	switch s {
	case api.TaskStatus_TASK_STATUS_TODO:
		return "todo"
	case api.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return "in_progress"
	case api.TaskStatus_TASK_STATUS_DONE:
		return "done"
	default:
		return "todo"
	}
}

func stringToTaskStatus(s string) api.TaskStatus {
	switch s {
	case "todo":
		return api.TaskStatus_TASK_STATUS_TODO
	case "in_progress":
		return api.TaskStatus_TASK_STATUS_IN_PROGRESS
	case "done":
		return api.TaskStatus_TASK_STATUS_DONE
	default:
		return api.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func protoPredicateToString(p api.Predicate) string {
	switch p {
	case api.Predicate_PREDICATE_PART_OF:
		return "part_of"
	default:
		return ""
	}
}

func protoDirectionToString(d api.Direction) string {
	switch d {
	case api.Direction_DIRECTION_OUTGOING:
		return "outgoing"
	case api.Direction_DIRECTION_INCOMING:
		return "incoming"
	default:
		return ""
	}
}

func stringToPredicate(s string) api.Predicate {
	switch s {
	case "part_of":
		return api.Predicate_PREDICATE_PART_OF
	default:
		return api.Predicate_PREDICATE_UNSPECIFIED
	}
}

func stringToDirection(s string) api.Direction {
	switch s {
	case "outgoing":
		return api.Direction_DIRECTION_OUTGOING
	case "incoming":
		return api.Direction_DIRECTION_INCOMING
	default:
		return api.Direction_DIRECTION_UNSPECIFIED
	}
}
