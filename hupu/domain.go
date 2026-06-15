package hupu

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes hupu as a kit Domain: a driver that a multi-domain
// host enables with a single blank import,
//
//	import _ "github.com/tamnd/hupu-cli/hupu"
//
// The init below registers it; the host then dereferences hupu:// URIs by
// routing to the operations Register installs.
func init() { kit.Register(Domain{}) }

// Domain is the hupu kit driver. It carries no state.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "hupu",
		Hosts:  []string{"bbs.hupu.com"},
		Identity: kit.Identity{
			Binary: "hupu",
			Short:  "A command line for Hupu sports forum.",
			Long: `A command line for Hupu sports forum.

hupu reads public Hupu BBS data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.`,
			Site: "bbs.hupu.com",
			Repo: "https://github.com/tamnd/hupu-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newKitClient)

	// hot: list the current hot threads from the BBS home page.
	kit.Handle(app, kit.OpMeta{
		Name:    "hot",
		Group:   "read",
		List:    true,
		Summary: "List hot BBS posts",
		URIType: "post",
	}, listHot)

	// search: full-text search via the mobile API.
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		List:    true,
		Summary: "Search BBS posts",
		URIType: "post",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, listSearch)
}

// newKitClient builds the client from the kit-resolved config.
func newKitClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type hotInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Query  string  `kit:"arg" help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listHot(ctx context.Context, in hotInput, emit func(*Post) error) error {
	posts, err := in.Client.Hot(ctx, in.Limit)
	if err != nil {
		return err
	}
	for i := range posts {
		if err := emit(&posts[i]); err != nil {
			return err
		}
	}
	return nil
}

func listSearch(ctx context.Context, in searchInput, emit func(*Post) error) error {
	if strings.TrimSpace(in.Query) == "" {
		return errs.Usage("query is required")
	}
	posts, err := in.Client.Search(ctx, in.Query, in.Limit)
	if err != nil {
		return err
	}
	for i := range posts {
		if err := emit(&posts[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, no network ---

// tidRE matches a numeric thread id in a URL path segment or bare string.
var tidRE = regexp.MustCompile(`\b(\d+)\b`)

// Classify turns a post URL or bare thread ID into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = postID(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized hupu reference: %q", input)
	}
	return "post", id, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "post" {
		return "", errs.Usage("hupu has no resource type %q", uriType)
	}
	return fmt.Sprintf("https://bbs.hupu.com/%s.html", id), nil
}

// postID extracts a numeric thread id from a full URL or a bare id string.
func postID(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		if m := tidRE.FindStringSubmatch(u.Path); m != nil {
			return m[1]
		}
		return ""
	}
	if m := tidRE.FindStringSubmatch(input); m != nil {
		return m[1]
	}
	return ""
}
