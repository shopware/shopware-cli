package proxy

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// DNSPort is the fixed local port the embedded DNS server listens on. A high
// port is used so the daemon does not need elevated privileges; the OS
// resolver configuration points at it explicitly.
const DNSPort = 53535

// dnsTTL is deliberately short so teardown or config changes propagate
// quickly to resolvers and browsers.
const dnsTTL = 5

// maxDNSMessageSize is the classic UDP DNS message limit; our queries and
// single-record answers fit comfortably.
const maxDNSMessageSize = 512

// RunDNSServer blocks, answering every A query under baseDomain with
// 127.0.0.1 on UDP 127.0.0.1:<dnsPort>. AAAA queries under the domain get an
// empty NOERROR answer (avoiding IPv6 fallback delays in browsers); anything
// outside the domain gets NXDOMAIN. It is intended to run inside the
// detached "project proxy internal-dns-serve" child process.
func RunDNSServer(ctx context.Context, dnsPort int, baseDomain string) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: dnsPort})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	domain := strings.ToLower(baseDomain)
	buf := make([]byte, maxDNSMessageSize)

	// Queries are answered sequentially: every answer is computed from two
	// string comparisons, so there is nothing to gain from concurrency.
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // the read failed because ctx closed the socket: clean shutdown
			}

			return err
		}

		if resp := buildDNSResponse(buf[:n], domain); resp != nil {
			_, _ = conn.WriteToUDP(resp, addr)
		}
	}
}

// buildDNSResponse answers a single DNS query according to the zone rules.
// Malformed packets yield nil (dropped, like any DNS server would).
func buildDNSResponse(query []byte, domain string) []byte {
	var parser dnsmessage.Parser

	header, err := parser.Start(query)
	if err != nil || header.Response {
		return nil
	}

	question, err := parser.Question()
	if err != nil {
		return nil
	}

	name := strings.ToLower(strings.TrimSuffix(question.Name.String(), "."))
	inZone := name == domain || strings.HasSuffix(name, "."+domain)

	respHeader := dnsmessage.Header{
		ID:               header.ID,
		Response:         true,
		Authoritative:    true,
		RecursionDesired: header.RecursionDesired,
	}
	if !inZone {
		respHeader.RCode = dnsmessage.RCodeNameError
	}

	builder := dnsmessage.NewBuilder(make([]byte, 0, maxDNSMessageSize), respHeader)
	builder.EnableCompression()

	if err := builder.StartQuestions(); err != nil {
		return nil
	}
	if err := builder.Question(question); err != nil {
		return nil
	}

	if inZone && question.Type == dnsmessage.TypeA && question.Class == dnsmessage.ClassINET {
		if err := builder.StartAnswers(); err != nil {
			return nil
		}

		err := builder.AResource(
			dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: dnsTTL},
			dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
		)
		if err != nil {
			return nil
		}
	}

	out, err := builder.Finish()
	if err != nil {
		return nil
	}

	return out
}

// queryDNS sends a single question to the DNS server at addr and returns the
// parsed response. It is used by `proxy verify` (and the tests) to probe the
// embedded server directly.
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
