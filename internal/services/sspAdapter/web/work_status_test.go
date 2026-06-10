package sppAdapterWeb

import "testing"

func TestWorkStatusControlsStreamsIndependently(t *testing.T) {
	status := NewWorkStatus(false, false)

	if err := status.Set(PostBid_POP_ADL_V_2_5_URL, true); err != nil {
		t.Fatalf("Set(%s) error = %v", PostBid_POP_ADL_V_2_5_URL, err)
	}

	popAdult, err := status.Get(PostBid_POP_ADL_V_2_5_URL)
	if err != nil {
		t.Fatalf("Get(%s) error = %v", PostBid_POP_ADL_V_2_5_URL, err)
	}
	banAdult, err := status.Get(PostBid_BAN_ADL_V_2_5_URL)
	if err != nil {
		t.Fatalf("Get(%s) error = %v", PostBid_BAN_ADL_V_2_5_URL, err)
	}

	if !popAdult {
		t.Fatalf("%s = false, want true", PostBid_POP_ADL_V_2_5_URL)
	}
	if banAdult {
		t.Fatalf("%s = true, want false", PostBid_BAN_ADL_V_2_5_URL)
	}
}

func TestWorkStatusSetAllSetsEveryStream(t *testing.T) {
	status := NewWorkStatus(false, false)
	status.SetAll(true)

	for _, stream := range []string{
		PostBid_POP_ADL_V_2_5_URL,
		PostBid_POP_MC_V_2_5_URL,
		PostBid_IPP_ADL_V_2_5_URL,
		PostBid_IPP_MC_V_2_5_URL,
		PostBid_BAN_ADL_V_2_5_URL,
		PostBid_BAN_MC_V_2_5_URL,
		PostBid_NAT_ADL_V_2_5_URL,
		PostBid_NAT_MC_V_2_5_URL,
	} {
		work, err := status.Get(stream)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", stream, err)
		}
		if !work {
			t.Fatalf("%s = false, want true", stream)
		}
	}
}
