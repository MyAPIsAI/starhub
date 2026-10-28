package controller

import (
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestPlazaAvailabilityRequiresConfiguredGroupButPreservesExplicitZero(t *testing.T) {
	newTestDB(t)
	if err := model.InsertChannel(&model.Channel{Name: "Coverage fixture", Type: 1, Models: "gpt-4o", Group: "default,vip,zero,missing", Status: common.ChannelStatusEnabled}); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateRatio(setting.OptionKeyGroupRatio, `{"vip":0.8,"zero":0}`); err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"default", "missing", "vip", "zero"} {
		rows, err := buildPlazaRows(group, group)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows: %v %v", rows, err)
		}
		row := rows[0]
		if !reflect.DeepEqual(row.AvailableGroups, []string{"vip", "zero"}) {
			t.Fatalf("unconfigured coverage advertised: %+v", row.AvailableGroups)
		}
		if group == "default" || group == "missing" {
			if row.IsAvailable || len(row.Protocols) != 0 || row.UnavailableReason == nil || *row.UnavailableReason != "group_not_configured" {
				t.Fatalf("unconfigured group advertised: %+v", row)
			}
		} else if !row.IsAvailable || len(row.Protocols) == 0 || row.UnavailableReason != nil {
			t.Fatalf("configured group hidden: %+v", row)
		}
	}
}
