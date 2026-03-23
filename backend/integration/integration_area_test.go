package integration_test

import (
	"testing"

	api "github.com/liamawhite/ng/api/golang"
)

// TestAreaCRUD_API verifies the full create/get/update/delete lifecycle for areas.
func TestAreaCRUD_API(t *testing.T) {
	e := newEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "work"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if area.Id == "" {
		t.Fatal("Create returned empty ID")
	}
	if area.Title != "work" {
		t.Fatalf("Create: got title=%q, want work", area.Title)
	}
	if !fileExists(e.dir, area.Id) {
		t.Fatal("Create: file not on disk")
	}

	got, err := e.areas.Get(bg, &api.GetAreaRequest{Id: area.Id})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Id != area.Id || got.Title != area.Title {
		t.Fatalf("Get: id=%q title=%q, want id=%q title=%q", got.Id, got.Title, area.Id, area.Title)
	}

	list, err := e.areas.List(bg, &api.ListAreasRequest{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Areas) != 1 || list.Areas[0].Id != area.Id {
		t.Fatalf("List: got %d areas, want 1 with id=%q", len(list.Areas), area.Id)
	}

	_, err = e.areas.Update(bg, &api.UpdateAreaRequest{Id: area.Id, Title: "personal"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = e.areas.Get(bg, &api.GetAreaRequest{Id: area.Id})
	if got.Title != "personal" {
		t.Fatalf("Get after Update: title=%q, want personal", got.Title)
	}

	_, err = e.areas.Delete(bg, &api.DeleteAreaRequest{Id: area.Id})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = e.areas.Get(bg, &api.GetAreaRequest{Id: area.Id})
	if !isNotFound(err) {
		t.Fatalf("Get after Delete: expected NotFound, got %v", err)
	}
	if fileExists(e.dir, area.Id) {
		t.Fatal("Delete: file still on disk")
	}
}

// TestAreaInGraph verifies that projects referencing an area appear in ListRelated.
func TestAreaInGraph(t *testing.T) {
	e := newEnv(t)

	area, err := e.areas.Create(bg, &api.CreateAreaRequest{Title: "work"})
	if err != nil {
		t.Fatalf("Create area: %v", err)
	}

	proj, err := e.projects.Create(bg, &api.CreateProjectRequest{
		Title:  "Work Project",
		AreaId: area.Id,
	})
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	if proj.AreaId != area.Id {
		t.Fatalf("project.AreaId=%q, want %q", proj.AreaId, area.Id)
	}

	// Projects in area appear as incoming in_area edges on the area node.
	related, err := e.graph.ListRelated(bg, &api.ListRelatedRequest{
		Id:        area.Id,
		Predicate: api.Predicate_PREDICATE_IN_AREA,
		Direction: api.Direction_DIRECTION_INCOMING,
	})
	if err != nil {
		t.Fatalf("ListRelated: %v", err)
	}
	if len(related.Entities) != 1 || related.Entities[0].Entity.GetProject().GetId() != proj.Id {
		t.Fatalf("ListRelated: expected project %q in area's incoming edges, got %d entities", proj.Id, len(related.Entities))
	}
}
