package service

import (
	"os"
	"sync"

	"starhub/model"
)

var smsConfigurationMu sync.Mutex

// SMSManagedByEnvironment preserves the existing externally managed bridge.
func SMSManagedByEnvironment() bool {
	return os.Getenv("STARHUB_SMS_BRIDGE_URL") != ""
}

// ReloadSMSConfiguration reads and applies the latest committed configuration
// under one lock, so an older concurrent save cannot overwrite a newer sender.
func ReloadSMSConfiguration() error {
	smsConfigurationMu.Lock()
	defer smsConfigurationMu.Unlock()
	var provider SMSProvider
	if SMSManagedByEnvironment() {
		bridge, err := newMessageBridge(os.Getenv("STARHUB_SMS_BRIDGE_URL"), os.Getenv("STARHUB_SMS_BRIDGE_TOKEN"))
		if err != nil {
			return err
		}
		provider = bridge
	} else if model.DB != nil {
		value, err := model.GetSMSConfiguration()
		if err != nil {
			return err
		}
		provider, err = newAliyunSMSProvider(value)
		if err != nil {
			return err
		}
	}
	SetSMSProvider(provider)
	return nil
}
