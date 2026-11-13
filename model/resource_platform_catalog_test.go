package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPlatformCatalogDeltaDeletesRemovedModelsAndRenamedChannels(t *testing.T) {
	newTestDB(t)
	ch := &Channel{Name: "old", Key: "secret-upstream-key", Models: "model-a,model-b", Group: "default", Status: 1}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	first, err := ReadPlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(first)
	if strings.Contains(string(encoded), ch.Key) {
		t.Fatal("catalog exposed credential")
	}
	ch.Name, ch.Models = "new", "model-a"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	next, delta, err := PlatformCatalogChanges(first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision <= first.Revision || len(delta) != 2 || delta[0].ChangeType != "delete" || delta[0].Channel.Name != "old" || len(delta[0].Channel.Models) != 2 || delta[1].ChangeType != "upsert" || len(delta[1].Channel.Models) != 1 {
		t.Fatalf("bad delta: %+v", delta)
	}
	stable, empty, err := PlatformCatalogChanges(next.Revision)
	if err != nil || len(empty) != 0 || stable.Revision != next.Revision {
		t.Fatalf("unstable revision: %v", err)
	}
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	_, delta, err = PlatformCatalogChanges(next.Revision)
	if err != nil || len(delta) != 1 || delta[0].ChangeType != "delete" {
		t.Fatalf("deletion missing: %+v %v", delta, err)
	}
	if _, _, err := PlatformCatalogChanges(0); !errors.Is(err, ErrCatalogRevisionExpired) {
		t.Fatalf("expected full sync fallback: %v", err)
	}
}
