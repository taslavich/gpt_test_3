package pubuuid

import (
	"fmt"
	"github.com/google/uuid"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"strings"
	"testing"
	"time"
)

func TestSignatureBindings(t *testing.T) {
	secret := strings.Repeat("x", 32)
	id := uuid.NewString()
	stamp := fmt.Sprint(time.Now().Unix())
	impID := "1"
	reqID := "req"
	r := &ortb.BidRequest{Id: &reqID, Imp: []*ortb.Imp{{Id: &impID}}}
	sig := Sign(secret, id, "feed", stamp, r)
	if e := Validate(secret, id, "feed", stamp, sig, r, time.Now()); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct{ id, feed, stamp, sig string }{
		{"broken", "feed", stamp, sig}, {strings.ToUpper(id), "feed", stamp, sig}, {id, "other", stamp, sig}, {id, "feed", "1", sig}, {id, "feed", stamp, "00"},
	} {
		if e := Validate(secret, test.id, test.feed, test.stamp, test.sig, r, time.Now()); e == nil {
			t.Fatal(test)
		}
	}
	changedID := "another-request"
	r.Id = &changedID
	if e := Validate(secret, id, "feed", stamp, sig, r, time.Now()); e == nil {
		t.Fatal("modified body accepted")
	}
	r.Id = &reqID
	r.Imp = append(r.Imp, &ortb.Imp{Id: &impID})
	if e := Validate(secret, id, "feed", stamp, Sign(secret, id, "feed", stamp, r), r, time.Now()); e == nil {
		t.Fatal("two imps accepted")
	}
	if FeedAllowed("adult,mainstream", "evil") || !FeedAllowed("adult, mainstream", "mainstream") {
		t.Fatal("feed list")
	}
}
