# RTB Publisher system alerts

Set BOT_BASE_URL and BOT_INTERNAL_SECRET in the active pub.env or
systemd environment to the same values as the other RTB services. Restart
pub. It uses the existing bot endpoint POST /internal/messages/send
with X-Bot-Secret. The bot routes it through TEXT_MESSAGES_CHAT_ID
(«Уведомления системы»).

Startup and listener failures send a single best-effort notice (2-second
timeout). Runtime failures send a digest per instance: first promptly, then
at most once per minute. Failed sends retain counts in memory for the next
attempt. A process restart clears pending counts; detailed errors remain in
the service logs. The HTTP request path never waits for the bot.

The digest covers Redis checks/configuration and storage errors, RTB failures,
overload, reservation/confirmation errors, infrastructure/runtime failures,
and telemetry outbox errors. Normal no-fill, client refusals and the expected
absence of expired delivery keys do not generate alerts. Digests include
the host and counts by category, without visitor data, request URLs or
upstream response bodies.
