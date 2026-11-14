package model

import (
	"fmt"
	"testing"
)

func TestNetworkCapabilityFilterRejectsMalformedAndUnknownValues(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			if err := initDatabaseFunctions(db); err != nil {
				t.Fatal(err)
			}
			if err := migrateDatabaseSchema(db, &ResourceGatewayRoute{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Migrator().DropTable(&ResourceGatewayRoute{}) })
			for i, payload := range []string{`["text"]`, `[null]`, `["unknown"]`, `"text"`, `{}`, `[`, `[["text"]]`, `[true]`, `["TEXT"]`, `["text "]`, `["\u0000"]`} {
				row := ResourceGatewayRoute{ChannelID: 1, Model: fmt.Sprint(i), Group: "default", UpstreamModel: "test", Capabilities: payload, Protocols: `["OpenAI"]`}
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}
			var models []string
			if err := db.Model(&ResourceGatewayRoute{}).Where(resourceAdvancedSQL("resource_gateway_routes")).Pluck("model", &models).Error; err != nil || len(models) != 1 || models[0] != "0" {
				t.Fatal("invalid capability passed SQL prefilter", models, err)
			}
		})
	}
}
