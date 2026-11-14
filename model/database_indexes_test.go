package model

import "testing"

func TestNetworkNonemptyUniqueOwnershipIndex(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			if err := db.AutoMigrate(&ResourceGatewayManagedChannel{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Migrator().DropTable(&ResourceGatewayManagedChannel{}) })
			for range 2 {
				if err := createNonemptyUniqueIndex(db, &ResourceGatewayManagedChannel{}, "resource_gateway_managed_channels", "idx_resource_gateway_source_binding", "source_channel_id"); err != nil {
					t.Fatal(err)
				}
			}
			for _, row := range []ResourceGatewayManagedChannel{{ChannelID: 1}, {ChannelID: 2}, {ChannelID: 3, SourceChannelID: "source"}} {
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&ResourceGatewayManagedChannel{ChannelID: 4, SourceChannelID: "source"}).Error; err == nil {
				t.Fatal("duplicate nonempty ownership key accepted")
			}
		})
	}
}
