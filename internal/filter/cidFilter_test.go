package filter

/*import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func makeBidResponse(cids ...string) *ortb_V2_5.BidResponse {
	if len(cids) == 0 {
		return &ortb_V2_5.BidResponse{
			Seatbid: []*ortb_V2_5.SeatBid{},
		}
	}

	bids := make([]*ortb_V2_5.Bid, 0, len(cids))
	for _, c := range cids {
		cidCopy := c // создаём копию для указателя
		bids = append(bids, &ortb_V2_5.Bid{
			Cid: &cidCopy,
		})
	}

	seatbid := &ortb_V2_5.SeatBid{
		Bid: bids,
	}

	return &ortb_V2_5.BidResponse{
		Seatbid: []*ortb_V2_5.SeatBid{seatbid},
	}
}

func countBids(resp *ortb_V2_5.BidResponse) int {
	count := 0
	for _, sb := range resp.Seatbid {
		if sb == nil {
			continue
		}
		for _, b := range sb.Bid {
			if b != nil {
				count++
			}
		}
	}
	return count
}

func TestCidIdBox_Allowed(t *testing.T) {
	tests := []struct {
		name       string
		box        *CidIdBox
		inputResp  *ortb_V2_5.BidResponse
		wantReturn bool
		wantCount  int // ожидаемое количество Bid после фильтрации
	}{
		{
			name:       "nil box",
			box:        nil,
			inputResp:  makeBidResponse("c1", "c2"),
			wantReturn: true,
			wantCount:  2,
		},
		{
			name:       "apply false",
			box:        &CidIdBox{Apply: false, IsWhiteList: true, CidIds: map[string]bool{"c1": true}},
			inputResp:  makeBidResponse("c1", "c2"),
			wantReturn: true,
			wantCount:  2,
		},
		{
			name:       "nil bidResponse",
			box:        &CidIdBox{Apply: true, IsWhiteList: true, CidIds: map[string]bool{}},
			inputResp:  nil,
			wantReturn: false, // согласно коду: если bidResponse == nil, возвращаем true
			wantCount:  0,
		},
		{
			name:       "white list - allowed only c1",
			box:        &CidIdBox{Apply: true, IsWhiteList: true, CidIds: map[string]bool{"c1": true}},
			inputResp:  makeBidResponse("c1", "c2", "c3"),
			wantReturn: true, // остаётся c1
			wantCount:  1,
		},
		{
			name:       "white list - none allowed",
			box:        &CidIdBox{Apply: true, IsWhiteList: true, CidIds: map[string]bool{"c4": true}},
			inputResp:  makeBidResponse("c1", "c2"),
			wantReturn: false, // все удалены
			wantCount:  0,
		},
		{
			name:       "black list - block c1",
			box:        &CidIdBox{Apply: true, IsWhiteList: false, CidIds: map[string]bool{"c1": true}},
			inputResp:  makeBidResponse("c1", "c2", "c3"),
			wantReturn: true, // остаются c2, c3
			wantCount:  2,
		},
		{
			name:       "black list - block all",
			box:        &CidIdBox{Apply: true, IsWhiteList: false, CidIds: map[string]bool{"c1": true, "c2": true, "c3": true}},
			inputResp:  makeBidResponse("c1", "c2", "c3"),
			wantReturn: false,
			wantCount:  0,
		},
		{
			name:       "empty CidIds with white list - block all",
			box:        &CidIdBox{Apply: true, IsWhiteList: true, CidIds: map[string]bool{}},
			inputResp:  makeBidResponse("c1", "c2"),
			wantReturn: false,
			wantCount:  0,
		},
		{
			name:       "empty CidIds with black list - allow all",
			box:        &CidIdBox{Apply: true, IsWhiteList: false, CidIds: map[string]bool{}},
			inputResp:  makeBidResponse("c1", "c2"),
			wantReturn: true,
			wantCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.box.Allowed(tt.inputResp)
			if got != tt.wantReturn {
				t.Errorf("Allowed() returned = %v, want %v", got, tt.wantReturn)
			}
			if tt.inputResp != nil {
				cnt := countBids(tt.inputResp)
				if cnt != tt.wantCount {
					t.Errorf("after Allowed, number of bids = %d, want %d", cnt, tt.wantCount)
				}
			}
		})
	}
}
*/
