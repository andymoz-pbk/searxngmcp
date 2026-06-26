//go:build integration

package main

import (
	"strings"
	"testing"
)

func TestIntegration_DNSLookup_ibmcom(t *testing.T) {
	for _, rt := range []string{"A", "AAAA"} {
		records, err := dnsLookup("ibm.com", rt, "", 53)
		if err != nil {
			t.Fatalf("ibm.com %s lookup failed: %v", rt, err)
		}
		if len(records) == 0 {
			t.Fatalf("ibm.com %s: no records returned", rt)
		}
		for _, r := range records {
			t.Logf("ibm.com %s = %s (TTL %d)", rt, r.Value, r.TTL)
		}
	}
}

func TestIntegration_DNSLookup_wwwibmcom(t *testing.T) {
	for _, rt := range []string{"A", "AAAA"} {
		records, err := dnsLookup("www.ibm.com", rt, "", 53)
		if err != nil {
			t.Fatalf("www.ibm.com %s lookup failed: %v", rt, err)
		}
		if len(records) == 0 {
			t.Fatalf("www.ibm.com %s: no records returned", rt)
		}
		for _, r := range records {
			t.Logf("www.ibm.com %s = %s (TTL %d)", rt, r.Value, r.TTL)
		}
	}
}

func TestIntegration_DNSLookup_googlecom(t *testing.T) {
	for _, rt := range []string{"A", "AAAA"} {
		records, err := dnsLookup("google.com", rt, "", 53)
		if err != nil {
			t.Fatalf("google.com %s lookup failed: %v", rt, err)
		}
		if len(records) == 0 {
			t.Fatalf("google.com %s: no records returned", rt)
		}
		for _, r := range records {
			t.Logf("google.com %s = %s (TTL %d)", rt, r.Value, r.TTL)
		}
	}
}

func TestIntegration_DNSLookup_wwwgooglecom(t *testing.T) {
	for _, rt := range []string{"A", "AAAA"} {
		records, err := dnsLookup("www.google.com", rt, "", 53)
		if err != nil {
			t.Fatalf("www.google.com %s lookup failed: %v", rt, err)
		}
		if len(records) == 0 {
			t.Fatalf("www.google.com %s: no records returned", rt)
		}
		for _, r := range records {
			t.Logf("www.google.com %s = %s (TTL %d)", rt, r.Value, r.TTL)
		}
	}
}

func TestIntegration_DNSLookup_ibmcom_MX(t *testing.T) {
	records, err := dnsLookup("ibm.com", "MX", "", 53)
	if err != nil {
		t.Fatalf("ibm.com MX lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("ibm.com MX: no records returned")
	}
	for _, r := range records {
		t.Logf("ibm.com MX = %s (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_googlecom_MX(t *testing.T) {
	records, err := dnsLookup("google.com", "MX", "", 53)
	if err != nil {
		t.Fatalf("google.com MX lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("google.com MX: no records returned")
	}
	for _, r := range records {
		t.Logf("google.com MX = %s (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_ibmcom_NS(t *testing.T) {
	records, err := dnsLookup("ibm.com", "NS", "", 53)
	if err != nil {
		t.Fatalf("ibm.com NS lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("ibm.com NS: no records returned")
	}
	for _, r := range records {
		t.Logf("ibm.com NS = %s (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_googlecom_NS(t *testing.T) {
	records, err := dnsLookup("google.com", "NS", "", 53)
	if err != nil {
		t.Fatalf("google.com NS lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("google.com NS: no records returned")
	}
	for _, r := range records {
		t.Logf("google.com NS = %s (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_ibmcom_SOA(t *testing.T) {
	records, err := dnsLookup("ibm.com", "SOA", "", 53)
	if err != nil {
		t.Fatalf("ibm.com SOA lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("ibm.com SOA: no records returned")
	}
	for _, r := range records {
		t.Logf("ibm.com SOA = %s (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_googlecom_SOA(t *testing.T) {
	records, err := dnsLookup("google.com", "SOA", "", 53)
	if err != nil {
		t.Fatalf("google.com SOA lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("google.com SOA: no records returned")
	}
	for _, r := range records {
		t.Logf("google.com SOA = %s (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_ibmcom_TXT(t *testing.T) {
	records, err := dnsLookup("ibm.com", "TXT", "", 53)
	if err != nil {
		t.Fatalf("ibm.com TXT lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("ibm.com TXT: no records returned")
	}
	for _, r := range records {
		t.Logf("ibm.com TXT = %q (TTL %d)", r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_ibmcom_ALL(t *testing.T) {
	records, err := dnsLookup("ibm.com", "ALL", "", 53)
	if err != nil {
		t.Fatalf("ibm.com ALL lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("ibm.com ALL: no records returned")
	}
	for _, r := range records {
		t.Logf("ibm.com ALL: %s %s (TTL %d)", r.Type, r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_googlecom_ALL(t *testing.T) {
	records, err := dnsLookup("google.com", "ALL", "", 53)
	if err != nil {
		t.Fatalf("google.com ALL lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("google.com ALL: no records returned")
	}
	for _, r := range records {
		t.Logf("google.com ALL: %s %s (TTL %d)", r.Type, r.Value, r.TTL)
	}
}

func TestIntegration_DNSLookup_ibmcom_LeadingSpace(t *testing.T) {
	records, err := dnsLookup(" ibm.com", "A", "", 53)
	if err != nil {
		t.Fatalf("' ibm.com' (leading space) lookup failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("' ibm.com' A: no records returned")
	}
	t.Logf("' ibm.com' A = %s (TTL %d)", records[0].Value, records[0].TTL)
}

func TestIntegration_DNSLookup_AllDomains_IPv4Only(t *testing.T) {
	domains := []string{"ibm.com", "www.ibm.com", "google.com", "www.google.com"}
	for _, d := range domains {
		records, err := dnsLookup(d, "A", "", 53)
		if err != nil {
			t.Errorf("%s A lookup failed: %v", d, err)
			continue
		}
		if len(records) == 0 {
			t.Errorf("%s A: no records", d)
			continue
		}
		for _, r := range records {
			if !strings.Contains(r.Value, ".") {
				t.Errorf("%s A value %q does not look like an IPv4 address", d, r.Value)
			}
		}
	}
}

func TestIntegration_DNSLookup_CustomServer(t *testing.T) {
	records, err := dnsLookup("google.com", "A", "8.8.8.8", 53)
	if err != nil {
		t.Fatalf("google.com A via 8.8.8.8 failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("google.com A via 8.8.8.8: no records")
	}
	t.Logf("google.com A via 8.8.8.8 = %s", records[0].Value)
}
