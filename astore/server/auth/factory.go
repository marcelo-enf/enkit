package auth

import (
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"math/rand"
	"strings"
	"time"

	"github.com/enfabrica/enkit/astore/common"
	"github.com/enfabrica/enkit/lib/kflags"
	"golang.org/x/crypto/nacl/box"
)

type Flags struct {
	TimeLimit  time.Duration
	AuthURL    string
	Principals string
	CA         []byte
}

func DefaultFlags() *Flags {
	return &Flags{
		TimeLimit: time.Minute * 30,
	}
}

func (f *Flags) Register(set kflags.FlagSet, prefix string) *Flags {
	set.DurationVar(&f.TimeLimit, prefix+"time-limit", f.TimeLimit, "How long to wait at most for the user to complete authentication, before freeing resources")
	set.StringVar(&f.Principals, prefix+"principals", f.Principals, "Authorized ssh users which the ability to auth, in a comma separated string e.g. \"john,root,admin,smith\"")
	set.ByteFileVar(&f.CA, prefix+"ca", "/etc/ssh/ca.pem", "Path to the certificate authority private file")
	return f
}

func WithFlags(f *Flags) Modifier {
	return func(s *Server) error {
		if err := WithTimeLimit(f.TimeLimit)(s); err != nil {
			return err
		}
		if err := WithAuthURL(f.AuthURL)(s); err != nil {
			return err
		}
		if err := WithCA(f.CA)(s); err != nil {
			return err
		}
		if err := WithPrincipals(f.Principals)(s); err != nil {
			return err
		}
		if s.authURL == "" || s.authURL == "/" {
			return fmt.Errorf("an auth-url must be supplied using the --auth-url parameter")
		}
		return nil
	}
}

type Modifier func(*Server) error

// WithAuthURL supplies the URL to send users to for authentication.
// It is the URL of a running astore server.
func WithAuthURL(url string) Modifier {
	return func(s *Server) error {
		s.authURL = url
		return nil
	}
}

func WithTimeLimit(limit time.Duration) Modifier {
	return func(s *Server) error {
		s.limit = limit
		return nil
	}
}

func WithCA(fileContent []byte) Modifier {
	return func(server *Server) error {
		signer, err := ssh.ParsePrivateKey(fileContent)
		if err != nil {
			return err
		}
		server.caSigner = signer
		server.marshalledCAPublicKey = ssh.MarshalAuthorizedKey(signer.PublicKey())
		return nil
	}
}

func WithPrincipals(raw string) Modifier {
	return func(server *Server) error {
		splitString := strings.Split(raw, ",")
		if len(splitString) == 0 || (len(splitString) == 1 && splitString[0] == "") {
			return errors.New("there cannot be 0 principals in the auth server")
		}
		server.principals = splitString
		return nil
	}
}

func New(rng *rand.Rand, mods ...Modifier) (*Server, error) {
	pub, priv, err := box.GenerateKey(rng)
	if err != nil {
		return nil, err
	}

	s := &Server{
		rng:        rng,
		serverPub:  (*common.Key)(pub),
		serverPriv: (*common.Key)(priv),
		jars:       map[common.Key]*Jar{},
		limit:      30 * time.Minute,
	}

	for _, m := range mods {
		if err := m(s); err != nil {
			return nil, err
		}
	}

	s.authURL = strings.TrimSuffix(s.authURL, "/")
	if s.authURL == "" {
		return nil, fmt.Errorf("API usage error - an authentication URL must be set")
	}

	return s, nil
}
