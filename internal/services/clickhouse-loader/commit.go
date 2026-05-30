package clickhouse_loader

import (
	"time"

	"github.com/segmentio/kafka-go"
)

func rememberCommitMessage(commitMap map[int]kafka.Message, msg kafka.Message) {
	prev, ok := commitMap[msg.Partition]
	if !ok || msg.Offset > prev.Offset {
		commitMap[msg.Partition] = msg
	}
}

func compactCommitMessages(commitMap map[int]kafka.Message) []kafka.Message {
	messages := make([]kafka.Message, 0, len(commitMap))
	for _, msg := range commitMap {
		messages = append(messages, msg)
	}
	return messages
}

func batchTimeout(timeoutSec int, timeoutMs int) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	if timeoutSec > 0 {
		return time.Duration(timeoutSec) * time.Second
	}
	return 800 * time.Millisecond
}
