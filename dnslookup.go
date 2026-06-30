package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

var dnsTypes = map[string]uint16{
	"A": 1, "NS": 2, "CNAME": 5, "SOA": 6,
	"MX": 15, "TXT": 16, "AAAA": 28, "SRV": 33, "PTR": 12,
}

var dnsTypeNames = map[uint16]string{
	1: "A", 2: "NS", 5: "CNAME", 6: "SOA",
	12: "PTR", 15: "MX", 16: "TXT", 28: "AAAA", 33: "SRV",
}

func encodeDNSName(name string) []byte {
	var buf []byte
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		buf = append(buf, 0)
		return buf
	}
	for _, label := range strings.Split(name, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0)
	return buf
}

func readDNSName(data []byte, offset int) (string, int) {
	return readDNSNameDepth(data, offset, 0)
}

func readDNSNameDepth(data []byte, offset int, depth int) (string, int) {
	if depth > 10 || offset >= len(data) {
		return "", offset
	}
	var labels []string
	for {
		if offset >= len(data) {
			return "", offset
		}
		length := data[offset]
		if length == 0 {
			offset++
			break
		}
		if length&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				return "", offset + 2
			}
			ptr := int(binary.BigEndian.Uint16(data[offset:offset+2]) & 0x3FFF)
			resolved, _ := readDNSNameDepth(data, ptr, depth+1)
			labels = append(labels, resolved)
			offset += 2
			return strings.Join(labels, "."), offset
		}
		offset++
		if offset+int(length) > len(data) {
			return "", offset
		}
		labels = append(labels, string(data[offset:offset+int(length)]))
		offset += int(length)
	}
	return strings.Join(labels, "."), offset
}

func buildQuery(name string, qtype uint16) []byte {
	var id [2]byte
	rand.Read(id[:])
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], binary.BigEndian.Uint16(id[:]))
	binary.BigEndian.PutUint16(header[2:4], 0x0100)
	binary.BigEndian.PutUint16(header[4:6], 1)
	binary.BigEndian.PutUint16(header[10:12], 1)
	question := encodeDNSName(name)
	question = binary.BigEndian.AppendUint16(question, qtype)
	question = binary.BigEndian.AppendUint16(question, 1)
	opt := []byte{0x00, 0x00, 0x29, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	return append(header, append(question, opt...)...)
}

type dnsAnswer struct {
	name  string
	rtype uint16
	ttl   uint32
	rdata []byte
	rdoff int // offset of rdata within the full response (for name decompression)
}

func parseDNSResponse(data []byte) ([]dnsAnswer, bool, error) {
	if len(data) < 12 {
		return nil, false, fmt.Errorf("response too short")
	}
	truncated := data[2]&0x08 != 0
	rcode := data[3] & 0x0F
	if rcode == 3 {
		return nil, truncated, fmt.Errorf("NXDOMAIN")
	}
	if rcode != 0 {
		return nil, truncated, fmt.Errorf("DNS response code: %d", rcode)
	}
	ancount := int(binary.BigEndian.Uint16(data[6:8]))
	offset := 12
	if offset >= len(data) {
		return nil, truncated, fmt.Errorf("truncated question section")
	}
	_, offset = readDNSName(data, offset)
	if offset+4 > len(data) {
		return nil, truncated, fmt.Errorf("truncated question type/class")
	}
	offset += 4
	var answers []dnsAnswer
	for i := 0; i < ancount; i++ {
		if offset >= len(data) {
			return answers, truncated, nil
		}
		name, newOffset := readDNSName(data, offset)
		if newOffset+10 > len(data) {
			return answers, truncated, nil
		}
		rtype := binary.BigEndian.Uint16(data[newOffset : newOffset+2])
		if rtype == 41 {
			rdlength := binary.BigEndian.Uint16(data[newOffset+8 : newOffset+10])
			offset = newOffset + 10 + int(rdlength)
			continue
		}
		ttl := binary.BigEndian.Uint32(data[newOffset+4 : newOffset+8])
		rdlength := binary.BigEndian.Uint16(data[newOffset+8 : newOffset+10])
		offset = newOffset + 10
		if offset+int(rdlength) > len(data) {
			return answers, truncated, nil
		}
		rdata := data[offset : offset+int(rdlength)]
		rdoff := offset
		offset += int(rdlength)
		answers = append(answers, dnsAnswer{
			name:  name,
			rtype: rtype,
			ttl:   ttl,
			rdata: rdata,
			rdoff: rdoff,
		})
	}
	return answers, truncated, nil
}

func rdataToString(rtype uint16, rdata []byte, fullData []byte, rdoff int) (string, error) {
	switch rtype {
	case 1:
		if len(rdata) != 4 {
			return "", fmt.Errorf("bad A length: %d", len(rdata))
		}
		return net.IP(rdata).String(), nil
	case 28:
		if len(rdata) != 16 {
			return "", fmt.Errorf("bad AAAA length: %d", len(rdata))
		}
		return net.IP(rdata).String(), nil
	case 2, 5, 12:
		name, _ := readDNSName(fullData, rdoff)
		return strings.TrimSuffix(name, "."), nil
	case 15:
		if len(rdata) < 2 {
			return "", fmt.Errorf("bad MX length: %d", len(rdata))
		}
		pref := binary.BigEndian.Uint16(rdata[0:2])
		target := "."
		if len(rdata) > 2 {
			target, _ = readDNSName(fullData, rdoff+2)
			target = strings.TrimSuffix(target, ".")
		}
		return fmt.Sprintf("%d %s", pref, target), nil
	case 6:
		mname, off := readDNSName(fullData, rdoff)
		rname, off2 := readDNSName(fullData, off)
		if off2+20 > len(fullData) {
			return "", fmt.Errorf("bad SOA record")
		}
		serial := binary.BigEndian.Uint32(fullData[off2 : off2+4])
		refresh := binary.BigEndian.Uint32(fullData[off2+4 : off2+8])
		retry := binary.BigEndian.Uint32(fullData[off2+8 : off2+12])
		expire := binary.BigEndian.Uint32(fullData[off2+12 : off2+16])
		min := binary.BigEndian.Uint32(fullData[off2+16 : off2+20])
		return fmt.Sprintf("%s %s %d %d %d %d %d", mname, rname, serial, refresh, retry, expire, min), nil
	case 16:
		var parts []string
		o := 0
		for o < len(rdata) {
			l := int(rdata[o])
			o++
			if o+l > len(rdata) {
				parts = append(parts, string(rdata[o:]))
				break
			}
			parts = append(parts, string(rdata[o:o+l]))
			o += l
		}
		return strings.Join(parts, ""), nil
	case 33:
		if len(rdata) < 6 {
			return "", fmt.Errorf("bad SRV length: %d", len(rdata))
		}
		prio := binary.BigEndian.Uint16(rdata[0:2])
		weight := binary.BigEndian.Uint16(rdata[2:4])
		port := binary.BigEndian.Uint16(rdata[4:6])
		target, _ := readDNSName(fullData, rdoff+6)
		target = strings.TrimSuffix(target, ".")
		return fmt.Sprintf("%d %d %d %s", prio, weight, port, target), nil
	default:
		return "", fmt.Errorf("unsupported rtype: %d", rtype)
	}
}

func getSystemNameservers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return []string{"1.1.1.1"}
	}
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			s := strings.TrimSpace(line[len("nameserver "):])
			servers = append(servers, s)
		}
	}
	if len(servers) == 0 {
		return []string{"1.1.1.1"}
	}
	return servers
}

func reverseName(ip string) (string, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP: %s", ip)
	}
	if v4 := parsed.To4(); v4 != nil {
		parts := strings.Split(v4.String(), ".")
		return fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", parts[3], parts[2], parts[1], parts[0]), nil
	}
	v6 := parsed.To16()
	var hexParts []string
	for i := 15; i >= 0; i-- {
		hexParts = append(hexParts,
			fmt.Sprintf("%x", v6[i]&0x0f),
			fmt.Sprintf("%x", (v6[i]>>4)&0x0f))
	}
	return strings.Join(hexParts, ".") + ".ip6.arpa", nil
}

func resolveRaw(name string, qtype uint16, server string, port int) ([]DNSRecord, error) {
	addr := net.JoinHostPort(server, fmt.Sprintf("%d", port))
	query := buildQuery(name, qtype)

	var answers []dnsAnswer
	var resp []byte

	// Try UDP first
	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(query); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write failed: %w", err)
	}
	udpResp := make([]byte, 4096)
	n, err := conn.Read(udpResp)
	conn.Close()
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	resp = udpResp[:n]
	answers, truncated, err := parseDNSResponse(resp)
	if err != nil {
		return nil, err
	}

	// If truncated, retry over TCP
	if truncated {
		tcpAnswers, tcpResp, tcpErr := resolveRawTCP(name, qtype, server, port)
		if tcpErr == nil {
			answers = tcpAnswers
			resp = tcpResp
		}
	}

	return answersToRecords(answers, resp, name), nil
}

func resolveRawTCP(name string, qtype uint16, server string, port int) ([]dnsAnswer, []byte, error) {
	addr := net.JoinHostPort(server, fmt.Sprintf("%d", port))
	query := buildQuery(name, qtype)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("TCP connection failed: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Write length-prefixed query
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(query)))
	if _, err := conn.Write(lenBuf); err != nil {
		return nil, nil, fmt.Errorf("TCP write length failed: %w", err)
	}
	if _, err := conn.Write(query); err != nil {
		return nil, nil, fmt.Errorf("TCP write failed: %w", err)
	}

	// Read length prefix
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, nil, fmt.Errorf("TCP read length failed: %w", err)
	}
	respLen := int(binary.BigEndian.Uint16(lenBuf))
	if respLen == 0 || respLen > 65535 {
		return nil, nil, fmt.Errorf("invalid TCP response length: %d", respLen)
	}

	resp := make([]byte, respLen)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, nil, fmt.Errorf("TCP read failed: %w", err)
	}

	answers, _, err := parseDNSResponse(resp)
	if err != nil {
		return nil, nil, err
	}
	return answers, resp, nil
}

func answersToRecords(answers []dnsAnswer, resp []byte, name string) []DNSRecord {
	var records []DNSRecord
	for _, a := range answers {
		value, err := rdataToString(a.rtype, a.rdata, resp, a.rdoff)
		if err != nil {
			continue
		}
		typeName := dnsTypeNames[a.rtype]
		if typeName == "" {
			continue
		}
		records = append(records, DNSRecord{
			Type:  typeName,
			Name:  name,
			Value: value,
			TTL:   int(a.ttl),
		})
	}
	return records
}

func dnsLookup(name string, rtype string, server string, port int) ([]DNSRecord, error) {
	name = strings.TrimSpace(name)
	displayName := strings.TrimSuffix(name, ".")
	queryName := displayName

	if (rtype == "PTR" || rtype == "ALL") && net.ParseIP(displayName) != nil {
		rev, err := reverseName(displayName)
		if err != nil {
			return nil, err
		}
		displayName = rev
		queryName = rev
	}
	var servers []string
	if server != "" {
		servers = []string{server}
	} else {
		servers = getSystemNameservers()
	}
	if rtype == "ALL" {
		var all []DNSRecord
		typeList := []string{"A", "AAAA", "MX", "NS", "CNAME", "TXT", "SOA"}
		for _, t := range typeList {
			qt, ok := dnsTypes[t]
			if !ok {
				continue
			}
			var lastErr error
			for _, s := range servers {
				records, err := resolveRaw(queryName, qt, s, port)
				if err != nil {
					lastErr = err
					continue
				}
				all = append(all, records...)
				break
			}
			_ = lastErr
		}
		if len(all) == 0 {
			return nil, fmt.Errorf("no records found for %s", displayName)
		}
		return dedupRecords(all), nil
	}
	qt, ok := dnsTypes[rtype]
	if !ok {
		return nil, fmt.Errorf("unsupported record type: %s", rtype)
	}
	var lastErr error
	for _, s := range servers {
		records, err := resolveRaw(queryName, qt, s, port)
		if err != nil {
			lastErr = err
			continue
		}
		if len(records) == 0 {
			return nil, fmt.Errorf("no %s records found for %s", rtype, displayName)
		}
		return records, nil
	}
	return nil, fmt.Errorf("lookup failed: %w", lastErr)
}

func dedupRecords(records []DNSRecord) []DNSRecord {
	seen := make(map[string]bool)
	var out []DNSRecord
	for _, r := range records {
		key := r.Type + "\x00" + r.Name + "\x00" + r.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}
