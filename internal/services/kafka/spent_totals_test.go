package kafka_service

import (
	"math"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func validSpentTotal() SpentTotalMessage {
	return SpentTotalMessage{EntityType: "user", EntityID: "42", SpentTotal: 12.34, SnapshotTS: time.Now().UTC().Format(time.RFC3339)}
}

func TestSpentTotalValidate(t *testing.T) {
	if err := validSpentTotal().Validate(); err != nil {
		t.Fatalf("valid message rejected: %v", err)
	}
	cases := map[string]SpentTotalMessage{
		"entity":    {EntityType: "account", EntityID: "42", SpentTotal: 1, SnapshotTS: time.Now().UTC().Format(time.RFC3339)},
		"empty_id":  {EntityType: "user", EntityID: " ", SpentTotal: 1, SnapshotTS: time.Now().UTC().Format(time.RFC3339)},
		"negative":  {EntityType: "user", EntityID: "42", SpentTotal: -1, SnapshotTS: time.Now().UTC().Format(time.RFC3339)},
		"nan":       {EntityType: "user", EntityID: "42", SpentTotal: math.NaN(), SnapshotTS: time.Now().UTC().Format(time.RFC3339)},
		"inf":       {EntityType: "user", EntityID: "42", SpentTotal: math.Inf(1), SnapshotTS: time.Now().UTC().Format(time.RFC3339)},
		"timestamp": {EntityType: "user", EntityID: "42", SpentTotal: 1, SnapshotTS: "yesterday"},
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := msg.Validate(); err == nil {
				t.Fatalf("invalid message accepted: %+v", msg)
			}
		})
	}
}

func TestSpentTotalsBalancerIsHash(t *testing.T) {
	if _, ok := spentTotalsBalancer().(*kafka.Hash); !ok {
		t.Fatalf("spent_totals writer must use kafka.Hash balancer")
	}
}

func TestNewKafkaWriterKeepsOtherWritersLeastBytes(t *testing.T) {
	w := newKafkaWriter([]string{"127.0.0.1:9092"}, "ortb", &kafka.LeastBytes{})
	if _, ok := w.Balancer.(*kafka.LeastBytes); !ok {
		t.Fatalf("non-financial writer balancer changed: %T", w.Balancer)
	}
}
