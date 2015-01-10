package main

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
)

var dnsClient = &dns.Client{}

// Query the given nameserver for all domains
func resolveDomains(nameserver string) (results resultMap, err error) {
	results = make(resultMap)

	for _, domain := range domains {
		result, err := resolve(nameserver, domain)
		if err != nil {
			return nil, err
		}
		results[domain] = result
	}

	return results, nil
}

// Query the given nameserver for a single domain
func resolve(nameserver string, domain string) (records stringSet, err error) {
	m := &dns.Msg{}
	m.RecursionDesired = true
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)

	// execute the query
	r, _, err := dnsClient.Exchange(m, net.JoinHostPort(nameserver, "53"))
	if r == nil {
		// network problem or timeout
		return nil, simplifyError(err)
	}

	// NXDomain rcode?
	if r.Rcode == dns.RcodeNameError {
		return nil, nil
	}

	// Other erroneous rcode?
	if r.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("%v for %s", dns.RcodeToString[r.Rcode], domain)
	}

	records = make(stringSet)

	// Add addresses to set
	for _, a := range r.Answer {
		if record, ok := a.(*dns.A); ok {
			records.add(record.A.String())
		}
	}

	return records, nil
}

func ptrName(address string) string {
	reverse, err := dns.ReverseAddr(address)
	if err != nil {
		return ""
	}

	m := &dns.Msg{}
	m.RecursionDesired = true
	m.SetQuestion(reverse, dns.TypePTR)

	// execute the query
	r, _, err := dnsClient.Exchange(m, net.JoinHostPort(referenceNameserver, "53"))
	if r == nil {
		return ""
	}

	// Other erroneous rcode?
	if r.Rcode != dns.RcodeSuccess {
		return ""
	}

	// Add addresses to set
	for _, a := range r.Answer {
		if record, ok := a.(*dns.PTR); ok {
			return record.Ptr
		}
	}
	return ""
}