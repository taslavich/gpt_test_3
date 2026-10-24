package pub

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"strings"
	"time"
)

type pageSession struct {
	Code    string `json:"code"`
	Visitor string `json:"visitor_id"`
	Session string `json:"page_session_id"`
	Expires int64  `json:"expires"`
}

func sessionToken(secret, code, visitor, session string) string {
	v := pageSession{Code: code, Visitor: visitor, Session: session, Expires: time.Now().Add(24 * time.Hour).Unix()}
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b) + "." + c.Sign(secret, v)
}
func validSession(secret string, in Request) error {
	parts := strings.Split(in.SessionToken, ".")
	if len(parts) != 2 {
		return errors.New("invalid_page_session")
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return errors.New("invalid_page_session")
	}
	var v pageSession
	if json.Unmarshal(b, &v) != nil || !c.Verify(secret, v, parts[1]) || v.Code != in.Code || v.Visitor != in.VisitorID || v.Session != in.SessionID || v.Expires < time.Now().Unix() || v.Expires > time.Now().Add(24*time.Hour).Unix()+5 {
		return errors.New("expired_or_invalid_page_session")
	}
	return nil
}
