package playlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Names()) != 0 {
		t.Fatal("new store should be empty")
	}

	if _, err := store.Create("Road Trip"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("Focus"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("road trip"); err == nil {
		t.Fatal("duplicate playlist name should fail")
	}
	if _, err := store.Create("  "); err == nil {
		t.Fatal("empty name should fail")
	}

	trackA := filepath.Join(dir, "a.mp3")
	trackB := filepath.Join(dir, "b.mp3")
	if added, err := store.Add("Road Trip", trackA); err != nil || !added {
		t.Fatalf("add first: %v %v", added, err)
	}
	if added, err := store.Add("Road Trip", trackA); err != nil || added {
		t.Fatalf("add duplicate: %v %v", added, err)
	}
	if added, err := store.Add("Road Trip", trackB); err != nil || !added {
		t.Fatalf("add second: %v %v", added, err)
	}
	if added, err := store.Add("Missing", trackA); err == nil || added {
		t.Fatal("adding to a missing playlist should fail")
	}

	pl, ok := store.Get("road trip")
	if !ok || len(pl.Tracks) != 2 {
		t.Fatalf("playlist = %+v", pl)
	}

	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	pl2, ok := reloaded.Get("Road Trip")
	if !ok || len(pl2.Tracks) != 2 {
		t.Fatalf("reloaded playlist = %+v", pl2)
	}

	if removed, err := store.Remove("Road Trip", trackA); err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if removed, err := store.Remove("Road Trip", trackA); err != nil || removed {
		t.Fatalf("remove missing: %v %v", removed, err)
	}
	if changed, err := store.RemovePathEverywhere(trackB); err != nil || !changed {
		t.Fatalf("remove everywhere: %v %v", changed, err)
	}
	pl, _ = store.Get("Road Trip")
	if len(pl.Tracks) != 0 {
		t.Fatalf("playlist should be empty: %+v", pl)
	}

	if deleted, err := store.Delete("Focus"); err != nil || !deleted {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	if _, ok := store.Get("Focus"); ok {
		t.Fatal("playlist still present after delete")
	}
}

func TestBatchPlaylistOps(t *testing.T) {
	dir := t.TempDir()
	store, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Source", "Target"} {
		if _, err := store.Create(name); err != nil {
			t.Fatal(err)
		}
	}

	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")
	c := filepath.Join(dir, "c.mp3")

	added, skipped, err := store.AddMany("Source", []string{a, a, b})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || skipped != 1 {
		t.Fatalf("AddMany = %d added, %d skipped; want 2, 1", added, skipped)
	}
	if !store.Contains("Source", a) || store.Contains("Target", a) {
		t.Fatal("Contains returned the wrong result")
	}
	if _, err := store.Add("Target", c); err != nil {
		t.Fatal(err)
	}

	added, moved, skipped, err := store.MoveMany("Source", "Target", []string{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || moved != 2 || skipped != 1 {
		t.Fatalf("MoveMany = %d added, %d moved, %d skipped; want 2, 2, 1", added, moved, skipped)
	}
	source, _ := store.Get("Source")
	if len(source.Tracks) != 0 {
		t.Fatalf("source still has tracks: %+v", source.Tracks)
	}
	target, _ := store.Get("Target")
	if len(target.Tracks) != 3 {
		t.Fatalf("target tracks = %+v", target.Tracks)
	}

	removed, err := store.RemoveMany("Target", []string{a, c})
	if err != nil || removed != 2 {
		t.Fatalf("RemoveMany = %d, %v; want 2, nil", removed, err)
	}
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	target, _ = reloaded.Get("Target")
	if len(target.Tracks) != 1 || target.Tracks[0] != b {
		t.Fatalf("persisted target = %+v", target.Tracks)
	}
}

func TestCleanMissing(t *testing.T) {
	dir := t.TempDir()
	store, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("Mix"); err != nil {
		t.Fatal(err)
	}
	present := filepath.Join(dir, "present.mp3")
	missing := filepath.Join(dir, "missing.mp3")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("Mix", present); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("Mix", missing); err != nil {
		t.Fatal(err)
	}
	removed, err := store.CleanMissing()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	pl, _ := store.Get("Mix")
	if len(pl.Tracks) != 1 || pl.Tracks[0] != present {
		t.Fatalf("unexpected tracks: %+v", pl.Tracks)
	}
}
