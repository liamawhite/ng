package server

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/liamawhite/ng/api/golang"
	"github.com/liamawhite/ng/backend/pkg/graph"
	"github.com/liamawhite/ng/backend/pkg/store"
)

func timeToTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func completedAtForStatus(statusStr string, existing *time.Time) *time.Time {
	completed := []string{"completed", "done"}
	for _, s := range completed {
		if statusStr == s {
			if existing != nil {
				return existing
			}
			now := time.Now()
			return &now
		}
	}
	return nil
}

// addChildEdge appends a parent→child edge (with the given predicate) to the
// parent node's edge list and persists both the file and the graph.
func addChildEdge(s *store.FileStore, parentID, childID, predicate string) error {
	parentNode, ok := s.Graph().GetNode(parentID)
	if !ok {
		return fmt.Errorf("parent node %q not found", parentID)
	}
	edges := s.Graph().GetOutgoingEdges(parentID)
	edges = append(edges, graph.Edge{SourceID: parentID, Predicate: predicate, TargetID: childID})
	return s.Update(parentNode, edges)
}

// removeChildEdge removes all edges targeting childID from the parent node's
// edge list and persists both the file and the graph.
func removeChildEdge(s *store.FileStore, parentID, childID string) error {
	parentNode, ok := s.Graph().GetNode(parentID)
	if !ok {
		return fmt.Errorf("parent node %q not found", parentID)
	}
	edges := s.Graph().GetOutgoingEdges(parentID)
	filtered := make([]graph.Edge, 0, len(edges))
	for _, e := range edges {
		if e.TargetID != childID {
			filtered = append(filtered, e)
		}
	}
	return s.Update(parentNode, filtered)
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

func priorityToInt(p api.Priority) int32 {
	switch p {
	case api.Priority_PRIORITY_1:
		return 1
	case api.Priority_PRIORITY_2:
		return 2
	case api.Priority_PRIORITY_3:
		return 3
	case api.Priority_PRIORITY_4:
		return 4
	case api.Priority_PRIORITY_5:
		return 5
	default:
		return 4 // default priority
	}
}

func intToPriority(n int32) api.Priority {
	switch n {
	case 1:
		return api.Priority_PRIORITY_1
	case 2:
		return api.Priority_PRIORITY_2
	case 3:
		return api.Priority_PRIORITY_3
	case 4:
		return api.Priority_PRIORITY_4
	case 5:
		return api.Priority_PRIORITY_5
	default:
		return api.Priority_PRIORITY_4 // default priority
	}
}

func effortUnitToString(u api.EffortUnit) string {
	switch u {
	case api.EffortUnit_EFFORT_UNIT_DAYS:
		return "days"
	case api.EffortUnit_EFFORT_UNIT_WEEKS:
		return "weeks"
	case api.EffortUnit_EFFORT_UNIT_MONTHS:
		return "months"
	default:
		return ""
	}
}

func stringToEffortUnit(s string) api.EffortUnit {
	switch s {
	case "days":
		return api.EffortUnit_EFFORT_UNIT_DAYS
	case "weeks":
		return api.EffortUnit_EFFORT_UNIT_WEEKS
	case "months":
		return api.EffortUnit_EFFORT_UNIT_MONTHS
	default:
		return api.EffortUnit_EFFORT_UNIT_UNSPECIFIED
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
	case api.TaskStatus_TASK_STATUS_BLOCKED:
		return "blocked"
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
	case "blocked":
		return api.TaskStatus_TASK_STATUS_BLOCKED
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
	case api.Predicate_PREDICATE_TASK:
		return "task"
	case api.Predicate_PREDICATE_SUBTASK:
		return "subtask"
	case api.Predicate_PREDICATE_SUBPROJECT:
		return "subproject"
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
	case "task":
		return api.Predicate_PREDICATE_TASK
	case "subtask":
		return api.Predicate_PREDICATE_SUBTASK
	case "subproject":
		return api.Predicate_PREDICATE_SUBPROJECT
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
