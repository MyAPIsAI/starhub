package model

import (
	"strings"
	"testing"
)

func TestProductionStorageBudgetsAreExactAndSurviveRepeatedInitialization(t *testing.T) {
	newTestDB(t)
	if err := initProductionStorage(DB); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&ProductionStorageState{}).Where("id=1").Updates(map[string]any{"max_rows": 2, "max_bytes": 8}).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []ProductionAttemptRecord{{ID: "one", Payload: "test"}, {ID: "two", Payload: "data"}} {
		if err := DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := DB.Create(&ProductionAttemptRecord{ID: "three", Payload: "x"}).Error; err == nil {
		t.Fatal("row budget ignored")
	}
	state, err := ReadProductionStorage()
	if err != nil || state.Rows != 2 || state.Bytes != 8 {
		t.Fatal(state, err)
	}
	if err := DB.Delete(&ProductionAttemptRecord{}, "id = ?", "one").Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&ProductionAttemptRecord{ID: "large", Payload: strings.Repeat("x", 5)}).Error; err == nil {
		t.Fatal("byte budget ignored")
	}
	if err := initProductionStorage(DB); err != nil {
		t.Fatal(err)
	}
	state, err = ReadProductionStorage()
	if err != nil || state.Rows != 1 || state.Bytes != 4 {
		t.Fatal("restart changed storage accounting", state, err)
	}
}
