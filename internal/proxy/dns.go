package proxy

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// DNSPort is the fixed local port the shared DNS container publishes on
// 127.0.0.1. A high port is used so nothing needs elevated privileges; the OS
// resolver configuration points at it explicitly.
const DNSPort = 53535

// dnsTTL is deliberately short so teardown or a domain change propagates
// quickly to resolvers and browsers. It is baked into the CoreDNS Corefile.
const dnsTTL = 5

// maxDNSMessageSize is the classic UDP DNS message limit; our queries and
// single-record answers fit comfortably.
const maxDNSMessageSize = 512

// queryDNS sends a single question to the DNS server at addr and returns the
// parsed response. It is used by `proxy verify` to probe the shared DNS
// container directly, bypassing the OS resolver.
func queryDNS(ctx context.Context, addr, name string, qtype dnsmessage.Type, timeout time.Duration) (dnsmessage.Message, error) {
	var id [2]byte
	_, _ = rand.Read(id[:])
	queryID := binary.BigEndian.Uint16(id[:])

	dnsName, err := dnsmessage.NewName(name + ".")
	if err != nil {
		return dnsmessage.Message{}, fmt.Errorf("invalid DNS name %q: %w", name, err)
	}

	builder := dnsmessage.NewBuilder(make([]byte, 0, maxDNSMessageSize), dnsmessage.Header{ID: queryID, RecursionDesired: true})
	if err := builder.StartQuestions(); err != nil {
		return dnsmessage.Message{}, err
	}
	if err := builder.Question(dnsmessage.Question{Name: dnsName, Type: qtype, Class: dnsmessage.ClassINET}); err != nil {
		return dnsmessage.Message{}, err
	}

	query, err := builder.Finish()
	if err != nil {
		return dnsmessage.Message{}, err
	}

	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "udp", addr)
	if err != nil {
		return dnsmessage.Message{}, err
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return dnsmessage.Message{}, err
	}

	if _, err := conn.Write(query); err != nil {
		return dnsmessage.Message{}, err
	}

	buf := make([]byte, maxDNSMessageSize)
	n, err := conn.Read(buf)
	if err != nil {
		return dnsmessage.Message{}, err
	}

	var resp dnsmessage.Message
	if err := resp.Unpack(buf[:n]); err != nil {
		return dnsmessage.Message{}, err
	}

	if resp.ID != queryID {
		return dnsmessage.Message{}, fmt.Errorf("response ID %d does not match query ID %d", resp.ID, queryID)
	}

	return resp, nil
}
