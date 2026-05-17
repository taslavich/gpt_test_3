package clickhouse_types

type UuidImpBidResponse = map[string]*Bid

type Bid struct {
	WinDspDomain *string  `json:"win_dsp_domain,omitempty"`
	WinPrice     *float32 `json:"win_price,omitempty"`
	WinDspPrice  *float32 `json:"win_dsp_price,omitempty"`
	WinCid       *string  `json:"win_cid,omitempty"`
	WinCrid      *string  `json:"win_crid,omitempty"`
	WinUserId    *string  `json:"win_user_id,omitempty"`
	WinFlag      *string  `json:"win_flag,omitempty"`
}

func GetEmpty(impIdUuid map[string]string) UuidImpBidResponse {
	bidResponse := make(UuidImpBidResponse)
	str := ""
	var flo float32 = 0
	srtFalse := "0"
	for _, uuid := range impIdUuid {
		bidResponse[uuid] = &Bid{
			&str,
			&flo,
			&flo,
			&str,
			&str,
			&str,
			&srtFalse,
		}
	}
	return bidResponse
}
