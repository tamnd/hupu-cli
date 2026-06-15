package hupu

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (resolve), which need no network.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "hupu" {
		t.Errorf("Scheme = %q, want hupu", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "hupu" {
		t.Errorf("Identity.Binary = %q, want hupu", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"123456789", "post", "123456789"},
		{"https://" + Host + "/987654321.html", "post", "987654321"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyBad(t *testing.T) {
	_, _, err := Domain{}.Classify("not-a-tid")
	if err == nil {
		t.Error("Classify(bad) expected error, got nil")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("post", "123456789")
	want := baseURL + "/123456789.html"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateBadType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "123456789")
	if err == nil {
		t.Error("Locate(unknown) expected error, got nil")
	}
}

func TestResolveOn(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.ResolveOn("hupu", "987654321")
	if err != nil || got.String() != "hupu://post/987654321" {
		t.Errorf("ResolveOn = (%q, %v), want hupu://post/987654321", got.String(), err)
	}
}
