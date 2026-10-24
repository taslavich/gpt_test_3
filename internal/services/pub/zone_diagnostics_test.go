package pub

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestPublisherZoneRefusalCodes(t *testing.T) {
	for _, status := range []int{403, 429, 502, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s, code := fixture(t)
			s.HTTP.Transport = transport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: http.NoBody}, nil
			})
			w := send(s, input(code))
			if w.Code != 502 {
				t.Fatal(w.Code, w.Body.String())
			}
			ctx := context.Background()
			ids, e := s.ORTB[0].SMembers(ctx, s.Config.ORTBReadySet).Result()
			if e != nil || len(ids) != 1 {
				t.Fatal(ids, e)
			}
			fields, e := s.ORTB[0].HGetAll(ctx, ids[0]).Result()
			if e != nil {
				t.Fatal(e)
			}
			if fields["CODE"] != fmt.Sprint(status) || fields["pub_user_id"] != code.UserID || fields["ad_code_id"] != code.ID || fields["zone_id"] == "" || fields["SITE_ID"] == "" {
				t.Fatal(fields)
			}
		})
	}
}
