package percenter

const QUERY = `
	SELECT
		spp_domain AS domain,
		geo_column AS geo,
		toStartOfMinute(updated_at) as ts5,
		sum(
		arraySum(
			arrayMap(
				bid -> ifNull(JSONExtractFloat(bid, 'price'), 0.0),
				arrayFlatten(
					arrayMap(
						seat -> JSONExtractArrayRaw(JSONExtractRaw(seat, 'bid')),
						JSONExtractArrayRaw(JSONExtractRaw(bid_response_winner, 'Seatbid'))
					)
				)
			)
		)
		)/1000 AS total_price,
		sum(
		arraySum(
			arrayMap(
				bid -> ifNull(JSONExtractFloat(bid, 'dsp_price'), 0.0),
				arrayFlatten(
					arrayMap(
						seat -> JSONExtractArrayRaw(JSONExtractRaw(seat, 'bid')),
						JSONExtractArrayRaw(JSONExtractRaw(bid_response_winner, 'Seatbid'))
					)
				)
			)
		)
		)/1000 AS total_dsp_price,
		total_dsp_price - total_price as frofit
		FROM rtb.stat_new
		WHERE updated_at >= now() - INTERVAL 11 MINUTE AND updated_at < now() - INTERVAL 1 MINUTE
			AND uuid in (SELECT DISTINCT(uuid) FROM stat_new
							WHERE success == true
							and updated_at >= now() - INTERVAL 11 MINUTE AND updated_at < now() - INTERVAL 1 MINUTE)
		GROUP BY domain, geo, ts5
	`
