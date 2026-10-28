package controller

import (
	"context"
	"fmt"
	"testing"

	"starhub/model"
)

func TestServiceLevelTargetRequestIsExplicitAndStrict(t *testing.T) {
	newTestDB(t)
	group := &model.ServiceGroupConfig{GroupID: "target-group", Name: "Target", Multiplier: "1"}
	if err := model.UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{}`, fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0}`, group.ID), fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0,"targets":{}}`, group.ID), fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0,"targets":{"success_rate_ppm":99.5}}`, group.ID), fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0,"targets":{"p95_latency_ms":0}}`, group.ID), fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0,"targets":{"p95_latency_ms":1,"unknown":true}}`, group.ID), fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0,"targets":null,"multiplier":"2"}`, group.ID)} {
		expectStatus(t, call(UpdateServiceGroupSLA, testRequest{method: "PUT", target: "/api/service-group/target-group/sla", path: map[string]string{"id": group.GroupID}, body: body}), 400)
	}
	stored, err := model.GetServiceGroup(group.GroupID)
	if err != nil || stored.SLARevision != 0 || stored.ServiceLevel().Targets != nil {
		t.Fatalf("invalid request changed target: %+v %v", stored, err)
	}
	body := fmt.Sprintf(`{"expected_group_id":%d,"expected_revision":0,"targets":{"success_rate_ppm":0,"p95_latency_ms":null}}`, group.ID)
	var result struct {
		SLA model.ServiceGroupSLA `json:"sla"`
	}
	decodeData(t, call(UpdateServiceGroupSLA, testRequest{method: "PUT", target: "/api/service-group/target-group/sla", path: map[string]string{"id": group.GroupID}, body: body}), &result)
	if result.SLA.Revision != 1 || result.SLA.Targets == nil || result.SLA.Targets.SuccessRatePPM == nil || *result.SLA.Targets.SuccessRatePPM != 0 || result.SLA.Targets.P95LatencyMS != nil {
		t.Fatalf("zero target lost: %+v", result)
	}
	if _, err := model.UpdateServiceGroupSLA(context.Background(), group.GroupID, group.ID, 0, nil); err == nil {
		t.Fatal("stale request accepted")
	}
}
