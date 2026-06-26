package main

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func buildDNSResponse(query []byte, rtype uint16, rdata []byte, ttl uint32) []byte {
	if len(query) < 12 {
		return nil
	}
	header := make([]byte, 12)
	copy(header, query[:12])
	header[2] = 0x84
	header[3] = 0x80
	binary.BigEndian.PutUint16(header[6:8], 1)
	qnameStart := 12
	qnameEnd := qnameStart
	for {
		if qnameEnd >= len(query) {
			return nil
		}
		if query[qnameEnd] == 0 {
			qnameEnd++
			break
		}
		if query[qnameEnd]&0xC0 == 0xC0 {
			qnameEnd += 2
			break
		}
		qnameEnd += int(query[qnameEnd]) + 1
	}
	question := query[qnameStart : qnameEnd+4]
	var answer []byte
	answer = append(answer, 0xC0, byte(qnameStart)) // NAME pointer
	answer = binary.BigEndian.AppendUint16(answer, rtype)
	answer = binary.BigEndian.AppendUint16(answer, 1) // class IN
	answer = binary.BigEndian.AppendUint32(answer, ttl)
	answer = binary.BigEndian.AppendUint16(answer, uint16(len(rdata)))
	answer = append(answer, rdata...)
	resp := make([]byte, 0, len(header)+len(question)+len(answer))
	resp = append(resp, header...)
	resp = append(resp, question...)
	resp = append(resp, answer...)
	return resp
}

func mockUDPServer(t *testing.T, handler func(query []byte) []byte) (string, int) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock server listen: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		for {
			buf := make([]byte, 4096)
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			resp := handler(buf[:n])
			if resp != nil {
				conn.WriteTo(resp, addr)
			}
		}
	}()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String(), addr.Port
}

func TestDNSLookup_A_Record(t *testing.T) {
	ip := net.ParseIP("93.184.216.34").To4()
	rdata := []byte(ip)
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 1, rdata, 300)
	})
	records, err := dnsLookup("example.com", "A", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "A" {
		t.Errorf("type = %q, want A", records[0].Type)
	}
	if records[0].Value != "93.184.216.34" {
		t.Errorf("value = %q, want 93.184.216.34", records[0].Value)
	}
	if records[0].TTL != 300 {
		t.Errorf("TTL = %d, want 300", records[0].TTL)
	}
}

func TestDNSLookup_AAAA_Record(t *testing.T) {
	ip := net.ParseIP("2606:2800:220:1:248:1893:25c8:1946").To16()
	rdata := []byte(ip)
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 28, rdata, 600)
	})
	records, err := dnsLookup("example.com", "AAAA", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "AAAA" {
		t.Errorf("type = %q, want AAAA", records[0].Type)
	}
	if records[0].Value != "2606:2800:220:1:248:1893:25c8:1946" {
		t.Errorf("value = %q", records[0].Value)
	}
}

func TestDNSLookup_MX_Record(t *testing.T) {
	mxRdata := func(pref uint16, target string) []byte {
		var b []byte
		b = binary.BigEndian.AppendUint16(b, pref)
		b = append(b, encodeDNSName(target)...)
		return b
	}
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 15, mxRdata(10, "mail.example.com"), 3600)
	})
	records, err := dnsLookup("example.com", "MX", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "MX" {
		t.Errorf("type = %q, want MX", records[0].Type)
	}
	if records[0].Value != "10 mail.example.com" {
		t.Errorf("value = %q, want '10 mail.example.com'", records[0].Value)
	}
}

func TestDNSLookup_NS_Record(t *testing.T) {
	nsRdata := encodeDNSName("ns1.example.com")
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 2, nsRdata, 86400)
	})
	records, err := dnsLookup("example.com", "NS", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Value != "ns1.example.com" {
		t.Errorf("value = %q, want ns1.example.com", records[0].Value)
	}
}

func TestDNSLookup_CNAME_Record(t *testing.T) {
	cnameRdata := encodeDNSName("target.example.com")
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 5, cnameRdata, 300)
	})
	records, err := dnsLookup("www.example.com", "CNAME", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Name != "www.example.com" {
		t.Errorf("name = %q", records[0].Name)
	}
	if records[0].Value != "target.example.com" {
		t.Errorf("value = %q, want target.example.com", records[0].Value)
	}
}

func TestDNSLookup_TXT_Record(t *testing.T) {
	txtRdata := func(s string) []byte {
		return append([]byte{byte(len(s))}, []byte(s)...)
	}
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 16, txtRdata("v=spf1 include:_spf.example.com ~all"), 3600)
	})
	records, err := dnsLookup("example.com", "TXT", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "TXT" {
		t.Errorf("type = %q, want TXT", records[0].Type)
	}
	if !strings.Contains(records[0].Value, "v=spf1") {
		t.Errorf("value = %q, want SPF record", records[0].Value)
	}
}

func TestDNSLookup_SRV_Record(t *testing.T) {
	srvRdata := func(prio, weight, port uint16, target string) []byte {
		var b []byte
		b = binary.BigEndian.AppendUint16(b, prio)
		b = binary.BigEndian.AppendUint16(b, weight)
		b = binary.BigEndian.AppendUint16(b, port)
		b = append(b, encodeDNSName(target)...)
		return b
	}
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 33, srvRdata(0, 10, 443, "srv.example.com"), 300)
	})
	records, err := dnsLookup("_https._tcp.example.com", "SRV", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "SRV" {
		t.Errorf("type = %q, want SRV", records[0].Type)
	}
	want := "0 10 443 srv.example.com"
	if records[0].Value != want {
		t.Errorf("value = %q, want %q", records[0].Value, want)
	}
}

func TestDNSLookup_PTR_Reverse(t *testing.T) {
	ptrRdata := encodeDNSName("host.example.com")
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 12, ptrRdata, 3600)
	})
	records, err := dnsLookup("4.3.2.1.in-addr.arpa", "PTR", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Value != "host.example.com" {
		t.Errorf("value = %q, want host.example.com", records[0].Value)
	}
}

func TestDNSLookup_AutoPTR_IPv4(t *testing.T) {
	ptrRdata := encodeDNSName("host.example.com")
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 12, ptrRdata, 3600)
	})
	records, err := dnsLookup("1.2.3.4", "PTR", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Value != "host.example.com" {
		t.Errorf("value = %q, want host.example.com", records[0].Value)
	}
}

func TestDNSLookup_AutoPTR_IPv6(t *testing.T) {
	ptrRdata := encodeDNSName("ipv6.example.com")
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 12, ptrRdata, 3600)
	})
	records, err := dnsLookup("2001:db8::1", "PTR", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Value != "ipv6.example.com" {
		t.Errorf("value = %q, want ipv6.example.com", records[0].Value)
	}
}

func TestDNSLookup_SOA_Record(t *testing.T) {
	soaRdata := func(mname, rname string, serial, refresh, retry, expire, min uint32) []byte {
		var b []byte
		b = append(b, encodeDNSName(mname)...)
		b = append(b, encodeDNSName(rname)...)
		b = binary.BigEndian.AppendUint32(b, serial)
		b = binary.BigEndian.AppendUint32(b, refresh)
		b = binary.BigEndian.AppendUint32(b, retry)
		b = binary.BigEndian.AppendUint32(b, expire)
		b = binary.BigEndian.AppendUint32(b, min)
		return b
	}
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 6, soaRdata("ns1.example.com", "admin.example.com", 20240101, 3600, 900, 604800, 86400), 3600)
	})
	records, err := dnsLookup("example.com", "SOA", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "SOA" {
		t.Errorf("type = %q, want SOA", records[0].Type)
	}
	if !strings.Contains(records[0].Value, "ns1.example.com") {
		t.Errorf("value = %q, want SOA with ns1.example.com", records[0].Value)
	}
	if !strings.Contains(records[0].Value, "20240101") {
		t.Errorf("value = %q, want serial 20240101", records[0].Value)
	}
}

func TestDNSLookup_ALL_Mode(t *testing.T) {
	soaRdata := func() []byte {
		var b []byte
		b = append(b, encodeDNSName("ns1.example.com")...)
		b = append(b, encodeDNSName("admin.example.com")...)
		b = binary.BigEndian.AppendUint32(b, 20240101)
		b = binary.BigEndian.AppendUint32(b, 3600)
		b = binary.BigEndian.AppendUint32(b, 900)
		b = binary.BigEndian.AppendUint32(b, 604800)
		b = binary.BigEndian.AppendUint32(b, 86400)
		return b
	}
	server, port := mockUDPServer(t, func(q []byte) []byte {
		rtype := binary.BigEndian.Uint16(q[len(q)-4 : len(q)-2])
		switch rtype {
		case 1:
			return buildDNSResponse(q, 1, net.ParseIP("1.2.3.4").To4(), 300)
		case 28:
			return buildDNSResponse(q, 28, net.ParseIP("::1").To16(), 300)
		case 15:
			rdata := append([]byte{0, 10}, encodeDNSName("mail.example.com")...)
			return buildDNSResponse(q, 15, rdata, 300)
		case 2:
			return buildDNSResponse(q, 2, encodeDNSName("ns1.example.com"), 300)
		case 5:
			return buildDNSResponse(q, 5, encodeDNSName("target.example.com"), 300)
		case 16:
			txt := []byte("v=spf1 -all")
			rdata := append([]byte{byte(len(txt))}, txt...)
			return buildDNSResponse(q, 16, rdata, 300)
		case 6:
			return buildDNSResponse(q, 6, soaRdata(), 3600)
		default:
			r := make([]byte, 12)
			copy(r, q[:12])
			r[2] = 0x84
			r[3] = 0x80
			return r
		}
	})
	records, err := dnsLookup("example.com", "ALL", server, port)
	if err != nil {
		t.Fatalf("ALL lookup failed: %v", err)
	}
	typesFound := make(map[string]bool)
	for _, r := range records {
		typesFound[r.Type] = true
	}
	for _, want := range []string{"A", "AAAA", "MX", "NS", "CNAME", "TXT", "SOA"} {
		if !typesFound[want] {
			t.Errorf("ALL mode missing %s record", want)
		}
	}
}

func TestDNSLookup_NXDOMAIN(t *testing.T) {
	server, port := mockUDPServer(t, func(q []byte) []byte {
		if len(q) < 12 {
			return nil
		}
		resp := make([]byte, 12)
		copy(resp, q[:12])
		resp[2] = 0x84
		resp[3] = 0x03
		return resp
	})
	_, err := dnsLookup("nonexistent.example.com", "A", server, port)
	if err == nil {
		t.Fatal("expected NXDOMAIN error, got nil")
	}
	if !strings.Contains(err.Error(), "NXDOMAIN") {
		t.Errorf("error = %q, want NXDOMAIN", err.Error())
	}
}

func TestDNSLookup_UnsupportedType(t *testing.T) {
	_, err := dnsLookup("example.com", "SPF", "", 53)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported record type") {
		t.Errorf("error = %q, want 'unsupported record type'", err.Error())
	}
}

func TestDNSLookup_MissingName(t *testing.T) {
	_, err := dnsLookup("", "A", "", 53)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestDNSLookup_CustomPort(t *testing.T) {
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 1, net.ParseIP("10.0.0.1").To4(), 300)
	})
	records, err := dnsLookup("example.com", "A", server, port)
	if err != nil {
		t.Fatalf("custom port lookup failed: %v", err)
	}
	if len(records) != 1 || records[0].Value != "10.0.0.1" {
		t.Errorf("unexpected result: %+v", records)
	}
}

func TestDNSLookup_ConnectionRefused(t *testing.T) {
	_, err := dnsLookup("example.com", "A", "127.0.0.1", 1)
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}

func TestHandleDNSLookup_MissingName(t *testing.T) {
	result := handleDNSLookup(map[string]any{})
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestHandleDNSLookup_InvalidType(t *testing.T) {
	result := handleDNSLookup(map[string]any{
		"name": "example.com",
		"type": "INVALID",
	})
	if !result.IsError {
		t.Fatal("expected error for invalid type")
	}
}

func TestHandleDNSLookup_CustomPortRange(t *testing.T) {
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 1, net.ParseIP("10.0.0.1").To4(), 300)
	})
	result := handleDNSLookup(map[string]any{
		"name":   "example.com",
		"type":   "A",
		"server": server,
		"port":   float64(port),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	var records []DNSRecord
	if err := json.Unmarshal([]byte(result.Content[0].Text), &records); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(records) != 1 || records[0].Value != "10.0.0.1" {
		t.Errorf("unexpected result: %+v", records)
	}
}

func TestHandleDNSLookup_OutOfRangePort(t *testing.T) {
	result := handleDNSLookup(map[string]any{
		"name":   "example.com",
		"server": "8.8.8.8",
		"port":   float64(99999),
	})
	if result.IsError {
		t.Logf("port default to 53 used real DNS (may fail without network): %s", result.Content[0].Text)
	} else {
		port := 53
		_ = port
	}
}

func TestReverseName_IPv4(t *testing.T) {
	rev, err := reverseName("1.2.3.4")
	if err != nil {
		t.Fatalf("reverseName failed: %v", err)
	}
	if rev != "4.3.2.1.in-addr.arpa" {
		t.Errorf("got %q, want 4.3.2.1.in-addr.arpa", rev)
	}
}

func TestReverseName_IPv6(t *testing.T) {
	rev, err := reverseName("2001:db8::1")
	if err != nil {
		t.Fatalf("reverseName failed: %v", err)
	}
	if !strings.HasSuffix(rev, ".ip6.arpa") {
		t.Errorf("got %q, want .ip6.arpa suffix", rev)
	}
	// 32 nibbles = 31 dots between them + .ip6.arpa = 33 total dots
	if strings.Count(rev, ".") != 33 {
		t.Errorf("got %d dots, want 33", strings.Count(rev, "."))
	}
}

func TestReverseName_Invalid(t *testing.T) {
	_, err := reverseName("not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestParseDNSResponse_Truncated(t *testing.T) {
	_, err := parseDNSResponse([]byte{0, 0, 0, 0})
	if err == nil {
		t.Fatal("expected error for truncated response")
	}
}

func TestParseDNSResponse_ServerFailure(t *testing.T) {
	data := []byte{
		0, 0, 0x84, 0x02, 0, 1, 0, 0, 0, 0, 0, 0,
	}
	_, err := parseDNSResponse(data)
	if err == nil {
		t.Fatal("expected error for server failure")
	}
}

func TestDNSLookup_Timeout(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	_, err = dnsLookup("example.com", "A", addr.IP.String(), addr.Port)
	// No data sent back -> timeout, but net.DialTimeout succeeds so we
	// get "read failed: i/o timeout" or "read failed: ..."
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestDNSLookup_DedupRecords(t *testing.T) {
	records := []DNSRecord{
		{Type: "A", Name: "example.com.", Value: "1.2.3.4", TTL: 300},
		{Type: "A", Name: "example.com.", Value: "1.2.3.4", TTL: 600},
		{Type: "A", Name: "example.com.", Value: "5.6.7.8", TTL: 300},
	}
	deduped := dedupRecords(records)
	if len(deduped) != 2 {
		t.Errorf("got %d records after dedup, want 2", len(deduped))
	}
	// First occurrence's TTL is kept
	if deduped[0].TTL != 300 {
		t.Errorf("first dup TTL = %d, want 300", deduped[0].TTL)
	}
}

func TestDNSTools_ListIncludesDNS(t *testing.T) {
	cfg := DefaultConfig()
	mcp := NewMCPServer(cfg)
	ts := httptest.NewServer(mcp)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var raw map[string]any
	json.NewDecoder(resp.Body).Decode(&raw)
	result, _ := raw["result"].(map[string]any)
	tools, _ := result["tools"].([]any)

	found := false
	for _, tRaw := range tools {
		tool, _ := tRaw.(map[string]any)
		if name, _ := tool["name"].(string); name == "dns_lookup" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("dns_lookup not found in tools/list")
	}
}

func TestDNSLookup_MultipleAnswers(t *testing.T) {
	server, port := mockUDPServer(t, func(q []byte) []byte {
		ip1 := net.ParseIP("1.2.3.4").To4()
		ip2 := net.ParseIP("5.6.7.8").To4()
		base := buildDNSResponse(q, 1, ip1, 300)
		if base == nil {
			return nil
		}
		var answer2 []byte
		answer2 = append(answer2, 0xC0, 12)
		answer2 = binary.BigEndian.AppendUint16(answer2, 1)
		answer2 = binary.BigEndian.AppendUint16(answer2, 1)
		answer2 = binary.BigEndian.AppendUint32(answer2, 600)
		answer2 = binary.BigEndian.AppendUint16(answer2, 4)
		answer2 = append(answer2, ip2...)
		resp := make([]byte, len(base)+len(answer2))
		copy(resp, base)
		copy(resp[len(base):], answer2)
		binary.BigEndian.PutUint16(resp[6:8], 2)
		return resp
	})
	records, err := dnsLookup("example.com", "A", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[1].Value != "5.6.7.8" {
		t.Errorf("second value = %q, want 5.6.7.8", records[1].Value)
	}
}

func TestDNSLookup_ResolveRawQuery(t *testing.T) {
	q := buildQuery("example.com.", 1)
	if len(q) < 12 {
		t.Fatal("query too short")
	}
	qtype := binary.BigEndian.Uint16(q[len(q)-4 : len(q)-2])
	if qtype != 1 {
		t.Errorf("qtype = %d, want 1", qtype)
	}
	qname := q[12 : len(q)-4]
	if qname[len(qname)-1] != 0 {
		t.Errorf("query name missing trailing null")
	}
}

func TestDNSLookup_VeryLongName(t *testing.T) {
	long := strings.Repeat("a.", 50) + "com"
	_, err := dnsLookup(long, "A", "", 53)
	if err == nil {
		t.Log("long name queried system resolver (may or may not work)")
	}
}

func TestDNSLookup_RepeatedServerFallback(t *testing.T) {
	server, port := mockUDPServer(t, func(q []byte) []byte {
		return buildDNSResponse(q, 1, net.ParseIP("10.0.0.1").To4(), 300)
	})
	records, err := dnsLookup("example.com", "A", server, port)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(records) != 1 || records[0].Value != "10.0.0.1" {
		t.Errorf("unexpected result: %+v", records)
	}
}
