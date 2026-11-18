package model

import (
	"fmt"
	"math"
	"testing"
)

func TestChannelProbeUsageSeparatesTestsFromCustomerBilling(t *testing.T) {
	newTestDB(t)
	image, err := StartChannelImageProbe(7, "image-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteChannelImageProbe(image.ID, "succeeded", 1); err != nil {
		t.Fatal(err)
	}
	if err := CompleteChannelImageProbe(image.ID, "succeeded", 9); err != nil {
		t.Fatal(err)
	}
	_, _ = StartChannelImageProbe(7, "image-model")
	_, _ = StartChannelImageProbe(8, "other-channel")
	for _, task := range []*Task{
		{ChannelID: 7, Platform: ArkVideoProbePlatform, Status: "succeeded", ResultMeta: `{"seconds":4,"usage":{"completion_tokens":40594}}`},
		{ChannelID: 7, Platform: ArkVideoProbePlatform, Status: "succeeded", ResultMeta: `{"seconds":4}`},
		{ChannelID: 7, Platform: ArkVideoProbePlatform, Status: "in_progress"},
		{ChannelID: 8, Platform: ArkVideoProbePlatform, Status: "succeeded", ResultMeta: `{"usage":{"completion_tokens":99}}`},
		{ChannelID: 7, Platform: "ark-video", Status: "succeeded", ResultMeta: `{"usage":{"completion_tokens":999}}`},
	} {
		if err := InsertTask(task); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		usage, err := GetChannelProbeUsage(t.Context(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if usage.ImageTests != 2 || usage.ImageSuccesses != 1 || usage.Images != 1 || usage.UncertainImages != 1 || usage.VideoTests != 3 || usage.VideoSuccesses != 2 || usage.VideoSeconds != 8 || usage.VideoOutputTokens != "40594" || usage.MissingVideoUsage != 1 || usage.PendingVideos != 1 {
			t.Fatalf("incorrect test usage: %+v", usage)
		}
	}
	var logs int64
	if err := DB.Model(&Log{}).Count(&logs).Error; err != nil || logs != 0 {
		t.Fatal("admin tests created customer usage logs")
	}
}

func TestChannelProbeUsageKeepsLargeTokenTotalsExact(t *testing.T) {
	newTestDB(t)
	for range 2 {
		if err := InsertTask(&Task{ChannelID: 1, Platform: ArkVideoProbePlatform, Status: "succeeded", ResultMeta: fmt.Sprintf(`{"usage":{"completion_tokens":%d}}`, int64(math.MaxInt64))}); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := GetChannelProbeUsage(t.Context(), 1)
	if err != nil || usage.VideoOutputTokens != "18446744073709551614" {
		t.Fatalf("token total lost precision: %+v, %v", usage, err)
	}
}
