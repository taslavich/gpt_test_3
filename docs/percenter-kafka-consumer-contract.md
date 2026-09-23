# `percenter_observability` Kafka consumer contract

The producer in this repository provides **at-least-once physical Kafka delivery**. It does not provide exactly-once broker delivery and the final consumer/storage is not implemented in this repository.

Every Kafka message uses the observability `event_id` as both payload identity and Kafka key. A production consumer MUST implement the equivalent of `KafkaObservabilityAtomicApplier.ApplyEventAndDedupeAtomically(eventID, payload)` with one atomic storage transaction/primitive that:

1. checks the durable dedupe identity for `event_id`;
2. applies the logical history/counter mutation only when that identity has not already committed;
3. commits the mutation and dedupe marker together;
4. on crash leaves either both committed or neither committed.

A permanent `SET NX`/claim before a non-transactional mutation is not sufficient because a crash between claim and mutation can lose the event. Concurrent consumers, replay after crash and Kafka rebalance must all yield one logical mutation per `event_id`.

`ApplyKafkaObservabilityMessageAtomically` validates Kafka-key/payload identity and accepts only the single atomic consumer-storage interface above. There is intentionally no repository helper shaped as `Exists(event_id) -> Apply(payload)` and no helper named as if producer-side delivery were idempotent/exactly-once. Adding a split check/apply abstraction is a contract violation and should be rejected during integration/code review.
