package controller

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestServiceGroupNonFiniteMultiplierNeverMutatesState(t *testing.T) {
	for _, multiplier := range []string{"NaN", "+Inf", "-Inf", "Infinity", "-Infinity", "1e999"} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing=%t", multiplier, existing), func(t *testing.T) {
				newTestDB(t)
				admin := newUser(t, "finite-admin", common.RoleAdmin, 1000)
				if existing {
					response := call(UpsertServiceGroup, testRequest{method: http.MethodPut,
						target: "/api/service-group", userID: admin.ID, role: common.RoleAdmin,
						body: `{"group_id":"finite","name":"Original","multiplier":"0.75","enabled":true}`})
					if response.Code != http.StatusOK {
						t.Fatalf("setup group: %d %s", response.Code, response.Body)
					}
				}
				groups, err := model.ListServiceGroups()
				if err != nil {
					t.Fatal(err)
				}
				options, err := model.AllOptions()
				if err != nil {
					t.Fatal(err)
				}
				changes, err := model.ListOptionChanges(100)
				if err != nil {
					t.Fatal(err)
				}
				version, err := model.OptionVersion()
				if err != nil {
					t.Fatal(err)
				}
				ratio, present := setting.GroupRatioValue("finite")
				response := call(UpsertServiceGroup, testRequest{method: http.MethodPut,
					target: "/api/service-group", userID: admin.ID, role: common.RoleAdmin,
					body: fmt.Sprintf(`{"group_id":"finite","name":"Must not persist","multiplier":%q,"enabled":true}`, multiplier)})
				expectError(t, response, http.StatusBadRequest, "multiplier")
				afterGroups, err := model.ListServiceGroups()
				if err != nil {
					t.Fatal(err)
				}
				afterOptions, err := model.AllOptions()
				if err != nil {
					t.Fatal(err)
				}
				afterChanges, err := model.ListOptionChanges(100)
				if err != nil {
					t.Fatal(err)
				}
				afterVersion, err := model.OptionVersion()
				if err != nil {
					t.Fatal(err)
				}
				afterRatio, afterPresent := setting.GroupRatioValue("finite")
				if !reflect.DeepEqual(groups, afterGroups) || !reflect.DeepEqual(options, afterOptions) ||
					!reflect.DeepEqual(changes, afterChanges) || version != afterVersion || ratio != afterRatio || present != afterPresent {
					t.Fatal("invalid multiplier changed metadata, stored options, history, version or serving ratio")
				}
			})
		}
	}
}
