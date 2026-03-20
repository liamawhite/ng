package graph

import "sync"

type EntityType string

const (
	EntityTypeProject EntityType = "project"
	EntityTypeTask    EntityType = "task"
)

type Node struct {
	ID       string
	Type     EntityType
	Title    string
	Content  string
	Status   string // tasks only
	FilePath string
}

type Edge struct {
	SourceID  string
	Predicate string // "part_of"
	TargetID  string
}

type RelatedNode struct {
	Node      *Node
	Predicate string
	Direction string // "outgoing" | "incoming"
}

type Graph struct {
	mu       sync.RWMutex
	nodes    map[string]*Node
	outgoing map[string][]Edge // source ID → edges leaving that node
	incoming map[string][]Edge // target ID → edges pointing to that node
}

func New() *Graph {
	return &Graph{
		nodes:    make(map[string]*Node),
		outgoing: make(map[string][]Edge),
		incoming: make(map[string][]Edge),
	}
}

// UpsertNode adds or replaces a node.
func (g *Graph) UpsertNode(node *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[node.ID] = node
}

// DeleteNode removes a node and all its edges.
func (g *Graph) DeleteNode(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.nodes, id)

	// Remove outgoing edges from this node, and clean up incoming index.
	for _, e := range g.outgoing[id] {
		g.removeFromIncoming(e.TargetID, id)
	}
	delete(g.outgoing, id)

	// Remove incoming edges to this node, and clean up outgoing index.
	for _, e := range g.incoming[id] {
		g.removeFromOutgoing(e.SourceID, id)
	}
	delete(g.incoming, id)
}

// SetEdges replaces all outgoing edges for sourceID.
func (g *Graph) SetEdges(sourceID string, edges []Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove old incoming entries for edges we're replacing.
	for _, e := range g.outgoing[sourceID] {
		g.removeFromIncoming(e.TargetID, sourceID)
	}

	// Set new outgoing edges.
	g.outgoing[sourceID] = edges

	// Rebuild incoming entries.
	for _, e := range edges {
		g.incoming[e.TargetID] = append(g.incoming[e.TargetID], e)
	}
}

func (g *Graph) GetNode(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}

// GetOutgoingEdges returns the outgoing edges for a node.
func (g *Graph) GetOutgoingEdges(id string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	edges := g.outgoing[id]
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result
}

func (g *Graph) ListNodes(entityType EntityType) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []*Node
	for _, n := range g.nodes {
		if n.Type == entityType {
			result = append(result, n)
		}
	}
	return result
}

// ListRelated returns nodes related to id.
// predicate="" matches all predicates.
// direction="" matches both directions.
func (g *Graph) ListRelated(id, predicate, direction string) []*RelatedNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*RelatedNode

	if direction == "" || direction == "outgoing" {
		for _, e := range g.outgoing[id] {
			if predicate != "" && e.Predicate != predicate {
				continue
			}
			if n, ok := g.nodes[e.TargetID]; ok {
				result = append(result, &RelatedNode{
					Node:      n,
					Predicate: e.Predicate,
					Direction: "outgoing",
				})
			}
		}
	}

	if direction == "" || direction == "incoming" {
		for _, e := range g.incoming[id] {
			if predicate != "" && e.Predicate != predicate {
				continue
			}
			if n, ok := g.nodes[e.SourceID]; ok {
				result = append(result, &RelatedNode{
					Node:      n,
					Predicate: e.Predicate,
					Direction: "incoming",
				})
			}
		}
	}

	return result
}

// removeFromIncoming removes all edges sourced from sourceID in the incoming list for targetID.
func (g *Graph) removeFromIncoming(targetID, sourceID string) {
	edges := g.incoming[targetID]
	filtered := edges[:0]
	for _, e := range edges {
		if e.SourceID != sourceID {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		delete(g.incoming, targetID)
	} else {
		g.incoming[targetID] = filtered
	}
}

// removeFromOutgoing removes all edges targeting targetID in the outgoing list for sourceID.
func (g *Graph) removeFromOutgoing(sourceID, targetID string) {
	edges := g.outgoing[sourceID]
	filtered := edges[:0]
	for _, e := range edges {
		if e.TargetID != targetID {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		delete(g.outgoing, sourceID)
	} else {
		g.outgoing[sourceID] = filtered
	}
}
