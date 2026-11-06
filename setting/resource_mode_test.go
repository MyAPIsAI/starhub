package setting

import "testing"

func TestResourceProviderModeDefaults(t *testing.T) {
	ResetResourceProviderModeToDefault()
	if got := ResourceProviderModeValue(); got != ResourceProviderModeLocal {
		t.Fatalf("default mode = %s, want local", got)
	}
	if IsCommercialMode() {
		t.Fatal("should not be in commercial mode by default")
	}
}

func TestUpdateResourceProviderMode(t *testing.T) {
	ResetResourceProviderModeToDefault()
	defer ResetResourceProviderModeToDefault()

	if err := UpdateResourceProviderMode(OptionKeyResourceProviderMode, ResourceProviderModeCommercial); err != nil {
		t.Fatal(err)
	}
	if !IsCommercialMode() {
		t.Fatal("should be in commercial mode after update")
	}

	if err := UpdateResourceProviderMode(OptionKeyResourceProviderMode, ResourceProviderModeLocal); err != nil {
		t.Fatal(err)
	}
	if IsCommercialMode() {
		t.Fatal("should be in local mode after update")
	}
}

func TestUpdateResourceProviderModeInvalidValue(t *testing.T) {
	ResetResourceProviderModeToDefault()
	if err := UpdateResourceProviderMode(OptionKeyResourceProviderMode, "invalid"); err == nil {
		t.Fatal("invalid mode value should be rejected")
	}
}

func TestUpdateResourceProviderModeInvalidKey(t *testing.T) {
	if err := UpdateResourceProviderMode("wrong_key", "local"); err == nil {
		t.Fatal("wrong key should be rejected")
	}
}

func TestSnapshotResourceProviderMode(t *testing.T) {
	ResetResourceProviderModeToDefault()
	defer ResetResourceProviderModeToDefault()

	UpdateResourceProviderMode(OptionKeyResourceProviderMode, ResourceProviderModeCommercial)
	snapshot, err := SnapshotResourceProviderMode(OptionKeyResourceProviderMode)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != ResourceProviderModeCommercial {
		t.Fatalf("snapshot = %s, want commercial", snapshot)
	}
}
