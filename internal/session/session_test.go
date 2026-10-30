package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	state := State{
		Volume:     73,
		Repeat:     "all",
		Shuffle:    true,
		View:       "Road Trip",
		Theme:      "nord",
		Sort:       "album",
		Track:      filepath.Join(dir, "a.mp3"),
		PositionMS: 42000,
		Speed:      1.25,
		EQ:         []float64{6, 5, 3, 1, 0, 0, 0, 0, 0, 0},
		EQPreamp:   -3,
	}
	if err := Save(dir, state); err != nil {
		t.Fatal(err)
	}
	got := Load(dir)
	if !reflect.DeepEqual(got, state) {
		t.Fatalf("loaded %+v, want %+v", got, state)
	}
}

func TestDefaultsAndNormalization(t *testing.T) {
	dir := t.TempDir()
	got := Load(dir)
	def := Default()
	if !reflect.DeepEqual(got, def) {
		t.Fatalf("missing file: got %+v, want %+v", got, def)
	}

	state := State{
		Volume:     200,
		Repeat:     "sometimes",
		View:       "  ",
		PositionMS: -10,
		Speed:      1.24,
		EQ:         []float64{99, -99, 1},
		EQPreamp:   40,
	}
	if err := Save(dir, state); err != nil {
		t.Fatal(err)
	}
	got = Load(dir)
	if got.Volume != 100 || got.Repeat != "off" || got.View != "all" || got.PositionMS != 0 {
		t.Fatalf("state was not normalized: %+v", got)
	}
	if got.Speed != 1.25 {
		t.Fatalf("speed = %v, want 1.25", got.Speed)
	}
	if len(got.EQ) != eqBandCount || got.EQ[0] != maxEQGain || got.EQ[1] != minEQGain || got.EQ[2] != 1 {
		t.Fatalf("EQ was not normalized: %+v", got.EQ)
	}
	if got.EQPreamp != maxEQGain {
		t.Fatalf("preamp = %v, want %v", got.EQPreamp, maxEQGain)
	}

	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "session.json.bak"))
	if got := Load(dir); !reflect.DeepEqual(got, Default()) {
		t.Fatalf("invalid JSON should return defaults, got %+v", got)
	}
}

func TestLoadOldSessionWithoutEffects(t *testing.T) {
	dir := t.TempDir()
	old := `{"volume":55,"repeat":"all","view":"all","sort":"artist"}`
	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(dir)
	if got.Speed != 1 || got.EQPreamp != 0 || len(got.EQ) != eqBandCount {
		t.Fatalf("old session was not upgraded: %+v", got)
	}
	for _, gain := range got.EQ {
		if gain != 0 {
			t.Fatalf("old session EQ is not flat: %+v", got.EQ)
		}
	}
}
