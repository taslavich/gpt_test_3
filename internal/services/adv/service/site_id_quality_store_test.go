package auction

import (
	"os"
	"strings"
	"testing"
)

func validSiteIDQualityJSON() []byte {
	return []byte(`{
		"usual":{"isWhiteList":false,"siteIds":[]},
		"high":{"isWhiteList":true,"siteIds":["site-high"]},
		"ultra":{"isWhiteList":false,"siteIds":["site-bad"]}
	}`)
}

func TestSiteIDQualityStoreSemantics(t *testing.T) {
	path := t.TempDir() + "/site_id_quality.json"
	if err := os.WriteFile(path, validSiteIDQualityJSON(), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewSiteIDQualityStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Missing site.id always passes, regardless of whitelist/blacklist mode.
	for _, segment := range []string{"usual", "high", "ultra"} {
		if !store.Allows(segment, "") || !store.Allows(segment, "   ") {
			t.Fatalf("missing site.id must pass segment %s", segment)
		}
	}

	// Empty blacklist passes every present site.id.
	if !store.Allows("usual", "any-site") {
		t.Fatal("empty blacklist must pass present site.id")
	}

	// Whitelist permits listed IDs and rejects unlisted IDs.
	if !store.Allows("high", "site-high") {
		t.Fatal("listed whitelist site.id must pass")
	}
	if store.Allows("high", "site-other") {
		t.Fatal("unlisted whitelist site.id must be rejected")
	}

	// Blacklist rejects listed IDs and permits unlisted IDs.
	if store.Allows("ultra", "site-bad") {
		t.Fatal("listed blacklist site.id must be rejected")
	}
	if !store.Allows("ultra", "site-good") {
		t.Fatal("unlisted blacklist site.id must pass")
	}

	// Defensive fail-closed behavior for an unknown segment when site.id exists.
	if store.Allows("missing", "site-high") {
		t.Fatal("unknown quality segment with present site.id must fail closed")
	}
}

func TestSiteIDQualityStoreRequiresAllThreeNonNilSegments(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing ultra",
			body: `{"usual":{"isWhiteList":false,"siteIds":[]},"high":{"isWhiteList":false,"siteIds":[]}}`,
			want: `map "ultra" is missing`,
		},
		{
			name: "null high",
			body: `{"usual":{"isWhiteList":false,"siteIds":[]},"high":null,"ultra":{"isWhiteList":false,"siteIds":[]}}`,
			want: `map "high" must not be null`,
		},
		{
			name: "missing whitelist mode",
			body: `{"usual":{"siteIds":[]},"high":{"isWhiteList":false,"siteIds":[]},"ultra":{"isWhiteList":false,"siteIds":[]}}`,
			want: `missing required field "isWhiteList"`,
		},
		{
			name: "null site ids",
			body: `{"usual":{"isWhiteList":false,"siteIds":null},"high":{"isWhiteList":false,"siteIds":[]},"ultra":{"isWhiteList":false,"siteIds":[]}}`,
			want: `field "siteIds" must be an array, not null`,
		},
		{
			name: "unknown segment",
			body: `{"usual":{"isWhiteList":false,"siteIds":[]},"high":{"isWhiteList":false,"siteIds":[]},"ultra":{"isWhiteList":false,"siteIds":[]},"premium":{"isWhiteList":false,"siteIds":[]}}`,
			want: `invalid ADV site ID quality segment "premium"`,
		},
		{
			name: "unknown field",
			body: `{"usual":{"isWhiteList":false,"siteIds":[],"apply":true},"high":{"isWhiteList":false,"siteIds":[]},"ultra":{"isWhiteList":false,"siteIds":[]}}`,
			want: `contains unknown field "apply"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSiteIDQualityMaps([]byte(tc.body))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSiteIDQualityStoreInvalidUpdatePreservesFileAndRuntime(t *testing.T) {
	path := t.TempDir() + "/site_id_quality.json"
	initial := validSiteIDQualityJSON()
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewSiteIDQualityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Allows("high", "site-high") || store.Allows("high", "site-other") {
		t.Fatal("unexpected initial runtime state")
	}

	invalid := []byte(`{"usual":{"isWhiteList":false,"siteIds":[]},"high":{"isWhiteList":false,"siteIds":[]}}`)
	if err := store.UpdateJSON(invalid); err == nil {
		t.Fatal("invalid update must fail")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid update changed persisted file")
	}
	if !store.Allows("high", "site-high") || store.Allows("high", "site-other") {
		t.Fatal("invalid update changed runtime snapshot")
	}
}

func TestSiteIDQualityStoreEmptyWhitelistRejectsPresentSiteID(t *testing.T) {
	path := t.TempDir() + "/site_id_quality.json"
	body := []byte(`{
		"usual":{"isWhiteList":false,"siteIds":[]},
		"high":{"isWhiteList":true,"siteIds":[]},
		"ultra":{"isWhiteList":false,"siteIds":[]}
	}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewSiteIDQualityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Allows("high", "site-present") {
		t.Fatal("present site.id must fail an empty whitelist")
	}
	if !store.Allows("high", "") {
		t.Fatal("missing site.id must still pass an empty whitelist")
	}
}
