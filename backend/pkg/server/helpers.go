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

func inAreaEdges(sourceID, areaID string) []graph.Edge {
	if areaID == "" {
		return nil
	}
	return []graph.Edge{{SourceID: sourceID, Predicate: "in_area", TargetID: areaID}}
}

func projectStatusToString(s api.ProjectStatus) string {
	switch s {
	case api.ProjectStatus_PROJECT_STATUS_ACTIVE:
		return "active"
	case api.ProjectStatus_PROJECT_STATUS_BACKLOG:
		return "backlog"
	case api.ProjectStatus_PROJECT_STATUS_BLOCKED:
		return "blocked"
	case api.ProjectStatus_PROJECT_STATUS_COMPLETED:
		return "completed"
	case api.ProjectStatus_PROJECT_STATUS_ABANDONED:
		return "abandoned"
	default:
		return ""
	}
}

func stringToProjectStatus(s string) api.ProjectStatus {
	switch s {
	case "active":
		return api.ProjectStatus_PROJECT_STATUS_ACTIVE
	case "backlog":
		return api.ProjectStatus_PROJECT_STATUS_BACKLOG
	case "blocked":
		return api.ProjectStatus_PROJECT_STATUS_BLOCKED
	case "completed":
		return api.ProjectStatus_PROJECT_STATUS_COMPLETED
	case "abandoned":
		return api.ProjectStatus_PROJECT_STATUS_ABANDONED
	default:
		return api.ProjectStatus_PROJECT_STATUS_UNSPECIFIED
	}
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
	case api.Predicate_PREDICATE_IN_AREA:
		return "in_area"
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
	case "in_area":
		return api.Predicate_PREDICATE_IN_AREA
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
