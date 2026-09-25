package ortb_V2_5

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
)

func videoI32(v int32) *int32 { return &v }

func TestVideoProtoRoundTrip(t *testing.T) {
	want := &BidRequest{Imp: []*Imp{{Id: strPtr("1"), Video: &Video{
		Mimes: []string{"video/mp4", "video/webm"}, Minduration: videoI32(5), Maxduration: videoI32(30),
		Startdelay: videoI32(0), Protocols: []int32{2, 3, 5, 6}, W: videoI32(400), H: videoI32(300),
		Placement: videoI32(5), Plcmt: videoI32(3), Linearity: videoI32(1), Battr: []int32{8, 9},
		Minbitrate: videoI32(300), Maxbitrate: videoI32(9600), Boxingallowed: videoI32(1),
		Playbackmethod: []int32{2}, Pos: videoI32(2), Api: []int32{2, 7}, Skip: videoI32(1),
		Skipmin: videoI32(30), Skipafter: videoI32(30),
	}}}}
	encoded, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got BidRequest
	if err := proto.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.GetImp()) != 1 || got.GetImp()[0].GetVideo() == nil {
		t.Fatal("video disappeared after protobuf round trip")
	}
	if !reflect.DeepEqual(want.GetImp()[0].GetVideo().GetMimes(), got.GetImp()[0].GetVideo().GetMimes()) || got.GetImp()[0].GetVideo().GetPlcmt() != 3 || got.GetImp()[0].GetVideo().GetSkipafter() != 30 {
		t.Fatalf("VIDEO fields changed after protobuf round trip: %+v", got.GetImp()[0].GetVideo())
	}
}

func strPtr(v string) *string { return &v }
